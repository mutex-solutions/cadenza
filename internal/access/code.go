package access

import (
	"crypto/rand"
	"math/big"
	"strings"
)

// codeAlphabet leaves out characters that are easy to confuse: 0/O, 1/I/L, 5/S, 8/B.
const codeAlphabet = "ACDEFGHJKMNPQRTUVWXYZ234679"

// NewPairingCode returns a code like "KQ7-4XM": 6 characters, about 28 bits,
// valid for PairingTTL. Short-lived and rate-limited, so that is enough.
func NewPairingCode() string {
	var b strings.Builder
	for i := range 6 {
		if i == 3 {
			b.WriteByte('-')
		}
		n, err := rand.Int(rand.Reader, big.NewInt(int64(len(codeAlphabet))))
		if err != nil {
			panic("access: crypto/rand failed: " + err.Error())
		}
		b.WriteByte(codeAlphabet[n.Int64()])
	}
	return b.String()
}

// NormalizeCode accepts what a person types: any case, with or without the
// dash or spaces.
func NormalizeCode(s string) string {
	s = strings.ToUpper(strings.NewReplacer("-", "", " ", "").Replace(s))
	if len(s) != 6 {
		return s
	}
	return s[:3] + "-" + s[3:]
}
