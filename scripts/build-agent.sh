#!/usr/bin/env bash
# Builds the Windows agent from any machine. The result is one file,
# dist/QueueUpAgent.exe, with nothing to install alongside it.
set -euo pipefail
cd "$(dirname "$0")/.."

VERSION=$(git describe --tags --always 2>/dev/null || echo dev)
mkdir -p dist

CGO_ENABLED=0 GOOS=windows GOARCH=amd64 \
  go build -trimpath -ldflags "-s -w -H windowsgui -X main.Version=$VERSION" \
  -o dist/QueueUpAgent.exe ./cmd/agent

# -H windowsgui stops a console window flashing up at login. The pair and
# status commands still work from a terminal; they just don't open one.
echo "built dist/QueueUpAgent.exe (version $VERSION)"
ls -lh dist/QueueUpAgent.exe | awk '{print "size:", $5}'
