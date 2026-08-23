package grok

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/rezoch340/any-aicli-remote/backend/internal/voice"
)

func TestVoiceStatusAndMissingKey(testContext *testing.T) {
	service := NewVoice("")
	status := service.Status()
	if !status.OK || status.TTS || status.Provider != "browser-fallback" || status.Hint == nil || len(status.Voices) != len(VoiceIdentifiers) {
		testContext.Fatalf("status = %#v", status)
	}
	if _, operationError := service.Synthesize(context.Background(), voice.Request{Text: "hello"}); !errors.Is(operationError, voice.APIKeyMissingError) {
		testContext.Fatalf("missing key error = %v", operationError)
	}
	withKey := NewVoice("secret").Status()
	if !withKey.TTS || withKey.Provider != "xai" || withKey.Hint != nil {
		testContext.Fatalf("key status = %#v", withKey)
	}
}

func TestVoiceSynthesizeRequestAndAudioResponse(testContext *testing.T) {
	var receivedPayload voicePayload
	server := httptest.NewServer(http.HandlerFunc(func(responseWriter http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodPost {
			testContext.Errorf("method = %s", request.Method)
		}
		if value := request.Header.Get("Authorization"); value != "Bearer secret" {
			testContext.Errorf("authorization = %q", value)
		}
		if value := request.Header.Get("Accept"); value != "audio/mpeg" {
			testContext.Errorf("accept = %q", value)
		}
		if operationError := json.NewDecoder(request.Body).Decode(&receivedPayload); operationError != nil {
			testContext.Errorf("decode payload: %v", operationError)
		}
		responseWriter.Header().Set("Content-Type", "audio/test")
		_, _ = responseWriter.Write([]byte("audio-data"))
	}))
	defer server.Close()

	speed := 9.0
	longText := strings.Repeat("你", 15_001)
	audio, operationError := NewVoiceWithClient("secret", server.URL, server.Client()).Synthesize(context.Background(), voice.Request{
		Input: longText, Voice: "ara", Language: "zh", Speed: &speed,
	})
	if operationError != nil {
		testContext.Fatal(operationError)
	}
	if string(audio.Data) != "audio-data" || audio.ContentType != "audio/test" || audio.VoiceID != "ara" {
		testContext.Fatalf("audio = %#v", audio)
	}
	if receivedPayload.VoiceID != "ara" || receivedPayload.Language != "zh" || !receivedPayload.TextNormalization {
		testContext.Fatalf("payload = %#v", receivedPayload)
	}
	if len([]rune(receivedPayload.Text)) != 14_991 || !strings.HasSuffix(receivedPayload.Text, "…") {
		testContext.Fatalf("text length = %d", len([]rune(receivedPayload.Text)))
	}
	if receivedPayload.Speed == nil || *receivedPayload.Speed != 1.5 {
		testContext.Fatalf("speed = %v", receivedPayload.Speed)
	}
	if receivedPayload.OutputFormat.Codec != "mp3" || receivedPayload.OutputFormat.SampleRate != 24_000 || receivedPayload.OutputFormat.BitRate != 128_000 {
		testContext.Fatalf("format = %#v", receivedPayload.OutputFormat)
	}
}

func TestVoiceSynthesizeDefaultsAndValidation(testContext *testing.T) {
	var receivedPayload voicePayload
	server := httptest.NewServer(http.HandlerFunc(func(responseWriter http.ResponseWriter, request *http.Request) {
		_ = json.NewDecoder(request.Body).Decode(&receivedPayload)
		responseWriter.WriteHeader(http.StatusOK)
	}))
	defer server.Close()
	service := NewVoiceWithClient("secret", server.URL, server.Client())
	if _, operationError := service.Synthesize(context.Background(), voice.Request{}); !errors.Is(operationError, voice.TextRequiredError) {
		testContext.Fatalf("empty text error = %v", operationError)
	}
	if _, operationError := service.Synthesize(context.Background(), voice.Request{Text: " hello "}); operationError != nil {
		testContext.Fatal(operationError)
	}
	if receivedPayload.Text != "hello" || receivedPayload.VoiceID != "eve" || receivedPayload.Language != "en" || receivedPayload.Speed != nil {
		testContext.Fatalf("defaults = %#v", receivedPayload)
	}
}

func TestVoiceSynthesizeUpstreamError(testContext *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(responseWriter http.ResponseWriter, _ *http.Request) {
		responseWriter.WriteHeader(http.StatusTooManyRequests)
		_, _ = responseWriter.Write([]byte(strings.Repeat("rate limited", 100)))
	}))
	defer server.Close()
	_, operationError := NewVoiceWithClient("secret", server.URL, server.Client()).Synthesize(context.Background(), voice.Request{Text: "hello"})
	var upstreamError *voice.UpstreamError
	if !errors.As(operationError, &upstreamError) || upstreamError.Status != http.StatusTooManyRequests || len([]rune(upstreamError.Body)) != 400 {
		testContext.Fatalf("upstream error = %#v / %v", upstreamError, operationError)
	}
}

func TestDiscoverVoiceAPIKey(testContext *testing.T) {
	testContext.Setenv("XAI_API_KEY", "")
	testContext.Setenv("GROK_API_KEY", "")
	testContext.Setenv("xai_api_key", "")
	homeDirectory := testContext.TempDir()
	directory := filepath.Join(homeDirectory, ".grok")
	if operationError := os.MkdirAll(directory, 0o700); operationError != nil {
		testContext.Fatal(operationError)
	}
	if operationError := os.WriteFile(filepath.Join(directory, "credentials.json"), []byte(`{"apiKey":"from-file"}`), 0o600); operationError != nil {
		testContext.Fatal(operationError)
	}
	if value := DiscoverVoiceAPIKey(homeDirectory); value != "from-file" {
		testContext.Fatal(value)
	}
	testContext.Setenv("GROK_API_KEY", "from-env")
	if value := DiscoverVoiceAPIKey(homeDirectory); value != "from-env" {
		testContext.Fatal(value)
	}
	testContext.Setenv("XAI_API_KEY", "preferred")
	if value := DiscoverVoiceAPIKey(homeDirectory); value != "preferred" {
		testContext.Fatal(value)
	}
}
