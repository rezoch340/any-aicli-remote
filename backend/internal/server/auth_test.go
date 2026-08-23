package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/rezoch340/any-aicli-remote/backend/internal/compat"
)

const testSecret = "0123456789abcdef"

func TestAuthMiddlewareBypasses(testingContext *testing.T) {
	cases := []struct {
		name   string
		secret string
		target string
	}{
		{name: "empty secret", target: "/private"},
		{name: "health", secret: testSecret, target: "/health"},
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

func TestAuthMiddlewareDeepHealthRequiresPairingKey(testingContext *testing.T) {
	for _, remote := range []string{"192.0.2.10:1234", "127.0.0.1:1234", "[::1]:1234"} {
		testingContext.Run(remote, func(testingContext *testing.T) {
			response := serveAuth(testSecret, "/health/deep", remote, nil, okHandler())
			if response.Code != http.StatusUnauthorized {
				testingContext.Fatalf("status = %d", response.Code)
			}
		})
	}
}

func TestAuthMiddlewareDemoQueryDoesNotBypassAuthentication(testingContext *testing.T) {
	for _, target := range []string{"/private?demo=1", "/ws?demo=1", "/api/stack/stop?demo=1"} {
		testingContext.Run(target, func(testingContext *testing.T) {
			response := serveAuth(testSecret, target, "192.0.2.10:1234", nil, okHandler())
			if response.Code != http.StatusUnauthorized {
				testingContext.Fatalf("status = %d", response.Code)
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
		{name: "cookie", target: "/private", headers: http.Header{"Cookie": {compat.AuthenticationCookieName + "=" + testSecret}}},
		{name: "header", target: "/private", headers: http.Header{compat.AuthenticationHeaderName: {testSecret}}},
		{name: "legacy cookie compatibility", target: "/private", headers: http.Header{"Cookie": {"grok_remote_key=" + testSecret}}},
		{name: "legacy header compatibility", target: "/private", headers: http.Header{"X-Grok-Remote-Key": {testSecret}}},
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
		"Cookie":                        {compat.AuthenticationCookieName + "=" + testSecret},
		compat.AuthenticationHeaderName: {testSecret},
	}
	response := serveAuth(testSecret, "/private?key=wrong", "192.0.2.10:1234", headers, okHandler())
	if response.Code != http.StatusUnauthorized {
		testingContext.Fatalf("wrong query key did not take precedence: %d", response.Code)
	}

	headers = http.Header{
		"Cookie":                        {compat.AuthenticationCookieName + "=wrong"},
		compat.AuthenticationHeaderName: {testSecret},
	}
	response = serveAuth(testSecret, "/private", "192.0.2.10:1234", headers, okHandler())
	if response.Code != http.StatusUnauthorized {
		testingContext.Fatalf("wrong cookie did not take precedence: %d", response.Code)
	}
}

func TestAuthMiddlewareLoopbackStillRequiresPairingKey(testingContext *testing.T) {
	for _, remote := range []string{"127.0.0.1:1234", "127.23.4.5:1234", "[::1]:1234", "[::ffff:127.0.0.1]:1234"} {
		testingContext.Run(remote, func(testingContext *testing.T) {
			response := serveAuth(testSecret, "/private?key=wrong", remote, nil, okHandler())
			if response.Code != http.StatusUnauthorized {
				testingContext.Fatalf("status = %d", response.Code)
			}
			if response.Header().Get("Set-Cookie") != "" {
				testingContext.Fatal("unauthorized loopback response set cookie")
			}
		})
	}

	headers := http.Header{compat.AuthenticationHeaderName: {testSecret}}
	response := serveAuth(testSecret, "/private", "127.0.0.1:1234", headers, okHandler())
	if response.Code != http.StatusOK {
		testingContext.Fatalf("authenticated loopback status: %d", response.Code)
	}
	assertAuthCookie(testingContext, response)
}

func TestAuthMiddlewareUnauthorizedWebSocket(testingContext *testing.T) {
	headers := http.Header{"Accept": {"text/html"}, "Origin": {"https://attacker.example"}}
	for _, remote := range []string{"192.0.2.10:1234", "127.0.0.1:1234", "[::1]:1234"} {
		testingContext.Run(remote, func(testingContext *testing.T) {
			response := serveAuth(testSecret, "/ws", remote, headers, okHandler())
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
		})
	}
}

func TestAuthMiddlewareUnauthorizedHTMLPairingPage(testingContext *testing.T) {
	headers := http.Header{"Accept": {"application/xhtml+xml, TEXT/HTML;q=0.9"}}
	for _, target := range []string{
		"http://remote.example:24443/private",
		"http://remote.example:24443/pair",
		"http://remote.example:24443/?auto=1",
	} {
		testingContext.Run(target, func(testingContext *testing.T) {
			response := serveAuth(testSecret, target, "192.0.2.10:1234", headers, okHandler())
			if response.Code != http.StatusUnauthorized {
				testingContext.Fatalf("status = %d", response.Code)
			}
			if contentType := response.Header().Get("Content-Type"); contentType != "text/html; charset=utf-8" {
				testingContext.Fatalf("content type = %q", contentType)
			}
			body := response.Body.String()
			for _, expectedText := range []string{"Pairing key required", "trusted launcher"} {
				if !strings.Contains(body, expectedText) {
					testingContext.Fatalf("pairing page missing %q: %s", expectedText, body)
				}
			}
			if strings.Contains(body, testSecret) || strings.Contains(body, "?key=") {
				testingContext.Fatalf("unauthorized page disclosed pairing material: %s", body)
			}
		})
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
	const want = "unauthorized · scan the trusted launcher QR code or provide the pairing key"
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

func serveAuth(secret, target, remote string, headers http.Header, next http.Handler) *httptest.ResponseRecorder {
	request := httptest.NewRequest(http.MethodGet, target, nil)
	request.RemoteAddr = remote
	for name, values := range headers {
		for _, value := range values {
			request.Header.Add(name, value)
		}
	}
	response := httptest.NewRecorder()
	authMiddleware(secret, 30*24*60*60, next).ServeHTTP(response, request)
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
	if cookie.Name != compat.AuthenticationCookieName || cookie.Value != testSecret || cookie.Path != "/" || cookie.MaxAge != 30*24*60*60 {
		testingContext.Fatalf("cookie = %#v", cookie)
	}
	if !cookie.HttpOnly || cookie.SameSite != http.SameSiteLaxMode {
		testingContext.Fatalf("cookie flags = %#v", cookie)
	}
}
