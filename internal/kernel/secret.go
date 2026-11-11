package kernel

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
)

// NewSecret returns prefix + 43 random URL-safe characters (256 bits).
// Secrets are shown once and stored only as HashSecret.
func NewSecret(prefix string) string {
	var b [32]byte
	if _, err := rand.Read(b[:]); err != nil {
		panic("kernel: crypto/rand failed: " + err.Error())
	}
	return prefix + base64.RawURLEncoding.EncodeToString(b[:])
}

// HashSecret is the stored form of a high-entropy secret (keys, tokens).
// Plain SHA-256 is right here: the input is random, not a password.
func HashSecret(s string) string {
	h := sha256.Sum256([]byte(s))
	return hex.EncodeToString(h[:])
}

// SecretEqual compares two strings in constant time.
func SecretEqual(a, b string) bool {
	return subtle.ConstantTimeCompare([]byte(a), []byte(b)) == 1
}
