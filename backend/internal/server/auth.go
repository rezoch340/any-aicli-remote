package server

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/rezoch340/any-aicli-remote/backend/internal/compat"
)

func authMiddleware(secret string, cookieMaxAge int, next http.Handler) http.Handler {
	if next == nil {
		next = http.NotFoundHandler()
	}
	return http.HandlerFunc(func(responseWriter http.ResponseWriter, request *http.Request) {
		if secret == "" || request.URL.Path == "/health" {
			next.ServeHTTP(responseWriter, request)
			return
		}

		supplied := request.URL.Query().Get("key")
		if supplied == "" {
			supplied = compat.AuthenticationCookie(request)
		}
		if supplied == "" {
			supplied = compat.AuthenticationHeader(request)
		}

		if supplied != secret {
			writeUnauthorized(responseWriter, request)
			return
		}

		if request.URL.Path != "/ws" {
			http.SetCookie(responseWriter, &http.Cookie{
				Name:     compat.AuthenticationCookieName,
				Value:    secret,
				Path:     "/",
				MaxAge:   cookieMaxAge,
				HttpOnly: true,
				SameSite: http.SameSiteLaxMode,
			})
		}
		next.ServeHTTP(responseWriter, request)
	})
}

func writeUnauthorized(responseWriter http.ResponseWriter, request *http.Request) {
	if request.URL.Path == "/ws" {
		http.Error(responseWriter, "unauthorized", http.StatusUnauthorized)
		return
	}
	if request.Method == http.MethodGet && strings.Contains(strings.ToLower(request.Header.Get("Accept")), "text/html") {
		responseWriter.Header().Set("Content-Type", "text/html; charset=utf-8")
		responseWriter.WriteHeader(http.StatusUnauthorized)
		_, _ = responseWriter.Write([]byte("<!doctype html><meta charset=utf-8><meta name=viewport content=\"width=device-width,initial-scale=1\">" +
			"<title>Any AI CLI Remote — pair</title>" +
			"<body style=\"font-family:system-ui;max-width:36rem;margin:2rem auto;padding:0 1rem;line-height:1.5;background:#0b0d10;color:#e8eaed\">" +
			"<h1 style=\"font-size:1.25rem\">Pairing key required</h1>" +
			"<p>Scan the QR code shown by the trusted launcher, or open a pairing link that already contains your key.</p>" +
			"</body>"))
		return
	}

	responseWriter.Header().Set("Content-Type", "application/json; charset=utf-8")
	responseWriter.WriteHeader(http.StatusUnauthorized)
	_ = json.NewEncoder(responseWriter).Encode(map[string]string{
		"error": "unauthorized · scan the trusted launcher QR code or provide the pairing key",
	})
}
