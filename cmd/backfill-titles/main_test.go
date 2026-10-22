package main

import (
	"context"
	"errors"
	"strings"
	"testing"

	"bookmarks/internal/bookmarks"
)

type fakeStore struct {
	list   []bookmarks.Bookmark
	titles map[string]string
}

func (s *fakeStore) ListBookmarks(context.Context, bookmarks.ListOptions) ([]bookmarks.Bookmark, error) {
	return s.list, nil
}

func (s *fakeStore) SetBookmarkTitleIfBlank(_ context.Context, id, title string) (bool, error) {
	if _, ok := s.titles[id]; ok {
		return false, nil
	}
	s.titles[id] = title
	return true, nil
}

type fakeFetcher map[string]string

func (f fakeFetcher) FetchTitle(_ context.Context, url string) (string, error) {
	title, ok := f[url]
	if !ok {
		return "", errors.New("unreachable")
	}
	return title, nil
}

func newFixtures() (*fakeStore, fakeFetcher) {
	st := &fakeStore{
		list: []bookmarks.Bookmark{
			{ID: "1", URL: "https://a.test"},
			{ID: "2", URL: "https://b.test", Title: "Already"},
			{ID: "3", URL: "https://down.test", Title: "  "},
			{ID: "4", URL: "https://blank.test"},
			{ID: "5", URL: "https://raced.test"},
		},
		titles: map[string]string{"5": "Set meanwhile"},
	}
	f := fakeFetcher{
		"https://a.test":     "A\tTitle",
		"https://b.test":     "never fetched",
		"https://blank.test": "",
		"https://raced.test": "Raced",
	}
	return st, f
}

func TestRun(t *testing.T) {
	st, f := newFixtures()
	var out strings.Builder

	c, err := run(context.Background(), st, f, options{}, &out)
	if err != nil {
		t.Fatal(err)
	}

	want := counts{filled: 1, empty: 1, skipped: 1, failed: 1}
	if c != want {
		t.Errorf("counts = %+v, want %+v", c, want)
	}
	if st.titles["1"] != "A\tTitle" {
		t.Errorf("title 1 = %q", st.titles["1"])
	}
	if _, ok := st.titles["2"]; ok {
		t.Error("titled bookmark was written")
	}
	wantOut := "filled\t1\thttps://a.test\tA Title\n" +
		"error\t3\thttps://down.test\tunreachable\n" +
		"empty\t4\thttps://blank.test\t\n" +
		"skipped\t5\thttps://raced.test\t\n"
	if out.String() != wantOut {
		t.Errorf("output:\n%s\nwant:\n%s", out.String(), wantOut)
	}
}

func TestRunDryRun(t *testing.T) {
	st, f := newFixtures()
	var out strings.Builder

	c, err := run(context.Background(), st, f, options{dryRun: true, limit: 1}, &out)
	if err != nil {
		t.Fatal(err)
	}
	if c != (counts{dryRun: 1}) {
		t.Errorf("counts = %+v", c)
	}
	if len(st.titles) != 1 {
		t.Errorf("dry run wrote titles: %v", st.titles)
	}
	if got, want := out.String(), "dry-run\t1\thttps://a.test\tA Title\n"; got != want {
		t.Errorf("output = %q, want %q", got, want)
	}
}

func TestRunCanceled(t *testing.T) {
	st, f := newFixtures()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if _, err := run(ctx, st, f, options{}, &strings.Builder{}); !errors.Is(err, context.Canceled) {
		t.Errorf("err = %v, want context.Canceled", err)
	}
}
