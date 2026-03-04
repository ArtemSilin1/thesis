package server

import (
	"context"
	"fmt"
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

	fmt.Printf("Server started at: %s\n\n\n", address)

	return s.httpServer.ListenAndServe()
}

func (s *Server) Shutdown(ctx context.Context) error {
	return s.httpServer.Shutdown(ctx)
}
