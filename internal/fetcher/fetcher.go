package fetcher

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// Max Title bytes to read. Intended to limit reads for title extraction.
const maxTitleBytes = 32 * 1024

type Config struct {
	HttpClient *http.Client
}

type Fetcher struct {
	HttpClient *http.Client
}

func (f *Fetcher) FetchTitle(ctx context.Context, url string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", fmt.Errorf("fetch title failed to send request: %w", err)
	}

	resp, err := f.HttpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("fetch title failed to get response: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("fetch title response failed: returned %s", resp.Status)
	}

	contentType := resp.Header.Get("Content-Type")
	if !strings.HasPrefix(contentType, "text/html") {
		return "", fmt.Errorf("fetch title: unknown content type")
	}

	limited := io.LimitReader(resp.Body, maxTitleBytes)
	body, err := io.ReadAll(limited)
	if err != nil {
		return "", fmt.Errorf("fetch title failed to read limited response: %w", err)
	}

	return string(body), nil
}

func NewFetcher(cfg Config) *Fetcher {
	httpClient := cfg.HttpClient
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	return &Fetcher{HttpClient: httpClient}
}
