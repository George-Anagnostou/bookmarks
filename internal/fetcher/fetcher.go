package fetcher

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"mime"
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
	if contentType != "" {
		mediaType, _, err := mime.ParseMediaType(contentType)
		if err != nil {
			return "", fmt.Errorf("parse content type: %w", err)
		}
		if mediaType != "text/html" && mediaType != "application/xhtml+xml" {
			return "", fmt.Errorf("unsupported content type: %s", mediaType)
		}
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

	if t := findMetaOGTitle(doc); strings.TrimSpace(t) != "" {
		return strings.TrimSpace(t), nil
	}

	if t := findTitleText(doc); strings.TrimSpace(t) != "" {
		return strings.TrimSpace(t), nil
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
		seenOG := false
		var content string
		for _, a := range n.Attr {
			switch a.Key {
			case "property", "name":
				if strings.ToLower(a.Val) == "og:title" {
					seenOG = true
				}
			case "content":
				content = a.Val
			}
		}
		if seenOG && content != "" {
			return content
		}
	}

	for c := n.FirstChild; c != nil; c = c.NextSibling {
		if title := findMetaOGTitle(c); title != "" {
			return title
		}
	}

	return ""
}

func findTitleText(n *html.Node) string {
	if n.Type == html.ElementNode && n.Data == "title" {
		if c := n.FirstChild; c != nil && c.Type == html.TextNode {
			return n.FirstChild.Data
		}
		return ""
	}

	for c := n.FirstChild; c != nil; c = c.NextSibling {
		if title := findTitleText(c); title != "" {
			return title
		}
	}

	return ""
}
