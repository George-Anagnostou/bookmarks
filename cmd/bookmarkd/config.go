package main

import (
	"errors"
	"net/http"
	"strings"
	"time"
)

type config struct {
	Addr   string
	DBPath string
	Token  string
}

func loadConfig(lookup func(string) (string, bool)) (config, error) {
	cfg := config{
		Addr:   "127.0.0.1:8080",
		DBPath: "data/bookmarks.db",
	}

	if value, ok := lookup("BOOKMARKS_ADDR"); ok {
		cfg.Addr = strings.TrimSpace(value)
	}

	if value, ok := lookup("BOOKMARKS_DBPATH"); ok {
		cfg.DBPath = strings.TrimSpace(value)
	}

	if value, ok := lookup("BOOKMARKS_TOKEN"); ok {
		cfg.Token = strings.TrimSpace(value)
	}

	if cfg.Addr == "" {
		return config{}, errors.New("BOOKMARKS_ADDR must be set")
	}

	if cfg.DBPath == "" {
		return config{}, errors.New("BOOKMARKS_DBPATH must be set")
	}

	if cfg.Token == "" {
		return config{}, errors.New("BOOKMARKS_TOKEN must be set")
	}

	return cfg, nil
}

func newHTTPServer(cfg config, handler http.Handler) *http.Server {
	return &http.Server{
		Addr:              cfg.Addr,
		Handler:           handler,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      10 * time.Second,
		IdleTimeout:       60 * time.Second,
	}
}
