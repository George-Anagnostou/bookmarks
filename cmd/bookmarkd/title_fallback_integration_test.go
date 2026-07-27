package main

import (
	"context"
	"io"
	"log"
	"net"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"bookmarks/internal/apiclient"
	"bookmarks/internal/bookmarks"
)

func TestBookmarkdFetchesTitleAfterCreatingBookmark(t *testing.T) {
	fetchStarted := make(chan struct{})
	releaseFetch := make(chan struct{})
	var startedOnce sync.Once
	var releaseOnce sync.Once
	release := func() { releaseOnce.Do(func() { close(releaseFetch) }) }

	page := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		startedOnce.Do(func() { close(fetchStarted) })
		<-releaseFetch
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(`<meta property="og:title" content="Fetched title"><title>HTML title</title>`))
	}))
	defer func() {
		release()
		page.Close()
	}()

	client := startBookmarkdForTitleFallbackTest(t)

	type createResult struct {
		bookmark bookmarks.Bookmark
		created  bool
		err      error
	}
	resultc := make(chan createResult, 1)
	go func() {
		bookmark, created, err := client.CreateBookmark(context.Background(), bookmarks.CreateInput{URL: page.URL})
		resultc <- createResult{bookmark: bookmark, created: created, err: err}
	}()

	var result createResult
	select {
	case result = <-resultc:
	case <-time.After(time.Second):
		t.Fatal("creating a bookmark waited for title fetching")
	}
	if result.err != nil {
		t.Fatalf("CreateBookmark() error = %v", result.err)
	}
	if !result.created {
		t.Fatal("CreateBookmark() created = false, want true")
	}

	select {
	case <-fetchStarted:
	case <-time.After(time.Second):
		t.Fatal("title fetch did not start")
	}
	release()

	waitForBookmarkTitle(t, client, result.bookmark.ID, "Fetched title")
}

func TestBookmarkdFetchesTitleForWhitespaceInput(t *testing.T) {
	page := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(`<title>Fallback HTML title</title>`))
	}))
	defer page.Close()

	client := startBookmarkdForTitleFallbackTest(t)
	bookmark, created, err := client.CreateBookmark(context.Background(), bookmarks.CreateInput{
		URL:   page.URL,
		Title: " \t ",
	})
	if err != nil {
		t.Fatalf("CreateBookmark() error = %v", err)
	}
	if !created {
		t.Fatal("CreateBookmark() created = false, want true")
	}

	waitForBookmarkTitle(t, client, bookmark.ID, "Fallback HTML title")
}

func TestBookmarkdDoesNotFetchTitleWhenProvided(t *testing.T) {
	fetchStarted := make(chan struct{})
	var startedOnce sync.Once
	page := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		startedOnce.Do(func() { close(fetchStarted) })
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(`<title>Fetched title</title>`))
	}))
	defer page.Close()

	client := startBookmarkdForTitleFallbackTest(t)
	bookmark, created, err := client.CreateBookmark(context.Background(), bookmarks.CreateInput{
		URL:   page.URL,
		Title: "Written by the user",
	})
	if err != nil {
		t.Fatalf("CreateBookmark() error = %v", err)
	}
	if !created {
		t.Fatal("CreateBookmark() created = false, want true")
	}
	if bookmark.Title != "Written by the user" {
		t.Fatalf("created bookmark title = %q, want user title", bookmark.Title)
	}

	select {
	case <-fetchStarted:
		t.Fatal("title fetch started despite a user-provided title")
	case <-time.After(100 * time.Millisecond):
	}
}

func TestBookmarkdDoesNotFetchTitleForDuplicateBookmark(t *testing.T) {
	fetchStarted := make(chan struct{})
	var startedOnce sync.Once
	page := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		startedOnce.Do(func() { close(fetchStarted) })
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(`<title>Fetched title</title>`))
	}))
	defer page.Close()

	client := startBookmarkdForTitleFallbackTest(t)
	first, created, err := client.CreateBookmark(context.Background(), bookmarks.CreateInput{
		URL:   page.URL,
		Title: "Original title",
	})
	if err != nil {
		t.Fatalf("first CreateBookmark() error = %v", err)
	}
	if !created {
		t.Fatal("first CreateBookmark() created = false, want true")
	}

	second, created, err := client.CreateBookmark(context.Background(), bookmarks.CreateInput{URL: page.URL})
	if err != nil {
		t.Fatalf("second CreateBookmark() error = %v", err)
	}
	if created {
		t.Fatal("second CreateBookmark() created = true, want false")
	}
	if second.ID != first.ID {
		t.Fatalf("duplicate bookmark ID = %q, want %q", second.ID, first.ID)
	}
	if second.Title != "Original title" {
		t.Fatalf("duplicate bookmark title = %q, want original title", second.Title)
	}

	select {
	case <-fetchStarted:
		t.Fatal("title fetch started for a duplicate bookmark")
	case <-time.After(100 * time.Millisecond):
	}
}

func TestBookmarkdFetchedTitleDoesNotOverwriteManualEdit(t *testing.T) {
	fetchStarted := make(chan struct{})
	releaseFetch := make(chan struct{})
	var startedOnce sync.Once
	var releaseOnce sync.Once
	release := func() { releaseOnce.Do(func() { close(releaseFetch) }) }

	page := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		startedOnce.Do(func() { close(fetchStarted) })
		<-releaseFetch
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(`<title>Fetched title</title>`))
	}))
	defer func() {
		release()
		page.Close()
	}()

	client := startBookmarkdForTitleFallbackTest(t)
	bookmark, created, err := client.CreateBookmark(context.Background(), bookmarks.CreateInput{URL: page.URL})
	if err != nil {
		t.Fatalf("CreateBookmark() error = %v", err)
	}
	if !created {
		t.Fatal("CreateBookmark() created = false, want true")
	}

	select {
	case <-fetchStarted:
	case <-time.After(time.Second):
		t.Fatal("title fetch did not start")
	}

	manualTitle := "Written by the user"
	if _, err := client.UpdateBookmark(context.Background(), bookmark.ID, bookmarks.UpdateInput{Title: &manualTitle}); err != nil {
		t.Fatalf("UpdateBookmark() error = %v", err)
	}
	release()

	assertBookmarkTitleRemains(t, client, bookmark.ID, manualTitle)
}

func startBookmarkdForTitleFallbackTest(t *testing.T) *apiclient.Client {
	t.Helper()

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("reserve local address: %v", err)
	}
	addr := listener.Addr().String()
	if err := listener.Close(); err != nil {
		t.Fatalf("release local address: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	errC := make(chan error, 1)
	go func() {
		errC <- run(ctx, config{
			Addr:   addr,
			DBPath: filepath.Join(t.TempDir(), "bookmarks.db"),
			Token:  "test-token",
		}, log.New(io.Discard, "", 0))
	}()

	t.Cleanup(func() {
		cancel()
		select {
		case err := <-errC:
			if err != nil {
				t.Errorf("bookmarkd stopped with error: %v", err)
			}
		case <-time.After(time.Second):
			t.Error("bookmarkd did not stop")
		}
	})

	baseURL := "http://" + addr
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		resp, err := http.Get(baseURL + "/healthz")
		if err == nil {
			_ = resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				client, err := apiclient.New(apiclient.Config{BaseURL: baseURL, Token: "test-token"})
				if err != nil {
					t.Fatalf("new API client: %v", err)
				}
				return client
			}
		}

		select {
		case err := <-errC:
			t.Fatalf("bookmarkd failed to start: %v", err)
		default:
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("bookmarkd did not become healthy")
	return nil
}

func waitForBookmarkTitle(t *testing.T, client *apiclient.Client, id, wantTitle string) {
	t.Helper()

	deadline := time.Now().Add(time.Second)
	var lastTitle string
	for time.Now().Before(deadline) {
		bookmarksList, err := client.ListBookmarks(context.Background(), bookmarks.ListQuery{})
		if err != nil {
			t.Fatalf("ListBookmarks() error = %v", err)
		}
		for _, bookmark := range bookmarksList {
			if bookmark.ID == id {
				lastTitle = bookmark.Title
				if bookmark.Title == wantTitle {
					return
				}
			}
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("bookmark %q title = %q, want %q", id, lastTitle, wantTitle)
}

func assertBookmarkTitleRemains(t *testing.T, client *apiclient.Client, id, wantTitle string) {
	t.Helper()

	deadline := time.Now().Add(200 * time.Millisecond)
	for time.Now().Before(deadline) {
		bookmarksList, err := client.ListBookmarks(context.Background(), bookmarks.ListQuery{})
		if err != nil {
			t.Fatalf("ListBookmarks() error = %v", err)
		}
		for _, bookmark := range bookmarksList {
			if bookmark.ID == id && bookmark.Title != wantTitle {
				t.Fatalf("bookmark %q title = %q, want unchanged %q", id, bookmark.Title, wantTitle)
			}
		}
		time.Sleep(10 * time.Millisecond)
	}
}
