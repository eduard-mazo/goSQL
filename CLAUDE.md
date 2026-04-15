# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Overview

Background Go daemon (`roc-collector`) that polls ROC flow-meter stations via **Modbus TCP** and writes hourly measurement records to `ROC_SENALES` (signal catalog) and `ROC_VALORES` (time series). Supports two backends — Oracle (production) and SQLite (fallback/offline). Both drivers are pure Go, no CGO: [sijms/go-ora](https://github.com/sijms/go-ora) for Oracle, [modernc.org/sqlite](https://pkg.go.dev/modernc.org/sqlite) for SQLite.

## Commands

```bash
make build           # compile → ./bin/roc-valores
make run             # build + daemon (Oracle)
make seed            # seed Oracle ROC_SENALES and exit
make sync            # seed + one delta-sync cycle (Oracle) and exit
make run-sqlite      # daemon with SQLite (SQLITE_DB=./roc.db)
make seed-sqlite     # seed SQLite and exit
make sync-sqlite     # seed + one delta-sync cycle (SQLite) and exit
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
# Oracle (requires .env)
./bin/roc-valores seed
./bin/roc-valores sync
./bin/roc-valores run        # default

# SQLite (no .env needed — creates DB file on first run)
./bin/roc-valores --sqlite ./roc.db seed
./bin/roc-valores --sqlite ./roc.db sync
./bin/roc-valores --sqlite ./roc.db run

# SQLite → Oracle transfer (requires both --sqlite and .env)
./bin/roc-valores --sqlite ./roc.db push       # forward: FECHA > Oracle MAX
./bin/roc-valores --sqlite ./roc.db backfill   # backward: FECHA < Oracle MIN

# Dashboard web server
./bin/roc-valores --sqlite ./roc.db serve          # SQLite backend, http://localhost:8080
./bin/roc-valores --sqlite ./roc.db serve :3000     # custom port
./bin/roc-valores serve                             # Oracle backend
```

### CLI subcommands reference

| Command    | Backend needed     | Description                                                       |
|------------|--------------------|-------------------------------------------------------------------|
| `seed`     | Oracle or SQLite   | Populate `ROC_SENALES` from `config.yaml` and exit                |
| `sync`     | Oracle or SQLite   | Seed + one delta-sync cycle (poll devices via Modbus) and exit    |
| `run`      | Oracle or SQLite   | Daemon mode: seed, sync now, then re-sync every hour at :05       |
| `push`     | `--sqlite` + `.env`| **Forward sync** — send SQLite rows with `FECHA > Oracle MAX(FECHA)` to Oracle. Covers new data collected since last push. |
| `backfill` | `--sqlite` + `.env`| **Backward sync** — send SQLite rows with `FECHA < Oracle MIN(FECHA)` to Oracle. Covers historical data migrated into SQLite from older sources (e.g. `modbus.db`). |
| `serve`    | Oracle or SQLite   | **Dashboard** — starts HTTP server with REST API + web frontend on `:8080` (default). Optional port arg: `serve :3000`. |

Both `push` and `backfill` are idempotent (MERGE / ON CONFLICT DO NOTHING) and safe to re-run.

Run without building:
```bash
go run ./cmd/main.go seed
go run ./cmd/main.go --sqlite ./roc.db sync
go run ./cmd/main.go --sqlite ./roc.db push
go run ./cmd/main.go --sqlite ./roc.db backfill
go run ./cmd/main.go --sqlite ./roc.db serve
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
config/          → Load .env, build Oracle DSN via go_ora.BuildUrl()
db/              → DB struct (wraps *sql.DB + Dialect); New (Oracle), NewSQLite, HealthCheck, WithTx
db/migrate.go    → SQLite-only migration runner (go:embed + schema_migrations tracking table)
db/migrations/   → Numbered *.sql files applied in order on SQLite startup
modbus/          → Modbus TCP client (FC03 only), float32 decoders, ROC date/time decoder
collector/       → Station config types (YAML), Collector struct, delta-sync logic
models/          → RocSenal, RocValor structs + S() / F() pointer helpers
repository/      → Dialect-aware CRUD: SenalRepository, ValorRepository (Oracle + SQLite)
api/             → REST/JSON HTTP server (read-only): stations, signals, values, overview, stats
web/             → Static frontend dashboard (HTML/CSS/JS, Chart.js) served by api.Server
cmd/             → Entry point: [--sqlite PATH] seed | sync | push | backfill | serve | run
config.yaml      → 9 ROC stations (some with multiple medidores)
```

Data flow: `main` → `collector.New` (reads config.yaml) → `EnsureSignals` (seeds ROC_SENALES, caches SENAL_IDs) → `SyncAll` goroutines → Modbus TCP → `valorRepo.UpsertBatch` → Oracle **or SQLite**.

## Schema

Both backends use the same logical tables. Oracle prefixes them with `HEPMGA.`.

```sql
-- Signal catalog: one row per (station, meter, signal)
ROC_SENALES: SENAL_ID, B1 (station), B2 (meter/null), B3 (signal name),
             ELEMENT, UNIDADES, CREATED (DEFAULT SYSTIMESTAMP / STRFTIME), ACTIVO

-- Time series: one row per (signal, hour)
ROC_VALORES: FECHA (ROC device timestamp), SYNCED_AT (poll time),
             SENAL_ID (FK), VALOR (NULLable float)
```

Oracle schema is managed externally by a DBA. SQLite schema is created automatically via `db/migrations/` on first run.

**SQLite-specific notes:**
- `NUMBER` → `REAL`, `TIMESTAMP(6)` → `TEXT` (RFC3339 UTC strings)
- `UNIQUE(SENAL_ID, FECHA)` on `ROC_VALORES` enables `ON CONFLICT DO NOTHING` upserts
- `MaxOpenConns` is forced to 1 (WAL mode, one writer)

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

- **Oracle bind variables use `:name` syntax** (not `?`). Always pass `sql.Named("name", value)`. SQLite uses positional `?`.
- **Dialect branching**: check `r.db.Dialect == db.DialectSQLite` in repositories. Helpers `senalesTable(d)` and `valoresTable(d)` return the correct table name per dialect.
- **NULLable columns**: `*string` / `*float64` in structs; scan via `sql.NullString` / `sql.NullFloat64`, convert with `nullStrToPtr`.
- **SYNCED_AT** = `time.Now()` at poll time (the collector has no controller-provided sync timestamp).
- **All writes use `db.WithTx()`** — automatic commit/rollback/panic-recovery.
- **`UpsertBatch`** uses Oracle MERGE ON `(SENAL_ID, FECHA)` or SQLite `INSERT ... ON CONFLICT(SENAL_ID, FECHA) DO NOTHING` — idempotent on both backends.
- **`SenalID` and `VALOR` are `float64` / `*float64`** because Oracle NUMBER maps to float64 in go-ora (SQLite REAL also maps to float64).
- **Delta sync**: on each poll, reads `MAX(FECHA)` for a task's first signal, then fetches only the missing circular-buffer slots (0–839) from the device.
- **Endianness**: ROC devices use `cdab` (word-swapped) by default; LLANOS uses `dcba` for its pointer register and `abcd` for data. Per-station and per-medidor overrides are in `config.yaml`.
- **Signal extraction**: `modes[sig.Flotante - 1].Pick(dbEndian)` — `flotante` is 1-based, modes are 0-based.
- **`FindAllMap`** in SenalRepository does a single full SELECT to bootstrap `EnsureSignals`; `NextID` and `Insert` are only called during seeding (single-threaded).
- **SQLite migrations**: adding a new `db/migrations/00N_name.sql` file is enough — the runner applies it automatically at next startup and records it in `schema_migrations`.
