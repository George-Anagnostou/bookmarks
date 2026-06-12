package main

import (
	"net/http"
	"testing"
)

func TestLoadConfigUsesDefaults(t *testing.T) {
	cfg, err := loadConfig(mapLookup(map[string]string{
		"BOOKMARKS_TOKEN": "test-token",
	}))
	if err != nil {
		t.Fatal(err)
	}

	if cfg.Addr != "127.0.0.1:8080" {
		t.Fatalf("Addr = %q", cfg.Addr)
	}

	if cfg.DBPath != "data/bookmarks.db" {
		t.Fatalf("DBPath = %q", cfg.DBPath)
	}

	if cfg.Token != "test-token" {
		t.Fatalf("Token = %q", cfg.Token)
	}
}

func TestLoadConfigUsesOverrides(t *testing.T) {
	cfg, err := loadConfig(mapLookup(map[string]string{
		"BOOKMARKS_ADDR":   "127.0.0.1:9999",
		"BOOKMARKS_DBPATH": "/tmp/bookmarks.db",
		"BOOKMARKS_TOKEN":  "test-token",
	}))
	if err != nil {
		t.Fatal(err)
	}

	if cfg.Addr != "127.0.0.1:9999" {
		t.Fatalf("Addr = %q", cfg.Addr)
	}
	if cfg.DBPath != "/tmp/bookmarks.db" {
		t.Fatalf("DBPath = %q", cfg.DBPath)
	}
}

func TestLoadConfigRequiresToken(t *testing.T) {
	_, err := loadConfig(mapLookup(nil))
	if err == nil {
		t.Fatal("expected error")
	}
}

func mapLookup(values map[string]string) func(string) (string, bool) {
	return func(key string) (string, bool) {
		value, ok := values[key]
		return value, ok
	}
}

func TestLoadConfigRejectsBlankToken(t *testing.T) {
	_, err := loadConfig(mapLookup(map[string]string{
		"BOOKMARKS_TOKEN": " ",
	}))
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestNewHTTPServer(t *testing.T) {
	cfg := config{Addr: "127.0.0.1:9999", Token: "test-token"}
	handler := http.NewServeMux()

	srv := newHTTPServer(cfg, handler)

	if srv.Addr != cfg.Addr {
		t.Fatalf("Addr = %q", srv.Addr)
	}

	if srv.Handler != handler {
		t.Fatalf("Handler was not set")
	}

	if srv.ReadHeaderTimeout == 0 {
		t.Fatalf("ReadHeaderTimeout was not set")
	}

	if srv.ReadTimeout == 0 {
		t.Fatal("ReadTimeout was not set")
	}

	if srv.WriteTimeout == 0 {
		t.Fatal("WriteTimeout was not set")
	}

	if srv.IdleTimeout == 0 {
		t.Fatal("IdleTimeout was not set")
	}
}
