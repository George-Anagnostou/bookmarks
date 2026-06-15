package apiclient

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"bookmarks/internal/bookmarks"
)

var ErrNotImplemented = errors.New("not implemented")

type Config struct {
	BaseURL    string
	Token      string
	HTTPClient *http.Client
}

type Client struct {
	baseURL    string
	token      string
	httpClient *http.Client
}

func New(cfg Config) (*Client, error) {
	baseURL := strings.TrimSpace(cfg.BaseURL)
	if baseURL == "" {
		return nil, errors.New("base url is required")
	}

	token := strings.TrimSpace(cfg.Token)
	if token == "" {
		return nil, errors.New("token is required")
	}

	u, err := url.Parse(baseURL)
	if err != nil {
		return nil, fmt.Errorf("parse base url: %w", err)
	}

	if u.Scheme != "http" && u.Scheme != "https" {
		return nil, errors.New("base url must use http or https")
	}

	if u.Host == "" {
		return nil, errors.New("base url host is required")
	}

	baseURL = strings.TrimRight(baseURL, "/")

	httpClient := cfg.HTTPClient
	if httpClient == nil {
		httpClient = http.DefaultClient
	}

	return &Client{
		baseURL:    baseURL,
		token:      token,
		httpClient: httpClient,
	}, nil
}

func (c *Client) CreateBookmark(ctx context.Context, input bookmarks.CreateInput) (bookmarks.Bookmark, bool, error) {
	return bookmarks.Bookmark{}, false, ErrNotImplemented
}

func (c *Client) ListBookmarks(ctx context.Context) ([]bookmarks.Bookmark, error) {
	return nil, ErrNotImplemented
}
