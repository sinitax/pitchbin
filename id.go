package main

import (
	"crypto/rand"
	"fmt"
	"math/big"
	"strings"
	"unicode"
)

const (
	randCharset = "0123456789abcdefghijklmnopqrstuvwxyz"
	randLength  = 8
	maxSlugLen  = 48
)

func generateRand() (string, error) {
	b := make([]byte, randLength)
	max := big.NewInt(int64(len(randCharset)))
	for i := range b {
		n, err := rand.Int(rand.Reader, max)
		if err != nil {
			return "", err
		}
		b[i] = randCharset[n.Int64()]
	}
	return string(b), nil
}

// Slugify turns a title into a URL-safe slug.
// "Proposal: Refactor Auth Module!" → "proposal-refactor-auth-module"
func Slugify(title string) string {
	var b strings.Builder
	prevDash := false
	for _, r := range strings.ToLower(title) {
		switch {
		case unicode.IsLetter(r) || unicode.IsDigit(r):
			b.WriteRune(r)
			prevDash = false
		case !prevDash && b.Len() > 0:
			b.WriteByte('-')
			prevDash = true
		}
		if b.Len() >= maxSlugLen {
			break
		}
	}
	return strings.TrimRight(b.String(), "-")
}

// GenerateID creates the URL path segment for a pitch.
// Public: <slug> (with numeric suffix on collision)
// Private: <slug>-<rand>
func GenerateID(store *Store, title string, private bool) (string, error) {
	slug := Slugify(title)
	if slug == "" {
		slug = "pitch"
	}

	if private {
		r, err := generateRand()
		if err != nil {
			return "", err
		}
		return slug + "-" + r, nil
	}

	// Public: try slug as-is, then slug-2, slug-3, ...
	if !store.PitchExists(slug) {
		return slug, nil
	}
	for i := 2; i < 1000; i++ {
		candidate := fmt.Sprintf("%s-%d", slug, i)
		if !store.PitchExists(candidate) {
			return candidate, nil
		}
	}
	// Fallback to random suffix
	r, err := generateRand()
	if err != nil {
		return "", err
	}
	return slug + "-" + r, nil
}
