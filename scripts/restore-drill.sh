#!/bin/sh
# Restore drill: copy the newest backup into a temporary directory, verify the
# copy with SQLite and bookmarkd's read-only schema check, and report the
# bookmark count. The live database and the backup files are never modified.
#
# usage: restore-drill.sh [BACKUP_DIR]
#
# Without BACKUP_DIR, the directory comes from BOOKMARKS_BACKUP_DIR (read from
# /etc/bookmarks/bookmarkd.env when present) or defaults to
# /var/backups/bookmarks. BOOKMARKD_BIN selects the bookmarkd binary used for
# the schema check (default /usr/local/bin/bookmarkd).
set -eu

if [ "$#" -gt 1 ]; then
	echo "usage: $0 [BACKUP_DIR]" >&2
	exit 2
fi

if [ "$#" -eq 0 ] && [ -r /etc/bookmarks/bookmarkd.env ]; then
	# shellcheck disable=SC1091
	. /etc/bookmarks/bookmarkd.env
fi
BACKUP_DIR="${1:-${BOOKMARKS_BACKUP_DIR:-/var/backups/bookmarks}}"
BOOKMARKD_BIN="${BOOKMARKD_BIN:-/usr/local/bin/bookmarkd}"

# Backup names embed a UTC timestamp, so the last match in glob order is newest.
latest=""
for file in "$BACKUP_DIR"/bookmarks-*.db; do
	if [ -f "$file" ]; then
		latest="$file"
	fi
done
if [ "$latest" = "" ]; then
	echo "no backups found in $BACKUP_DIR" >&2
	exit 1
fi

if [ -n "$(find "$latest" -mtime +1)" ]; then
	echo "warning: newest backup is more than two days old: $latest" >&2
fi

work_dir="$(mktemp -d "${TMPDIR:-/tmp}/bookmarks-restore-drill.XXXXXX")"
trap 'rm -rf "$work_dir"' EXIT
restored="$work_dir/bookmarks.db"
cp "$latest" "$restored"

echo "backup:         $latest"
echo "restored to:    $restored"

integrity="$(sqlite3 -readonly "$restored" 'PRAGMA integrity_check;')"
if [ "$integrity" != "ok" ]; then
	echo "integrity_check failed: $integrity" >&2
	exit 1
fi
echo "integrity:      ok"
echo "schema version: $(sqlite3 -readonly "$restored" 'PRAGMA user_version;')"
echo "bookmark rows:  $(sqlite3 -readonly "$restored" 'SELECT count(*) FROM bookmarks;')"

echo "schema check:   $BOOKMARKD_BIN check"
"$BOOKMARKD_BIN" check "$restored"

echo "restore drill passed"
