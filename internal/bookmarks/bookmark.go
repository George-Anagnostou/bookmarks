package bookmarks

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/url"
	"strings"
	"time"
)

var (
	ErrEmptyURL       = errors.New("url is required")
	ErrUnsupported    = errors.New("url must use http or https")
	ErrMissingHost    = errors.New("url host is required")
	ErrURLUserInfo    = errors.New("url must not include credentials")
	ErrNotFound       = errors.New("bookmark not found")
	ErrDuplicateURL   = errors.New("bookmark url already exists")
	ErrNoUpdateFields = errors.New("bookmark edit must update at least one field")
)

type Bookmark struct {
	ID            string    `json:"id"`
	URL           string    `json:"url"`
	NormalizedURL string    `json:"normalized_url"`
	Title         string    `json:"title,omitempty"`
	Tags          []string  `json:"tags,omitempty"`
	Notes         string    `json:"notes,omitempty"`
	Source        string    `json:"source,omitempty"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}

type CreateInput struct {
	URL    string   `json:"url"`
	Title  string   `json:"title,omitempty"`
	Tags   []string `json:"tags,omitempty"`
	Notes  string   `json:"notes,omitempty"`
	Source string   `json:"source,omitempty"`
}

type UpdateInput struct {
	URL    *string `json:"url,omitempty"`
	Title  *string `json:"title,omitempty"`
	Notes  *string `json:"notes,omitempty"`
	Source *string `json:"source,omitempty"`
}

type Store interface {
	CreateBookmark(ctx context.Context, input CreateInput) (Bookmark, bool, error)
	ListBookmarks(ctx context.Context) ([]Bookmark, error)
	UpdateBookmark(ctx context.Context, id string, input UpdateInput) (Bookmark, error)
	DeleteBookmark(ctx context.Context, id string) error
}

func NormalizeURL(raw string) (string, error) {
	s := strings.TrimSpace(raw)
	if s == "" {
		return "", ErrEmptyURL
	}

	if !strings.Contains(s, "://") {
		s = "https://" + s
	}

	u, err := url.Parse(s)
	if err != nil {
		return "", fmt.Errorf("parse url: %w", err)
	}

	u.Scheme = strings.ToLower(u.Scheme)
	if u.Scheme != "http" && u.Scheme != "https" {
		return "", ErrUnsupported
	}
	if u.User != nil {
		return "", ErrURLUserInfo
	}
	if u.Host == "" {
		return "", ErrMissingHost
	}

	host := strings.ToLower(u.Hostname())
	port := u.Port()
	switch {
	case port == "":
		u.Host = host
	case (u.Scheme == "http" && port == "80") || (u.Scheme == "https" && port == "443"):
		u.Host = host
	default:
		u.Host = net.JoinHostPort(host, port)
	}

	if u.Path == "" {
		u.Path = "/"
	}

	return u.String(), nil
}
