# SQLite migrations

## Contract

- Migrations are SQL records in `migrations.go`, numbered consecutively from 1.
- Released migrations are immutable. Append a new migration for each change.
- `PRAGMA user_version` records the last applied migration. Version 0 means an
  empty database; existing unversioned schemas require explicit adoption.
- Startup applies all pending migrations in one transaction. Schema changes,
  data changes, and version updates commit together or roll back together.
- A current database has no migrations reapplied. Negative versions and versions
  newer than the binary supports are errors.
- The server starts listening only after migrations succeed. A migration error
  prevents startup and identifies the failed migration where applicable.
- Migrations are forward-only. Release rollback may require restoring a backup
  along with its compatible binary.
- Run one server process per database and one deployment at a time. Stop the
  server before backing up and upgrading. `SetMaxOpenConns(1)` only limits
  connections within a process; it is not a cross-process lock.

## Reading the implementation

`sqlite.Open` calls `migrate`, which passes the registry to `migrateWith`.
`migrateWith` validates the list, begins a transaction, reads and validates the
database version, executes pending SQL, records each version, and commits once.
The deferred rollback handles early returns; after a successful commit it is
harmless. All database work in this function uses `tx`, not `db`.

`MigrationError` adds the migration version, name, and failed stage while
preserving the database error through `Unwrap`.

## Adding a migration

1. Leave released SQL and migration records unchanged.
2. Add the next SQL constant and append a record with version, name, and SQL.
3. Prefer plain `CREATE TABLE` / `CREATE INDEX`: version tracking prevents reruns,
   and unexpected existing objects should cause an error. Version 1 retains its
   original `IF NOT EXISTS` statements; the version-0 guard prevents silently
   adopting an incompatible existing schema.
4. Keep SQL inside the runner's transaction. Do not include `BEGIN`, `COMMIT`,
   `ROLLBACK`, or assignments to `user_version`. `VACUUM` cannot run inside this
   transaction, and changing `foreign_keys` inside a transaction has no effect.
5. Test the upgrade from an explicit prior version with representative rows.
   Assert the resulting schema, data, and version, then rerun to prove it skips
   applied migrations. Use `migrateWith(ctx, db, migrations[:1])` when a fixture
   specifically requires version 1, rather than `migrate`, which means latest.
6. Run `make check` and follow the deployment procedure in
   [operations](../../docs/operations.md).

The tests also exercise a successful migration followed by a failing migration:
the earlier migration's schema, data, and version changes must all roll back.

See the [testing walkthrough](../../docs/migrations-testing.md) for commands to
run the automated checks and exercise a local server with a disposable database.

## Schema diagnostics

`Check` is a read-only Go helper. `CheckReport.OK()` means the version is latest
and the checked schema properties match: bookmark column metadata and the
presence of named tables, indexes, triggers, and views. Extra objects are reported
as differences. A missing bookmarks table produces differences and a count of 0.

This is a limited diagnostic: index definitions, UNIQUE constraints, foreign
keys, trigger/view definitions, and STRICT table options are not fully checked.
It is not sufficient to authorize adoption of an unknown database. Review the
actual schema using the one-time adoption procedure in the operations guide.

## Deployment and recovery

The deployment script stops the server, requires a successful backup for an
existing database, preserves the old binary, installs the new one, and starts
the server. Refresh backup tooling before the first deployment of this version.

See [operations](../../docs/operations.md) for version-0 adoption, backup setup,
deployment failures, and restoring a compatible binary/database pair.
