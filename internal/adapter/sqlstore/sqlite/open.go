// Package sqlite is Cadenza's SQLite storage, for trials and small installs.
// It uses a pure-Go driver, so the server binary needs no C compiler.
package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"net/url"
	"strings"

	_ "modernc.org/sqlite" // registers the "sqlite" driver

	"github.com/mutex-solutions/cadenza/internal/adapter/sqlstore"
	"github.com/mutex-solutions/cadenza/internal/adapter/sqlstore/sqlite/db"
)

// Open opens (creating if needed) the database file at path and applies the
// schema. Use ":memory:" for a private in-memory database (tests).
func Open(ctx context.Context, path string) (*Store, error) {
	dsn := dsnFor(path)
	sdb, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}
	if path == ":memory:" {
		// Every connection to ":memory:" is a separate database; keep one.
		sdb.SetMaxOpenConns(1)
	} else {
		// SQLite allows one writer; a few connections let reads continue.
		sdb.SetMaxOpenConns(4)
	}
	if err := sdb.PingContext(ctx); err != nil {
		sdb.Close()
		return nil, fmt.Errorf("open sqlite: %w", err)
	}
	if err := sqlstore.Migrate(ctx, sdb, "sqlite"); err != nil {
		sdb.Close()
		return nil, err
	}
	return &Store{db: sdb, q: db.New(sdb)}, nil
}

func dsnFor(path string) string {
	pragmas := url.Values{}
	for _, p := range []string{"busy_timeout(5000)", "foreign_keys(1)", "synchronous(NORMAL)"} {
		pragmas.Add("_pragma", p)
	}
	if path != ":memory:" {
		pragmas.Add("_pragma", "journal_mode(WAL)")
		// Take the write lock when a transaction starts, so two writers wait
		// for each other instead of failing with "database is locked".
		pragmas.Set("_txlock", "immediate")
	}
	if strings.HasPrefix(path, "file:") {
		return path + "&" + pragmas.Encode()
	}
	return "file:" + path + "?" + pragmas.Encode()
}
