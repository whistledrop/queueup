#!/usr/bin/env bash
# A throwaway local relay for trying the website against, with a fresh
# database each run and a known admin token. Never touches production.
#   ./scripts/dev-relay.sh            listens on :8090
set -euo pipefail
cd "$(dirname "$0")/.."
db="$(mktemp -d)/queueup-dev.db"
export QUEUEUP_ADDR="${QUEUEUP_ADDR:-:8090}"
export QUEUEUP_DB="$db"
export QUEUEUP_ADMIN_TOKEN="${QUEUEUP_ADMIN_TOKEN:-dev-admin}"
export QUEUEUP_SERVER_SOURCE="${QUEUEUP_SERVER_SOURCE:-stub}"
echo "dev relay on $QUEUEUP_ADDR, database $db, admin token $QUEUEUP_ADMIN_TOKEN"
exec go run ./cmd/relay serve
