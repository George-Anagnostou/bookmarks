package server

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"time"

	"bookmarks/internal/bookmarks"
)

func TestCreateBookmark(t *testing.T) {
	now := time.Date(2026, 6, 10, 12, 0, 0, 0, time.UTC)
	bookmark := bookmarks.Bookmark{
		ID:            "bookmark-1",
		URL:           "https://example.com/a",
		NormalizedURL: "https://example.com/a",
		Title:         "Example",
		Notes:         "Read later",
		Source:        "test",
		CreatedAt:     now,
		UpdatedAt:     now,
	}

	store := &fakeStore{
		createBookmark: func(ctx context.Context, input bookmarks.CreateInput) (bookmarks.Bookmark, bool, error) {
			if input.URL != "https://example.com/a" {
				t.Fatalf("input.URL = %q, want %q", input.URL, "https://example.com/a")
			}
			if input.Title != "Example" {
				t.Fatalf("input.Title = %q, want %q", input.Title, "Example")
			}
			if input.Notes != "Read later" {
				t.Fatalf("input.Notes = %q, want %q", input.Notes, "Read later")
			}
			if input.Source != "test" {
				t.Fatalf("input.Source = %q, want %q", input.Source, "test")
			}
			return bookmark, true, nil
		},
	}

	handler := New(Config{Store: store, Token: "test-token"}).Handler()
	req := newJSONRequest(t, http.MethodPost, "/api/bookmarks", map[string]string{
		"url":    "https://example.com/a",
		"title":  "Example",
		"notes":  "Read later",
		"source": "test",
	})
	req.Header.Set("Authorization", "Bearer test-token")

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d; body = %s", rec.Code, http.StatusCreated, rec.Body.String())
	}
	assertJSONContentType(t, rec)

	var got createBookmarkResponse
	if err := json.NewDecoder(rec.Body).Decode(&got); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if !got.Created {
		t.Fatal("created = false, want true")
	}
	if got.Bookmark.ID != bookmark.ID {
		t.Fatalf("bookmark ID = %q, want %q", got.Bookmark.ID, bookmark.ID)
	}
}

func TestCreateBookmarkDuplicate(t *testing.T) {
	store := &fakeStore{
		createBookmark: func(ctx context.Context, input bookmarks.CreateInput) (bookmarks.Bookmark, bool, error) {
			return bookmarks.Bookmark{
				ID:            "bookmark-1",
				URL:           "https://example.com/a",
				NormalizedURL: "https://example.com/a",
			}, false, nil
		},
	}

	handler := New(Config{Store: store, Token: "test-token"}).Handler()
	req := newJSONRequest(t, http.MethodPost, "/api/bookmarks", map[string]string{
		"url": "https://example.com/a",
	})
	req.Header.Set("Authorization", "Bearer test-token")

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}
	assertJSONContentType(t, rec)

	var got createBookmarkResponse
	if err := json.NewDecoder(rec.Body).Decode(&got); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if got.Created {
		t.Fatal("created = true, want false")
	}
}

func TestCreateBookmarkRequiresBearerToken(t *testing.T) {
	handler := New(Config{Store: &fakeStore{}, Token: "test-token"}).Handler()

	tests := []struct {
		name          string
		authorization string
	}{
		{
			name: "missing",
		},
		{
			name:          "wrong token",
			authorization: "Bearer wrong-token",
		},
		{
			name:          "wrong scheme",
			authorization: "Basic test-token",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := newJSONRequest(t, http.MethodPost, "/api/bookmarks", map[string]string{
				"url": "https://example.com/a",
			})
			if tt.authorization != "" {
				req.Header.Set("Authorization", tt.authorization)
			}

			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, req)

			if rec.Code != http.StatusUnauthorized {
				t.Fatalf("status = %d, want %d", rec.Code, http.StatusUnauthorized)
			}
		})
	}
}

func TestCreateBookmarkRejectsBadRequests(t *testing.T) {
	tests := []struct {
		name       string
		method     string
		body       string
		storeError error
		wantStatus int
	}{
		{
			name:       "wrong method",
			method:     http.MethodPut,
			wantStatus: http.StatusMethodNotAllowed,
		},
		{
			name:       "bad json",
			method:     http.MethodPost,
			body:       `{"url":`,
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "trailing json",
			method:     http.MethodPost,
			body:       `{"url":"https://example.com/a"} {}`,
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "oversized body",
			method:     http.MethodPost,
			body:       `{"url":"https://example.com/a","notes":"` + strings.Repeat("x", maxBookmarkBodyBytes) + `"}`,
			wantStatus: http.StatusRequestEntityTooLarge,
		},
		{
			name:       "empty url",
			method:     http.MethodPost,
			body:       `{}`,
			storeError: bookmarks.ErrEmptyURL,
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "unsupported url",
			method:     http.MethodPost,
			body:       `{"url":"ftp://example.com/file"}`,
			storeError: bookmarks.ErrUnsupported,
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "store failure",
			method:     http.MethodPost,
			body:       `{"url":"https://example.com/a"}`,
			storeError: errors.New("database unavailable"),
			wantStatus: http.StatusInternalServerError,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := &fakeStore{
				createBookmark: func(ctx context.Context, input bookmarks.CreateInput) (bookmarks.Bookmark, bool, error) {
					if tt.storeError != nil {
						return bookmarks.Bookmark{}, false, tt.storeError
					}
					return bookmarks.Bookmark{}, true, nil
				},
			}
			handler := New(Config{Store: store, Token: "test-token"}).Handler()

			req := httptest.NewRequest(tt.method, "/api/bookmarks", bytes.NewBufferString(tt.body))
			req.Header.Set("Authorization", "Bearer test-token")
			req.Header.Set("Content-Type", "application/json")

			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, req)

			if rec.Code != tt.wantStatus {
				t.Fatalf("status = %d, want %d; body = %s", rec.Code, tt.wantStatus, rec.Body.String())
			}
			if rec.Code != http.StatusMethodNotAllowed {
				assertJSONContentType(t, rec)
			}
		})
	}
}

func TestListBookmarksJSON(t *testing.T) {
	now := time.Date(2026, 6, 11, 9, 30, 0, 0, time.UTC)
	want := []bookmarks.Bookmark{
		{
			ID:            "bookmark-2",
			URL:           "https://example.com/b",
			NormalizedURL: "https://example.com/b",
			Title:         "Second",
			Notes:         "Later",
			Source:        "mac",
			CreatedAt:     now,
			UpdatedAt:     now,
		},
		{
			ID:            "bookmark-1",
			URL:           "https://example.com/a",
			NormalizedURL: "https://example.com/a",
			Title:         "First",
			Source:        "ios",
			CreatedAt:     now.Add(-time.Hour),
			UpdatedAt:     now.Add(-time.Hour),
		},
	}

	store := &fakeStore{
		listBookmarks: func(ctx context.Context) ([]bookmarks.Bookmark, error) {
			return want, nil
		},
	}
	handler := New(Config{Store: store, Token: "test-token"}).Handler()
	req := httptest.NewRequest(http.MethodGet, "/api/bookmarks", nil)
	req.Header.Set("Authorization", "Bearer test-token")

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}
	assertJSONContentType(t, rec)

	var got listBookmarksResponse
	if err := json.NewDecoder(rec.Body).Decode(&got); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if !reflect.DeepEqual(got.Bookmarks, want) {
		t.Fatalf("bookmarks = %#v, want %#v", got.Bookmarks, want)
	}
}

func TestListBookmarksJSONReturnsEmptyArray(t *testing.T) {
	store := &fakeStore{
		listBookmarks: func(ctx context.Context) ([]bookmarks.Bookmark, error) {
			return nil, nil
		},
	}
	handler := New(Config{Store: store, Token: "test-token"}).Handler()
	req := httptest.NewRequest(http.MethodGet, "/api/bookmarks", nil)
	req.Header.Set("Authorization", "Bearer test-token")

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}
	assertJSONContentType(t, rec)

	var got listBookmarksResponse
	if err := json.NewDecoder(rec.Body).Decode(&got); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if got.Bookmarks == nil {
		t.Fatal("bookmarks = nil, want empty array")
	}
	if len(got.Bookmarks) != 0 {
		t.Fatalf("len(bookmarks) = %d, want 0", len(got.Bookmarks))
	}
}

func TestListBookmarksJSONRequiresBearerToken(t *testing.T) {
	handler := New(Config{Store: &fakeStore{}, Token: "test-token"}).Handler()

	tests := []struct {
		name          string
		authorization string
	}{
		{name: "missing"},
		{name: "wrong token", authorization: "Bearer invalid-test-token"},
		{name: "wrong scheme", authorization: "Basic test-token"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/api/bookmarks", nil)
			if tt.authorization != "" {
				req.Header.Set("Authorization", tt.authorization)
			}

			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, req)

			if rec.Code != http.StatusUnauthorized {
				t.Fatalf("status = %d, want %d", rec.Code, http.StatusUnauthorized)
			}
			assertJSONContentType(t, rec)
		})
	}
}

func TestListBookmarksJSONHandlesStoreError(t *testing.T) {
	store := &fakeStore{
		listBookmarks: func(ctx context.Context) ([]bookmarks.Bookmark, error) {
			return nil, errors.New("database unavailable")
		},
	}
	handler := New(Config{Store: store, Token: "test-token"}).Handler()
	req := httptest.NewRequest(http.MethodGet, "/api/bookmarks", nil)
	req.Header.Set("Authorization", "Bearer test-token")

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d; body = %s", rec.Code, http.StatusInternalServerError, rec.Body.String())
	}
	assertJSONContentType(t, rec)
}

func TestUpdateBookmark(t *testing.T) {
	now := time.Date(2026, 6, 24, 10, 0, 0, 0, time.UTC)
	bookmark := bookmarks.Bookmark{
		ID:            "bookmark-1",
		URL:           "https://example.com/new",
		NormalizedURL: "https://example.com/new",
		Title:         "Updated",
		Notes:         "",
		Source:        "bookmarkctl",
		CreatedAt:     now.Add(-time.Hour),
		UpdatedAt:     now,
	}

	store := &fakeStore{
		updateBookmark: func(ctx context.Context, id string, input bookmarks.UpdateInput) (bookmarks.Bookmark, error) {
			if id != "bookmark-1" {
				t.Fatalf("id = %q, want %q", id, "bookmark-1")
			}
			if input.Title == nil || *input.Title != "Updated" {
				t.Fatalf("Title = %#v, want Updated", input.Title)
			}
			if input.Notes == nil || *input.Notes != "" {
				t.Fatalf("Notes = %#v, want empty string pointer", input.Notes)
			}
			if input.Source == nil || *input.Source != "bookmarkctl" {
				t.Fatalf("Source = %#v, want bookmarkctl", input.Source)
			}
			if input.URL == nil || *input.URL != "https://example.com/new" {
				t.Fatalf("URL = %#v, want https://example.com/new", input.URL)
			}
			return bookmark, nil
		},
	}

	handler := New(Config{Store: store, Token: "test-token"}).Handler()
	req := newJSONRequest(t, http.MethodPatch, "/api/bookmarks/bookmark-1", map[string]string{
		"url":    "https://example.com/new",
		"title":  "Updated",
		"notes":  "",
		"source": "bookmarkctl",
	})
	req.Header.Set("Authorization", "Bearer test-token")

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}
	assertJSONContentType(t, rec)

	var got struct {
		Bookmark bookmarks.Bookmark `json:"bookmark"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&got); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if !reflect.DeepEqual(got.Bookmark, bookmark) {
		t.Fatalf("bookmark = %#v, want %#v", got.Bookmark, bookmark)
	}
}

func TestUpdateBookmarkRequiresBearerToken(t *testing.T) {
	handler := New(Config{Store: &fakeStore{}, Token: "test-token"}).Handler()

	tests := []struct {
		name          string
		authorization string
	}{
		{name: "missing"},
		{name: "wrong token", authorization: "Bearer wrong-token"},
		{name: "wrong scheme", authorization: "Basic test-token"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := newJSONRequest(t, http.MethodPatch, "/api/bookmarks/bookmark-1", map[string]string{
				"title": "Updated",
			})
			if tt.authorization != "" {
				req.Header.Set("Authorization", tt.authorization)
			}

			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, req)

			if rec.Code != http.StatusUnauthorized {
				t.Fatalf("status = %d, want %d; body = %s", rec.Code, http.StatusUnauthorized, rec.Body.String())
			}
			assertJSONContentType(t, rec)
		})
	}
}

func TestUpdateBookmarkRejectsBadRequests(t *testing.T) {
	tests := []struct {
		name       string
		method     string
		body       string
		storeError error
		wantStatus int
	}{
		{
			name:       "wrong method",
			method:     http.MethodPut,
			body:       `{"title":"Updated"}`,
			wantStatus: http.StatusMethodNotAllowed,
		},
		{
			name:       "bad json",
			method:     http.MethodPatch,
			body:       `{"title":`,
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "trailing json",
			method:     http.MethodPatch,
			body:       `{"title":"Updated"} {}`,
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "oversized body",
			method:     http.MethodPatch,
			body:       `{"notes":"` + strings.Repeat("x", maxBookmarkBodyBytes) + `"}`,
			wantStatus: http.StatusRequestEntityTooLarge,
		},
		{
			name:       "empty update",
			method:     http.MethodPatch,
			body:       `{}`,
			storeError: bookmarks.ErrNoUpdateFields,
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "empty url",
			method:     http.MethodPatch,
			body:       `{"url":""}`,
			storeError: bookmarks.ErrEmptyURL,
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "unsupported url",
			method:     http.MethodPatch,
			body:       `{"url":"ftp://example.com/file"}`,
			storeError: bookmarks.ErrUnsupported,
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "missing host",
			method:     http.MethodPatch,
			body:       `{"url":"https:///path"}`,
			storeError: bookmarks.ErrMissingHost,
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "url user info",
			method:     http.MethodPatch,
			body:       `{"url":"https://user@example.com/a"}`,
			storeError: bookmarks.ErrURLUserInfo,
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "not found",
			method:     http.MethodPatch,
			body:       `{"title":"Updated"}`,
			storeError: bookmarks.ErrNotFound,
			wantStatus: http.StatusNotFound,
		},
		{
			name:       "duplicate url",
			method:     http.MethodPatch,
			body:       `{"url":"https://example.com/a"}`,
			storeError: bookmarks.ErrDuplicateURL,
			wantStatus: http.StatusConflict,
		},
		{
			name:       "store failure",
			method:     http.MethodPatch,
			body:       `{"title":"Updated"}`,
			storeError: errors.New("database unavailable"),
			wantStatus: http.StatusInternalServerError,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := &fakeStore{}
			if tt.storeError != nil {
				store.updateBookmark = func(ctx context.Context, id string, input bookmarks.UpdateInput) (bookmarks.Bookmark, error) {
					if id != "bookmark-1" {
						t.Fatalf("id = %q, want %q", id, "bookmark-1")
					}
					return bookmarks.Bookmark{}, tt.storeError
				}
			}

			handler := New(Config{Store: store, Token: "test-token"}).Handler()
			req := httptest.NewRequest(tt.method, "/api/bookmarks/bookmark-1", bytes.NewBufferString(tt.body))
			req.Header.Set("Authorization", "Bearer test-token")
			req.Header.Set("Content-Type", "application/json")

			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, req)

			if rec.Code != tt.wantStatus {
				t.Fatalf("status = %d, want %d; body = %s", rec.Code, tt.wantStatus, rec.Body.String())
			}
			if rec.Code != http.StatusMethodNotAllowed {
				assertJSONContentType(t, rec)
			}
		})
	}
}

func TestDeleteBookmark(t *testing.T) {
	store := &fakeStore{
		deleteBookmark: func(ctx context.Context, id string) error {
			if id != "bookmark-1" {
				t.Fatalf("id = %q, want %q", id, "bookmark-1")
			}
			return nil
		},
	}

	handler := New(Config{Store: store, Token: "test-token"}).Handler()
	req := httptest.NewRequest(http.MethodDelete, "/api/bookmarks/bookmark-1", nil)
	req.Header.Set("Authorization", "Bearer test-token")

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d; body = %s", rec.Code, http.StatusNoContent, rec.Body.String())
	}
	if rec.Body.Len() != 0 {
		t.Fatalf("body = %q, want empty", rec.Body.String())
	}
	if got := rec.Header().Get("Content-Type"); got != "" {
		t.Fatalf("Content-Type = %q, want empty", got)
	}
}

func TestDeleteBookmarkRequiresBearerToken(t *testing.T) {
	handler := New(Config{Store: &fakeStore{}, Token: "test-token"}).Handler()

	tests := []struct {
		name          string
		authorization string
	}{
		{name: "missing"},
		{name: "wrong token", authorization: "Bearer wrong-token"},
		{name: "wrong scheme", authorization: "Basic test-token"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodDelete, "/api/bookmarks/bookmark-1", nil)
			if tt.authorization != "" {
				req.Header.Set("Authorization", tt.authorization)
			}

			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, req)

			if rec.Code != http.StatusUnauthorized {
				t.Fatalf("status = %d, want %d; body = %s", rec.Code, http.StatusUnauthorized, rec.Body.String())
			}
			assertJSONContentType(t, rec)
		})
	}
}

func TestDeleteBookmarkHandlesStoreErrors(t *testing.T) {
	tests := []struct {
		name       string
		storeError error
		wantStatus int
	}{
		{
			name:       "not found",
			storeError: bookmarks.ErrNotFound,
			wantStatus: http.StatusNotFound,
		},
		{
			name:       "store failure",
			storeError: errors.New("database unavailable"),
			wantStatus: http.StatusInternalServerError,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := &fakeStore{
				deleteBookmark: func(ctx context.Context, id string) error {
					if id != "bookmark-1" {
						t.Fatalf("id = %q, want %q", id, "bookmark-1")
					}
					return tt.storeError
				},
			}

			handler := New(Config{Store: store, Token: "test-token"}).Handler()
			req := httptest.NewRequest(http.MethodDelete, "/api/bookmarks/bookmark-1", nil)
			req.Header.Set("Authorization", "Bearer test-token")

			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, req)

			if rec.Code != tt.wantStatus {
				t.Fatalf("status = %d, want %d; body = %s", rec.Code, tt.wantStatus, rec.Body.String())
			}
			assertJSONContentType(t, rec)
		})
	}
}

func TestHealthz(t *testing.T) {
	handler := New(Config{Store: &fakeStore{}, Token: "test-token"}).Handler()
	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}
	assertJSONContentType(t, rec)

	var got map[string]string
	if err := json.NewDecoder(rec.Body).Decode(&got); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if got["status"] != "ok" {
		t.Fatalf("status field = %q, want ok", got["status"])
	}
	if len(got) != 1 {
		t.Fatalf("response fields = %#v, want only status", got)
	}
}

func TestHealthzDoesNotRequireBearerToken(t *testing.T) {
	handler := New(Config{Store: &fakeStore{}, Token: "test-token"}).Handler()
	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}
}

type fakeStore struct {
	createBookmark func(context.Context, bookmarks.CreateInput) (bookmarks.Bookmark, bool, error)
	listBookmarks  func(context.Context) ([]bookmarks.Bookmark, error)
	updateBookmark func(context.Context, string, bookmarks.UpdateInput) (bookmarks.Bookmark, error)
	deleteBookmark func(context.Context, string) error
}

func (s *fakeStore) CreateBookmark(ctx context.Context, input bookmarks.CreateInput) (bookmarks.Bookmark, bool, error) {
	if s.createBookmark == nil {
		panic("unexpected CreateBookmark call")
	}
	return s.createBookmark(ctx, input)
}

func (s *fakeStore) ListBookmarks(ctx context.Context) ([]bookmarks.Bookmark, error) {
	if s.listBookmarks == nil {
		panic("unexpected ListBookmarks call")
	}
	return s.listBookmarks(ctx)
}

func (s *fakeStore) UpdateBookmark(ctx context.Context, id string, input bookmarks.UpdateInput) (bookmarks.Bookmark, error) {
	if s.updateBookmark == nil {
		panic("unexpected UpdateBookmark call")
	}
	return s.updateBookmark(ctx, id, input)
}

func (s *fakeStore) DeleteBookmark(ctx context.Context, id string) error {
	if s.deleteBookmark == nil {
		panic("unexpected DeleteBookmark call")
	}
	return s.deleteBookmark(ctx, id)
}

func newJSONRequest(t *testing.T, method string, path string, body any) *http.Request {
	t.Helper()

	var buf bytes.Buffer
	if err := json.NewEncoder(&buf).Encode(body); err != nil {
		t.Fatalf("encode request: %v", err)
	}

	req := httptest.NewRequest(method, path, &buf)
	req.Header.Set("Content-Type", "application/json")
	return req
}

func assertJSONContentType(t *testing.T, rec *httptest.ResponseRecorder) {
	t.Helper()

	if got := rec.Header().Get("Content-Type"); got != "application/json" {
		t.Fatalf("Content-Type = %q, want application/json", got)
	}
}
