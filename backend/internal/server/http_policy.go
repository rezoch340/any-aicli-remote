package server

import (
	"net/http"
	"time"

	"github.com/rezoch340/any-aicli-remote/backend/internal/config"
)

// newHTTPServer maps validated canonical HTTP tuning into the stdlib server.
func newHTTPServer(handler http.Handler, policy config.HTTPDocument) *http.Server {
	return &http.Server{
		Handler:           handler,
		ReadHeaderTimeout: policy.ReadHeaderTimeout.Duration,
		IdleTimeout:       policy.IdleTimeout.Duration,
		MaxHeaderBytes:    policy.MaxHeaderBytes,
	}
}

func cookieMaxAgeSeconds(policy config.HTTPDocument) int {
	return int(policy.AuthenticationCookieMaxAge.Duration / time.Second)
}
