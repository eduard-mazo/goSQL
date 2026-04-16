package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"

	"goSQL/api"
	"goSQL/db"
)

// runServe starts the REST API server.
//
// Usage:  roc-valores [--sqlite path] serve [:port]
//
// The server exposes JSON endpoints under /api/* and nothing else.
// The Vue dashboard (web-dashboard/) runs separately — either via
// Vite dev server during development or as a standalone static deploy.
func runServe(ctx context.Context, database *db.DB, addr string) error {
	srv := api.New(database)

	// Graceful shutdown on SIGINT / SIGTERM.
	srvCtx, cancel := context.WithCancel(ctx)
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-quit
		log.Println("[serve] shutting down…")
		cancel()
	}()

	return srv.ListenAndServe(srvCtx, addr)
}
