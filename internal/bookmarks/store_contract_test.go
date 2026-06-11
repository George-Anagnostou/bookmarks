package bookmarks

import (
	"context"
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

		got, err := store.ListBookmarks(context.Background())
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
			t.Fatalf("ListBookmarks() error = %v", err)
		}
		if !created {
			t.Fatalf("ListBookmarks() created = false, want true")
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
	})

	t.Run("check unsupported schemas", func(t *testing.T) {
		store := newStore(t)
		_, created, err := store.CreateBookmark(context.Background(), CreateInput{URL: "ftp://example.com"})
		if err != ErrUnsupported {
			t.Fatalf("CreateBookmark failed to error: %v, expected %v", err, ErrUnsupported)
		}
		if created {
			t.Fatalf("CreateBookmark failed to error: %v, expected %v", err, ErrUnsupported)
		}
	})
}
