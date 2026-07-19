package fetcher

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestFetchTitlePrefersOGTitle(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Write([]byte(`<!doctype html><html><head><meta property="og:title" content="OG Title"><title>HTML Title</title></head><body></body></html>`))
	}))
	defer s.Close()

	f := NewFetcher(Config{})
	title, err := f.FetchTitle(context.Background(), s.URL)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if title != "OG Title" {
		t.Fatalf("title = %q, want %q", title, "OG Title")
	}
}

func TestFetchTitleFallsBackToTitleTag(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Write([]byte(`<!doctype html><html><head><title>  Fallback Title  </title></head><body></body></html>`))
	}))
	defer s.Close()

	f := NewFetcher(Config{})
	title, err := f.FetchTitle(context.Background(), s.URL)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if title != "Fallback Title" {
		t.Fatalf("title = %q, want %q", title, "Fallback Title")
	}
}

func TestFetchTitleReturnsEmptyWhenNoTitle(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Write([]byte(`<!doctype html><html><head><meta name="description" content="desc"></head><body></body></html>`))
	}))
	defer s.Close()

	f := NewFetcher(Config{})
	title, err := f.FetchTitle(context.Background(), s.URL)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if title != "" {
		t.Fatalf("title = %q, want empty string", title)
	}
}

func TestFetchTitleReturnsErrorForUnreachable(t *testing.T) {
	f := NewFetcher(Config{})
	_, err := f.FetchTitle(context.Background(), "http://127.0.0.1:1")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestFetchTitleRespectsContextTimeout(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(2 * time.Second)
		w.Write([]byte(`<title>Too Late</title>`))
	}))
	defer s.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	f := NewFetcher(Config{})
	_, err := f.FetchTitle(ctx, s.URL)
	if err == nil {
		t.Fatal("expected context error, got nil")
	}
}

// --- Additional coverage for edge cases (not overly strict) ---

func TestFetchTitleTrimsWhitespaceInOGTitle(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Write([]byte(`<!doctype html><html><head><meta property="og:title" content="  Spaced OG Title  "></head><body></body></html>`))
	}))
	defer s.Close()

	f := NewFetcher(Config{})
	title, err := f.FetchTitle(context.Background(), s.URL)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if title != "Spaced OG Title" {
		t.Fatalf("title = %q, want trimmed %q", title, "Spaced OG Title")
	}
}

func TestFetchTitleExtractsTitleWithInlineMarkup(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Write([]byte(`<!doctype html><html><head><title>Hello <b>World</b></title></head><body></body></html>`))
	}))
	defer s.Close()

	f := NewFetcher(Config{})
	title, err := f.FetchTitle(context.Background(), s.URL)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if title != "Hello World" {
		t.Fatalf("title = %q, want %q", title, "Hello World")
	}
}

func TestFetchTitleFirstOGTitleWins(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Write([]byte(`<html><head><meta property="og:title" content="First OG"><meta property="og:title" content="Second OG"><title>HTML</title></head></html>`))
	}))
	defer s.Close()

	f := NewFetcher(Config{})
	title, err := f.FetchTitle(context.Background(), s.URL)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if title != "First OG" {
		t.Fatalf("title = %q, want first og:title", title)
	}
}

func TestFetchTitleCaseInsensitiveOGProperty(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Write([]byte(`<html><head><meta property="Og:Title" content="Case OG"></head></html>`))
	}))
	defer s.Close()

	f := NewFetcher(Config{})
	title, err := f.FetchTitle(context.Background(), s.URL)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if title != "Case OG" {
		t.Fatalf("title = %q, want %q (case-insensitive property match desired)", title, "Case OG")
	}
}

func TestFetchTitleOGViaNameAttribute(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Write([]byte(`<html><head><meta name="og:title" content="Name OG"></head></html>`))
	}))
	defer s.Close()

	f := NewFetcher(Config{})
	title, err := f.FetchTitle(context.Background(), s.URL)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if title != "Name OG" {
		t.Fatalf("title = %q, want %q (name=og:title fallback desired)", title, "Name OG")
	}
}

func TestFetchTitleEmptyOrWhitespaceTitleReturnsEmpty(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Write([]byte(`<html><head><title>   </title><meta property="og:title" content=""></head></html>`))
	}))
	defer s.Close()

	f := NewFetcher(Config{})
	title, err := f.FetchTitle(context.Background(), s.URL)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if title != "" {
		t.Fatalf("title = %q, want empty", title)
	}
}

func TestFetchTitleMalformedMetaNoContent(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Write([]byte(`<html><head><meta property="og:title"></head></html>`))
	}))
	defer s.Close()

	f := NewFetcher(Config{})
	title, err := f.FetchTitle(context.Background(), s.URL)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if title != "" {
		t.Fatalf("title = %q, want empty for missing content", title)
	}
}

func TestFetchTitleTitleOutsideHead(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Write([]byte(`<html><body><title>Body Title</title></body></html>`))
	}))
	defer s.Close()

	f := NewFetcher(Config{})
	title, err := f.FetchTitle(context.Background(), s.URL)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if title != "Body Title" {
		t.Fatalf("title = %q, want %q (title outside head)", title, "Body Title")
	}
}

func TestFetchTitleNon200ReturnsError(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusNotFound)
		w.Write([]byte(`<!doctype html><title>Not Found</title>`))
	}))
	defer s.Close()

	f := NewFetcher(Config{})
	_, err := f.FetchTitle(context.Background(), s.URL)
	if err == nil {
		t.Fatal("expected error for non-200, got nil")
	}
}

func TestFetchTitleAcceptsXHTMLContentType(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/xhtml+xml; charset=utf-8")
		w.Write([]byte(`<html xmlns="http://www.w3.org/1999/xhtml"><head><title>XHTML Title</title></head></html>`))
	}))
	defer s.Close()

	f := NewFetcher(Config{})
	title, err := f.FetchTitle(context.Background(), s.URL)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if title != "XHTML Title" {
		t.Fatalf("title = %q, want %q", title, "XHTML Title")
	}
}

func TestFetchTitleTitleAfterReadLimitIsIgnored(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		// ~40k of junk before the title
		junk := strings.Repeat("x", maxTitleBytes)
		w.Write([]byte(`<html><head>` + junk + `<title>Late Title</title></head></html>`))
	}))
	defer s.Close()

	f := NewFetcher(Config{})
	title, err := f.FetchTitle(context.Background(), s.URL)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if title != "" {
		t.Fatalf("title = %q, want empty (title after max bytes limit)", title)
	}
}

func TestFetchTitleFindsTitleWithinMaxBytes(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		junk := strings.Repeat("x", maxTitleBytes-100)
		w.Write([]byte(`<html><head>` + junk + `<title>Within Limit Title</title></head></html>`))
	}))
	defer s.Close()

	f := NewFetcher(Config{})
	title, err := f.FetchTitle(context.Background(), s.URL)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if title != "Within Limit Title" {
		t.Fatalf("title = %q, want title within limit", title)
	}
}

func TestFetchTitleIgnoresTitleBeyondMaxBytes(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		junk := strings.Repeat("x", maxTitleBytes)
		w.Write([]byte(`<html><head>` + junk + `<title>Beyond Limit Title</title></head></html>`))
	}))
	defer s.Close()

	f := NewFetcher(Config{})
	title, err := f.FetchTitle(context.Background(), s.URL)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if title != "" {
		t.Fatalf("title = %q, want empty for title beyond max bytes limit", title)
	}
}

func TestFetchTitleGetsPrefixOfVeryLongTitleText(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		long := "StartOfTitle" + strings.Repeat("X", maxTitleBytes*2)
		w.Write([]byte(`<html><head><title>` + long + `</title></head></html>`))
	}))
	defer s.Close()

	f := NewFetcher(Config{})
	title, err := f.FetchTitle(context.Background(), s.URL)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.HasPrefix(title, "StartOfTitle") || len(title) == 0 || len(title) > maxTitleBytes {
		t.Fatalf("title = %q (len=%d), want prefix of long title truncated by read limit", title, len(title))
	}
}
