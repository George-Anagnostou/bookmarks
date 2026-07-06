package bookmarks

import (
	"context"
	"errors"
	"testing"
)

func runStoreContractTests(t *testing.T, newStore func(t *testing.T) Store) {
	t.Helper()

	t.Run("create and list bookmark", func(t *testing.T) {
		store := newStore(t)
		bookmark, created, err := store.CreateBookmark(context.Background(), CreateInput{
			URL:   "https://example.com/a",
			Title: "Example",
		})
		if err != nil {
			t.Fatalf("CreateBookmark() error = %v", err)
		}
		if !created {
			t.Fatal("CreateBookmark() created = false, want true")
		}
		if bookmark.NormalizedURL != "https://example.com/a" {
			t.Fatalf("NormalizedURL = %q", bookmark.NormalizedURL)
		}

		got, err := store.ListBookmarks(context.Background(), ListQuery{})
		if err != nil {
			t.Fatalf("ListBookmarks() error = %v", err)
		}
		if len(got) != 1 {
			t.Fatalf("ListBookmarks() returned %d bookmarks, want 1", len(got))
		}
	})

	t.Run("duplicate normalized url is idempotent", func(t *testing.T) {
		store := newStore(t)
		first, created, err := store.CreateBookmark(context.Background(), CreateInput{URL: "https://Example.com:443/a"})
		if err != nil {
			t.Fatalf("CreateBookmark() first error = %v", err)
		}
		if !created {
			t.Fatal("first CreateBookmark() created = false, want true")
		}

		second, created, err := store.CreateBookmark(context.Background(), CreateInput{URL: "https://example.com/a"})
		if err != nil {
			t.Fatalf("CreateBookmark() second error = %v", err)
		}
		if created {
			t.Fatal("second CreateBookmark() created = true, want false")
		}
		if second.ID != first.ID {
			t.Fatalf("duplicate returned ID %q, want %q", second.ID, first.ID)
		}
	})

	t.Run("check fields come back from ListBookmarks()", func(t *testing.T) {
		store := newStore(t)
		url := "https://example.com/a"
		title := "Example"
		source := "laptop"
		first, created, err := store.CreateBookmark(context.Background(), CreateInput{
			URL:    url,
			Title:  title,
			Source: source,
		})
		if err != nil {
			t.Fatalf("CreateBookmark() error = %v", err)
		}
		if !created {
			t.Fatalf("CreateBookmark() created = false, want true")
		}

		if first.URL != url {
			t.Fatalf("got %v, wanted %v", first.URL, url)
		}
		if first.Title != title {
			t.Fatalf("got %v, wanted %v", first.Title, title)
		}
		if first.Source != source {
			t.Fatalf("got %v, wanted %v", first.Source, source)
		}

		bookmarks, err := store.ListBookmarks(context.Background(), ListQuery{})
		if err != nil {
			t.Fatalf("ListBookmarks() error = %v", err)
		}

		if len(bookmarks) != 1 {
			t.Fatalf("ListBookmarks() returned %d bookmarks, want 1", len(bookmarks))
		}

		got := bookmarks[0]

		if got.URL != url {
			t.Fatalf("got %v, wanted %v", got.URL, url)
		}
		if got.Title != title {
			t.Fatalf("got %v, wanted %v", got.Title, title)
		}
		if got.Source != source {
			t.Fatalf("got %v, wanted %v", got.Source, source)
		}
	})

	t.Run("list bookmark query searches text fields", func(t *testing.T) {
		store := newStore(t)
		inputs := []CreateInput{
			{
				URL:    "https://sqlite.org/fts5.html",
				Title:  "SQLite FTS",
				Notes:  "reference documentation",
				Source: "desktop",
			},
			{
				URL:    "https://example.com/articles/go",
				Title:  "Go Article",
				Notes:  "mentions databases",
				Source: "ios-shortcut",
			},
			{
				URL:    "https://example.com/read-later",
				Title:  "Reading Queue",
				Notes:  "saved from phone",
				Source: "mobile",
			},
		}
		createdByURL := make(map[string]Bookmark)
		for _, input := range inputs {
			bookmark, created, err := store.CreateBookmark(context.Background(), input)
			if err != nil {
				t.Fatalf("CreateBookmark() error = %v", err)
			}
			if !created {
				t.Fatal("CreateBookmark() created = false, want true")
			}
			createdByURL[input.URL] = bookmark
		}

		tests := []struct {
			name    string
			query   string
			wantURL string
		}{
			{name: "title", query: "fts", wantURL: "https://sqlite.org/fts5.html"},
			{name: "url", query: "articles/go", wantURL: "https://example.com/articles/go"},
			{name: "normalized url", query: "sqlite.org", wantURL: "https://sqlite.org/fts5.html"},
			{name: "notes", query: "PHONE", wantURL: "https://example.com/read-later"},
			{name: "source", query: "ios", wantURL: "https://example.com/articles/go"},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				got, err := store.ListBookmarks(context.Background(), ListQuery{Query: tt.query})
				if err != nil {
					t.Fatalf("ListBookmarks() error = %v", err)
				}
				if len(got) != 1 {
					t.Fatalf("ListBookmarks() returned %d bookmarks, want 1: %#v", len(got), got)
				}
				if got[0].ID != createdByURL[tt.wantURL].ID {
					t.Fatalf("bookmark ID = %q, want %q", got[0].ID, createdByURL[tt.wantURL].ID)
				}
			})
		}
	})

	t.Run("list bookmark query trims whitespace and empty query lists all", func(t *testing.T) {
		store := newStore(t)
		first, created, err := store.CreateBookmark(context.Background(), CreateInput{
			URL:   "https://example.com/first",
			Title: "First",
		})
		if err != nil {
			t.Fatalf("CreateBookmark() first error = %v", err)
		}
		if !created {
			t.Fatal("first CreateBookmark() created = false, want true")
		}
		second, created, err := store.CreateBookmark(context.Background(), CreateInput{
			URL:   "https://example.com/second",
			Title: "Second",
		})
		if err != nil {
			t.Fatalf("CreateBookmark() second error = %v", err)
		}
		if !created {
			t.Fatal("second CreateBookmark() created = false, want true")
		}

		got, err := store.ListBookmarks(context.Background(), ListQuery{Query: "  first  "})
		if err != nil {
			t.Fatalf("ListBookmarks() trimmed query error = %v", err)
		}
		if len(got) != 1 || got[0].ID != first.ID {
			t.Fatalf("trimmed query returned %#v, want first bookmark", got)
		}

		got, err = store.ListBookmarks(context.Background(), ListQuery{Query: "   "})
		if err != nil {
			t.Fatalf("ListBookmarks() empty query error = %v", err)
		}
		if len(got) != 2 {
			t.Fatalf("empty query returned %d bookmarks, want 2", len(got))
		}
		if got[0].ID != second.ID || got[1].ID != first.ID {
			t.Fatalf("empty query order = %#v, want newest first", got)
		}
	})

	t.Run("list bookmark limit and offset page newest first results", func(t *testing.T) {
		store := newStore(t)
		for _, url := range []string{
			"https://example.com/one",
			"https://example.com/two",
			"https://example.com/three",
			"https://example.com/four",
		} {
			_, created, err := store.CreateBookmark(context.Background(), CreateInput{URL: url})
			if err != nil {
				t.Fatalf("CreateBookmark(%q) error = %v", url, err)
			}
			if !created {
				t.Fatalf("CreateBookmark(%q) created = false, want true", url)
			}
		}

		all, err := store.ListBookmarks(context.Background(), ListQuery{})
		if err != nil {
			t.Fatalf("ListBookmarks() all error = %v", err)
		}
		if len(all) != 4 {
			t.Fatalf("ListBookmarks() all returned %d bookmarks, want 4", len(all))
		}

		got, err := store.ListBookmarks(context.Background(), ListQuery{
			Limit:  2,
			Offset: 1,
		})
		if err != nil {
			t.Fatalf("ListBookmarks() error = %v", err)
		}
		if len(got) != 2 {
			t.Fatalf("ListBookmarks() returned %d bookmarks, want 2", len(got))
		}

		want := all[1:3]
		for i := range want {
			if got[i].ID != want[i].ID {
				t.Fatalf("bookmark %d ID = %q, want %q", i, got[i].ID, want[i].ID)
			}
		}
	})

	t.Run("list bookmark query limit and offset compose", func(t *testing.T) {
		store := newStore(t)
		for _, input := range []CreateInput{
			{URL: "https://example.com/go-one", Title: "Go one"},
			{URL: "https://example.com/rust", Title: "Rust"},
			{URL: "https://example.com/go-two", Title: "Go two"},
			{URL: "https://example.com/go-three", Title: "Go three"},
		} {
			_, created, err := store.CreateBookmark(context.Background(), input)
			if err != nil {
				t.Fatalf("CreateBookmark(%q) error = %v", input.URL, err)
			}
			if !created {
				t.Fatalf("CreateBookmark(%q) created = false, want true", input.URL)
			}
		}

		allGo, err := store.ListBookmarks(context.Background(), ListQuery{Query: "go"})
		if err != nil {
			t.Fatalf("ListBookmarks() all matching error = %v", err)
		}
		if len(allGo) != 3 {
			t.Fatalf("ListBookmarks() all matching returned %d bookmarks, want 3", len(allGo))
		}

		got, err := store.ListBookmarks(context.Background(), ListQuery{
			Query:  "go",
			Limit:  1,
			Offset: 1,
		})
		if err != nil {
			t.Fatalf("ListBookmarks() error = %v", err)
		}
		if len(got) != 1 {
			t.Fatalf("ListBookmarks() returned %d bookmarks, want 1", len(got))
		}
		if got[0].ID != allGo[1].ID {
			t.Fatalf("bookmark ID = %q, want %q", got[0].ID, allGo[1].ID)
		}
	})

	t.Run("check unsupported schemas", func(t *testing.T) {
		store := newStore(t)
		_, created, err := store.CreateBookmark(context.Background(), CreateInput{URL: "ftp://example.com"})
		if !errors.Is(err, ErrUnsupported) {
			t.Fatalf("CreateBookmark failed to error: %v, expected %v", err, ErrUnsupported)
		}
		if created {
			t.Fatalf("CreateBookmark failed to error: %v, expected %v", err, ErrUnsupported)
		}
	})

	t.Run("update bookmark fields", func(t *testing.T) {
		store := newStore(t)
		original, created, err := store.CreateBookmark(context.Background(), CreateInput{
			URL:    "https://example.com/a",
			Title:  "Example",
			Notes:  "Original notes",
			Source: "laptop",
		})
		if err != nil {
			t.Fatalf("CreateBookmark() error = %v", err)
		}
		if !created {
			t.Fatal("CreateBookmark() created = false, want true")
		}

		updatedTitle := "Updated Example"
		updatedSource := "different laptop"
		updatedNotes := "These are some notes"

		updated, err := store.UpdateBookmark(context.Background(), original.ID, UpdateInput{
			Title:  &updatedTitle,
			Notes:  &updatedNotes,
			Source: &updatedSource,
		})
		if err != nil {
			t.Fatalf("UpdateBookmark() error = %v", err)
		}
		if updated.ID != original.ID {
			t.Fatalf("ID = %q, want %q", updated.ID, original.ID)
		}
		if updated.URL != original.URL {
			t.Fatalf("URL = %q, want unchanged %q", updated.URL, original.URL)
		}
		if updated.NormalizedURL != original.NormalizedURL {
			t.Fatalf("NormalizedURL = %q, want unchanged %q", updated.NormalizedURL, original.NormalizedURL)
		}
		if updated.Title != updatedTitle {
			t.Fatalf("Title = %q, want %q", updated.Title, updatedTitle)
		}
		if updated.Notes != updatedNotes {
			t.Fatalf("Notes = %q, want %q", updated.Notes, updatedNotes)
		}
		if updated.Source != updatedSource {
			t.Fatalf("Source = %q, want %q", updated.Source, updatedSource)
		}
		if !updated.CreatedAt.Equal(original.CreatedAt) {
			t.Fatalf("CreatedAt = %v, want unchanged %v", updated.CreatedAt, original.CreatedAt)
		}
		if updated.UpdatedAt.Before(original.UpdatedAt) {
			t.Fatalf("UpdatedAt = %v, want not before original %v", updated.UpdatedAt, original.UpdatedAt)
		}
	})

	t.Run("update bookmark can clear fields", func(t *testing.T) {
		store := newStore(t)
		original, created, err := store.CreateBookmark(context.Background(), CreateInput{
			URL:    "https://example.com/a",
			Title:  "Example",
			Notes:  "Original notes",
			Source: "laptop",
		})
		if err != nil {
			t.Fatalf("CreateBookmark() error = %v", err)
		}
		if !created {
			t.Fatal("CreateBookmark() created = false, want true")
		}

		updated, err := store.UpdateBookmark(context.Background(), original.ID, UpdateInput{
			Title:  stringPtr(""),
			Notes:  stringPtr(""),
			Source: stringPtr(""),
		})
		if err != nil {
			t.Fatalf("UpdateBookmark() error = %v", err)
		}
		if updated.URL != original.URL {
			t.Fatalf("URL = %q, want unchanged %q", updated.URL, original.URL)
		}
		if updated.Title != "" {
			t.Fatalf("Title = %q, want empty", updated.Title)
		}
		if updated.Notes != "" {
			t.Fatalf("Notes = %q, want empty", updated.Notes)
		}
		if updated.Source != "" {
			t.Fatalf("Source = %q, want empty", updated.Source)
		}
	})

	t.Run("update bookmark trims and normalizes url", func(t *testing.T) {
		store := newStore(t)
		original, created, err := store.CreateBookmark(context.Background(), CreateInput{
			URL:    "https://example.com/a",
			Title:  "Example",
			Notes:  "Original notes",
			Source: "laptop",
		})
		if err != nil {
			t.Fatalf("CreateBookmark() error = %v", err)
		}
		if !created {
			t.Fatal("CreateBookmark() created = false, want true")
		}

		updatedURL := "  hTTpS://exAMPLE.Com:443/b  "
		updated, err := store.UpdateBookmark(context.Background(), original.ID, UpdateInput{
			URL: &updatedURL,
		})
		if err != nil {
			t.Fatalf("UpdateBookmark() error = %v", err)
		}

		if updated.URL != "hTTpS://exAMPLE.Com:443/b" {
			t.Fatalf("URL = %q, want trimmed literal URL %q", updated.URL, "hTTpS://exAMPLE.Com:443/b")
		}
		if updated.NormalizedURL != "https://example.com/b" {
			t.Fatalf("NormalizedURL = %q, want %q", updated.NormalizedURL, "https://example.com/b")
		}
		if updated.Title != original.Title {
			t.Fatalf("Title = %q, want unchanged %q", updated.Title, original.Title)
		}
		if updated.Notes != original.Notes {
			t.Fatalf("Notes = %q, want unchanged %q", updated.Notes, original.Notes)
		}
		if updated.Source != original.Source {
			t.Fatalf("Source = %q, want unchanged %q", updated.Source, original.Source)
		}
	})

	t.Run("update bookmark returns ErrNoUpdateFields error on empty input", func(t *testing.T) {
		store := newStore(t)
		original, created, err := store.CreateBookmark(context.Background(), CreateInput{
			URL:   "https://example.com/a",
			Title: "Example",
		})
		if err != nil {
			t.Fatalf("CreateBookmark() error = %v", err)
		}
		if !created {
			t.Fatal("CreateBookmark() created = false, want true")
		}

		_, err = store.UpdateBookmark(context.Background(), original.ID, UpdateInput{})
		if !errors.Is(err, ErrNoUpdateFields) {
			t.Fatalf("UpdateBookmark() error = %v, want %v", err, ErrNoUpdateFields)
		}
	})

	t.Run("update bookmark returns ErrNotFound error on unknown id", func(t *testing.T) {
		store := newStore(t)
		_, created, err := store.CreateBookmark(context.Background(), CreateInput{
			URL:   "https://example.com/a",
			Title: "Example",
		})
		if err != nil {
			t.Fatalf("CreateBookmark() error = %v", err)
		}
		if !created {
			t.Fatal("CreateBookmark() created = false, want true")
		}

		_, err = store.UpdateBookmark(context.Background(), "abc123", UpdateInput{
			Title: stringPtr("Updated Title"),
		})
		if !errors.Is(err, ErrNotFound) {
			t.Fatalf("UpdateBookmark() error = %v, want %v", err, ErrNotFound)
		}
	})

	t.Run("update bookmark returns url validation errors", func(t *testing.T) {
		tests := []struct {
			name    string
			url     string
			wantErr error
		}{
			{
				name:    "empty url",
				url:     "",
				wantErr: ErrEmptyURL,
			},
			{
				name:    "missing host",
				url:     "https:///path",
				wantErr: ErrMissingHost,
			},
			{
				name:    "url user info",
				url:     "https://user@example.com:443/a",
				wantErr: ErrURLUserInfo,
			},
			{
				name:    "unsupported url",
				url:     "ftp://example.com",
				wantErr: ErrUnsupported,
			},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				store := newStore(t)
				original, created, err := store.CreateBookmark(context.Background(), CreateInput{
					URL:   "https://example.com/a",
					Title: "Example",
				})
				if err != nil {
					t.Fatalf("CreateBookmark() error = %v", err)
				}
				if !created {
					t.Fatal("CreateBookmark() created = false, want true")
				}

				_, err = store.UpdateBookmark(context.Background(), original.ID, UpdateInput{
					URL: &tt.url,
				})
				if !errors.Is(err, tt.wantErr) {
					t.Fatalf("UpdateBookmark() error = %v, want %v", err, tt.wantErr)
				}
			})
		}
	})

	t.Run("update bookmark returns ErrDuplicateURL when normalized url already exists", func(t *testing.T) {
		store := newStore(t)
		first, created, err := store.CreateBookmark(context.Background(), CreateInput{
			URL:   "https://example.com/a",
			Title: "First",
		})
		if err != nil {
			t.Fatalf("CreateBookmark() first error = %v", err)
		}
		if !created {
			t.Fatal("first CreateBookmark() created = false, want true")
		}

		second, created, err := store.CreateBookmark(context.Background(), CreateInput{
			URL:   "https://example.com/b",
			Title: "Second",
		})
		if err != nil {
			t.Fatalf("CreateBookmark() second error = %v", err)
		}
		if !created {
			t.Fatal("second CreateBookmark() created = false, want true")
		}

		duplicateURL := "https://EXAMPLE.com:443/a"
		_, err = store.UpdateBookmark(context.Background(), second.ID, UpdateInput{
			URL: &duplicateURL,
		})
		if !errors.Is(err, ErrDuplicateURL) {
			t.Fatalf("UpdateBookmark() error = %v, want %v", err, ErrDuplicateURL)
		}

		got, err := store.ListBookmarks(context.Background(), ListQuery{})
		if err != nil {
			t.Fatalf("ListBookmarks() error = %v", err)
		}
		if len(got) != 2 {
			t.Fatalf("ListBookmarks() returned %d bookmarks, want 2", len(got))
		}
		for _, bookmark := range got {
			if bookmark.ID == first.ID && bookmark.URL != first.URL {
				t.Fatalf("first bookmark URL = %q, want unchanged %q", bookmark.URL, first.URL)
			}
			if bookmark.ID == second.ID && bookmark.URL != second.URL {
				t.Fatalf("second bookmark URL = %q, want unchanged %q", bookmark.URL, second.URL)
			}
		}
	})

	t.Run("update bookmark allows its own normalized url", func(t *testing.T) {
		store := newStore(t)
		original, created, err := store.CreateBookmark(context.Background(), CreateInput{
			URL:   "https://Example.com:443/a",
			Title: "Example",
		})
		if err != nil {
			t.Fatalf("CreateBookmark() error = %v", err)
		}
		if !created {
			t.Fatal("CreateBookmark() created = false, want true")
		}

		updatedURL := "  https://example.com/a  "
		updated, err := store.UpdateBookmark(context.Background(), original.ID, UpdateInput{
			URL: &updatedURL,
		})
		if err != nil {
			t.Fatalf("UpdateBookmark() error = %v", err)
		}
		if updated.ID != original.ID {
			t.Fatalf("ID = %q, want %q", updated.ID, original.ID)
		}
		if updated.URL != "https://example.com/a" {
			t.Fatalf("URL = %q, want trimmed literal URL %q", updated.URL, "https://example.com/a")
		}
		if updated.NormalizedURL != original.NormalizedURL {
			t.Fatalf("NormalizedURL = %q, want unchanged %q", updated.NormalizedURL, original.NormalizedURL)
		}
		if updated.Title != original.Title {
			t.Fatalf("Title = %q, want unchanged %q", updated.Title, original.Title)
		}
	})

	t.Run("delete bookmark removes bookmark", func(t *testing.T) {
		store := newStore(t)
		original, created, err := store.CreateBookmark(context.Background(), CreateInput{
			URL:   "https://example.com/a",
			Title: "Example",
		})
		if err != nil {
			t.Fatalf("CreateBookmark() error = %v", err)
		}
		if !created {
			t.Fatal("CreateBookmark() created = false, want true")
		}

		if err := store.DeleteBookmark(context.Background(), original.ID); err != nil {
			t.Fatalf("DeleteBookmark() error = %v", err)
		}

		got, err := store.ListBookmarks(context.Background(), ListQuery{})
		if err != nil {
			t.Fatalf("ListBookmarks() error = %v", err)
		}
		if len(got) != 0 {
			t.Fatalf("ListBookmarks() returned %d bookmarks, want 0", len(got))
		}
	})

	t.Run("delete bookmark removes only one bookmark", func(t *testing.T) {
		store := newStore(t)
		first, created, err := store.CreateBookmark(context.Background(), CreateInput{
			URL:   "https://example.com/a",
			Title: "Example",
		})
		if err != nil {
			t.Fatalf("CreateBookmark() error = %v", err)
		}
		if !created {
			t.Fatal("CreateBookmark() created = false, want true")
		}

		second, created, err := store.CreateBookmark(context.Background(), CreateInput{
			URL:   "https://example.com/b",
			Title: "Example 2",
		})
		if err != nil {
			t.Fatalf("CreateBookmark() error = %v", err)
		}
		if !created {
			t.Fatal("CreateBookmark() created = false, want true")
		}
		if second.ID == first.ID {
			t.Fatalf("bookmark IDs should not be equal: %v should not equal %v", second.ID, first.ID)
		}

		if err := store.DeleteBookmark(context.Background(), first.ID); err != nil {
			t.Fatalf("DeleteBookmark() error = %v", err)
		}

		got, err := store.ListBookmarks(context.Background(), ListQuery{})
		if err != nil {
			t.Fatalf("ListBookmarks() error = %v", err)
		}
		if len(got) != 1 {
			t.Fatalf("ListBookmarks() returned %d bookmarks, want 1", len(got))
		}
		if got[0].ID != second.ID {
			t.Fatalf("remaining bookmark ID = %q, want %q", got[0].ID, second.ID)
		}
	})

	t.Run("delete bookmark returns ErrNotFound on unknown ID", func(t *testing.T) {
		store := newStore(t)
		original, created, err := store.CreateBookmark(context.Background(), CreateInput{
			URL:   "https://example.com/a",
			Title: "Example",
		})
		if err != nil {
			t.Fatalf("CreateBookmark() error = %v", err)
		}
		if !created {
			t.Fatal("CreateBookmark() created = false, want true")
		}

		if err := store.DeleteBookmark(context.Background(), "abc123"); !errors.Is(err, ErrNotFound) {
			t.Fatalf("DeleteBookmark() error = %v, want %v", err, ErrNotFound)
		}

		got, err := store.ListBookmarks(context.Background(), ListQuery{})
		if err != nil {
			t.Fatalf("ListBookmarks() error = %v", err)
		}
		if len(got) != 1 {
			t.Fatalf("ListBookmarks() returned %d bookmarks, want 1", len(got))
		}
		if got[0].ID != original.ID {
			t.Fatalf("remaining bookmark ID = %q, want %q", got[0].ID, original.ID)
		}
	})
}

func stringPtr(s string) *string {
	return &s
}
