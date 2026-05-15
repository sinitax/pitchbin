package main

import (
	"testing"
	"time"
)

func newTestStore(t *testing.T) *Store {
	t.Helper()
	s, err := NewStore(":memory:")
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func TestStorePitchCRUD(t *testing.T) {
	s := newTestStore(t)

	p := &Pitch{
		ID:       "test-id",
		Title:    "Test",
		Author:   "alice",
		Markdown: "# Hello",
		HTML:     "<h1>Hello</h1>",
		Created:  time.Now().Unix(),
		Expires:  time.Now().Add(30 * 24 * time.Hour).Unix(),
	}

	if err := s.InsertPitch(p); err != nil {
		t.Fatalf("InsertPitch: %v", err)
	}

	if !s.PitchExists("test-id") {
		t.Error("PitchExists should return true")
	}
	if s.PitchExists("nonexistent") {
		t.Error("PitchExists should return false for nonexistent")
	}

	got, err := s.GetPitch("test-id")
	if err != nil {
		t.Fatalf("GetPitch: %v", err)
	}
	if got.Title != "Test" || got.Author != "alice" || got.Markdown != "# Hello" {
		t.Errorf("GetPitch returned wrong data: %+v", got)
	}
	if got.Views != 0 {
		t.Errorf("Views = %d, want 0", got.Views)
	}

	// Increment views
	s.IncrementViews("test-id")
	s.IncrementViews("test-id")
	got, _ = s.GetPitch("test-id")
	if got.Views != 2 {
		t.Errorf("Views after 2 increments = %d, want 2", got.Views)
	}
}

func TestStorePitchNotFound(t *testing.T) {
	s := newTestStore(t)
	_, err := s.GetPitch("nonexistent")
	if err == nil {
		t.Error("GetPitch should return error for nonexistent pitch")
	}
}

func TestStoreUseStamp(t *testing.T) {
	s := newTestStore(t)

	ok, err := s.UseStamp("hash1")
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Error("first UseStamp should succeed")
	}

	ok, err = s.UseStamp("hash1")
	if err != nil {
		t.Fatal(err)
	}
	if ok {
		t.Error("second UseStamp with same hash should fail (replay)")
	}

	ok, _ = s.UseStamp("hash2")
	if !ok {
		t.Error("different hash should succeed")
	}
}

func TestStoreAnnotations(t *testing.T) {
	s := newTestStore(t)

	// Need a pitch first
	s.InsertPitch(&Pitch{ID: "p1", Markdown: "x", HTML: "x", Created: 1, Expires: 0})

	a := &Annotation{
		PitchID:   "p1",
		Author:    "bob",
		Comment:   "great point",
		Quote:     "some text",
		TextStart: 10,
		TextEnd:   19,
		Created:   time.Now().Unix(),
	}

	id, err := s.InsertAnnotation(a)
	if err != nil {
		t.Fatalf("InsertAnnotation: %v", err)
	}
	if id <= 0 {
		t.Errorf("InsertAnnotation returned id=%d, want >0", id)
	}

	// Insert another
	a2 := &Annotation{PitchID: "p1", Author: "alice", Comment: "agreed", Quote: "other", TextStart: 5, TextEnd: 10, Created: time.Now().Unix()}
	s.InsertAnnotation(a2)

	annotations, err := s.GetAnnotations("p1")
	if err != nil {
		t.Fatalf("GetAnnotations: %v", err)
	}
	if len(annotations) != 2 {
		t.Fatalf("GetAnnotations len = %d, want 2", len(annotations))
	}
	// Should be sorted by text_start
	if annotations[0].TextStart > annotations[1].TextStart {
		t.Error("annotations not sorted by text_start")
	}

	// Empty pitch
	empty, err := s.GetAnnotations("nonexistent")
	if err != nil {
		t.Fatal(err)
	}
	if len(empty) != 0 {
		t.Errorf("GetAnnotations for nonexistent = %d, want 0", len(empty))
	}
}

func TestStoreCleanExpired(t *testing.T) {
	s := newTestStore(t)

	// Expired pitch
	s.InsertPitch(&Pitch{ID: "expired", Markdown: "x", HTML: "x", Created: 1, Expires: 1})
	// Valid pitch
	s.InsertPitch(&Pitch{ID: "valid", Markdown: "x", HTML: "x", Created: 1, Expires: time.Now().Add(time.Hour).Unix()})
	// Permanent pitch
	s.InsertPitch(&Pitch{ID: "perm", Markdown: "x", HTML: "x", Created: 1, Expires: 0})

	// Old stamp
	s.UseStamp("old-stamp")
	// Hack: set its created to the past
	s.db.Exec(`UPDATE used_stamps SET created = ? WHERE hash = ?`, time.Now().Unix()-700, "old-stamp")
	// Fresh stamp
	s.UseStamp("fresh-stamp")

	pitches, stamps, err := s.CleanExpired()
	if err != nil {
		t.Fatal(err)
	}
	if pitches != 1 {
		t.Errorf("cleaned pitches = %d, want 1", pitches)
	}
	if stamps != 1 {
		t.Errorf("cleaned stamps = %d, want 1", stamps)
	}

	// Verify expired is gone, others remain
	if s.PitchExists("expired") {
		t.Error("expired pitch should be cleaned")
	}
	if !s.PitchExists("valid") {
		t.Error("valid pitch should remain")
	}
	if !s.PitchExists("perm") {
		t.Error("permanent pitch should remain")
	}
}
