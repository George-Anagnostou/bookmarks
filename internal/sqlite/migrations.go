package sqlite

import (
	"context"
	"database/sql"
	"fmt"
)

var migrations = []string{
	schemaV1,
}

// migrate applies the package's registered migrations.
func migrate(ctx context.Context, db *sql.DB) error {
	return migrateWith(ctx, db, migrations)
}

// migrateWith applies migrationList transactionally. It accepts an explicit
// list so migration behavior can be tested without changing package state.
func migrateWith(ctx context.Context, db *sql.DB, migrationList []string) error {
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
		nextVersion := i + 1

		if _, err := tx.ExecContext(ctx, migrationList[i]); err != nil {
			return fmt.Errorf("apply migration %d: %w", nextVersion, err)
		}

		statement := fmt.Sprintf("PRAGMA user_version = %d", nextVersion)
		if _, err := tx.ExecContext(ctx, statement); err != nil {
			return fmt.Errorf("record migration %d: %w", nextVersion, err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit migrations: %w", err)
	}

	return nil
}
