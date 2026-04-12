package main

import (
	"context"
	"flag"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"goSQL/collector"
	"goSQL/config"
	"goSQL/db"
)

// Injected at build time via -ldflags (see Makefile).
var version, commit, buildTime string

func main() {
	log.SetFlags(log.LstdFlags | log.Lshortfile)
	log.Printf("roc-collector  version=%s commit=%s built=%s", version, commit, buildTime)

	// ── flags ────────────────────────────────────────────────────────────────
	fs := flag.NewFlagSet("roc-valores", flag.ExitOnError)
	sqlitePath := fs.String("sqlite", "", "use SQLite at `path` instead of Oracle (creates DB if absent)")
	if err := fs.Parse(os.Args[1:]); err != nil {
		log.Fatalf("flags: %v", err)
	}
	args := fs.Args() // remaining positional args after flags

	cmd := "run"
	if len(args) > 0 {
		cmd = args[0]
	}

	ctx := context.Background()

	// ── push: needs both SQLite (source) and Oracle (destination) ───────────
	if cmd == "push" {
		if *sqlitePath == "" {
			log.Fatalf("'push' requiere --sqlite <path>  (fuente de datos SQLite)")
		}
		if err := runPushToOracle(ctx, *sqlitePath); err != nil {
			log.Fatalf("push: %v", err)
		}
		return
	}

	// ── database (single backend for seed / sync / run) ───────────────────
	var database *db.DB

	if *sqlitePath != "" {
		var err error
		database, err = db.NewSQLite(*sqlitePath)
		if err != nil {
			log.Fatalf("sqlite: %v", err)
		}
	} else {
		cfg, err := config.Load()
		if err != nil {
			log.Fatalf("config: %v", err)
		}
		database, err = db.New(cfg)
		if err != nil {
			log.Fatalf("db: %v", err)
		}
	}
	defer database.Close()

	if err := database.HealthCheck(ctx); err != nil {
		log.Fatalf("healthcheck: %v", err)
	}

	c, err := collector.New(database, "config.yaml")
	if err != nil {
		log.Fatalf("collector: %v", err)
	}

	// ── subcommands ──────────────────────────────────────────────────────────
	switch cmd {
	case "seed":
		if err := c.EnsureSignals(ctx); err != nil {
			log.Fatalf("seed: %v", err)
		}
		log.Println("seed completado")

	case "sync":
		if err := c.EnsureSignals(ctx); err != nil {
			log.Fatalf("EnsureSignals: %v", err)
		}
		c.SyncAll(ctx)

	default: // "run" — daemon mode
		if err := c.EnsureSignals(ctx); err != nil {
			log.Fatalf("EnsureSignals: %v", err)
		}
		runDaemon(ctx, c)
	}
}

// runDaemon runs an initial sync, then syncs at :05 of every subsequent hour.
func runDaemon(ctx context.Context, c *collector.Collector) {
	log.Println("[main] daemon iniciado — sync cada hora en :05")

	c.SyncAll(ctx)

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	for {
		next := nextSyncTime()
		wait := time.Until(next)
		log.Printf("[main] próximo sync: %s (en %s)", next.Format("15:04:05"), wait.Truncate(time.Second))

		select {
		case <-quit:
			log.Println("[main] señal de parada recibida, cerrando...")
			return
		case <-time.After(wait):
			c.SyncAll(ctx)
		}
	}
}

// nextSyncTime returns the next :05 mark.
func nextSyncTime() time.Time {
	now := time.Now()
	candidate := time.Date(now.Year(), now.Month(), now.Day(), now.Hour(), 5, 0, 0, now.Location())
	if !candidate.After(now) {
		candidate = candidate.Add(time.Hour)
	}
	return candidate
}
