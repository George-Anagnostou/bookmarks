package main

import (
	"context"
	"errors"
	"io"
	"log"
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
	wantErr := errors.New("listen failed")

	srv := fakeHTTPServer{
		listenAndServe: func() error {
			return wantErr
		},
		shutdown: func(ctx context.Context) error {
			t.Fatal("Shutdown should not be called")
			return nil
		},
	}

	err := serveHTTP(context.Background(), srv, "127.0.0.1:8080", testLogger())
	if !errors.Is(err, wantErr) {
		t.Fatalf("err = %v, want %v", err, wantErr)
	}
}

func TestServeHTTPIgnoresServerClosed(t *testing.T) {
	srv := fakeHTTPServer{
		listenAndServe: func() error {
			return http.ErrServerClosed
		},
		shutdown: func(ctx context.Context) error {
			t.Fatal("Shutdown should not be called")
			return nil
		},
	}

	if err := serveHTTP(context.Background(), srv, "127.0.0.1:8080", testLogger()); err != nil {
		t.Fatalf("err = %v, want nil", err)
	}
}

func TestServeHTTPShutsDownWhenContextCanceled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())

	listening := make(chan struct{})
	shutdownCalled := make(chan struct{})

	srv := fakeHTTPServer{
		listenAndServe: func() error {
			close(listening)
			<-shutdownCalled
			return http.ErrServerClosed
		},
		shutdown: func(ctx context.Context) error {
			close(shutdownCalled)
			return nil
		},
	}

	errc := make(chan error, 1)
	go func() {
		errc <- serveHTTP(ctx, srv, "127.0.0.1:8080", testLogger())
	}()

	<-listening
	cancel()

	if err := <-errc; err != nil {
		t.Fatalf("err = %v, want nil", err)
	}
}

func TestServeHTTPReturnsShutdownError(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	wantErr := errors.New("shutdown failed")

	listening := make(chan struct{})
	releaseServer := make(chan struct{})

	srv := fakeHTTPServer{
		listenAndServe: func() error {
			close(listening)
			<-releaseServer
			return http.ErrServerClosed
		},
		shutdown: func(ctx context.Context) error {
			close(releaseServer)
			return wantErr
		},
	}

	errc := make(chan error, 1)
	go func() {
		errc <- serveHTTP(ctx, srv, "127.0.0.1:8080", testLogger())
	}()

	<-listening
	cancel()

	if err := <-errc; !errors.Is(err, wantErr) {
		t.Fatalf("err = %v, want %v", err, wantErr)
	}
}

func testLogger() *log.Logger {
	return log.New(io.Discard, "", 0)
}
