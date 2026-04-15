package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"goSQL/api"
	"goSQL/db"
)

// runServe starts the dashboard HTTP server.
// It opens the database (Oracle or SQLite) and serves:
//   - /api/*   — JSON endpoints (stations, signals, values, overview, stats)
//   - /*       — static frontend from the web/ directory
func runServe(ctx context.Context, database *db.DB, addr string) error {
	// Locate the web/ directory relative to the executable, then fall back
	// to the working directory.  This allows "go run" and compiled binary
	// to both find the static assets.
	webDir := findWebDir()
	if webDir == "" {
		return fmt.Errorf("cannot find web/ directory — run from the project root or place web/ next to the binary")
	}
	log.Printf("[serve] static files → %s", webDir)

	srv := api.New(database, webDir)

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

// findWebDir probes several locations for the web/ folder.
func findWebDir() string {
	candidates := []string{
		"web", // cwd
	}

	// next to the executable
	if exe, err := os.Executable(); err == nil {
		candidates = append(candidates, filepath.Join(filepath.Dir(exe), "web"))
	}

	for _, c := range candidates {
		if info, err := os.Stat(c); err == nil && info.IsDir() {
			abs, _ := filepath.Abs(c)
			return abs
		}
	}
	return ""
}
