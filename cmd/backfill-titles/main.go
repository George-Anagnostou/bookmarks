// Command backfill-titles is a temporary one-off tool that fills in titles for
// bookmarks saved without one. It reuses the server's SSRF-safe fetcher and
// only writes titles that are still blank. Delete this directory once the
// backfill is confirmed.
//
// Run it on the VPS as the service user, after taking a backup. Try a small
// dry run first:
//
//	sudo -u bookmarks backfill-titles -db /var/lib/bookmarks/data/bookmarks.db -dry-run -limit 5
//	sudo -u bookmarks backfill-titles -db /var/lib/bookmarks/data/bookmarks.db
//
// Each bookmark prints one tab-separated line to stdout:
// status, id, url, and title or error. Status is one of filled, dry-run,
// empty, skipped, or error. A summary goes to stderr. Fetch failures are
// reported but do not fail the run; rerunning retries them.
package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"bookmarks/internal/bookmarks"
	"bookmarks/internal/fetcher"
	"bookmarks/internal/sqlite"
)

type store interface {
	ListBookmarks(ctx context.Context, options bookmarks.ListOptions) ([]bookmarks.Bookmark, error)
	SetBookmarkTitleIfBlank(ctx context.Context, id, title string) (bool, error)
}

type titleFetcher interface {
	FetchTitle(ctx context.Context, url string) (string, error)
}

type options struct {
	dryRun bool
	limit  int
	delay  time.Duration
}

type counts struct {
	filled, dryRun, empty, skipped, failed int
}

func main() {
	log.SetFlags(0)
	log.SetPrefix("backfill-titles: ")

	dbPath := flag.String("db", envOr("BOOKMARKS_DBPATH", "data/bookmarks.db"), "SQLite database path (default from BOOKMARKS_DBPATH)")
	dryRun := flag.Bool("dry-run", false, "fetch and print titles without writing them")
	limit := flag.Int("limit", 0, "maximum number of untitled bookmarks to process (0 = all)")
	delay := flag.Duration("delay", 500*time.Millisecond, "pause between fetches")
	flag.Parse()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	st, err := sqlite.Open(ctx, *dbPath)
	if err != nil {
		log.Fatal(err)
	}
	defer st.Close()

	opts := options{dryRun: *dryRun, limit: *limit, delay: *delay}
	c, err := run(ctx, st, fetcher.NewFetcher(fetcher.Config{}), opts, os.Stdout)
	log.Printf("filled=%d dry-run=%d empty=%d skipped=%d error=%d",
		c.filled, c.dryRun, c.empty, c.skipped, c.failed)
	if err != nil {
		st.Close()
		log.Fatal(err)
	}
}

func envOr(key, fallback string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v
	}
	return fallback
}

// run fetches and stores titles for untitled bookmarks. It returns an error
// only for store failures or cancellation, not for individual fetch failures.
func run(ctx context.Context, st store, f titleFetcher, opts options, w io.Writer) (counts, error) {
	var c counts

	all, err := st.ListBookmarks(ctx, bookmarks.ListOptions{})
	if err != nil {
		return c, err
	}

	var untitled []bookmarks.Bookmark
	for _, b := range all {
		if strings.TrimSpace(b.Title) == "" {
			untitled = append(untitled, b)
		}
	}
	if opts.limit > 0 && len(untitled) > opts.limit {
		untitled = untitled[:opts.limit]
	}

	for i, b := range untitled {
		if i > 0 && opts.delay > 0 {
			select {
			case <-ctx.Done():
				return c, ctx.Err()
			case <-time.After(opts.delay):
			}
		}
		if err := ctx.Err(); err != nil {
			return c, err
		}

		title, err := f.FetchTitle(ctx, b.URL)
		switch {
		case err != nil:
			c.failed++
			fmt.Fprintf(w, "error\t%s\t%s\t%v\n", b.ID, b.URL, err)
			continue
		case strings.TrimSpace(title) == "":
			c.empty++
			fmt.Fprintf(w, "empty\t%s\t%s\t\n", b.ID, b.URL)
			continue
		case opts.dryRun:
			c.dryRun++
			fmt.Fprintf(w, "dry-run\t%s\t%s\t%s\n", b.ID, b.URL, clean(title))
			continue
		}

		ok, err := st.SetBookmarkTitleIfBlank(ctx, b.ID, title)
		if err != nil {
			return c, err
		}
		if !ok {
			// Titled or deleted since the list was read.
			c.skipped++
			fmt.Fprintf(w, "skipped\t%s\t%s\t\n", b.ID, b.URL)
			continue
		}
		c.filled++
		fmt.Fprintf(w, "filled\t%s\t%s\t%s\n", b.ID, b.URL, clean(title))
	}

	return c, nil
}

// clean keeps a title on one TSV field.
func clean(s string) string {
	return strings.Join(strings.Fields(s), " ")
}
