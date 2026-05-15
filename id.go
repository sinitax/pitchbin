package main

import (
	"crypto/rand"
	"math/big"
)

const (
	idCharset = "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz"
	idLength  = 8
)

func GenerateID() (string, error) {
	b := make([]byte, idLength)
	max := big.NewInt(int64(len(idCharset)))
	for i := range b {
		n, err := rand.Int(rand.Reader, max)
		if err != nil {
			return "", err
		}
		b[i] = idCharset[n.Int64()]
	}
	return string(b), nil
}
