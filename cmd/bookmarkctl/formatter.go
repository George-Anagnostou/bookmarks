package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"text/tabwriter"

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
	if isTTY(stdout) {
		return ListFormatTable, nil
	}
	return ListFormatTSV, nil
}

func isTerminal(w io.Writer) bool {
	f, ok := w.(*os.File)
	if !ok {
		return false
	}
	return term.IsTerminal(int(f.Fd()))
}

func writeListTable(w io.Writer, bookmarkList []bookmarks.Bookmark, opts ListFormatOptions) error {
	writer := tabwriter.NewWriter(w, 0, 0, 1, ' ', 0)

	if opts.Long {
		fmt.Fprintln(writer, "Title\tURL\tNotes\tSource\tCreatedAt\tUpdatedAt\tNormalizedURL\tID")
		for _, bookmark := range bookmarkList {
			fmt.Fprintf(writer, "%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\n", bookmark.Title, bookmark.URL, bookmark.Notes, bookmark.Source, bookmark.CreatedAt, bookmark.UpdatedAt, bookmark.NormalizedURL, bookmark.ID)
		}
	} else {
		fmt.Fprintln(writer, "Title\tURL")
		for _, bookmark := range bookmarkList {
			fmt.Fprintf(writer, "%s\t%s\n", bookmark.Title, bookmark.URL)
		}
	}
	writer.Flush()
	return nil
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
