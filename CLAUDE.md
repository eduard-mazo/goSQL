# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Overview

Background Go daemon (`roc-collector`) that polls ROC flow-meter stations via **Modbus TCP** and writes hourly measurement records to Oracle tables `HEPMGA.ROC_SENALES` (signal catalog) and `HEPMGA.ROC_VALORES` (time series). Uses [sijms/go-ora](https://github.com/sijms/go-ora) — pure Go Oracle driver, no CGO.

## Commands

```bash
make build           # compile → ./bin/roc-valores
make run             # build + run daemon (default subcommand)
make dev             # go run without producing a binary
make test            # all unit tests
make test-verbose    # tests with -v
make test-cover      # coverage report → cover.html
make check           # fmt-check + vet + test (CI suite)
make vet             # go vet
make fmt             # gofmt -w
make tidy            # go mod tidy
make clean           # remove ./bin, cover.out, cover.html
```

Run subcommands directly:
```bash
./bin/roc-valores seed   # insert missing signals into ROC_SENALES and exit
./bin/roc-valores sync   # seed + one full delta-sync cycle, then exit
./bin/roc-valores run    # seed + daemon (sync at :05 every hour) [default]
```

Run without building:
```bash
go run ./cmd/main.go seed
go run ./cmd/main.go sync
```

## Configuration

Requires a `.env` file at the project root (not committed):
```env
DB_HOST=SGIODB-scan
DB_PORT=1526
DB_SERVICE=giodb
DB_USER=...
DB_PASSWORD=...
DB_MAX_OPEN_CONNS=5
DB_MAX_IDLE_CONNS=2
DB_CONN_MAX_LIFETIME_MIN=30
```

Station definitions are loaded from `config.yaml` at runtime (not embedded).

## Architecture

```
config/      → Load .env, build Oracle DSN via go_ora.BuildUrl()
db/          → Wrap *sql.DB; HealthCheck, WithTx (commit/rollback helper)
modbus/      → Modbus TCP client (FC03 only), float32 decoders, ROC date/time decoder
collector/   → Station config types (YAML), Collector struct, delta-sync logic
models/      → RocSenal, RocValor structs + S() / F() pointer helpers
repository/  → Oracle CRUD: SenalRepository (read + seed), ValorRepository (read + upsert)
cmd/         → Entry point: seed | sync | run (daemon)
config.yaml  → 9 ROC stations (some with multiple medidores)
```

Data flow: `main` → `collector.New` (reads config.yaml) → `EnsureSignals` (seeds ROC_SENALES, caches SENAL_IDs) → `SyncAll` goroutines → Modbus TCP → `valorRepo.UpsertBatch` → Oracle.

## Oracle Schema

```sql
-- Signal catalog: one row per (station, meter, signal)
HEPMGA.ROC_SENALES: SENAL_ID, B1 (station), B2 (meter/null), B3 (signal name),
                    ELEMENT, UNIDADES, CREATED (DEFAULT SYSTIMESTAMP), ACTIVO

-- Time series: one row per (signal, hour)
HEPMGA.ROC_VALORES: FECHA (ROC device timestamp), SYNCED_AT (poll time),
                    SENAL_ID (FK), VALOR (NULLable float)
```

## Key Conventions

- **Oracle bind variables use `:name` syntax** (not `?`). Always pass `sql.Named("name", value)`.
- **NULLable columns**: `*string` / `*float64` in structs; scan via `sql.NullString` / `sql.NullFloat64`, convert with `nullStrToPtr`.
- **SYNCED_AT** = `time.Now()` at poll time (the collector has no controller-provided sync timestamp).
- **All writes use `db.WithTx()`** — automatic commit/rollback/panic-recovery.
- **`UpsertBatch`** uses Oracle MERGE ON `(SENAL_ID, FECHA)` — idempotent; re-syncing never creates duplicates.
- **`SenalID` and `VALOR` are `float64` / `*float64`** because Oracle NUMBER maps to float64 in go-ora.
- **Delta sync**: on each poll, reads `MAX(FECHA)` from Oracle for a task's first signal, then fetches only the missing circular-buffer slots (0–839) from the device.
- **Endianness**: ROC devices use `cdab` (word-swapped) by default; LLANOS uses `dcba` for its pointer register and `abcd` for data. Per-station and per-medidor overrides are in `config.yaml`.
- **Signal layout**: each 40-byte ROC record = 10 × float32. `modes[0]` = date (MMDDYY), `modes[1]` = time (HHMM), `modes[2..9]` = 8 signal values.
- **`NextID`** in SenalRepository is not concurrent-safe — call only from single-threaded `EnsureSignals`.
