package api

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"time"
)

// Server wraps http.Server with graceful-shutdown semantics.
type Server struct {
	hs *http.Server
}

func NewServer(addr string, handler http.Handler) *Server {
	return &Server{
		hs: &http.Server{
			Addr:              addr,
			Handler:           handler,
			ReadHeaderTimeout: 10 * time.Second,
		},
	}
}

// ListenAndServe blocks until the listener returns. ErrServerClosed is
// the expected return value after Shutdown.
func (s *Server) ListenAndServe() error {
	slog.Info("http listening", "addr", s.hs.Addr)
	err := s.hs.ListenAndServe()
	if errors.Is(err, http.ErrServerClosed) {
		return nil
	}
	return err
}

// Shutdown gives in-flight requests up to ctx's deadline to drain.
func (s *Server) Shutdown(ctx context.Context) error {
	return s.hs.Shutdown(ctx)
}
