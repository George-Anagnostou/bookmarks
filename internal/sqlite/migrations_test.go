package sqlite

import (
	"context"
	"database/sql"
	"errors"
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

	testMigrations := []migration{
		{Version: 1, Name: "create bookmarks", SQL: schemaV1},
		{Version: 2, Name: "failing probe", SQL: `CREATE TABLE rollback_probe (id INTEGER PRIMARY KEY);
		 INVALID SQL;`},
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

func TestMigrateIdentifiesFailureAndCanRetry(t *testing.T) {
	db := openMigrationTestDB(t)
	ctx := context.Background()
	if err := migrate(ctx, db); err != nil {
		t.Fatalf("initial migrate() error = %v", err)
	}

	failedMigrations := []migration{
		migrations[0],
		{
			Version: 2,
			Name:    "add recovery probe",
			SQL:     "CREATE TABLE recovery_probe (id INTEGER PRIMARY KEY); INVALID SQL;",
		},
	}
	err := migrateWith(ctx, db, failedMigrations)
	if err == nil {
		t.Fatal("migrateWith() error = nil, want migration failure")
	}

	var migrationErr *MigrationError
	if !errors.As(err, &migrationErr) {
		t.Fatalf("migrateWith() error type = %T, want *MigrationError", err)
	}
	if migrationErr.Version != 2 || migrationErr.Name != "add recovery probe" || migrationErr.Stage != "apply" {
		t.Errorf("MigrationError = %+v, want version 2, name add recovery probe, stage apply", migrationErr)
	}
	if migrationErr.Unwrap() == nil {
		t.Fatal("MigrationError does not retain its underlying database error")
	}

	if got := schemaVersion(t, db); got != 1 {
		t.Fatalf("schema version after failed migration = %d, want 1", got)
	}
	if sqliteTableExists(t, db, "recovery_probe") {
		t.Fatal("recovery_probe exists after failed migration; transaction was not rolled back")
	}

	retryMigrations := []migration{
		migrations[0],
		{
			Version: 2,
			Name:    "add recovery probe",
			SQL:     "CREATE TABLE recovery_probe (id INTEGER PRIMARY KEY);",
		},
	}
	if err := migrateWith(ctx, db, retryMigrations); err != nil {
		t.Fatalf("retry migrateWith() error = %v", err)
	}
	if got := schemaVersion(t, db); got != 2 {
		t.Errorf("schema version after retry = %d, want 2", got)
	}
	if !sqliteTableExists(t, db, "recovery_probe") {
		t.Error("recovery_probe does not exist after successful retry")
	}
}

func TestMigrateRejectsInvalidMigrationMetadata(t *testing.T) {
	tests := []struct {
		name       string
		migrations []migration
	}{
		{
			name: "non-contiguous version",
			migrations: []migration{
				{Version: 2, Name: "create bookmarks", SQL: schemaV1},
			},
		},
		{
			name: "empty name",
			migrations: []migration{
				{Version: 1, SQL: schemaV1},
			},
		},
		{
			name: "empty SQL",
			migrations: []migration{
				{Version: 1, Name: "create bookmarks"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db := openMigrationTestDB(t)
			if err := migrateWith(context.Background(), db, tt.migrations); err == nil {
				t.Fatal("migrateWith() error = nil, want invalid migration metadata error")
			}
			if got := schemaVersion(t, db); got != 0 {
				t.Errorf("schema version = %d, want 0", got)
			}
		})
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
