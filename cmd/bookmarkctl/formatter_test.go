package main

import (
	"bytes"
	"encoding/json"
	"io"
	"reflect"
	"strings"
	"testing"
	"time"

	"bookmarks/internal/bookmarks"
)

func TestWriteListTSV(t *testing.T) {
	var buf bytes.Buffer

	// TSV is always "full" (all details), -l has no effect on TSV.
	// Column order chosen so that cut -f1=Title, -f2=URL, -f3=ID continue to work:
	// Title, URL, ID, Notes, Source, CreatedAt, UpdatedAt, NormalizedURL, Tags
	err := WriteListBookmarks(&buf, sampleListBookmarks(), ListFormatOptions{Format: ListFormatTSV})
	if err != nil {
		t.Fatalf("WriteListBookmarks() error = %v", err)
	}

	// Assert full TSV structure without pinning the exact time format used for CreatedAt/UpdatedAt.
	// The CLI may change how it renders times; we only care that the columns are present and other fields match.
	output := strings.TrimSuffix(buf.String(), "\n")
	lines := strings.Split(output, "\n")
	if len(lines) != 2 {
		t.Fatalf("printed %d lines, want 2: %q", len(lines), output)
	}
	for i, line := range lines {
		fields := strings.Split(line, "\t")
		if len(fields) != 8 {
			t.Fatalf("row %d has %d columns, want 8: %q", i, len(fields), line)
		}
		// Columns: 0=Title, 1=URL, 2=Notes, 3=Source, 4=CreatedAt, 5=UpdatedAt, 6=NormalizedURL, 7=ID
		if i == 0 {
			if fields[0] != "Example" || fields[1] != "https://example.com/a" || fields[7] != "bookmark-1" || fields[6] != "https://example.com/a" {
				t.Fatalf("row 0 = %#v, unexpected non-time fields", fields)
			}
		} else {
			if fields[0] != "" || fields[1] != "https://example.com/b" || fields[7] != "bookmark-2" || fields[6] != "https://example.com/b" {
				t.Fatalf("row 1 = %#v, unexpected non-time fields", fields)
			}
		}
		if fields[4] == "" || fields[5] == "" {
			t.Fatalf("row %d missing time values in columns 4/5: %q", i, line)
		}
	}
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

	// Normal (short) table: only Title and URL per the ls-inspired design.
	// TSV/JSON are always "full" regardless of Long.
	err := WriteListBookmarks(&buf, sampleListBookmarks(), ListFormatOptions{Format: ListFormatTable})
	if err != nil {
		t.Fatalf("WriteListBookmarks() error = %v", err)
	}

	output := buf.String()
	lines := strings.Split(strings.TrimSuffix(output, "\n"), "\n")
	if len(lines) == 0 {
		t.Fatalf("no output")
	}
	header := lines[0]
	// Short table headers: only Title and URL (ls-style normal view)
	for _, label := range []string{"Title", "URL"} {
		if !strings.Contains(header, label) {
			t.Fatalf("short table header missing %q: %q", label, header)
		}
	}
	if strings.Contains(header, "ID") {
		t.Fatalf("short table header should not include ID: %q", header)
	}
	// No ID values should leak into short table output for this sample
	for _, id := range []string{"bookmark-1", "bookmark-2"} {
		if strings.Contains(output, id) {
			t.Fatalf("short table should not expose ID values: found %q in %q", id, output)
		}
	}

	if len(lines) != 3 {
		t.Fatalf("printed %d lines, want 3 (header + 2 rows): %q", len(lines), output)
	}

	for _, want := range []string{
		"https://example.com/a",
		"Example",
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

	// Normal table: Title + URL only; still need alignment between rows.
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

	// Short (normal) empty table: header only, Title and URL, no ID.
	err := WriteListBookmarks(&buf, nil, ListFormatOptions{Format: ListFormatTable})
	if err != nil {
		t.Fatalf("WriteListBookmarks() error = %v", err)
	}

	output := strings.TrimSuffix(buf.String(), "\n")
	lines := strings.Split(output, "\n")
	if len(lines) != 1 {
		t.Fatalf("empty short table should be exactly one header line, got %q", buf.String())
	}
	header := lines[0]
	if !strings.Contains(header, "Title") || !strings.Contains(header, "URL") {
		t.Fatalf("short table header should contain Title and URL: %q", header)
	}
	if strings.Contains(header, "ID") {
		t.Fatalf("short table header must not contain ID: %q", header)
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

// sampleListBookmarksLong returns bookmarks with all fields populated for testing long output.
func sampleListBookmarksLong() []bookmarks.Bookmark {
	t1 := time.Date(2024, 6, 1, 12, 0, 0, 0, time.UTC)
	t2 := time.Date(2024, 6, 2, 13, 30, 0, 0, time.UTC)
	return []bookmarks.Bookmark{
		{
			ID:            "bookmark-1",
			URL:           "https://example.com/a",
			NormalizedURL: "https://example.com/a",
			Title:         "Example",
			Notes:         "read this",
			Source:        "ios",
			CreatedAt:     t1,
			UpdatedAt:     t1,
		},
		{
			ID:            "bookmark-2",
			URL:           "https://example.com/b",
			NormalizedURL: "https://example.com/b",
			Title:         "",
			Notes:         "",
			Source:        "",
			CreatedAt:     t2,
			UpdatedAt:     t2,
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

// --- Long table tests (only table format respects Long) ---

func TestWriteListTableLong(t *testing.T) {
	var buf bytes.Buffer

	err := WriteListBookmarks(&buf, sampleListBookmarksLong(), ListFormatOptions{
		Format: ListFormatTable,
		Long:   true,
	})
	if err != nil {
		t.Fatalf("WriteListBookmarks() error = %v", err)
	}

	output := buf.String()

	// Long table must include all key fields in the header.
	// We use friendly uppercase headers similar to existing Title/URL/ID style.
	for _, label := range []string{"Title", "URL", "Notes", "Source", "CreatedAt", "UpdatedAt", "ID"} {
		if !strings.Contains(output, label) {
			t.Fatalf("long table missing header %q: %q", label, output)
		}
	}

	// Data from first bookmark should appear (notes/source)
	if !strings.Contains(output, "read this") {
		t.Fatalf("long table missing notes content: %q", output)
	}
	if !strings.Contains(output, "ios") {
		t.Fatalf("long table missing source: %q", output)
	}

	// Second bookmark has empty title but should still show URL
	if !strings.Contains(output, "https://example.com/b") {
		t.Fatalf("long table missing second URL: %q", output)
	}

	// Time values: do not assert exact format (domain uses RFC3339 internally; CLI table uses %s on time.Time).
	// Just ensure the dates we set appear somewhere in the rendered long table output.
	if !strings.Contains(output, "2024-06-01") || !strings.Contains(output, "2024-06-02") {
		t.Fatalf("long table missing expected date portions from CreatedAt/UpdatedAt: %q", output)
	}
}

func TestWriteListTableLongEmpty(t *testing.T) {
	var buf bytes.Buffer

	err := WriteListBookmarks(&buf, nil, ListFormatOptions{Format: ListFormatTable, Long: true})
	if err != nil {
		t.Fatalf("WriteListBookmarks() error = %v", err)
	}

	output := strings.TrimSuffix(buf.String(), "\n")
	// Header line must still be present for discoverability
	assertTableHasHeaders(t, output, []string{"Title", "URL", "ID", "Notes"})
	if strings.Count(output, "\n") != 0 {
		t.Fatalf("empty long table should print only a header line, got %q", buf.String())
	}
}

func TestWriteListTSVIgnoresLong(t *testing.T) {
	var buf bytes.Buffer

	// Even with Long:true, TSV must remain the full column set (same as without -l)
	err := WriteListBookmarks(&buf, sampleListBookmarks(), ListFormatOptions{
		Format: ListFormatTSV,
		Long:   true,
	})
	if err != nil {
		t.Fatalf("WriteListBookmarks() error = %v", err)
	}

	// TSV full output must have 8 columns per the current schema.
	// Do not assert exact time formatting here (format-agnostic); just ensure time columns are populated.
	output := strings.TrimSuffix(buf.String(), "\n")
	lines := strings.Split(output, "\n")
	if len(lines) != 2 {
		t.Fatalf("printed %d lines, want 2: %q", len(lines), output)
	}
	for i, line := range lines {
		fields := strings.Split(line, "\t")
		if len(fields) != 8 {
			t.Fatalf("row %d has %d columns, want 8: %q", i, len(fields), line)
		}
		// Title, URL, Notes, Source, CreatedAt, UpdatedAt, NormalizedURL, ID
		if i == 0 {
			if fields[0] != "Example" || fields[1] != "https://example.com/a" || fields[7] != "bookmark-1" {
				t.Fatalf("row 0 non-time fields mismatch: %#v", fields)
			}
		} else {
			if fields[0] != "" || fields[1] != "https://example.com/b" || fields[7] != "bookmark-2" {
				t.Fatalf("row 1 non-time fields mismatch: %#v", fields)
			}
		}
		if fields[4] == "" || fields[5] == "" {
			t.Fatalf("row %d missing time column values: %q", i, line)
		}
	}
}

func TestWriteListJSONIgnoresLong(t *testing.T) {
	var buf bytes.Buffer

	// JSON always emits full bookmark objects; Long is irrelevant
	err := WriteListBookmarks(&buf, sampleListBookmarksLong(), ListFormatOptions{
		Format: ListFormatJSON,
		Long:   true,
	})
	if err != nil {
		t.Fatalf("WriteListBookmarks() error = %v", err)
	}

	var got []bookmarks.Bookmark
	if err := json.Unmarshal(buf.Bytes(), &got); err != nil {
		t.Fatalf("json.Unmarshal() error = %v, output = %q", err, buf.String())
	}
	if !reflect.DeepEqual(got, sampleListBookmarksLong()) {
		t.Fatalf("decoded bookmarks = %#v, want %#v", got, sampleListBookmarksLong())
	}
}

// assertTableHasHeaders checks that the table output contains the expected headers (order-independent).
func assertTableHasHeaders(t *testing.T, output string, headers []string) {
	t.Helper()
	for _, h := range headers {
		if !strings.Contains(output, h) {
			t.Fatalf("table output missing header %q: %q", h, output)
		}
	}
}

// assertTableRowCount checks number of lines (header + data rows).
func assertTableRowCount(t *testing.T, output string, wantLines int) {
	t.Helper()
	lines := strings.Split(strings.TrimSuffix(output, "\n"), "\n")
	if len(lines) != wantLines {
		t.Fatalf("printed %d lines, want %d: %q", len(lines), wantLines, output)
	}
}
