package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"bookmarks/internal/bookmarks"
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

	if err := run(ctx, cfg, logger); err != nil {
		logger.Fatal(err)
	}
}

func run(ctx context.Context, cfg config, logger *log.Logger) error {
	if err := os.MkdirAll(filepath.Dir(cfg.DBPath), 0o755); err != nil {
		return err
	}

	store, err := bookmarks.OpenSQLStore(cfg.DBPath)
	if err != nil {
		return err
	}
	defer store.Close()

	app := server.New(server.Config{
		Store: store,
		Token: cfg.Token,
	})

	srv := newHTTPServer(cfg, app.Handler())
	return serveHTTP(ctx, srv, cfg.Addr, logger)
}
