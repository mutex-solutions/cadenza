package sqlite

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/mutex-solutions/cadenza/internal/adapter/sqlstore/storetest"
)

func TestStoreSuite(t *testing.T) {
	storetest.Run(t, func(t *testing.T) storetest.Store {
		s, err := Open(context.Background(), ":memory:")
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { s.Close() })
		return s
	})
}

func TestFileDatabaseReopens(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "cadenza.db")
	s, err := Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Settings().SetSetting(ctx, "k", "v"); err != nil {
		t.Fatal(err)
	}
	s.Close()
	// Reopening must not re-run migrations or lose data.
	s, err = Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if v, err := s.Settings().GetSetting(ctx, "k"); err != nil || v != "v" {
		t.Fatalf("got %q, %v", v, err)
	}
	if err := s.Ping(ctx); err != nil {
		t.Fatal(err)
	}
}

func TestDSN(t *testing.T) {
	if got := dsnFor(":memory:"); got[:15] != "file::memory:?_" {
		t.Fatalf("memory dsn %q", got)
	}
	got := dsnFor("/data/c.db")
	for _, want := range []string{"journal_mode%28WAL%29", "_txlock=immediate", "busy_timeout%285000%29"} {
		if !contains(got, want) {
			t.Errorf("dsn %q lacks %q", got, want)
		}
	}
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || len(sub) == 0 || index(s, sub) >= 0)
}

func index(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}
