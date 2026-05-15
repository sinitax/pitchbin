package main

import (
	"testing"
)

func TestSlugify(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"Proposal: Refactor Auth Module!", "proposal-refactor-auth-module"},
		{"hello world", "hello-world"},
		{"  spaces  everywhere  ", "spaces-everywhere"},
		{"ALLCAPS", "allcaps"},
		{"with---dashes", "with-dashes"},
		{"trailing dash ", "trailing-dash"},
		{"", ""},
		{"123 numbers 456", "123-numbers-456"},
		{"special!@#$%chars", "special-chars"},
		{"a", "a"},
		{"   ", ""},
	}

	for _, tt := range tests {
		got := Slugify(tt.input)
		if got != tt.want {
			t.Errorf("Slugify(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestSlugifyMaxLength(t *testing.T) {
	long := ""
	for range 100 {
		long += "a"
	}
	slug := Slugify(long)
	if len(slug) > maxSlugLen {
		t.Errorf("Slugify produced slug of length %d, want <= %d", len(slug), maxSlugLen)
	}
}

func TestGenerateRand(t *testing.T) {
	seen := make(map[string]bool)
	for range 100 {
		r, err := generateRand()
		if err != nil {
			t.Fatal(err)
		}
		if len(r) != randLength {
			t.Errorf("generateRand() length = %d, want %d", len(r), randLength)
		}
		if seen[r] {
			t.Errorf("generateRand() produced duplicate: %s", r)
		}
		seen[r] = true
	}
}

func TestGenerateID(t *testing.T) {
	store := newTestStore(t)

	// Public: slug from title
	id, err := GenerateID(store, "My Test Pitch", false)
	if err != nil {
		t.Fatal(err)
	}
	if id != "my-test-pitch" {
		t.Errorf("GenerateID public = %q, want %q", id, "my-test-pitch")
	}

	// Insert it so next call collides
	store.InsertPitch(&Pitch{ID: "my-test-pitch", Markdown: "x", HTML: "x", Created: 1, Expires: 0})

	// Public collision: slug-2
	id2, err := GenerateID(store, "My Test Pitch", false)
	if err != nil {
		t.Fatal(err)
	}
	if id2 != "my-test-pitch-2" {
		t.Errorf("GenerateID collision = %q, want %q", id2, "my-test-pitch-2")
	}

	// Private: slug-<rand>
	id3, err := GenerateID(store, "My Test Pitch", true)
	if err != nil {
		t.Fatal(err)
	}
	if len(id3) <= len("my-test-pitch-") {
		t.Errorf("GenerateID private too short: %q", id3)
	}
	if id3[:14] != "my-test-pitch-" {
		t.Errorf("GenerateID private prefix = %q, want %q", id3[:14], "my-test-pitch-")
	}

	// Empty title
	id4, err := GenerateID(store, "", false)
	if err != nil {
		t.Fatal(err)
	}
	if id4 != "pitch" {
		t.Errorf("GenerateID empty title = %q, want %q", id4, "pitch")
	}
}
