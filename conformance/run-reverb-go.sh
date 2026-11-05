#!/usr/bin/env bash
# Runs the suite against reverb-go. Builds the binary first unless BIN is set.
set -euo pipefail
here="$(cd "$(dirname "$0")" && pwd)"
root="$(dirname "$here")"
PORT="${CONF_PORT:-6002}"
BIN="${BIN:-$(mktemp -d)/reverb-go}"
[ -x "$BIN" ] || (cd "$root" && go build -o "$BIN" ./cmd/reverb-go)

export CONF_TARGET=reverb-go CONF_HOST=127.0.0.1 CONF_PORT="$PORT"
export CONF_APP_ID=conf-app CONF_APP_KEY=conf-key CONF_APP_SECRET=conf-secret

REVERB_APP_ID=$CONF_APP_ID REVERB_APP_KEY=$CONF_APP_KEY REVERB_APP_SECRET=$CONF_APP_SECRET \
  REVERB_SERVER_HOST=127.0.0.1 REVERB_SERVER_PORT="$PORT" "$BIN" &
pid=$!
trap 'kill "$pid" 2>/dev/null || true; wait "$pid" 2>/dev/null || true' EXIT

for _ in $(seq 1 100); do
  curl -fs "http://127.0.0.1:$PORT/up" >/dev/null 2>&1 && break
  sleep 0.1
done
curl -fs "http://127.0.0.1:$PORT/up" >/dev/null || { echo "reverb-go did not start"; exit 1; }

cd "$here"
npm test
