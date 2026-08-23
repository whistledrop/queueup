#!/usr/bin/env bash
# Copies the canonical data files into the agent binary's embedded set.
# Run after editing configs/patterns.json or adding a scenario; a test fails
# loudly if you forget.
set -euo pipefail
cd "$(dirname "$0")/.."
cp configs/patterns.json internal/embedded/patterns.json
rm -f internal/embedded/scenarios/*.json
cp testdata/scenarios/*.json internal/embedded/scenarios/
echo "embedded data synced"
