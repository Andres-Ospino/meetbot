package bot

import (
	"context"
	"fmt"
	"log"
	"net/url"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/meetbot/meetbot/internal/ai"
	"github.com/meetbot/meetbot/internal/cdp"
	"github.com/meetbot/meetbot/internal/media"
	"github.com/meetbot/meetbot/internal/webhook"
)

type Config struct {
	BotName         string
	MeetingURL      string
	WebhookURL      string
	RecordVideo     bool
	MaxMinutes      int
	CDPAddress      string
	ArtifactsDir    string
	PulseMonitor    string
	X11Display      string
	VisionHelper    ai.VisionHelper
	ChatMessage     string // Mensaje opcional a enviar en el chat de la llamada al unirse
}

type JobStatus string

const (
	StatusStarting JobStatus = "starting"
	StatusJoining  JobStatus = "joining"
	StatusWaiting  JobStatus = "waiting"
	StatusInCall   JobStatus = "in_call"
	StatusLeaving  JobStatus = "leaving"
	StatusFinished JobStatus = "finished"
	StatusFailed   JobStatus = "failed"
)

type Job struct {
	ID           string     `json:"id"`
	Config       Config     `json:"config"`
	Status       JobStatus  `json:"status"`
	Message      string     `json:"message"`
	StartedAt    time.Time  `json:"started_at"`
	JoinedAt     *time.Time `json:"joined_at,omitempty"`
	FinishedAt   *time.Time `json:"finished_at,omitempty"`
	Participants []string   `json:"participants"`
	cancelFn     context.CancelFunc
	mu           sync.RWMutex
}

type Engine struct {
	jobs       map[string]*Job
	dispatcher *webhook.Dispatcher
	mu         sync.RWMutex
}

func NewEngine() *Engine {
	return &Engine{
		jobs:       make(map[string]*Job),
		dispatcher: webhook.NewDispatcher(),
	}
}

func (e *Engine) Start(ctx context.Context, cfg Config) (*Job, error) {
	if cfg.BotName == "" {
		cfg.BotName = "Meeting Notetaker"
	}
	if cfg.MaxMinutes <= 0 {
		cfg.MaxMinutes = 180
	}
	if cfg.CDPAddress == "" {
		cfg.CDPAddress = "localhost:9222"
	}
	if cfg.ArtifactsDir == "" {
		cfg.ArtifactsDir = "/tmp/meetbot-artifacts"
	}
	if cfg.VisionHelper == nil {
		cfg.VisionHelper = ai.NewOpenAIHelperFromEnv()
	}

	u, err := url.Parse(strings.TrimSpace(cfg.MeetingURL))
	if err != nil || u.Scheme == "" || u.Host == "" {
		return nil, fmt.Errorf("URL de reunión inválida: %s", cfg.MeetingURL)
	}

	jobID := "bot_" + uuid.New().String()[:8]
	jobCtx, cancel := context.WithTimeout(context.Background(), time.Duration(cfg.MaxMinutes)*time.Minute)

	job := &Job{
		ID:           jobID,
		Config:       cfg,
		Status:       StatusStarting,
		Message:      "Iniciando asistente...",
		StartedAt:    time.Now().UTC(),
		Participants: make([]string, 0),
		cancelFn:     cancel,
	}

	e.mu.Lock()
	e.jobs[jobID] = job
	e.mu.Unlock()

	go e.run(jobCtx, job)

	return job, nil
}

func (e *Engine) Get(id string) (*Job, bool) {
	e.mu.RLock()
	defer e.mu.RUnlock()
	j, ok := e.jobs[id]
	return j, ok
}

func (e *Engine) Stop(id string) bool {
	e.mu.RLock()
	j, ok := e.jobs[id]
	e.mu.RUnlock()
	if !ok {
		return false
	}
	j.mu.Lock()
	defer j.mu.Unlock()
	if j.cancelFn != nil {
		j.cancelFn()
		j.Status = StatusLeaving
		j.Message = "Salida solicitada manualmente."
		return true
	}
	return false
}

func (e *Engine) run(ctx context.Context, job *Job) {
	defer func() {
		job.mu.Lock()
		now := time.Now().UTC()
		job.FinishedAt = &now
		if job.Status != StatusFailed {
			job.Status = StatusFinished
		}
		job.mu.Unlock()
	}()

	log.Printf("[%s] Conectando a navegador vía CDP (%s)...", job.ID, job.Config.CDPAddress)
	var browser cdp.Browser
	var err error

	// Intentar conectar al navegador con reintentos
	for i := 0; i < 15; i++ {
		browser, err = cdp.Connect(ctx, job.Config.CDPAddress)
		if err == nil {
			break
		}
		select {
		case <-ctx.Done():
			job.mu.Lock()
			job.Status = StatusFailed
			job.Message = "Cancelado antes de conectar con el navegador."
			job.mu.Unlock()
			return
		case <-time.After(1 * time.Second):
		}
	}
	if err != nil {
		job.mu.Lock()
		job.Status = StatusFailed
		job.Message = fmt.Sprintf("No se pudo conectar a Chrome CDP: %v", err)
		job.mu.Unlock()
		return
	}
	defer browser.Close()

	// Preparar grabador
	audioFile := filepath.Join(job.Config.ArtifactsDir, job.ID, "meeting.opus")
	videoFile := filepath.Join(job.Config.ArtifactsDir, job.ID, "meeting.mp4")
	rec := media.NewRecorder(audioFile, videoFile)

	// Iniciar grabación de audio
	if err := rec.StartAudio(job.Config.PulseMonitor); err != nil {
		log.Printf("[%s] Advertencia: grabador de audio no pudo iniciar: %v", job.ID, err)
	}
	if job.Config.RecordVideo {
		_ = rec.StartVideo(job.Config.X11Display)
	}
	defer rec.Stop()

	_ = e.dispatcher.SendEvent(ctx, job.Config.WebhookURL, webhook.EventPayload{
		Event:      webhook.EventStarted,
		BotID:      job.ID,
		MeetingURL: job.Config.MeetingURL,
		Status:     string(StatusStarting),
		Timestamp:  time.Now().UTC(),
	})

	// Ciclo de entrada a la reunión
	if err := e.joinLoop(ctx, job, browser); err != nil {
		job.mu.Lock()
		job.Status = StatusFailed
		job.Message = fmt.Sprintf("Fallo al intentar unirse: %v", err)
		job.mu.Unlock()
		return
	}

	// Reunión activa: monitoreo continuo
	now := time.Now().UTC()
	job.mu.Lock()
	job.JoinedAt = &now
	job.Status = StatusInCall
	job.Message = "Asistente dentro de la llamada."
	job.mu.Unlock()

	_ = e.dispatcher.SendEvent(ctx, job.Config.WebhookURL, webhook.EventPayload{
		Event:      webhook.EventJoined,
		BotID:      job.ID,
		MeetingURL: job.Config.MeetingURL,
		Status:     string(StatusInCall),
		Timestamp:  now,
	})

	// Acciones al ingresar
	if job.Config.ChatMessage != "" {
		_, _ = browser.SendChat(ctx, job.Config.ChatMessage)
	}
	browser.EnableCaptions(ctx)

	// Bucle de estancia en la reunión
	e.monitorCall(ctx, job, browser)

	// Detener grabador y entregar resultados
	rec.Stop()

	audioData, _ := rec.ReadAudio()
	var videoData []byte
	if job.Config.RecordVideo {
		videoData, _ = rec.ReadVideo()
	}

	durationSec := int(time.Since(now).Seconds())
	payload := webhook.EventPayload{
		Event:        webhook.EventEnded,
		BotID:        job.ID,
		MeetingURL:   job.Config.MeetingURL,
		Status:       string(StatusFinished),
		DurationSec:  durationSec,
		Participants: job.Participants,
		Timestamp:    time.Now().UTC(),
	}

	if err := e.dispatcher.DeliverArtifacts(context.Background(), job.Config.WebhookURL, payload, audioData, "meeting.opus", videoData, "meeting.mp4"); err != nil {
		log.Printf("[%s] Error entregando artefactos finales por webhook: %v", job.ID, err)
	} else {
		log.Printf("[%s] Entrega de reunión completada con éxito.", job.ID)
	}
}

func (e *Engine) joinLoop(ctx context.Context, job *Job, b cdp.Browser) error {
	deadline := time.Now().Add(4 * time.Minute)
	for time.Now().Before(deadline) {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(2 * time.Second):
		}

		snap, err := b.Snapshot(ctx)
		if err != nil {
			continue
		}

		state := ClassifyPage(snap)
		log.Printf("[%s] Estado de página detectado: %s", job.ID, state)

		switch state {
		case StateJoined:
			return nil
		case StateWaiting:
			job.mu.Lock()
			job.Status = StatusWaiting
			job.Message = "En sala de espera; aguardando admisión."
			job.mu.Unlock()
			continue
		case StateDenied:
			return fmt.Errorf("solicitud rechazada por el anfitrión o enlace inválido")
		case StateEnded:
			return fmt.Errorf("la llamada ya había finalizado")
		case StateOffsite:
			return fmt.Errorf("la reunión redirigió fuera de la llamada (posible inicio de sesión requerido)")
		}

		act := DecideAction(snap, job.Config.BotName)
		switch act.Kind {
		case "type":
			log.Printf("[%s] Escribiendo nombre del bot en campo %d...", job.ID, act.Input)
			_ = b.Type(ctx, act.Input, act.Text)
		case "click":
			log.Printf("[%s] Pulsando botón: %q...", job.ID, act.Text)
			_, _ = b.Click(ctx, act.Text)
		default:
			// Fallback de visión con IA opcional
			if job.Config.VisionHelper != nil {
				if shot, err := b.Screenshot(ctx); err == nil && shot != "" {
					if btn, _ := job.Config.VisionHelper.FindButton(ctx, shot, snap.Text); btn != "" {
						log.Printf("[%s] Botón identificado mediante IA: %q", job.ID, btn)
						_, _ = b.Click(ctx, btn)
					}
				}
			}
		}
	}
	return fmt.Errorf("tiempo de espera agotado intentando unirse a la reunión")
}

func (e *Engine) monitorCall(ctx context.Context, job *Job, b cdp.Browser) {
	ticker := time.NewTicker(4 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			snap, err := b.Snapshot(ctx)
			if err != nil {
				continue
			}

			// Actualizar lista de participantes
			if names, err := b.Participants(ctx); err == nil && len(names) > 0 {
				job.mu.Lock()
				merged := mergeParticipants(job.Participants, names)
				job.Participants = merged
				job.mu.Unlock()
			}

			// Verificar si terminó la reunión o lo sacaron
			if !IsInCall(snap) {
				state := ClassifyPage(snap)
				if state == StateEnded || state == StateOffsite {
					log.Printf("[%s] Llamada terminada según la pantalla.", job.ID)
					return
				}
			}
		}
	}
}

func mergeParticipants(current, newOnes []string) []string {
	seen := make(map[string]bool)
	var res []string
	for _, p := range current {
		p = strings.TrimSpace(p)
		if p != "" && !seen[p] {
			seen[p] = true
			res = append(res, p)
		}
	}
	for _, p := range newOnes {
		p = strings.TrimSpace(p)
		if p != "" && !seen[p] {
			seen[p] = true
			res = append(res, p)
		}
	}
	return res
}
