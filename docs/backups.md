# VPS Backup Strategy

This project stores bookmark data in one SQLite database. A backup plan should
therefore be simple: make consistent SQLite snapshots, keep a short local
retention window, copy backups off the VPS, and regularly prove that restore
works.

The recommended alpha setup is:

- `bookmarkd` runs under a dedicated `bookmarks` user.
- `BOOKMARKS_DBPATH=/var/lib/bookmarks/bookmarks.db`.
- Local backups are written to `/var/backups/bookmarks`.
- Backup jobs run as `root`, read the database, and create backup files owned by
  `root` with restrictive permissions.
- Offsite copies go to another machine or object store.

## Backup Method

Use SQLite's online backup operation rather than copying the database file
directly. This matters because `bookmarkd` may be writing while the backup runs.
The `sqlite3` shell exposes the online backup API through `.backup`.

Install the SQLite CLI on Debian:

```sh
sudo apt update
sudo apt install sqlite3
```

The core backup command is:

```sh
sqlite3 /var/lib/bookmarks/bookmarks.db ".backup '/var/backups/bookmarks/bookmarks-20260618T120000Z.db'"
```

This creates a consistent backup database while the service can keep running.

Avoid plain `cp /var/lib/bookmarks/bookmarks.db ...` for scheduled backups. It
can be acceptable only when `bookmarkd` is stopped and there are no WAL or SHM
sidecar files to consider.

## Backup Script

Create `/usr/local/sbin/bookmarks-backup` on the VPS:

```sh
#!/bin/sh
set -eu

DB_PATH="${BOOKMARKS_DBPATH:-/var/lib/bookmarks/bookmarks.db}"
BACKUP_DIR="${BOOKMARKS_BACKUP_DIR:-/var/backups/bookmarks}"
RETENTION_DAYS="${BOOKMARKS_BACKUP_RETENTION_DAYS:-30}"

timestamp="$(date -u +%Y%m%dT%H%M%SZ)"
backup_path="${BACKUP_DIR}/bookmarks-${timestamp}.db"
tmp_path="${backup_path}.tmp"
trap 'rm -f "$tmp_path"' EXIT

install -d -o root -g root -m 0750 "$BACKUP_DIR"

if [ ! -f "$DB_PATH" ]; then
  echo "database does not exist: $DB_PATH" >&2
  exit 1
fi

sqlite3 "$DB_PATH" ".backup '${tmp_path}'"
integrity_result="$(sqlite3 "$tmp_path" "PRAGMA integrity_check;")"
if [ "$integrity_result" != "ok" ]; then
  echo "backup failed integrity_check: $integrity_result" >&2
  exit 1
fi
sqlite3 "$tmp_path" "SELECT COUNT(*) FROM bookmarks;" >/dev/null

chmod 0640 "$tmp_path"
mv "$tmp_path" "$backup_path"

find "$BACKUP_DIR" \
  -type f \
  -name 'bookmarks-*.db' \
  -mtime "+${RETENTION_DAYS}" \
  -delete

echo "created backup: $backup_path"
```

Then make it executable:

```sh
sudo chmod 0750 /usr/local/sbin/bookmarks-backup
```

If the production database path differs from `/var/lib/bookmarks/bookmarks.db`,
set `BOOKMARKS_DBPATH` in the systemd unit below or source the same env file used
by `bookmarkd`.

## systemd Timer

Prefer a systemd timer over cron on Debian VPS hosts already using systemd. It
gives cleaner logs, status, and missed-run handling.

Create `/etc/systemd/system/bookmarks-backup.service`:

```ini
[Unit]
Description=Back up bookmark SQLite database

[Service]
Type=oneshot
Environment=BOOKMARKS_DBPATH=/var/lib/bookmarks/bookmarks.db
Environment=BOOKMARKS_BACKUP_DIR=/var/backups/bookmarks
Environment=BOOKMARKS_BACKUP_RETENTION_DAYS=30
ExecStart=/usr/local/sbin/bookmarks-backup
```

Create `/etc/systemd/system/bookmarks-backup.timer`:

```ini
[Unit]
Description=Run bookmark database backup daily

[Timer]
OnCalendar=*-*-* 03:15:00
Persistent=true
RandomizedDelaySec=15m

[Install]
WantedBy=timers.target
```

Enable and test:

```sh
sudo systemctl daemon-reload
sudo systemctl start bookmarks-backup.service
sudo systemctl status bookmarks-backup.service
sudo systemctl enable --now bookmarks-backup.timer
systemctl list-timers bookmarks-backup.timer
```

View logs:

```sh
journalctl -u bookmarks-backup.service
```

## Cron Alternative

Cron is also acceptable for this project. Add a root cron entry:

```cron
15 3 * * * /usr/local/sbin/bookmarks-backup
```

Use systemd timers unless there is a specific reason to prefer cron.

## Retention

Suggested alpha retention:

- Keep daily local backups for 30 days.
- Keep weekly offsite backups for 3 to 6 months.
- Keep at least one known-good backup before schema changes.

The script above handles local 30-day retention. Offsite retention depends on the
copy target.

## Offsite Copies

Local backups protect against bad deploys, accidental deletes, and corrupted
database files. They do not protect against losing the VPS.

Good offsite options:

- `rsync` to a home server or another VPS.
- `scp` to a private machine.
- `rclone` to an object store.
- Provider snapshot backups as a secondary layer, not the only backup.

Minimal `rsync` example from another trusted machine:

```sh
rsync -avz --delete \
  your-vps:/var/backups/bookmarks/ \
  /srv/backups/bookmarks/
```

If using object storage, encrypt before upload unless the storage destination is
fully trusted. A simple alpha approach is to run offsite copies from a trusted
machine that pulls backups over SSH, so the VPS does not need credentials for the
backup destination.

## Restore Procedure

Restores should be practiced before they are needed.

1. Pick a backup file:

   ```sh
   ls -lh /var/backups/bookmarks/bookmarks-*.db
   ```

2. Verify it:

   ```sh
   sqlite3 /var/backups/bookmarks/bookmarks-20260618T120000Z.db "PRAGMA integrity_check;"
   sqlite3 /var/backups/bookmarks/bookmarks-20260618T120000Z.db "SELECT COUNT(*) FROM bookmarks;"
   ```

3. Stop the service:

   ```sh
   sudo systemctl stop bookmarkd
   ```

4. Preserve the current database before replacing it:

   ```sh
   sudo install -d -o root -g root -m 0750 /var/backups/bookmarks/restore-safety
   sudo cp /var/lib/bookmarks/bookmarks.db /var/backups/bookmarks/restore-safety/bookmarks-before-restore.db
   ```

5. Move any SQLite sidecar files aside before replacing the database:

   ```sh
   sudo mv /var/lib/bookmarks/bookmarks.db-wal /var/backups/bookmarks/restore-safety/ 2>/dev/null || true
   sudo mv /var/lib/bookmarks/bookmarks.db-shm /var/backups/bookmarks/restore-safety/ 2>/dev/null || true
   ```

6. Restore the chosen backup:

   ```sh
   sudo install -o bookmarks -g bookmarks -m 0640 \
     /var/backups/bookmarks/bookmarks-20260618T120000Z.db \
     /var/lib/bookmarks/bookmarks.db
   ```

7. Start and verify:

   ```sh
   sudo systemctl start bookmarkd
   sudo systemctl status bookmarkd
   curl -H "Authorization: Bearer $BOOKMARKS_TOKEN" \
     https://bookmarks.example.com/api/bookmarks
   ```

## Permissions

Recommended ownership:

```text
/var/lib/bookmarks              bookmarks:bookmarks 0750
/var/lib/bookmarks/bookmarks.db bookmarks:bookmarks 0640
/var/backups/bookmarks          root:root           0750
/var/backups/bookmarks/*.db     root:root           0640
/usr/local/sbin/bookmarks-backup root:root          0750
```

The backup directory should not be web-readable or owned by the nginx user. The
database contains private browsing history, and future versions may include notes
or shared-user metadata.

## Verification

A backup strategy is not complete until restore has been tested.

Recommended checks:

- Every backup run executes `PRAGMA integrity_check`.
- Every backup run confirms the `bookmarks` table can be queried.
- Once per month, restore the latest backup onto a temporary path and run:

  ```sh
  sqlite3 /tmp/bookmarks-restore-test.db "PRAGMA integrity_check;"
  sqlite3 /tmp/bookmarks-restore-test.db "SELECT id, url, created_at FROM bookmarks ORDER BY created_at DESC LIMIT 5;"
  ```

- Before schema changes, run a manual backup and verify it.
- After schema changes, run a backup and a restore test.

## Open Tickets

### Ticket: Add VPS backup script

Create `/usr/local/sbin/bookmarks-backup` on the VPS using the script above.

Acceptance criteria:

- Running `sudo systemctl start bookmarks-backup.service` creates a timestamped
  backup in `/var/backups/bookmarks`.
- The backup file passes `PRAGMA integrity_check`.
- The backup file can query `SELECT COUNT(*) FROM bookmarks`.
- Backup files are not world-readable.

### Ticket: Add daily systemd backup timer

Install and enable `bookmarks-backup.timer`.

Acceptance criteria:

- `systemctl list-timers bookmarks-backup.timer` shows the next scheduled run.
- `journalctl -u bookmarks-backup.service` shows successful backup output.
- `Persistent=true` is configured so missed runs execute after the VPS comes
  back online.

### Ticket: Add offsite copy

Pick one offsite destination and copy `/var/backups/bookmarks/*.db` to it.

Acceptance criteria:

- At least one backup exists outside the VPS.
- Offsite copy runs without interactive input.
- Offsite backups have a retention policy.
- A backup copied offsite can be downloaded and opened with `sqlite3`.

### Ticket: Practice restore

Perform a restore drill on the VPS or on a temporary machine.

Acceptance criteria:

- A backup is restored to a temporary database path.
- `PRAGMA integrity_check` passes on the restored database.
- The restored database contains expected recent bookmarks.
- The exact restore commands are recorded or confirmed against this document.

### Ticket: Add pre-migration backup habit

Before future schema changes, create and verify a manual backup.

Acceptance criteria:

- Each schema migration issue or PR notes the backup file used before migration.
- Rollback instructions identify which backup to restore.
