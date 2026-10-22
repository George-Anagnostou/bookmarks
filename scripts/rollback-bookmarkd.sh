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

echo "restoring the previous binary for $SERVICE_NAME on $REMOTE"
echo "the database must already be compatible with that binary; see docs/operations.md"
ssh "$REMOTE" sh -eu -s -- "$SERVICE_NAME" "$REMOTE_TARGET" <<'EOF'
service_name="$1"
target="$2"
previous="${target}.previous"

as_root() {
	if [ "$(id -u)" -eq 0 ]; then
		"$@"
	else
		sudo "$@"
	fi
}

if [ ! -x "$previous" ]; then
	echo "previous binary is missing or not executable: $previous" >&2
	exit 1
fi

as_root systemctl stop "$service_name"
as_root install -o root -g root -m 0755 "$previous" "${target}.new"
as_root mv "${target}.new" "$target"
as_root systemctl start "$service_name"
as_root systemctl --no-pager --full status "$service_name"
EOF

echo "verifying $VERIFY_URL"
curl -fsS --retry 10 --retry-connrefused --retry-delay 1 --retry-max-time 30 --max-time 5 "$VERIFY_URL" >/dev/null

echo "rollback verified"
