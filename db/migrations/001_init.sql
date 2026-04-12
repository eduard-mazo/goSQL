-- Migration 001: initial SQLite schema
-- Applied only when the daemon runs with --sqlite.
-- Oracle schema (HEPMGA.ROC_SENALES / HEPMGA.ROC_VALORES) is managed externally.

CREATE TABLE IF NOT EXISTS ROC_SENALES (
    SENAL_ID REAL    NOT NULL PRIMARY KEY,
    B1       TEXT,
    B2       TEXT,
    B3       TEXT,
    ELEMENT  TEXT,
    UNIDADES TEXT,
    CREATED  TEXT    DEFAULT (STRFTIME('%Y-%m-%dT%H:%M:%fZ', 'now')),
    ACTIVO   TEXT    NOT NULL DEFAULT 'S'
);

CREATE TABLE IF NOT EXISTS ROC_VALORES (
    FECHA      TEXT NOT NULL,
    SYNCED_AT  TEXT,
    SENAL_ID   REAL NOT NULL,
    VALOR      REAL,
    UNIQUE (SENAL_ID, FECHA)
);
