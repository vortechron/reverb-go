#!/usr/bin/env bash
# Runs the suite against real Laravel Reverb, started from the werewolf app
# without editing any file there: only environment overrides.
set -euo pipefail
here="$(cd "$(dirname "$0")" && pwd)"
APP="${WEREWOLF_DIR:-/Users/amiruladli/Projects/werewolf}"
PORT="${CONF_PORT:-6001}"
export CONF_TARGET=laravel-reverb CONF_HOST=127.0.0.1 CONF_PORT="$PORT"
export CONF_APP_ID=conf-app CONF_APP_KEY=conf-key CONF_APP_SECRET=conf-secret

log="$(mktemp -t reverb-conf.XXXXXX)"
(
  cd "$APP"
  env REVERB_APP_ID=$CONF_APP_ID REVERB_APP_KEY=$CONF_APP_KEY REVERB_APP_SECRET=$CONF_APP_SECRET \
      REVERB_SCALING_ENABLED=false REVERB_APP_ACCEPT_CLIENT_EVENTS_FROM=members \
      APP_CONFIG_CACHE=/tmp/reverb-conf-missing-config.php APP_ENV=local \
      php artisan reverb:start --host=127.0.0.1 --port="$PORT"
) >"$log" 2>&1 &
pid=$!
trap 'kill "$pid" 2>/dev/null || true; wait "$pid" 2>/dev/null || true' EXIT

for _ in $(seq 1 100); do
  curl -fs "http://127.0.0.1:$PORT/up" >/dev/null 2>&1 && break
  sleep 0.2
done
curl -fs "http://127.0.0.1:$PORT/up" >/dev/null || { echo "Reverb did not start"; cat "$log"; exit 1; }

cd "$here"
npm test
