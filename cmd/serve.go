package main

import (
	"context"
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
//   - /*       — static frontend (if a dist/web directory is found)
//
// In development the Vue app runs on its own Vite dev server and proxies
// /api calls to this backend, so no static directory is needed.
func runServe(ctx context.Context, database *db.DB, addr string) error {
	webDir := findWebDir()
	if webDir != "" {
		log.Printf("[serve] static files → %s", webDir)
	} else {
		log.Println("[serve] no static directory found — API-only mode")
		log.Println("[serve] (run 'npm run build' inside web-dashboard/ to generate dist/)")
	}

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

// findWebDir probes several locations for the frontend build output.
// Priority: web-dashboard/dist (Vue build) → web/ (legacy vanilla).
func findWebDir() string {
	var candidates []string

	// cwd-relative
	candidates = append(candidates,
		filepath.Join("web-dashboard", "dist"), // Vue build output
		"web", // legacy vanilla dashboard
	)

	// next to the executable
	if exe, err := os.Executable(); err == nil {
		dir := filepath.Dir(exe)
		candidates = append(candidates,
			filepath.Join(dir, "web-dashboard", "dist"),
			filepath.Join(dir, "web"),
		)
	}

	for _, c := range candidates {
		if info, err := os.Stat(c); err == nil && info.IsDir() {
			abs, _ := filepath.Abs(c)
			return abs
		}
	}
	return ""
}
