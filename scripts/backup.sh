#!/usr/bin/env bash
# Bring a copy of the live database down to this machine.
#
# You do not need to run this on a schedule. Fly snapshots the volume every
# night and keeps a month, which is what covers the disk dying. This is for
# when you want the thing in your own hands: before a risky change, or so the
# customer list exists somewhere that is not Fly.
#
#   ./scripts/backup.sh            writes ./backups/queueup-<date>.db
set -euo pipefail
cd "$(dirname "$0")/.."

app="${FLY_APP:-queueup-relay}"
stamp="$(date -u +%Y-%m-%d-%H%M)"
remote="/data/queueup-$stamp.db"
mkdir -p backups
local_file="backups/queueup-$stamp.db"

echo "Asking the relay for a consistent copy..."
fly ssh console -a "$app" -C "/app/relay backup $remote" >/dev/null

echo "Downloading..."
fly ssh sftp get "$remote" "$local_file" -a "$app" >/dev/null

# Always clear the server copy, even if the download failed: a second copy of
# every customer sitting next to the live database is a second thing to lose.
echo "Clearing the server copy..."
fly ssh console -a "$app" -C "rm -f $remote" >/dev/null || true

size=$(ls -lh "$local_file" | awk '{print $5}')
echo
echo "Done. $local_file ($size)"
echo "Open it with any SQLite tool. The customer list is the accounts table."
