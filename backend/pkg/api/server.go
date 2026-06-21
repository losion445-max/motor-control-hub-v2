package api

import (
	"context"
	"net/http"
	"time"
)

// NewServer builds an HTTP server with WebSocket and health endpoints.
//
//	GET /ws     — WebSocket connection for robot control
//	GET /health — liveness probe (200 OK)
func NewServer(addr string, h *Handler) *http.Server {
	mux := http.NewServeMux()
	mux.Handle("/ws", h)
	mux.HandleFunc("/health", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	return &http.Server{
		Addr:         addr,
		Handler:      mux,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 0,             // WebSocket connections are long-lived
		IdleTimeout:  120 * time.Second,
	}
}

// Shutdown gracefully stops the server within the deadline of ctx.
func Shutdown(ctx context.Context, srv *http.Server) error {
	return srv.Shutdown(ctx)
}
