package repository

import (
	"context"
	"database/sql"
	"fmt"

	"goSQL/db"
	"goSQL/models"
)

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