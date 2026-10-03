package server

import (
	"context"

	"bookmarks/internal/bookmarks"
)

// BookmarkStore provides the bookmark operations used by the HTTP server.
// The component that opens the store owns its lifecycle.
type BookmarkStore interface {
	CreateBookmark(ctx context.Context, input bookmarks.CreateInput) (bookmarks.Bookmark, bool, error)
	ListBookmarks(ctx context.Context, options bookmarks.ListOptions) ([]bookmarks.Bookmark, error)
	UpdateBookmark(ctx context.Context, id string, input bookmarks.UpdateInput) (bookmarks.Bookmark, error)
	DeleteBookmark(ctx context.Context, id string) error
	SetBookmarkTitleIfBlank(ctx context.Context, id, title string) (bool, error)
}
