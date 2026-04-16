# ──────────────────────────────────────────────────────────────────────────────
#  roc-valores — Makefile
#  Módulo Go: goSQL  |  Drivers: sijms/go-ora (Oracle) + modernc/sqlite (SQLite)
# ──────────────────────────────────────────────────────────────────────────────

# ── variables ─────────────────────────────────────────────────────────────────
BINARY    := roc-valores
MODULE    := goSQL
MAIN      := ./cmd
BUILD_DIR := ./bin

# Detecta Windows (MSYS2 devuelve MSYS_NT-* o MINGW64_NT-*)
UNAME_S := $(shell uname -s 2>/dev/null || echo Windows)
ifeq ($(findstring NT,$(UNAME_S)),NT)
  EXT := .exe
else
  EXT :=
endif

TARGET   := $(BUILD_DIR)/$(BINARY)$(EXT)
VERSION  := $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
COMMIT   := $(shell git rev-parse --short HEAD 2>/dev/null || echo unknown)
BUILD_TS := $(shell date -u +"%Y-%m-%dT%H:%M:%SZ" 2>/dev/null || echo unknown)

LDFLAGS  := -s -w \
            -X main.version=$(VERSION) \
            -X main.commit=$(COMMIT) \
            -X main.buildTime=$(BUILD_TS)

PKGS     = $(shell go list ./... 2>/dev/null)

# SQLite database path (override: make serve SQLITE_DB=./other.db)
SQLITE_DB ?= ./roc.db

# API server port (override: make serve API_PORT=3000)
API_PORT ?= 8080

# Dashboard (Vue 3 + Vite) — requires Node >= 18
DASHBOARD_DIR := ./web-dashboard

# ── targets ──────────────────────────────────────────────────────────────────
.DEFAULT_GOAL := help

.PHONY: all build build-race \
        run seed sync \
        run-sqlite seed-sqlite sync-sqlite push-to-oracle backfill-oracle \
        serve serve-oracle \
        dashboard-install dashboard-dev dashboard-build \
        dev \
        test test-verbose test-race test-cover \
        vet fmt fmt-check lint staticcheck check \
        tidy deps deps-list \
        clean help

# ── build ─────────────────────────────────────────────────────────────────────

## all: fmt + vet + build
all: fmt vet build

## build: compila el binario en ./bin/
build:
	@echo "==> build  $(TARGET)  [$(VERSION)]"
	@mkdir -p $(BUILD_DIR)
	go build -ldflags "$(LDFLAGS)" -o $(TARGET) $(MAIN)
	@echo "    OK → $(TARGET)"

## build-race: compila con el detector de carreras (solo desarrollo)
build-race:
	@echo "==> build (race detector)"
	@mkdir -p $(BUILD_DIR)
	go build -race -ldflags "$(LDFLAGS)" -o $(TARGET) $(MAIN)

# ── run (Oracle) ──────────────────────────────────────────────────────────────

## run: daemon Oracle — seed + sync cada hora en :05
run: build
	$(TARGET) run

## seed: inserta señales faltantes en Oracle ROC_SENALES y termina
seed: build
	$(TARGET) seed

## sync: seed + un ciclo delta-sync en Oracle y termina
sync: build
	$(TARGET) sync

# ── run (SQLite) ──────────────────────────────────────────────────────────────

## run-sqlite: daemon SQLite (SQLITE_DB=./roc.db) — seed + sync cada hora en :05
run-sqlite: build
	$(TARGET) --sqlite $(SQLITE_DB) run

## seed-sqlite: inserta señales faltantes en SQLite y termina
seed-sqlite: build
	$(TARGET) --sqlite $(SQLITE_DB) seed

## sync-sqlite: seed + un ciclo delta-sync en SQLite y termina
sync-sqlite: build
	$(TARGET) --sqlite $(SQLITE_DB) sync

## push-to-oracle: empuja datos de SQLite (SQLITE_DB) a Oracle (requiere .env)
push-to-oracle: build
	$(TARGET) --sqlite $(SQLITE_DB) push

## backfill-oracle: envía datos históricos de SQLite (SQLITE_DB) a Oracle (requiere .env)
backfill-oracle: build
	$(TARGET) --sqlite $(SQLITE_DB) backfill

# ── serve (API REST) ─────────────────────────────────────────────────────────

## serve: API REST con SQLite (API_PORT=8080, SQLITE_DB=./roc.db)
serve: build
	$(TARGET) --sqlite $(SQLITE_DB) serve :$(API_PORT)

## serve-oracle: API REST con Oracle (requiere .env, API_PORT=8080)
serve-oracle: build
	$(TARGET) serve :$(API_PORT)

# ── dashboard (Vue 3 + Vite) ─────────────────────────────────────────────────

## dashboard-install: instala dependencias npm del dashboard
dashboard-install:
	@echo "==> npm install (web-dashboard)"
	cd $(DASHBOARD_DIR) && npm install

## dashboard-dev: Vite dev server :5173 — proxy /api → localhost:API_PORT
dashboard-dev:
	@echo "==> vite dev (proxy /api → http://localhost:$(API_PORT))"
	cd $(DASHBOARD_DIR) && VITE_API_PORT=$(API_PORT) npm run dev

## dashboard-build: compila Vue → web-dashboard/dist/ (para deploy estático)
dashboard-build:
	@echo "==> dashboard build"
	cd $(DASHBOARD_DIR) && npm run build
	@echo "    OK → $(DASHBOARD_DIR)/dist/"

## dev: compila y corre sin generar binario (Oracle, subcomando run)
dev:
	go run $(MAIN) run

# ── test ──────────────────────────────────────────────────────────────────────

## test: corre todos los tests unitarios
test:
	@echo "==> test"
	go test -count=1 -timeout 60s $(PKGS)

## test-verbose: tests con salida detallada
test-verbose:
	@echo "==> test (verbose)"
	go test -v -count=1 -timeout 60s $(PKGS)

## test-race: tests con detector de data races
test-race:
	@echo "==> test (race)"
	go test -race -count=1 -timeout 60s $(PKGS)

## test-cover: cobertura de tests → cover.html
test-cover:
	@echo "==> test (coverage)"
	go test -count=1 -timeout 60s -coverprofile=cover.out $(PKGS)
	go tool cover -html=cover.out -o cover.html
	@echo "    Reporte → cover.html"

# ── calidad ───────────────────────────────────────────────────────────────────

## vet: análisis estático estándar de Go
vet:
	@echo "==> vet"
	go vet $(PKGS)

## fmt: formatea el código fuente con gofmt
fmt:
	@echo "==> fmt"
	gofmt -l -w .

## fmt-check: verifica formato sin modificar archivos (útil en CI)
fmt-check:
	@echo "==> fmt check"
	@DIFF=$$(gofmt -l .); \
	if [ -n "$$DIFF" ]; then \
		echo "Archivos sin formatear:"; echo "$$DIFF"; exit 1; \
	fi
	@echo "    OK"

## lint: golangci-lint (instalar: go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest)
lint:
	@echo "==> lint"
	@command -v golangci-lint >/dev/null 2>&1 || \
		{ echo "  SKIP: golangci-lint no instalado."; exit 0; }
	golangci-lint run ./...

## staticcheck: análisis avanzado (instalar: go install honnef.co/go/tools/cmd/staticcheck@latest)
staticcheck:
	@echo "==> staticcheck"
	@command -v staticcheck >/dev/null 2>&1 || \
		{ echo "  SKIP: staticcheck no instalado."; exit 0; }
	staticcheck $(PKGS)

## check: suite completa — fmt-check + vet + test
check: fmt-check vet test

# ── dependencias ──────────────────────────────────────────────────────────────

## tidy: limpia y sincroniza go.mod / go.sum
tidy:
	@echo "==> go mod tidy"
	go mod tidy

## deps: descarga dependencias al cache local
deps:
	@echo "==> go mod download"
	go mod download

## deps-list: muestra las dependencias directas
deps-list:
	@echo "==> dependencias directas"
	go list -m -f '{{if not .Indirect}}{{.}}{{end}}' all

# ── limpieza ──────────────────────────────────────────────────────────────────

## clean: elimina binarios, cobertura y dist del dashboard
clean:
	@echo "==> clean"
	@rm -rf $(BUILD_DIR) cover.out cover.html $(DASHBOARD_DIR)/dist
	@echo "    OK"

# ── ayuda ─────────────────────────────────────────────────────────────────────

## help: lista todos los targets disponibles
help:
	@echo ""
	@echo "  $(MODULE) — $(BINARY)"
	@echo "  Uso: make <target>"
	@echo ""
	@grep -E '^## ' $(MAKEFILE_LIST) | \
		sed 's/## //' | \
		awk -F: '{printf "  \033[36m%-18s\033[0m %s\n", $$1, $$2}'
	@echo ""
	@echo "  Variables:"
	@echo "    SQLITE_DB=./roc.db      ruta a la base SQLite"
	@echo "    API_PORT=8080           puerto del servidor API"
	@echo ""
	@echo "  ── Dashboard ──"
	@echo "  Desarrollo:   terminal 1: make serve"
	@echo "                terminal 2: make dashboard-dev"
	@echo "  Puerto custom: make serve API_PORT=3000"
	@echo "                 make dashboard-dev API_PORT=3000"
	@echo ""
	@echo "  CLI directo:"
	@echo "    ./bin/roc-valores --sqlite ./roc-valores.db serve :3000"
	@echo ""
