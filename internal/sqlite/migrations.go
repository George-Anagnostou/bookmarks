package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
)

type migration struct {
	Version int
	Name    string
	SQL     string
}

var migrations = []migration{
	{Version: 1, Name: "create bookmarks", SQL: schemaV1},
}

// MigrationError identifies the migration stage that failed and preserves the
// underlying database error for inspection with errors.Is or errors.As.
type MigrationError struct {
	Version int
	Name    string
	Stage   string
	Err     error
}

func (e *MigrationError) Error() string {
	return fmt.Sprintf("migration %d (%s) %s: %v", e.Version, e.Name, e.Stage, e.Err)
}

func (e *MigrationError) Unwrap() error {
	return e.Err
}

// migrate applies the package's registered migrations.
func migrate(ctx context.Context, db *sql.DB) error {
	return migrateWith(ctx, db, migrations)
}

// migrateWith applies migrationList transactionally. It accepts an explicit
// list so migration behavior can be tested without changing package state.
func migrateWith(ctx context.Context, db *sql.DB, migrationList []migration) error {
	if err := validateMigrations(migrationList); err != nil {
		return fmt.Errorf("validate migrations: %w", err)
	}

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin migration transaction: %w", err)
	}
	defer tx.Rollback()

	var version int
	if err := tx.QueryRowContext(ctx, "PRAGMA user_version").Scan(&version); err != nil {
		return fmt.Errorf("read schema version: %w", err)
	}

	if version < 0 || version > len(migrationList) {
		return fmt.Errorf("validate schema version: %d", version)
	}

	for i := version; i < len(migrationList); i++ {
		current := migrationList[i]

		if _, err := tx.ExecContext(ctx, current.SQL); err != nil {
			return &MigrationError{
				Version: current.Version,
				Name:    current.Name,
				Stage:   "apply",
				Err:     err,
			}
		}

		statement := fmt.Sprintf("PRAGMA user_version = %d", current.Version)
		if _, err := tx.ExecContext(ctx, statement); err != nil {
			return &MigrationError{
				Version: current.Version,
				Name:    current.Name,
				Stage:   "record version",
				Err:     err,
			}
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit migrations: %w", err)
	}

	return nil
}

func validateMigrations(migrationList []migration) error {
	for i, current := range migrationList {
		wantVersion := i + 1
		if current.Version != wantVersion {
			return fmt.Errorf("migration at position %d has version %d, want %d", i, current.Version, wantVersion)
		}
		if strings.TrimSpace(current.Name) == "" {
			return fmt.Errorf("migration %d has an empty name", current.Version)
		}
		if strings.TrimSpace(current.SQL) == "" {
			return fmt.Errorf("migration %d (%s) has empty SQL", current.Version, current.Name)
		}
	}
	return nil
}
