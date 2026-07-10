package fetcher

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"

	"golang.org/x/net/html"
)

// Max Title bytes to read. Intended to limit reads for title extraction.
const maxTitleBytes = 32 * 1024

type Config struct {
	HTTPClient *http.Client
}

type Fetcher struct {
	httpClient *http.Client
}

func (f *Fetcher) FetchTitle(ctx context.Context, url string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", fmt.Errorf("create request: %w", err)
	}

	resp, err := f.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("bad status: %s", resp.Status)
	}

	contentType := resp.Header.Get("Content-Type")
	if contentType != "" && !strings.HasPrefix(contentType, "text/html") && !strings.HasPrefix(contentType, "application/xhtml+xml") {
		return "", fmt.Errorf("bad content type")
	}

	limited := io.LimitReader(resp.Body, maxTitleBytes)
	body, err := io.ReadAll(limited)
	if err != nil {
		return "", fmt.Errorf("read body: %w", err)
	}

	doc, err := html.Parse(bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("parse html: %w", err)
	}

	if t := findMetaOGTitle(doc); t != "" {
		return t, nil
	}

	if t := findTitleText(doc); t != "" {
		return t, nil
	}

	return "", nil
}

func NewFetcher(cfg Config) *Fetcher {
	httpClient := cfg.HTTPClient
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	return &Fetcher{httpClient: httpClient}
}

func findMetaOGTitle(n *html.Node) string {
	if n.Type == html.ElementNode && n.Data == "meta" {
		var isOG, content string
		for _, a := range n.Attr {
			switch a.Key {
			case "property":
				if a.Val == "og:title" {
					isOG = a.Val
				}
			case "content":
				content = a.Val
			}
		}
		if isOG != "" && content != "" {
			return content
		}
	}

	for c := n.FirstChild; c != nil; c = c.NextSibling {
		if got := findMetaOGTitle(c); got != "" {
			return got
		}
	}

	return ""
}

func findTitleText(n *html.Node) string {
	if n.Type == html.ElementNode && n.Data == "title" {
		if c := n.FirstChild; c != nil && c.Type == html.TextNode {
			return strings.TrimSpace(c.Data)
		}
	}

	for c := n.FirstChild; c != nil; c = c.NextSibling {
		if got := findTitleText(c); got != "" {
			return got
		}
	}

	return ""
}
