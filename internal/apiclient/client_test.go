package apiclient

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"
	"time"

	"bookmarks/internal/bookmarks"
)

func TestCreateBookmark(t *testing.T) {
	now := time.Date(2026, 6, 14, 12, 0, 0, 0, time.UTC)
	wantBookmark := bookmarks.Bookmark{
		ID:            "bookmark-1",
		URL:           "https://example.com/a",
		NormalizedURL: "https://example.com/a",
		Title:         "Example",
		Notes:         "Read later",
		Source:        "bookmarkctl",
		CreatedAt:     now,
		UpdatedAt:     now,
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("method = %s, want %s", r.Method, http.MethodPost)
		}
		if r.URL.Path != "/api/bookmarks" {
			t.Fatalf("path = %s, want /api/bookmarks", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer test-token" {
			t.Fatalf("Authorization = %q, want bearer token", got)
		}
		if got := r.Header.Get("Content-Type"); got != "application/json" {
			t.Fatalf("Content-Type = %q, want application/json", got)
		}

		var input bookmarks.CreateInput
		if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
			t.Fatalf("decode request body: %v", err)
		}
		if input.URL != "https://example.com/a" {
			t.Fatalf("input.URL = %q", input.URL)
		}
		if input.Title != "Example" {
			t.Fatalf("input.Title = %q", input.Title)
		}
		if input.Notes != "Read later" {
			t.Fatalf("input.Notes = %q", input.Notes)
		}
		if input.Source != "bookmarkctl" {
			t.Fatalf("input.Source = %q", input.Source)
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(struct {
			Bookmark bookmarks.Bookmark `json:"bookmark"`
			Created  bool               `json:"created"`
		}{
			Bookmark: wantBookmark,
			Created:  true,
		})
	}))
	t.Cleanup(server.Close)

	client := newTestClient(t, server.URL)
	gotBookmark, created, err := client.CreateBookmark(context.Background(), bookmarks.CreateInput{
		URL:    "https://example.com/a",
		Title:  "Example",
		Notes:  "Read later",
		Source: "bookmarkctl",
	})
	if err != nil {
		t.Fatalf("CreateBookmark() error = %v", err)
	}
	if !created {
		t.Fatal("created = false, want true")
	}
	if !reflect.DeepEqual(gotBookmark, wantBookmark) {
		t.Fatalf("bookmark = %#v, want %#v", gotBookmark, wantBookmark)
	}
}

func TestCreateBookmarkDuplicate(t *testing.T) {
	wantBookmark := bookmarks.Bookmark{
		ID:            "bookmark-1",
		URL:           "https://example.com/a",
		NormalizedURL: "https://example.com/a",
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(struct {
			Bookmark bookmarks.Bookmark `json:"bookmark"`
			Created  bool               `json:"created"`
		}{
			Bookmark: wantBookmark,
			Created:  false,
		})
	}))
	t.Cleanup(server.Close)

	client := newTestClient(t, server.URL)
	gotBookmark, created, err := client.CreateBookmark(context.Background(), bookmarks.CreateInput{
		URL: "https://example.com/a",
	})
	if err != nil {
		t.Fatalf("CreateBookmark() error = %v", err)
	}
	if created {
		t.Fatal("created = true, want false")
	}
	if !reflect.DeepEqual(gotBookmark, wantBookmark) {
		t.Fatalf("bookmark = %#v, want %#v", gotBookmark, wantBookmark)
	}
}

func TestCreateBookmarkReturnsErrorForNonSuccessStatus(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		_ = json.NewEncoder(w).Encode(struct {
			Error string `json:"error"`
		}{
			Error: "unauthorized",
		})
	}))
	t.Cleanup(server.Close)

	client := newTestClient(t, server.URL)
	_, _, err := client.CreateBookmark(context.Background(), bookmarks.CreateInput{
		URL: "https://example.com/a",
	})
	if err == nil {
		t.Fatal("CreateBookmark() error = nil, want error")
	}
}

func TestListBookmarks(t *testing.T) {
	now := time.Date(2026, 6, 14, 12, 0, 0, 0, time.UTC)
	want := []bookmarks.Bookmark{
		{
			ID:            "bookmark-2",
			URL:           "https://example.com/b",
			NormalizedURL: "https://example.com/b",
			Title:         "Second",
			CreatedAt:     now,
			UpdatedAt:     now,
		},
		{
			ID:            "bookmark-1",
			URL:           "https://example.com/a",
			NormalizedURL: "https://example.com/a",
			Title:         "First",
			CreatedAt:     now.Add(-time.Hour),
			UpdatedAt:     now.Add(-time.Hour),
		},
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Fatalf("method = %s, want %s", r.Method, http.MethodGet)
		}
		if r.URL.Path != "/api/bookmarks" {
			t.Fatalf("path = %s, want /api/bookmarks", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer test-token" {
			t.Fatalf("Authorization = %q, want bearer token", got)
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(struct {
			Bookmarks []bookmarks.Bookmark `json:"bookmarks"`
		}{
			Bookmarks: want,
		})
	}))
	t.Cleanup(server.Close)

	client := newTestClient(t, server.URL)
	got, err := client.ListBookmarks(context.Background())
	if err != nil {
		t.Fatalf("ListBookmarks() error = %v", err)
	}
	if len(got) != len(want) {
		t.Fatalf("len(bookmarks) = %d, want %d", len(got), len(want))
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("bookmarks = %#v, want %#v", got, want)
	}
}

func TestListBookmarksEmpty(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(struct {
			Bookmarks []bookmarks.Bookmark `json:"bookmarks"`
		}{
			Bookmarks: []bookmarks.Bookmark{},
		})
	}))
	t.Cleanup(server.Close)

	client := newTestClient(t, server.URL)
	got, err := client.ListBookmarks(context.Background())
	if err != nil {
		t.Fatalf("ListBookmarks() error = %v", err)
	}
	if got == nil {
		t.Fatal("bookmarks = nil, want empty slice")
	}
	if len(got) != 0 {
		t.Fatalf("len(bookmarks) = %d, want 0", len(got))
	}
}

func TestUpdateBookmark(t *testing.T) {
	now := time.Date(2026, 6, 24, 12, 0, 0, 0, time.UTC)
	wantBookmark := bookmarks.Bookmark{
		ID:            "bookmark-1",
		URL:           "https://example.com/new",
		NormalizedURL: "https://example.com/new",
		Title:         "Updated",
		Notes:         "",
		Source:        "bookmarkctl",
		CreatedAt:     now.Add(-time.Hour),
		UpdatedAt:     now,
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPatch {
			t.Fatalf("method = %s, want %s", r.Method, http.MethodPatch)
		}
		if r.URL.Path != "/api/bookmarks/bookmark-1" {
			t.Fatalf("path = %s, want /api/bookmarks/bookmark-1", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer test-token" {
			t.Fatalf("Authorization = %q, want bearer token", got)
		}
		if got := r.Header.Get("Content-Type"); got != "application/json" {
			t.Fatalf("Content-Type = %q, want application/json", got)
		}

		var input bookmarks.UpdateInput
		if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
			t.Fatalf("decode request body: %v", err)
		}
		if input.URL == nil || *input.URL != "https://example.com/new" {
			t.Fatalf("input.URL = %#v, want https://example.com/new", input.URL)
		}
		if input.Title == nil || *input.Title != "Updated" {
			t.Fatalf("input.Title = %#v, want Updated", input.Title)
		}
		if input.Notes == nil || *input.Notes != "" {
			t.Fatalf("input.Notes = %#v, want empty string pointer", input.Notes)
		}
		if input.Source == nil || *input.Source != "bookmarkctl" {
			t.Fatalf("input.Source = %#v, want bookmarkctl", input.Source)
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(struct {
			Bookmark bookmarks.Bookmark `json:"bookmark"`
		}{
			Bookmark: wantBookmark,
		})
	}))
	t.Cleanup(server.Close)

	client := newTestClient(t, server.URL)
	gotBookmark, err := client.UpdateBookmark(context.Background(), "bookmark-1", bookmarks.UpdateInput{
		URL:    stringPtr("https://example.com/new"),
		Title:  stringPtr("Updated"),
		Notes:  stringPtr(""),
		Source: stringPtr("bookmarkctl"),
	})
	if err != nil {
		t.Fatalf("UpdateBookmark() error = %v", err)
	}
	if !reflect.DeepEqual(gotBookmark, wantBookmark) {
		t.Fatalf("bookmark = %#v, want %#v", gotBookmark, wantBookmark)
	}
}

func TestUpdateBookmarkReturnsErrorForNonSuccessStatus(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusConflict)
		_ = json.NewEncoder(w).Encode(struct {
			Error string `json:"error"`
		}{
			Error: "conflict",
		})
	}))
	t.Cleanup(server.Close)

	client := newTestClient(t, server.URL)
	_, err := client.UpdateBookmark(context.Background(), "bookmark-1", bookmarks.UpdateInput{
		Title: stringPtr("Updated"),
	})
	if err == nil {
		t.Fatal("UpdateBookmark() error = nil, want error")
	}
}

func TestUpdateBookmarkRequiresOKStatus(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(struct {
			Bookmark bookmarks.Bookmark `json:"bookmark"`
		}{
			Bookmark: bookmarks.Bookmark{ID: "bookmark-1"},
		})
	}))
	t.Cleanup(server.Close)

	client := newTestClient(t, server.URL)
	_, err := client.UpdateBookmark(context.Background(), "bookmark-1", bookmarks.UpdateInput{
		Title: stringPtr("Updated"),
	})
	if err == nil {
		t.Fatal("UpdateBookmark() error = nil, want error")
	}
}

func TestUpdateBookmarkPathEscapesID(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.RequestURI != "/api/bookmarks/folder%2Fbookmark%201" {
			t.Fatalf("RequestURI = %q, want escaped bookmark ID", r.RequestURI)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(struct {
			Bookmark bookmarks.Bookmark `json:"bookmark"`
		}{
			Bookmark: bookmarks.Bookmark{ID: "folder/bookmark 1"},
		})
	}))
	t.Cleanup(server.Close)

	client := newTestClient(t, server.URL)
	if _, err := client.UpdateBookmark(context.Background(), "folder/bookmark 1", bookmarks.UpdateInput{
		Title: stringPtr("Updated"),
	}); err != nil {
		t.Fatalf("UpdateBookmark() error = %v", err)
	}
}

func TestDeleteBookmark(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete {
			t.Fatalf("method = %s, want %s", r.Method, http.MethodDelete)
		}
		if r.URL.Path != "/api/bookmarks/bookmark-1" {
			t.Fatalf("path = %s, want /api/bookmarks/bookmark-1", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer test-token" {
			t.Fatalf("Authorization = %q, want bearer token", got)
		}

		w.WriteHeader(http.StatusNoContent)
	}))
	t.Cleanup(server.Close)

	client := newTestClient(t, server.URL)
	if err := client.DeleteBookmark(context.Background(), "bookmark-1"); err != nil {
		t.Fatalf("DeleteBookmark() error = %v", err)
	}
}

func TestDeleteBookmarkReturnsErrorForNonSuccessStatus(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		_ = json.NewEncoder(w).Encode(struct {
			Error string `json:"error"`
		}{
			Error: "not found",
		})
	}))
	t.Cleanup(server.Close)

	client := newTestClient(t, server.URL)
	if err := client.DeleteBookmark(context.Background(), "bookmark-1"); err == nil {
		t.Fatal("DeleteBookmark() error = nil, want error")
	}
}

func TestDeleteBookmarkRequiresNoContentStatus(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(struct {
			Deleted bool `json:"deleted"`
		}{
			Deleted: true,
		})
	}))
	t.Cleanup(server.Close)

	client := newTestClient(t, server.URL)
	if err := client.DeleteBookmark(context.Background(), "bookmark-1"); err == nil {
		t.Fatal("DeleteBookmark() error = nil, want error")
	}
}

func TestDeleteBookmarkPathEscapesID(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.RequestURI != "/api/bookmarks/folder%2Fbookmark%201" {
			t.Fatalf("RequestURI = %q, want escaped bookmark ID", r.RequestURI)
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	t.Cleanup(server.Close)

	client := newTestClient(t, server.URL)
	if err := client.DeleteBookmark(context.Background(), "folder/bookmark 1"); err != nil {
		t.Fatalf("DeleteBookmark() error = %v", err)
	}
}

func TestClientReturnsErrorForNonSuccessStatus(t *testing.T) {
	tests := []struct {
		name   string
		status int
	}{
		{
			name:   "unauthorized",
			status: http.StatusUnauthorized,
		},
		{
			name:   "server error",
			status: http.StatusInternalServerError,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(tt.status)
				_ = json.NewEncoder(w).Encode(struct {
					Error string `json:"error"`
				}{
					Error: http.StatusText(tt.status),
				})
			}))
			t.Cleanup(server.Close)

			client := newTestClient(t, server.URL)
			_, err := client.ListBookmarks(context.Background())
			if err == nil {
				t.Fatal("ListBookmarks() error = nil, want error")
			}
		})
	}
}

func TestNewAcceptsConfig(t *testing.T) {
	client, err := New(Config{
		BaseURL: "https://bookmarks.example.com/",
		Token:   "test-token",
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	if client.baseURL != "https://bookmarks.example.com" {
		t.Fatalf("baseURL = %q, want trailing slash trimmed", client.baseURL)
	}
	if client.token != "test-token" {
		t.Fatalf("token = %q, want configured token", client.token)
	}
	if client.httpClient != http.DefaultClient {
		t.Fatal("httpClient was not defaulted")
	}
}

func TestNewRejectsBadConfig(t *testing.T) {
	tests := []struct {
		name string
		cfg  Config
	}{
		{
			name: "missing base url",
			cfg: Config{
				Token: "test-token",
			},
		},
		{
			name: "missing token",
			cfg: Config{
				BaseURL: "https://bookmarks.example.com",
			},
		},
		{
			name: "token has newline",
			cfg: Config{
				Token: "test-token\n",
			},
		},
		{
			name: "invalid base url",
			cfg: Config{
				BaseURL: "://bad-url",
				Token:   "test-token",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := New(tt.cfg)
			if err == nil {
				t.Fatal("New() error = nil, want error")
			}
		})
	}
}

func newTestClient(t *testing.T, baseURL string) *Client {
	t.Helper()

	client, err := New(Config{
		BaseURL: baseURL,
		Token:   "test-token",
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	return client
}

func stringPtr(s string) *string {
	return &s
}
