package main

import (
	"crypto/sha256"
	"fmt"
	"testing"
	"time"
)

func TestHasLeadingZeroBits(t *testing.T) {
	tests := []struct {
		hash []byte
		bits int
		want bool
	}{
		{[]byte{0x00, 0x00, 0xFF}, 16, true},
		{[]byte{0x00, 0x00, 0xFF}, 17, false},
		{[]byte{0x00, 0x01, 0xFF}, 15, true},
		{[]byte{0x00, 0x01, 0xFF}, 16, false},
		{[]byte{0x00, 0x00, 0x00}, 24, true},
		{[]byte{0x0F, 0x00}, 4, true},
		{[]byte{0x0F, 0x00}, 5, false},
		{[]byte{0xFF}, 0, true},
	}

	for _, tt := range tests {
		got := hasLeadingZeroBits(tt.hash, tt.bits)
		if got != tt.want {
			t.Errorf("hasLeadingZeroBits(%x, %d) = %v, want %v", tt.hash, tt.bits, got, tt.want)
		}
	}
}

func TestVerifyStamp_Valid(t *testing.T) {
	// Compute a valid stamp with low difficulty
	bits := 8
	ts := time.Now().Unix()
	prefix := fmt.Sprintf("pitchbin:1:%d:abcdef0123456789:", ts)

	var stamp string
	for nonce := 0; ; nonce++ {
		s := fmt.Sprintf("%s%d", prefix, nonce)
		h := sha256.Sum256([]byte(s))
		if hasLeadingZeroBits(h[:], bits) {
			stamp = s
			break
		}
	}

	if err := VerifyStamp(stamp, bits); err != nil {
		t.Errorf("VerifyStamp valid stamp: %v", err)
	}
}

func TestVerifyStamp_Expired(t *testing.T) {
	oldTs := time.Now().Unix() - 600 // 10 minutes ago
	stamp := fmt.Sprintf("pitchbin:1:%d:abcdef0123456789:0", oldTs)
	err := VerifyStamp(stamp, 0) // 0 bits = any hash passes
	if err == nil {
		t.Error("VerifyStamp should reject expired stamp")
	}
}

func TestVerifyStamp_BadFormat(t *testing.T) {
	tests := []struct {
		name  string
		stamp string
	}{
		{"too few parts", "pitchbin:1:123:abc"},
		{"wrong service", "other:1:123:abc:0"},
		{"wrong version", "pitchbin:2:123:abc:0"},
		{"bad timestamp", "pitchbin:1:notanumber:abc:0"},
	}

	for _, tt := range tests {
		if err := VerifyStamp(tt.stamp, 0); err == nil {
			t.Errorf("VerifyStamp(%q) should fail for %s", tt.stamp, tt.name)
		}
	}
}

func TestVerifyStamp_InsufficientWork(t *testing.T) {
	ts := time.Now().Unix()
	// nonce=0 almost certainly won't satisfy 32 bits
	stamp := fmt.Sprintf("pitchbin:1:%d:abcdef0123456789:0", ts)
	err := VerifyStamp(stamp, 32)
	if err == nil {
		t.Error("VerifyStamp should reject insufficient work")
	}
}

func TestStampHash(t *testing.T) {
	h := StampHash("test")
	if len(h) != 64 {
		t.Errorf("StampHash length = %d, want 64", len(h))
	}
	// Deterministic
	if h != StampHash("test") {
		t.Error("StampHash not deterministic")
	}
	if h == StampHash("other") {
		t.Error("StampHash collision")
	}
}
