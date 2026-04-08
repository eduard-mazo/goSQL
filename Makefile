# ──────────────────────────────────────────────────────────────────────────────
#  roc-valores — Makefile
#  Módulo Go: goSQL  |  Driver: sijms/go-ora (sin CGO)
#  Ejecutar desde terminal MSYS2 (--login -i) para que el entorno Go esté completo.
# ──────────────────────────────────────────────────────────────────────────────

# ── variables ─────────────────────────────────────────────────────────────────
BINARY    := roc-valores
MODULE    := goSQL
MAIN      := ./cmd/main.go
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

# Flags de compilación: binario standalone, sin información de debug innecesaria
LDFLAGS  := -s -w \
            -X main.version=$(VERSION) \
            -X main.commit=$(COMMIT) \
            -X main.buildTime=$(BUILD_TS)

# Paquetes Go del proyecto (excluye vendor si existiera)
PKGS     := $(shell go list ./... 2>/dev/null)

# ── targets por defecto ───────────────────────────────────────────────────────
.DEFAULT_GOAL := help

.PHONY: all build build-race run run-senales run-valores run-insertar run-batch \
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

# ── run ───────────────────────────────────────────────────────────────────────

## run: ejecuta el binario compilado (subcomando 'senales' por defecto)
run: build
	$(TARGET) senales

## run-senales: consulta y muestra señales activas
run-senales: build
	$(TARGET) senales

## run-valores: muestra los últimos 10 valores
run-valores: build
	$(TARGET) valores

## run-insertar: inserta un valor de prueba
run-insertar: build
	$(TARGET) insertar

## run-batch: inserta un lote de valores de prueba
run-batch: build
	$(TARGET) batch

## dev: compila y corre sin generar binario en disco
dev:
	go run $(MAIN) senales

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
		{ echo "  SKIP: golangci-lint no instalado. Instalar con:"; \
		  echo "  go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest"; exit 0; }
	golangci-lint run ./...

## staticcheck: análisis avanzado (instalar: go install honnef.co/go/tools/cmd/staticcheck@latest)
staticcheck:
	@echo "==> staticcheck"
	@command -v staticcheck >/dev/null 2>&1 || \
		{ echo "  SKIP: staticcheck no instalado. Instalar con:"; \
		  echo "  go install honnef.co/go/tools/cmd/staticcheck@latest"; exit 0; }
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

## clean: elimina binarios y artefactos de cobertura
clean:
	@echo "==> clean"
	@rm -rf $(BUILD_DIR) cover.out cover.html
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
