#!/bin/sh
set -eu

DOMAIN="${BOOKMARKS_DOMAIN:?set BOOKMARKS_DOMAIN}"
TOKEN="${BOOKMARKS_TOKEN:?set BOOKMARKS_TOKEN}"
ADDR="${BOOKMARKS_ADDR:-127.0.0.1:8080}"
DB_PATH="${BOOKMARKS_DBPATH:-/var/lib/bookmarks/bookmarks.db}"
BACKUP_DIR="${BOOKMARKS_BACKUP_DIR:-/var/backups/bookmarks}"
BACKUP_RETENTION_DAYS="${BOOKMARKS_BACKUP_RETENTION_DAYS:-30}"

if [ "$(id -u)" -ne 0 ]; then
	echo "run this script as root on the Debian VPS" >&2
	exit 1
fi

case "$DOMAIN" in
	"" | *[!A-Za-z0-9._-]*)
		echo "BOOKMARKS_DOMAIN may only contain letters, numbers, dots, underscores, and hyphens" >&2
		exit 1
		;;
esac

echo "installing Debian packages"
apt-get update
apt-get install -y nginx sqlite3

echo "creating service user and directories"
useradd --system --home /var/lib/bookmarks --shell /usr/sbin/nologin bookmarks 2>/dev/null || true
install -d -o bookmarks -g bookmarks -m 0750 /var/lib/bookmarks
install -d -o root -g root -m 0750 /etc/bookmarks
install -d -o root -g root -m 0750 "$BACKUP_DIR"

echo "writing /etc/bookmarks/bookmarkd.env"
cat >/etc/bookmarks/bookmarkd.env <<EOF
BOOKMARKS_ADDR=$ADDR
BOOKMARKS_DBPATH=$DB_PATH
BOOKMARKS_TOKEN=$TOKEN
EOF
chown root:root /etc/bookmarks/bookmarkd.env
chmod 0600 /etc/bookmarks/bookmarkd.env

echo "installing bookmarkd systemd unit"
cat >/etc/systemd/system/bookmarkd.service <<'EOF'
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

echo "installing backup script"
cat >/usr/local/sbin/bookmarks-backup <<'EOF'
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
EOF
chown root:root /usr/local/sbin/bookmarks-backup
chmod 0750 /usr/local/sbin/bookmarks-backup

echo "installing backup systemd units"
cat >/etc/systemd/system/bookmarks-backup.service <<EOF
[Unit]
Description=Back up bookmark SQLite database

[Service]
Type=oneshot
Environment=BOOKMARKS_DBPATH=$DB_PATH
Environment=BOOKMARKS_BACKUP_DIR=$BACKUP_DIR
Environment=BOOKMARKS_BACKUP_RETENTION_DAYS=$BACKUP_RETENTION_DAYS
ExecStart=/usr/local/sbin/bookmarks-backup
EOF

cat >/etc/systemd/system/bookmarks-backup.timer <<'EOF'
[Unit]
Description=Run bookmark database backup daily

[Timer]
OnCalendar=*-*-* 03:15:00
Persistent=true
RandomizedDelaySec=15m

[Install]
WantedBy=timers.target
EOF

echo "installing nginx site for $DOMAIN"
cat >"/etc/nginx/sites-available/$DOMAIN" <<EOF
server {
    listen 80;
    server_name $DOMAIN;

    location / {
        proxy_pass http://$ADDR;
        proxy_set_header Host \$host;
        proxy_set_header X-Real-IP \$remote_addr;
        proxy_set_header X-Forwarded-For \$proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto \$scheme;
    }
}
EOF

ln -sf "/etc/nginx/sites-available/$DOMAIN" "/etc/nginx/sites-enabled/$DOMAIN"
nginx -t
systemctl reload nginx || systemctl restart nginx

echo "reloading systemd and enabling services"
systemctl daemon-reload
systemctl enable bookmarkd
systemctl enable --now bookmarks-backup.timer

echo "bootstrap complete"
echo "install /usr/local/bin/bookmarkd with the deploy script, then configure TLS for $DOMAIN"
