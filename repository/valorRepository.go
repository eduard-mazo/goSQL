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

// ValorRepository opera sobre ROC_VALORES (serie temporal).
type ValorRepository struct {
	db *db.DB
}

func NewValorRepository(database *db.DB) *ValorRepository {
	return &ValorRepository{db: database}
}

func valoresTable(d db.Dialect) string {
	if d == db.DialectSQLite {
		return "ROC_VALORES"
	}
	return "HEPMGA.ROC_VALORES"
}

// ── escritura ────────────────────────────────────────────────────────────────

// UpsertBatch inserts multiple values, skipping rows that already exist
// for the same (SENAL_ID, FECHA) pair.
func (r *ValorRepository) UpsertBatch(ctx context.Context, valores []models.RocValor) error {
	if len(valores) == 0 {
		return nil
	}

	if r.db.Dialect == db.DialectSQLite {
		return r.db.WithTx(ctx, func(tx *sql.Tx) error {
			stmt, err := tx.PrepareContext(ctx,
				`INSERT INTO ROC_VALORES (FECHA, SYNCED_AT, SENAL_ID, VALOR)
				 VALUES (?, ?, ?, ?)
				 ON CONFLICT(SENAL_ID, FECHA) DO NOTHING`)
			if err != nil {
				return fmt.Errorf("ValorRepo.UpsertBatch prepare: %w", err)
			}
			defer stmt.Close()
			for _, v := range valores {
				if _, err := stmt.ExecContext(ctx,
					v.Fecha.UTC().Format(time.RFC3339),
					v.SyncedAt.UTC().Format(time.RFC3339),
					v.SenalID, v.Valor,
				); err != nil {
					return fmt.Errorf("ValorRepo.UpsertBatch senal_id=%.0f fecha=%s: %w",
						v.SenalID, v.Fecha.Format(time.RFC3339), err)
				}
			}
			log.Printf("[repo] UpsertBatch → %d valores procesados", len(valores))
			return nil
		})
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

// Insert inserts a single value.
func (r *ValorRepository) Insert(ctx context.Context, v models.RocValor) error {
	return r.db.WithTx(ctx, func(tx *sql.Tx) error {
		var err error
		if r.db.Dialect == db.DialectSQLite {
			_, err = tx.ExecContext(ctx,
				`INSERT INTO ROC_VALORES (FECHA, SYNCED_AT, SENAL_ID, VALOR) VALUES (?, ?, ?, ?)`,
				v.Fecha.UTC().Format(time.RFC3339),
				v.SyncedAt.UTC().Format(time.RFC3339),
				v.SenalID, v.Valor,
			)
		} else {
			_, err = tx.ExecContext(ctx,
				`INSERT INTO HEPMGA.ROC_VALORES (FECHA, SYNCED_AT, SENAL_ID, VALOR)
				 VALUES (:fecha, :synced_at, :senal_id, :valor)`,
				sql.Named("fecha", v.Fecha),
				sql.Named("synced_at", v.SyncedAt),
				sql.Named("senal_id", v.SenalID),
				sql.Named("valor", v.Valor),
			)
		}
		if err != nil {
			return fmt.Errorf("ValorRepo.Insert: %w", err)
		}
		log.Printf("[repo] INSERT ROC_VALORES senal_id=%.0f fecha=%s", v.SenalID, v.Fecha.Format(time.RFC3339))
		return nil
	})
}

// InsertBatch inserts multiple values in a single transaction.
func (r *ValorRepository) InsertBatch(ctx context.Context, valores []models.RocValor) error {
	if len(valores) == 0 {
		return nil
	}
	return r.db.WithTx(ctx, func(tx *sql.Tx) error {
		var (
			stmt *sql.Stmt
			err  error
		)
		if r.db.Dialect == db.DialectSQLite {
			stmt, err = tx.PrepareContext(ctx,
				`INSERT INTO ROC_VALORES (FECHA, SYNCED_AT, SENAL_ID, VALOR) VALUES (?, ?, ?, ?)`)
		} else {
			stmt, err = tx.PrepareContext(ctx,
				`INSERT INTO HEPMGA.ROC_VALORES (FECHA, SYNCED_AT, SENAL_ID, VALOR)
				 VALUES (:fecha, :synced_at, :senal_id, :valor)`)
		}
		if err != nil {
			return fmt.Errorf("ValorRepo.InsertBatch prepare: %w", err)
		}
		defer stmt.Close()

		for _, v := range valores {
			if r.db.Dialect == db.DialectSQLite {
				_, err = stmt.ExecContext(ctx,
					v.Fecha.UTC().Format(time.RFC3339),
					v.SyncedAt.UTC().Format(time.RFC3339),
					v.SenalID, v.Valor,
				)
			} else {
				_, err = stmt.ExecContext(ctx,
					sql.Named("fecha", v.Fecha),
					sql.Named("synced_at", v.SyncedAt),
					sql.Named("senal_id", v.SenalID),
					sql.Named("valor", v.Valor),
				)
			}
			if err != nil {
				return fmt.Errorf("ValorRepo.InsertBatch senal_id=%.0f: %w", v.SenalID, err)
			}
		}
		log.Printf("[repo] InsertBatch → %d valores insertados", len(valores))
		return nil
	})
}

// ── lectura ──────────────────────────────────────────────────────────────────

// MaxFechaBySenalID returns the MAX(FECHA) stored for a given SENAL_ID.
// Returns (zero, false, nil) when there are no rows yet.
func (r *ValorRepository) MaxFechaBySenalID(ctx context.Context, senalID float64) (time.Time, bool, error) {
	if r.db.Dialect == db.DialectSQLite {
		var s sql.NullString
		err := r.db.QueryRowContext(ctx,
			`SELECT MAX(FECHA) FROM ROC_VALORES WHERE SENAL_ID = ?`, senalID,
		).Scan(&s)
		if err != nil {
			return time.Time{}, false, fmt.Errorf("ValorRepo.MaxFechaBySenalID: %w", err)
		}
		if !s.Valid || s.String == "" {
			return time.Time{}, false, nil
		}
		t, err := time.Parse(time.RFC3339, s.String)
		if err != nil {
			return time.Time{}, false, fmt.Errorf("ValorRepo.MaxFechaBySenalID parse %q: %w", s.String, err)
		}
		return t.Local(), true, nil
	}

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

// FindBySenalIDFrom returns values for a signal with FECHA strictly after `after`,
// ordered by FECHA ascending. Used for incremental SQLite → Oracle push.
func (r *ValorRepository) FindBySenalIDFrom(ctx context.Context, senalID float64, after time.Time) ([]models.RocValor, error) {
	tbl := valoresTable(r.db.Dialect)
	var (
		rows *sql.Rows
		err  error
	)
	if r.db.Dialect == db.DialectSQLite {
		rows, err = r.db.QueryContext(ctx,
			`SELECT FECHA, SYNCED_AT, SENAL_ID, VALOR FROM `+tbl+
				` WHERE SENAL_ID = ? AND FECHA > ? ORDER BY FECHA ASC`,
			senalID, after.UTC().Format(time.RFC3339))
	} else {
		rows, err = r.db.QueryContext(ctx,
			`SELECT FECHA, SYNCED_AT, SENAL_ID, VALOR FROM `+tbl+
				` WHERE SENAL_ID = :senal_id AND FECHA > :after ORDER BY FECHA ASC`,
			sql.Named("senal_id", senalID),
			sql.Named("after", after))
	}
	if err != nil {
		return nil, fmt.Errorf("ValorRepo.FindBySenalIDFrom: %w", err)
	}
	defer rows.Close()
	return r.scanValores(rows)
}

// FindBySenalID returns all values for a signal, most recent first.
func (r *ValorRepository) FindBySenalID(ctx context.Context, senalID float64) ([]models.RocValor, error) {
	tbl := valoresTable(r.db.Dialect)
	var (
		rows *sql.Rows
		err  error
	)
	if r.db.Dialect == db.DialectSQLite {
		rows, err = r.db.QueryContext(ctx,
			`SELECT FECHA, SYNCED_AT, SENAL_ID, VALOR FROM `+tbl+
				` WHERE SENAL_ID = ? ORDER BY FECHA DESC`, senalID)
	} else {
		rows, err = r.db.QueryContext(ctx,
			`SELECT FECHA, SYNCED_AT, SENAL_ID, VALOR FROM `+tbl+
				` WHERE SENAL_ID = :senal_id ORDER BY FECHA DESC`,
			sql.Named("senal_id", senalID))
	}
	if err != nil {
		return nil, fmt.Errorf("ValorRepo.FindBySenalID: %w", err)
	}
	defer rows.Close()
	return r.scanValores(rows)
}

// FindByRango returns values for a signal within an inclusive FECHA range.
func (r *ValorRepository) FindByRango(ctx context.Context, senalID float64, desde, hasta time.Time) ([]models.RocValor, error) {
	tbl := valoresTable(r.db.Dialect)
	var (
		rows *sql.Rows
		err  error
	)
	if r.db.Dialect == db.DialectSQLite {
		rows, err = r.db.QueryContext(ctx,
			`SELECT FECHA, SYNCED_AT, SENAL_ID, VALOR FROM `+tbl+
				` WHERE SENAL_ID = ? AND FECHA BETWEEN ? AND ? ORDER BY FECHA ASC`,
			senalID,
			desde.UTC().Format(time.RFC3339),
			hasta.UTC().Format(time.RFC3339))
	} else {
		rows, err = r.db.QueryContext(ctx,
			`SELECT FECHA, SYNCED_AT, SENAL_ID, VALOR FROM `+tbl+
				` WHERE SENAL_ID = :senal_id AND FECHA BETWEEN :desde AND :hasta ORDER BY FECHA ASC`,
			sql.Named("senal_id", senalID),
			sql.Named("desde", desde),
			sql.Named("hasta", hasta))
	}
	if err != nil {
		return nil, fmt.Errorf("ValorRepo.FindByRango: %w", err)
	}
	defer rows.Close()
	return r.scanValores(rows)
}

// FindUltimos returns the N most recent records across all signals.
func (r *ValorRepository) FindUltimos(ctx context.Context, n int) ([]models.RocValor, error) {
	tbl := valoresTable(r.db.Dialect)
	var (
		rows *sql.Rows
		err  error
	)
	if r.db.Dialect == db.DialectSQLite {
		rows, err = r.db.QueryContext(ctx,
			`SELECT FECHA, SYNCED_AT, SENAL_ID, VALOR FROM `+tbl+
				` ORDER BY FECHA DESC LIMIT ?`, n)
	} else {
		rows, err = r.db.QueryContext(ctx,
			`SELECT FECHA, SYNCED_AT, SENAL_ID, VALOR FROM `+tbl+
				` ORDER BY FECHA DESC FETCH FIRST :n ROWS ONLY`,
			sql.Named("n", n))
	}
	if err != nil {
		return nil, fmt.Errorf("ValorRepo.FindUltimos: %w", err)
	}
	defer rows.Close()
	return r.scanValores(rows)
}

// ── scan helpers ──────────────────────────────────────────────────────────────

// scanValores is a method so it can dispatch on dialect for time handling.
// SQLite stores times as RFC3339 text; Oracle returns native timestamps.
func (r *ValorRepository) scanValores(rows *sql.Rows) ([]models.RocValor, error) {
	var result []models.RocValor
	for rows.Next() {
		var v models.RocValor
		var valor sql.NullFloat64

		if r.db.Dialect == db.DialectSQLite {
			var fechaStr, syncedAtStr string
			if err := rows.Scan(&fechaStr, &syncedAtStr, &v.SenalID, &valor); err != nil {
				return nil, fmt.Errorf("ValorRepo scan: %w", err)
			}
			t, err := time.Parse(time.RFC3339, fechaStr)
			if err != nil {
				return nil, fmt.Errorf("ValorRepo scan FECHA %q: %w", fechaStr, err)
			}
			v.Fecha = t.Local()
			t2, err := time.Parse(time.RFC3339, syncedAtStr)
			if err != nil {
				return nil, fmt.Errorf("ValorRepo scan SYNCED_AT %q: %w", syncedAtStr, err)
			}
			v.SyncedAt = t2.Local()
		} else {
			if err := rows.Scan(&v.Fecha, &v.SyncedAt, &v.SenalID, &valor); err != nil {
				return nil, fmt.Errorf("ValorRepo scan: %w", err)
			}
		}

		if valor.Valid {
			v.Valor = &valor.Float64
		}
		result = append(result, v)
	}
	return result, rows.Err()
}
