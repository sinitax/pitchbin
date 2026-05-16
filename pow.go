package main

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"
)

const (
	powVersion    = 1
	powService    = "pitchbin"
	powMaxAgeSec  = 300 // 5 minutes
)

// VerifyStamp verifies a hashcash-style PoW stamp.
// Stamp format: pitchbin:1:<unix_ts>:<random_hex>:<nonce>
// The SHA-256 of the stamp must have at least `bits` leading zero bits.
// The timestamp must be within powMaxAgeSec of now.
func VerifyStamp(stamp string, bits int) error {
	parts := strings.Split(stamp, ":")
	if len(parts) != 5 {
		return fmt.Errorf("invalid stamp format")
	}

	if parts[0] != powService {
		return fmt.Errorf("invalid service")
	}

	ver, err := strconv.Atoi(parts[1])
	if err != nil || ver != powVersion {
		return fmt.Errorf("invalid version")
	}

	ts, err := strconv.ParseInt(parts[2], 10, 64)
	if err != nil {
		return fmt.Errorf("invalid timestamp")
	}

	age := time.Now().Unix() - ts
	if age < -powMaxAgeSec || age > powMaxAgeSec {
		return fmt.Errorf("stamp expired or from the future")
	}

	// Verify hash difficulty
	hash := sha256.Sum256([]byte(stamp))
	if !hasLeadingZeroBits(hash[:], bits) {
		return fmt.Errorf("insufficient proof of work")
	}

	return nil
}

// StampHash returns the hex-encoded SHA-256 of a stamp (for replay detection).
func StampHash(stamp string) string {
	h := sha256.Sum256([]byte(stamp))
	return hex.EncodeToString(h[:])
}

func hasLeadingZeroBits(hash []byte, bits int) bool {
	fullBytes := bits / 8
	remainBits := bits % 8

	for i := 0; i < fullBytes; i++ {
		if hash[i] != 0 {
			return false
		}
	}

	if remainBits > 0 {
		mask := byte(math.MaxUint8 << (8 - remainBits))
		if hash[fullBytes]&mask != 0 {
			return false
		}
	}

	return true
}
