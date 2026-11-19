package webhook

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"time"
)

type EventType string

const (
	EventStarted EventType = "bot.started"
	EventJoined  EventType = "bot.joined"
	EventEnded   EventType = "bot.ended"
	EventFailed  EventType = "bot.failed"
)

type EventPayload struct {
	Event        EventType `json:"event"`
	BotID        string    `json:"bot_id"`
	MeetingURL   string    `json:"meeting_url"`
	Status       string    `json:"status"`
	Message      string    `json:"message,omitempty"`
	DurationSec  int       `json:"duration_seconds,omitempty"`
	Participants []string  `json:"participants,omitempty"`
	Timestamp    time.Time `json:"timestamp"`
}

type Dispatcher struct {
	client *http.Client
}

func NewDispatcher() *Dispatcher {
	return &Dispatcher{
		client: &http.Client{Timeout: 60 * time.Second},
	}
}

// SendEvent envía una notificación liviana de estado (JSON).
func (d *Dispatcher) SendEvent(ctx context.Context, webhookURL string, payload EventPayload) error {
	if webhookURL == "" {
		return nil
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, webhookURL, bytes.NewReader(raw))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "meetbot/1.0")

	resp, err := d.client.Do(req)
	if err != nil {
		return fmt.Errorf("error enviando webhook: %w", err)
	}
	defer resp.Body.Close()
	return nil
}

// DeliverArtifacts envía el resultado completo de la reunión (metadata + archivos de audio y video multipart).
func (d *Dispatcher) DeliverArtifacts(ctx context.Context, webhookURL string, payload EventPayload, audioData []byte, audioName string, videoData []byte, videoName string) error {
	if webhookURL == "" {
		return nil
	}

	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)

	metaJSON, _ := json.Marshal(payload)
	_ = writer.WriteField("metadata", string(metaJSON))
	_ = writer.WriteField("bot_id", payload.BotID)
	_ = writer.WriteField("meeting_url", payload.MeetingURL)

	if len(audioData) > 0 {
		part, err := writer.CreateFormFile("audio", audioName)
		if err == nil {
			_, _ = io.Copy(part, bytes.NewReader(audioData))
		}
	}

	if len(videoData) > 0 {
		part, err := writer.CreateFormFile("video", videoName)
		if err == nil {
			_, _ = io.Copy(part, bytes.NewReader(videoData))
		}
	}

	if err := writer.Close(); err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, webhookURL, body)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())
	req.Header.Set("User-Agent", "meetbot/1.0")

	resp, err := d.client.Do(req)
	if err != nil {
		return fmt.Errorf("error entregando artefactos por webhook: %w", err)
	}
	defer resp.Body.Close()
	return nil
}
