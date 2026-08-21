package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"bookmarks/internal/bookmarks"
	"bookmarks/internal/fetcher"
	"bookmarks/internal/server"
)

func main() {
	cfg, err := loadConfig(os.LookupEnv)
	if err != nil {
		log.Fatal(err)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	logger := log.New(os.Stdout, "", log.LstdFlags)

	pageFetcher := fetcher.NewFetcher(fetcher.Config{})
	if err := run(ctx, cfg, logger, pageFetcher); err != nil {
		logger.Fatal(err)
	}
}

func run(ctx context.Context, cfg config, logger *log.Logger, pageFetcher server.Fetcher) error {
	if err := os.MkdirAll(filepath.Dir(cfg.DBPath), 0o755); err != nil {
		return err
	}

	store, err := bookmarks.OpenSQLStore(cfg.DBPath)
	if err != nil {
		return err
	}
	defer store.Close()

	app := server.NewServer(server.Config{
		Store:   store,
		Token:   cfg.Token,
		Fetcher: pageFetcher,
	})

	srv := newHTTPServer(cfg, app.Handler())
	return serveHTTP(ctx, srv, cfg.Addr, logger)
}
