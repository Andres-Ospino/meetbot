#!/bin/bash
set -euo pipefail

RESOLUTION="${RESOLUTION:-1440x900x24}"
export DISPLAY="${DISPLAY:-:99}"
export HOME=/root
export XDG_RUNTIME_DIR="${XDG_RUNTIME_DIR:-/tmp/runtime-root}"
mkdir -p "$XDG_RUNTIME_DIR" /root/.config /tmp/meetbot-artifacts
chmod 700 "$XDG_RUNTIME_DIR"

echo "[meetbot] Iniciando Xvfb en ${DISPLAY} (${RESOLUTION})..."
Xvfb :99 -screen 0 "$RESOLUTION" -ac +extension RANDR +extension GLX &
sleep 2

# Puertos CDP: Chrome escucha internamente en 127.0.0.1:9229; publicamos en 0.0.0.0:9222
socat TCP-LISTEN:9222,fork,reuseaddr,bind=0.0.0.0 TCP:127.0.0.1:9229 &

# Configuración de audio virtual
/usr/local/bin/start-audio.sh || echo "[meetbot] Audio virtual no pudo inicializarse"

# Iniciar Chrome
/usr/local/bin/launch-chrome.sh "about:blank"
sleep 2

# Ejecutar el comando especificado o por defecto el servidor HTTP
if [ "$#" -gt 0 ]; then
  exec "$@"
else
  exec /usr/local/bin/meetbot-server
fi
