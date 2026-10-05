// Package sqlite implements SQLite persistence for the bookmark service.
package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"net/url"
	"path/filepath"
)

// Store persists application data in SQLite. Use Open to create a Store.
type Store struct {
	db *sql.DB
}

// Open opens a database file, configures its connections, and initializes its
// schema. The caller is responsible for closing the returned store.
func Open(ctx context.Context, path string) (*Store, error) {
	absPath, err := filepath.Abs(path)
	if err != nil {
		return nil, fmt.Errorf("resolve sqlite path: %w", err)
	}

	db, err := sql.Open("sqlite", sqliteDSN(absPath))
	if err != nil {
		return nil, fmt.Errorf("open sqlite database: %w", err)
	}
	db.SetMaxOpenConns(1)

	if err := migrate(ctx, db); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("migrate sqlite database: %w", err)
	}

	return &Store{db: db}, nil
}

// Close releases the store's database resources.
func (s *Store) Close() error {
	return s.db.Close()
}

func sqliteDSN(path string) string {
	u := url.URL{
		Scheme: "file",
		Path:   path,
	}
	q := u.Query()
	q.Add("_pragma", "foreign_keys(1)")
	q.Add("_pragma", "busy_timeout(5000)")
	q.Add("_pragma", "journal_mode(WAL)")
	u.RawQuery = q.Encode()
	return u.String()
}
