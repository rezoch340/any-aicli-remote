package grok

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"math"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/rezoch340/any-aicli-remote/backend/internal/voice"
)

const DefaultVoiceEndpoint = "https://api.x.ai/v1/tts"

var VoiceIdentifiers = []string{"eve", "ara", "leo", "rex", "sal", "luna", "orion", "helix"}

type VoiceService struct {
	apiKey   string
	endpoint string
	client   *http.Client
}

func NewVoice(apiKey string) *VoiceService {
	return NewVoiceWithClient(apiKey, DefaultVoiceEndpoint, &http.Client{Timeout: 60 * time.Second})
}

func NewVoiceFromEnvironment() *VoiceService {
	return NewVoice(DiscoverVoiceAPIKey(""))
}

func NewVoiceWithClient(apiKey, endpoint string, client *http.Client) *VoiceService {
	if strings.TrimSpace(endpoint) == "" {
		endpoint = DefaultVoiceEndpoint
	}
	if client == nil {
		client = &http.Client{Timeout: 60 * time.Second}
	}
	return &VoiceService{apiKey: strings.TrimSpace(apiKey), endpoint: endpoint, client: client}
}

func (service *VoiceService) Status() voice.StatusResult {
	hasKey := strings.TrimSpace(service.apiKey) != ""
	status := voice.StatusResult{
		OK:     true,
		TTS:    hasKey,
		STT:    "browser",
		Voices: append([]string(nil), VoiceIdentifiers...),
	}
	if hasKey {
		status.Provider = "xai"
		return status
	}
	status.Provider = "browser-fallback"
	hint := "Configure the selected provider's voice API key for server-side speech synthesis"
	status.Hint = &hint
	return status
}

func (service *VoiceService) Synthesize(operationContext context.Context, input voice.Request) (voice.Audio, error) {
	if strings.TrimSpace(service.apiKey) == "" {
		return voice.Audio{}, voice.APIKeyMissingError
	}
	text := strings.TrimSpace(input.Text)
	if text == "" {
		text = strings.TrimSpace(input.Input)
	}
	if text == "" {
		return voice.Audio{}, voice.TextRequiredError
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
	payload := voicePayload{
		Text:              text,
		VoiceID:           voiceID,
		Language:          language,
		OutputFormat:      voiceOutputFormat{Codec: "mp3", SampleRate: 24_000, BitRate: 128_000},
		TextNormalization: true,
	}
	if input.Speed != nil && !math.IsNaN(*input.Speed) && !math.IsInf(*input.Speed, 0) {
		speed := max(0.7, min(1.5, *input.Speed))
		payload.Speed = &speed
	}
	body, operationError := json.Marshal(payload)
	if operationError != nil {
		return voice.Audio{}, operationError
	}
	request, operationError := http.NewRequestWithContext(operationContext, http.MethodPost, service.endpoint, bytes.NewReader(body))
	if operationError != nil {
		return voice.Audio{}, operationError
	}
	request.Header.Set("Authorization", "Bearer "+service.apiKey)
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Accept", "audio/mpeg")
	response, operationError := service.client.Do(request)
	if operationError != nil {
		return voice.Audio{}, operationError
	}
	defer response.Body.Close()
	data, operationError := io.ReadAll(response.Body)
	if operationError != nil {
		return voice.Audio{}, operationError
	}
	if response.StatusCode != http.StatusOK {
		return voice.Audio{}, &voice.UpstreamError{
			Status: response.StatusCode,
			Body:   truncateVoiceText(strings.ToValidUTF8(string(data), "�"), 400),
		}
	}
	contentType := strings.TrimSpace(response.Header.Get("Content-Type"))
	if contentType == "" {
		contentType = "audio/mpeg"
	}
	return voice.Audio{Data: data, ContentType: contentType, VoiceID: voiceID}, nil
}

// DiscoverVoiceAPIKey reads credentials owned by the Grok provider adapter.
func DiscoverVoiceAPIKey(homeDirectory string) string {
	for _, key := range []string{"XAI_API_KEY", "GROK_API_KEY", "xai_api_key"} {
		if value := strings.TrimSpace(os.Getenv(key)); value != "" {
			return value
		}
	}
	if strings.TrimSpace(homeDirectory) == "" {
		resolvedHome, operationError := os.UserHomeDir()
		if operationError != nil {
			return ""
		}
		homeDirectory = resolvedHome
	}
	data, operationError := os.ReadFile(filepath.Join(homeDirectory, ".grok", "credentials.json"))
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

type voiceOutputFormat struct {
	Codec      string `json:"codec"`
	SampleRate int    `json:"sample_rate"`
	BitRate    int    `json:"bit_rate"`
}

type voicePayload struct {
	Text              string            `json:"text"`
	VoiceID           string            `json:"voice_id"`
	Language          string            `json:"language"`
	OutputFormat      voiceOutputFormat `json:"output_format"`
	TextNormalization bool              `json:"text_normalization"`
	Speed             *float64          `json:"speed,omitempty"`
}

func truncateVoiceText(text string, maximum int) string {
	runes := []rune(text)
	if len(runes) <= maximum {
		return text
	}
	return string(runes[:maximum])
}

var _ voice.Service = (*VoiceService)(nil)
