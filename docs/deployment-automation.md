# Deployment Automation and Hardening

This document captures the current Debian VPS deployment shape and proposes a
small automation path for repeatable updates. The project is still intentionally
simple: one Go daemon, one SQLite database, nginx in front, and systemd keeping
the process running.

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
curl -fsS \
  -H "Authorization: Bearer $BOOKMARKS_TOKEN" \
  https://bookmarks.example.com/api/bookmarks
```

Expected response for an empty database:

```json
{"bookmarks":[]}
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
7. Verify `/api/bookmarks` returns a successful response.
8. Keep the previous binary long enough to roll back.

A first automation pass can live entirely in local shell scripts or a Makefile.
That is enough for this project. A full deployment tool would be premature.

## Proposed Server Makefile Targets

These are proposals only. Do not add them until the workflow feels right.

```makefile
BINARY_DIR := dist
SERVER_BIN := $(BINARY_DIR)/bookmarkd-linux-amd64

.PHONY: test build-server-linux deploy-server

test:
	go test ./...

build-server-linux:
	mkdir -p $(BINARY_DIR)
	GOOS=linux GOARCH=amd64 go build -trimpath -o $(SERVER_BIN) ./cmd/bookmarkd

deploy-server: test build-server-linux
	./scripts/deploy-bookmarkd.sh $(SERVER_BIN)
```

If the VPS is ARM, either change `GOARCH` to `arm64` or add a separate server
target:

```makefile
build-server-linux-arm64:
	mkdir -p $(BINARY_DIR)
	GOOS=linux GOARCH=arm64 go build -trimpath -o $(BINARY_DIR)/bookmarkd-linux-arm64 ./cmd/bookmarkd
```

The `bookmarkctl` CLI is intentionally not part of server deployment. Build it
for the machine that will run it:

```sh
go build -trimpath -o bookmarkctl ./cmd/bookmarkctl
```

That may be macOS, Linux, BSD, or another local client environment. If local CLI
installation becomes repetitive, add separate client-side automation later.

## Proposed Deploy Script

This script would run from the development machine and copy a built daemon to
the VPS. It assumes SSH access and sudo privileges on the VPS.

```sh
#!/bin/sh
set -eu

if [ "$#" -ne 1 ]; then
  echo "usage: $0 path/to/bookmarkd" >&2
  exit 2
fi

LOCAL_BIN="$1"
REMOTE_HOST="${BOOKMARKS_DEPLOY_HOST:?set BOOKMARKS_DEPLOY_HOST}"
REMOTE_USER="${BOOKMARKS_DEPLOY_USER:-root}"
REMOTE_TMP="/tmp/bookmarkd.$$"

scp "$LOCAL_BIN" "$REMOTE_USER@$REMOTE_HOST:$REMOTE_TMP"

ssh "$REMOTE_USER@$REMOTE_HOST" sh -eu <<EOF
sudo install -o root -g root -m 0755 "$REMOTE_TMP" /usr/local/bin/bookmarkd.new
sudo rm -f "$REMOTE_TMP"

if [ -x /usr/local/bin/bookmarkd ]; then
  sudo cp /usr/local/bin/bookmarkd /usr/local/bin/bookmarkd.previous
fi

sudo mv /usr/local/bin/bookmarkd.new /usr/local/bin/bookmarkd
sudo systemctl restart bookmarkd
sudo systemctl --no-pager --full status bookmarkd
EOF
```

Open question: if deployment is usually from macOS to Debian, this script is
probably enough. If deployment is usually performed on the VPS itself via
`git pull`, the better script would run on the VPS and build locally.

## Proposed Remote Bootstrap Script

This script would run once on a fresh Debian VPS. It should be reviewed
carefully before use because it creates users, directories, service files, and
nginx config.

```sh
#!/bin/sh
set -eu

DOMAIN="${BOOKMARKS_DOMAIN:?set BOOKMARKS_DOMAIN}"
TOKEN="${BOOKMARKS_TOKEN:?set BOOKMARKS_TOKEN}"

sudo useradd --system --home /var/lib/bookmarks --shell /usr/sbin/nologin bookmarks 2>/dev/null || true
sudo install -d -o bookmarks -g bookmarks -m 0750 /var/lib/bookmarks
sudo install -d -o root -g root -m 0750 /etc/bookmarks

sudo tee /etc/bookmarks/bookmarkd.env >/dev/null <<EOF
BOOKMARKS_ADDR=127.0.0.1:8080
BOOKMARKS_DBPATH=/var/lib/bookmarks/bookmarks.db
BOOKMARKS_TOKEN=$TOKEN
EOF

sudo chown root:root /etc/bookmarks/bookmarkd.env
sudo chmod 0600 /etc/bookmarks/bookmarkd.env

sudo tee /etc/systemd/system/bookmarkd.service >/dev/null <<'EOF'
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
EOF

sudo tee "/etc/nginx/sites-available/$DOMAIN" >/dev/null <<EOF
server {
    listen 80;
    server_name $DOMAIN;

    location / {
        proxy_pass http://127.0.0.1:8080;
        proxy_set_header Host \\\$host;
        proxy_set_header X-Real-IP \\\$remote_addr;
        proxy_set_header X-Forwarded-For \\\$proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto \\\$scheme;
    }
}
EOF

sudo ln -sf "/etc/nginx/sites-available/$DOMAIN" "/etc/nginx/sites-enabled/$DOMAIN"
sudo nginx -t
sudo systemctl reload nginx
sudo systemctl daemon-reload
sudo systemctl enable bookmarkd
```

This script intentionally does not install TLS. For Debian/nginx, certbot or an
existing ACME workflow should own certificate issuance and renewal.

## Proposed Health Checks

The app currently has authenticated API endpoints. For deployment verification,
the deploy script can use the existing list endpoint:

```sh
curl -fsS \
  -H "Authorization: Bearer $BOOKMARKS_TOKEN" \
  "$BOOKMARKS_URL/api/bookmarks" >/dev/null
```

Later, a `GET /healthz` endpoint could simplify system checks. It should return
only process/database health and should not expose bookmark data.

Possible behavior:

```text
GET /healthz -> 200 OK
```

If public and unauthenticated, it should reveal as little as possible. If
private and authenticated, it can include more diagnostics.

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

Before schema migrations exist, avoid deploying code that changes the database
shape without a database backup.

## Tickets

1. Document the current VPS deployment.

   Acceptance criteria:

   - The repo contains a deployment doc with the daemon env vars, systemd unit,
     nginx proxy shape, and manual verification command.
   - The doc names the expected data directory and env file paths.
   - The doc clearly states that `bookmarkd` binds to localhost only.

2. Add a local server build target.

   Acceptance criteria:

   - A Makefile or script can run tests and build a Linux binary for
     `bookmarkd`.
   - The build output goes under an ignored `dist/` directory.
   - The target supports the VPS architecture, either `amd64` or `arm64`.
   - `bookmarkctl` is not built as part of server deployment.

3. Add a deploy script for binary updates.

   Acceptance criteria:

   - The script copies a built `bookmarkd` binary to the VPS.
   - The script preserves the previous binary before replacing it.
   - The script restarts `bookmarkd` through systemd.
   - The script reports service status after restart.
   - The script does not print `BOOKMARKS_TOKEN`.

4. Add a one-time VPS bootstrap script.

   Acceptance criteria:

   - The script creates the `bookmarks` service user if missing.
   - The script creates `/var/lib/bookmarks` and `/etc/bookmarks` with safe
     ownership and permissions.
   - The script writes or updates the systemd unit.
   - The script writes or updates the nginx site.
   - The script validates nginx config before reload.
   - TLS setup remains explicit and documented rather than hidden.

5. Add deployment verification.

   Acceptance criteria:

   - The deploy workflow checks the API after restart.
   - Failed verification returns a non-zero exit code.
   - The check works with the current authenticated `/api/bookmarks` endpoint.

6. Add rollback documentation.

   Acceptance criteria:

   - The doc explains where the previous binary is stored.
   - The doc includes the exact rollback commands.
   - The deploy script and rollback doc agree on file paths.

7. Add database backup preflight.

   Acceptance criteria:

   - The deploy workflow either creates a SQLite backup before restart or
     explicitly calls a backup script.
   - Backup failures stop deployment.
   - The backup file name includes a timestamp.

8. Consider a health endpoint.

   Acceptance criteria:

   - Decide whether `GET /healthz` should be authenticated.
   - If implemented, tests cover success and database failure behavior.
   - Deployment verification can use `/healthz` without exposing bookmark data.
