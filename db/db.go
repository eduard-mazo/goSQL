package db

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"time"

	"goSQL/config"

	_ "github.com/sijms/go-ora/v2" // registra el driver "oracle"
)

type DB struct {
	*sql.DB
}

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
	return &DB{sqlDB}, nil
}

// HealthCheck lanza un SELECT 1 FROM DUAL para verificar conectividad real.
func (d *DB) HealthCheck(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	var n int
	return d.QueryRowContext(ctx, "SELECT 1 FROM DUAL").Scan(&n)
}

// WithTx ejecuta fn dentro de una transacción con commit/rollback automático.
func (d *DB) WithTx(ctx context.Context, fn func(*sql.Tx) error) (err error) {
	tx, err := d.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelReadCommitted})
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