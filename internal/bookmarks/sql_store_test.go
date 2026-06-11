package bookmarks

import (
	"database/sql"
	"path/filepath"
	"testing"

	_ "modernc.org/sqlite"
)

func TestOpenSQLStoreAppliesSchema(t *testing.T) {
	store, err := OpenSQLStore(filepath.Join(t.TempDir(), "bookmarks.db"))
	if err != nil {
		t.Fatalf("OpenSQLStore() error = %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	tables := []string{"bookmarks", "tags", "bookmark_tags"}
	for _, table := range tables {
		if !sqliteTableExists(t, store.db, table) {
			t.Fatalf("expected table %q to exist", table)
		}
	}
}

func TestSQLiteDSN(t *testing.T) {
	got := sqliteDSN("/tmp/book marks.db")
	want := "file:///tmp/book%20marks.db?_pragma=foreign_keys%281%29&_pragma=busy_timeout%285000%29&_pragma=journal_mode%28WAL%29"
	if got != want {
		t.Fatalf("sqliteDSN() = %q, want %q", got, want)
	}
}

func sqliteTableExists(t *testing.T, db *sql.DB, name string) bool {
	t.Helper()

	var count int
	err := db.QueryRow(`
		SELECT count(*)
		FROM sqlite_master
		WHERE type = 'table' AND name = ?
	`, name).Scan(&count)
	if err != nil {
		t.Fatalf("query sqlite_master: %v", err)
	}
	return count == 1
}
