#!/bin/bash
set -euo pipefail

# Tarjeta de sonido virtual: Chrome reproduce en "reunion" y meetbot graba su monitor.
export XDG_RUNTIME_DIR="${XDG_RUNTIME_DIR:-/tmp/runtime-root}"
mkdir -p "$XDG_RUNTIME_DIR"
chmod 700 "$XDG_RUNTIME_DIR"

command -v pulseaudio >/dev/null 2>&1 || exit 0

pulseaudio --daemonize=yes --exit-idle-time=-1 --system=false --disallow-exit 2>/tmp/pulse.log || true

for i in $(seq 1 10); do
  pactl info >/dev/null 2>&1 && break
  sleep 0.5
done

pactl load-module module-null-sink sink_name=reunion sink_properties=device.description=Reunion >/dev/null 2>&1 || true
pactl set-default-sink reunion 2>/dev/null || true

# Micrófono virtual en silencio para que la videollamada no reclame por micrófono físico
pactl load-module module-null-sink sink_name=mic_silencio >/dev/null 2>&1 || true
pactl set-default-source mic_silencio.monitor 2>/dev/null || true

echo "[meetbot-audio] Tarjeta virtual lista: $(pactl get-default-sink 2>/dev/null || echo 'reunion')"
