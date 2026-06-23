package bookmarks

import (
	"context"
	"database/sql"
	"fmt"
	"net/url"
	"path/filepath"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

const schemaSQL = `
CREATE TABLE IF NOT EXISTS bookmarks (
  id TEXT PRIMARY KEY,
  url TEXT NOT NULL,
  normalized_url TEXT NOT NULL UNIQUE,
  title TEXT NOT NULL DEFAULT '',
  notes TEXT NOT NULL DEFAULT '',
  source TEXT NOT NULL DEFAULT '',
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL,
  archived_at TEXT,
  read_at TEXT
) STRICT;

CREATE TABLE IF NOT EXISTS tags (
  id INTEGER PRIMARY KEY,
  name TEXT NOT NULL UNIQUE
) STRICT;

CREATE TABLE IF NOT EXISTS bookmark_tags (
  bookmark_id TEXT NOT NULL REFERENCES bookmarks(id) ON DELETE CASCADE,
  tag_id INTEGER NOT NULL REFERENCES tags(id) ON DELETE CASCADE,
  PRIMARY KEY (bookmark_id, tag_id)
) STRICT;

CREATE INDEX IF NOT EXISTS bookmarks_created_at_idx
ON bookmarks(created_at DESC);
`

type SQLStore struct {
	db *sql.DB
}

var _ Store = (*SQLStore)(nil)

func OpenSQLStore(path string) (*SQLStore, error) {
	absPath, err := filepath.Abs(path)
	if err != nil {
		return nil, fmt.Errorf("resolve sqlite path: %w", err)
	}

	db, err := sql.Open("sqlite", sqliteDSN(absPath))
	if err != nil {
		return nil, fmt.Errorf("open sqlite database: %w", err)
	}
	db.SetMaxOpenConns(1)

	if _, err := db.Exec(schemaSQL); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("apply sqlite schema: %w", err)
	}

	return &SQLStore{db: db}, nil
}

func (s *SQLStore) Close() error {
	return s.db.Close()
}

func (s *SQLStore) CreateBookmark(ctx context.Context, input CreateInput) (Bookmark, bool, error) {
	id, err := NewID()
	if err != nil {
		return Bookmark{}, false, err
	}

	normalizedURL, err := NormalizeURL(input.URL)
	if err != nil {
		return Bookmark{}, false, err
	}

	now := time.Now().UTC().Truncate(time.Second)

	bookmark := Bookmark{
		ID:            id,
		URL:           strings.TrimSpace(input.URL),
		NormalizedURL: normalizedURL,
		Title:         input.Title,
		Notes:         input.Notes,
		Source:        input.Source,
		CreatedAt:     now,
		UpdatedAt:     now,
	}

	result, err := s.db.ExecContext(
		ctx, `
		INSERT OR IGNORE INTO bookmarks (
			id,
			url,
			normalized_url,
			title,
			notes,
			source,
			created_at,
			updated_at
		)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
		`,
		bookmark.ID,
		bookmark.URL,
		bookmark.NormalizedURL,
		bookmark.Title,
		bookmark.Notes,
		bookmark.Source,
		bookmark.CreatedAt.Format(time.RFC3339),
		bookmark.UpdatedAt.Format(time.RFC3339),
	)
	if err != nil {
		return Bookmark{}, false, fmt.Errorf("insert bookmark: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return Bookmark{}, false, fmt.Errorf("check inserted bookmark: %w", err)
	}

	if rowsAffected == 0 {
		bookmark, err := s.bookmarkByNormalizedURL(ctx, normalizedURL)
		if err != nil {
			return Bookmark{}, false, err
		}

		return bookmark, false, nil
	}

	return bookmark, true, nil
}

func (s *SQLStore) ListBookmarks(ctx context.Context) ([]Bookmark, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, url, normalized_url, title, notes, source, created_at, updated_at
		FROM bookmarks
		ORDER BY created_at DESC, id DESC
	`)
	if err != nil {
		return nil, fmt.Errorf("list bookmarks: %w", err)
	}
	defer rows.Close()

	var bookmarks []Bookmark

	for rows.Next() {
		bkmk, err := scanBookmark(rows)
		if err != nil {
			return bookmarks, fmt.Errorf("scan bookmarks: %w", err)
		}

		bookmarks = append(bookmarks, bkmk)
	}

	if err = rows.Err(); err != nil {
		return bookmarks, fmt.Errorf("iterate bookmarks: %w", err)
	}

	return bookmarks, nil
}

func (s *SQLStore) UpdateBookmark(ctx context.Context, id string, input UpdateInput) (Bookmark, error) {
	return Bookmark{}, ErrNotImplemented
}
func (s *SQLStore) DeleteBookmark(ctx context.Context, id string) error { return ErrNotImplemented }

func (s *SQLStore) bookmarkByNormalizedURL(ctx context.Context, normalizedURL string) (Bookmark, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT id, url, normalized_url, title, notes, source, created_at, updated_at
		FROM bookmarks
		WHERE normalized_url = ?
	`, normalizedURL)

	bkmk, err := scanBookmark(row)
	if err != nil {
		return Bookmark{}, err
	}
	return bkmk, nil
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

type bookmarkScanner interface {
	Scan(dest ...any) error
}

func scanBookmark(scanner bookmarkScanner) (Bookmark, error) {
	var b Bookmark
	var createdAt string
	var updatedAt string

	err := scanner.Scan(
		&b.ID,
		&b.URL,
		&b.NormalizedURL,
		&b.Title,
		&b.Notes,
		&b.Source,
		&createdAt,
		&updatedAt,
	)
	if err != nil {
		return Bookmark{}, err
	}

	b.CreatedAt, err = time.Parse(time.RFC3339, createdAt)
	if err != nil {
		return Bookmark{}, err
	}

	b.UpdatedAt, err = time.Parse(time.RFC3339, updatedAt)
	if err != nil {
		return Bookmark{}, err
	}

	return b, nil
}
