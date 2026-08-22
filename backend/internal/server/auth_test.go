package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

const testSecret = "0123456789abcdef"

func TestAuthMiddlewareBypasses(testingContext *testing.T) {
	cases := []struct {
		name   string
		secret string
		target string
	}{
		{name: "empty secret", target: "/private"},
		{name: "demo", secret: testSecret, target: "/private?demo=1"},
		{name: "health", secret: testSecret, target: "/health"},
		{name: "deep health", secret: testSecret, target: "/health/deep"},
	}
	for _, testCase := range cases {
		testingContext.Run(testCase.name, func(testingContext *testing.T) {
			response := serveAuth(testCase.secret, testCase.target, "192.0.2.10:1234", nil, http.HandlerFunc(func(responseWriter http.ResponseWriter, _ *http.Request) {
				responseWriter.WriteHeader(http.StatusNoContent)
			}))
			if response.Code != http.StatusNoContent {
				testingContext.Fatalf("status = %d", response.Code)
			}
			if response.Header().Get("Set-Cookie") != "" {
				testingContext.Fatalf("bypass set cookie: %s", response.Header().Get("Set-Cookie"))
			}
		})
	}
}

func TestAuthMiddlewareCredentialSourcesAndPrecedence(testingContext *testing.T) {
	cases := []struct {
		name    string
		target  string
		headers http.Header
	}{
		{name: "query", target: "/private?key=" + testSecret},
		{name: "cookie", target: "/private", headers: http.Header{"Cookie": {authCookieName + "=" + testSecret}}},
		{name: "header", target: "/private", headers: http.Header{"X-Grok-Remote-Key": {testSecret}}},
	}
	for _, testCase := range cases {
		testingContext.Run(testCase.name, func(testingContext *testing.T) {
			response := serveAuth(testSecret, testCase.target, "192.0.2.10:1234", testCase.headers, okHandler())
			if response.Code != http.StatusOK || response.Body.String() != "ok" {
				testingContext.Fatalf("response = %d %q", response.Code, response.Body.String())
			}
			assertAuthCookie(testingContext, response)
		})
	}

	headers := http.Header{
		"Cookie":            {authCookieName + "=" + testSecret},
		"X-Grok-Remote-Key": {testSecret},
	}
	response := serveAuth(testSecret, "/private?key=wrong", "192.0.2.10:1234", headers, okHandler())
	if response.Code != http.StatusUnauthorized {
		testingContext.Fatalf("wrong query key did not take precedence: %d", response.Code)
	}

	headers = http.Header{
		"Cookie":            {authCookieName + "=wrong"},
		"X-Grok-Remote-Key": {testSecret},
	}
	response = serveAuth(testSecret, "/private", "192.0.2.10:1234", headers, okHandler())
	if response.Code != http.StatusUnauthorized {
		testingContext.Fatalf("wrong cookie did not take precedence: %d", response.Code)
	}
}

func TestAuthMiddlewareLoopbackBypassUsesTCPPeerOnly(testingContext *testing.T) {
	for _, remote := range []string{"127.0.0.1:1234", "127.23.4.5:1234", "[::1]:1234", "[::ffff:127.0.0.1]:1234"} {
		testingContext.Run(remote, func(testingContext *testing.T) {
			response := serveAuth(testSecret, "/private?key=wrong", remote, nil, okHandler())
			if response.Code != http.StatusOK {
				testingContext.Fatalf("status = %d", response.Code)
			}
			assertAuthCookie(testingContext, response)
		})
	}

	headers := http.Header{"X-Forwarded-For": {"127.0.0.1"}}
	response := serveAuth(testSecret, "/private", "192.0.2.10:1234", headers, okHandler())
	if response.Code != http.StatusUnauthorized {
		testingContext.Fatalf("trusted forwarded peer: %d", response.Code)
	}
}

func TestAuthMiddlewareUnauthorizedWebSocket(testingContext *testing.T) {
	headers := http.Header{"Accept": {"text/html"}}
	response := serveAuth(testSecret, "/ws", "192.0.2.10:1234", headers, okHandler())
	if response.Code != http.StatusUnauthorized {
		testingContext.Fatalf("status = %d", response.Code)
	}
	if strings.TrimSpace(response.Body.String()) != "unauthorized" {
		testingContext.Fatalf("body = %q", response.Body.String())
	}
	if response.Header().Get("Content-Type") != "text/plain; charset=utf-8" {
		testingContext.Fatalf("content type = %q", response.Header().Get("Content-Type"))
	}
	if response.Header().Get("Set-Cookie") != "" {
		testingContext.Fatal("websocket response set cookie")
	}
}

func TestAuthMiddlewareUnauthorizedHTMLPairingPage(testingContext *testing.T) {
	headers := http.Header{"Accept": {"application/xhtml+xml, TEXT/HTML;q=0.9"}}
	response := serveAuth(testSecret, "http://example.test:35126/private", "192.0.2.10:1234", headers, okHandler())
	if response.Code != http.StatusUnauthorized {
		testingContext.Fatalf("status = %d", response.Code)
	}
	if got := response.Header().Get("Content-Type"); got != "text/html; charset=utf-8" {
		testingContext.Fatalf("content type = %q", got)
	}
	body := response.Body.String()
	for _, want := range []string{
		"Pairing key required",
		"http://192.168.1.4:35126/?key=" + testSecret + "&auto=1",
		"http://127.0.0.1:35126/?key=" + testSecret + "&auto=1",
	} {
		if !strings.Contains(body, want) {
			testingContext.Fatalf("pairing page missing %q: %s", want, body)
		}
	}
}

func TestAuthMiddlewareUnauthorizedJSON(testingContext *testing.T) {
	response := serveAuth(testSecret, "/private", "192.0.2.10:1234", nil, okHandler())
	if response.Code != http.StatusUnauthorized {
		testingContext.Fatalf("status = %d", response.Code)
	}
	if got := response.Header().Get("Content-Type"); got != "application/json; charset=utf-8" {
		testingContext.Fatalf("content type = %q", got)
	}
	var body map[string]string
	if errorValue := json.Unmarshal(response.Body.Bytes(), &body); errorValue != nil {
		testingContext.Fatal(errorValue)
	}
	const want = "unauthorized · open the paired link from connect.url, or add ?key=<secret>"
	if body["error"] != want {
		testingContext.Fatalf("error = %q", body["error"])
	}
}

func TestAuthMiddlewareCookieAndWebSocketRules(testingContext *testing.T) {
	response := serveAuth(testSecret, "/private?key="+testSecret, "192.0.2.10:1234", nil, okHandler())
	assertAuthCookie(testingContext, response)

	response = serveAuth(testSecret, "/ws?key="+testSecret, "192.0.2.10:1234", nil, okHandler())
	if response.Code != http.StatusOK {
		testingContext.Fatalf("status = %d", response.Code)
	}
	if response.Header().Get("Set-Cookie") != "" {
		testingContext.Fatal("authorized websocket set cookie")
	}
}

func TestAuthMiddlewarePairingPagePortFallback(testingContext *testing.T) {
	headers := http.Header{"Accept": {"text/html"}}
	response := serveAuthWithOptions(testSecret, "http://example.test/private", "192.0.2.10:1234", headers, okHandler(), "192.168.1.4", 0)
	if !strings.Contains(response.Body.String(), "http://192.168.1.4:2421/") {
		testingContext.Fatalf("default port missing: %s", response.Body.String())
	}

	response = serveAuthWithOptions(testSecret, "http://example.test/private", "192.0.2.10:1234", headers, okHandler(), "192.168.1.4", 20997)
	if !strings.Contains(response.Body.String(), "http://192.168.1.4:20997/") {
		testingContext.Fatalf("configured port missing: %s", response.Body.String())
	}
}

func serveAuth(secret, target, remote string, headers http.Header, next http.Handler) *httptest.ResponseRecorder {
	return serveAuthWithOptions(secret, target, remote, headers, next, "192.168.1.4", 2421)
}

func serveAuthWithOptions(secret, target, remote string, headers http.Header, next http.Handler, lanIP string, port int) *httptest.ResponseRecorder {
	request := httptest.NewRequest(http.MethodGet, target, nil)
	request.RemoteAddr = remote
	for name, values := range headers {
		for _, value := range values {
			request.Header.Add(name, value)
		}
	}
	response := httptest.NewRecorder()
	authMiddleware(secret, lanIP, port, next).ServeHTTP(response, request)
	return response
}

func okHandler() http.Handler {
	return http.HandlerFunc(func(responseWriter http.ResponseWriter, _ *http.Request) {
		_, _ = responseWriter.Write([]byte("ok"))
	})
}

func assertAuthCookie(testingContext *testing.T, response *httptest.ResponseRecorder) {
	testingContext.Helper()
	cookies := response.Result().Cookies()
	if len(cookies) != 1 {
		testingContext.Fatalf("cookies = %#v", cookies)
	}
	cookie := cookies[0]
	if cookie.Name != authCookieName || cookie.Value != testSecret || cookie.Path != "/" || cookie.MaxAge != 30*24*60*60 {
		testingContext.Fatalf("cookie = %#v", cookie)
	}
	if !cookie.HttpOnly || cookie.SameSite != http.SameSiteLaxMode {
		testingContext.Fatalf("cookie flags = %#v", cookie)
	}
}
