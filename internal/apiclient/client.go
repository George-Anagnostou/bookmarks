package apiclient

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"bookmarks/internal/bookmarks"
)

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
	var body bytes.Buffer
	if err := json.NewEncoder(&body).Encode(input); err != nil {
		return bookmarks.Bookmark{}, false, fmt.Errorf("encode request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/api/bookmarks", &body)
	if err != nil {
		return bookmarks.Bookmark{}, false, fmt.Errorf("create request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return bookmarks.Bookmark{}, false, fmt.Errorf("send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return bookmarks.Bookmark{}, false, fmt.Errorf("bookmarks api: %s", resp.Status)
	}

	var out struct {
		Bookmark bookmarks.Bookmark `json:"bookmark"`
		Created  bool               `json:"created"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return bookmarks.Bookmark{}, false, fmt.Errorf("decode response: %w", err)
	}

	return out.Bookmark, out.Created, nil
}

func (c *Client) ListBookmarks(ctx context.Context) ([]bookmarks.Bookmark, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/api/bookmarks", nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+c.token)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("bookmarks api: %s", resp.Status)
	}

	var out struct {
		Bookmarks []bookmarks.Bookmark `json:"bookmarks"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}

	if out.Bookmarks == nil {
		out.Bookmarks = []bookmarks.Bookmark{}
	}

	return out.Bookmarks, nil
}
