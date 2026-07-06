package main

import (
	"bytes"
	"context"
	"encoding/json"
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
		[]string{"add", "https://example.com/a", "-title", "Example", "-notes", "Read later"},
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

func TestRunListCallsClient(t *testing.T) {
	client := &fakeBookmarkClient{
		listBookmarks: sampleListBookmarks(),
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
	if !client.listCalled {
		t.Fatal("ListBookmarks was not called")
	}
	if client.listQuery != (bookmarks.ListQuery{}) {
		t.Fatalf("ListBookmarks query = %#v, want zero value", client.listQuery)
	}
	if stdout.Len() == 0 {
		t.Fatal("stdout is empty, want output")
	}
}

func TestRunListPassesQueryOptions(t *testing.T) {
	client := &fakeBookmarkClient{}
	var stdout, stderr bytes.Buffer

	err := run(
		context.Background(),
		[]string{"list", "-query", "sqlite", "-limit", "25", "-offset", "50"},
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
	if !client.listCalled {
		t.Fatal("ListBookmarks was not called")
	}
	want := bookmarks.ListQuery{
		Query:  "sqlite",
		Limit:  25,
		Offset: 50,
	}
	if client.listQuery != want {
		t.Fatalf("ListBookmarks query = %#v, want %#v", client.listQuery, want)
	}
}

func TestRunListOutputJSON(t *testing.T) {
	client := &fakeBookmarkClient{
		listBookmarks: sampleListBookmarks(),
	}
	var stdout, stderr bytes.Buffer

	err := run(
		context.Background(),
		[]string{"list", "-output", "json"},
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
	if !client.listCalled {
		t.Fatal("ListBookmarks was not called")
	}

	var got []bookmarks.Bookmark
	if err := json.Unmarshal(stdout.Bytes(), &got); err != nil {
		t.Fatalf("json.Unmarshal() error = %v, stdout = %q", err, stdout.String())
	}
	if len(got) != len(sampleListBookmarks()) {
		t.Fatalf("decoded bookmarks len = %d, want %d", len(got), len(sampleListBookmarks()))
	}
}

func TestRunListRejectsBadQueryOptionsBeforeCreatingClient(t *testing.T) {
	tests := []struct {
		name string
		args []string
	}{
		{
			name: "negative limit",
			args: []string{"list", "-limit", "-1"},
		},
		{
			name: "negative offset",
			args: []string{"list", "-offset", "-1"},
		},
		{
			name: "unknown output",
			args: []string{"list", "-output", "wat"},
		},
		{
			name: "extra arg",
			args: []string{"list", "extra"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			err := run(
				context.Background(),
				tt.args,
				validLookup(),
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
		})
	}
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

func TestRunEditUpdatesBookmark(t *testing.T) {
	client := &fakeBookmarkClient{
		updateBookmark: bookmarks.Bookmark{
			ID:  "bookmark-1",
			URL: "https://example.com/new",
		},
	}
	var stdout, stderr bytes.Buffer

	err := run(
		context.Background(),
		[]string{"edit", "bookmark-1", "-url", "https://example.com/new", "-title", "Updated", "-notes", "", "-source", "bookmarkctl"},
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

	if !client.updateCalled {
		t.Fatal("UpdateBookmark was not called")
	}
	if client.updateID != "bookmark-1" {
		t.Fatalf("update ID = %q, want bookmark-1", client.updateID)
	}
	assertStringPtr(t, "URL", client.updateInput.URL, "https://example.com/new")
	assertStringPtr(t, "Title", client.updateInput.Title, "Updated")
	assertStringPtr(t, "Notes", client.updateInput.Notes, "")
	assertStringPtr(t, "Source", client.updateInput.Source, "bookmarkctl")

	if got := stdout.String(); got != "updated bookmark-1 https://example.com/new\n" {
		t.Fatalf("stdout = %q", got)
	}
}

func TestRunEditCanClearFields(t *testing.T) {
	client := &fakeBookmarkClient{
		updateBookmark: bookmarks.Bookmark{
			ID:  "bookmark-1",
			URL: "https://example.com/a",
		},
	}
	var stdout, stderr bytes.Buffer

	err := run(
		context.Background(),
		[]string{"edit", "bookmark-1", "-title", "", "-notes", ""},
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

	assertStringPtr(t, "Title", client.updateInput.Title, "")
	assertStringPtr(t, "Notes", client.updateInput.Notes, "")
	if client.updateInput.URL != nil {
		t.Fatalf("URL = %#v, want nil", client.updateInput.URL)
	}
	if client.updateInput.Source != nil {
		t.Fatalf("Source = %#v, want nil", client.updateInput.Source)
	}
}

func TestRunEditRejectsBadArgsBeforeCreatingClient(t *testing.T) {
	tests := []struct {
		name string
		args []string
	}{
		{
			name: "missing id",
			args: []string{"edit"},
		},
		{
			name: "extra arg",
			args: []string{"edit", "bookmark-1", "extra", "-title", "Updated"},
		},
		{
			name: "no update flags",
			args: []string{"edit", "bookmark-1"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			err := run(
				context.Background(),
				tt.args,
				validLookup(),
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
		})
	}
}

func TestRunEditReturnsClientErrors(t *testing.T) {
	client := &fakeBookmarkClient{
		updateErr: errors.New("boom"),
	}
	var stdout, stderr bytes.Buffer

	err := run(
		context.Background(),
		[]string{"edit", "bookmark-1", "-title", "Updated"},
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

func TestRunDeleteDeletesBookmark(t *testing.T) {
	client := &fakeBookmarkClient{}
	var stdout, stderr bytes.Buffer

	err := run(
		context.Background(),
		[]string{"delete", "bookmark-1"},
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

	if !client.deleteCalled {
		t.Fatal("DeleteBookmark was not called")
	}
	if client.deleteID != "bookmark-1" {
		t.Fatalf("delete ID = %q, want bookmark-1", client.deleteID)
	}
	if got := stdout.String(); got != "deleted bookmark-1\n" {
		t.Fatalf("stdout = %q", got)
	}
}

func TestRunDeleteRejectsBadArgsBeforeCreatingClient(t *testing.T) {
	tests := []struct {
		name string
		args []string
	}{
		{
			name: "missing id",
			args: []string{"delete"},
		},
		{
			name: "extra arg",
			args: []string{"delete", "bookmark-1", "extra"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			err := run(
				context.Background(),
				tt.args,
				validLookup(),
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
		})
	}
}

func TestRunDeleteReturnsClientErrors(t *testing.T) {
	client := &fakeBookmarkClient{
		deleteErr: errors.New("boom"),
	}
	var stdout, stderr bytes.Buffer

	err := run(
		context.Background(),
		[]string{"delete", "bookmark-1"},
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

func TestRunEditReturnsNewClientErrors(t *testing.T) {
	var stdout, stderr bytes.Buffer

	err := run(
		context.Background(),
		[]string{"edit", "bookmark-1", "-title", "Updated"},
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

func TestRunDeleteReturnsNewClientErrors(t *testing.T) {
	var stdout, stderr bytes.Buffer

	err := run(
		context.Background(),
		[]string{"delete", "bookmark-1"},
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

func assertStringPtr(t *testing.T, name string, got *string, want string) {
	t.Helper()

	if got == nil {
		t.Fatalf("%s = nil, want %q", name, want)
	}
	if *got != want {
		t.Fatalf("%s = %q, want %q", name, *got, want)
	}
}

func validLookup() func(string) (string, bool) {
	return mapLookup(map[string]string{
		"BOOKMARKS_URL":   "http://localhost:8080",
		"BOOKMARKS_TOKEN": "test-token",
	})
}

type fakeBookmarkClient struct {
	createCalled   bool
	createInput    bookmarks.CreateInput
	createBookmark bookmarks.Bookmark
	createCreated  bool
	createErr      error

	listCalled    bool
	listQuery     bookmarks.ListQuery
	listBookmarks []bookmarks.Bookmark
	listErr       error

	updateCalled   bool
	updateID       string
	updateInput    bookmarks.UpdateInput
	updateBookmark bookmarks.Bookmark
	updateErr      error

	deleteCalled bool
	deleteID     string
	deleteErr    error
}

func (f *fakeBookmarkClient) CreateBookmark(ctx context.Context, input bookmarks.CreateInput) (bookmarks.Bookmark, bool, error) {
	f.createCalled = true
	f.createInput = input
	return f.createBookmark, f.createCreated, f.createErr
}

func (f *fakeBookmarkClient) ListBookmarks(ctx context.Context, query bookmarks.ListQuery) ([]bookmarks.Bookmark, error) {
	f.listCalled = true
	f.listQuery = query
	return f.listBookmarks, f.listErr
}

func (f *fakeBookmarkClient) UpdateBookmark(ctx context.Context, id string, input bookmarks.UpdateInput) (bookmarks.Bookmark, error) {
	f.updateCalled = true
	f.updateID = id
	f.updateInput = input
	return f.updateBookmark, f.updateErr
}

func (f *fakeBookmarkClient) DeleteBookmark(ctx context.Context, id string) error {
	f.deleteCalled = true
	f.deleteID = id
	return f.deleteErr
}
