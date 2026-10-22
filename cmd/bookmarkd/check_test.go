package main

import (
	"bytes"
	"context"
	"encoding/json"
	"path/filepath"
	"testing"

	"bookmarks/internal/sqlite"
)

func TestRunCheckReportsCurrentDatabase(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "bookmarks.db")
	store, err := sqlite.Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}

	var out bytes.Buffer
	if err := runCheck(ctx, []string{path}, &out); err != nil {
		t.Fatalf("runCheck() error = %v\n%s", err, out.String())
	}

	var report sqlite.CheckReport
	if err := json.Unmarshal(out.Bytes(), &report); err != nil {
		t.Fatalf("decode report: %v\n%s", err, out.String())
	}
	if !report.OK() {
		t.Fatalf("report = %+v, want OK", report)
	}
}

func TestRunCheckRejectsBadInput(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "missing.db")
	for _, args := range [][]string{nil, {"a.db", "b.db"}, {missing}} {
		var out bytes.Buffer
		if err := runCheck(context.Background(), args, &out); err == nil {
			t.Errorf("runCheck(%q) error = nil, want error", args)
		}
	}
}
