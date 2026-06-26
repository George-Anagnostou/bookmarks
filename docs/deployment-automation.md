# Deployment Automation and Hardening

Status: done for the current alpha deployment workflow.

This document captures the current Debian VPS deployment shape and the
implemented automation path for repeatable updates. The project is still
intentionally simple: one Go daemon, one SQLite database, nginx in front, and
systemd keeping the process running.

## Current Deployment Model

Traffic should flow like this:

```text
internet -> nginx :443 -> bookmarkd 127.0.0.1:8080 -> SQLite database
```

`bookmarkd` should bind only to localhost. nginx owns public TLS and forwards
API traffic to the local daemon.

Server-side daemon configuration:

```text
BOOKMARKS_ADDR=127.0.0.1:8080
BOOKMARKS_DBPATH=/var/lib/bookmarks/bookmarks.db
BOOKMARKS_TOKEN=<long random token>
```

## Manual Deployment Reference

These commands describe the deployment we want automation to reproduce.

Build the Linux server binary from a development machine:

```sh
GOOS=linux GOARCH=amd64 go build -trimpath -o bookmarkd ./cmd/bookmarkd
```

Use `GOARCH=arm64` for an ARM VPS.

Create the service user and data directory on Debian:

```sh
sudo useradd --system --home /var/lib/bookmarks --shell /usr/sbin/nologin bookmarks
sudo install -d -o bookmarks -g bookmarks -m 0750 /var/lib/bookmarks
sudo install -d -o root -g root -m 0750 /etc/bookmarks
```

Install the daemon:

```sh
sudo install -o root -g root -m 0755 bookmarkd /usr/local/bin/bookmarkd
```

Generate a token:

```sh
openssl rand -hex 32
```

Create `/etc/bookmarks/bookmarkd.env`:

```sh
BOOKMARKS_ADDR=127.0.0.1:8080
BOOKMARKS_DBPATH=/var/lib/bookmarks/bookmarks.db
BOOKMARKS_TOKEN=<long random token>
```

Lock down the env file:

```sh
sudo chown root:root /etc/bookmarks/bookmarkd.env
sudo chmod 0600 /etc/bookmarks/bookmarkd.env
```

Install the systemd unit at `/etc/systemd/system/bookmarkd.service`:

```ini
[Unit]
Description=Bookmark manager API
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
User=bookmarks
Group=bookmarks
EnvironmentFile=/etc/bookmarks/bookmarkd.env
ExecStart=/usr/local/bin/bookmarkd
Restart=on-failure
RestartSec=2s
WorkingDirectory=/var/lib/bookmarks

NoNewPrivileges=true
PrivateTmp=true
ProtectSystem=strict
ProtectHome=true
ReadWritePaths=/var/lib/bookmarks

[Install]
WantedBy=multi-user.target
```

Enable and start:

```sh
sudo systemctl daemon-reload
sudo systemctl enable --now bookmarkd
sudo systemctl status bookmarkd
```

nginx should proxy the public HTTPS host to the localhost daemon:

```nginx
server {
    listen 80;
    server_name bookmarks.example.com;

    location / {
        proxy_pass http://127.0.0.1:8080;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
    }
}
```

After TLS is configured, the important check is:

```sh
curl -fsS https://bookmarks.example.com/healthz
```

Expected response:

```json
{"status":"ok"}
```

## Hardening Checklist

- `bookmarkd` binds to `127.0.0.1`, not `0.0.0.0`.
- nginx is the only public HTTP entrypoint.
- HTTPS is enabled for the public hostname.
- `/etc/bookmarks/bookmarkd.env` is mode `0600`, owned by `root`.
- `/var/lib/bookmarks` is owned by the `bookmarks` service user and is not
  world-readable.
- The systemd service runs as the `bookmarks` user, not `root`.
- `ProtectSystem=strict`, `ProtectHome=true`, `PrivateTmp=true`, and
  `NoNewPrivileges=true` are enabled in the unit.
- The firewall allows public `80/tcp` and `443/tcp`, but does not expose
  `8080/tcp`.
- nginx access logs are acceptable, but bearer tokens are never placed in query
  strings.
- The SQLite database has a backup plan before frequent real usage.
- Update and rollback steps are documented before changing the deployed binary.

## Frictionless Update Workflow

The update path should be boring:

1. Run tests locally.
2. Build a Linux `bookmarkd` binary.
3. Copy the new binary to the VPS under a temporary name.
4. Stop or restart the service with systemd.
5. Atomically install the new binary.
6. Start the service.
7. Verify `/healthz` returns a successful response.
8. Keep the previous binary long enough to roll back.

A first automation pass can live entirely in local shell scripts or a Makefile.
That is enough for this project. A full deployment tool would be premature.

## Configuration Names

The deployment workflow uses a few names that should stay distinct:

- `BOOKMARKS_ADDR`: where `bookmarkd` listens on the VPS. This should normally
  be `127.0.0.1:8080`. It belongs in `/etc/bookmarks/bookmarkd.env`, not in the
  normal local deploy config.
- `BOOKMARKS_DOMAIN`: the public hostname nginx serves, such as
  `bookmarks.example.com`.
- `BOOKMARKS_URL`: the full public URL used by deploy verification, such as
  `https://bookmarks.example.com`. If it is not set, scripts derive it from
  `BOOKMARKS_DOMAIN`.
- `BOOKMARKS_DEPLOY_HOST`: the SSH target. This can be an `~/.ssh/config` alias
  such as `mysite`, a `user@host` value, an IP address, or a provider hostname.
  If it is not set, scripts use `BOOKMARKS_DOMAIN`.
- `BOOKMARKS_DEPLOY_USER`: optional SSH user. Leave this unset when
  `BOOKMARKS_DEPLOY_HOST` is an SSH config alias that already defines `User`, or
  when `BOOKMARKS_DEPLOY_HOST` already contains `user@host`.
- `BOOKMARKS_TOKEN`: the current bearer token. The server needs it in
  `/etc/bookmarks/bookmarkd.env`, and authenticated clients need it for API
  requests. The normal local deploy workflow no longer needs it because
  verification uses public `/healthz`.

For normal local deployment, copy `.env.deploy.example` to `.env.deploy` and set
the production values there. `.env.deploy` is ignored by git and loaded by the
Makefile.

## Normal Update Workflow

After the VPS has been bootstrapped, the normal update path is one command from
the development machine:

```sh
make update
```

That command runs tests, rebuilds the local `bookmarkctl` CLI, builds the Linux
`bookmarkd` binary, copies it to the VPS, creates a database backup if the
production database exists, preserves the previous binary, restarts the systemd
service, and verifies public `/healthz`.

Rollback is also one command:

```sh
make rollback
```

## Local Build Targets

Builds are handled by the root `Makefile`:

```sh
make test
make build-cli
make build-server
make update
make rollback
```

The default server build target is Linux amd64:

```text
dist/bookmarkd-linux-amd64
```

For an ARM VPS, override the architecture:

```sh
make build-server SERVER_GOARCH=arm64
```

The `bookmarkctl` CLI build installs the client for the local machine running
the command:

```sh
make build-cli
```

That may be macOS, Linux, BSD, or another local client environment. The server
binary is still cross-compiled separately for the VPS target.

## Deploy Script

`scripts/deploy-bookmarkd.sh` runs from the development machine and copies a
built daemon to the VPS over SSH. `BOOKMARKS_DEPLOY_HOST` can be an SSH config
alias, so root SSH can live in `~/.ssh/config` for now:

```sh
cp .env.deploy.example .env.deploy
$EDITOR .env.deploy
make update
```

Optional variables:

```sh
BOOKMARKS_DEPLOY_SERVICE=bookmarkd
BOOKMARKS_DEPLOY_TARGET=/usr/local/bin/bookmarkd
BOOKMARKS_DEPLOY_USER=root
```

The `make update` workflow:

- Runs local tests and builds the server binary.
- Copies the Linux binary to the VPS under `/tmp`.
- Runs `/usr/local/sbin/bookmarks-backup` if the production database exists.
- Preserves the current binary as `/usr/local/bin/bookmarkd.previous`.
- Installs the new binary at `/usr/local/bin/bookmarkd`.
- Restarts `bookmarkd` through systemd.
- Verifies the public health endpoint with unauthenticated `GET /healthz`.

If the database exists and the backup script is missing or fails, deployment
stops before the binary is replaced.

Future hardening: create a sudo-capable deploy user and set
`BOOKMARKS_DEPLOY_USER` to that user instead of using direct root SSH.

## Debian Bootstrap Script

`scripts/bootstrap-debian.sh` runs once on a Debian VPS. Copy it to the VPS and
run it as root there:

```sh
scp scripts/bootstrap-debian.sh root@bookmarks.example.com:/root/bootstrap-debian.sh
ssh root@bookmarks.example.com

export BOOKMARKS_DOMAIN=bookmarks.example.com
export BOOKMARKS_TOKEN=<token>
sh /root/bootstrap-debian.sh
```

The bootstrap script installs `nginx` and `sqlite3`, creates the `bookmarks`
service user, creates `/var/lib/bookmarks`, writes `/etc/bookmarks/bookmarkd.env`,
installs the systemd unit, installs the nginx site, installs
`/usr/local/sbin/bookmarks-backup`, and enables the daily backup timer.

This script intentionally does not install TLS. For Debian/nginx, certbot or an
existing ACME workflow should own certificate issuance and renewal.

## Deployment Checklist

One-time setup:

1. Point DNS for `BOOKMARKS_DOMAIN` at the VPS.
2. Generate a token with `openssl rand -hex 32`.
3. Copy `.env.deploy.example` to `.env.deploy` and fill in the real domain and
   SSH target.
4. Copy `scripts/bootstrap-debian.sh` to the VPS and run it as root with
   `BOOKMARKS_DOMAIN` and `BOOKMARKS_TOKEN` set.
5. Run `make update` locally to install the first `bookmarkd` binary.
6. Configure TLS for `BOOKMARKS_DOMAIN`.
7. Set `BOOKMARKS_URL := https://...` in `.env.deploy` if it is not already set.
8. Run `make update` again and confirm `/healthz` verification succeeds over
   HTTPS.

Normal update:

1. Make code changes locally.
2. Run `make update`.
3. If verification fails after restart, run `make rollback`.

## Health Check

The app exposes a public liveness endpoint for deployment verification:

```sh
curl -fsS "$BOOKMARKS_URL/healthz"
```

Expected response:

```json
{"status":"ok"}
```

`GET /healthz` is intentionally unauthenticated and minimal. It should not
expose bookmark data, version details, hostnames, database paths, uptime,
configuration, or diagnostics. It is a liveness check for deploy scripts and
simple monitoring, not an administrative status endpoint.

If richer diagnostics are needed later, add a separate authenticated endpoint
instead of expanding public `/healthz`.

## Rollback Plan

The deploy script should preserve the previous binary at:

```text
/usr/local/bin/bookmarkd.previous
```

Manual rollback:

```sh
sudo cp /usr/local/bin/bookmarkd.previous /usr/local/bin/bookmarkd
sudo systemctl restart bookmarkd
sudo systemctl status bookmarkd
```

From the development machine, the scripted rollback path is:

```sh
make rollback
```

Before schema migrations exist, avoid deploying code that changes the database
shape without a database backup.

## Ticket Status

1. Done: document the current VPS deployment.
2. Done: add a local server build target.
3. Done: add a deploy script for binary updates.
4. Done: add a one-time Debian VPS bootstrap script.
5. Done: add deployment verification through `GET /healthz`.
6. Done: add rollback documentation and `make rollback`.
7. Deferred to the backup workstream: operational backup validation. The deploy
   script can call `/usr/local/sbin/bookmarks-backup` when the production
   database exists, but backup policy and restore verification remain tracked in
   `docs/backups.md`.
8. Done: implement `GET /healthz` as an unauthenticated minimal liveness check.

Future hardening:

- Migrate deployment from root SSH to a sudo-capable deploy user.
- Decide whether public `/healthz` should remain the only operational endpoint
  or whether authenticated readiness diagnostics are useful.
