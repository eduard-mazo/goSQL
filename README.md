# roc-valores

Servicio Go para leer y escribir sobre `HEPMGA.ROC_SENALES` y `HEPMGA.ROC_VALORES` usando [**go-ora**](https://github.com/sijms/go-ora) — driver Oracle puro Go, sin CGO ni Oracle Instant Client.

---

## Prerrequisitos

- Go 1.21+
- Acceso de red al host Oracle (`SGIODB-scan:1526`)
- Usuario con permisos de `SELECT` sobre `ROC_SENALES` e `INSERT/SELECT` sobre `ROC_VALORES`

---

## Configuración

Editar el archivo `.env` en la raíz del proyecto:

```env
DB_HOST=SGIODB-scan
DB_PORT=1526
DB_SERVICE=giodb
DB_USER=tu_usuario
DB_PASSWORD=tu_contraseña

DB_MAX_OPEN_CONNS=5
DB_MAX_IDLE_CONNS=2
DB_CONN_MAX_LIFETIME_MIN=30
```

La cadena de conexión (DSN) se construye automáticamente mediante `go_ora.BuildUrl()` con `SERVER=DEDICATED`, compatible con entornos Oracle RAC / SCAN.

---

El binario resultante **no requiere Oracle Instant Client** instalado en el equipo destino.

---

## Estructura del proyecto

```
roc-valores/
├── cmd/
│   └── main.go                        # Punto de entrada + demos CLI
├── config/
│   └── config.go                      # Carga .env y construye el DSN
├── db/
│   └── db.go                          # Pool de conexiones, HealthCheck, WithTx
├── models/
│   ├── roc_senal.go                   # Struct RocSenal + helper S()
│   └── roc_valor.go                   # Struct RocValor + helper F()
├── repository/
│   ├── senal_repository.go            # Lectura de ROC_SENALES
│   └── valor_repository.go            # Lectura y escritura de ROC_VALORES
├── .env                               # Variables de entorno (no subir a git)
├── .gitignore
├── go.mod
└── Makefile
```

---

## Esquema de tablas

```sql
-- Catálogo de señales (solo lectura desde la aplicación)
CREATE TABLE HEPMGA.ROC_SENALES (
    SENAL_ID  NUMBER,
    B1        VARCHAR2(50 BYTE),        -- NULLable
    B2        VARCHAR2(50 BYTE),        -- NULLable
    B3        VARCHAR2(50 BYTE),        -- NULLable
    ELEMENT   VARCHAR2(50 BYTE),        -- NULLable
    UNIDADES  VARCHAR2(50 BYTE),        -- NULLable
    CREATED   TIMESTAMP(6) DEFAULT SYSTIMESTAMP,
    ACTIVO    VARCHAR2(1 BYTE)          -- 'S' activa / 'N' inactiva
);

-- Serie de tiempo de mediciones
CREATE TABLE HEPMGA.ROC_VALORES (
    FECHA      TIMESTAMP(6),            -- momento de la medición en campo
    SYNCED_AT  TIMESTAMP(6),            -- momento en que el controlador disparó el envío
    SENAL_ID   NUMBER,                  -- FK a ROC_SENALES
    VALOR      NUMBER                   -- NULLable
);
```

> **Nota sobre `SYNCED_AT`:** este campo **no** se genera con `SYSDATE` ni con `time.Now()` en la aplicación. Es capturado directamente del controlador de campo y se persiste exactamente como llega, preservando la trazabilidad del origen del dato.

---

## API del repositorio

### `SenalRepository` — `HEPMGA.ROC_SENALES` (lectura)

| Método                   | Descripción                                             |
| ------------------------ | ------------------------------------------------------- |
| `FindAll(ctx)`           | Todas las señales, ordenadas por `SENAL_ID`             |
| `FindActivas(ctx)`       | Solo señales con `ACTIVO = 'S'`                         |
| `FindByID(ctx, senalID)` | Una señal por su `SENAL_ID`; retorna `nil` si no existe |

### `ValorRepository` — `HEPMGA.ROC_VALORES` (lectura y escritura)

| Método                                    | Descripción                                                  |
| ----------------------------------------- | ------------------------------------------------------------ |
| `FindBySenalID(ctx, senalID)`             | Todos los valores de una señal, más recientes primero        |
| `FindByRango(ctx, senalID, desde, hasta)` | Valores de una señal en rango de `FECHA` (inclusive)         |
| `FindUltimos(ctx, n)`                     | Los N registros más recientes de todas las señales           |
| `Insert(ctx, v)`                          | Inserta un valor; `SYNCED_AT` viene del controlador tal cual |
| `InsertBatch(ctx, []v)`                   | Inserta múltiples valores en una sola transacción            |

---

## Modelos

### `models.RocSenal`

```go
type RocSenal struct {
    SenalID  float64    // NUMBER
    B1       *string    // VARCHAR2 NULLable
    B2       *string
    B3       *string
    Element  *string
    Unidades *string
    Created  time.Time  // TIMESTAMP(6)
    Activo   string     // 'S' / 'N'
}

func (s RocSenal) EstaActivo() bool  // true si ACTIVO = 'S'
func S(v string) *string             // helper: literal → *string
```

### `models.RocValor`

```go
type RocValor struct {
    Fecha    time.Time  // TIMESTAMP(6) — medición en campo
    SyncedAt time.Time  // TIMESTAMP(6) — disparo del controlador
    SenalID  float64    // NUMBER — FK a ROC_SENALES
    Valor    *float64   // NUMBER NULLable
}

func F(v float64) *float64  // helper: literal → *float64
```

---

## Ejemplo de inserción

```go
valorRepo := repository.NewValorRepository(database)

v := models.RocValor{
    Fecha:    timestampMedicion,    // cuando ocurrió la medición
    SyncedAt: timestampControlador, // cuando el controlador disparó — viene del campo
    SenalID:  1,
    Valor:    models.F(42.75),
}

if err := valorRepo.Insert(ctx, v); err != nil {
    log.Fatal(err)
}
```

Para lotes:

```go
lote := []models.RocValor{
    {Fecha: t0, SyncedAt: syncT0, SenalID: 1, Valor: models.F(10.1)},
    {Fecha: t0, SyncedAt: syncT1, SenalID: 2, Valor: models.F(20.2)},
    {Fecha: t0, SyncedAt: syncT2, SenalID: 3, Valor: nil}, // VALOR NULL
}

if err := valorRepo.InsertBatch(ctx, lote); err != nil {
    log.Fatal(err)
}
```

---

## Decisiones técnicas

| Aspecto         | Decisión                       | Razón                                         |
| --------------- | ------------------------------ | --------------------------------------------- |
| Driver Oracle   | `sijms/go-ora`                 | Go puro, sin CGO, binario standalone          |
| Bind variables  | `:nombre` (no `?`)             | Sintaxis requerida por Oracle                 |
| NULLs numéricos | `*float64` + `sql.NullFloat64` | Evita panic al escanear NULL                  |
| NULLs de texto  | `*string` + `sql.NullString`   | Idem para VARCHAR2 NULLable                   |
| `SYNCED_AT`     | Valor del controlador          | Trazabilidad exacta del origen del dato       |
| Transacciones   | `db.WithTx()`                  | Commit/rollback automático, seguro ante panic |
| Aislamiento     | `READ COMMITTED`               | Nivel estándar Oracle; evita lecturas sucias  |
