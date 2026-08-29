package httptransport

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"
)

type Server struct {
	httpServer *http.Server
}

func NewServer(address string, handler http.Handler) (*Server, error) {
	if strings.TrimSpace(address) == "" {
		return nil, fmt.Errorf("HTTP server address is required")
	}
	if handler == nil {
		return nil, fmt.Errorf("HTTP handler is required")
	}
	return &Server{httpServer: &http.Server{
		Addr:              address,
		Handler:           handler,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      15 * time.Second,
		IdleTimeout:       60 * time.Second,
	}}, nil
}

func (s *Server) ListenAndServe() error {
	err := s.httpServer.ListenAndServe()
	if errors.Is(err, http.ErrServerClosed) {
		return nil
	}
	return err
}

func (s *Server) Shutdown(ctx context.Context) error {
	return s.httpServer.Shutdown(ctx)
}
