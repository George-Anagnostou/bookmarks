package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"strings"
	"testing"
)

func TestCheckAcceptsLatestSchemaAndReportsBookmarkCount(t *testing.T) {
	path := createCurrentSchemaDatabase(t)
	db := openMigrationTestDBAt(t, path)
	if _, err := db.Exec(`
		INSERT INTO bookmarks (
			id, url, normalized_url, title, notes, source, created_at, updated_at
		) VALUES ('one', 'https://one.test', 'https://one.test/', '', '', '', '2026-01-01T00:00:00Z', '2026-01-01T00:00:00Z'),
		         ('two', 'https://two.test', 'https://two.test/', '', '', '', '2026-01-02T00:00:00Z', '2026-01-02T00:00:00Z')
	`); err != nil {
		t.Fatalf("insert fixture bookmarks: %v", err)
	}
	if err := db.Close(); err != nil {
		t.Fatalf("close fixture database: %v", err)
	}

	report, err := Check(context.Background(), path)
	if err != nil {
		t.Fatalf("Check() error = %v", err)
	}
	if !report.OK() {
		t.Fatalf("Check() report = %+v, want OK", report)
	}
	if report.SchemaVersion != len(migrations) {
		t.Errorf("SchemaVersion = %d, want %d", report.SchemaVersion, len(migrations))
	}
	if report.BookmarkCount != 2 {
		t.Errorf("BookmarkCount = %d, want 2", report.BookmarkCount)
	}
}

func TestCheckReportsSchemaVersionMismatch(t *testing.T) {
	path := createCurrentSchemaDatabase(t)
	db := openMigrationTestDBAt(t, path)
	if _, err := db.Exec("PRAGMA user_version = 0"); err != nil {
		t.Fatalf("set schema version: %v", err)
	}
	if err := db.Close(); err != nil {
		t.Fatalf("close fixture database: %v", err)
	}

	report, err := Check(context.Background(), path)
	if err != nil {
		t.Fatalf("Check() error = %v", err)
	}
	if report.OK() {
		t.Fatal("Check() report is OK for an outdated schema version")
	}
	if report.SchemaVersion != 0 {
		t.Errorf("SchemaVersion = %d, want 0", report.SchemaVersion)
	}
	if report.ExpectedVersion != len(migrations) {
		t.Errorf("ExpectedVersion = %d, want %d", report.ExpectedVersion, len(migrations))
	}
}

func TestCheckReportsMissingIndex(t *testing.T) {
	path := createCurrentSchemaDatabase(t)
	db := openMigrationTestDBAt(t, path)
	if _, err := db.Exec("DROP INDEX bookmarks_created_at_idx"); err != nil {
		t.Fatalf("drop expected index: %v", err)
	}
	if err := db.Close(); err != nil {
		t.Fatalf("close fixture database: %v", err)
	}

	report, err := Check(context.Background(), path)
	if err != nil {
		t.Fatalf("Check() error = %v", err)
	}
	if report.OK() {
		t.Fatal("Check() report is OK after expected index was removed")
	}
	if !containsDifference(report.SchemaDifferences, "bookmarks_created_at_idx") {
		t.Errorf("SchemaDifferences = %v, want a difference naming bookmarks_created_at_idx", report.SchemaDifferences)
	}
}

func TestCheckReportsColumnTypeMismatch(t *testing.T) {
	path := filepath.Join(t.TempDir(), "wrong-column-type.db")
	db := openMigrationTestDBAt(t, path)
	_, err := db.Exec(`
		CREATE TABLE bookmarks (
			id TEXT PRIMARY KEY,
			url INTEGER NOT NULL,
			normalized_url TEXT NOT NULL UNIQUE,
			title TEXT NOT NULL DEFAULT '',
			notes TEXT NOT NULL DEFAULT '',
			source TEXT NOT NULL DEFAULT '',
			created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL,
			archived_at TEXT,
			read_at TEXT
		) STRICT;
		CREATE INDEX bookmarks_created_at_idx ON bookmarks(created_at DESC);
		PRAGMA user_version = 1;
	`)
	if err != nil {
		t.Fatalf("create wrong-schema fixture: %v", err)
	}
	if err := db.Close(); err != nil {
		t.Fatalf("close fixture database: %v", err)
	}

	report, err := Check(context.Background(), path)
	if err != nil {
		t.Fatalf("Check() error = %v", err)
	}
	if report.OK() {
		t.Fatal("Check() report is OK when bookmarks.url has the wrong type")
	}
	if !containsDifference(report.SchemaDifferences, "url") {
		t.Errorf("SchemaDifferences = %v, want a difference naming url", report.SchemaDifferences)
	}
}

func TestCheckReportsUnexpectedTable(t *testing.T) {
	path := createCurrentSchemaDatabase(t)
	db := openMigrationTestDBAt(t, path)
	if _, err := db.Exec("CREATE TABLE unexpected (id INTEGER PRIMARY KEY)"); err != nil {
		t.Fatalf("create unexpected table: %v", err)
	}
	if err := db.Close(); err != nil {
		t.Fatalf("close fixture database: %v", err)
	}

	report, err := Check(context.Background(), path)
	if err != nil {
		t.Fatalf("Check() error = %v", err)
	}
	if report.OK() {
		t.Fatal("Check() report is OK with an unexpected table")
	}
	if !containsDifference(report.SchemaDifferences, "unexpected") {
		t.Errorf("SchemaDifferences = %v, want a difference naming unexpected table", report.SchemaDifferences)
	}
}

func TestCheckDoesNotModifyDatabase(t *testing.T) {
	path := createCurrentSchemaDatabase(t)
	beforeDB := openMigrationTestDBAt(t, path)
	beforeVersion := schemaVersion(t, beforeDB)
	if err := beforeDB.Close(); err != nil {
		t.Fatalf("close fixture database: %v", err)
	}

	if _, err := Check(context.Background(), path); err != nil {
		t.Fatalf("Check() error = %v", err)
	}

	afterDB := openMigrationTestDBAt(t, path)
	defer afterDB.Close()
	if got := schemaVersion(t, afterDB); got != beforeVersion {
		t.Fatalf("Check() changed schema version to %d, want unchanged %d", got, beforeVersion)
	}
}

func TestCheckReturnsErrorForMissingDatabase(t *testing.T) {
	path := filepath.Join(t.TempDir(), "missing.db")
	if _, err := Check(context.Background(), path); err == nil || errors.Is(err, ErrCheckNotImplemented) {
		t.Fatalf("Check() error = %v, want an error opening missing database", err)
	}
}

func createCurrentSchemaDatabase(t *testing.T) string {
	t.Helper()

	path := filepath.Join(t.TempDir(), "check-test.db")
	db := openMigrationTestDBAt(t, path)
	if err := migrate(context.Background(), db); err != nil {
		t.Fatalf("migrate fixture database: %v", err)
	}
	if err := db.Close(); err != nil {
		t.Fatalf("close fixture database: %v", err)
	}
	return path
}

func openMigrationTestDBAt(t *testing.T, path string) *sql.DB {
	t.Helper()

	absPath, err := filepath.Abs(path)
	if err != nil {
		t.Fatalf("resolve test database path: %v", err)
	}
	db, err := sql.Open("sqlite", sqliteDSN(absPath))
	if err != nil {
		t.Fatalf("open test database: %v", err)
	}
	db.SetMaxOpenConns(1)
	t.Cleanup(func() {
		if err := db.Close(); err != nil {
			t.Errorf("close test database: %v", err)
		}
	})
	return db
}

func containsDifference(differences []string, part string) bool {
	for _, difference := range differences {
		if strings.Contains(difference, part) {
			return true
		}
	}
	return false
}
