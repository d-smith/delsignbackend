#!/usr/bin/env bash
#
# Copy the client API docs (docs/api/) into a client repo, with a VERSION file
# naming the backend commit they came from.
#
#   scripts/export-client-docs.sh <client-repo-dir> [subdir]
#
# The files go to <client-repo-dir>/<subdir>, by default docs/delsign-api. That
# directory is replaced, so files removed here disappear there too.

set -euo pipefail

if [[ $# -lt 1 || $# -gt 2 ]]; then
	echo "usage: $0 <client-repo-dir> [subdir (default docs/delsign-api)]" >&2
	exit 2
fi

ROOT=$(cd "$(dirname "$0")/.." && pwd)
SRC=$ROOT/docs/api
CLIENT=$1
DEST=$CLIENT/${2:-docs/delsign-api}

[[ -d $CLIENT ]] || { echo "not a directory: $CLIENT" >&2; exit 1; }

commit=$(git -C "$ROOT" rev-parse HEAD)
# Uncommitted changes in docs/api mean the copy doesn't match that commit
if [[ -n $(git -C "$ROOT" status --porcelain -- docs/api) ]]; then
	commit="$commit (plus uncommitted changes to docs/api)"
fi

rm -rf "$DEST"
mkdir -p "$DEST"
cp "$SRC"/* "$DEST"/
cat >"$DEST/VERSION" <<EOF
Exported from delsignbackend docs/api/ by scripts/export-client-docs.sh.
Backend commit: $commit
Branch: $(git -C "$ROOT" rev-parse --abbrev-ref HEAD)
Exported: $(date -u +%Y-%m-%dT%H:%M:%SZ)
Don't edit these files here; export again from the backend.
EOF

echo "exported to $DEST:"
ls "$DEST"
