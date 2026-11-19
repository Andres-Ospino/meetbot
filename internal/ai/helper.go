package ai

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

// VisionHelper permite consultar a un modelo de visión si las heurísticas DOM no encuentran el botón.
type VisionHelper interface {
	FindButton(ctx context.Context, screenshotB64 string, pageText string) (string, error)
}

// NoopHelper es el proveedor por defecto (sin IA externa, cero costos).
type NoopHelper struct{}

func (n *NoopHelper) FindButton(ctx context.Context, screenshotB64 string, pageText string) (string, error) {
	return "", nil
}

// OpenAICompatibleHelper permite consultar cualquier API con formato OpenAI (OpenAI, Ollama, vLLM, etc.)
type OpenAICompatibleHelper struct {
	BaseURL string
	APIKey  string
	Model   string
	client  *http.Client
}

func NewOpenAIHelperFromEnv() VisionHelper {
	apiKey := strings.TrimSpace(os.Getenv("MEETBOT_AI_API_KEY"))
	baseURL := strings.TrimSpace(os.Getenv("MEETBOT_AI_BASE_URL"))
	model := strings.TrimSpace(os.Getenv("MEETBOT_AI_MODEL"))

	if apiKey == "" && baseURL == "" {
		return &NoopHelper{}
	}
	if baseURL == "" {
		baseURL = "https://api.openai.com/v1"
	}
	if model == "" {
		model = "gpt-4o-mini"
	}

	return &OpenAICompatibleHelper{
		BaseURL: strings.TrimRight(baseURL, "/"),
		APIKey:  apiKey,
		Model:   model,
		client:  &http.Client{Timeout: 15 * time.Second},
	}
}

func (h *OpenAICompatibleHelper) FindButton(ctx context.Context, screenshotB64 string, pageText string) (string, error) {
	prompt := fmt.Sprintf(`Eres un asistente que controla un bot intentando unirse a una videollamada (Meet/Zoom/Teams).
Texto visible en la página:
"""%s"""

Analiza la pantalla e indica ÚNICAMENTE el texto exacto del botón que se debe pulsar para entrar a la llamada o saltar avisos previos (por ejemplo "Unirse ahora", "Solicitar unirse", "Continuar sin micrófono").
Responde solo con el texto del botón, sin comillas ni explicaciones adicionales. Si no hay botón claro, responde "NONE".`, pageText)

	payload := map[string]any{
		"model": h.Model,
		"messages": []any{
			map[string]any{
				"role": "user",
				"content": []any{
					map[string]any{"type": "text", "text": prompt},
					map[string]any{
						"type": "image_url",
						"image_url": map[string]any{
							"url": "data:image/jpeg;base64," + screenshotB64,
						},
					},
				},
			},
		},
		"max_tokens": 50,
	}

	raw, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, h.BaseURL+"/chat/completions", bytes.NewReader(raw))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	if h.APIKey != "" {
		req.Header.Set("Authorization", "Bearer "+h.APIKey)
	}

	resp, err := h.client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 300 {
		return "", fmt.Errorf("error del proveedor de IA (%d): %s", resp.StatusCode, string(body))
	}

	var res struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(body, &res); err != nil || len(res.Choices) == 0 {
		return "", fmt.Errorf("respuesta inválida de IA: %w", err)
	}

	btn := strings.TrimSpace(res.Choices[0].Message.Content)
	if btn == "NONE" || btn == "" {
		return "", nil
	}
	return btn, nil
}
