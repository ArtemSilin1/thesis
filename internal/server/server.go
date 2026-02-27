package server

import (
	"context"
	"net/http"
	"time"
)

type Server struct {
	httpServer *http.Server
}

func (s *Server) RunServer(address string, h http.Handler) error {
	s.httpServer = &http.Server{
		Addr:           address,
		Handler:        h,
		MaxHeaderBytes: 1 << 20,
		WriteTimeout:   30 * time.Second,
		ReadTimeout:    10 * time.Second,
	}

	return s.httpServer.ListenAndServe()
}

func (s *Server) Shutdown(ctx context.Context) error {
	return s.httpServer.Shutdown(ctx)
}
