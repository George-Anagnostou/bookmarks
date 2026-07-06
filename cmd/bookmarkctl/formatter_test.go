package main

import (
	"bytes"
	"encoding/json"
	"io"
	"reflect"
	"strings"
	"testing"

	"bookmarks/internal/bookmarks"
)

func TestWriteListTSV(t *testing.T) {
	var buf bytes.Buffer

	err := WriteListBookmarks(&buf, sampleListBookmarks(), ListFormatOptions{Format: ListFormatTSV})
	if err != nil {
		t.Fatalf("WriteListBookmarks() error = %v", err)
	}

	assertBookmarkRows(t, buf.String(), [][]string{
		{"Example", "https://example.com/a", "bookmark-1"},
		{"", "https://example.com/b", "bookmark-2"},
	})
}

func TestWriteListTSVEmpty(t *testing.T) {
	var buf bytes.Buffer

	err := WriteListBookmarks(&buf, nil, ListFormatOptions{Format: ListFormatTSV})
	if err != nil {
		t.Fatalf("WriteListBookmarks() error = %v", err)
	}
	if buf.Len() != 0 {
		t.Fatalf("stdout = %q, want empty", buf.String())
	}
}

func TestWriteListJSON(t *testing.T) {
	var buf bytes.Buffer

	err := WriteListBookmarks(&buf, sampleListBookmarks(), ListFormatOptions{Format: ListFormatJSON})
	if err != nil {
		t.Fatalf("WriteListBookmarks() error = %v", err)
	}

	var got []bookmarks.Bookmark
	if err := json.Unmarshal(buf.Bytes(), &got); err != nil {
		t.Fatalf("json.Unmarshal() error = %v, output = %q", err, buf.String())
	}
	if !reflect.DeepEqual(got, sampleListBookmarks()) {
		t.Fatalf("decoded bookmarks = %#v, want %#v", got, sampleListBookmarks())
	}
}

func TestWriteListJSONEmpty(t *testing.T) {
	var buf bytes.Buffer

	err := WriteListBookmarks(&buf, nil, ListFormatOptions{Format: ListFormatJSON})
	if err != nil {
		t.Fatalf("WriteListBookmarks() error = %v", err)
	}
	if got := buf.String(); got != "[]\n" {
		t.Fatalf("stdout = %q, want %q", got, "[]\n")
	}
}

func TestWriteListTable(t *testing.T) {
	var buf bytes.Buffer

	err := WriteListBookmarks(&buf, sampleListBookmarks(), ListFormatOptions{Format: ListFormatTable})
	if err != nil {
		t.Fatalf("WriteListBookmarks() error = %v", err)
	}

	output := buf.String()
	for _, label := range []string{"TITLE", "URL", "ID"} {
		if !strings.Contains(output, label) {
			t.Fatalf("table output missing header %q: %q", label, output)
		}
	}

	lines := strings.Split(strings.TrimSuffix(output, "\n"), "\n")
	if len(lines) != 3 {
		t.Fatalf("printed %d lines, want 3 (header + 2 rows): %q", len(lines), output)
	}

	for _, want := range []string{
		"bookmark-1",
		"https://example.com/a",
		"Example",
		"bookmark-2",
		"https://example.com/b",
		"",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("table output missing %q: %q", want, output)
		}
	}
}

func TestWriteListTableAlignsColumns(t *testing.T) {
	bookmarkList := []bookmarks.Bookmark{
		{
			ID:    "short-id",
			Title: "A",
			URL:   "https://a.example",
		},
		{
			ID:    "much-longer-bookmark-id",
			Title: "Longer title here",
			URL:   "https://b.example/also/long",
		},
	}
	var buf bytes.Buffer

	err := WriteListBookmarks(&buf, bookmarkList, ListFormatOptions{Format: ListFormatTable})
	if err != nil {
		t.Fatalf("WriteListBookmarks() error = %v", err)
	}

	lines := strings.Split(strings.TrimSuffix(buf.String(), "\n"), "\n")
	if len(lines) != 3 {
		t.Fatalf("printed %d lines, want 3: %q", len(lines), buf.String())
	}

	urlStartFirstRow := strings.Index(lines[1], "https://")
	urlStartSecondRow := strings.Index(lines[2], "https://")
	if urlStartFirstRow < 0 || urlStartSecondRow < 0 {
		t.Fatalf("table output missing URL column: %q", buf.String())
	}
	if urlStartFirstRow != urlStartSecondRow {
		t.Fatalf("URL column misaligned: first row at %d, second row at %d:\n%s", urlStartFirstRow, urlStartSecondRow, buf.String())
	}
}

func TestWriteListTableEmpty(t *testing.T) {
	var buf bytes.Buffer

	err := WriteListBookmarks(&buf, nil, ListFormatOptions{Format: ListFormatTable})
	if err != nil {
		t.Fatalf("WriteListBookmarks() error = %v", err)
	}

	output := strings.TrimSuffix(buf.String(), "\n")
	if !strings.Contains(output, "TITLE") {
		t.Fatalf("table output missing header: %q", buf.String())
	}
	if strings.Count(output, "\n") != 0 {
		t.Fatalf("empty table should print only a header line, got %q", buf.String())
	}
}

func TestWriteListBookmarksRejectsUnknownFormat(t *testing.T) {
	var buf bytes.Buffer

	err := WriteListBookmarks(&buf, sampleListBookmarks(), ListFormatOptions{Format: ListFormat("wat")})
	if err == nil {
		t.Fatal("WriteListBookmarks() error = nil, want error")
	}
	if buf.Len() != 0 {
		t.Fatalf("stdout = %q, want empty on error", buf.String())
	}
}

func TestResolveListFormatExplicit(t *testing.T) {
	tests := []struct {
		name     string
		explicit string
		want     ListFormat
	}{
		{name: "tsv", explicit: "tsv", want: ListFormatTSV},
		{name: "table", explicit: "table", want: ListFormatTable},
		{name: "json", explicit: "json", want: ListFormatJSON},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ResolveListFormat(tt.explicit, &bytes.Buffer{}, func(io.Writer) bool {
				t.Fatal("isTTY should not be called when format is explicit")
				return false
			})
			if err != nil {
				t.Fatalf("ResolveListFormat() error = %v", err)
			}
			if got != tt.want {
				t.Fatalf("ResolveListFormat() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestResolveListFormatRejectsUnknownExplicit(t *testing.T) {
	_, err := ResolveListFormat("wat", &bytes.Buffer{}, func(io.Writer) bool {
		return true
	})
	if err == nil {
		t.Fatal("ResolveListFormat() error = nil, want error")
	}
}

func TestResolveListFormatDefaults(t *testing.T) {
	tests := []struct {
		name  string
		isTTY bool
		want  ListFormat
	}{
		{name: "terminal", isTTY: true, want: ListFormatTable},
		{name: "pipe", isTTY: false, want: ListFormatTSV},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ResolveListFormat("", &bytes.Buffer{}, func(io.Writer) bool {
				return tt.isTTY
			})
			if err != nil {
				t.Fatalf("ResolveListFormat() error = %v", err)
			}
			if got != tt.want {
				t.Fatalf("ResolveListFormat() = %q, want %q", got, tt.want)
			}
		})
	}
}

func sampleListBookmarks() []bookmarks.Bookmark {
	return []bookmarks.Bookmark{
		{
			ID:            "bookmark-1",
			URL:           "https://example.com/a",
			NormalizedURL: "https://example.com/a",
			Title:         "Example",
		},
		{
			ID:            "bookmark-2",
			URL:           "https://example.com/b",
			NormalizedURL: "https://example.com/b",
		},
	}
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

