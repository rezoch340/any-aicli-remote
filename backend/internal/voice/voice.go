// Package voice defines provider-neutral speech synthesis contracts.
package voice

import (
	"context"
	"errors"
	"fmt"
	"strings"
)

var (
	APIKeyMissingError    = errors.New("provider voice API key not configured — browser fallback only")
	TextRequiredError     = errors.New("text required")
	ResponseTooLargeError = errors.New("voice response body too large")
)

// Service is implemented by a provider adapter when it offers speech synthesis.
type Service interface {
	Status() StatusResult
	Synthesize(context.Context, Request) (Audio, error)
}

type StatusResult struct {
	OK       bool     `json:"ok"`
	TTS      bool     `json:"tts"`
	STT      string   `json:"stt"`
	Provider string   `json:"provider"`
	Voices   []string `json:"voices"`
	Hint     *string  `json:"hint"`
}

// Request accepts both the current and legacy field names used by older clients.
// Text and VoiceID take precedence over Input and Voice.
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
