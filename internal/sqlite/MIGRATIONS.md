# SQLite migration work plan

This document is a review scaffold for making migration handling dependable as
the schema evolves. It records implemented safeguards and remaining work.

## Current implementation

- Migrations are records in `migrations.go` with an explicit version, name,
  and SQL; versions are currently required to be contiguous from 1.
- `migrateWith` applies pending migrations in one transaction and records the
  resulting version with SQLite's `user_version` pragma.
- `Check` opens the database read-only, reports its version and bookmark count,
  compares columns in `bookmarks`, and checks the presence of schema objects
  against the schema produced by the current migration list.
- Migration tests cover a fresh database, an already-current database, a
  database newer than the binary, and rollback after a failed migration.

## Proposed guarantees to review

- [ ] Released migrations are immutable. A schema change after release is a
  new migration, not an edit to an existing one.
- [x] Migration versions are contiguous and invalid ordering or gaps are
  rejected before any migration runs.
- [x] A failed migration leaves both schema and recorded version unchanged.
- [x] Applying migrations to a current database makes no changes.
- [ ] Decide whether migrations are forward-only. If rollback is not supported,
  document backup/restore as the recovery mechanism.
- [ ] Define whether and how concurrent application processes are prevented
  from migrating the same database at the same time.

## Migration implementation work

- [x] Replace positional SQL strings with migration records containing an
  explicit version, a human-readable name, and SQL.
- [x] Validate contiguous migration versions and reject empty names or SQL
  before opening a transaction.
- [x] Include migration version and name in migration-application errors.
- [x] Return a typed error identifying the failed migration and stage, while
  preserving the underlying database error for inspection.
- [ ] Consider application-function migrations if SQL-only migrations become
  insufficient.
- [ ] Decide whether to record checksums to detect edits to released
  migrations. Define how checksum changes are handled before enforcing them.
- [ ] Confirm transaction behavior for the DDL and PRAGMAs used by future
  migrations with the SQLite driver in use.

## Migration test matrix

- [ ] Apply each migration separately and check the resulting schema and
  recorded version.
- [ ] Upgrade fixtures from each supported prior release to the latest
  version.
- [ ] Verify data is preserved or transformed correctly by each upgrade.
- [ ] Fail a later migration in a multi-migration run and verify all changes
  from that run are rolled back.
- [x] Verify a failed upgrade leaves the prior version intact and a corrected
  migration can be retried successfully.
- [ ] Test negative, unsupported-future, and otherwise invalid versions.
- [ ] Test context cancellation and representative SQLite operational errors
  (for example, a locked or read-only database).
- [ ] Keep representative database fixtures from released versions, or
  document how those fixtures are generated and maintained.

## Schema-check work

`Check` currently checks columns in `bookmarks`, not the complete schema.
Decide which objects and properties constitute a valid schema, then add
inspection and comparison for:

- [x] Presence of expected and unexpected tables and schema objects.
- [ ] Index definitions, including uniqueness, indexed columns, order, and
  partial-index predicates. Current checking only compares object type/name.
- [ ] Foreign keys and other relevant constraints.
- [ ] Triggers and views, if the application uses them.
- [ ] Table options such as `STRICT`.
- [ ] Whether equivalent SQLite declarations should compare equal, rather than
  comparing raw SQL text.

Also decide how `Check` should treat a missing `bookmarks` table: return an
operational error, or return a report containing a schema difference. Its
current bookmark-count query returns an error in that case. Decide whether a
correct older schema is “valid but needs upgrade” or simply not OK; currently
`CheckReport.OK()` means latest version plus no reported column differences.

## Operational safety and reporting

- [ ] Establish when and how a database backup is made before migration.
- [ ] Define whether the application may serve requests before migration has
  completed and how it handles an unsupported schema version.
- [ ] Add migration start/completion logging without logging sensitive data.
- [ ] Decide whether schema differences should remain strings or become
  structured values (object, property, expected value, actual value, severity).
- [ ] Decide whether extra columns or objects are errors, warnings, or allowed
  extensions.

## Review decisions

Fill these in before expanding the implementation:

- Migration policy (forward-only or rollback support):
- Definition of a valid schema version for `Check`:
- Schema objects `Check` must validate:
- Policy for unknown/extra schema objects:
- Backup and concurrent-migration policy:
- Migration metadata/checksum policy:
- Compatibility requirements for old database files:
