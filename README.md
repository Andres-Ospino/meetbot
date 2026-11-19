# MeetBot

MeetBot es un asistente autónomo y autoalojable para reuniones virtuales (Google Meet, Zoom y Microsoft Teams). Se une a las llamadas como un participante más, captura el audio en formato de alta fidelidad (Opus/WAV), video opcional (MP4) y entrega los resultados mediante Webhooks o CLI local.

Es una alternativa 100% abierta y sin costos por minuto a servicios como Recall.ai o Vexa.

---

## Características

- **Sin dependencias de pago**: Control nativo de Chrome mediante Chrome DevTools Protocol (CDP) sobre WebSockets.
- **Audio y video virtual**: Tarjeta de sonido virtual con PulseAudio y grabación con ffmpeg.
- **Detección inteligente**: Heurísticas multilingües (español e inglés) para sala de espera, pantalla previa y botones de acceso.
- **Asistencia visual opcional**: Fallback con modelos de visión (compatible con cualquier API tipo OpenAI/Ollama) si la interfaz cambia.
- **Entrega por Webhooks**: Notificaciones de estado (`bot.started`, `bot.joined`, `bot.ended`) y envío multipart de los archivos grabados.
- **Modos de uso**: Binario CLI interactivo o servidor API REST.

---

## Estructura del Proyecto

```text
meetbot/
├── cmd/
│   ├── meetbot/         # CLI para ejecución local desde terminal
│   └── server/          # Servidor HTTP REST para orquestar bots
├── internal/
│   ├── bot/             # Máquina de estados y motor de la reunión
│   ├── cdp/             # Cliente WebSocket nativo para Chrome DevTools Protocol
│   ├── media/           # Grabador de audio (PulseAudio) y video con ffmpeg
│   ├── ai/              # Proveedor opcional de visión/fallback
│   └── webhook/         # Despacho de eventos y entrega de archivos multipart
├── docker/
│   ├── Dockerfile       # Imagen lista con Chrome, PulseAudio, ffmpeg y meetbot
│   └── docker-compose.yml
└── Makefile
```

---

## Uso Rápido con Docker

```bash
cd docker
docker compose up -d
```

### Iniciar un bot vía API REST

```bash
curl -X POST http://localhost:8080/v1/meetings/join \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer secret-token-123" \
  -d '{
    "meeting_url": "https://meet.google.com/abc-defg-hij",
    "bot_name": "Asistente de Notas",
    "webhook_url": "https://mi-servidor.com/api/webhook",
    "record_video": false,
    "max_minutes": 60
  }'
```

### Consultar estado

```bash
curl http://localhost:8080/v1/meetings/bot_915d2866 \
  -H "Authorization: Bearer secret-token-123"
```

### Retirar al bot

```bash
curl -X POST http://localhost:8080/v1/meetings/bot_915d2866/leave \
  -H "Authorization: Bearer secret-token-123"
```

---

## Compilación Local

Requisitos: Go 1.25+, Chrome/Chromium, ffmpeg, PulseAudio.

```bash
# Compilar binarios
make build

# Ejecutar pruebas unitarias
make test
```
