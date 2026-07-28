package bookmarks

import (
	"context"
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

func TestSQLStoreContract(t *testing.T) {
	runStoreContractTests(t, func(t *testing.T) Store {
		store, err := OpenSQLStore(filepath.Join(t.TempDir(), "bookmarks.db"))
		if err != nil {
			t.Fatalf("OpenSQLStore() error = %v", err)
		}
		t.Cleanup(func() { _ = store.Close() })
		return store
	})
}

func TestSQLStorePersistsAcrossReopen(t *testing.T) {
	path := filepath.Join(t.TempDir(), "bookmarks.db")

	store, err := OpenSQLStore(path)
	if err != nil {
		t.Fatal(err)
	}

	_, created, err := store.CreateBookmark(context.Background(), CreateInput{
		URL:   "https://example.com/a",
		Title: "Example",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !created {
		t.Fatal("created = false, want true")
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}

	store, err = OpenSQLStore(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })

	got, err := store.ListBookmarks(context.Background(), ListQuery{})
	if err != nil {
		t.Fatal(err)
	}

	if len(got) != 1 {
		t.Fatalf("len = %d, want 1", len(got))
	}
	if got[0].NormalizedURL != "https://example.com/a" {
		t.Fatalf("NormalizedURL = %q", got[0].NormalizedURL)
	}
}

func TestSQLStoreSetTitleIfBlank(t *testing.T) {
	tests := []struct {
		name         string
		storedTitle  string
		fetchedTitle string
		wantChanged  bool
		wantTitle    string
	}{
		{
			name:         "sets blank title",
			fetchedTitle: "Fetched title",
			wantChanged:  true,
			wantTitle:    "Fetched title",
		},
		{
			name:         "does not overwrite existing title",
			storedTitle:  "Written by the user",
			fetchedTitle: "Fetched title",
			wantChanged:  false,
			wantTitle:    "Written by the user",
		},
		{
			name:         "ignores blank fetched title",
			fetchedTitle: " \t ",
			wantChanged:  false,
			wantTitle:    "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store, err := OpenSQLStore(filepath.Join(t.TempDir(), "bookmarks.db"))
			if err != nil {
				t.Fatalf("OpenSQLStore() error = %v", err)
			}
			t.Cleanup(func() { _ = store.Close() })

			bookmark, created, err := store.CreateBookmark(context.Background(), CreateInput{
				URL:   "https://example.com/a",
				Title: tt.storedTitle,
			})
			if err != nil {
				t.Fatalf("CreateBookmark() error = %v", err)
			}
			if !created {
				t.Fatal("CreateBookmark() created = false, want true")
			}

			changed, err := store.SetTitleIfBlank(context.Background(), bookmark.ID, tt.fetchedTitle)
			if err != nil {
				t.Fatalf("SetTitleIfBlank() error = %v", err)
			}
			if changed != tt.wantChanged {
				t.Fatalf("SetTitleIfBlank() changed = %t, want %t", changed, tt.wantChanged)
			}

			bookmarks, err := store.ListBookmarks(context.Background(), ListQuery{})
			if err != nil {
				t.Fatalf("ListBookmarks() error = %v", err)
			}
			if len(bookmarks) != 1 {
				t.Fatalf("ListBookmarks() returned %d bookmarks, want 1", len(bookmarks))
			}
			if bookmarks[0].Title != tt.wantTitle {
				t.Fatalf("bookmark title = %q, want %q", bookmarks[0].Title, tt.wantTitle)
			}
		})
	}
}

func TestSQLStoreSetTitleIfBlankMissingBookmarkIsNoOp(t *testing.T) {
	store, err := OpenSQLStore(filepath.Join(t.TempDir(), "bookmarks.db"))
	if err != nil {
		t.Fatalf("OpenSQLStore() error = %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	changed, err := store.SetTitleIfBlank(context.Background(), "missing", "Fetched title")
	if err != nil {
		t.Fatalf("SetTitleIfBlank() error = %v", err)
	}
	if changed {
		t.Fatal("SetTitleIfBlank() changed = true, want false")
	}
}
