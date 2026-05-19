package main

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"database/sql"
	"embed"
	"encoding/hex"
	"encoding/json"
	"html/template"
	"log"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
)

//go:embed templates/*
var templateFS embed.FS

//go:embed favicon.svg
var faviconSVG []byte

type Server struct {
	store             *Store
	renderer          *Renderer
	baseURL           string
	powBits           int
	annotationPowBits int
	maxSize           int
	tmpl              *template.Template
	limiter           *RateLimiter
	mux               *http.ServeMux
	trustedProxy      string
	devMode           bool
	homeHTML          string
}

type pitchRequest struct {
	Stamp    string `json:"stamp"`
	Title    string `json:"title"`
	Slug     string `json:"slug"`
	Author   string `json:"author"`
	Markdown string `json:"markdown"`
	Expires  string `json:"expires"`
	Private  bool   `json:"private"`
	Revise   bool   `json:"revise"`
}

type pitchResponse struct {
	ID        string `json:"id"`
	URL       string `json:"url"`
	Secret    string `json:"secret,omitempty"`
	ExpiresAt string `json:"expires_at,omitempty"`
}

type pitchPage struct {
	Title           string
	Author          string
	AuthorURL       string
	HTML            template.HTML
	Created         time.Time
	Expires         *time.Time
	Views           int64
	ID              string
	RawURL          string
	BaseURL         string
	Readonly        bool
	AnnotationsJSON template.JS
	PowBits         int
	RevisionCount   int
	CurrentRevision int // 0 = latest
}

func (p pitchPage) RevisionList() []int {
	revs := make([]int, p.RevisionCount)
	for i := range revs {
		revs[i] = p.RevisionCount - i
	}
	return revs
}

func NewServer(store *Store, renderer *Renderer, baseURL string, powBits, annotationPowBits, maxSize, rateLimit int, trustedProxy string, devMode bool) *Server {
	tmpl := template.Must(template.ParseFS(templateFS, "templates/pitch.html"))

	homeHTML, err := renderer.Render([]byte(homeMarkdown))
	if err != nil {
		log.Fatalf("failed to render home page: %v", err)
	}

	// Seed the home pitch row for view counting
	if !store.PitchExists(homeID) {
		store.InsertPitch(&Pitch{
			ID:       homeID,
			Title:    "pitchbin",
			Author:   "sinitax",
			Markdown: homeMarkdown,
			HTML:     homeHTML,
			Created:  time.Date(2026, 5, 15, 0, 0, 0, 0, time.UTC).Unix(),
			Expires:  0,
		})
	}

	s := &Server{
		store:             store,
		renderer:          renderer,
		baseURL:           strings.TrimRight(baseURL, "/"),
		powBits:           powBits,
		annotationPowBits: annotationPowBits,
		maxSize:           maxSize,
		tmpl:              tmpl,
		limiter:           NewRateLimiter(rateLimit, time.Minute),
		mux:               http.NewServeMux(),
		trustedProxy:      trustedProxy,
		devMode:           devMode,
		homeHTML:          homeHTML,
	}

	s.mux.HandleFunc("GET /{$}", s.handleHome)
	s.mux.HandleFunc("GET /favicon.ico", handleFavicon)
	s.mux.HandleFunc("GET /favicon.svg", handleFavicon)
	s.mux.HandleFunc("GET /robots.txt", handleRobots)
	s.mux.HandleFunc("POST /api/pitch", s.handleSubmit)
	s.mux.HandleFunc("PUT /api/pitch/{id}", s.handleUpdatePitch)
	s.mux.HandleFunc("DELETE /api/pitch/{id}", s.handleDeletePitch)
	s.mux.HandleFunc("GET /api/info", s.handleInfo)
	s.mux.HandleFunc("GET /api/{id}/annotations", s.handleGetAnnotations)
	s.mux.HandleFunc("POST /api/{id}/annotations", s.handlePostAnnotation)
	s.mux.HandleFunc("PUT /api/{id}/annotations/{aid}", s.handleUpdateAnnotation)
	s.mux.HandleFunc("DELETE /api/{id}/annotations/{aid}", s.handleDeleteAnnotation)
	s.mux.HandleFunc("GET /q3-migration-plan", handleExampleRedirect)
	s.mux.HandleFunc("GET /auth-module-review", handleExampleRedirect)
	s.mux.HandleFunc("GET /{id}/raw", s.handleRaw)
	s.mux.HandleFunc("GET /{id}/annotated", s.handleAnnotated)
	s.mux.HandleFunc("GET /{id}", s.handleView)

	return s
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.mux.ServeHTTP(w, r)
}

func (s *Server) handleInfo(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"service": "pitchbin",
		"pow": map[string]any{
			"algorithm":      "sha256",
			"bits":           s.powBits,
			"annotation_bits": s.annotationPowBits,
			"version":        powVersion,
			"format":         "pitchbin:<version>:<unix_ts>:<random_hex>:<nonce>",
		},
	})
}

func (s *Server) handleSubmit(w http.ResponseWriter, r *http.Request) {
	ip := clientIP(r, s.trustedProxy)
	if !s.limiter.Allow(ip) {
		writeJSON(w, http.StatusTooManyRequests, map[string]string{"error": "rate limited"})
		return
	}

	var req pitchRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, int64(s.maxSize+4096))).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}

	if len(req.Markdown) == 0 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "markdown is required"})
		return
	}
	if len(req.Markdown) > s.maxSize {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "markdown too large"})
		return
	}

	// Verify proof of work
	if err := VerifyStamp(req.Stamp, s.powBits); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid proof of work: " + err.Error()})
		return
	}

	// Check replay
	stampHash := StampHash(req.Stamp)
	ok, err := s.store.UseStamp(stampHash)
	if err != nil {
		log.Printf("stamp check error: %v", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
		return
	}
	if !ok {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "stamp already used"})
		return
	}

	// Render markdown
	html, err := s.renderer.Render([]byte(req.Markdown))
	if err != nil {
		log.Printf("render error: %v", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to render markdown"})
		return
	}

	// Generate ID
	slugSource := req.Title
	if req.Slug != "" {
		slugSource = req.Slug
	}
	id, err := GenerateID(s.store, slugSource, req.Private)
	if err != nil {
		log.Printf("id generation error: %v", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to generate ID"})
		return
	}

	now := time.Now()
	expires := parseExpiry(req.Expires, now)

	// Generate edit secret
	secretBytes := make([]byte, 16)
	if _, err := rand.Read(secretBytes); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
		return
	}
	secret := hex.EncodeToString(secretBytes)
	secretHash := hashSecret(secret)

	pitch := &Pitch{
		ID:         id,
		Title:      req.Title,
		Author:     req.Author,
		Markdown:   req.Markdown,
		HTML:       html,
		Created:    now.Unix(),
		Expires:    expires,
		SecretHash: secretHash,
	}

	if err := s.store.InsertPitch(pitch); err != nil {
		log.Printf("insert error: %v", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to save pitch"})
		return
	}

	resp := pitchResponse{
		ID:     id,
		URL:    s.baseURL + "/" + id,
		Secret: secret,
	}
	if expires > 0 {
		resp.ExpiresAt = time.Unix(expires, 0).UTC().Format(time.RFC3339)
	}

	writeJSON(w, http.StatusCreated, resp)
}

func hashSecret(secret string) string {
	h := sha256.Sum256([]byte(secret))
	return hex.EncodeToString(h[:])
}

func (s *Server) verifySecret(w http.ResponseWriter, r *http.Request) (*Pitch, bool) {
	id := r.PathValue("id")
	pitch, err := s.store.GetPitch(id)
	if err == sql.ErrNoRows {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
		return nil, false
	}
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
		return nil, false
	}
	if pitch.SecretHash == "" {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "pitch has no edit secret"})
		return nil, false
	}

	secret := r.Header.Get("X-Pitch-Secret")
	if secret == "" {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "missing X-Pitch-Secret header"})
		return nil, false
	}
	if subtle.ConstantTimeCompare([]byte(hashSecret(secret)), []byte(pitch.SecretHash)) != 1 {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "invalid secret"})
		return nil, false
	}

	return pitch, true
}

func (s *Server) handleUpdatePitch(w http.ResponseWriter, r *http.Request) {
	pitch, ok := s.verifySecret(w, r)
	if !ok {
		return
	}

	var req pitchRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, int64(s.maxSize+4096))).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}

	if len(req.Markdown) == 0 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "markdown is required"})
		return
	}
	if len(req.Markdown) > s.maxSize {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "markdown too large"})
		return
	}

	html, err := s.renderer.Render([]byte(req.Markdown))
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to render markdown"})
		return
	}

	// Save current state as revision before updating
	if req.Revise {
		revCount, err := s.store.GetRevisionCount(pitch.ID)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
			return
		}
		rev := &Revision{
			PitchID:  pitch.ID,
			Revision: revCount + 1,
			Title:    pitch.Title,
			Author:   pitch.Author,
			Markdown: pitch.Markdown,
			HTML:     pitch.HTML,
			Created:  pitch.Created,
		}
		if err := s.store.InsertRevision(rev); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to save revision"})
			return
		}
	}

	if req.Title != "" {
		pitch.Title = req.Title
	}
	if req.Author != "" {
		pitch.Author = req.Author
	}
	pitch.Markdown = req.Markdown
	pitch.HTML = html
	pitch.Created = time.Now().Unix()
	if req.Expires != "" {
		pitch.Expires = parseExpiry(req.Expires, time.Now())
	}

	if err := s.store.UpdatePitch(pitch); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to update pitch"})
		return
	}

	resp := pitchResponse{
		ID:  pitch.ID,
		URL: s.baseURL + "/" + pitch.ID,
	}
	writeJSON(w, http.StatusOK, resp)
}

func (s *Server) handleDeletePitch(w http.ResponseWriter, r *http.Request) {
	pitch, ok := s.verifySecret(w, r)
	if !ok {
		return
	}

	if err := s.store.DeletePitch(pitch.ID); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to delete pitch"})
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

func (s *Server) handleView(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	pitch, err := s.store.GetPitch(id)
	if err == sql.ErrNoRows {
		http.NotFound(w, r)
		return
	}
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	// Check if expired
	if pitch.Expires > 0 && pitch.Expires < time.Now().Unix() {
		http.NotFound(w, r)
		return
	}

	revCount, _ := s.store.GetRevisionCount(pitch.ID)

	// Check for revision query parameter
	var revNum int
	var title, author, markdown, html string
	var created int64
	var readonly bool

	if revStr := r.URL.Query().Get("rev"); revStr != "" {
		revNum, err = strconv.Atoi(revStr)
		if err != nil || revNum < 1 {
			http.NotFound(w, r)
			return
		}
		rev, err := s.store.GetRevision(id, revNum)
		if err == sql.ErrNoRows {
			http.NotFound(w, r)
			return
		}
		if err != nil {
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		title, author, markdown, html, created = rev.Title, rev.Author, rev.Markdown, rev.HTML, rev.Created
		readonly = true
	} else {
		s.store.IncrementViews(id)
		title, author, markdown, html, created = pitch.Title, pitch.Author, pitch.Markdown, pitch.HTML, pitch.Created
	}

	renderedHTML := html
	if s.devMode {
		if h, renderErr := s.renderer.Render([]byte(markdown)); renderErr == nil {
			renderedHTML = h
		}
	}

	createdTime := time.Unix(created, 0).UTC()
	var expires *time.Time
	if pitch.Expires > 0 {
		t := time.Unix(pitch.Expires, 0).UTC()
		expires = &t
	}

	page := pitchPage{
		Title:           title,
		Author:          author,
		HTML:            template.HTML(renderedHTML),
		Created:         createdTime,
		Expires:         expires,
		Views:           pitch.Views + 1,
		ID:              pitch.ID,
		RawURL:          s.baseURL + "/" + pitch.ID + "/raw",
		BaseURL:         s.baseURL,
		PowBits:         s.annotationPowBits,
		Readonly:        readonly,
		RevisionCount:   revCount,
		CurrentRevision: revNum,
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := s.tmpl.Execute(w, page); err != nil {
		log.Printf("template error: %v", err)
	}
}

func (s *Server) handleRaw(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	pitch, err := s.store.GetPitch(id)
	if err == sql.ErrNoRows {
		http.NotFound(w, r)
		return
	}
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	if pitch.Expires > 0 && pitch.Expires < time.Now().Unix() {
		http.NotFound(w, r)
		return
	}

	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Write([]byte(pitch.Markdown))
}

func (s *Server) handleAnnotated(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	pitch, err := s.store.GetPitch(id)
	if err == sql.ErrNoRows {
		http.NotFound(w, r)
		return
	}
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	if pitch.Expires > 0 && pitch.Expires < time.Now().Unix() {
		http.NotFound(w, r)
		return
	}

	annotations, err := s.store.GetAnnotations(id)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	result := toCriticMarkup(pitch.Markdown, annotations)

	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Write([]byte(result))
}

// toCriticMarkup inserts CriticMarkup comment markers into markdown
// for each annotation, matching by quote text.
func toCriticMarkup(markdown string, annotations []Annotation) string {
	if len(annotations) == 0 {
		return markdown
	}

	// Process annotations from end to start so insertions don't shift offsets
	// Sort by position in markdown (find each quote, work backwards)
	type match struct {
		pos     int
		end     int
		comment string
		author  string
	}
	var matches []match
	used := make(map[int]bool) // track used positions to avoid duplicates

	for _, a := range annotations {
		if a.Quote == "" {
			continue
		}
		// Find the quote in markdown, starting from the beginning
		searchFrom := 0
		idx := strings.Index(markdown[searchFrom:], a.Quote)
		for idx >= 0 {
			absIdx := searchFrom + idx
			if !used[absIdx] {
				used[absIdx] = true
				author := a.Author
				if author == "" {
					author = "anonymous"
				}
				matches = append(matches, match{
					pos:     absIdx,
					end:     absIdx + len(a.Quote),
					comment: a.Comment,
					author:  author,
				})
				break
			}
			searchFrom = absIdx + 1
			idx = strings.Index(markdown[searchFrom:], a.Quote)
		}
	}

	// Sort by position descending so we can insert from end
	for i := 0; i < len(matches); i++ {
		for j := i + 1; j < len(matches); j++ {
			if matches[j].pos > matches[i].pos {
				matches[i], matches[j] = matches[j], matches[i]
			}
		}
	}

	result := markdown
	for _, m := range matches {
		marker := "{>>" + m.author + ": " + m.comment + "<<}"
		result = result[:m.end] + marker + result[m.end:]
	}

	return result
}

func getSession(w http.ResponseWriter, r *http.Request) string {
	if c, err := r.Cookie("pitchbin_session"); err == nil && c.Value != "" {
		return c.Value
	}
	b := make([]byte, 16)
	rand.Read(b)
	sess := hex.EncodeToString(b)
	http.SetCookie(w, &http.Cookie{
		Name:     "pitchbin_session",
		Value:    sess,
		Path:     "/",
		MaxAge:   365 * 24 * 3600,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	})
	return sess
}

func (s *Server) handleGetAnnotations(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	sess := getSession(w, r)

	annotations, err := s.store.GetAnnotations(id)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
		return
	}
	if annotations == nil {
		annotations = []Annotation{}
	}

	for i := range annotations {
		annotations[i].Editable = annotations[i].Session == sess
	}

	writeJSON(w, http.StatusOK, annotations)
}

func (s *Server) handlePostAnnotation(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	sess := getSession(w, r)

	if !s.store.PitchExists(id) {
		http.NotFound(w, r)
		return
	}

	var req struct {
		Stamp     string `json:"stamp"`
		Author    string `json:"author"`
		Comment   string `json:"comment"`
		Quote     string `json:"quote"`
		TextStart int    `json:"text_start"`
		TextEnd   int    `json:"text_end"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 16384)).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request"})
		return
	}
	if req.Comment == "" || req.Quote == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "comment and quote are required"})
		return
	}
	if len(req.Comment) > 1024 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "comment too long (max 1KB)"})
		return
	}

	if err := VerifyStamp(req.Stamp, s.annotationPowBits); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid proof of work: " + err.Error()})
		return
	}
	stampHash := StampHash(req.Stamp)
	ok, err := s.store.UseStamp(stampHash)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
		return
	}
	if !ok {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "stamp already used"})
		return
	}

	a := &Annotation{
		PitchID:   id,
		Session:   sess,
		Author:    req.Author,
		Comment:   req.Comment,
		Quote:     req.Quote,
		TextStart: req.TextStart,
		TextEnd:   req.TextEnd,
		Created:   time.Now().Unix(),
	}
	aid, err := s.store.InsertAnnotation(a)
	if err != nil {
		log.Printf("annotation insert error: %v", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
		return
	}
	a.ID = aid
	a.Editable = true
	writeJSON(w, http.StatusCreated, a)
}

func (s *Server) handleUpdateAnnotation(w http.ResponseWriter, r *http.Request) {
	sess := getSession(w, r)
	aidStr := r.PathValue("aid")
	aid, err := strconv.ParseInt(aidStr, 10, 64)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid annotation id"})
		return
	}

	a, err := s.store.GetAnnotation(aid)
	if err == sql.ErrNoRows {
		http.NotFound(w, r)
		return
	}
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
		return
	}

	if a.Session != sess {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "not your annotation"})
		return
	}

	var req struct {
		Stamp   string `json:"stamp"`
		Author  string `json:"author"`
		Comment string `json:"comment"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 16384)).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request"})
		return
	}

	if err := VerifyStamp(req.Stamp, s.annotationPowBits); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid proof of work: " + err.Error()})
		return
	}
	stampHash := StampHash(req.Stamp)
	ok, err2 := s.store.UseStamp(stampHash)
	if err2 != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
		return
	}
	if !ok {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "stamp already used"})
		return
	}

	// If author name changed, update all annotations by this session on this pitch
	if req.Author != a.Author {
		s.store.UpdateAuthorBySession(a.PitchID, sess, req.Author)
	}

	if req.Comment != "" {
		s.store.UpdateAnnotation(aid, req.Author, req.Comment)
	}

	a.Author = req.Author
	if req.Comment != "" {
		a.Comment = req.Comment
	}
	a.Editable = true
	writeJSON(w, http.StatusOK, a)
}

func (s *Server) handleDeleteAnnotation(w http.ResponseWriter, r *http.Request) {
	sess := getSession(w, r)
	aidStr := r.PathValue("aid")
	aid, err := strconv.ParseInt(aidStr, 10, 64)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid annotation id"})
		return
	}

	a, err := s.store.GetAnnotation(aid)
	if err == sql.ErrNoRows {
		http.NotFound(w, r)
		return
	}
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
		return
	}

	if a.Session != sess {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "not your annotation"})
		return
	}

	s.store.DeleteAnnotation(aid)
	w.WriteHeader(http.StatusNoContent)
}

func handleExampleRedirect(w http.ResponseWriter, r *http.Request) {
	http.Redirect(w, r, "/", http.StatusFound)
}

func handleFavicon(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "image/svg+xml")
	w.Header().Set("Cache-Control", "public, max-age=86400")
	w.Write(faviconSVG)
}

func handleRobots(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Write([]byte("User-agent: *\nAllow: /$\nDisallow: /\n"))
}

func parseExpiry(s string, now time.Time) int64 {
	switch s {
	case "permanent":
		return 0
	case "30d":
		return now.Add(30 * 24 * time.Hour).Unix()
	case "90d":
		return now.Add(90 * 24 * time.Hour).Unix()
	default: // 7d default
		return now.Add(7 * 24 * time.Hour).Unix()
	}
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

func clientIP(r *http.Request, trustedProxy string) string {
	if trustedProxy != "" {
		remote := r.RemoteAddr
		if i := strings.LastIndex(remote, ":"); i != -1 {
			remote = remote[:i]
		}
		if remote == trustedProxy {
			if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
				if i := strings.Index(xff, ","); i != -1 {
					return strings.TrimSpace(xff[:i])
				}
				return strings.TrimSpace(xff)
			}
		}
	}
	// Strip port
	addr := r.RemoteAddr
	if i := strings.LastIndex(addr, ":"); i != -1 {
		return addr[:i]
	}
	return addr
}

// RateLimiter is a simple per-key sliding window rate limiter.
type RateLimiter struct {
	mu      sync.Mutex
	entries map[string][]time.Time
	limit   int
	window  time.Duration
}

func NewRateLimiter(limit int, window time.Duration) *RateLimiter {
	return &RateLimiter{
		entries: make(map[string][]time.Time),
		limit:   limit,
		window:  window,
	}
}

func (rl *RateLimiter) Allow(key string) bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	now := time.Now()
	cutoff := now.Add(-rl.window)

	times := rl.entries[key]
	// Remove old entries
	valid := times[:0]
	for _, t := range times {
		if t.After(cutoff) {
			valid = append(valid, t)
		}
	}

	if len(valid) >= rl.limit {
		rl.entries[key] = valid
		return false
	}

	rl.entries[key] = append(valid, now)
	return true
}

// Cleanup removes all expired entries from the rate limiter.
func (rl *RateLimiter) Cleanup() {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	cutoff := time.Now().Add(-rl.window)
	for key, times := range rl.entries {
		valid := times[:0]
		for _, t := range times {
			if t.After(cutoff) {
				valid = append(valid, t)
			}
		}
		if len(valid) == 0 {
			delete(rl.entries, key)
		} else {
			rl.entries[key] = valid
		}
	}
}
