package voice

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const DefaultEndpoint = "https://api.x.ai/v1/tts"

var (
	APIKeyMissingError = errors.New("XAI_API_KEY not set — browser fallback only")
	TextRequiredError  = errors.New("text required")
	Voices             = []string{"eve", "ara", "leo", "rex", "sal", "luna", "orion", "helix"}
)

type Service struct {
	apiKey   string
	endpoint string
	client   *http.Client
}

type StatusResult struct {
	OK       bool     `json:"ok"`
	TTS      bool     `json:"tts"`
	STT      string   `json:"stt"`
	Provider string   `json:"provider"`
	Voices   []string `json:"voices"`
	Hint     *string  `json:"hint"`
}

// Request accepts both the current and legacy field names used by the Python
// backend. Text/VoiceID take precedence over Input/Voice.
type Request struct {
	Text     string   `json:"text"`
	Input    string   `json:"input"`
	VoiceID  string   `json:"voice_id"`
	Voice    string   `json:"voice"`
	Language string   `json:"language"`
	Speed    *float64 `json:"speed"`
}

type Audio struct {
	Data        []byte
	ContentType string
	VoiceID     string
}

type UpstreamError struct {
	Status int
	Body   string
}

func (upstreamError *UpstreamError) Error() string {
	if strings.TrimSpace(upstreamError.Body) != "" {
		return upstreamError.Body
	}
	return fmt.Sprintf("HTTP %d", upstreamError.Status)
}

func New(apiKey string) *Service {
	return NewWithClient(apiKey, DefaultEndpoint, &http.Client{Timeout: 60 * time.Second})
}

func NewFromEnvironment() *Service {
	return New(DiscoverAPIKey(""))
}

func NewWithClient(apiKey, endpoint string, client *http.Client) *Service {
	if strings.TrimSpace(endpoint) == "" {
		endpoint = DefaultEndpoint
	}
	if client == nil {
		client = &http.Client{Timeout: 60 * time.Second}
	}
	return &Service{apiKey: strings.TrimSpace(apiKey), endpoint: endpoint, client: client}
}

func (service *Service) Status() StatusResult {
	hasKey := strings.TrimSpace(service.apiKey) != ""
	status := StatusResult{
		OK:     true,
		TTS:    hasKey,
		STT:    "browser",
		Voices: append([]string(nil), Voices...),
	}
	if hasKey {
		status.Provider = "xai"
		return status
	}
	status.Provider = "browser-fallback"
	hint := "Set XAI_API_KEY for real Grok voice (else browser speechSynthesis)"
	status.Hint = &hint
	return status
}

func (service *Service) Synthesize(operationContext context.Context, input Request) (Audio, error) {
	if strings.TrimSpace(service.apiKey) == "" {
		return Audio{}, APIKeyMissingError
	}
	text := strings.TrimSpace(input.Text)
	if text == "" {
		text = strings.TrimSpace(input.Input)
	}
	if text == "" {
		return Audio{}, TextRequiredError
	}
	if runes := []rune(text); len(runes) > 15_000 {
		text = string(runes[:14_990]) + "…"
	}
	voiceID := strings.TrimSpace(input.VoiceID)
	if voiceID == "" {
		voiceID = strings.TrimSpace(input.Voice)
	}
	if voiceID == "" {
		voiceID = "eve"
	}
	language := strings.TrimSpace(input.Language)
	if language == "" {
		language = "en"
	}
	payload := ttsPayload{
		Text:              text,
		VoiceID:           voiceID,
		Language:          language,
		OutputFormat:      outputFormat{Codec: "mp3", SampleRate: 24_000, BitRate: 128_000},
		TextNormalization: true,
	}
	if input.Speed != nil && !math.IsNaN(*input.Speed) && !math.IsInf(*input.Speed, 0) {
		speed := max(0.7, min(1.5, *input.Speed))
		payload.Speed = &speed
	}
	body, operationError := json.Marshal(payload)
	if operationError != nil {
		return Audio{}, operationError
	}
	request, operationError := http.NewRequestWithContext(operationContext, http.MethodPost, service.endpoint, bytes.NewReader(body))
	if operationError != nil {
		return Audio{}, operationError
	}
	request.Header.Set("Authorization", "Bearer "+service.apiKey)
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Accept", "audio/mpeg")
	response, operationError := service.client.Do(request)
	if operationError != nil {
		return Audio{}, operationError
	}
	defer response.Body.Close()
	data, operationError := io.ReadAll(response.Body)
	if operationError != nil {
		return Audio{}, operationError
	}
	if response.StatusCode != http.StatusOK {
		return Audio{}, &UpstreamError{
			Status: response.StatusCode,
			Body:   truncate(strings.ToValidUTF8(string(data), "�"), 400),
		}
	}
	contentType := strings.TrimSpace(response.Header.Get("Content-Type"))
	if contentType == "" {
		contentType = "audio/mpeg"
	}
	return Audio{Data: data, ContentType: contentType, VoiceID: voiceID}, nil
}

// DiscoverAPIKey uses the same environment and ~/.grok/credentials.json keys
// as the Python backend. Passing an empty home resolves the current user's home.
func DiscoverAPIKey(home string) string {
	for _, key := range []string{"XAI_API_KEY", "GROK_API_KEY", "xai_api_key"} {
		if value := strings.TrimSpace(os.Getenv(key)); value != "" {
			return value
		}
	}
	if strings.TrimSpace(home) == "" {
		resolved, operationError := os.UserHomeDir()
		if operationError != nil {
			return ""
		}
		home = resolved
	}
	data, operationError := os.ReadFile(filepath.Join(home, ".grok", "credentials.json"))
	if operationError != nil {
		return ""
	}
	var credentials map[string]any
	if json.Unmarshal(data, &credentials) != nil {
		return ""
	}
	for _, key := range []string{"XAI_API_KEY", "apiKey", "api_key", "xaiApiKey", "token"} {
		if value, valid := credentials[key].(string); valid {
			if value = strings.TrimSpace(value); value != "" {
				return value
			}
		}
	}
	return ""
}

type outputFormat struct {
	Codec      string `json:"codec"`
	SampleRate int    `json:"sample_rate"`
	BitRate    int    `json:"bit_rate"`
}

type ttsPayload struct {
	Text              string       `json:"text"`
	VoiceID           string       `json:"voice_id"`
	Language          string       `json:"language"`
	OutputFormat      outputFormat `json:"output_format"`
	TextNormalization bool         `json:"text_normalization"`
	Speed             *float64     `json:"speed,omitempty"`
}

func truncate(text string, max int) string {
	runes := []rune(text)
	if len(runes) <= max {
		return text
	}
	return string(runes[:max])
}
