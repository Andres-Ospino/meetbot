# Etapa 1: Compilación de binarios Go
FROM golang:1.25-alpine AS builder

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o /bin/meetbot-server ./cmd/server
RUN CGO_ENABLED=0 GOOS=linux go build -o /bin/meetbot ./cmd/meetbot

# Etapa 2: Imagen final con entorno gráfico virtual, Chrome, PulseAudio y ffmpeg
FROM ubuntu:22.04

ENV DEBIAN_FRONTEND=noninteractive
ENV DISPLAY=:99
ENV RESOLUTION=1440x900x24
ENV LANG=C.UTF-8
ENV MEETBOT_CDP_ADDR=127.0.0.1:9222
ENV MEETBOT_PULSE_MONITOR=reunion.monitor
ENV MEETBOT_ARTIFACTS_DIR=/recordings

RUN apt-get update && apt-get install -y --no-install-recommends \
    xvfb \
    xdotool \
    pulseaudio \
    pulseaudio-utils \
    ffmpeg \
    socat \
    curl \
    wget \
    ca-certificates \
    gnupg \
    fonts-liberation \
    fonts-noto-core \
    procps \
    && rm -rf /var/lib/apt/lists/*

# Instalación oficial de Google Chrome estable
RUN wget -q -O - https://dl.google.com/linux/linux_signing_key.pub \
      | gpg --dearmor -o /usr/share/keyrings/google-linux-signing-keyring.gpg \
    && echo "deb [arch=amd64 signed-by=/usr/share/keyrings/google-linux-signing-keyring.gpg] http://dl.google.com/linux/chrome/deb/ stable main" \
      > /etc/apt/sources.list.d/google-chrome.list \
    && apt-get update \
    && apt-get install -y --no-install-recommends google-chrome-stable \
    && rm -rf /var/lib/apt/lists/*

# Copiar binarios y scripts
COPY --from=builder /bin/meetbot-server /usr/local/bin/meetbot-server
COPY --from=builder /bin/meetbot /usr/local/bin/meetbot

COPY docker/start-audio.sh /usr/local/bin/start-audio.sh
COPY docker/launch-chrome.sh /usr/local/bin/launch-chrome.sh
COPY docker/entrypoint.sh /entrypoint.sh

RUN chmod +x /usr/local/bin/start-audio.sh \
             /usr/local/bin/launch-chrome.sh \
             /entrypoint.sh \
             /usr/local/bin/meetbot-server \
             /usr/local/bin/meetbot \
    && mkdir -p /recordings

VOLUME ["/recordings"]
EXPOSE 8080 9222

ENTRYPOINT ["/entrypoint.sh"]
CMD ["/usr/local/bin/meetbot-server"]
