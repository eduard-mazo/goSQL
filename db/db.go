package db

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"time"

	"goSQL/config"

	_ "github.com/sijms/go-ora/v2" // registers "oracle" driver
	_ "modernc.org/sqlite"         // registers "sqlite" driver (pure Go, no CGO)
)

// Dialect identifies which database backend is in use.
type Dialect int

const (
	DialectOracle Dialect = iota
	DialectSQLite
)

// DB wraps *sql.DB and carries the active dialect so repositories can
// emit the correct SQL variant without needing separate implementations.
type DB struct {
	*sql.DB
	Dialect Dialect
}

// New opens an Oracle connection using the supplied DBConfig.
func New(cfg *config.DBConfig) (*DB, error) {
	log.Printf("[db] conectando → %s:%d/%s (go-ora, sin CGO)",
		cfg.Host, cfg.Port, cfg.Service)

	sqlDB, err := sql.Open("oracle", cfg.DSN)
	if err != nil {
		return nil, fmt.Errorf("sql.Open: %w", err)
	}

	sqlDB.SetMaxOpenConns(cfg.MaxOpenConns)
	sqlDB.SetMaxIdleConns(cfg.MaxIdleConns)
	sqlDB.SetConnMaxLifetime(cfg.ConnMaxLifetime)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := sqlDB.PingContext(ctx); err != nil {
		_ = sqlDB.Close()
		return nil, fmt.Errorf("ping fallido: %w", err)
	}

	log.Println("[db] ✓ conexión establecida con Oracle")
	return &DB{sqlDB, DialectOracle}, nil
}

// NewSQLite opens (or creates) a SQLite database at path, runs pending
// migrations, and returns a DB with DialectSQLite.
func NewSQLite(path string) (*DB, error) {
	log.Printf("[db] abriendo SQLite → %s", path)

	// WAL mode for concurrent reads; foreign keys on.
	dsn := fmt.Sprintf("file:%s?_pragma=journal_mode(WAL)&_pragma=foreign_keys(1)", path)
	sqlDB, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("sql.Open sqlite: %w", err)
	}
	// A single writer connection avoids SQLITE_BUSY under WAL.
	sqlDB.SetMaxOpenConns(1)

	if err := runMigrations(sqlDB); err != nil {
		_ = sqlDB.Close()
		return nil, fmt.Errorf("sqlite migrations: %w", err)
	}

	log.Println("[db] ✓ SQLite abierto y migrado")
	return &DB{sqlDB, DialectSQLite}, nil
}

// HealthCheck verifies connectivity.
func (d *DB) HealthCheck(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	var n int
	if d.Dialect == DialectSQLite {
		return d.QueryRowContext(ctx, "SELECT 1").Scan(&n)
	}
	return d.QueryRowContext(ctx, "SELECT 1 FROM DUAL").Scan(&n)
}

// WithTx executes fn inside a transaction with automatic commit/rollback.
func (d *DB) WithTx(ctx context.Context, fn func(*sql.Tx) error) (err error) {
	tx, err := d.BeginTx(ctx, nil) // nil = driver default; go-ora rejects non-default isolation
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer func() {
		if p := recover(); p != nil {
			_ = tx.Rollback()
			panic(p)
		}
		if err != nil {
			_ = tx.Rollback()
		}
	}()
	if err = fn(tx); err != nil {
		return err
	}
	return tx.Commit()
}
