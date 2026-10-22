package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
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
	if err := migrateWith(ctx, db, migrations[:1]); err != nil {
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
	if err := migrateWith(ctx, db, migrations[:1]); err != nil {
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

func TestMigrateUpgradePreservesDataAndDoesNotReapply(t *testing.T) {
	db := openMigrationTestDB(t)
	ctx := context.Background()
	if err := migrateWith(ctx, db, migrations[:1]); err != nil {
		t.Fatal(err)
	}
	insertMigrationBookmark(t, db)

	upgrade := []migration{
		migrations[0],
		{Version: 2, Name: "add rating", SQL: `ALTER TABLE bookmarks ADD COLUMN rating INTEGER NOT NULL DEFAULT 0;`},
		{Version: 3, Name: "update title", SQL: `UPDATE bookmarks SET title = title || ' migrated';`},
	}
	for run := 1; run <= 2; run++ {
		if err := migrateWith(ctx, db, upgrade); err != nil {
			t.Fatalf("upgrade run %d: %v", run, err)
		}
		if got := schemaVersion(t, db); got != 3 {
			t.Fatalf("schema version = %d, want 3", got)
		}
		assertMigrationBookmark(t, db, "Original migrated")
		var rating int
		if err := db.QueryRow("SELECT rating FROM bookmarks WHERE id = 'one'").Scan(&rating); err != nil {
			t.Fatal(err)
		}
		if rating != 0 {
			t.Fatalf("rating = %d, want default 0", rating)
		}
	}
}

func TestMigrateRollsBackEntireUpgrade(t *testing.T) {
	db := openMigrationTestDB(t)
	ctx := context.Background()
	if err := migrateWith(ctx, db, migrations[:1]); err != nil {
		t.Fatal(err)
	}
	insertMigrationBookmark(t, db)

	upgrade := []migration{
		migrations[0],
		{Version: 2, Name: "change schema and data", SQL: `
			ALTER TABLE bookmarks ADD COLUMN rating INTEGER NOT NULL DEFAULT 0;
			UPDATE bookmarks SET title = 'Changed';
		`},
		{Version: 3, Name: "fail later", SQL: `INVALID SQL;`},
	}
	err := migrateWith(ctx, db, upgrade)
	var migrationErr *MigrationError
	if !errors.As(err, &migrationErr) || migrationErr.Version != 3 {
		t.Fatalf("upgrade error = %v, want failure at version 3", err)
	}
	if got := schemaVersion(t, db); got != 1 {
		t.Fatalf("schema version = %d, want 1", got)
	}
	assertMigrationBookmark(t, db, "Original")
	columns, err := bookmarkColumns(ctx, db)
	if err != nil {
		t.Fatal(err)
	}
	for _, column := range columns {
		if column.Name == "rating" {
			t.Fatal("rating column survived rollback of the upgrade")
		}
	}
}

func TestMigrateRejectsUnversionedSchema(t *testing.T) {
	for _, tt := range []struct {
		name   string
		schema string
	}{
		{name: "matching version 1", schema: schemaV1},
		{name: "incompatible bookmarks", schema: `CREATE TABLE bookmarks (title TEXT, created_at TEXT);`},
		{name: "unrelated table", schema: `CREATE TABLE other (id INTEGER PRIMARY KEY);`},
		{name: "view only", schema: `CREATE VIEW other AS SELECT 1;`},
	} {
		t.Run(tt.name, func(t *testing.T) {
			db := openMigrationTestDB(t)
			if _, err := db.Exec(tt.schema); err != nil {
				t.Fatal(err)
			}
			if tt.name == "matching version 1" {
				insertMigrationBookmark(t, db)
			}
			var before string
			if err := db.QueryRow("SELECT group_concat(sql, ';') FROM sqlite_schema").Scan(&before); err != nil {
				t.Fatal(err)
			}
			err := migrate(context.Background(), db)
			if err == nil || !strings.Contains(err.Error(), "schema version is 0") {
				t.Fatalf("migrate() error = %v, want unversioned-schema error", err)
			}
			if got := schemaVersion(t, db); got != 0 {
				t.Fatalf("schema version = %d, want 0", got)
			}
			var after string
			if err := db.QueryRow("SELECT group_concat(sql, ';') FROM sqlite_schema").Scan(&after); err != nil {
				t.Fatal(err)
			}
			if after != before {
				t.Fatalf("schema changed after rejection: before %q, after %q", before, after)
			}
			if tt.name == "matching version 1" {
				assertMigrationBookmark(t, db, "Original")
			}
		})
	}
}

func TestMigrateRejectsNegativeSchemaVersion(t *testing.T) {
	db := openMigrationTestDB(t)
	if _, err := db.Exec("PRAGMA user_version = -1"); err != nil {
		t.Fatal(err)
	}
	if err := migrate(context.Background(), db); err == nil {
		t.Fatal("migrate() succeeded with a negative schema version")
	}
	if got := schemaVersion(t, db); got != -1 {
		t.Fatalf("schema version = %d, want unchanged -1", got)
	}
	if sqliteTableExists(t, db, "bookmarks") {
		t.Fatal("migration ran despite invalid schema version")
	}
}

func insertMigrationBookmark(t *testing.T, db *sql.DB) {
	t.Helper()
	if _, err := db.Exec(`
		INSERT INTO bookmarks (id, url, normalized_url, title, notes, created_at, updated_at)
		VALUES ('one', 'https://example.com', 'https://example.com/', 'Original', 'Keep these notes',
		        '2026-01-01T00:00:00Z', '2026-01-01T00:00:00Z')
	`); err != nil {
		t.Fatal(err)
	}
}

func assertMigrationBookmark(t *testing.T, db *sql.DB, wantTitle string) {
	t.Helper()
	var count int
	if err := db.QueryRow("SELECT count(*) FROM bookmarks").Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("bookmark count = %d, want 1", count)
	}
	var title, url, notes string
	if err := db.QueryRow("SELECT title, url, notes FROM bookmarks WHERE id = 'one'").Scan(&title, &url, &notes); err != nil {
		t.Fatal(err)
	}
	if title != wantTitle || url != "https://example.com" || notes != "Keep these notes" {
		t.Fatalf("unexpected bookmark: title=%q url=%q notes=%q", title, url, notes)
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
