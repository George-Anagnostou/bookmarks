#!/bin/sh
# Run scripts/restore-drill.sh on the VPS as root over SSH.
set -eu

BOOKMARKS_DOMAIN="${BOOKMARKS_DOMAIN:-}"
REMOTE_HOST="${BOOKMARKS_DEPLOY_HOST:-$BOOKMARKS_DOMAIN}"
REMOTE_USER="${BOOKMARKS_DEPLOY_USER:-}"

if [ "$REMOTE_HOST" = "" ]; then
	echo "set BOOKMARKS_DEPLOY_HOST or BOOKMARKS_DOMAIN" >&2
	exit 1
fi

if [ "$REMOTE_USER" != "" ]; then
	REMOTE="${REMOTE_USER}@${REMOTE_HOST}"
else
	REMOTE="$REMOTE_HOST"
fi

SCRIPT_DIR="$(CDPATH='' cd -- "$(dirname -- "$0")" && pwd)"

echo "running restore drill on $REMOTE"
# The single-quoted command runs on the remote host.
# shellcheck disable=SC2016
ssh "$REMOTE" 'if [ "$(id -u)" -eq 0 ]; then sh -eu -s; else sudo sh -eu -s; fi' <"$SCRIPT_DIR/restore-drill.sh"
