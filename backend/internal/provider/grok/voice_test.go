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
	"sync"
	"testing"
	"time"

	"github.com/rezoch340/any-aicli-remote/backend/internal/voice"
)

func TestVoiceStatusAndMissingKey(testContext *testing.T) {
	service := mustVoice(testContext, "")
	status := service.Status()
	if !status.OK || status.TTS || status.Provider != "browser-fallback" || status.Hint == nil || len(status.Voices) != len(voiceIdentifiers) {
		testContext.Fatalf("status = %#v", status)
	}
	if _, operationError := service.Synthesize(context.Background(), voice.Request{Text: "hello"}); !errors.Is(operationError, voice.APIKeyMissingError) {
		testContext.Fatalf("missing key error = %v", operationError)
	}
	withKey := mustVoice(testContext, "secret").Status()
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
	audio, operationError := mustVoiceClient(testContext, "secret", server.URL, server.Client()).Synthesize(context.Background(), voice.Request{
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
	service := mustVoiceClient(testContext, "secret", server.URL, server.Client())
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
	_, operationError := mustVoiceClient(testContext, "secret", server.URL, server.Client()).Synthesize(context.Background(), voice.Request{Text: "hello"})
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

func voiceTestPolicy() voice.Policy {
	return voice.Policy{RequestTimeout: time.Second, TextMaxRunes: 15000, TruncatedTextRunes: 14990, SuccessBodyMaxBytes: 64 * 1024 * 1024, ErrorBodyMaxBytes: 16 * 1024, ErrorBodyMaxRunes: 400}
}
func mustVoice(testContext *testing.T, key string) *VoiceService {
	testContext.Helper()
	service, errorValue := NewVoice(key, voiceTestPolicy())
	if errorValue != nil {
		testContext.Fatal(errorValue)
	}
	return service
}
func mustVoiceClient(testContext *testing.T, key, endpoint string, client *http.Client) *VoiceService {
	testContext.Helper()
	service, errorValue := NewVoiceWithClient(key, endpoint, client, voiceTestPolicy())
	if errorValue != nil {
		testContext.Fatal(errorValue)
	}
	return service
}

func TestVoicePolicyLimitsAndClientTimeout(testContext *testing.T) {
	var received voicePayload
	var receivedMutex sync.Mutex
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		receivedMutex.Lock()
		_ = json.NewDecoder(request.Body).Decode(&received)
		receivedMutex.Unlock()
		time.Sleep(50 * time.Millisecond)
		_, _ = writer.Write([]byte("abcdef"))
	}))
	defer server.Close()
	policy := voiceTestPolicy()
	policy.RequestTimeout = 10 * time.Millisecond
	client := &http.Client{Timeout: time.Hour}
	service := mustVoiceClientWithPolicy(testContext, "secret", server.URL, client, policy)
	if service.client.Timeout != policy.RequestTimeout {
		testContext.Fatal(service.client.Timeout)
	}
	if _, errorValue := service.Synthesize(context.Background(), voice.Request{Text: "hi"}); errorValue == nil {
		testContext.Fatal("slow response accepted")
	}
	time.Sleep(60 * time.Millisecond)
	policy.RequestTimeout = time.Second
	policy.TextMaxRunes = 4
	policy.TruncatedTextRunes = 3
	policy.SuccessBodyMaxBytes = 3
	service = mustVoiceClientWithPolicy(testContext, "secret", server.URL, server.Client(), policy)
	if _, errorValue := service.Synthesize(context.Background(), voice.Request{Text: "甲乙丙丁"}); !errors.Is(errorValue, voice.ResponseTooLargeError) {
		testContext.Fatalf("max text error=%v", errorValue)
	}
	receivedMutex.Lock()
	boundaryText := received.Text
	receivedMutex.Unlock()
	if boundaryText != "甲乙丙丁" {
		testContext.Fatalf("boundary text=%q", boundaryText)
	}
	policy.SuccessBodyMaxBytes = 10
	service = mustVoiceClientWithPolicy(testContext, "secret", server.URL, server.Client(), policy)
	if _, errorValue := service.Synthesize(context.Background(), voice.Request{Text: "甲乙丙丁戊"}); errorValue != nil {
		testContext.Fatal(errorValue)
	}
	receivedMutex.Lock()
	truncatedText := received.Text
	receivedMutex.Unlock()
	if truncatedText != "甲乙丙…" {
		testContext.Fatalf("truncated=%q", truncatedText)
	}
}
func TestVoiceErrorPolicyLimitsAndStatusCopy(testContext *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.WriteHeader(http.StatusBadRequest)
		_, _ = writer.Write(append([]byte("甲乙丙丁"), 0xff))
	}))
	defer server.Close()
	policy := voiceTestPolicy()
	policy.ErrorBodyMaxBytes = 10
	policy.ErrorBodyMaxRunes = 2
	service := mustVoiceClientWithPolicy(testContext, "secret", server.URL, server.Client(), policy)
	_, errorValue := service.Synthesize(context.Background(), voice.Request{Text: "ok"})
	var upstream *voice.UpstreamError
	if !errors.As(errorValue, &upstream) || len([]rune(upstream.Body)) > 2 {
		testContext.Fatalf("error=%#v", errorValue)
	}
	first := service.Status()
	first.Voices[0] = "changed"
	if service.Status().Voices[0] == "changed" {
		testContext.Fatal("voice list mutable")
	}
}
func mustVoiceClientWithPolicy(testContext *testing.T, key, endpoint string, client *http.Client, policy voice.Policy) *VoiceService {
	testContext.Helper()
	service, errorValue := NewVoiceWithClient(key, endpoint, client, policy)
	if errorValue != nil {
		testContext.Fatal(errorValue)
	}
	return service
}
