package main

import (
	"bytes"
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"

	"bookmarks/internal/apiclient"
	"bookmarks/internal/bookmarks"
)

func TestRunRequiresCommand(t *testing.T) {
	var stdout, stderr bytes.Buffer

	err := run(
		context.Background(),
		nil,
		mapLookup(nil),
		&stdout,
		&stderr,
		func(apiclient.Config) (bookmarkClient, error) {
			t.Fatal("newClient should not be called")
			return nil, nil
		},
	)
	if err == nil {
		t.Fatal("run() error = nil, want error")
	}
}

func TestRunRejectsUnknownCommand(t *testing.T) {
	var stdout, stderr bytes.Buffer

	err := run(
		context.Background(),
		[]string{"wat"},
		mapLookup(nil),
		&stdout,
		&stderr,
		func(apiclient.Config) (bookmarkClient, error) {
			t.Fatal("newClient should not be called")
			return nil, nil
		},
	)
	if err == nil {
		t.Fatal("run() error = nil, want error")
	}
	if !strings.Contains(err.Error(), "unknown command") {
		t.Fatalf("run() error = %q, want unknown command", err)
	}
}

func TestRunAddCreatesBookmark(t *testing.T) {
	client := &fakeBookmarkClient{
		createBookmark: bookmarks.Bookmark{
			URL:           "https://example.com/a",
			NormalizedURL: "https://example.com/a",
			Title:         "Example",
		},
		createCreated: true,
	}
	var gotConfig apiclient.Config
	var stdout, stderr bytes.Buffer

	err := run(
		context.Background(),
		[]string{"add", "-title", "Example", "-notes", "Read later", "https://example.com/a"},
		mapLookup(map[string]string{
			"BOOKMARKS_URL":   "http://localhost:8080",
			"BOOKMARKS_TOKEN": "test-token",
		}),
		&stdout,
		&stderr,
		func(cfg apiclient.Config) (bookmarkClient, error) {
			gotConfig = cfg
			return client, nil
		},
	)
	if err != nil {
		t.Fatalf("run() error = %v", err)
	}

	if gotConfig.BaseURL != "http://localhost:8080" {
		t.Fatalf("BaseURL = %q", gotConfig.BaseURL)
	}
	if gotConfig.Token != "test-token" {
		t.Fatalf("Token = %q", gotConfig.Token)
	}

	wantInput := bookmarks.CreateInput{
		URL:    "https://example.com/a",
		Title:  "Example",
		Notes:  "Read later",
		Source: "bookmarkctl",
	}
	if !reflect.DeepEqual(client.createInput, wantInput) {
		t.Fatalf("CreateBookmark input = %#v, want %#v", client.createInput, wantInput)
	}

	if got := stdout.String(); got != "created https://example.com/a\n" {
		t.Fatalf("stdout = %q", got)
	}
}

func TestRunAddReportsExistingBookmark(t *testing.T) {
	client := &fakeBookmarkClient{
		createBookmark: bookmarks.Bookmark{
			URL:           "https://example.com/a",
			NormalizedURL: "https://example.com/a",
		},
		createCreated: false,
	}
	var stdout, stderr bytes.Buffer

	err := run(
		context.Background(),
		[]string{"add", "https://example.com/a"},
		validLookup(),
		&stdout,
		&stderr,
		func(apiclient.Config) (bookmarkClient, error) {
			return client, nil
		},
	)
	if err != nil {
		t.Fatalf("run() error = %v", err)
	}

	if got := stdout.String(); got != "exists https://example.com/a\n" {
		t.Fatalf("stdout = %q", got)
	}
}

func TestRunAddRequiresURL(t *testing.T) {
	client := &fakeBookmarkClient{}
	var stdout, stderr bytes.Buffer

	err := run(
		context.Background(),
		[]string{"add"},
		validLookup(),
		&stdout,
		&stderr,
		func(apiclient.Config) (bookmarkClient, error) {
			return client, nil
		},
	)
	if err == nil {
		t.Fatal("run() error = nil, want error")
	}
	if client.createCalled {
		t.Fatal("CreateBookmark was called")
	}
}

func TestRunListPrintsBookmarks(t *testing.T) {
	client := &fakeBookmarkClient{
		listBookmarks: []bookmarks.Bookmark{
			{
				URL:           "https://example.com/a",
				NormalizedURL: "https://example.com/a",
				Title:         "Example",
			},
			{
				URL:           "https://example.com/b",
				NormalizedURL: "https://example.com/b",
			},
		},
	}
	var stdout, stderr bytes.Buffer

	err := run(
		context.Background(),
		[]string{"list"},
		validLookup(),
		&stdout,
		&stderr,
		func(apiclient.Config) (bookmarkClient, error) {
			return client, nil
		},
	)
	if err != nil {
		t.Fatalf("run() error = %v", err)
	}

	assertBookmarkRows(t, stdout.String(), [][]string{
		{"https://example.com/a", "Example"},
		{"https://example.com/b", ""},
	})
}

func TestRunReturnsClientErrors(t *testing.T) {
	client := &fakeBookmarkClient{
		createErr: errors.New("boom"),
	}
	var stdout, stderr bytes.Buffer

	err := run(
		context.Background(),
		[]string{"add", "https://example.com/a"},
		validLookup(),
		&stdout,
		&stderr,
		func(apiclient.Config) (bookmarkClient, error) {
			return client, nil
		},
	)
	if err == nil {
		t.Fatal("run() error = nil, want error")
	}
}

func TestRunReturnsListErrors(t *testing.T) {
	client := &fakeBookmarkClient{
		listErr: errors.New("boom"),
	}
	var stdout, stderr bytes.Buffer

	err := run(
		context.Background(),
		[]string{"list"},
		validLookup(),
		&stdout,
		&stderr,
		func(apiclient.Config) (bookmarkClient, error) {
			return client, nil
		},
	)
	if err == nil {
		t.Fatal("run() error = nil, want error")
	}
}

func TestRunReturnsNewClientErrors(t *testing.T) {
	var stdout, stderr bytes.Buffer

	err := run(
		context.Background(),
		[]string{"add", "https://example.com/a"},
		validLookup(),
		&stdout,
		&stderr,
		func(apiclient.Config) (bookmarkClient, error) {
			return nil, errors.New("boom")
		},
	)
	if err == nil {
		t.Fatal("run() error = nil, want error")
	}
}

func validLookup() func(string) (string, bool) {
	return mapLookup(map[string]string{
		"BOOKMARKS_URL":   "http://localhost:8080",
		"BOOKMARKS_TOKEN": "test-token",
	})
}

func assertBookmarkRows(t *testing.T, output string, want [][]string) {
	t.Helper()

	output = strings.TrimSuffix(output, "\n")
	gotLines := strings.Split(output, "\n")
	if len(gotLines) != len(want) {
		t.Fatalf("printed %d rows, want %d: %q", len(gotLines), len(want), output)
	}

	for i, line := range gotLines {
		gotFields := strings.Split(line, "\t")
		if !reflect.DeepEqual(gotFields, want[i]) {
			t.Fatalf("row %d = %#v, want %#v", i, gotFields, want[i])
		}
	}
}

type fakeBookmarkClient struct {
	createCalled   bool
	createInput    bookmarks.CreateInput
	createBookmark bookmarks.Bookmark
	createCreated  bool
	createErr      error

	listBookmarks []bookmarks.Bookmark
	listErr       error
}

func (f *fakeBookmarkClient) CreateBookmark(ctx context.Context, input bookmarks.CreateInput) (bookmarks.Bookmark, bool, error) {
	f.createCalled = true
	f.createInput = input
	return f.createBookmark, f.createCreated, f.createErr
}

func (f *fakeBookmarkClient) ListBookmarks(ctx context.Context) ([]bookmarks.Bookmark, error) {
	return f.listBookmarks, f.listErr
}
