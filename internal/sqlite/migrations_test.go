package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"path/filepath"
	"testing"
)

func TestMigrateFreshDatabase(t *testing.T) {
	db := openMigrationTestDB(t)

	if err := migrate(context.Background(), db); err != nil {
		t.Fatalf("migrate() error = %v", err)
	}

	if got := schemaVersion(t, db); got != len(migrations) {
		t.Fatalf("schema version = %d, want %d", got, len(migrations))
	}
	if !sqliteTableExists(t, db, "bookmarks") {
		t.Fatal("migration did not create bookmarks table")
	}
}

func TestMigrateAlreadyCurrentDatabase(t *testing.T) {
	db := openMigrationTestDB(t)

	if err := migrate(context.Background(), db); err != nil {
		t.Fatalf("first migrate() error = %v", err)
	}

	if err := migrate(context.Background(), db); err != nil {
		t.Fatalf("second migrate() error = %v", err)
	}

	if got := schemaVersion(t, db); got != len(migrations) {
		t.Fatalf("schema version = %d, want %d", got, len(migrations))
	}
}

func TestMigrateRejectsNewerSchemaVersion(t *testing.T) {
	db := openMigrationTestDB(t)

	newerVersion := len(migrations) + 1
	if _, err := db.Exec(fmt.Sprintf("PRAGMA user_version = %d", newerVersion)); err != nil {
		t.Fatalf("set schema version: %v", err)
	}

	if err := migrate(context.Background(), db); err == nil {
		t.Fatal("migrate() error = nil, want error for newer schema version")
	}

	if got := schemaVersion(t, db); got != newerVersion {
		t.Fatalf("schema version = %d, want unchanged version %d", got, newerVersion)
	}
}

func TestMigrateRollsBackFailedMigration(t *testing.T) {
	db := openMigrationTestDB(t)
	ctx := context.Background()

	// Start from a successfully applied version 1, as if this database were
	// being upgraded by a later release.
	if err := migrate(ctx, db); err != nil {
		t.Fatalf("initial migrate() error = %v", err)
	}

	testMigrations := []string{
		schemaV1,
		`CREATE TABLE rollback_probe (id INTEGER PRIMARY KEY);
		 INVALID SQL;`,
	}
	if err := migrateWith(ctx, db, testMigrations); err == nil {
		t.Fatal("migrateWith() error = nil, want migration failure")
	}

	if got := schemaVersion(t, db); got != 1 {
		t.Fatalf("schema version = %d, want 1 after rollback", got)
	}
	if sqliteTableExists(t, db, "rollback_probe") {
		t.Fatal("rollback_probe exists after failed migration")
	}
}

func openMigrationTestDB(t *testing.T) *sql.DB {
	t.Helper()

	path := filepath.Join(t.TempDir(), "migration-test.db")
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

func schemaVersion(t *testing.T, db *sql.DB) int {
	t.Helper()

	var version int
	if err := db.QueryRow("PRAGMA user_version").Scan(&version); err != nil {
		t.Fatalf("read schema version: %v", err)
	}
	return version
}
