#!/bin/bash
set -euo pipefail

export DISPLAY="${DISPLAY:-:99}"
URL="${1:-about:blank}"

EXTRA_FLAGS=(
  --use-fake-ui-for-media-stream
  --autoplay-policy=no-user-gesture-required
  --lang=es-419
  --accept-lang=es-419,es,en
  --no-sandbox
  --disable-gpu
  --disable-dev-shm-usage
  --no-first-run
  --disable-session-crashed-bubble
  --start-maximized
  --remote-debugging-port=9229
  --user-data-dir=/root/.config/meetbot-chrome
)

if command -v google-chrome >/dev/null 2>&1; then
  google-chrome "${EXTRA_FLAGS[@]}" "$URL" >/tmp/chrome.log 2>&1 &
elif command -v chromium-browser >/dev/null 2>&1; then
  chromium-browser "${EXTRA_FLAGS[@]}" "$URL" >/tmp/chrome.log 2>&1 &
elif command -v chromium >/dev/null 2>&1; then
  chromium "${EXTRA_FLAGS[@]}" "$URL" >/tmp/chrome.log 2>&1 &
fi

echo "[meetbot-chrome] Chrome iniciado en background (depuración en :9229)"
