package repository

import (
	"context"
	"database/sql"
	"fmt"

	"goSQL/db"
	"goSQL/models"
)

// SenalRepository opera sobre ROC_SENALES (catálogo — seed + lectura).
type SenalRepository struct {
	db *db.DB
}

func NewSenalRepository(database *db.DB) *SenalRepository {
	return &SenalRepository{db: database}
}

// ── helpers ──────────────────────────────────────────────────────────────────

func senalesTable(d db.Dialect) string {
	if d == db.DialectSQLite {
		return "ROC_SENALES"
	}
	return "HEPMGA.ROC_SENALES"
}

func nullStrToPtr(n sql.NullString) *string {
	if !n.Valid {
		return nil
	}
	return &n.String
}

// ── bootstrap (EnsureSignals) ─────────────────────────────────────────────────

// FindAllMap loads every ROC_SENALES row and returns a lookup map.
// Key = "B1|B2|B3|ELEMENT" → SENAL_ID. NULL columns are treated as "".
func (r *SenalRepository) FindAllMap(ctx context.Context) (map[string]float64, error) {
	q := `SELECT SENAL_ID, B1, B2, B3, ELEMENT FROM ` + senalesTable(r.db.Dialect)
	rows, err := r.db.QueryContext(ctx, q)
	if err != nil {
		return nil, fmt.Errorf("SenalRepo.FindAllMap: %w", err)
	}
	defer rows.Close()

	m := make(map[string]float64)
	for rows.Next() {
		var senalID float64
		var b1, b2, b3, element sql.NullString
		if err := rows.Scan(&senalID, &b1, &b2, &b3, &element); err != nil {
			return nil, fmt.Errorf("SenalRepo.FindAllMap scan: %w", err)
		}
		key := fmt.Sprintf("%s|%s|%s|%s", b1.String, b2.String, b3.String, element.String)
		m[key] = senalID
	}
	return m, rows.Err()
}

// NextID returns MAX(SENAL_ID) + 1 (or 1 when the table is empty).
// Not safe for concurrent callers — use only during single-threaded seeding.
func (r *SenalRepository) NextID(ctx context.Context) (float64, error) {
	var q string
	if r.db.Dialect == db.DialectSQLite {
		q = `SELECT COALESCE(MAX(SENAL_ID), 0) + 1 FROM ROC_SENALES`
	} else {
		q = `SELECT NVL(MAX(SENAL_ID), 0) + 1 FROM HEPMGA.ROC_SENALES`
	}
	var next float64
	if err := r.db.QueryRowContext(ctx, q).Scan(&next); err != nil {
		return 0, fmt.Errorf("SenalRepo.NextID: %w", err)
	}
	return next, nil
}

// Insert inserts a new signal row with an explicit SENAL_ID.
// CREATED is set by the DEFAULT expression on both Oracle and SQLite.
func (r *SenalRepository) Insert(ctx context.Context, s models.RocSenal) error {
	if r.db.Dialect == db.DialectSQLite {
		const q = `INSERT INTO ROC_SENALES (SENAL_ID, B1, B2, B3, ELEMENT, UNIDADES, ACTIVO)
		           VALUES (?, ?, ?, ?, ?, ?, ?)`
		if _, err := r.db.ExecContext(ctx, q,
			s.SenalID, s.B1, s.B2, s.B3, s.Element, s.Unidades, s.Activo,
		); err != nil {
			return fmt.Errorf("SenalRepo.Insert: %w", err)
		}
		return nil
	}
	const q = `INSERT INTO HEPMGA.ROC_SENALES (SENAL_ID, B1, B2, B3, ELEMENT, UNIDADES, ACTIVO)
	           VALUES (:senal_id, :b1, :b2, :b3, :element, :unidades, :activo)`
	if _, err := r.db.ExecContext(ctx, q,
		sql.Named("senal_id", s.SenalID),
		sql.Named("b1", s.B1),
		sql.Named("b2", s.B2),
		sql.Named("b3", s.B3),
		sql.Named("element", s.Element),
		sql.Named("unidades", s.Unidades),
		sql.Named("activo", s.Activo),
	); err != nil {
		return fmt.Errorf("SenalRepo.Insert: %w", err)
	}
	return nil
}

// ── reads ─────────────────────────────────────────────────────────────────────

// FindAll returns all signals ordered by SENAL_ID.
func (r *SenalRepository) FindAll(ctx context.Context) ([]models.RocSenal, error) {
	q := `SELECT SENAL_ID, B1, B2, B3, ELEMENT, UNIDADES, CREATED, ACTIVO
	      FROM ` + senalesTable(r.db.Dialect) + `
	      ORDER BY SENAL_ID ASC`
	rows, err := r.db.QueryContext(ctx, q)
	if err != nil {
		return nil, fmt.Errorf("SenalRepo.FindAll: %w", err)
	}
	defer rows.Close()
	return scanSenales(rows)
}

// FindActivas returns only signals where ACTIVO = 'S'.
func (r *SenalRepository) FindActivas(ctx context.Context) ([]models.RocSenal, error) {
	q := `SELECT SENAL_ID, B1, B2, B3, ELEMENT, UNIDADES, CREATED, ACTIVO
	      FROM ` + senalesTable(r.db.Dialect) + `
	      WHERE ACTIVO = 'S'
	      ORDER BY SENAL_ID ASC`
	rows, err := r.db.QueryContext(ctx, q)
	if err != nil {
		return nil, fmt.Errorf("SenalRepo.FindActivas: %w", err)
	}
	defer rows.Close()
	return scanSenales(rows)
}

// FindByID returns a signal by its SENAL_ID, or nil if not found.
func (r *SenalRepository) FindByID(ctx context.Context, senalID float64) (*models.RocSenal, error) {
	var row *sql.Row
	if r.db.Dialect == db.DialectSQLite {
		row = r.db.QueryRowContext(ctx,
			`SELECT SENAL_ID, B1, B2, B3, ELEMENT, UNIDADES, CREATED, ACTIVO
			 FROM ROC_SENALES WHERE SENAL_ID = ?`, senalID)
	} else {
		row = r.db.QueryRowContext(ctx,
			`SELECT SENAL_ID, B1, B2, B3, ELEMENT, UNIDADES, CREATED, ACTIVO
			 FROM HEPMGA.ROC_SENALES WHERE SENAL_ID = :senal_id`,
			sql.Named("senal_id", senalID))
	}
	return scanOneSenal(row)
}

// FindByKeys finds a signal by B1, B2, B3. Returns nil if not found.
func (r *SenalRepository) FindByKeys(ctx context.Context, b1, b2, b3 string) (*models.RocSenal, error) {
	tbl := senalesTable(r.db.Dialect)
	var row *sql.Row

	if r.db.Dialect == db.DialectSQLite {
		if b2 == "" {
			row = r.db.QueryRowContext(ctx,
				`SELECT SENAL_ID, B1, B2, B3, ELEMENT, UNIDADES, CREATED, ACTIVO
				 FROM `+tbl+` WHERE B1 = ? AND B2 IS NULL AND B3 = ?`, b1, b3)
		} else {
			row = r.db.QueryRowContext(ctx,
				`SELECT SENAL_ID, B1, B2, B3, ELEMENT, UNIDADES, CREATED, ACTIVO
				 FROM `+tbl+` WHERE B1 = ? AND B2 = ? AND B3 = ?`, b1, b2, b3)
		}
	} else {
		if b2 == "" {
			row = r.db.QueryRowContext(ctx,
				`SELECT SENAL_ID, B1, B2, B3, ELEMENT, UNIDADES, CREATED, ACTIVO
				 FROM `+tbl+` WHERE B1 = :b1 AND B2 IS NULL AND B3 = :b3`,
				sql.Named("b1", b1), sql.Named("b3", b3))
		} else {
			row = r.db.QueryRowContext(ctx,
				`SELECT SENAL_ID, B1, B2, B3, ELEMENT, UNIDADES, CREATED, ACTIVO
				 FROM `+tbl+` WHERE B1 = :b1 AND B2 = :b2 AND B3 = :b3`,
				sql.Named("b1", b1), sql.Named("b2", b2), sql.Named("b3", b3))
		}
	}
	return scanOneSenal(row)
}

// ── scan helpers ──────────────────────────────────────────────────────────────

func scanOneSenal(row *sql.Row) (*models.RocSenal, error) {
	var s models.RocSenal
	var b1, b2, b3, element, unidades sql.NullString
	err := row.Scan(&s.SenalID, &b1, &b2, &b3, &element, &unidades, &s.Created, &s.Activo)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("SenalRepo scan: %w", err)
	}
	s.B1 = nullStrToPtr(b1)
	s.B2 = nullStrToPtr(b2)
	s.B3 = nullStrToPtr(b3)
	s.Element = nullStrToPtr(element)
	s.Unidades = nullStrToPtr(unidades)
	return &s, nil
}

func scanSenales(rows *sql.Rows) ([]models.RocSenal, error) {
	var result []models.RocSenal
	for rows.Next() {
		var s models.RocSenal
		var b1, b2, b3, element, unidades sql.NullString
		if err := rows.Scan(
			&s.SenalID, &b1, &b2, &b3,
			&element, &unidades, &s.Created, &s.Activo,
		); err != nil {
			return nil, fmt.Errorf("SenalRepo scan: %w", err)
		}
		s.B1 = nullStrToPtr(b1)
		s.B2 = nullStrToPtr(b2)
		s.B3 = nullStrToPtr(b3)
		s.Element = nullStrToPtr(element)
		s.Unidades = nullStrToPtr(unidades)
		result = append(result, s)
	}
	return result, rows.Err()
}
