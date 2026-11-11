// Package sqlstore holds Cadenza's SQL storage: one schema per database
// engine, shared queries compiled by sqlc for both, and the repository code
// in sqlite/ (hand-written) and postgres/ (generated from sqlite/repo.go by
// `make gen`, so the two cannot drift).
package sqlstore

import (
	"context"
	"database/sql"
	"embed"
	"fmt"
	"io/fs"
	"sort"
	"strconv"
	"strings"
)

//go:embed schema
var schemaFS embed.FS

// Migrate applies the engine's schema files that have not run yet, in name
// order, each in its own transaction. Engine is "sqlite" or "postgres".
func Migrate(ctx context.Context, db *sql.DB, engine string) error {
	if _, err := db.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS schema_migrations (version BIGINT PRIMARY KEY)`); err != nil {
		return fmt.Errorf("create schema_migrations: %w", err)
	}
	files, err := fs.Glob(schemaFS, "schema/"+engine+"/*.sql")
	if err != nil || len(files) == 0 {
		return fmt.Errorf("no schema files for engine %q", engine)
	}
	sort.Strings(files)
	for _, f := range files {
		base := f[strings.LastIndex(f, "/")+1:]
		v, err := strconv.ParseInt(strings.SplitN(base, "_", 2)[0], 10, 64)
		if err != nil {
			return fmt.Errorf("schema file %s must start with a number", base)
		}
		var n int
		if err := db.QueryRowContext(ctx, rebind(engine, `SELECT COUNT(*) FROM schema_migrations WHERE version = ?`), v).Scan(&n); err != nil {
			return err
		}
		if n > 0 {
			continue
		}
		body, err := schemaFS.ReadFile(f)
		if err != nil {
			return err
		}
		tx, err := db.BeginTx(ctx, nil)
		if err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, string(body)); err != nil {
			tx.Rollback()
			return fmt.Errorf("apply %s: %w", base, err)
		}
		if _, err := tx.ExecContext(ctx, rebind(engine, `INSERT INTO schema_migrations (version) VALUES (?)`), v); err != nil {
			tx.Rollback()
			return err
		}
		if err := tx.Commit(); err != nil {
			return err
		}
	}
	return nil
}

func rebind(engine, q string) string {
	if engine != "postgres" {
		return q
	}
	return strings.Replace(q, "?", "$1", 1)
}

// Settings is a small key-value store for server secrets and keys that are
// generated on first start (Web Push keys, link-signing secret).
type Settings interface {
	GetSetting(ctx context.Context, key string) (string, error) // NotFound if unset
	SetSetting(ctx context.Context, key, value string) error
}
