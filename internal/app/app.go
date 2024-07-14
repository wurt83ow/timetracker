package app

import (
	"context"
	"errors"
	"log"
	"net/http"
	"time"
)

type Server struct {
	srv *http.Server
	ctx context.Context
	// db  *pgxpool.Pool
}

func NewServer(ctx context.Context) *Server {
	server := new(Server)
	server.ctx = ctx

	return server
}

func (server *Server) Serve() {
}

func (server *Server) Shutdown() {
	log.Printf("server stopped")

	const shutdownTimeout = 5 * time.Second
	ctxShutDown, cancel := context.WithTimeout(context.Background(), shutdownTimeout)

	defer cancel()

	if err := server.srv.Shutdown(ctxShutDown); err != nil {
		if !errors.Is(err, http.ErrServerClosed) {
			//nolint:gocritic
			log.Fatalf("server Shutdown Failed:%s", err)
		}
	}

	log.Println("server exited properly")
}
