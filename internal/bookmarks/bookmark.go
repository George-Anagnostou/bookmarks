// Package bookmarks defines bookmark data and URL normalization rules.
package bookmarks

import "time"

type Bookmark struct {
	ID            string    `json:"id"`
	URL           string    `json:"url"`
	NormalizedURL string    `json:"normalized_url"`
	Title         string    `json:"title,omitempty"`
	Notes         string    `json:"notes,omitempty"`
	Source        string    `json:"source,omitempty"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}

type CreateInput struct {
	URL    string `json:"url"`
	Title  string `json:"title,omitempty"`
	Notes  string `json:"notes,omitempty"`
	Source string `json:"source,omitempty"`
}

// UpdateInput describes a partial update. Nil fields are left unchanged;
// pointers to empty strings explicitly clear fields.
type UpdateInput struct {
	URL    *string `json:"url,omitempty"`
	Title  *string `json:"title,omitempty"`
	Notes  *string `json:"notes,omitempty"`
	Source *string `json:"source,omitempty"`
}

// ListOptions controls bookmark search and pagination. Query is a
// case-insensitive text search across URL, normalized URL, title, notes, and
// source. Non-positive limits are unbounded; non-positive offsets skip nothing.
type ListOptions struct {
	Query  string
	Limit  int
	Offset int
}
