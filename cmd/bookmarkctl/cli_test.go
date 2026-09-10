package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"reflect"
	"strings"
	"testing"

	"bookmarks/internal/apiclient"
	"bookmarks/internal/bookmarks"
)

func TestRunRootHelp(t *testing.T) {
	var stdout, stderr bytes.Buffer

	err := runCLI(
		context.Background(),
		[]string{"--help"},
		validLookup(),
		&stdout,
		&stderr,
		newClientMustNotBeCalled(t),
	)
	if err != nil {
		t.Fatalf("run() error = %v, want nil", err)
	}
	if stderr.Len() != 0 {
		t.Fatalf("stderr = %q, want empty", stderr.String())
	}

	for _, command := range []string{"add", "list", "edit", "delete"} {
		if !strings.Contains(stdout.String(), command) {
			t.Fatalf("root usage = %q, want command %q", stdout.String(), command)
		}
	}
	if strings.Contains(stdout.String(), "Show help for a command") {
		t.Fatalf("root usage = %q, want no help command", stdout.String())
	}
}

func TestRunRootVersion(t *testing.T) {
	var stdout, stderr bytes.Buffer

	err := runCLI(
		context.Background(),
		[]string{"--version"},
		validLookup(),
		&stdout,
		&stderr,
		newClientMustNotBeCalled(t),
	)
	if err != nil {
		t.Fatalf("run() error = %v, want nil", err)
	}
	if got, want := stdout.String(), version+"\n"; got != want {
		t.Fatalf("stdout = %q, want %q", got, want)
	}
}

func TestRunRequiresCommand(t *testing.T) {
	var stdout, stderr bytes.Buffer

	err := runCLI(
		context.Background(),
		nil,
		validLookup(),
		&stdout,
		&stderr,
		newClientMustNotBeCalled(t),
	)
	if err == nil {
		t.Fatal("run() error = nil, want error")
	}
	var usageErr *usageError
	if !errors.As(err, &usageErr) {
		t.Fatalf("run() error = %T, want *usageError", err)
	}
}

func TestRunRejectsUnknownCommand(t *testing.T) {
	var stdout, stderr bytes.Buffer

	err := runCLI(
		context.Background(),
		[]string{"wat"},
		validLookup(),
		&stdout,
		&stderr,
		newClientMustNotBeCalled(t),
	)
	if err == nil || !strings.Contains(err.Error(), "unknown command") {
		t.Fatalf("run() error = %v, want unknown command error", err)
	}
}

func TestRunCommandHelpDoesNotCreateClient(t *testing.T) {
	for _, command := range []string{"add", "list", "edit", "delete"} {
		t.Run(command, func(t *testing.T) {
			var stdout, stderr bytes.Buffer

			err := runCLI(
				context.Background(),
				[]string{command, "--help"},
				validLookup(),
				&stdout,
				&stderr,
				newClientMustNotBeCalled(t),
			)
			if err != nil {
				t.Fatalf("run() error = %v, want nil", err)
			}
			if stdout.Len() == 0 {
				t.Fatal("stdout is empty, want command usage")
			}
			if stderr.Len() != 0 {
				t.Fatalf("stderr = %q, want empty", stderr.String())
			}
		})
	}
}

func TestRunHelpIsNotACommand(t *testing.T) {
	var stdout, stderr bytes.Buffer

	err := runCLI(
		context.Background(),
		[]string{"help"},
		validLookup(),
		&stdout,
		&stderr,
		newClientMustNotBeCalled(t),
	)
	if err == nil || !strings.Contains(err.Error(), "unknown command") {
		t.Fatalf("run() error = %v, want unknown command error", err)
	}
}

func TestRunAdd(t *testing.T) {
	client := &fakeBookmarkClient{
		createBookmark: bookmarks.Bookmark{
			ID:  "bookmark-1",
			URL: "https://example.com/a",
		},
		createCreated: true,
	}
	var stdout, stderr bytes.Buffer

	err := runCLI(
		context.Background(),
		[]string{"add", "-title", "Example", "-notes", "Read later", "https://example.com/a"},
		validLookup(),
		&stdout,
		&stderr,
		clientFactory(client),
	)
	if err != nil {
		t.Fatalf("run() error = %v", err)
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
	if got, want := stdout.String(), "added https://example.com/a\n"; got != want {
		t.Fatalf("stdout = %q, want %q", got, want)
	}
}

func TestRunAddExistingStillConfirmsAction(t *testing.T) {
	client := &fakeBookmarkClient{
		createBookmark: bookmarks.Bookmark{URL: "https://example.com/a"},
		createCreated:  false,
	}
	var stdout, stderr bytes.Buffer

	err := runCLI(
		context.Background(),
		[]string{"add", "https://example.com/a"},
		validLookup(),
		&stdout,
		&stderr,
		clientFactory(client),
	)
	if err != nil {
		t.Fatalf("run() error = %v", err)
	}
	if got := stdout.String(); got != "exists https://example.com/a\n" {
		t.Fatalf("stdout = %q, want existing-bookmark confirmation", got)
	}
}

func TestRunListDefaultsToTableWithIDTitleAndURL(t *testing.T) {
	client := &fakeBookmarkClient{
		listBookmarks: []bookmarks.Bookmark{
			{ID: "bookmark-1", Title: "Example", URL: "https://example.com/a"},
		},
	}
	var stdout, stderr bytes.Buffer

	err := runCLI(
		context.Background(),
		[]string{"list"},
		validLookup(),
		&stdout,
		&stderr,
		clientFactory(client),
	)
	if err != nil {
		t.Fatalf("run() error = %v", err)
	}

	lines := strings.Split(strings.TrimSpace(stdout.String()), "\n")
	if len(lines) != 2 {
		t.Fatalf("table lines = %d, want 2: %q", len(lines), stdout.String())
	}
	for _, field := range []string{"ID", "Title", "URL"} {
		if !strings.Contains(lines[0], field) {
			t.Fatalf("table header = %q, want %q", lines[0], field)
		}
	}
	if !strings.Contains(lines[1], "bookmark-1") {
		t.Fatalf("table row = %q, want bookmark ID", lines[1])
	}
}

func TestRunListPassesQueryOptions(t *testing.T) {
	client := &fakeBookmarkClient{}
	var stdout, stderr bytes.Buffer

	err := runCLI(
		context.Background(),
		[]string{"list", "-query", "sqlite", "-limit", "25", "-offset", "50", "-format", "json"},
		validLookup(),
		&stdout,
		&stderr,
		clientFactory(client),
	)
	if err != nil {
		t.Fatalf("run() error = %v", err)
	}
	want := bookmarks.ListQuery{Query: "sqlite", Limit: 25, Offset: 50}
	if client.listQuery != want {
		t.Fatalf("ListBookmarks query = %#v, want %#v", client.listQuery, want)
	}
}

func TestRunListJSON(t *testing.T) {
	client := &fakeBookmarkClient{listBookmarks: sampleListBookmarks()}
	var stdout, stderr bytes.Buffer

	err := runCLI(
		context.Background(),
		[]string{"list", "-format", "json"},
		validLookup(),
		&stdout,
		&stderr,
		clientFactory(client),
	)
	if err != nil {
		t.Fatalf("run() error = %v", err)
	}
	var got []bookmarks.Bookmark
	if err := json.Unmarshal(stdout.Bytes(), &got); err != nil {
		t.Fatalf("json.Unmarshal() error = %v, stdout = %q", err, stdout.String())
	}
	if !reflect.DeepEqual(got, sampleListBookmarks()) {
		t.Fatalf("decoded bookmarks = %#v, want %#v", got, sampleListBookmarks())
	}
}

func TestRunListRejectsInvalidArgumentsBeforeCreatingClient(t *testing.T) {
	tests := []struct {
		name string
		args []string
	}{
		{name: "negative limit", args: []string{"list", "-limit", "-1"}},
		{name: "negative offset", args: []string{"list", "-offset", "-1"}},
		{name: "unknown format", args: []string{"list", "-format", "wat"}},
		{name: "extra argument", args: []string{"list", "extra"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			err := runCLI(
				context.Background(),
				tt.args,
				validLookup(),
				&stdout,
				&stderr,
				newClientMustNotBeCalled(t),
			)
			if err == nil {
				t.Fatal("run() error = nil, want error")
			}
		})
	}
}

func TestRunEdit(t *testing.T) {
	client := &fakeBookmarkClient{
		updateBookmark: bookmarks.Bookmark{ID: "bookmark-1", URL: "https://example.com/new"},
	}
	var stdout, stderr bytes.Buffer

	err := runCLI(
		context.Background(),
		[]string{"edit", "-url", "https://example.com/new", "-title", "Updated", "-notes", "", "-source", "bookmarkctl", "bookmark-1"},
		validLookup(),
		&stdout,
		&stderr,
		clientFactory(client),
	)
	if err != nil {
		t.Fatalf("run() error = %v", err)
	}
	if client.updateID != "bookmark-1" {
		t.Fatalf("update ID = %q, want bookmark-1", client.updateID)
	}
	assertStringPtr(t, "URL", client.updateInput.URL, "https://example.com/new")
	assertStringPtr(t, "Title", client.updateInput.Title, "Updated")
	assertStringPtr(t, "Notes", client.updateInput.Notes, "")
	assertStringPtr(t, "Source", client.updateInput.Source, "bookmarkctl")
	if got, want := stdout.String(), "updated bookmark-1\n"; got != want {
		t.Fatalf("stdout = %q, want %q", got, want)
	}
}

func TestRunEditRequiresAtLeastOneField(t *testing.T) {
	var stdout, stderr bytes.Buffer

	err := runCLI(
		context.Background(),
		[]string{"edit", "bookmark-1"},
		validLookup(),
		&stdout,
		&stderr,
		newClientMustNotBeCalled(t),
	)
	if err == nil || !strings.Contains(err.Error(), "at least one field") {
		t.Fatalf("run() error = %v, want missing-field error", err)
	}
}

func TestRunDelete(t *testing.T) {
	client := &fakeBookmarkClient{}
	var stdout, stderr bytes.Buffer

	err := runCLI(
		context.Background(),
		[]string{"delete", "bookmark-1"},
		validLookup(),
		&stdout,
		&stderr,
		clientFactory(client),
	)
	if err != nil {
		t.Fatalf("run() error = %v", err)
	}
	if client.deleteID != "bookmark-1" {
		t.Fatalf("delete ID = %q, want bookmark-1", client.deleteID)
	}
	if got, want := stdout.String(), "deleted bookmark-1\n"; got != want {
		t.Fatalf("stdout = %q, want %q", got, want)
	}
}

func TestRunRejectsClientErrors(t *testing.T) {
	tests := []struct {
		name string
		args []string
		make func() *fakeBookmarkClient
	}{
		{name: "add", args: []string{"add", "https://example.com"}, make: func() *fakeBookmarkClient {
			return &fakeBookmarkClient{createErr: errors.New("boom")}
		}},
		{name: "list", args: []string{"list"}, make: func() *fakeBookmarkClient {
			return &fakeBookmarkClient{listErr: errors.New("boom")}
		}},
		{name: "edit", args: []string{"edit", "bookmark-1", "-title", "Updated"}, make: func() *fakeBookmarkClient {
			return &fakeBookmarkClient{updateErr: errors.New("boom")}
		}},
		{name: "delete", args: []string{"delete", "bookmark-1"}, make: func() *fakeBookmarkClient {
			return &fakeBookmarkClient{deleteErr: errors.New("boom")}
		}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := tt.make()
			var stdout, stderr bytes.Buffer
			err := runCLI(
				context.Background(),
				tt.args,
				validLookup(),
				&stdout,
				&stderr,
				clientFactory(client),
			)
			if err == nil {
				t.Fatal("run() error = nil, want error")
			}
		})
	}
}

func newClientMustNotBeCalled(t *testing.T) func(apiclient.Config) (bookmarkClient, error) {
	t.Helper()
	return func(apiclient.Config) (bookmarkClient, error) {
		t.Fatal("newClient should not be called")
		return nil, nil
	}
}

func runCLI(
	ctx context.Context,
	args []string,
	lookup func(string) (string, bool),
	stdout io.Writer,
	stderr io.Writer,
	newClient func(apiclient.Config) (bookmarkClient, error),
) error {
	app := &cli{
		lookup:    lookup,
		stdout:    stdout,
		stderr:    stderr,
		newClient: newClient,
	}
	return app.run(ctx, args)
}

func clientFactory(client *fakeBookmarkClient) func(apiclient.Config) (bookmarkClient, error) {
	return func(apiclient.Config) (bookmarkClient, error) {
		return client, nil
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
