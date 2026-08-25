#!/usr/bin/env bash
# Plays every test scenario through the agent, back to back.
# Run from the project root:  ./scripts/demo.sh
set -uo pipefail
cd "$(dirname "$0")/.."

for f in testdata/scenarios/*.json; do
  name=$(basename "$f" .json)
  echo
  echo "=============================================================="
  echo " $name"
  echo "=============================================================="
  go run ./cmd/agent sim \
    --scenario "$f" \
    --server 51.83.128.10:28015 --log "/tmp/queueup-demo-$name.log" \
    --speed 6 --confirm 2s --jitter 300ms \
    --max-attempts 2 2>&1 | grep -v "^  WARNING\|^  Until\|^  Fix by\|^  steam_not_running"
done
echo
echo "All scenarios finished. Failures above are expected for the"
echo "server_full, rejected, steam_not_logged_in and crash scenarios."
