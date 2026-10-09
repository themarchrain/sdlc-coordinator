package api

import (
	"context"
	"errors"
	"log"
	"net/http"
	"time"

	"go.uber.org/zap"
)

type Server struct {
	server *http.Server
}

func NewServer(addr string, handler http.Handler, logger *zap.Logger) *Server {
	if logger == nil {
		logger = zap.NewNop()
	}
	return &Server{
		server: &http.Server{
			Addr:              addr,
			Handler:           handler,
			ReadHeaderTimeout: 5 * time.Second,
			ErrorLog:          log.New(newZapLogWriter(logger, "http_server"), "", 0),
		},
	}
}

func (s *Server) Run() error {
	err := s.server.ListenAndServe()
	if errors.Is(err, http.ErrServerClosed) {
		return nil
	}
	return err
}

func (s *Server) Shutdown(ctx context.Context) error {
	return s.server.Shutdown(ctx)
}
