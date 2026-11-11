package prepare

import (
	"testing"

	"github.com/mutex-solutions/cadenza/internal/kernel"
)

func TestHashFile(t *testing.T) {
	sum, n, err := HashFile(writeFile(t, "a.txt", []byte("test")))
	if err != nil {
		t.Fatal(err)
	}
	// SHA-256("test"), same value as the media package tests.
	if sum != "9f86d081884c7d659a2feaa0c55ad015a3bf4f1b2b0b822cd15d6c15b0f00a08" || n != 4 {
		t.Fatalf("got %s %d", sum, n)
	}
	if hashBytes([]byte("test")) != sum {
		t.Fatal("hashBytes disagrees with HashFile")
	}
	if _, _, err := HashFile("/nonexistent"); !kernel.Is(err, kernel.KindInternal) {
		t.Fatalf("want internal error, got %v", err)
	}
	if _, _, err := HashFile(t.TempDir()); err == nil {
		t.Fatal("want error hashing a folder")
	}
}
