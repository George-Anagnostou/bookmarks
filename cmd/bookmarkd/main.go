package main

import (
	"log"
	"os"
	"path/filepath"

	"bookmarks/internal/bookmarks"
	"bookmarks/internal/server"
)

func main() {
	cfg, err := loadConfig(os.LookupEnv)
	if err != nil {
		log.Fatal(err)
	}

	if err := os.MkdirAll(filepath.Dir(cfg.DBPath), 0o755); err != nil {
		log.Fatal(err)
	}

	store, err := bookmarks.OpenSQLStore(cfg.DBPath)
	if err != nil {
		log.Fatal(err)
	}
	defer store.Close()

	app := server.New(server.Config{
		Store: store,
		Token: cfg.Token,
	})

	srv := newHTTPServer(cfg, app.Handler())

	log.Printf("listening on %s", cfg.Addr)
	if err := srv.ListenAndServe(); err != nil {
		log.Fatal(err)
	}
}
