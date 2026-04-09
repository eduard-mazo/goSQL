package repository

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"time"

	"goSQL/db"
	"goSQL/models"
)

// MaxFechaBySenalID returns the MAX(FECHA) stored for a given SENAL_ID.
// Returns (zero, false, nil) when there are no rows yet.
func (r *ValorRepository) MaxFechaBySenalID(ctx context.Context, senalID float64) (time.Time, bool, error) {
	var n sql.NullTime
	err := r.db.QueryRowContext(ctx,
		`SELECT MAX(FECHA) FROM HEPMGA.ROC_VALORES WHERE SENAL_ID = :senal_id`,
		sql.Named("senal_id", senalID),
	).Scan(&n)
	if err != nil {
		return time.Time{}, false, fmt.Errorf("ValorRepo.MaxFechaBySenalID: %w", err)
	}
	if !n.Valid {
		return time.Time{}, false, nil
	}
	return n.Time, true, nil
}

// UpsertBatch inserts multiple values, skipping rows that already exist
// for the same (SENAL_ID, FECHA) pair.
func (r *ValorRepository) UpsertBatch(ctx context.Context, valores []models.RocValor) error {
	if len(valores) == 0 {
		return nil
	}

	// Oracle MERGE: insert only when the (SENAL_ID, FECHA) pair is absent.
	const q = `
		MERGE INTO HEPMGA.ROC_VALORES d
		USING DUAL ON (d.SENAL_ID = :senal_id AND d.FECHA = :fecha)
		WHEN NOT MATCHED THEN
			INSERT (FECHA, SYNCED_AT, SENAL_ID, VALOR)
			VALUES (:fecha, :synced_at, :senal_id, :valor)`

	return r.db.WithTx(ctx, func(tx *sql.Tx) error {
		stmt, err := tx.PrepareContext(ctx, q)
		if err != nil {
			return fmt.Errorf("ValorRepo.UpsertBatch prepare: %w", err)
		}
		defer stmt.Close()

		for _, v := range valores {
			if _, err := stmt.ExecContext(ctx,
				sql.Named("senal_id", v.SenalID),
				sql.Named("fecha", v.Fecha),
				sql.Named("synced_at", v.SyncedAt),
				sql.Named("valor", v.Valor),
			); err != nil {
				return fmt.Errorf("ValorRepo.UpsertBatch senal_id=%.0f fecha=%s: %w",
					v.SenalID, v.Fecha.Format(time.RFC3339), err)
			}
		}
		log.Printf("[repo] UpsertBatch → %d valores procesados", len(valores))
		return nil
	})
}

// ValorRepository opera sobre HEPMGA.ROC_VALORES.
type ValorRepository struct {
	db *db.DB
}

func NewValorRepository(database *db.DB) *ValorRepository {
	return &ValorRepository{db: database}
}

// ─── LECTURA ────────────────────────────────────────────────────────────────

// FindBySenalID devuelve todos los valores de una señal, más recientes primero.
func (r *ValorRepository) FindBySenalID(ctx context.Context, senalID float64) ([]models.RocValor, error) {
	const q = `
		SELECT FECHA, SYNCED_AT, SENAL_ID, VALOR
		FROM   HEPMGA.ROC_VALORES
		WHERE  SENAL_ID = :senal_id
		ORDER  BY FECHA DESC`

	rows, err := r.db.QueryContext(ctx, q, sql.Named("senal_id", senalID))
	if err != nil {
		return nil, fmt.Errorf("ValorRepo.FindBySenalID: %w", err)
	}
	defer rows.Close()

	return scanValores(rows)
}

// FindByRango devuelve valores de una señal en un rango de FECHA (inclusive).
func (r *ValorRepository) FindByRango(ctx context.Context, senalID float64, desde, hasta time.Time) ([]models.RocValor, error) {
	const q = `
		SELECT FECHA, SYNCED_AT, SENAL_ID, VALOR
		FROM   HEPMGA.ROC_VALORES
		WHERE  SENAL_ID = :senal_id
		  AND  FECHA    BETWEEN :desde AND :hasta
		ORDER  BY FECHA ASC`

	rows, err := r.db.QueryContext(ctx, q,
		sql.Named("senal_id", senalID),
		sql.Named("desde", desde),
		sql.Named("hasta", hasta),
	)
	if err != nil {
		return nil, fmt.Errorf("ValorRepo.FindByRango: %w", err)
	}
	defer rows.Close()

	return scanValores(rows)
}

// FindUltimos devuelve los N registros más recientes de todas las señales.
func (r *ValorRepository) FindUltimos(ctx context.Context, n int) ([]models.RocValor, error) {
	const q = `
		SELECT FECHA, SYNCED_AT, SENAL_ID, VALOR
		FROM   HEPMGA.ROC_VALORES
		ORDER  BY FECHA DESC
		FETCH  FIRST :n ROWS ONLY`

	rows, err := r.db.QueryContext(ctx, q, sql.Named("n", n))
	if err != nil {
		return nil, fmt.Errorf("ValorRepo.FindUltimos: %w", err)
	}
	defer rows.Close()

	return scanValores(rows)
}

// ─── ESCRITURA ──────────────────────────────────────────────────────────────

// Insert inserta un valor.
//
// SYNCED_AT se almacena exactamente como viene del controlador de campo —
// NO se sobreescribe con SYSDATE ni con time.Now().
func (r *ValorRepository) Insert(ctx context.Context, v models.RocValor) error {
	const q = `
		INSERT INTO HEPMGA.ROC_VALORES (FECHA, SYNCED_AT, SENAL_ID, VALOR)
		VALUES (:fecha, :synced_at, :senal_id, :valor)`

	return r.db.WithTx(ctx, func(tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, q,
			sql.Named("fecha", v.Fecha),
			sql.Named("synced_at", v.SyncedAt), // timestamp del controlador, tal cual
			sql.Named("senal_id", v.SenalID),
			sql.Named("valor", v.Valor),
		)
		if err != nil {
			return fmt.Errorf("ValorRepo.Insert: %w", err)
		}
		log.Printf("[repo] INSERT ROC_VALORES senal_id=%.0f fecha=%s synced_at=%s",
			v.SenalID,
			v.Fecha.Format(time.RFC3339),
			v.SyncedAt.Format(time.RFC3339),
		)
		return nil
	})
}

// InsertBatch inserta múltiples valores en una sola transacción.
// Cada RocValor lleva su propio SYNCED_AT capturado del controlador.
func (r *ValorRepository) InsertBatch(ctx context.Context, valores []models.RocValor) error {
	if len(valores) == 0 {
		return nil
	}

	const q = `
		INSERT INTO HEPMGA.ROC_VALORES (FECHA, SYNCED_AT, SENAL_ID, VALOR)
		VALUES (:fecha, :synced_at, :senal_id, :valor)`

	return r.db.WithTx(ctx, func(tx *sql.Tx) error {
		stmt, err := tx.PrepareContext(ctx, q)
		if err != nil {
			return fmt.Errorf("ValorRepo.InsertBatch prepare: %w", err)
		}
		defer stmt.Close()

		for _, v := range valores {
			if _, err := stmt.ExecContext(ctx,
				sql.Named("fecha", v.Fecha),
				sql.Named("synced_at", v.SyncedAt), // timestamp del controlador, tal cual
				sql.Named("senal_id", v.SenalID),
				sql.Named("valor", v.Valor),
			); err != nil {
				return fmt.Errorf("ValorRepo.InsertBatch senal_id=%.0f: %w", v.SenalID, err)
			}
		}

		log.Printf("[repo] InsertBatch → %d valores insertados", len(valores))
		return nil
	})
}

// ─── HELPERS ────────────────────────────────────────────────────────────────

func scanValores(rows *sql.Rows) ([]models.RocValor, error) {
	var result []models.RocValor

	for rows.Next() {
		var v models.RocValor
		var valor sql.NullFloat64

		if err := rows.Scan(&v.Fecha, &v.SyncedAt, &v.SenalID, &valor); err != nil {
			return nil, fmt.Errorf("ValorRepo scan: %w", err)
		}

		if valor.Valid {
			v.Valor = &valor.Float64
		}

		result = append(result, v)
	}

	return result, rows.Err()
}