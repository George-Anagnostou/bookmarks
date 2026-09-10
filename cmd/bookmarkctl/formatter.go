package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"golang.org/x/term"

	"bookmarks/internal/bookmarks"
)

type ListFormat string

const (
	ListFormatTable ListFormat = "table"
	ListFormatTSV   ListFormat = "tsv"
	ListFormatJSON  ListFormat = "json"
)

type ListFormatOptions struct {
	Format ListFormat
	Width  int
	Long   bool
}

func WriteListBookmarks(w io.Writer, bookmarkList []bookmarks.Bookmark, opts ListFormatOptions) error {
	switch opts.Format {
	case ListFormatTable:
		return writeListTable(w, bookmarkList, opts)
	case ListFormatTSV:
		return writeListTSV(w, bookmarkList)
	case ListFormatJSON:
		return writeListJSON(w, bookmarkList)
	default:
		return fmt.Errorf("unknown list format %q", opts.Format)
	}
}

func ResolveListFormat(explicit string, stdout io.Writer, isTTY func(io.Writer) bool) (ListFormat, error) {
	if explicit != "" {
		switch ListFormat(explicit) {
		case ListFormatTSV, ListFormatTable, ListFormatJSON:
			return ListFormat(explicit), nil
		default:
			return "", fmt.Errorf("unknown list format %q", explicit)
		}
	}
	return ListFormatTable, nil
}

func isTerminal(w io.Writer) bool {
	f, ok := w.(*os.File)
	if !ok {
		return false
	}
	return term.IsTerminal(int(f.Fd()))
}

func terminalWidth(w io.Writer) (int, error) {
	f, ok := w.(*os.File)
	if !ok {
		return 0, fmt.Errorf("writer is not a terminal file")
	}

	width, _, err := term.GetSize(int(f.Fd()))
	if err != nil {
		return 0, fmt.Errorf("get terminal size: %w", err)
	}

	return width, nil
}

func truncate(value string, width int) string {
	if width <= 0 {
		return ""
	}

	runes := []rune(value)

	if len(runes) <= width {
		return value
	}

	const marker = "..."
	markerWidth := len([]rune(marker))

	if width <= markerWidth {
		return marker[:width]
	}

	return string(runes[:width-markerWidth]) + marker
}

func fitCell(value string, width int) string {
	value = truncate(value, width)
	padding := width - len([]rune(value))
	return value + strings.Repeat(" ", padding)
}

func writeListTable(w io.Writer, bookmarkList []bookmarks.Bookmark, opts ListFormatOptions) error {
	header := []string{
		"ID",
		"Title",
		"URL",
	}
	if opts.Long {
		header = []string{
			"Title",
			"URL",
			"Notes",
			"Source",
			"CreatedAt",
			"UpdatedAt",
			"NormalizedURL",
			"ID",
		}
	}

	rows := make([][]string, 0, len(bookmarkList))
	for _, bookmark := range bookmarkList {
		row := []string{
			bookmark.ID,
			bookmark.Title,
			bookmark.URL,
		}
		if opts.Long {
			row = []string{
				bookmark.Title,
				bookmark.URL,
				bookmark.Notes,
				bookmark.Source,
				bookmark.CreatedAt.Format(time.RFC3339),
				bookmark.UpdatedAt.Format(time.RFC3339),
				bookmark.NormalizedURL,
				bookmark.ID,
			}
		}
		rows = append(rows, row)
	}

	separator := "  "
	if opts.Width > 0 {
		maxSeparatorWidth := opts.Width / (len(header) - 1)
		if maxSeparatorWidth < len(separator) {
			separator = strings.Repeat(" ", maxSeparatorWidth)
		}
	}

	widths := tableColumnWidths(append([][]string{header}, rows...), opts.Width, separator)

	if err := writeRow(w, header, widths, separator); err != nil {
		return err
	}

	for _, row := range rows {
		if err := writeRow(w, row, widths, separator); err != nil {
			return err
		}
	}

	return nil
}

func tableColumnWidths(rows [][]string, totalWidth int, separator string) []int {
	if len(rows) == 0 {
		return nil
	}

	columnCount := len(rows[0])
	widths := make([]int, columnCount)
	if totalWidth <= 0 {
		for _, row := range rows {
			for i, value := range row {
				if width := len([]rune(value)); width > widths[i] {
					widths[i] = width
				}
			}
		}
		return widths
	}

	contentWidth := totalWidth - len(separator)*(columnCount-1)
	if contentWidth < 0 {
		contentWidth = 0
	}
	width := contentWidth / columnCount
	for i := range widths {
		widths[i] = width
	}
	for i := 0; i < contentWidth%columnCount; i++ {
		widths[i]++
	}

	return widths
}

func writeRow(w io.Writer, row []string, widths []int, separator string) error {
	cells := make([]string, len(row))

	for i, value := range row {
		if i == len(row)-1 {
			cells[i] = truncate(value, widths[i])
			continue
		}

		cells[i] = fitCell(value, widths[i])
	}

	_, err := fmt.Fprintln(w, strings.Join(cells, separator))
	return err
}

func writeListTSV(w io.Writer, bookmarkList []bookmarks.Bookmark) error {
	for _, bookmark := range bookmarkList {
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\n", bookmark.Title, bookmark.URL, bookmark.Notes, bookmark.Source, bookmark.CreatedAt, bookmark.UpdatedAt, bookmark.NormalizedURL, bookmark.ID)
	}
	return nil
}

func writeListJSON(w io.Writer, bookmarkList []bookmarks.Bookmark) error {
	if bookmarkList == nil {
		bookmarkList = []bookmarks.Bookmark{}
	}
	if err := json.NewEncoder(w).Encode(bookmarkList); err != nil {
		return fmt.Errorf("encode json: %w", err)
	}
	return nil
}
