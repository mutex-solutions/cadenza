package prepare

import (
	"crypto/sha256"
	"encoding/hex"
	"io"
	"os"

	"github.com/mutex-solutions/cadenza/internal/kernel"
)

// HashFile returns the lowercase hex SHA-256 and size of a file. It streams
// the file, so large videos never sit in memory.
func HashFile(path string) (sum string, size int64, err error) {
	f, err := os.Open(path)
	if err != nil {
		return "", 0, kernel.Internal(err, "cannot open %s", path)
	}
	defer f.Close()
	h := sha256.New()
	n, err := io.Copy(h, f)
	if err != nil {
		return "", 0, kernel.Internal(err, "cannot read %s", path)
	}
	return hex.EncodeToString(h.Sum(nil)), n, nil
}

// hashBytes returns the lowercase hex SHA-256 of b.
func hashBytes(b []byte) string {
	s := sha256.Sum256(b)
	return hex.EncodeToString(s[:])
}
