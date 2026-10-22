# Testing migrations

Run these commands from the repository root. You need Go, `sqlite3`, and `curl`.

## 1. Automated checks

```sh
make check
```

This checks formatting, shell syntax, all Go tests, vet, and race detection.
For a focused, uncached run with individual results:

```sh
go test -count=1 -v ./internal/sqlite -run 'TestMigrate|TestCheck'
go test -count=1 -v ./scripts
```

The important expectations are:

| Scenario | Expected result |
|----------|-----------------|
| Empty database | Schema created and version advanced |
| Already current | Applied migrations skipped; data preserved |
| Successful upgrade | Schema and data changes applied once |
| Later migration fails | All pending schema, data, and version changes rolled back |
| Failed migration retried after correction | Upgrade succeeds |
| Nonempty version 0 | Rejected without schema or data changes |
| Negative or future version | Rejected without applying migrations |
| Missing bookmarks table during `Check` | Schema differences reported |
| Deployment backup fails | Old binary retained; server stays stopped |
| Deployment sees nonempty version 0 | Rejected before stopping server |

The deployment tests execute the remote shell body with temporary files and fake
system commands. They verify ordering and failure handling; exercising real
systemd, permissions, and backup creation requires a Debian rehearsal.

## 2. Run a local server with a disposable database

Use the same terminal for the following commands. The temporary directory keeps
this exercise separate from your normal database. The build is native to your
computer; `make build-server` defaults to a Linux binary for the VPS.
The local `sqlite3` commands use a normal connection so SQLite can create WAL
sidecar files if needed, including with macOS's bundled CLI.

```sh
TEST_DIR="$(mktemp -d "${TMPDIR:-/tmp}/bookmarks-migrations.XXXXXX")"
go build -o "$TEST_DIR/bookmarkd" ./cmd/bookmarkd
export BOOKMARKS_DBPATH="$TEST_DIR/bookmarks.db"
export BOOKMARKS_ADDR=127.0.0.1:18081
export BOOKMARKS_TOKEN=migration-test-token
printf 'Test directory: %s\n' "$TEST_DIR"

"$TEST_DIR/bookmarkd" >"$TEST_DIR/server.log" 2>&1 &
SERVER_PID=$!
curl -fsS --retry 10 --retry-connrefused --retry-delay 1 --max-time 2 \
  http://127.0.0.1:18081/healthz
```

Expect a successful health response. If it fails, inspect
`cat "$TEST_DIR/server.log"`; change the port if it is already occupied.

Create a bookmark with a title so no background title fetch is needed:

```sh
curl -fsS http://127.0.0.1:18081/api/bookmarks \
  -H "Authorization: Bearer $BOOKMARKS_TOKEN" \
  -H 'Content-Type: application/json' \
  -d '{"url":"https://example.com/migration-test","title":"Migration test","notes":"Keep me"}'

sqlite3 "$BOOKMARKS_DBPATH" \
  'PRAGMA user_version; SELECT title, notes FROM bookmarks; PRAGMA integrity_check;'
```

For this release, expect version `1`, `Migration test|Keep me`, and `ok`.

## 3. Restart and verify persistence

```sh
kill "$SERVER_PID"
wait "$SERVER_PID"
"$TEST_DIR/bookmarkd" >"$TEST_DIR/server.log" 2>&1 &
SERVER_PID=$!
curl -fsS --retry 10 --retry-connrefused --retry-delay 1 --max-time 2 \
  http://127.0.0.1:18081/healthz
curl -fsS http://127.0.0.1:18081/api/bookmarks \
  -H "Authorization: Bearer $BOOKMARKS_TOKEN"
```

Expect the same single bookmark, including its ID, title, and notes. Stop the
server before the next exercise:

```sh
kill "$SERVER_PID"
wait "$SERVER_PID"
```

## 4. Exercise rejection and explicit adoption

Only run these version-changing commands against the disposable database above.
The schema is known to be version 1 because this binary just created it.

```sh
sqlite3 "$BOOKMARKS_DBPATH" 'PRAGMA user_version = 0;'
"$TEST_DIR/bookmarkd"
```

Expect a nonzero exit and an error containing `schema version is 0`, before the
server starts listening. Check that the bookmark and version remain intact:

```sh
sqlite3 "$BOOKMARKS_DBPATH" \
  'PRAGMA user_version; SELECT title, notes FROM bookmarks;'
```

Expect `0` and `Migration test|Keep me`. Label this known test schema correctly,
then simulate a database from a newer release:

```sh
sqlite3 "$BOOKMARKS_DBPATH" 'PRAGMA user_version = 1;'
sqlite3 "$BOOKMARKS_DBPATH" 'PRAGMA user_version = 999;'
"$TEST_DIR/bookmarkd"
sqlite3 "$BOOKMARKS_DBPATH" \
  'PRAGMA user_version; SELECT title, notes FROM bookmarks;'
```

Expect a nonzero server exit mentioning `newer than supported version 1`, version
`999` unchanged, and the original bookmark. Reset this test fixture to version 1
and start it once more to verify recovery:

```sh
sqlite3 "$BOOKMARKS_DBPATH" 'PRAGMA user_version = 1;'
"$TEST_DIR/bookmarkd" >"$TEST_DIR/server.log" 2>&1 &
SERVER_PID=$!
curl -fsS --retry 10 --retry-connrefused --retry-delay 1 --max-time 2 \
  http://127.0.0.1:18081/healthz
kill "$SERVER_PID"
wait "$SERVER_PID"
unset BOOKMARKS_DBPATH BOOKMARKS_ADDR BOOKMARKS_TOKEN
```

The test database and logs remain in `$TEST_DIR` for inspection. Changing version
numbers is appropriate here because the schema never changed; it is not a way
to downgrade a real migrated database.

## 5. Rehearse production adoption and deployment

For your actual version-0 database, first inspect its schema using the
[one-time adoption procedure](operations.md#one-time-adoption-of-an-existing-version-0-database).
Verify a SQLite backup, then rehearse adoption on a copy of that backup locally:
point `BOOKMARKS_DBPATH` at the copy, confirm version 0 is rejected, verify the
schema matches `schemaV1`, record version 1, and confirm the server starts and
bookmark counts/data are preserved.

For a complete deployment rehearsal, use a disposable Debian instance with the
standard service/config layout. Install the backup tooling, deploy with its SSH
host and URL explicitly configured, then verify:

- A fresh pre-deploy backup exists, passes `PRAGMA integrity_check`, and has the
  expected version and bookmark count.
- The service starts, health succeeds, and existing bookmarks can be read.
- A second deployment preserves data and the schema version.
- The documented restore procedure recovers that backup with a compatible binary.

Follow [operations](operations.md) for production deployment and recovery. The
multi-migration rollback tests use test-only SQL; this release still registers
only migration 1. Each future real migration needs its own data-preservation test.
