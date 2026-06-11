package server

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
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
			method:     http.MethodGet,
			wantStatus: http.StatusMethodNotAllowed,
		},
		{
			name:       "bad json",
			method:     http.MethodPost,
			body:       `{"url":`,
			wantStatus: http.StatusBadRequest,
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
		})
	}
}

type fakeStore struct {
	createBookmark func(context.Context, bookmarks.CreateInput) (bookmarks.Bookmark, bool, error)
	listBookmarks  func(context.Context) ([]bookmarks.Bookmark, error)
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
