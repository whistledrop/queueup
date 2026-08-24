#!/usr/bin/env bash
# Builds the Windows agent from any machine. The result is one file,
# dist/QueueUpAgent.exe, with nothing to install alongside it.
set -euo pipefail
cd "$(dirname "$0")/.."

VERSION=$(git describe --tags --always 2>/dev/null || echo dev)

# Baked into the exe so the download knows where its own service lives and the
# user never has to type a URL.
RELAY_URL=${RELAY_URL:-https://queueup-relay.fly.dev}
WEB_URL=${WEB_URL:-https://queueuprust.netlify.app}

mkdir -p dist

CGO_ENABLED=0 GOOS=windows GOARCH=amd64 \
  go build -trimpath -ldflags "-s -w -H windowsgui \
    -X main.Version=$VERSION \
    -X main.DefaultRelayURL=$RELAY_URL \
    -X main.DefaultWebURL=$WEB_URL" \
  -o dist/QueueUpAgent.exe ./cmd/agent

# -H windowsgui stops a console window flashing up at login. The pair and
# status commands still work from a terminal; they just don't open one.
echo "built dist/QueueUpAgent.exe (version $VERSION)"
echo "  relay: $RELAY_URL"
echo "  web:   $WEB_URL"
ls -lh dist/QueueUpAgent.exe | awk '{print "size:", $5}'
