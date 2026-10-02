package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	sqlitedriver "modernc.org/sqlite"
	sqlite3 "modernc.org/sqlite/lib"

	"bookmarks/internal/bookmarks"
)

// CreateBookmark returns the bookmark and whether it was newly created.
// A duplicate normalized URL returns the existing bookmark without modifying it.
func (s *Store) CreateBookmark(ctx context.Context, input bookmarks.CreateInput) (bookmarks.Bookmark, bool, error) {
	id, err := newID()
	if err != nil {
		return bookmarks.Bookmark{}, false, err
	}

	normalizedURL, err := bookmarks.NormalizeURL(input.URL)
	if err != nil {
		return bookmarks.Bookmark{}, false, err
	}

	now := time.Now().UTC().Truncate(time.Second)

	bookmark := bookmarks.Bookmark{
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
		INSERT INTO bookmarks (
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
		ON CONFLICT (normalized_url) DO NOTHING
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
		return bookmarks.Bookmark{}, false, fmt.Errorf("insert bookmark: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return bookmarks.Bookmark{}, false, fmt.Errorf("check inserted bookmark: %w", err)
	}

	if rowsAffected == 0 {
		bookmark, err := s.bookmarkByNormalizedURL(ctx, normalizedURL)
		if err != nil {
			return bookmarks.Bookmark{}, false, err
		}

		return bookmark, false, nil
	}

	return bookmark, true, nil
}

// ListBookmarks returns matching bookmarks, newest first, with insertion order
// breaking timestamp ties. An empty result may be nil.
func (s *Store) ListBookmarks(ctx context.Context, options bookmarks.ListOptions) ([]bookmarks.Bookmark, error) {
	var args []any
	where := []string{
		"LOWER(url) LIKE ?",
		"LOWER(normalized_url) LIKE ?",
		"LOWER(title) LIKE ?",
		"LOWER(notes) LIKE ?",
		"LOWER(source) LIKE ?",
	}

	q := strings.TrimSpace(options.Query)

	query := `
		SELECT id, url, normalized_url, title, notes, source, created_at, updated_at
		FROM bookmarks
	`

	if q != "" {
		query += " WHERE " + strings.Join(where, " OR ")
		args = append(args, "%"+strings.ToLower(q)+"%", "%"+strings.ToLower(q)+"%", "%"+strings.ToLower(q)+"%", "%"+strings.ToLower(q)+"%", "%"+strings.ToLower(q)+"%")
	}

	query += " ORDER BY created_at DESC, rowid DESC "

	if options.Limit > 0 {
		query += " LIMIT ? "
		args = append(args, options.Limit)
	} else if options.Offset > 0 {
		query += " LIMIT -1 "
	}

	if options.Offset > 0 {
		query += " OFFSET ? "
		args = append(args, options.Offset)
	}

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list bookmarks: %w", err)
	}
	defer rows.Close()

	var bookmarks []bookmarks.Bookmark

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

// UpdateBookmark applies the supplied fields. It returns ErrNoUpdateFields for
// empty input, ErrNotFound for a missing bookmark, or ErrDuplicateURL for a URL
// belonging to another bookmark. These errors are defined in package bookmarks.
func (s *Store) UpdateBookmark(ctx context.Context, id string, input bookmarks.UpdateInput) (bookmarks.Bookmark, error) {
	var sets []string
	var args []any

	if input.URL != nil {
		rawURL := strings.TrimSpace(*input.URL)

		normalizedURL, err := bookmarks.NormalizeURL(rawURL)
		if err != nil {
			return bookmarks.Bookmark{}, err
		}

		sets = append(sets, "url = ?", "normalized_url = ?")
		args = append(args, rawURL, normalizedURL)
	}

	if input.Title != nil {
		sets = append(sets, "title = ?")
		args = append(args, *input.Title)
	}

	if input.Notes != nil {
		sets = append(sets, "notes = ?")
		args = append(args, *input.Notes)
	}

	if input.Source != nil {
		sets = append(sets, "source = ?")
		args = append(args, *input.Source)
	}

	if len(sets) == 0 {
		return bookmarks.Bookmark{}, bookmarks.ErrNoUpdateFields
	}

	now := time.Now().UTC().Truncate(time.Second)
	sets = append(sets, "updated_at = ?")
	args = append(args, now.Format(time.RFC3339))

	args = append(args, id)

	query := `
		UPDATE bookmarks
		SET ` + strings.Join(sets, ", ") + `
		WHERE id = ?
		RETURNING id, url, normalized_url, title, notes, source, created_at, updated_at
	`

	row := s.db.QueryRowContext(ctx, query, args...)

	bookmark, err := scanBookmark(row)
	if errors.Is(err, sql.ErrNoRows) {
		return bookmarks.Bookmark{}, bookmarks.ErrNotFound
	}

	// Map normalized_url unique constraint violations to domain duplicate error
	var sqliteErr *sqlitedriver.Error
	if errors.As(err, &sqliteErr) && sqliteErr.Code() == sqlite3.SQLITE_CONSTRAINT_UNIQUE {
		return bookmarks.Bookmark{}, bookmarks.ErrDuplicateURL
	}

	if err != nil {
		return bookmarks.Bookmark{}, fmt.Errorf("update bookmark: %w", err)
	}

	return bookmark, nil
}

// DeleteBookmark deletes a bookmark or returns bookmarks.ErrNotFound.
func (s *Store) DeleteBookmark(ctx context.Context, id string) error {
	result, err := s.db.ExecContext(ctx, `
		DELETE
		FROM bookmarks
		WHERE id = ?
		`, id)
	if err != nil {
		return fmt.Errorf("delete bookmark: %w", err)
	}

	numRows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("delete bookmark: %w", err)
	}

	if numRows == 0 {
		return bookmarks.ErrNotFound
	}
	return nil
}

// SetBookmarkTitleIfBlank atomically fills a blank title and reports whether it
// changed. A blank supplied title or missing bookmark is a no-op.
func (s *Store) SetBookmarkTitleIfBlank(ctx context.Context, id, title string) (bool, error) {
	title = strings.TrimSpace(title)
	if title == "" {
		return false, nil
	}

	now := time.Now().UTC().Truncate(time.Second)
	result, err := s.db.ExecContext(ctx, `
		UPDATE bookmarks
		SET title = ?, updated_at = ?
		WHERE id = ? AND TRIM(title) = ''
	`, title, now.Format(time.RFC3339), id)
	if err != nil {
		return false, fmt.Errorf("set title if blank: %w", err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("check title: %w", err)
	}
	return rows == 1, nil
}

func (s *Store) bookmarkByNormalizedURL(ctx context.Context, normalizedURL string) (bookmarks.Bookmark, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT id, url, normalized_url, title, notes, source, created_at, updated_at
		FROM bookmarks
		WHERE normalized_url = ?
	`, normalizedURL)

	bkmk, err := scanBookmark(row)
	if err != nil {
		return bookmarks.Bookmark{}, err
	}
	return bkmk, nil
}

type bookmarkScanner interface {
	Scan(dest ...any) error
}

func scanBookmark(scanner bookmarkScanner) (bookmarks.Bookmark, error) {
	var b bookmarks.Bookmark
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
		return bookmarks.Bookmark{}, err
	}

	b.CreatedAt, err = time.Parse(time.RFC3339, createdAt)
	if err != nil {
		return bookmarks.Bookmark{}, err
	}

	b.UpdatedAt, err = time.Parse(time.RFC3339, updatedAt)
	if err != nil {
		return bookmarks.Bookmark{}, err
	}

	return b, nil
}
