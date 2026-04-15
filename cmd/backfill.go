package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"strings"
	"sync"
	"time"

	"goSQL/config"
	"goSQL/db"
	"goSQL/repository"
)

// backfillWorkers is the number of concurrent Oracle write goroutines.
// Same rationale as pushWorkers: stay below Oracle MaxOpenConns.
const backfillWorkers = 3

// runBackfillToOracle transfers historical data from SQLite to Oracle.
//
// Unlike push (which only sends FECHA > Oracle MAX), backfill targets
// the backward gap: FECHA < Oracle MIN. This covers data that was
// migrated into SQLite from older sources (e.g. modbus.db) after the
// initial push had already synced newer data.
//
// Safe to re-run: UpsertBatch uses MERGE / ON CONFLICT DO NOTHING.
func runBackfillToOracle(ctx context.Context, sqlitePath string) error {
	start := time.Now()

	// ── source: SQLite ───────────────────────────────────────────────────────
	sqliteDB, err := db.NewSQLite(sqlitePath)
	if err != nil {
		return fmt.Errorf("sqlite: %w", err)
	}
	defer sqliteDB.Close()
	sqliteDB.SetMaxOpenConns(backfillWorkers)

	// ── destination: Oracle ──────────────────────────────────────────────────
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("oracle config: %w", err)
	}
	oracleDB, err := db.New(cfg)
	if err != nil {
		return fmt.Errorf("oracle: %w", err)
	}
	defer oracleDB.Close()

	sqliteSenalRepo := repository.NewSenalRepository(sqliteDB)
	sqliteValorRepo := repository.NewValorRepository(sqliteDB)
	oracleSenalRepo := repository.NewSenalRepository(oracleDB)
	oracleValorRepo := repository.NewValorRepository(oracleDB)

	// ── build sqlite_id → oracle_id remap ────────────────────────────────────
	remap, err := buildIDRemap(ctx, sqliteSenalRepo, oracleSenalRepo)
	if err != nil {
		return fmt.Errorf("remap: %w", err)
	}
	if len(remap) == 0 {
		log.Println("[backfill] SQLite no tiene señales, nada que enviar")
		return nil
	}

	// ── stream historical values to Oracle ───────────────────────────────────
	pr, err := backfillValues(ctx, sqliteValorRepo, oracleValorRepo, remap)
	elapsed := time.Since(start)

	// ── summary ──────────────────────────────────────────────────────────────
	const sep = "────────────────────────────────────────────────────"
	log.Printf("[backfill] %s", sep)
	if err != nil {
		log.Printf("[backfill]  Estado:        INTERRUMPIDO")
	} else if pr.inserted == 0 {
		log.Printf("[backfill]  Estado:        AL DIA")
	} else {
		log.Printf("[backfill]  Estado:        OK")
	}
	log.Printf("[backfill]  Duración:      %.1fs", elapsed.Seconds())
	log.Printf("[backfill]  Señales:       %d total, %d con datos históricos, %d sin gap",
		pr.signalsTotal, pr.signalsWithNew, pr.signalsUpToDate)
	log.Printf("[backfill]  Insertados:    %d valores históricos en Oracle", pr.inserted)
	if !pr.minFecha.IsZero() {
		log.Printf("[backfill]  Rango:         %s → %s",
			pr.minFecha.Format("2006-01-02 15:04"),
			pr.maxFecha.Format("2006-01-02 15:04"))
	}
	if err != nil {
		log.Printf("[backfill]  Error:         %v", err)
	}
	log.Printf("[backfill] %s", sep)

	if err != nil {
		return fmt.Errorf("backfill values: %w", err)
	}
	return nil
}

// backfillValues sends SQLite historical values (FECHA < Oracle MIN) to Oracle.
// Uses backfillWorkers concurrent goroutines with an animated progress bar.
func backfillValues(
	ctx context.Context,
	sqliteValorRepo *repository.ValorRepository,
	oracleValorRepo *repository.ValorRepository,
	remap map[float64]float64,
) (pushResult, error) {

	type job struct{ sqliteID, oracleID float64 }

	jobsTotal := len(remap)
	jobCh := make(chan job, jobsTotal)
	for sqliteID, oracleID := range remap {
		jobCh <- job{sqliteID, oracleID}
	}
	close(jobCh)

	var (
		mu              sync.Mutex
		firstErr        error
		inserted        int
		signalsWithNew  int
		signalsUpToDate int
		done            int
		minFecha        time.Time
		maxFecha        time.Time
	)

	renderBar := func() {
		pct := done * 100 / jobsTotal
		const width = 30
		filled := width * pct / 100
		bar := strings.Repeat("█", filled) + strings.Repeat("░", width-filled)
		line := fmt.Sprintf("\r[backfill] [%s] %3d%%  %d/%d señales  históricos:%d",
			bar, pct, done, jobsTotal, inserted)
		fmt.Fprintf(os.Stderr, "%-85s", line)
	}

	bfCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	var wg sync.WaitGroup
	for range backfillWorkers {
		wg.Go(func() {
			for j := range jobCh {
				if bfCtx.Err() != nil {
					return
				}

				// Get Oracle MIN(FECHA) for this signal.
				oracleMin, hasOracle, err := oracleValorRepo.MinFechaBySenalID(bfCtx, j.oracleID)
				if err != nil {
					mu.Lock()
					if firstErr == nil {
						firstErr = fmt.Errorf("oracle MinFecha oracleID=%.0f: %w", j.oracleID, err)
						cancel()
					}
					mu.Unlock()
					return
				}

				if !hasOracle {
					// Oracle has no data at all — nothing to backfill (push handles full sync).
					mu.Lock()
					done++
					signalsUpToDate++
					renderBar()
					mu.Unlock()
					continue
				}

				// Get SQLite rows with FECHA < Oracle MIN.
				valores, err := sqliteValorRepo.FindBySenalIDBefore(bfCtx, j.sqliteID, oracleMin)
				if err != nil {
					mu.Lock()
					if firstErr == nil {
						firstErr = fmt.Errorf("sqlite read backfill sqliteID=%.0f: %w", j.sqliteID, err)
						cancel()
					}
					mu.Unlock()
					return
				}

				if len(valores) == 0 {
					mu.Lock()
					done++
					signalsUpToDate++
					renderBar()
					mu.Unlock()
					continue
				}

				// Re-stamp SENAL_ID with the Oracle ID before writing.
				for k := range valores {
					valores[k].SenalID = j.oracleID
				}

				ur, err := oracleValorRepo.UpsertBatch(bfCtx, valores)
				if err != nil {
					mu.Lock()
					if firstErr == nil {
						firstErr = fmt.Errorf("oracle upsert oracleID=%.0f: %w", j.oracleID, err)
						cancel()
					}
					mu.Unlock()
					return
				}

				mu.Lock()
				done++
				inserted += ur.Inserted
				if ur.Inserted > 0 {
					signalsWithNew++
					for _, v := range valores {
						if minFecha.IsZero() || v.Fecha.Before(minFecha) {
							minFecha = v.Fecha
						}
						if v.Fecha.After(maxFecha) {
							maxFecha = v.Fecha
						}
					}
				}
				renderBar()
				mu.Unlock()
			}
		})
	}

	wg.Wait()

	// Clear the progress bar line so the summary starts clean.
	fmt.Fprintf(os.Stderr, "\r%-85s\r", "")

	pr := pushResult{
		inserted:        inserted,
		signalsTotal:    jobsTotal,
		signalsWithNew:  signalsWithNew,
		signalsUpToDate: signalsUpToDate,
		minFecha:        minFecha,
		maxFecha:        maxFecha,
	}
	if firstErr != nil {
		return pr, firstErr
	}
	return pr, nil
}
