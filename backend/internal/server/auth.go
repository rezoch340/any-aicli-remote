package server

import (
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"
)

const (
	authCookieName  = "grok_remote_key"
	defaultHTTPPort = 2421
)

func authMiddleware(secret, lanIP string, port int, next http.Handler) http.Handler {
	if next == nil {
		next = http.NotFoundHandler()
	}
	return http.HandlerFunc(func(responseWriter http.ResponseWriter, request *http.Request) {
		if secret == "" || request.URL.Query().Get("demo") == "1" ||
			request.URL.Path == "/health" || request.URL.Path == "/health/deep" {
			next.ServeHTTP(responseWriter, request)
			return
		}

		supplied := request.URL.Query().Get("key")
		if supplied == "" {
			if cookie, errorValue := request.Cookie(authCookieName); errorValue == nil {
				supplied = cookie.Value
			}
		}
		if supplied == "" {
			supplied = request.Header.Get("X-Grok-Remote-Key")
		}

		loopback := isLoopbackPeer(request.RemoteAddr)
		if !loopback && supplied != secret {
			writeUnauthorized(responseWriter, request, secret, lanIP, port)
			return
		}

		if request.URL.Path != "/ws" && (supplied == secret || loopback) {
			http.SetCookie(responseWriter, &http.Cookie{
				Name:     authCookieName,
				Value:    secret,
				Path:     "/",
				MaxAge:   int((30 * 24 * time.Hour) / time.Second),
				HttpOnly: true,
				SameSite: http.SameSiteLaxMode,
			})
		}
		next.ServeHTTP(responseWriter, request)
	})
}

func isLoopbackPeer(remoteAddress string) bool {
	peer := strings.TrimSpace(remoteAddress)
	if host, _, errorValue := net.SplitHostPort(peer); errorValue == nil {
		peer = host
	}
	peer = strings.Trim(peer, "[]")
	return peer == "127.0.0.1" || peer == "::1" || peer == "::ffff:127.0.0.1" || strings.HasPrefix(peer, "127.")
}

func writeUnauthorized(responseWriter http.ResponseWriter, request *http.Request, secret, lanIP string, port int) {
	if request.URL.Path == "/ws" {
		http.Error(responseWriter, "unauthorized", http.StatusUnauthorized)
		return
	}
	if request.Method == http.MethodGet && strings.Contains(strings.ToLower(request.Header.Get("Accept")), "text/html") {
		pairingPort := requestPort(request, port)
		if lanIP == "" {
			lanIP = "127.0.0.1"
		}
		local := fmt.Sprintf("http://127.0.0.1:%d/?key=%s&auto=1", pairingPort, secret)
		phone := fmt.Sprintf("http://%s:%d/?key=%s&auto=1", lanIP, pairingPort, secret)
		responseWriter.Header().Set("Content-Type", "text/html; charset=utf-8")
		responseWriter.WriteHeader(http.StatusUnauthorized)
		_, _ = fmt.Fprintf(responseWriter, "<!doctype html><meta charset=utf-8><meta name=viewport content=\"width=device-width,initial-scale=1\">"+
			"<title>Grok Remote — pair</title>"+
			"<body style=\"font-family:system-ui;max-width:36rem;margin:2rem auto;padding:0 1rem;line-height:1.5;background:#0b0d10;color:#e8eaed\">"+
			"<h1 style=\"font-size:1.25rem\">Pairing key required</h1>"+
			"<p>Open the <b>paired link</b> (has <code>?key=…</code>). Same Wi‑Fi as the PC.</p>"+
			"<p><a style=\"color:#7dd3fc\" href=\"%s\">Open phone link</a></p>"+
			"<p style=\"word-break:break-all;font-size:12px;opacity:.85\">%s</p>"+
			"<p><a style=\"color:#a7f3d0\" href=\"%s\">Open on this PC (localhost)</a></p>"+
			"</body>", phone, phone, local)
		return
	}

	responseWriter.Header().Set("Content-Type", "application/json; charset=utf-8")
	responseWriter.WriteHeader(http.StatusUnauthorized)
	_ = json.NewEncoder(responseWriter).Encode(map[string]string{
		"error": "unauthorized · open the paired link from connect.url, or add ?key=<secret>",
	})
}

func requestPort(request *http.Request, fallback int) int {
	if _, rawPort, errorValue := net.SplitHostPort(request.Host); errorValue == nil {
		if parsed, errorValue := strconv.Atoi(rawPort); errorValue == nil && parsed > 0 && parsed <= 65535 {
			return parsed
		}
	}
	if fallback > 0 && fallback <= 65535 {
		return fallback
	}
	return defaultHTTPPort
}
