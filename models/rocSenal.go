package models

import "time"

// RocSenal representa una fila de HEPMGA.ROC_SENALES.
//
//	SENAL_ID  NUMBER
//	B1        VARCHAR2(50)
//	B2        VARCHAR2(50)
//	B3        VARCHAR2(50)
//	ELEMENT   VARCHAR2(50)
//	UNIDADES  VARCHAR2(50)
//	CREATED   TIMESTAMP(6)   DEFAULT SYSTIMESTAMP
//	ACTIVO    VARCHAR2(1)
type RocSenal struct {
	SenalID  float64   // NUMBER — identificador de la señal
	B1       *string   // VARCHAR2 NULLable
	B2       *string
	B3       *string
	Element  *string
	Unidades *string
	Created  time.Time // TIMESTAMP(6), default SYSTIMESTAMP
	Activo   string    // 'S' / 'N'  (VARCHAR2 1 BYTE)
}

// EstaActivo devuelve true si ACTIVO = 'S'.
func (s RocSenal) EstaActivo() bool { return s.Activo == "S" }

// S convierte un literal string en *string — helper de conveniencia.
func S(v string) *string { return &v }