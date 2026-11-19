package main

import (
	"encoding/json"
	"log"
	"net/http"
	"os"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"github.com/Andres-Ospino/meetbot/internal/bot"
)

type JoinRequest struct {
	MeetingURL  string `json:"meeting_url"`
	BotName     string `json:"bot_name,omitempty"`
	WebhookURL  string `json:"webhook_url,omitempty"`
	RecordVideo bool   `json:"record_video,omitempty"`
	MaxMinutes  int    `json:"max_minutes,omitempty"`
	ChatMessage string `json:"chat_message,omitempty"`
}

func main() {
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}
	apiKey := strings.TrimSpace(os.Getenv("MEETBOT_API_KEY"))

	engine := bot.NewEngine()

	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)

	r.Get("/healthz", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})

	r.Group(func(api chi.Router) {
		if apiKey != "" {
			api.Use(func(next http.Handler) http.Handler {
				return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					authHeader := r.Header.Get("Authorization")
					token := strings.TrimPrefix(authHeader, "Bearer ")
					if token != apiKey {
						writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "Unauthorized"})
						return
					}
					next.ServeHTTP(w, r)
				})
			})
		}

		api.Post("/v1/meetings/join", func(w http.ResponseWriter, r *http.Request) {
			var req JoinRequest
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Payload JSON inválido"})
				return
			}
			if req.MeetingURL == "" {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": "meeting_url es obligatorio"})
				return
			}

			cfg := bot.Config{
				MeetingURL:   req.MeetingURL,
				BotName:      req.BotName,
				WebhookURL:   req.WebhookURL,
				RecordVideo:  req.RecordVideo,
				MaxMinutes:   req.MaxMinutes,
				ChatMessage:  req.ChatMessage,
				CDPAddress:   os.Getenv("MEETBOT_CDP_ADDR"),
				ArtifactsDir: os.Getenv("MEETBOT_ARTIFACTS_DIR"),
				PulseMonitor: os.Getenv("MEETBOT_PULSE_MONITOR"),
				X11Display:   os.Getenv("DISPLAY"),
			}

			job, err := engine.Start(r.Context(), cfg)
			if err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
				return
			}

			writeJSON(w, http.StatusAccepted, job)
		})

		api.Get("/v1/meetings/{id}", func(w http.ResponseWriter, r *http.Request) {
			id := chi.URLParam(r, "id")
			job, ok := engine.Get(id)
			if !ok {
				writeJSON(w, http.StatusNotFound, map[string]string{"error": "Bot no encontrado"})
				return
			}
			writeJSON(w, http.StatusOK, job)
		})

		api.Post("/v1/meetings/{id}/leave", func(w http.ResponseWriter, r *http.Request) {
			id := chi.URLParam(r, "id")
			if engine.Stop(id) {
				writeJSON(w, http.StatusOK, map[string]any{"stopping": true})
			} else {
				writeJSON(w, http.StatusNotFound, map[string]string{"error": "Bot no encontrado o ya detenido"})
			}
		})
	})

	log.Printf("[meetbot-server] Servidor escuchando en :%s", port)
	if err := http.ListenAndServe(":"+port, r); err != nil {
		log.Fatalf("Error iniciando servidor: %v", err)
	}
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}
