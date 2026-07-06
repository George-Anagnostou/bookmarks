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

`make update` runs tests, builds `bookmarkctl` and a Linux `bookmarkd` binary,
copies the binary to the VPS, runs a pre-deploy backup if the database exists,
preserves the previous binary as `.previous`, restarts systemd, and curls public
`/healthz`.

Rollback:

```sh
make rollback
```

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

Backups use SQLite `.backup` while `bookmarkd` is running, then run
`PRAGMA integrity_check` and query the `bookmarks` table.

### Restore

1. Pick a backup and verify it:

   ```sh
   sqlite3 /var/backups/bookmarks/bookmarks-YYYYMMDDTHHMMSSZ.db "PRAGMA integrity_check;"
   sqlite3 /var/backups/bookmarks/bookmarks-YYYYMMDDTHHMMSSZ.db "SELECT COUNT(*) FROM bookmarks;"
   ```

2. Stop the service:

   ```sh
   sudo systemctl stop bookmarkd
   ```

3. Save the current database:

   ```sh
   sudo install -d -o root -g root -m 0750 /var/backups/bookmarks/restore-safety
   sudo cp /var/lib/bookmarks/data/bookmarks.db /var/backups/bookmarks/restore-safety/
   ```

4. Restore:

   ```sh
   sudo install -o bookmarks -g bookmarks -m 0640 \
     /var/backups/bookmarks/bookmarks-YYYYMMDDTHHMMSSZ.db \
     /var/lib/bookmarks/data/bookmarks.db
   ```

5. Start and verify:

   ```sh
   sudo systemctl start bookmarkd
   curl -fsS https://bookmarks.example.com/healthz
   ```

Run a restore drill to a temp path once after setup. Take a manual backup before
schema migrations.

### Future: offsite copy

Not implemented yet. Planned approach: pull backups from a trusted machine with
`rsync` over SSH, e.g. weekly to `~/Backups/bookmarks/`.