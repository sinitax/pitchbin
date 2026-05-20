package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func newTestServer(t *testing.T) (*Server, *Store) {
	t.Helper()
	store := newTestStore(t)
	renderer := NewRenderer()
	srv := NewServer(store, renderer, "http://test.local", 8, 8, 512000, 100, "", true) // 8-bit PoW, generous rate limit, dev mode
	return srv, store
}

var stampCounter uint64

func solveStamp(bits int) string {
	stampCounter++
	ts := time.Now().Unix()
	prefix := fmt.Sprintf("pitchbin:1:%d:test%016x:", ts, stampCounter)
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
		t.Error("expires_at should be set by default (7d)")
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
		"stamp":      solveStamp(8),
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
	body2, _ := json.Marshal(map[string]any{
		"stamp":      solveStamp(8),
		"author":     "alice",
		"comment":    "nice",
		"quote":      "hello",
		"text_start": 0,
		"text_end":   5,
	})
	req = httptest.NewRequest("POST", "/api/nope/annotations", bytes.NewReader(body2))
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
		{"", now.Add(7 * 24 * time.Hour).Unix()},            // default: 7d
		{"garbage", now.Add(7 * 24 * time.Hour).Unix()},     // unknown: 7d
		{"permanent", 0},
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
		xff          string
		addr         string
		trustedProxy string
		want         string
	}{
		// With trusted proxy, XFF is used
		{"1.2.3.4", "5.6.7.8:1234", "5.6.7.8", "1.2.3.4"},
		{"1.2.3.4, 5.6.7.8", "9.0.0.1:80", "9.0.0.1", "1.2.3.4"},
		// Without trusted proxy, XFF is ignored
		{"1.2.3.4", "5.6.7.8:1234", "", "5.6.7.8"},
		{"1.2.3.4", "5.6.7.8:1234", "10.0.0.1", "5.6.7.8"},
		// No XFF
		{"", "1.2.3.4:8080", "", "1.2.3.4"},
		{"", "1.2.3.4", "", "1.2.3.4"},
	}

	for _, tt := range tests {
		r := &http.Request{
			RemoteAddr: tt.addr,
			Header:     http.Header{},
		}
		if tt.xff != "" {
			r.Header.Set("X-Forwarded-For", tt.xff)
		}
		got := clientIP(r, tt.trustedProxy)
		if got != tt.want {
			t.Errorf("clientIP(xff=%q, addr=%q, proxy=%q) = %q, want %q", tt.xff, tt.addr, tt.trustedProxy, got, tt.want)
		}
	}
}

func TestXSSRendering(t *testing.T) {
	r := NewRenderer()

	// Each test: malicious markdown input -> list of strings that must NOT appear in output
	tests := []struct {
		name    string
		input   string
		reject  []string
		require []string // strings that SHOULD appear (to verify rendering works)
	}{
		{
			name:   "script tag",
			input:  "<script>alert('xss')</script>",
			reject: []string{"<script>", "</script>"},
		},
		{
			name:   "script in markdown",
			input:  "# Title\n\n<script>document.cookie</script>\n\ntext",
			reject: []string{"<script>", "document.cookie"},
			require: []string{"<h1>Title</h1>", "text"},
		},
		{
			name:   "img onerror",
			input:  `<img src=x onerror="alert('xss')">`,
			reject: []string{"onerror"},
		},
		{
			name:   "svg onload",
			input:  `<svg onload="alert('xss')">`,
			reject: []string{"onload"},
		},
		{
			name:   "event handler in tag",
			input:  `<div onmouseover="alert('xss')">hover me</div>`,
			reject: []string{"onmouseover"},
		},
		{
			name:   "javascript: URL in link",
			input:  `<a href="javascript:alert('xss')">click</a>`,
			reject: []string{"javascript:"},
		},
		{
			name:   "javascript: URL in markdown link",
			input:  `[click](javascript:alert('xss'))`,
			reject: []string{"javascript:"},
		},
		{
			name:   "data: URL in link",
			input:  `<a href="data:text/html,<script>alert('xss')</script>">click</a>`,
			reject: []string{"data:text/html"},
		},
		{
			name:   "iframe",
			input:  `<iframe src="https://evil.com"></iframe>`,
			reject: []string{"<iframe"},
		},
		{
			name:   "object tag",
			input:  `<object data="https://evil.com/flash.swf"></object>`,
			reject: []string{"<object"},
		},
		{
			name:   "embed tag",
			input:  `<embed src="https://evil.com/flash.swf">`,
			reject: []string{"<embed"},
		},
		{
			name:   "form tag",
			input:  `<form action="https://evil.com"><input type="submit"></form>`,
			reject: []string{"<form"},
		},
		{
			name:   "style tag with expression",
			input:  `<style>body{background:url('javascript:alert(1)')}</style>`,
			reject: []string{"<style>"},
		},
		{
			name:   "meta refresh",
			input:  `<meta http-equiv="refresh" content="0;url=https://evil.com">`,
			reject: []string{"<meta"},
		},
		{
			name:   "base tag",
			input:  `<base href="https://evil.com">`,
			reject: []string{"<base"},
		},
		{
			name:   "markdown image with onerror",
			input:  `![alt](x" onerror="alert('xss'))`,
			reject: []string{`onerror="alert`}, // bare onerror appears as text, not attribute
		},
		{
			name:    "legitimate markdown preserved",
			input:   "# Hello\n\n**bold** `code` [link](https://example.com)\n\n- item",
			reject:  []string{},
			require: []string{"<h1>Hello</h1>", "<strong>bold</strong>", "<code>code</code>", "https://example.com"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			out, err := r.Render([]byte(tt.input), "")
			if err != nil {
				t.Fatalf("render error: %v", err)
			}
			for _, bad := range tt.reject {
				if strings.Contains(strings.ToLower(out), strings.ToLower(bad)) {
					t.Errorf("output contains %q:\n%s", bad, out)
				}
			}
			for _, good := range tt.require {
				if !strings.Contains(out, good) {
					t.Errorf("output missing %q:\n%s", good, out)
				}
			}
		})
	}
}

func TestXSSEndToEnd(t *testing.T) {
	srv, _ := newTestServer(t)

	// Submit a pitch with XSS payloads in every field
	stamp := solveStamp(8)
	body, _ := json.Marshal(pitchRequest{
		Stamp:    stamp,
		Title:    `<script>alert("title")</script>`,
		Author:   `<img src=x onerror=alert("author")>`,
		Markdown: "# Hello\n\n<script>alert('body')</script>\n\n<img src=x onerror=alert(1)>\n\n[xss](javascript:alert(1))",
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

	// View the rendered page
	req = httptest.NewRequest("GET", "/"+resp.ID, nil)
	w = httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	if w.Code != 200 {
		t.Fatalf("view status = %d", w.Code)
	}

	page := w.Body.String()

	// Check that XSS payloads don't appear as executable code.
	// The page has a legitimate <script> for annotation JS, so check payloads specifically.
	// Note: escaped text like &lt;script&gt; or onerror=alert(&#34;) is safe — only check
	// for unescaped dangerous patterns.

	// Script payloads from markdown body should be stripped by bluemonday
	if strings.Contains(page, "<script>alert") {
		t.Error("script payload in markdown body not sanitized")
	}
	if strings.Contains(page, "javascript:alert") {
		t.Error("javascript: URL not sanitized")
	}

	// Title should be escaped by html/template (< becomes &lt;)
	if strings.Contains(page, `<script>alert("title")`) {
		t.Error("title contains unescaped script tag")
	}

	// Author should be escaped by html/template
	if strings.Contains(page, `<img src=x onerror`) {
		t.Error("author contains unescaped img tag with event handler")
	}

	// Verify the escaped versions ARE present (proving template escaping works)
	if !strings.Contains(page, `&lt;script&gt;`) {
		t.Error("title should contain escaped script tag")
	}
}

func TestXSSAnnotations(t *testing.T) {
	srv, store := newTestServer(t)
	store.InsertPitch(&Pitch{ID: "p1", Markdown: "hello", HTML: "hello", Created: 1, Expires: 0})

	// Submit annotation with XSS in every field
	body, _ := json.Marshal(map[string]any{
		"stamp":      solveStamp(8),
		"author":     `<script>alert("author")</script>`,
		"comment":    `<img src=x onerror=alert("comment")>`,
		"quote":      `<script>alert("quote")</script>`,
		"text_start": 0,
		"text_end":   5,
	})
	req := httptest.NewRequest("POST", "/api/p1/annotations", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	if w.Code != 201 {
		t.Fatalf("status = %d", w.Code)
	}

	// Fetch annotations — they're JSON so the raw values are stored,
	// but the frontend escapes them via escHtml() before inserting into DOM.
	// Verify the API returns the raw values (no server-side mutation).
	req = httptest.NewRequest("GET", "/api/p1/annotations", nil)
	w = httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	var annotations []Annotation
	json.NewDecoder(w.Body).Decode(&annotations)
	if len(annotations) != 1 {
		t.Fatalf("len = %d", len(annotations))
	}

	// The annotation data is stored as-is (XSS prevention is client-side via escHtml).
	// Verify the values round-trip correctly so the client can escape them.
	a := annotations[0]
	if a.Author != `<script>alert("author")</script>` {
		t.Errorf("author mangled: %q", a.Author)
	}
	if a.Comment != `<img src=x onerror=alert("comment")>` {
		t.Errorf("comment mangled: %q", a.Comment)
	}
}

func TestUpdatePitch(t *testing.T) {
	srv, _ := newTestServer(t)

	// Create a pitch
	stamp := solveStamp(8)
	body, _ := json.Marshal(pitchRequest{
		Stamp:    stamp,
		Title:    "Original",
		Markdown: "# Original",
	})
	req := httptest.NewRequest("POST", "/api/pitch", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	if w.Code != 201 {
		t.Fatalf("create: %d %s", w.Code, w.Body.String())
	}

	var created pitchResponse
	json.NewDecoder(w.Body).Decode(&created)
	if created.Secret == "" {
		t.Fatal("no secret returned")
	}

	// Update with correct secret + PoW
	updateBody, _ := json.Marshal(pitchRequest{
		Stamp:    solveStamp(8),
		Title:    "Updated",
		Markdown: "# Updated content",
	})
	req = httptest.NewRequest("PUT", "/api/pitch/"+created.ID, bytes.NewReader(updateBody))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Pitch-Secret", created.Secret)
	w = httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	if w.Code != 200 {
		t.Fatalf("update: %d %s", w.Code, w.Body.String())
	}

	// Verify updated content
	pitch, _ := srv.store.GetPitch(created.ID)
	if pitch.Title != "Updated" {
		t.Errorf("title = %q, want Updated", pitch.Title)
	}
	if pitch.Markdown != "# Updated content" {
		t.Errorf("markdown = %q", pitch.Markdown)
	}

	// Update with wrong secret
	req = httptest.NewRequest("PUT", "/api/pitch/"+created.ID, bytes.NewReader(updateBody))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Pitch-Secret", "wrong")
	w = httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	if w.Code != 403 {
		t.Errorf("wrong secret: %d, want 403", w.Code)
	}

	// Update without secret
	req = httptest.NewRequest("PUT", "/api/pitch/"+created.ID, bytes.NewReader(updateBody))
	req.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	if w.Code != 401 {
		t.Errorf("no secret: %d, want 401", w.Code)
	}
}

func TestDeletePitch(t *testing.T) {
	srv, _ := newTestServer(t)

	// Create a pitch
	stamp := solveStamp(8)
	body, _ := json.Marshal(pitchRequest{
		Stamp:    stamp,
		Title:    "To Delete",
		Markdown: "# Delete me",
	})
	req := httptest.NewRequest("POST", "/api/pitch", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	var created pitchResponse
	json.NewDecoder(w.Body).Decode(&created)

	// Delete with wrong secret
	req = httptest.NewRequest("DELETE", "/api/pitch/"+created.ID, nil)
	req.Header.Set("X-Pitch-Secret", "wrong")
	w = httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	if w.Code != 403 {
		t.Errorf("wrong secret: %d, want 403", w.Code)
	}

	// Delete with correct secret
	req = httptest.NewRequest("DELETE", "/api/pitch/"+created.ID, nil)
	req.Header.Set("X-Pitch-Secret", created.Secret)
	w = httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	if w.Code != 200 {
		t.Fatalf("delete: %d %s", w.Code, w.Body.String())
	}

	// Verify deleted
	req = httptest.NewRequest("GET", "/"+created.ID, nil)
	w = httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	if w.Code != 404 {
		t.Errorf("after delete: %d, want 404", w.Code)
	}
}

func TestFrontmatterStripping(t *testing.T) {
	r := NewRenderer()

	tests := []struct {
		name    string
		input   string
		require []string
		reject  []string
	}{
		{
			name:    "with frontmatter",
			input:   "---\ntitle: Hello\nauthor: me\n---\n# Body\n\nText",
			require: []string{"<h1>Body</h1>", "Text"},
			reject:  []string{"title:", "author:"},
		},
		{
			name:    "no frontmatter",
			input:   "# Body\n\nText",
			require: []string{"<h1>Body</h1>"},
		},
		{
			name:    "unclosed frontmatter treated as content",
			input:   "---\ntitle: Hello\n# Body",
			require: []string{"Body"},
		},
		{
			name:    "newline between words preserved",
			input:   "---\ntitle: Hello\n---\nword1\nword2\n",
			require: []string{"word1\nword2"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			out, err := r.Render([]byte(tt.input), "")
			if err != nil {
				t.Fatalf("render error: %v", err)
			}
			for _, want := range tt.require {
				if !strings.Contains(out, want) {
					t.Errorf("output missing %q:\n%s", want, out)
				}
			}
			for _, bad := range tt.reject {
				if strings.Contains(out, bad) {
					t.Errorf("output should not contain %q:\n%s", bad, out)
				}
			}
		})
	}
}

func TestMarkdownExtensions(t *testing.T) {
	r := NewRenderer()

	tests := []struct {
		name    string
		input   string
		require []string
	}{
		{
			name:    "footnotes",
			input:   "Hello[^1]\n\n[^1]: A footnote\n",
			require: []string{"<sup", "footnote"},
		},
		{
			name:    "definition list",
			input:   "Term\n:   Definition here\n",
			require: []string{"<dl>", "<dt>Term</dt>", "<dd>Definition here</dd>"},
		},
		{
			name:    "abbreviations",
			input:   "The HTML spec is great.\n\n*[HTML]: Hyper Text Markup Language\n",
			require: []string{"<abbr", "Hyper Text Markup Language"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			out, err := r.Render([]byte(tt.input), "")
			if err != nil {
				t.Fatalf("render error: %v", err)
			}
			for _, want := range tt.require {
				if !strings.Contains(out, want) {
					t.Errorf("output missing %q:\n%s", want, out)
				}
			}
		})
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
