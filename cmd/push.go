package main

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"

	"goSQL/config"
	"goSQL/db"
	"goSQL/models"
	"goSQL/repository"
)

// pushWorkers is the number of concurrent Oracle write goroutines.
// Set to 3 to stay well below Oracle MaxOpenConns (default 5): each worker
// may hold a write transaction while simultaneously issuing a MaxFecha query,
// so peak demand can be up to 2×workers connections briefly.
const pushWorkers = 3

// runPushToOracle transfers all data from a local SQLite database into Oracle.
//
// Why a remap is necessary:
//
//	SQLite SENAL_IDs are assigned starting from 1 on first seed.
//	Oracle SENAL_IDs may start at a different offset if the table already had rows.
//	The composite key B1|B2|B3|ELEMENT is the canonical signal identity.
//	buildIDRemap uses it to translate SQLite IDs → Oracle IDs before writing values.
//
// Incremental behaviour:
//
//	For each signal, Oracle MAX(FECHA) is checked first. Only values with
//	FECHA strictly after that timestamp are sent, so the push is safe to re-run.
func runPushToOracle(ctx context.Context, sqlitePath string) error {
	start := time.Now()

	// ── source: SQLite ───────────────────────────────────────────────────────
	sqliteDB, err := db.NewSQLite(sqlitePath)
	if err != nil {
		return fmt.Errorf("sqlite: %w", err)
	}
	defer sqliteDB.Close()
	// Relax the 1-writer limit since push only reads from this DB.
	sqliteDB.SetMaxOpenConns(pushWorkers)

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
		log.Println("[push] SQLite no tiene señales, nada que enviar")
		return nil
	}

	// ── stream values to Oracle (pushWorkers concurrent goroutines) ───────────
	total, err := pushValues(ctx, sqliteValorRepo, oracleValorRepo, remap)
	elapsed := time.Since(start)
	if err != nil {
		log.Printf("[push] interrumpido: %d valores enviados en %.1fs", total, elapsed.Seconds())
		return fmt.Errorf("push values: %w", err)
	}

	log.Printf("[push] completado: %d valores → Oracle en %.1fs", total, elapsed.Seconds())
	return nil
}

// pushValues sends SQLite values to Oracle using pushWorkers concurrent goroutines.
// Only records with FECHA > Oracle MAX(FECHA) are sent (incremental, idempotent).
// Progress is logged as "N/total (XX%) ..." after each signal completes.
func pushValues(
	ctx context.Context,
	sqliteValorRepo *repository.ValorRepository,
	oracleValorRepo *repository.ValorRepository,
	remap map[float64]float64,
) (int, error) {

	type job struct{ sqliteID, oracleID float64 }

	jobsTotal := len(remap)
	jobCh := make(chan job, jobsTotal)
	for sqliteID, oracleID := range remap {
		jobCh <- job{sqliteID, oracleID}
	}
	close(jobCh)

	var (
		mu       sync.Mutex
		firstErr error
		written  int
		skipped  int
		done     int
	)

	pushCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	var wg sync.WaitGroup
	for range pushWorkers {
		wg.Go(func() {
			for j := range jobCh {
				if pushCtx.Err() != nil {
					return
				}

				// Check how far Oracle already has data for this signal.
				lastOracleTS, hasOracle, err := oracleValorRepo.MaxFechaBySenalID(pushCtx, j.oracleID)
				if err != nil {
					mu.Lock()
					if firstErr == nil {
						firstErr = fmt.Errorf("oracle MaxFecha oracleID=%.0f: %w", j.oracleID, err)
						cancel()
					}
					mu.Unlock()
					return
				}

				// Read only the records Oracle doesn't have yet.
				var valores []models.RocValor
				if hasOracle {
					valores, err = sqliteValorRepo.FindBySenalIDFrom(pushCtx, j.sqliteID, lastOracleTS)
				} else {
					valores, err = sqliteValorRepo.FindBySenalID(pushCtx, j.sqliteID)
				}
				if err != nil {
					mu.Lock()
					if firstErr == nil {
						firstErr = fmt.Errorf("sqlite read sqliteID=%.0f: %w", j.sqliteID, err)
						cancel()
					}
					mu.Unlock()
					return
				}

				if len(valores) == 0 {
					mu.Lock()
					done++
					skipped++
					d := done
					mu.Unlock()
					log.Printf("[push] %d/%d (%d%%) sqliteID=%.0f → ya al día",
						d, jobsTotal, d*100/jobsTotal, j.sqliteID)
					continue
				}

				// Re-stamp SENAL_ID with the Oracle ID before writing.
				for k := range valores {
					valores[k].SenalID = j.oracleID
				}

				if err := oracleValorRepo.UpsertBatch(pushCtx, valores); err != nil {
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
				written += len(valores)
				d := done
				mu.Unlock()
				log.Printf("[push] %d/%d (%d%%) sqliteID=%.0f → oracleID=%.0f  %4d valores",
					d, jobsTotal, d*100/jobsTotal, j.sqliteID, j.oracleID, len(valores))
			}
		})
	}

	wg.Wait()

	// After wg.Wait() all goroutines have exited — safe to read without mutex.
	if firstErr != nil {
		return written, firstErr
	}
	if skipped > 0 {
		log.Printf("[push] %d señales ya al día en Oracle (sin nuevos valores)", skipped)
	}
	return written, nil
}

// buildIDRemap maps every SQLite SENAL_ID to its corresponding Oracle SENAL_ID
// using the composite key B1|B2|B3|ELEMENT as the stable signal identity.
// Signals present in SQLite but absent in Oracle are inserted into Oracle first.
func buildIDRemap(
	ctx context.Context,
	sqliteRepo *repository.SenalRepository,
	oracleRepo *repository.SenalRepository,
) (map[float64]float64, error) {

	// All SQLite signals with full column data.
	sqliteSenales, err := sqliteRepo.FindAll(ctx)
	if err != nil {
		return nil, fmt.Errorf("sqlite FindAll: %w", err)
	}

	// Oracle existing signals indexed by composite key.
	oracleByKey, err := oracleRepo.FindAllMap(ctx)
	if err != nil {
		return nil, fmt.Errorf("oracle FindAllMap: %w", err)
	}

	// Next available Oracle SENAL_ID.
	nextID, err := oracleRepo.NextID(ctx)
	if err != nil {
		return nil, fmt.Errorf("oracle NextID: %w", err)
	}

	remap := make(map[float64]float64, len(sqliteSenales))
	var toInsert []models.RocSenal

	for _, s := range sqliteSenales {
		key := senalKey(s)
		oracleID, ok := oracleByKey[key]
		if !ok {
			// Signal exists in SQLite but not Oracle — queue for batch insert.
			toInsert = append(toInsert, models.RocSenal{
				SenalID:  nextID,
				B1:       s.B1,
				B2:       s.B2,
				B3:       s.B3,
				Element:  s.Element,
				Unidades: s.Unidades,
				Activo:   s.Activo,
			})
			oracleByKey[key] = nextID
			oracleID = nextID
			nextID++
		}
		remap[s.SenalID] = oracleID
	}

	if len(toInsert) > 0 {
		if err := oracleRepo.InsertBatch(ctx, toInsert); err != nil {
			return nil, fmt.Errorf("oracle InsertBatch: %w", err)
		}
		for _, s := range toInsert {
			log.Printf("[push] nueva señal Oracle ID=%.0f: %s", s.SenalID, senalKey(s))
		}
	}

	log.Printf("[push] remap: %d señales (%d nuevas en Oracle)", len(remap), len(toInsert))
	return remap, nil
}

// senalKey builds the B1|B2|B3|ELEMENT composite key from a RocSenal.
func senalKey(s models.RocSenal) string {
	return fmt.Sprintf("%s|%s|%s|%s",
		ptrStr(s.B1), ptrStr(s.B2), ptrStr(s.B3), ptrStr(s.Element))
}

func ptrStr(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}
