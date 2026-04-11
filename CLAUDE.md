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

## Signal identity — unique key and SENAL_ID strategy

Every signal in `ROC_SENALES` is uniquely identified by the composite key **`B1|B2|B3|ELEMENT`**:

| Column  | Meaning                         | Example          |
|---------|---------------------------------|------------------|
| B1      | Station code                    | `GTASAJER`       |
| B2      | System / subsystem              | `MEDICION`       |
| B3      | Element group (brazo, punto…)   | `BRAZO1`         |
| ELEMENT | Signal code                     | `PPH`            |

`EnsureSignals` runs at startup (single-threaded):
1. **One SELECT** — loads all existing rows into `map["B1|B2|B3|ELEMENT"]→SENAL_ID`.
2. **One MAX query** — seeds a local counter for new IDs.
3. Iterates config signals: cache hit → reuse ID; cache miss → INSERT + increment counter.
4. Populates `signalIDs["taskKey:flotante"]→SENAL_ID` for use during sync.

`NextID` uses `SELECT NVL(MAX(SENAL_ID),0)+1` — safe because seeding is single-threaded. If a proper Oracle sequence is ever added, only `NextID` and `Insert` need changing.

## Signal layout in the Modbus record

Each 40-byte ROC hourly record = 10 × float32 (indexed 1–10):

| Index (flotante) | Content                       |
|------------------|-------------------------------|
| 1                | Date float (MMDDYY) — internal |
| 2                | Time float (HHMM) — internal   |
| 3–10             | Measurement signals            |

`SignalConfig.Flotante` declares which position each signal occupies. In code: `modes[flotante-1].Pick(dbEndian)`.

## config.yaml structure

Every station always has at least one `medidor`. Each `medidor` carries its own `signals` list. Signal entries are self-describing (all Oracle keys are explicit per signal):

```yaml
stations:
  - name: "GLAFELIS"          # used as task key prefix
    ip: "10.155.249.193"
    port: 502
    id: 1                     # Modbus UnitID
    ptr_endian: "cdab"
    db_endian:  "cdab"
    data_registers_count: 2   # 1=uint16 ptr, 2=float32 ptr
    medidores:
      - label: 1
        name: "M1"            # task key = "GLAFELIS / M1"
        pointer_address: 10000
        base_data_address: 700
        signals:
          - { flotante: 3, b1: "GLAFELIS", b2: "RINT", b3: "BRAZO1",
              element: "PPH", descripcion: "Presión estática", unidades: "un" }
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
- **Signal extraction**: `modes[sig.Flotante - 1].Pick(dbEndian)` — `flotante` is 1-based, modes are 0-based.
- **`FindAllMap`** in SenalRepository does a single full SELECT to bootstrap `EnsureSignals`; `NextID` and `Insert` are only called during seeding (single-threaded).
