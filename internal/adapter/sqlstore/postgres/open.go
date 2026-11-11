// Package postgres is Cadenza's Postgres storage, for real use.
package postgres

import (
	"context"
	"database/sql"
	"fmt"

	_ "github.com/jackc/pgx/v5/stdlib" // registers the "pgx" driver

	"github.com/mutex-solutions/cadenza/internal/adapter/sqlstore"
	"github.com/mutex-solutions/cadenza/internal/adapter/sqlstore/postgres/db"
)

// Open connects to Postgres at url (postgres://…) and applies the schema.
func Open(ctx context.Context, url string) (*Store, error) {
	sdb, err := sql.Open("pgx", url)
	if err != nil {
		return nil, fmt.Errorf("open postgres: %w", err)
	}
	sdb.SetMaxOpenConns(16)
	sdb.SetMaxIdleConns(4)
	if err := sdb.PingContext(ctx); err != nil {
		sdb.Close()
		return nil, fmt.Errorf("open postgres: %w", err)
	}
	if err := sqlstore.Migrate(ctx, sdb, "postgres"); err != nil {
		sdb.Close()
		return nil, err
	}
	return &Store{db: sdb, q: db.New(sdb)}, nil
}
