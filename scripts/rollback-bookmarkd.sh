#!/bin/sh
set -eu

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
SERVICE_NAME="${BOOKMARKS_DEPLOY_SERVICE:-bookmarkd}"
REMOTE_TARGET="${BOOKMARKS_DEPLOY_TARGET:-/usr/local/bin/bookmarkd}"
VERIFY_URL="${BOOKMARKS_URL%/}/healthz"

# remote ACTION SERVICE TARGET [MODE] runs the rollback body on the VPS.
# "plan" only reads state and prints mode=binary, mode=restore, or mode=unknown.
# "apply MODE" recomputes the plan, refuses if it no longer matches MODE, and
# then rolls back.
remote() {
	ssh "$REMOTE" sh -eu -s -- "$@" <<'EOF'
action="$1"
service_name="$2"
target="$3"
requested_mode="${4:-}"
previous="${target}.previous"

as_root() {
	if [ "$(id -u)" -eq 0 ]; then
		"$@"
	else
		sudo "$@"
	fi
}

fail() {
	echo "$*" >&2
	exit 1
}

require_version() {
	case "$2" in
		"" | *[!0-9]*) fail "invalid $1 schema version: '$2'" ;;
	esac
}

config="$(as_root sh -eu -c '
	. /etc/bookmarks/bookmarkd.env
	printf "%s\n" "${BOOKMARKS_DBPATH:?BOOKMARKS_DBPATH must be configured}"
	printf "%s\n" "${BOOKMARKS_BACKUP_DIR:-/var/backups/bookmarks}"
')"
db_path="$(printf '%s\n' "$config" | sed -n 1p)"
backup_dir="$(printf '%s\n' "$config" | sed -n 2p)"
case "$db_path" in
	/*) ;;
	./*) db_path="/var/lib/bookmarks/${db_path#./}" ;;
	*) db_path="/var/lib/bookmarks/$db_path" ;;
esac
saved_db="$backup_dir/pre-deploy.db"
record="$backup_dir/pre-deploy.env"

if [ ! -x "$previous" ]; then
	fail "previous binary is missing or not executable: $previous"
fi

# Decide whether the previous binary can use the current database as is.
mode=binary
if ! as_root test -f "$db_path"; then
	echo "no database at $db_path; restoring the binary only"
elif ! as_root test -f "$record"; then
	echo "no pre-deploy record at $record; cannot tell which schema the previous binary supports"
	mode=unknown
else
	current_version="$(as_root sqlite3 -readonly "$db_path" 'PRAGMA user_version;')"
	saved_version="$(as_root sed -n 's/^PRE_DEPLOY_SCHEMA_VERSION=//p' "$record")"
	require_version current "$current_version"
	require_version pre-deploy "$saved_version"
	echo "current database schema version: $current_version"
	echo "pre-deploy schema version:       $saved_version"
	if [ "$current_version" -gt "$saved_version" ]; then
		mode=restore
		as_root test -f "$saved_db" || fail "pre-deploy backup is missing: $saved_db"
		integrity="$(as_root sqlite3 -readonly "$saved_db" 'PRAGMA integrity_check;')"
		[ "$integrity" = "ok" ] || fail "pre-deploy backup failed integrity_check: $integrity"
		backup_version="$(as_root sqlite3 -readonly "$saved_db" 'PRAGMA user_version;')"
		[ "$backup_version" = "$saved_version" ] ||
			fail "pre-deploy backup has schema version $backup_version, record says $saved_version"
		echo "the database was migrated; rollback must restore $saved_db"
		echo "pre-deploy backup taken from: $(as_root sed -n 's/^PRE_DEPLOY_BACKUP=//p' "$record")"
	else
		echo "the database is compatible with the previous binary; restoring the binary only"
	fi
fi

if [ "$action" = "plan" ]; then
	echo "mode=$mode"
	exit 0
fi
[ "$action" = "apply" ] || fail "unknown action: $action"

# A forced binary-only rollback is the only accepted override of "unknown".
if [ "$mode" != "$requested_mode" ] && ! { [ "$mode" = "unknown" ] && [ "$requested_mode" = "binary" ]; }; then
	fail "rollback plan changed (now $mode, requested $requested_mode); rerun make rollback"
fi

as_root systemctl stop "$service_name"

if [ "$requested_mode" = "restore" ]; then
	as_root systemctl stop bookmarks-backup.timer bookmarks-backup.service
	safety_dir="$(as_root mktemp -d "$backup_dir/rollback-XXXXXXXX")"
	for file in "$db_path" "$db_path-wal" "$db_path-shm"; do
		if as_root test -e "$file"; then
			as_root mv "$file" "$safety_dir/"
		fi
	done
	echo "moved current database files to $safety_dir"
	as_root install -o bookmarks -g bookmarks -m 0640 "$saved_db" "$db_path"
	echo "restored $saved_db to $db_path"
fi

as_root cp "$target" "${target}.rolled-back"
as_root install -o root -g root -m 0755 "$previous" "${target}.new"
as_root mv "${target}.new" "$target"
echo "installed $previous; the replaced binary is at ${target}.rolled-back"
as_root systemctl start "$service_name"
if [ "$requested_mode" = "restore" ]; then
	as_root systemctl start bookmarks-backup.timer
fi
as_root systemctl --no-pager --full status "$service_name"
EOF
}

echo "checking rollback state for $SERVICE_NAME on $REMOTE"
plan="$(remote plan "$SERVICE_NAME" "$REMOTE_TARGET")"
printf '%s\n' "$plan"
mode="$(printf '%s\n' "$plan" | sed -n 's/^mode=//p')"

case "$mode" in
	binary) ;;
	restore)
		echo
		echo "WARNING: this replaces the live database with the pre-deploy backup."
		echo "Changes made since that backup are lost from the live database; the"
		echo "current database files are kept in a rollback-* directory for inspection."
		if [ "${BOOKMARKS_ROLLBACK_CONFIRM:-}" != "restore" ]; then
			printf 'Type "restore" to continue: '
			read -r answer || answer=""
			if [ "$answer" != "restore" ]; then
				echo "rollback cancelled" >&2
				exit 1
			fi
		fi
		;;
	unknown)
		if [ "${BOOKMARKS_ROLLBACK_MODE:-}" != "binary" ]; then
			echo "refusing to roll back without a pre-deploy record. If the previous binary" >&2
			echo "supports the current schema, rerun with BOOKMARKS_ROLLBACK_MODE=binary;" >&2
			echo "otherwise follow the restore procedure in docs/operations.md." >&2
			exit 1
		fi
		mode=binary
		;;
	*)
		echo "could not determine rollback mode" >&2
		exit 1
		;;
esac

echo "rolling back $SERVICE_NAME on $REMOTE (mode: $mode)"
remote apply "$SERVICE_NAME" "$REMOTE_TARGET" "$mode"

echo "verifying $VERIFY_URL"
curl -fsS --retry 10 --retry-connrefused --retry-delay 1 --retry-max-time 30 --max-time 5 "$VERIFY_URL" >/dev/null

echo "rollback verified"
