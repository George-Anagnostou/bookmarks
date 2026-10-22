# Safety net brief — CI, backups, rollback

Repo: `https://github.com/George-Anagnostou/bookmarks`  
Branch: `ops/safety-net`  
Constraint: shell, Makefile, and Go stdlib only; no new heavy dependencies. Schema
and Phase 1 API work are **out of scope**.

## Goal

Before more features land, make mistakes cheap to catch and cheap to undo:

1. CI runs `make check` on every PR and every push to `main`.
2. Backups leave the VPS (offsite copy) and are proven restorable (restore drill).
3. A deploy whose migration must be undone can be rolled back with one command,
   restoring the pre-deploy database together with the previous binary.
4. Small gaps in test coverage (`internal/access`) are closed.

## Current state (as of main `19b1536`)

| Area | Today |
|------|-------|
| CI | None. `make check` (gofmt, `sh -n`, test, vet, race) runs only locally in `make update` |
| Backups | Daily `bookmarks-backup.timer` on the VPS, 30-day retention, same disk as the database |
| Offsite | "Future" section in `docs/operations.md`; not implemented |
| Restore drill | Mentioned once ("run a restore drill once after setup"); no tooling |
| Schema check | `sqlite.Check` exists but no command exposes it |
| Deploy | Takes a pre-deploy backup but does not record which file or schema version |
| Rollback | `make rollback` swaps in `bookmarkd.previous` only; a committed migration makes the old binary refuse to start, and recovery is a manual restore |
| `internal/access` | No tests |
| Shell lint | `sh -n` only; shellcheck reports SC1007 in two scripts |

---

## a. CI

### Change

`.github/workflows/ci.yml`: on `pull_request` and `push` to `main`, read-only
token, `actions/checkout@v7`, `actions/setup-go@v7` with `go-version-file:
go.mod`, require `shellcheck` (preinstalled on `ubuntu-latest`), run
`make check`.

`make script-check` also runs `shellcheck scripts/*.sh` when shellcheck is
installed (skips with a message otherwise, so local checks still work).

### Acceptance

- [ ] A PR shows a `CI / check` status that runs `make check`.
- [ ] The Go version comes from `go.mod`; no version is duplicated in the workflow.
- [ ] All `scripts/*.sh` pass `sh -n` and `shellcheck`.

## b. Backups: offsite copy and restore drill

### Offsite

- `make install-backups` additionally installs `rsync`,
  `/usr/local/sbin/bookmarks-offsite`, `bookmarks-offsite.service`, and
  `bookmarks-offsite.timer` (daily ~04:15 UTC, one hour after the backup).
- Configured only through `/etc/bookmarks/bookmarkd.env`:
  `BOOKMARKS_OFFSITE_DEST`, `BOOKMARKS_OFFSITE_TOOL` (`rsync` default, or
  `rclone`), and the tools' own `RSYNC_RSH` / `RCLONE_CONFIG`. No provider,
  host, or credential is in the repo.
- Copies `bookmarks-*.db` only and never deletes at the destination.
- Unset destination: logs a skip and exits 0. A separate unit means an offsite
  failure never blocks a backup or a deploy.

### Restore drill

- `bookmarkd check DBPATH`: new read-only subcommand that prints the
  `sqlite.Check` report as JSON and exits non-zero if the schema differs.
  Needs no token or server config.
- `scripts/restore-drill.sh [DIR]`: copies the newest `bookmarks-*.db` to a
  temp directory, runs `integrity_check`, prints schema version and bookmark
  count, runs `bookmarkd check`, cleans up. Warns if the newest backup is more
  than ~2 days old. Never touches the live database or services.
- `make restore-drill` runs it on the VPS over SSH; `make restore-drill-local
  BACKUPS=dir` runs it against downloaded (e.g. offsite) copies.

### Acceptance

- [ ] With `BOOKMARKS_OFFSITE_DEST` set, `systemctl start bookmarks-offsite.service`
      copies the backups; with it unset, the service succeeds with a skip message.
- [ ] `make restore-drill` prints integrity `ok`, schema version, row count, the
      check report, and `restore drill passed`; it fails on a corrupt or
      schema-drifted backup.
- [ ] `docs/operations.md` documents configuration, testing, and drill cadence.

## c. Rollback

### Change

- Deploy: after the pre-deploy backup, require that a **new** backup file
  appeared, copy it to `$BACKUP_DIR/pre-deploy.db`, and write
  `pre-deploy.env` (`PRE_DEPLOY_SCHEMA_VERSION`, `PRE_DEPLOY_BACKUP`).
  Retention only prunes `bookmarks-*.db`, so these survive until the next deploy.
- `scripts/rollback-bookmarkd.sh` runs a read-only **plan** on the VPS, then an
  **apply**:
  - `binary`: database absent or not migrated → swap binary only, no prompt.
  - `restore`: live schema version > pre-deploy version → verify
    `pre-deploy.db` (integrity, version), warn about data loss, prompt for the
    word `restore` (or `BOOKMARKS_ROLLBACK_CONFIRM=restore`), stop server and
    backup timer, move live db/WAL/SHM to `rollback-XXXXXXXX/`, install
    `pre-deploy.db`, swap binary, restart, verify `/healthz`.
  - `unknown`: no record → refuse unless `BOOKMARKS_ROLLBACK_MODE=binary`.
  - Apply recomputes the plan and refuses if it changed after confirmation.
    The replaced binary is kept as `bookmarkd.rolled-back`.
- Runbook in `docs/operations.md` ("Rollback").

### Acceptance

- [ ] `scripts/rollback_test.go` covers binary, restore, plan change, unknown
      (forced binary allowed, restore refused), corrupt or mismatched backup,
      and missing `.previous`.
- [ ] `scripts/deploy_test.go` asserts the pre-deploy record and fails the
      deploy, keeping the old binary, when no new backup appears.

## d. Minimal tests for `internal/access`

`token_test.go`: `NewToken` gives distinct, URL-safe 32-byte tokens;
`HashToken` matches the SHA-256 test vector and distinguishes inputs.

---

## Out of scope (do not do)

- Schema or migration changes; Phase 1 API, client, and CLI work; tags.
- Pruning offsite copies, encryption at rest, choosing a provider.
- Automatic rollback on failed deploys; a deploy user instead of root SSH.
- Release builds or artifacts in CI.

## Decisions and open questions

- **Push from the VPS** (configurable rsync or rclone) instead of the roadmap's
  pull from a trusted machine, so the destination lives in the env file. Pull
  remains documented as an alternative. *Open:* push vs pull, and which
  destination (another machine over SSH, S3/B2 via rclone).
- Offsite is add-only. A push credential on the VPS could still overwrite
  same-named files at the destination; prefer an append-only/object-lock bucket or
  an `rrsync`-restricted SSH key.
- `pre-deploy.db` holds a full copy of the database (doubles one backup's disk
  use).
- A restore rollback discards writes since the deploy; the prompt is the guard.

---

## Your actions

Do these yourself, in order:

1. **Push and open the PR.** From a machine with GitHub access:
   `git push -u origin ops/safety-net`, then open a PR to `main` (or apply the
   patches in `safety-net-patches/` with `git am`).
2. **Confirm CI is green.** On the PR's *Checks* tab, `CI / check` must pass;
   also see the repo's *Actions* page. Merge when green.
3. **Require CI on `main`.** *Settings → Branches* (or *Rules → Rulesets*): add a
   rule for `main` requiring the `check` status to pass before merging. It
   becomes selectable after the workflow has run once.
4. **Deploy the merged `main`** with `make update`. This deploy writes the first
   `pre-deploy.db`/`pre-deploy.env` and installs a `bookmarkd` that has `check`.
5. **Install the new backup units:** `make install-backups` (installs `rsync`,
   `bookmarks-offsite`, and its timer).
6. **Prepare the offsite destination** (your choice; see open questions):
   - rsync: create a backup user and directory on the target, generate
     `sudo ssh-keygen -t ed25519 -f /root/.ssh/bookmarks-offsite -N ''` on the
     VPS, authorize the public key on the target (ideally restricted with
     `rrsync` to that directory), and accept the host key once with
     `sudo ssh -i /root/.ssh/bookmarks-offsite user@host true`; or
   - rclone: `sudo apt-get install rclone`, then
     `sudo rclone config --config /root/.config/rclone/rclone.conf` with a
     write-only key/bucket.
7. **Configure the VPS** in `/etc/bookmarks/bookmarkd.env` (keep mode 0600;
   double-quote values with spaces): `BOOKMARKS_OFFSITE_DEST`, plus
   `RSYNC_RSH` or `BOOKMARKS_OFFSITE_TOOL=rclone` and `RCLONE_CONFIG`.
8. **Test offsite:** `sudo systemctl start bookmarks-offsite.service`, then
   `journalctl -u bookmarks-offsite.service -n 20 --no-pager`, check the files
   at the destination, and `systemctl list-timers 'bookmarks-*'`.
9. **Run the restore drill:** `make restore-drill`. Optionally download an
   offsite copy and run `make restore-drill-local BACKUPS=<dir>`. Put a monthly
   reminder on your calendar.
10. **Test a rollback** at a quiet time: right after a deploy, run
    `make rollback`; expect `mode=binary` (no migration), `rollback verified`,
    then `make update` to go forward again. The `restore` path is covered by
    tests; exercise it for real only on a scratch VPS or when needed.
