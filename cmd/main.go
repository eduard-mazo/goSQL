package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"goSQL/config"
	"goSQL/db"
	"goSQL/models"
	"goSQL/repository"
)

func main() {
	log.SetFlags(log.LstdFlags | log.Lshortfile)
	log.Println("=== ROC_SENALES / ROC_VALORES — go-ora (sin CGO) ===")

	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("config: %v", err)
	}

	database, err := db.New(cfg)
	if err != nil {
		log.Fatalf("db.New: %v", err)
	}
	defer database.Close()

	ctx := context.Background()

	if err := database.HealthCheck(ctx); err != nil {
		log.Fatalf("healthcheck: %v", err)
	}
	log.Println("✓ Oracle responde")

	senalRepo := repository.NewSenalRepository(database)
	valorRepo := repository.NewValorRepository(database)

	cmd := "senales"
	if len(os.Args) > 1 {
		cmd = os.Args[1]
	}

	switch cmd {
	case "senales":
		demoSenales(ctx, senalRepo)
	case "valores":
		demoValores(ctx, valorRepo)
	case "insertar":
		demoInsertar(ctx, valorRepo)
	case "batch":
		demoBatch(ctx, valorRepo)
	default:
		demoSenales(ctx, senalRepo)
	}

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	log.Println("cerrando...")
}

// ── demos ────────────────────────────────────────────────────────────────────

func demoSenales(ctx context.Context, repo *repository.SenalRepository) {
	log.Println("\n── señales activas ──")

	senales, err := repo.FindActivas(ctx)
	if err != nil {
		log.Printf("ERROR: %v", err)
		return
	}
	if len(senales) == 0 {
		log.Println("sin registros")
		return
	}

	fmt.Printf("%-10s  %-15s  %-15s  %-15s  %-15s  %-10s\n",
		"SENAL_ID", "B1", "B2", "B3", "ELEMENT", "UNIDADES")
	fmt.Println("─────────────────────────────────────────────────────────────────────────────")
	for _, s := range senales {
		fmt.Printf("%-10.0f  %-15s  %-15s  %-15s  %-15s  %-10s\n",
			s.SenalID,
			strVal(s.B1), strVal(s.B2), strVal(s.B3),
			strVal(s.Element), strVal(s.Unidades),
		)
	}
}

func demoValores(ctx context.Context, repo *repository.ValorRepository) {
	log.Println("\n── últimos 10 valores ──")

	valores, err := repo.FindUltimos(ctx, 10)
	if err != nil {
		log.Printf("ERROR: %v", err)
		return
	}
	if len(valores) == 0 {
		log.Println("sin registros")
		return
	}

	fmt.Printf("%-10s  %-26s  %-26s  %-12s\n",
		"SENAL_ID", "FECHA", "SYNCED_AT (controlador)", "VALOR")
	fmt.Println("─────────────────────────────────────────────────────────────────────────────")
	for _, v := range valores {
		fmt.Printf("%-10.0f  %-26s  %-26s  %-12s\n",
			v.SenalID,
			v.Fecha.Format("2006-01-02 15:04:05.000"),
			v.SyncedAt.Format("2006-01-02 15:04:05.000"), // timestamp del campo
			fmtPtr(v.Valor),
		)
	}
}

func demoInsertar(ctx context.Context, repo *repository.ValorRepository) {
	log.Println("\n── insertando valor de prueba ──")

	// Simula timestamps recibidos del controlador de campo
	ahora := time.Now()
	syncedAt := ahora.Add(-500 * time.Millisecond) // el controlador disparó 500ms antes

	v := models.RocValor{
		Fecha:    ahora,
		SyncedAt: syncedAt, // viene del controlador — se guarda exactamente así
		SenalID:  1,
		Valor:    models.F(42.75),
	}

	if err := repo.Insert(ctx, v); err != nil {
		log.Printf("ERROR Insert: %v", err)
		return
	}
	log.Printf("✓ valor insertado senal_id=%.0f valor=%.2f synced_at=%s",
		v.SenalID, *v.Valor, v.SyncedAt.Format(time.RFC3339Nano))
}

func demoBatch(ctx context.Context, repo *repository.ValorRepository) {
	log.Println("\n── insertando lote ──")

	// Cada fila lleva el SYNCED_AT que capturó el controlador para ese evento
	t0 := time.Now()
	lote := []models.RocValor{
		{Fecha: t0, SyncedAt: t0.Add(-100 * time.Millisecond), SenalID: 1, Valor: models.F(10.1)},
		{Fecha: t0, SyncedAt: t0.Add(-200 * time.Millisecond), SenalID: 2, Valor: models.F(20.2)},
		{Fecha: t0, SyncedAt: t0.Add(-300 * time.Millisecond), SenalID: 3, Valor: nil}, // VALOR NULL
	}

	if err := repo.InsertBatch(ctx, lote); err != nil {
		log.Printf("ERROR InsertBatch: %v", err)
		return
	}
	log.Printf("✓ lote de %d valores insertado", len(lote))
}

// ── helpers ──────────────────────────────────────────────────────────────────

func fmtPtr(p *float64) string {
	if p == nil {
		return "NULL"
	}
	return fmt.Sprintf("%.4f", *p)
}

func strVal(p *string) string {
	if p == nil {
		return "NULL"
	}
	return *p
}