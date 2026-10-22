# Operations

## VPS layout

```text
internet → nginx :443 → bookmarkd 127.0.0.1:8081 → SQLite
```

| Path | Purpose |
|------|---------|
| `/usr/local/bin/bookmarkd` | Server binary |
| `/etc/bookmarks/bookmarkd.env` | Server config (mode 0600) |
| `/var/lib/bookmarks/data/bookmarks.db` | SQLite database |
| `/var/backups/bookmarks/` | Daily backup files |
| `/usr/local/sbin/bookmarks-backup` | Backup script |

Server env (`/etc/bookmarks/bookmarkd.env`):

```text
BOOKMARKS_ADDR=127.0.0.1:8081
BOOKMARKS_DBPATH=/var/lib/bookmarks/data/bookmarks.db
BOOKMARKS_TOKEN=<token>
```

Use an absolute `BOOKMARKS_DBPATH` on the VPS.

## First-time bootstrap

On a fresh Debian VPS as root:

```sh
BOOKMARKS_DOMAIN=bookmarks.example.com \
BOOKMARKS_TOKEN=<token> \
sh scripts/bootstrap-debian.sh
```

Then deploy the binary, configure TLS for nginx, and verify `/healthz`.

## Deploy from dev machine

```sh
cp .env.deploy.example .env.deploy
make update
```

`make update` runs formatting, shell syntax, tests, vet, and race checks; builds
and installs the local `bookmarkctl`; builds a Linux `bookmarkd` binary; copies
the server binary to the VPS; stops the server; runs a pre-deploy backup if the
database exists; preserves the previous binary as `.previous`; starts systemd;
and checks public `/healthz` with a short retry window. Migrations run during
startup before the server accepts requests. Expect brief downtime.

Run one server process and one deployment at a time. Deployment reads the
root-owned server configuration with root privileges. The backup service reads
that same environment file on every run.

**Before the first deployment of this migration system:** run
`make install-backups` to refresh the installed backup script and unit, then
complete the version-0 adoption procedure below for an existing unversioned
database. The deployment preflight rejects such a database before stopping the
old server.

If backup fails, deployment stops before replacing the installed binary and
leaves the server stopped. Fix the backup problem and retry, or explicitly start
the old server with `sudo systemctl start bookmarkd`. If startup or health
verification fails, inspect `sudo journalctl -u bookmarkd -n 50 --no-pager`.
The script does not automatically restore a database or binary.

To verify an already deployed server without building or deploying:

```sh
make verify
```

Binary rollback (only when the database is compatible with the previous binary):

```sh
make rollback
```

If migration failed, the migration transaction leaves the previous schema,
data, and version intact. If migration committed successfully, the old binary
may reject the newer database. In that case, follow the restore procedure and
restore the matching previous binary before starting the service. A backup
restore loses changes made after that backup. Preserve the failed release's
database files for later inspection.

Deploy config (`.env.deploy`):

| Variable | Purpose |
|----------|---------|
| `BOOKMARKS_DOMAIN` | Public hostname |
| `BOOKMARKS_DEPLOY_HOST` | SSH target (can be an `~/.ssh/config` alias) |
| `BOOKMARKS_URL` | Full URL for post-deploy verification |

## Backups

Install or refresh backup tooling on the VPS:

```sh
make install-backups
```

This installs:

- `/usr/local/sbin/bookmarks-backup`
- `bookmarks-backup.service` (oneshot)
- `bookmarks-backup.timer` (daily ~03:15 UTC, 30-day retention)

Manual backup:

```sh
sudo systemctl start bookmarks-backup.service
```

Check schedule and logs:

```sh
systemctl list-timers bookmarks-backup.timer
journalctl -u bookmarks-backup.service
ls -lh /var/backups/bookmarks/
```

Backups use SQLite `.backup`, then run `PRAGMA integrity_check` and query the
`bookmarks` table. Scheduled backups can run while the server is serving
requests. Deployment stops the server first, then restarts the backup oneshot
service to require a fresh backup with no intervening application writes.

Use the systemd service for manual backups too: it supplies the configured
database path, backup directory, and retention settings. The server environment
file takes precedence over the unit's database-path default.

## One-time adoption of an existing version-0 database

Version 0 is reserved for empty databases. A nonempty version-0 database must be
inspected before labeling it as version 1. This procedure is specifically for
the legacy schema matching `internal/sqlite/schema.go`'s `schemaV1`.

1. Refresh backup tooling with `make install-backups` from your development
   machine. On the VPS, inspect the existing database without modifying it:

   ```sh
   sudo sqlite3 -readonly /var/lib/bookmarks/data/bookmarks.db <<'SQL'
   PRAGMA user_version;
   PRAGMA integrity_check;
   SELECT type, name, sql FROM sqlite_schema
     WHERE substr(name, 1, 7) != 'sqlite_' ORDER BY type, name;
   PRAGMA table_xinfo(bookmarks);
   SELECT count(*) FROM bookmarks;
   SQL
   ```

   Expect version `0`, integrity result `ok`, the `bookmarks` table, and
   `bookmarks_created_at_idx`. Compare the full table/index SQL to `schemaV1`:
   all ten columns, types, defaults, nullability, primary key,
   `UNIQUE(normalized_url)`, `STRICT`, and the index on `created_at DESC` must
   agree. SQLite may omit `IF NOT EXISTS` in its stored SQL. If the schema
   differs, resolve that difference before proceeding; assigning version 1
   does not repair it. The Go `Check` helper does not check all these properties.

2. After confirming the match, stop the server and take a fresh backup. Run
   each step only after the previous one succeeds:

   ```sh
   sudo systemctl stop bookmarkd
   sudo systemctl restart bookmarks-backup.service
   sudo journalctl -u bookmarks-backup.service -n 10 --no-pager
   ```

   Record the backup filename and bookmark count. Keep this backup until the
   new release has been verified. Keep the server stopped during adoption.

3. Record the verified version, then recheck the version and bookmark count:

   ```sh
   sudo sqlite3 -bail /var/lib/bookmarks/data/bookmarks.db <<'SQL'
   BEGIN IMMEDIATE;
   PRAGMA user_version = 1;
   COMMIT;
   PRAGMA user_version;
   SELECT count(*) FROM bookmarks;
   SQL
   ```

   Expect version `1` and the same count. This changes only the recorded version.
   Now deploy with `make update` from your development machine. If deployment
   must wait, the legacy server can be restarted after confirming it supports
   this schema/version. Future schema changes append new migrations.

   A restore of the pre-adoption backup restores version `0` as well. Repeat
   this verified adoption before starting a binary that enforces the new rule.

## Restore

Use these steps with the standard VPS paths. Select the backup from the matching
deployment, and ensure it supports the binary you intend to run. Stop all other
processes using this database, including any interactive SQLite sessions.

1. Pick a backup and verify it:

   ```sh
   sudo sqlite3 -readonly /var/backups/bookmarks/bookmarks-YYYYMMDDTHHMMSSZ.db "PRAGMA integrity_check; PRAGMA user_version; SELECT COUNT(*) FROM bookmarks;"
   ```

2. Stop the server and backup jobs:

   ```sh
   sudo systemctl stop bookmarkd bookmarks-backup.timer bookmarks-backup.service
   ```

3. Move the current database and any WAL/SHM sidecars together into a new safety
   directory. This also leaves a clean destination for restoration:

   ```sh
   sudo sh -eu <<'SH'
   safety_dir="$(mktemp -d /var/backups/bookmarks/restore-XXXXXXXX)"
   db=/var/lib/bookmarks/data/bookmarks.db
   for file in "$db" "$db-wal" "$db-shm"; do
       if [ -e "$file" ]; then
           mv "$file" "$safety_dir/"
       fi
   done
   echo "saved current database files in $safety_dir"
   SH
   ```

4. Restore:

   ```sh
   sudo install -o bookmarks -g bookmarks -m 0640 \
     /var/backups/bookmarks/bookmarks-YYYYMMDDTHHMMSSZ.db \
     /var/lib/bookmarks/data/bookmarks.db
   ```

5. If reverting the release, install the compatible previous binary while the
   server is stopped:

   ```sh
   sudo install -o root -g root -m 0755 /usr/local/bin/bookmarkd.previous /usr/local/bin/bookmarkd.new
   sudo mv /usr/local/bin/bookmarkd.new /usr/local/bin/bookmarkd
   ```

6. Start and verify. Check logs if health does not succeed after startup:

   ```sh
   sudo systemctl start bookmarkd
   curl -fsS https://bookmarks.example.com/healthz
   sudo systemctl start bookmarks-backup.timer
   ```

Run a restore drill to a temp path once after setup. Restarting with a newer
binary automatically applies its pending migrations to the restored database.

## Future: offsite copy

Not implemented yet. Planned approach: pull backups from a trusted machine with
`rsync` over SSH, e.g. weekly to `~/Backups/bookmarks/`.
