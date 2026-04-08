package config

import (
	"fmt"
	"log"
	"os"
	"strconv"
	"time"

	"github.com/joho/godotenv"
	go_ora "github.com/sijms/go-ora/v2"
)

type DBConfig struct {
	Host            string
	Port            int
	Service         string
	User            string
	Password        string
	DSN             string
	MaxOpenConns    int
	MaxIdleConns    int
	ConnMaxLifetime time.Duration
}

func Load() (*DBConfig, error) {
	if err := godotenv.Load(); err != nil {
		log.Println("[config] sin .env, usando variables del sistema")
	}

	port, err := strconv.Atoi(getEnv("DB_PORT", "1526"))
	if err != nil {
		return nil, fmt.Errorf("DB_PORT invalido: %w", err)
	}

	maxOpen, _ := strconv.Atoi(getEnv("DB_MAX_OPEN_CONNS", "5"))
	maxIdle, _ := strconv.Atoi(getEnv("DB_MAX_IDLE_CONNS", "2"))
	lifetime, _ := strconv.Atoi(getEnv("DB_CONN_MAX_LIFETIME_MIN", "30"))

	host := getEnv("DB_HOST", "")
	service := getEnv("DB_SERVICE", "")
	user := getEnv("DB_USER", "")
	password := getEnv("DB_PASSWORD", "")

	// go-ora BuildUrl genera: oracle://user:pass@host:port/service
	dsn := go_ora.BuildUrl(host, port, service, user, password, nil)

	log.Printf("[config] DSN construido → oracle://%s:****@%s:%d/%s", user, host, port, service)

	cfg := &DBConfig{
		Host:            host,
		Port:            port,
		Service:         service,
		User:            user,
		Password:        password,
		DSN:             dsn,
		MaxOpenConns:    maxOpen,
		MaxIdleConns:    maxIdle,
		ConnMaxLifetime: time.Duration(lifetime) * time.Minute,
	}

	return cfg, nil
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
