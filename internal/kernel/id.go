package kernel

import (
	"crypto/rand"
	"encoding/base32"
	"strings"
)

// idEncoding is lowercase base32 without padding: ids are short, URL-safe and
// easy to read aloud.
var idEncoding = base32.NewEncoding("abcdefghijklmnopqrstuvwxyz234567").WithPadding(base32.NoPadding)

// NewID returns prefix + "_" + 16 random characters (80 bits), e.g. "t_k3j…".
func NewID(prefix string) string {
	var b [10]byte
	if _, err := rand.Read(b[:]); err != nil {
		panic("kernel: crypto/rand failed: " + err.Error())
	}
	return prefix + "_" + idEncoding.EncodeToString(b[:])
}

// HasPrefix reports whether id was made by NewID with this prefix.
func HasPrefix(id, prefix string) bool {
	rest, ok := strings.CutPrefix(id, prefix+"_")
	if !ok || len(rest) != 16 {
		return false
	}
	for _, c := range rest {
		if !(c >= 'a' && c <= 'z' || c >= '2' && c <= '7') {
			return false
		}
	}
	return true
}

// OwnerID identifies the person who owns everything. Cadenza has one owner
// today; every record carries it so more users can be added later.
type OwnerID string
