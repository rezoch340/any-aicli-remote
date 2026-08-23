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

	"github.com/rezoch340/any-aicli-remote/backend/internal/voice"
)

const DefaultVoiceEndpoint = "https://api.x.ai/v1/tts"
const (
	defaultVoiceID    = "eve"
	defaultLanguage   = "en"
	defaultCodec      = "mp3"
	defaultSampleRate = 24_000
	defaultBitRate    = 128_000
	minimumSpeed      = 0.7
	maximumSpeed      = 1.5
	audioMPEG         = "audio/mpeg"
)

var voiceIdentifiers = []string{"eve", "ara", "leo", "rex", "sal", "luna", "orion", "helix"}

type VoiceService struct {
	apiKey   string
	endpoint string
	client   *http.Client
	policy   voice.Policy
}

func NewVoice(apiKey string, policy voice.Policy) (*VoiceService, error) {
	return NewVoiceWithClient(apiKey, DefaultVoiceEndpoint, nil, policy)
}
func NewVoiceFromEnvironment(policy voice.Policy) (*VoiceService, error) {
	return NewVoice(DiscoverVoiceAPIKey(""), policy)
}
func NewVoiceWithClient(apiKey, endpoint string, client *http.Client, policy voice.Policy) (*VoiceService, error) {
	if errorValue := policy.Validate(); errorValue != nil {
		return nil, errorValue
	}
	if strings.TrimSpace(endpoint) == "" {
		endpoint = DefaultVoiceEndpoint
	}
	if client == nil {
		client = &http.Client{}
	} else {
		cloned := *client
		client = &cloned
	}
	client.Timeout = policy.RequestTimeout
	return &VoiceService{apiKey: strings.TrimSpace(apiKey), endpoint: endpoint, client: client, policy: policy}, nil
}
func (service *VoiceService) Status() voice.StatusResult {
	hasKey := strings.TrimSpace(service.apiKey) != ""
	status := voice.StatusResult{OK: true, TTS: hasKey, STT: "browser", Voices: append([]string(nil), voiceIdentifiers...)}
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
	runes := []rune(text)
	if len(runes) > service.policy.TextMaxRunes {
		text = string(runes[:service.policy.TruncatedTextRunes]) + "…"
	}
	voiceID := strings.TrimSpace(input.VoiceID)
	if voiceID == "" {
		voiceID = strings.TrimSpace(input.Voice)
	}
	if voiceID == "" {
		voiceID = defaultVoiceID
	}
	language := strings.TrimSpace(input.Language)
	if language == "" {
		language = defaultLanguage
	}
	payload := voicePayload{Text: text, VoiceID: voiceID, Language: language, OutputFormat: voiceOutputFormat{Codec: defaultCodec, SampleRate: defaultSampleRate, BitRate: defaultBitRate}, TextNormalization: true}
	if input.Speed != nil && !math.IsNaN(*input.Speed) && !math.IsInf(*input.Speed, 0) {
		speed := max(minimumSpeed, min(maximumSpeed, *input.Speed))
		payload.Speed = &speed
	}
	body, errorValue := json.Marshal(payload)
	if errorValue != nil {
		return voice.Audio{}, errorValue
	}
	request, errorValue := http.NewRequestWithContext(operationContext, http.MethodPost, service.endpoint, bytes.NewReader(body))
	if errorValue != nil {
		return voice.Audio{}, errorValue
	}
	request.Header.Set("Authorization", "Bearer "+service.apiKey)
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Accept", audioMPEG)
	response, errorValue := service.client.Do(request)
	if errorValue != nil {
		return voice.Audio{}, errorValue
	}
	defer response.Body.Close()
	limit := service.policy.SuccessBodyMaxBytes
	if response.StatusCode != http.StatusOK {
		limit = service.policy.ErrorBodyMaxBytes
	}
	data, errorValue := io.ReadAll(io.LimitReader(response.Body, limit+1))
	if errorValue != nil {
		return voice.Audio{}, errorValue
	}
	if int64(len(data)) > limit {
		if response.StatusCode == http.StatusOK {
			return voice.Audio{}, voice.ResponseTooLargeError
		}
		data = data[:limit]
	}
	if response.StatusCode != http.StatusOK {
		return voice.Audio{}, &voice.UpstreamError{Status: response.StatusCode, Body: truncateVoiceText(strings.ToValidUTF8(string(data), "�"), service.policy.ErrorBodyMaxRunes)}
	}
	contentType := strings.TrimSpace(response.Header.Get("Content-Type"))
	if contentType == "" {
		contentType = audioMPEG
	}
	return voice.Audio{Data: data, ContentType: contentType, VoiceID: voiceID}, nil
}
func DiscoverVoiceAPIKey(homeDirectory string) string {
	for _, key := range []string{"XAI_API_KEY", "GROK_API_KEY", "xai_api_key"} {
		if value := strings.TrimSpace(os.Getenv(key)); value != "" {
			return value
		}
	}
	if strings.TrimSpace(homeDirectory) == "" {
		resolvedHome, errorValue := os.UserHomeDir()
		if errorValue != nil {
			return ""
		}
		homeDirectory = resolvedHome
	}
	data, errorValue := os.ReadFile(filepath.Join(homeDirectory, ".grok", "credentials.json"))
	if errorValue != nil {
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
