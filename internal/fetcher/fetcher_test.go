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

func TestFetchTitleRespectsMaxBytesLimit(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
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
		t.Fatalf("title = %q, want empty when title after limit", title)
	}
}

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

func TestFetchTitleFallsBackWhenOGTitleIsWhitespace(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Write([]byte(`<!doctype html><html><head><meta property="og:title" content="   "><title>Fallback Title</title></head><body></body></html>`))
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
