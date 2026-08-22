package voice

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
)

func TestStatusAndMissingKey(testContext *testing.T) {
	service := New("")
	status := service.Status()
	if !status.OK || status.TTS || status.Provider != "browser-fallback" || status.Hint == nil || len(status.Voices) != len(Voices) {
		testContext.Fatalf("status = %#v", status)
	}
	if _, operationError := service.Synthesize(context.Background(), Request{Text: "hello"}); !errors.Is(operationError, APIKeyMissingError) {
		testContext.Fatalf("missing key error = %v", operationError)
	}
	withKey := New("secret").Status()
	if !withKey.TTS || withKey.Provider != "xai" || withKey.Hint != nil {
		testContext.Fatalf("key status = %#v", withKey)
	}
}

func TestSynthesizeRequestAndAudioResponse(testContext *testing.T) {
	var gotPayload ttsPayload
	server := httptest.NewServer(http.HandlerFunc(func(responseWriter http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodPost {
			testContext.Errorf("method = %s", request.Method)
		}
		if got := request.Header.Get("Authorization"); got != "Bearer secret" {
			testContext.Errorf("authorization = %q", got)
		}
		if got := request.Header.Get("Accept"); got != "audio/mpeg" {
			testContext.Errorf("accept = %q", got)
		}
		if operationError := json.NewDecoder(request.Body).Decode(&gotPayload); operationError != nil {
			testContext.Errorf("decode payload: %v", operationError)
		}
		responseWriter.Header().Set("Content-Type", "audio/test")
		_, _ = responseWriter.Write([]byte("audio-data"))
	}))
	defer server.Close()

	speed := 9.0
	longText := strings.Repeat("你", 15_001)
	audio, operationError := NewWithClient("secret", server.URL, server.Client()).Synthesize(context.Background(), Request{
		Input: longText, Voice: "ara", Language: "zh", Speed: &speed,
	})
	if operationError != nil {
		testContext.Fatal(operationError)
	}
	if string(audio.Data) != "audio-data" || audio.ContentType != "audio/test" || audio.VoiceID != "ara" {
		testContext.Fatalf("audio = %#v", audio)
	}
	if gotPayload.VoiceID != "ara" || gotPayload.Language != "zh" || !gotPayload.TextNormalization {
		testContext.Fatalf("payload = %#v", gotPayload)
	}
	if len([]rune(gotPayload.Text)) != 14_991 || !strings.HasSuffix(gotPayload.Text, "…") {
		testContext.Fatalf("text length = %d", len([]rune(gotPayload.Text)))
	}
	if gotPayload.Speed == nil || *gotPayload.Speed != 1.5 {
		testContext.Fatalf("speed = %v", gotPayload.Speed)
	}
	if gotPayload.OutputFormat.Codec != "mp3" || gotPayload.OutputFormat.SampleRate != 24_000 || gotPayload.OutputFormat.BitRate != 128_000 {
		testContext.Fatalf("format = %#v", gotPayload.OutputFormat)
	}
}

func TestSynthesizeDefaultsAndValidation(testContext *testing.T) {
	var got ttsPayload
	server := httptest.NewServer(http.HandlerFunc(func(responseWriter http.ResponseWriter, request *http.Request) {
		_ = json.NewDecoder(request.Body).Decode(&got)
		responseWriter.WriteHeader(http.StatusOK)
	}))
	defer server.Close()
	service := NewWithClient("secret", server.URL, server.Client())
	if _, operationError := service.Synthesize(context.Background(), Request{}); !errors.Is(operationError, TextRequiredError) {
		testContext.Fatalf("empty text error = %v", operationError)
	}
	if _, operationError := service.Synthesize(context.Background(), Request{Text: " hello "}); operationError != nil {
		testContext.Fatal(operationError)
	}
	if got.Text != "hello" || got.VoiceID != "eve" || got.Language != "en" || got.Speed != nil {
		testContext.Fatalf("defaults = %#v", got)
	}
}

func TestSynthesizeUpstreamError(testContext *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(responseWriter http.ResponseWriter, _ *http.Request) {
		responseWriter.WriteHeader(http.StatusTooManyRequests)
		_, _ = responseWriter.Write([]byte(strings.Repeat("rate limited", 100)))
	}))
	defer server.Close()
	_, operationError := NewWithClient("secret", server.URL, server.Client()).Synthesize(context.Background(), Request{Text: "hello"})
	var upstream *UpstreamError
	if !errors.As(operationError, &upstream) || upstream.Status != http.StatusTooManyRequests || len([]rune(upstream.Body)) != 400 {
		testContext.Fatalf("upstream error = %#v / %v", upstream, operationError)
	}
}

func TestDiscoverAPIKey(testContext *testing.T) {
	testContext.Setenv("XAI_API_KEY", "")
	testContext.Setenv("GROK_API_KEY", "")
	testContext.Setenv("xai_api_key", "")
	home := testContext.TempDir()
	directory := filepath.Join(home, ".grok")
	if operationError := os.MkdirAll(directory, 0o700); operationError != nil {
		testContext.Fatal(operationError)
	}
	if operationError := os.WriteFile(filepath.Join(directory, "credentials.json"), []byte(`{"apiKey":"from-file"}`), 0o600); operationError != nil {
		testContext.Fatal(operationError)
	}
	if got := DiscoverAPIKey(home); got != "from-file" {
		testContext.Fatal(got)
	}
	testContext.Setenv("GROK_API_KEY", "from-env")
	if got := DiscoverAPIKey(home); got != "from-env" {
		testContext.Fatal(got)
	}
	testContext.Setenv("XAI_API_KEY", "preferred")
	if got := DiscoverAPIKey(home); got != "preferred" {
		testContext.Fatal(got)
	}
}
