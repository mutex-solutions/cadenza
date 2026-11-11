package access

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"fmt"
	"strings"

	"golang.org/x/crypto/argon2"

	"github.com/mutex-solutions/cadenza/internal/kernel"
)

// Argon2id parameters (RFC 9106 second recommended option, with less memory
// so a 1-CPU server answers a login quickly).
const (
	argonTime    = 3
	argonMemory  = 64 * 1024 // KiB
	argonThreads = 1
	argonKeyLen  = 32
	MinPassword  = 10
	MaxPassword  = 256
)

// CheckPasswordRules returns an error if the password is too weak to set.
func CheckPasswordRules(pw string) error {
	if len(pw) < MinPassword {
		return kernel.Invalid("the password is too short").WithFix("use at least %d characters", MinPassword)
	}
	if len(pw) > MaxPassword {
		return kernel.Invalid("the password is too long").WithFix("use at most %d characters", MaxPassword)
	}
	return nil
}

// HashPassword returns an encoded argon2id hash.
func HashPassword(pw string) string {
	salt := make([]byte, 16)
	if _, err := rand.Read(salt); err != nil {
		panic("access: crypto/rand failed: " + err.Error())
	}
	key := argon2.IDKey([]byte(pw), salt, argonTime, argonMemory, argonThreads, argonKeyLen)
	return fmt.Sprintf("argon2id$v=19$m=%d,t=%d,p=%d$%s$%s", argonMemory, argonTime, argonThreads,
		base64.RawStdEncoding.EncodeToString(salt), base64.RawStdEncoding.EncodeToString(key))
}

// VerifyPassword reports whether pw matches an encoded hash.
func VerifyPassword(pw, encoded string) bool {
	parts := strings.Split(encoded, "$")
	if len(parts) != 5 || parts[0] != "argon2id" || parts[1] != "v=19" {
		return false
	}
	var m, t uint32
	var p uint8
	if _, err := fmt.Sscanf(parts[2], "m=%d,t=%d,p=%d", &m, &t, &p); err != nil {
		return false
	}
	salt, err1 := base64.RawStdEncoding.DecodeString(parts[3])
	want, err2 := base64.RawStdEncoding.DecodeString(parts[4])
	if err1 != nil || err2 != nil || len(want) == 0 {
		return false
	}
	got := argon2.IDKey([]byte(pw), salt, t, m, p, uint32(len(want)))
	return subtle.ConstantTimeCompare(got, want) == 1
}
