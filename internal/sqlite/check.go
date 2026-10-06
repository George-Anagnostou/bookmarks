package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"net/url"
)

// CheckReport describes the database schema and bookmark count observed by
// Check. SchemaDifferences is empty when the schema matches the expected
// schema for the latest migration version.
type CheckReport struct {
	SchemaVersion     int      `json:"schema_version"`
	ExpectedVersion   int      `json:"expected_version"`
	BookmarkCount     int64    `json:"bookmark_count"`
	SchemaDifferences []string `json:"schema_differences,omitempty"`
}

// OK reports whether the observed database is at the latest version and its
// schema matches the expected schema.
func (r CheckReport) OK() bool {
	return r.SchemaVersion == r.ExpectedVersion && len(r.SchemaDifferences) == 0
}

// Check inspects a database without applying migrations. Operational failures
// are returned as errors; schema mismatches belong in SchemaDifferences.
func Check(ctx context.Context, path string) (CheckReport, error) {
	// Build a read-only SQLite URI. url.URL safely escapes spaces and other
	// special characters in the path.
	u := url.URL{Scheme: "file", Path: path}
	q := u.Query()
	q.Set("mode", "ro")
	u.RawQuery = q.Encode()

	db, err := sql.Open("sqlite", u.String())
	if err != nil {
		return CheckReport{}, err
	}
	defer db.Close()

	if err := db.PingContext(ctx); err != nil {
		return CheckReport{}, err
	}

	var version int
	if err := db.QueryRowContext(ctx, "PRAGMA user_version").Scan(&version); err != nil {
		return CheckReport{}, err
	}

	var count int64
	if err := db.QueryRowContext(ctx, "SELECT count(*) FROM bookmarks").Scan(&count); err != nil {
		return CheckReport{}, err
	}

	actualColumns, err := bookmarkColumns(ctx, db)
	if err != nil {
		return CheckReport{}, fmt.Errorf("inspect bookmark columns: %w", err)
	}

	expectedColumns, err := expectedBookmarkColumns(ctx)
	if err != nil {
		return CheckReport{}, fmt.Errorf("build expected bookmark schema: %w", err)
	}
	differences := compareBookmarkColumns(expectedColumns, actualColumns)

	return CheckReport{
		SchemaVersion:     version,
		ExpectedVersion:   len(migrations),
		BookmarkCount:     count,
		SchemaDifferences: differences,
	}, nil
}

type columnInfo struct {
	Name       string
	Type       string
	NotNull    int
	Default    sql.NullString
	PrimaryKey int
	Hidden     int
}

func bookmarkColumns(ctx context.Context, db *sql.DB) ([]columnInfo, error) {
	rows, err := db.QueryContext(ctx, "PRAGMA table_xinfo(bookmarks)")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var columns []columnInfo
	for rows.Next() {
		var cid int
		var column columnInfo

		err := rows.Scan(
			&cid,
			&column.Name,
			&column.Type,
			&column.NotNull,
			&column.Default,
			&column.PrimaryKey,
			&column.Hidden,
		)
		if err != nil {
			return nil, err
		}

		columns = append(columns, column)
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	return columns, nil
}

func expectedBookmarkColumns(ctx context.Context) ([]columnInfo, error) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		return nil, fmt.Errorf("open reference database: %w", err)
	}
	defer db.Close()
	db.SetMaxOpenConns(1)

	if err := migrate(ctx, db); err != nil {
		return nil, fmt.Errorf("apply migrations to reference database: %w", err)
	}

	columns, err := bookmarkColumns(ctx, db)
	if err != nil {
		return nil, fmt.Errorf("inspect reference bookmark columns: %w", err)
	}
	return columns, nil
}

func compareBookmarkColumns(expected, actual []columnInfo) []string {
	actualByName := make(map[string]columnInfo, len(actual))
	for _, column := range actual {
		actualByName[column.Name] = column
	}

	expectedNames := make(map[string]struct{}, len(expected))
	var differences []string
	for _, want := range expected {
		expectedNames[want.Name] = struct{}{}
		got, ok := actualByName[want.Name]
		if !ok {
			differences = append(differences, fmt.Sprintf("bookmarks.%s: missing column", want.Name))
			continue
		}

		if want.Type != got.Type {
			differences = append(differences, fmt.Sprintf("bookmarks.%s: expected type %s, found %s", want.Name, want.Type, got.Type))
		}
		if want.NotNull != got.NotNull {
			differences = append(differences, fmt.Sprintf("bookmarks.%s: expected not-null=%d, found %d", want.Name, want.NotNull, got.NotNull))
		}
		if want.Default != got.Default {
			differences = append(differences, fmt.Sprintf("bookmarks.%s: expected default %v, found %v", want.Name, want.Default, got.Default))
		}
		if want.PrimaryKey != got.PrimaryKey {
			differences = append(differences, fmt.Sprintf("bookmarks.%s: expected primary-key position %d, found %d", want.Name, want.PrimaryKey, got.PrimaryKey))
		}
		if want.Hidden != got.Hidden {
			differences = append(differences, fmt.Sprintf("bookmarks.%s: expected hidden=%d, found %d", want.Name, want.Hidden, got.Hidden))
		}
	}

	for _, got := range actual {
		if _, ok := expectedNames[got.Name]; !ok {
			differences = append(differences, fmt.Sprintf("bookmarks.%s: unexpected column", got.Name))
		}
	}

	return differences
}
