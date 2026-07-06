#!/bin/sh
set -eu

DATA_DIR="/var/lib/bookmarks"
DB_PATH="${BOOKMARKS_DBPATH:-$DATA_DIR/bookmarks.db}"
BACKUP_DIR="${BOOKMARKS_BACKUP_DIR:-/var/backups/bookmarks}"
BACKUP_RETENTION_DAYS="${BOOKMARKS_BACKUP_RETENTION_DAYS:-30}"

if [ -r /etc/bookmarks/bookmarkd.env ]; then
	# shellcheck disable=SC1091
	. /etc/bookmarks/bookmarkd.env
	DB_PATH="${BOOKMARKS_DBPATH:-$DB_PATH}"
fi

case "$DB_PATH" in
	/*) ;;
	./*) DB_PATH="$DATA_DIR/${DB_PATH#./}" ;;
	*) DB_PATH="$DATA_DIR/$DB_PATH" ;;
esac

if [ "$(id -u)" -ne 0 ]; then
	echo "run this script as root on the Debian VPS" >&2
	exit 1
fi

echo "installing sqlite3 if needed"
apt-get update
apt-get install -y sqlite3

echo "creating backup directory"
install -d -o root -g root -m 0750 "$BACKUP_DIR"

echo "installing /usr/local/sbin/bookmarks-backup"
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

echo "installing systemd backup units"
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

echo "enabling backup timer"
systemctl daemon-reload
systemctl enable --now bookmarks-backup.timer

if [ -f "$DB_PATH" ]; then
	echo "running initial backup"
	systemctl start bookmarks-backup.service
	journalctl -u bookmarks-backup.service -n 5 --no-pager
else
	echo "database does not exist yet; timer enabled, first backup will run after data exists"
fi

echo "next scheduled backup:"
systemctl list-timers bookmarks-backup.timer --no-pager