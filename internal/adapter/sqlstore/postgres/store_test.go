package postgres

import (
	"context"
	"database/sql"
	"os"
	"testing"

	"github.com/mutex-solutions/cadenza/internal/adapter/sqlstore/storetest"
)

// The suite runs only when CADENZA_TEST_POSTGRES points at a database the
// tests may wipe, e.g. postgres://postgres:cadenza@localhost:5432/cadenza?sslmode=disable
func TestStoreSuite(t *testing.T) {
	url := os.Getenv("CADENZA_TEST_POSTGRES")
	if url == "" {
		t.Skip("CADENZA_TEST_POSTGRES not set")
	}
	storetest.Run(t, func(t *testing.T) storetest.Store {
		reset(t, url)
		s, err := Open(context.Background(), url)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { s.Close() })
		return s
	})
}

func reset(t *testing.T, url string) {
	t.Helper()
	db, err := sql.Open("pgx", url)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.Exec(`DROP SCHEMA public CASCADE; CREATE SCHEMA public`); err != nil {
		t.Fatal(err)
	}
}
