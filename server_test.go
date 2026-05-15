package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func newTestServer(t *testing.T) (*Server, *Store) {
	t.Helper()
	store := newTestStore(t)
	renderer := NewRenderer()
	srv := NewServer(store, renderer, "http://test.local", 8, 512000, 100) // 8-bit PoW, generous rate limit
	return srv, store
}

func solveStamp(bits int) string {
	ts := time.Now().Unix()
	prefix := fmt.Sprintf("pitchbin:1:%d:testtest01234567:", ts)
	for nonce := 0; ; nonce++ {
		s := fmt.Sprintf("%s%d", prefix, nonce)
		h := sha256.Sum256([]byte(s))
		if hasLeadingZeroBits(h[:], bits) {
			return s
		}
	}
}

func TestHandleInfo(t *testing.T) {
	srv, _ := newTestServer(t)

	req := httptest.NewRequest("GET", "/api/info", nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	if w.Code != 200 {
		t.Fatalf("status = %d, want 200", w.Code)
	}

	var body map[string]any
	json.NewDecoder(w.Body).Decode(&body)

	if body["service"] != "pitchbin" {
		t.Errorf("service = %v", body["service"])
	}
	pow := body["pow"].(map[string]any)
	if pow["bits"].(float64) != 8 {
		t.Errorf("pow bits = %v", pow["bits"])
	}
}

func TestSubmitAndView(t *testing.T) {
	srv, _ := newTestServer(t)

	stamp := solveStamp(8)
	body, _ := json.Marshal(pitchRequest{
		Stamp:    stamp,
		Title:    "Test Pitch",
		Author:   "tester",
		Markdown: "# Hello\n\nWorld",
		Expires:  "30d",
	})

	req := httptest.NewRequest("POST", "/api/pitch", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	if w.Code != 201 {
		t.Fatalf("submit status = %d, body = %s", w.Code, w.Body.String())
	}

	var resp pitchResponse
	json.NewDecoder(w.Body).Decode(&resp)

	if resp.ID != "test-pitch" {
		t.Errorf("id = %q, want %q", resp.ID, "test-pitch")
	}
	if resp.URL != "http://test.local/test-pitch" {
		t.Errorf("url = %q", resp.URL)
	}
	if resp.ExpiresAt == "" {
		t.Error("expires_at should be set")
	}

	// View the pitch
	req = httptest.NewRequest("GET", "/test-pitch", nil)
	w = httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	if w.Code != 200 {
		t.Fatalf("view status = %d", w.Code)
	}
	if ct := w.Header().Get("Content-Type"); ct != "text/html; charset=utf-8" {
		t.Errorf("content-type = %q", ct)
	}
	html := w.Body.String()
	if !bytes.Contains([]byte(html), []byte("Test Pitch")) {
		t.Error("page should contain title")
	}
	if !bytes.Contains([]byte(html), []byte("<h1>Hello</h1>")) {
		t.Error("page should contain rendered markdown")
	}

	// Raw
	req = httptest.NewRequest("GET", "/test-pitch/raw", nil)
	w = httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	if w.Code != 200 {
		t.Fatalf("raw status = %d", w.Code)
	}
	if w.Body.String() != "# Hello\n\nWorld" {
		t.Errorf("raw body = %q", w.Body.String())
	}
}

func TestSubmitReplayRejected(t *testing.T) {
	srv, _ := newTestServer(t)

	stamp := solveStamp(8)
	body, _ := json.Marshal(pitchRequest{
		Stamp:    stamp,
		Title:    "First",
		Markdown: "# test",
	})

	// First submit
	req := httptest.NewRequest("POST", "/api/pitch", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	if w.Code != 201 {
		t.Fatalf("first submit status = %d", w.Code)
	}

	// Replay with same stamp
	req = httptest.NewRequest("POST", "/api/pitch", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	if w.Code != 400 {
		t.Errorf("replay status = %d, want 400", w.Code)
	}
}

func TestSubmitValidation(t *testing.T) {
	srv, _ := newTestServer(t)

	tests := []struct {
		name string
		body pitchRequest
		want int
	}{
		{"empty markdown", pitchRequest{Stamp: solveStamp(8), Markdown: ""}, 400},
		{"bad stamp", pitchRequest{Stamp: "bad:stamp", Markdown: "# test"}, 400},
	}

	for _, tt := range tests {
		b, _ := json.Marshal(tt.body)
		req := httptest.NewRequest("POST", "/api/pitch", bytes.NewReader(b))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		srv.ServeHTTP(w, req)
		if w.Code != tt.want {
			t.Errorf("%s: status = %d, want %d, body = %s", tt.name, w.Code, tt.want, w.Body.String())
		}
	}
}

func TestSubmitPrivate(t *testing.T) {
	srv, _ := newTestServer(t)

	body, _ := json.Marshal(pitchRequest{
		Stamp:    solveStamp(8),
		Title:    "Secret Plan",
		Markdown: "# secret",
		Private:  true,
	})

	req := httptest.NewRequest("POST", "/api/pitch", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	if w.Code != 201 {
		t.Fatalf("status = %d", w.Code)
	}

	var resp pitchResponse
	json.NewDecoder(w.Body).Decode(&resp)

	// Private ID should be longer than just the slug
	if len(resp.ID) <= len("secret-plan-") {
		t.Errorf("private id too short: %q", resp.ID)
	}
}

func TestViewNotFound(t *testing.T) {
	srv, _ := newTestServer(t)

	req := httptest.NewRequest("GET", "/nonexistent", nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	if w.Code != 404 {
		t.Errorf("status = %d, want 404", w.Code)
	}
}

func TestViewExpired(t *testing.T) {
	srv, store := newTestServer(t)

	store.InsertPitch(&Pitch{
		ID:       "old",
		Markdown: "# old",
		HTML:     "<h1>old</h1>",
		Created:  1,
		Expires:  1, // expired in 1970
	})

	req := httptest.NewRequest("GET", "/old", nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	if w.Code != 404 {
		t.Errorf("expired pitch status = %d, want 404", w.Code)
	}
}

func TestAnnotations(t *testing.T) {
	srv, store := newTestServer(t)

	store.InsertPitch(&Pitch{ID: "p1", Markdown: "hello world", HTML: "hello world", Created: 1, Expires: 0})

	// POST annotation
	body, _ := json.Marshal(map[string]any{
		"author":     "alice",
		"comment":    "nice",
		"quote":      "hello",
		"text_start": 0,
		"text_end":   5,
	})
	req := httptest.NewRequest("POST", "/api/p1/annotations", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	if w.Code != 201 {
		t.Fatalf("post annotation status = %d, body = %s", w.Code, w.Body.String())
	}

	var ann Annotation
	json.NewDecoder(w.Body).Decode(&ann)
	if ann.Author != "alice" || ann.Comment != "nice" {
		t.Errorf("annotation = %+v", ann)
	}

	// GET annotations
	req = httptest.NewRequest("GET", "/api/p1/annotations", nil)
	w = httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	if w.Code != 200 {
		t.Fatalf("get annotations status = %d", w.Code)
	}

	var annotations []Annotation
	json.NewDecoder(w.Body).Decode(&annotations)
	if len(annotations) != 1 {
		t.Fatalf("annotations len = %d, want 1", len(annotations))
	}

	// POST to nonexistent pitch
	req = httptest.NewRequest("POST", "/api/nope/annotations", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	if w.Code != 404 {
		t.Errorf("annotation on nonexistent pitch = %d, want 404", w.Code)
	}

	// POST missing fields
	bad, _ := json.Marshal(map[string]any{"author": "x"})
	req = httptest.NewRequest("POST", "/api/p1/annotations", bytes.NewReader(bad))
	req.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	if w.Code != 400 {
		t.Errorf("missing fields status = %d, want 400", w.Code)
	}
}

func TestParseExpiry(t *testing.T) {
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	tests := []struct {
		input string
		want  int64
	}{
		{"7d", now.Add(7 * 24 * time.Hour).Unix()},
		{"30d", now.Add(30 * 24 * time.Hour).Unix()},
		{"90d", now.Add(90 * 24 * time.Hour).Unix()},
		{"permanent", 0},
		{"perm", 0},
		{"", now.Add(30 * 24 * time.Hour).Unix()},        // default
		{"garbage", now.Add(30 * 24 * time.Hour).Unix()},  // default
	}

	for _, tt := range tests {
		got := parseExpiry(tt.input, now)
		if got != tt.want {
			t.Errorf("parseExpiry(%q) = %d, want %d", tt.input, got, tt.want)
		}
	}
}

func TestRateLimiter(t *testing.T) {
	rl := NewRateLimiter(3, time.Minute)

	for i := range 3 {
		if !rl.Allow("ip1") {
			t.Errorf("request %d should be allowed", i+1)
		}
	}
	if rl.Allow("ip1") {
		t.Error("4th request should be rate limited")
	}
	// Different key
	if !rl.Allow("ip2") {
		t.Error("different IP should be allowed")
	}
}

func TestClientIP(t *testing.T) {
	tests := []struct {
		xff  string
		addr string
		want string
	}{
		{"1.2.3.4", "5.6.7.8:1234", "1.2.3.4"},
		{"1.2.3.4, 5.6.7.8", "9.0.0.1:80", "1.2.3.4"},
		{"", "1.2.3.4:8080", "1.2.3.4"},
		{"", "1.2.3.4", "1.2.3.4"},
	}

	for _, tt := range tests {
		r := &http.Request{
			RemoteAddr: tt.addr,
			Header:     http.Header{},
		}
		if tt.xff != "" {
			r.Header.Set("X-Forwarded-For", tt.xff)
		}
		got := clientIP(r)
		if got != tt.want {
			t.Errorf("clientIP(xff=%q, addr=%q) = %q, want %q", tt.xff, tt.addr, got, tt.want)
		}
	}
}

// Ensure markdown rendering produces sanitized output
func TestRenderIntegration(t *testing.T) {
	r := NewRenderer()
	html, err := r.Render([]byte("# Title\n\n**bold** and `code`\n\n<script>alert('xss')</script>"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains([]byte(html), []byte("<h1>Title</h1>")) {
		t.Error("should render h1")
	}
	if !bytes.Contains([]byte(html), []byte("<strong>bold</strong>")) {
		t.Error("should render bold")
	}
	if bytes.Contains([]byte(html), []byte("<script>")) {
		t.Error("should sanitize script tags")
	}
}

func readBody(t *testing.T, r io.Reader) string {
	t.Helper()
	b, err := io.ReadAll(r)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}
