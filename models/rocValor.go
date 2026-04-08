package models

import "time"

// RocValor representa una fila de HEPMGA.ROC_VALORES.
//
//	FECHA      TIMESTAMP(6)   — momento del dato en el campo
//	SYNCED_AT  TIMESTAMP(6)   — momento en que el controlador disparó el envío
//	SENAL_ID   NUMBER         — FK a ROC_SENALES
//	VALOR      NUMBER         — medición
//
// IMPORTANTE: SYNCED_AT NO se genera en el servidor Oracle ni en la aplicación Go.
// Es capturado directamente del controlador de campo y se almacena tal cual.
type RocValor struct {
	Fecha    time.Time // timestamp de la medición en campo
	SyncedAt time.Time // timestamp del disparo del controlador (viene del campo)
	SenalID  float64   // FK a ROC_SENALES
	Valor    *float64  // NUMBER NULLable
}

// F convierte un literal float64 en *float64 — helper de conveniencia.
func F(v float64) *float64 { return &v }