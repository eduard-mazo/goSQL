package main

import (
	"context"
	"fmt"
	"log"
	"time"

	"goSQL/config"
	"goSQL/db"
	"goSQL/models"
	"goSQL/repository"
)

// runPushToOracle transfers all data from a local SQLite database into Oracle.
//
// Why a remap is necessary:
//   SQLite SENAL_IDs are assigned starting from 1 on first seed.
//   Oracle SENAL_IDs may start at a different offset if the table already had rows.
//   The composite key B1|B2|B3|ELEMENT is the canonical signal identity.
//   buildIDRemap uses it to translate SQLite IDs → Oracle IDs before writing values.
//
// Incremental behaviour:
//   For each signal, Oracle MAX(FECHA) is checked first. Only values with
//   FECHA strictly after that timestamp are sent, so the push is safe to re-run.
func runPushToOracle(ctx context.Context, sqlitePath string) error {
	start := time.Now()

	// ── source: SQLite ───────────────────────────────────────────────────────
	sqliteDB, err := db.NewSQLite(sqlitePath)
	if err != nil {
		return fmt.Errorf("sqlite: %w", err)
	}
	defer sqliteDB.Close()

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

	// ── stream values to Oracle ───────────────────────────────────────────────
	total, err := pushValues(ctx, sqliteValorRepo, oracleValorRepo, remap)
	if err != nil {
		return fmt.Errorf("push values: %w", err)
	}

	log.Printf("[push] completado: %d valores → Oracle en %.1fs", total, time.Since(start).Seconds())
	return nil
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

// pushValues sends SQLite values to Oracle signal by signal.
// Only records with FECHA > Oracle MAX(FECHA) are sent (incremental).
func pushValues(
	ctx context.Context,
	sqliteValorRepo *repository.ValorRepository,
	oracleValorRepo *repository.ValorRepository,
	remap map[float64]float64,
) (int, error) {

	total := 0
	skipped := 0

	for sqliteID, oracleID := range remap {
		// Check how far Oracle already has data for this signal.
		lastOracleTS, hasOracle, err := oracleValorRepo.MaxFechaBySenalID(ctx, oracleID)
		if err != nil {
			return total, fmt.Errorf("oracle MaxFecha oracleID=%.0f: %w", oracleID, err)
		}

		// Read only the records Oracle doesn't have yet.
		var valores []models.RocValor
		if hasOracle {
			valores, err = sqliteValorRepo.FindBySenalIDFrom(ctx, sqliteID, lastOracleTS)
		} else {
			valores, err = sqliteValorRepo.FindBySenalID(ctx, sqliteID)
		}
		if err != nil {
			return total, fmt.Errorf("sqlite read sqliteID=%.0f: %w", sqliteID, err)
		}

		if len(valores) == 0 {
			skipped++
			continue
		}

		// Re-stamp SENAL_ID with the Oracle ID before writing.
		for i := range valores {
			valores[i].SenalID = oracleID
		}

		if err := oracleValorRepo.UpsertBatch(ctx, valores); err != nil {
			return total, fmt.Errorf("oracle upsert oracleID=%.0f: %w", oracleID, err)
		}

		total += len(valores)
		log.Printf("[push]  sqliteID=%.0f → oracleID=%.0f  %4d valores", sqliteID, oracleID, len(valores))
	}

	if skipped > 0 {
		log.Printf("[push] %d señales ya al día en Oracle (sin nuevos valores)", skipped)
	}
	return total, nil
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
