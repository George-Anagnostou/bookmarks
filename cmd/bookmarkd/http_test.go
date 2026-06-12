package main

import (
	"context"
	"net/http"
	"testing"
)

type fakeHTTPServer struct {
	listenAndServe func() error
	shutdown       func(context.Context) error
}

func (s fakeHTTPServer) ListenAndServe() error {
	return s.listenAndServe()
}

func (s fakeHTTPServer) Shutdown(ctx context.Context) error {
	return s.shutdown(ctx)
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

func TestServeHTTPReturnsListenError(t *testing.T) {
}

func TestServeHTTPIgnoresServerClosed(t *testing.T) {
}

func TestServeHTTPShutsDownWHenContextCanceled(t *testing.T) {
}

func TestServeHTTPReturnsShutdownError(t *testing.T) {
}
