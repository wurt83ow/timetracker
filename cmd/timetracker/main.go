package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/wurt83ow/timetracker/internal/app"
)

func main() {

	// Create a root context with the possibility of cancellation
	ctx, cancel := context.WithCancel(context.Background())

	// Create a channel for signal handling
	signalCh := make(chan os.Signal, 1)
	signal.Notify(signalCh, os.Interrupt, syscall.SIGTERM)

	// Start the server
	server := app.NewServer(ctx)
	go func() {
		// Wait for a signal
		sig := <-signalCh
		log.Printf("Received signal: %+v", sig)

		// Shutdown the server
		server.Shutdown()

		// Cancel the context
		cancel()
	}()

	// Start the server
	server.Serve()
}
