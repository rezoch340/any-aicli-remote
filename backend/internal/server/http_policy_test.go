package server

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/rezoch340/any-aicli-remote/backend/internal/config"
	"github.com/rezoch340/any-aicli-remote/backend/internal/voice"
)

func TestNewHTTPServerMapsCanonicalPolicy(testingContext *testing.T) {
	policy := config.DefaultDocument(testingContext.TempDir()).Tuning.HTTP
	policy.ReadHeaderTimeout.Duration = 17 * time.Millisecond
	policy.IdleTimeout.Duration = 23 * time.Millisecond
	policy.MaxHeaderBytes = 12345
	httpServer := newHTTPServer(http.NotFoundHandler(), policy)
	if httpServer.ReadHeaderTimeout != 17*time.Millisecond || httpServer.IdleTimeout != 23*time.Millisecond || httpServer.MaxHeaderBytes != 12345 {
		testingContext.Fatalf("HTTP server policy = %#v", httpServer)
	}
}

func TestHandlerUsesConfiguredRequestBodyLimit(testingContext *testing.T) {
	fixture := newRouteTestServer(testingContext)
	fixture.server.configuration.Canonical.Tuning.HTTP.MaxRequestBodyBytes = 20
	fixture.handler = fixture.server.Handler()

	oversize := fixture.request(testingContext, http.MethodPost, "/api/session/rename", map[string]any{"sessionId": routeSessionID, "title": "too long"}, remotePeer, true)
	if oversize.Code != http.StatusRequestEntityTooLarge {
		testingContext.Fatalf("oversize status = %d, body=%q", oversize.Code, oversize.Body.String())
	}

	fixture.server.configuration.Canonical.Tuning.HTTP.MaxRequestBodyBytes = 256
	fixture.handler = fixture.server.Handler()
	request := httptest.NewRequest(http.MethodPost, "/api/session/rename?key="+routeTestSecret, bytes.NewBufferString(`{"sessionId":"route-session","title":"ok"}`))
	request.RemoteAddr = remotePeer
	response := httptest.NewRecorder()
	fixture.handler.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		testingContext.Fatalf("small request status = %d, body=%q", response.Code, response.Body.String())
	}
}

func TestLooseJSONRejectsTrailingOversizeWithoutSideEffect(testingContext *testing.T) {
	fixture := newRouteTestServer(testingContext)
	fixture.server.configuration.Canonical.Tuning.HTTP.MaxRequestBodyBytes = 32
	fixture.handler = fixture.server.Handler()
	before, errorValue := fixture.server.room.FeedString("0", "")
	if errorValue != nil {
		testingContext.Fatal(errorValue)
	}
	payload := `{"who":"tester","text":"ok"}` + strings.Repeat(" ", 64)
	request := httptest.NewRequest(http.MethodPost, "/api/room/say?key="+routeTestSecret, strings.NewReader(payload))
	request.RemoteAddr = remotePeer
	response := httptest.NewRecorder()
	fixture.handler.ServeHTTP(response, request)
	if response.Code != http.StatusRequestEntityTooLarge {
		testingContext.Fatalf("status = %d, body=%q", response.Code, response.Body.String())
	}
	after, errorValue := fixture.server.room.FeedString("0", "")
	if errorValue != nil {
		testingContext.Fatal(errorValue)
	}
	if len(after) != len(before) {
		testingContext.Fatalf("oversize loose JSON changed room feed: before=%d after=%d", len(before), len(after))
	}
}

func TestCanonicalHTTPTimeoutContexts(testingContext *testing.T) {
	fixture := newRouteTestServer(testingContext)
	fixture.server.configuration.Canonical.Tuning.HTTP.DeepHealthTimeout.Duration = 37 * time.Millisecond
	fixture.server.configuration.Canonical.Tuning.HTTP.ProviderRequestTimeout.Duration = 43 * time.Millisecond
	request := httptest.NewRequest(http.MethodGet, "/", nil)

	assertContextTimeout(testingContext, fixture.server.deepHealthContext, request, 37*time.Millisecond)
	assertContextTimeout(testingContext, fixture.server.providerRequestContext, request, 43*time.Millisecond)
}

func TestHealthyRemoteUsesConfiguredProbeTimeout(testingContext *testing.T) {
	listener := httptest.NewServer(http.HandlerFunc(func(responseWriter http.ResponseWriter, _ *http.Request) {
		time.Sleep(30 * time.Millisecond)
		_, _ = responseWriter.Write([]byte(`{"ok":true}`))
	}))
	defer listener.Close()
	_, portText, errorValue := net.SplitHostPort(strings.TrimPrefix(listener.URL, "http://"))
	if errorValue != nil {
		testingContext.Fatal(errorValue)
	}
	portNumber, errorValue := strconv.Atoi(portText)
	if errorValue != nil {
		testingContext.Fatal(errorValue)
	}
	if healthyRemote(portNumber, 2*time.Millisecond, 512) {
		testingContext.Fatal("probe ignored configured timeout")
	}
}

func TestAuthenticationCookieUsesConfiguredMaxAge(testingContext *testing.T) {
	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/private?key="+testSecret, nil)
	authMiddleware(testSecret, 91, okHandler()).ServeHTTP(response, request)
	cookies := response.Result().Cookies()
	if len(cookies) != 1 || cookies[0].MaxAge != 91 {
		testingContext.Fatalf("cookie = %#v", cookies)
	}
}

func assertContextTimeout(testingContext *testing.T, factory func(*http.Request) (context.Context, context.CancelFunc), request *http.Request, expected time.Duration) {
	testingContext.Helper()
	before := time.Now()
	executionContext, cancel := factory(request)
	defer cancel()
	deadline, present := executionContext.Deadline()
	if !present {
		testingContext.Fatal("missing deadline")
	}
	remaining := deadline.Sub(before)
	if remaining < expected-5*time.Millisecond || remaining > expected+5*time.Millisecond {
		testingContext.Fatalf("timeout = %s, want %s", remaining, expected)
	}
}

func TestHealthyRemoteUsesConfiguredResponseLimit(testingContext *testing.T) {
	listener := httptest.NewServer(http.HandlerFunc(func(responseWriter http.ResponseWriter, _ *http.Request) {
		_, _ = responseWriter.Write([]byte(strings.Repeat("x", 16) + `{"ok":true}`))
	}))
	defer listener.Close()
	_, portText, operationError := net.SplitHostPort(strings.TrimPrefix(listener.URL, "http://"))
	if operationError != nil {
		testingContext.Fatal(operationError)
	}
	portNumber, operationError := strconv.Atoi(portText)
	if operationError != nil {
		testingContext.Fatal(operationError)
	}
	if healthyRemote(portNumber, time.Second, 16) {
		testingContext.Fatal("small configured limit accepted late health marker")
	}
	if !healthyRemote(portNumber, time.Second, 32) {
		testingContext.Fatal("larger configured limit did not read health marker")
	}
}

type failingVoiceService struct{ failure error }

func (service failingVoiceService) Status() voice.StatusResult { return voice.StatusResult{} }
func (service failingVoiceService) Synthesize(context.Context, voice.Request) (voice.Audio, error) {
	return voice.Audio{}, service.failure
}

func TestTTSGenericErrorUsesConfiguredRuneLimit(testingContext *testing.T) {
	fixture := newRouteTestServer(testingContext)
	fixture.server.configuration.Canonical.Tuning.HTTP.ErrorResponseMaxRunes = 4
	fixture.server.voice = failingVoiceService{failure: errors.New(strings.Repeat("界", 8))}
	fixture.handler = fixture.server.Handler()
	response := fixture.request(testingContext, http.MethodPost, "/api/tts", map[string]any{"text": "hello"}, remotePeer, true)
	if response.Code != http.StatusBadGateway {
		testingContext.Fatalf("status=%d body=%q", response.Code, response.Body.String())
	}
	var body map[string]any
	if operationError := json.Unmarshal(response.Body.Bytes(), &body); operationError != nil {
		testingContext.Fatal(operationError)
	}
	if errorText, valid := body["error"].(string); !valid || len([]rune(errorText)) != 4 {
		testingContext.Fatalf("configured error truncation not applied: %#v", body)
	}
}

func TestDeepHealthDetailUsesConfiguredRuneLimit(testingContext *testing.T) {
	fixture := newRouteTestServer(testingContext)
	fixture.server.configuration.Canonical.Tuning.HTTP.DeepHealthDetailMaxRunes = 4
	fixture.server.configuration.Canonical.Tuning.HTTP.DeepHealthTimeout.Duration = 50 * time.Millisecond
	request := httptest.NewRequest(http.MethodGet, "/health/deep", nil)
	response := httptest.NewRecorder()
	fixture.server.handleHealthDeep(response, request)
	if response.Code != http.StatusOK {
		testingContext.Fatalf("status=%d body=%q", response.Code, response.Body.String())
	}
	var body map[string]any
	if operationError := json.Unmarshal(response.Body.Bytes(), &body); operationError != nil {
		testingContext.Fatal(operationError)
	}
	detail, valid := body["detail"].(string)
	if !valid || detail == "" || len([]rune(detail)) != 4 {
		testingContext.Fatalf("configured deep detail truncation not applied: %#v", body)
	}
}
