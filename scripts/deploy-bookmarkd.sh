#!/bin/sh
set -eu

usage() {
	echo "usage: $0 path/to/bookmarkd" >&2
	exit 2
}

if [ "$#" -ne 1 ]; then
	usage
fi

LOCAL_BIN="$1"
BOOKMARKS_DOMAIN="${BOOKMARKS_DOMAIN:-}"
REMOTE_HOST="${BOOKMARKS_DEPLOY_HOST:-$BOOKMARKS_DOMAIN}"
REMOTE_USER="${BOOKMARKS_DEPLOY_USER:-}"
BOOKMARKS_URL="${BOOKMARKS_URL:-}"

if [ "$REMOTE_HOST" = "" ]; then
	echo "set BOOKMARKS_DEPLOY_HOST or BOOKMARKS_DOMAIN" >&2
	exit 1
fi

if [ "$BOOKMARKS_URL" = "" ]; then
	if [ "$BOOKMARKS_DOMAIN" = "" ]; then
		echo "set BOOKMARKS_URL or BOOKMARKS_DOMAIN" >&2
		exit 1
	fi
	BOOKMARKS_URL="https://$BOOKMARKS_DOMAIN"
fi

if [ "$REMOTE_USER" != "" ]; then
	REMOTE="${REMOTE_USER}@${REMOTE_HOST}"
else
	REMOTE="$REMOTE_HOST"
fi
REMOTE_TMP="/tmp/bookmarkd.$$"
SERVICE_NAME="${BOOKMARKS_DEPLOY_SERVICE:-bookmarkd}"
REMOTE_TARGET="${BOOKMARKS_DEPLOY_TARGET:-/usr/local/bin/bookmarkd}"
VERIFY_URL="${BOOKMARKS_URL%/}/healthz"

if [ ! -f "$LOCAL_BIN" ]; then
	echo "binary does not exist: $LOCAL_BIN" >&2
	exit 1
fi

echo "copying $LOCAL_BIN to $REMOTE:$REMOTE_TMP"
scp "$LOCAL_BIN" "$REMOTE:$REMOTE_TMP"

cleanup_remote_tmp() {
	ssh "$REMOTE" "rm -f '$REMOTE_TMP'" >/dev/null 2>&1 || true
}
trap cleanup_remote_tmp EXIT HUP INT TERM

echo "installing and restarting $SERVICE_NAME on $REMOTE"
ssh "$REMOTE" sh -eu -s -- "$REMOTE_TMP" "$SERVICE_NAME" "$REMOTE_TARGET" <<'EOF'
remote_tmp="$1"
service_name="$2"
target="$3"
previous="${target}.previous"
new="${target}.new"

as_root() {
	if [ "$(id -u)" -eq 0 ]; then
		"$@"
	else
		sudo "$@"
	fi
}

# Read the same root-owned configuration used by the server and backup service.
db_path="$(as_root sh -eu -c '
	. /etc/bookmarks/bookmarkd.env
	printf "%s\n" "${BOOKMARKS_DBPATH:?BOOKMARKS_DBPATH must be configured}"
')"
case "$db_path" in
	/*) ;;
	./*) db_path="/var/lib/bookmarks/${db_path#./}" ;;
	*) db_path="/var/lib/bookmarks/$db_path" ;;
esac

if as_root test -f "$db_path"; then
	if ! as_root test -x /usr/local/sbin/bookmarks-backup; then
		echo "database exists but /usr/local/sbin/bookmarks-backup is missing or not executable" >&2
		exit 1
	fi
	# Refuse an unversioned production database before stopping the old server.
	version="$(as_root sqlite3 -readonly "$db_path" 'PRAGMA user_version;')"
	objects="$(as_root sqlite3 -readonly "$db_path" "SELECT count(*) FROM sqlite_schema WHERE substr(name, 1, 7) != 'sqlite_';")"
	if [ "$version" = "0" ] && [ "$objects" != "0" ]; then
		echo "database is unversioned; follow the adoption procedure in docs/operations.md before deploying" >&2
		exit 1
	fi
fi

as_root install -o root -g root -m 0755 "$remote_tmp" "$new"
as_root rm -f "$remote_tmp"

echo "stopping $service_name before backup and migration"
as_root systemctl stop "$service_name"
if as_root test -f "$db_path"; then
	echo "creating database backup before deploy"
	# Restart ensures a fresh backup even if a scheduled backup is already running.
	# The service supplies the database path and backup settings, including the
	# server's EnvironmentFile. A failure leaves the old binary installed and the
	# server stopped so the operator can investigate before trying again.
	as_root systemctl restart bookmarks-backup.service
else
	echo "database does not exist yet; skipping backup preflight"
fi

if [ -x "$target" ]; then
	echo "preserving previous binary at $previous"
	as_root cp "$target" "$previous"
fi

as_root mv "$new" "$target"
as_root systemctl start "$service_name"
as_root systemctl --no-pager --full status "$service_name"
EOF

echo "verifying $VERIFY_URL"
curl -fsS --retry 10 --retry-connrefused --retry-delay 1 --retry-max-time 30 --max-time 5 "$VERIFY_URL" >/dev/null

echo "deploy verified"
