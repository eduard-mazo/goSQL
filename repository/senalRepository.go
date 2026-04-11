package repository

import (
	"context"
	"database/sql"
	"fmt"

	"goSQL/db"
	"goSQL/models"
)

// FindAllMap loads every ROC_SENALES row and returns a lookup map.
// Key = "B1|B2|B3|ELEMENT" → SENAL_ID. NULL columns are treated as "".
// Used by collector.EnsureSignals for an efficient single-query bootstrap.
func (r *SenalRepository) FindAllMap(ctx context.Context) (map[string]float64, error) {
	const q = `SELECT SENAL_ID, B1, B2, B3, ELEMENT FROM HEPMGA.ROC_SENALES`
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

// FindByKeys finds a signal by its B1 (station), B2 (meter, empty if none), B3 (signal name).
// Returns nil if not found.
func (r *SenalRepository) FindByKeys(ctx context.Context, b1, b2, b3 string) (*models.RocSenal, error) {
	var row *sql.Row
	if b2 == "" {
		const q = `SELECT SENAL_ID, B1, B2, B3, ELEMENT, UNIDADES, CREATED, ACTIVO
		           FROM HEPMGA.ROC_SENALES
		           WHERE B1 = :b1 AND B2 IS NULL AND B3 = :b3`
		row = r.db.QueryRowContext(ctx, q, sql.Named("b1", b1), sql.Named("b3", b3))
	} else {
		const q = `SELECT SENAL_ID, B1, B2, B3, ELEMENT, UNIDADES, CREATED, ACTIVO
		           FROM HEPMGA.ROC_SENALES
		           WHERE B1 = :b1 AND B2 = :b2 AND B3 = :b3`
		row = r.db.QueryRowContext(ctx, q, sql.Named("b1", b1), sql.Named("b2", b2), sql.Named("b3", b3))
	}

	var s models.RocSenal
	var b1v, b2v, b3v, element, unidades sql.NullString
	err := row.Scan(&s.SenalID, &b1v, &b2v, &b3v, &element, &unidades, &s.Created, &s.Activo)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("SenalRepo.FindByKeys: %w", err)
	}
	s.B1 = nullStrToPtr(b1v)
	s.B2 = nullStrToPtr(b2v)
	s.B3 = nullStrToPtr(b3v)
	s.Element = nullStrToPtr(element)
	s.Unidades = nullStrToPtr(unidades)
	return &s, nil
}

// NextID returns NVL(MAX(SENAL_ID), 0) + 1 from ROC_SENALES.
// Not safe for concurrent callers — use only during single-threaded seeding.
func (r *SenalRepository) NextID(ctx context.Context) (float64, error) {
	var next float64
	err := r.db.QueryRowContext(ctx,
		`SELECT NVL(MAX(SENAL_ID), 0) + 1 FROM HEPMGA.ROC_SENALES`,
	).Scan(&next)
	if err != nil {
		return 0, fmt.Errorf("SenalRepo.NextID: %w", err)
	}
	return next, nil
}

// Insert inserts a new signal row with an explicit SENAL_ID.
// CREATED is set by Oracle's DEFAULT SYSTIMESTAMP.
func (r *SenalRepository) Insert(ctx context.Context, s models.RocSenal) error {
	const q = `INSERT INTO HEPMGA.ROC_SENALES (SENAL_ID, B1, B2, B3, ELEMENT, UNIDADES, ACTIVO)
	           VALUES (:senal_id, :b1, :b2, :b3, :element, :unidades, :activo)`
	_, err := r.db.ExecContext(ctx, q,
		sql.Named("senal_id", s.SenalID),
		sql.Named("b1", s.B1),
		sql.Named("b2", s.B2),
		sql.Named("b3", s.B3),
		sql.Named("element", s.Element),
		sql.Named("unidades", s.Unidades),
		sql.Named("activo", s.Activo),
	)
	if err != nil {
		return fmt.Errorf("SenalRepo.Insert: %w", err)
	}
	return nil
}

// SenalRepository opera sobre HEPMGA.ROC_SENALES (catálogo — solo lectura).
type SenalRepository struct {
	db *db.DB
}

func NewSenalRepository(database *db.DB) *SenalRepository {
	return &SenalRepository{db: database}
}

// FindAll devuelve todas las señales activas ordenadas por SENAL_ID.
func (r *SenalRepository) FindAll(ctx context.Context) ([]models.RocSenal, error) {
	const q = `
		SELECT SENAL_ID, B1, B2, B3, ELEMENT, UNIDADES, CREATED, ACTIVO
		FROM   HEPMGA.ROC_SENALES
		ORDER  BY SENAL_ID ASC`

	rows, err := r.db.QueryContext(ctx, q)
	if err != nil {
		return nil, fmt.Errorf("SenalRepo.FindAll: %w", err)
	}
	defer rows.Close()

	return scanSenales(rows)
}

// FindActivas devuelve solo las señales con ACTIVO = 'S'.
func (r *SenalRepository) FindActivas(ctx context.Context) ([]models.RocSenal, error) {
	const q = `
		SELECT SENAL_ID, B1, B2, B3, ELEMENT, UNIDADES, CREATED, ACTIVO
		FROM   HEPMGA.ROC_SENALES
		WHERE  ACTIVO = 'S'
		ORDER  BY SENAL_ID ASC`

	rows, err := r.db.QueryContext(ctx, q)
	if err != nil {
		return nil, fmt.Errorf("SenalRepo.FindActivas: %w", err)
	}
	defer rows.Close()

	return scanSenales(rows)
}

// FindByID devuelve una señal por su SENAL_ID. Retorna nil si no existe.
func (r *SenalRepository) FindByID(ctx context.Context, senalID float64) (*models.RocSenal, error) {
	const q = `
		SELECT SENAL_ID, B1, B2, B3, ELEMENT, UNIDADES, CREATED, ACTIVO
		FROM   HEPMGA.ROC_SENALES
		WHERE  SENAL_ID = :senal_id`

	row := r.db.QueryRowContext(ctx, q, sql.Named("senal_id", senalID))

	var s models.RocSenal
	var b1, b2, b3, element, unidades sql.NullString

	err := row.Scan(&s.SenalID, &b1, &b2, &b3, &element, &unidades, &s.Created, &s.Activo)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("SenalRepo.FindByID: %w", err)
	}

	s.B1 = nullStrToPtr(b1)
	s.B2 = nullStrToPtr(b2)
	s.B3 = nullStrToPtr(b3)
	s.Element = nullStrToPtr(element)
	s.Unidades = nullStrToPtr(unidades)

	return &s, nil
}

// ── helpers ──────────────────────────────────────────────────────────────────

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

func nullStrToPtr(n sql.NullString) *string {
	if !n.Valid {
		return nil
	}
	return &n.String
}