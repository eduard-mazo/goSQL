// Package api exposes a read-only REST/JSON layer on top of the
// ROC_SENALES / ROC_VALORES repositories.  It is used by the "serve"
// subcommand to power the monitoring dashboard.
package api

import (
	"context"
	"database/sql"
	"encoding/json"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"goSQL/db"
	"goSQL/repository"
)

// Server holds the repositories and the HTTP mux.
type Server struct {
	db        *db.DB
	senalRepo *repository.SenalRepository
	valorRepo *repository.ValorRepository
	mux       *http.ServeMux
}

// New creates a Server, wires the API routes, and returns it.
// The server is API-only — the Vue dashboard runs separately.
func New(database *db.DB) *Server {
	s := &Server{
		db:        database,
		senalRepo: repository.NewSenalRepository(database),
		valorRepo: repository.NewValorRepository(database),
		mux:       http.NewServeMux(),
	}

	s.mux.HandleFunc("GET /api/stations", s.handleStations)
	s.mux.HandleFunc("GET /api/signals", s.handleSignals)
	s.mux.HandleFunc("GET /api/values", s.handleValues)
	s.mux.HandleFunc("GET /api/overview", s.handleOverview)
	s.mux.HandleFunc("GET /api/stats", s.handleStats)

	return s
}

// ListenAndServe starts the HTTP server.  Blocks until the context is
// cancelled, then shuts down gracefully.
func (s *Server) ListenAndServe(ctx context.Context, addr string) error {
	srv := &http.Server{
		Addr:    addr,
		Handler: cors(s.mux),
	}

	go func() {
		<-ctx.Done()
		shutCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = srv.Shutdown(shutCtx)
	}()

	log.Printf("[api] listening on %s", addr)
	if err := srv.ListenAndServe(); err != http.ErrServerClosed {
		return err
	}
	return nil
}

// ── middleware ────────────────────────────────────────────────────────────────

func cors(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// ── helpers ──────────────────────────────────────────────────────────────────

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeErr(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

func ptrOr(p *string, fallback string) string {
	if p == nil {
		return fallback
	}
	return *p
}

// ── handlers ─────────────────────────────────────────────────────────────────

// StationDTO is the JSON shape for /api/stations.
type StationDTO struct {
	Name        string `json:"name"`
	SignalCount int    `json:"signal_count"`
	ActiveCount int    `json:"active_count"`
}

// handleStations returns distinct B1 values with signal counts.
func (s *Server) handleStations(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	senales, err := s.senalRepo.FindAll(ctx)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}

	type stationAcc struct {
		total  int
		active int
	}
	byStation := make(map[string]*stationAcc)
	var order []string

	for _, sig := range senales {
		name := ptrOr(sig.B1, "?")
		acc, ok := byStation[name]
		if !ok {
			acc = &stationAcc{}
			byStation[name] = acc
			order = append(order, name)
		}
		acc.total++
		if sig.EstaActivo() {
			acc.active++
		}
	}

	out := make([]StationDTO, 0, len(order))
	for _, name := range order {
		acc := byStation[name]
		out = append(out, StationDTO{
			Name:        name,
			SignalCount: acc.total,
			ActiveCount: acc.active,
		})
	}
	writeJSON(w, 200, out)
}

// SignalDTO is the JSON shape for /api/signals.
type SignalDTO struct {
	SenalID  float64 `json:"senal_id"`
	B1       string  `json:"b1"`
	B2       string  `json:"b2"`
	B3       string  `json:"b3"`
	Element  string  `json:"element"`
	Unidades string  `json:"unidades"`
	Activo   bool    `json:"activo"`
}

// handleSignals returns all signals, optionally filtered by ?station=B1.
func (s *Server) handleSignals(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	senales, err := s.senalRepo.FindAll(ctx)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}

	stationFilter := r.URL.Query().Get("station")

	var out []SignalDTO
	for _, sig := range senales {
		b1 := ptrOr(sig.B1, "")
		if stationFilter != "" && !strings.EqualFold(b1, stationFilter) {
			continue
		}
		out = append(out, SignalDTO{
			SenalID:  sig.SenalID,
			B1:       b1,
			B2:       ptrOr(sig.B2, ""),
			B3:       ptrOr(sig.B3, ""),
			Element:  ptrOr(sig.Element, ""),
			Unidades: ptrOr(sig.Unidades, ""),
			Activo:   sig.EstaActivo(),
		})
	}
	if out == nil {
		out = []SignalDTO{}
	}
	writeJSON(w, 200, out)
}

// ValueDTO is the JSON shape for /api/values.
type ValueDTO struct {
	Fecha    string   `json:"fecha"`
	SyncedAt string   `json:"synced_at"`
	SenalID  float64  `json:"senal_id"`
	Valor    *float64 `json:"valor"`
}

// handleValues returns time series for a signal.
// Query params: senal_id (required), from, to (RFC3339, optional).
func (s *Server) handleValues(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	q := r.URL.Query()

	idStr := q.Get("senal_id")
	if idStr == "" {
		writeErr(w, 400, "missing senal_id parameter")
		return
	}
	senalID, err := strconv.ParseFloat(idStr, 64)
	if err != nil {
		writeErr(w, 400, "invalid senal_id")
		return
	}

	fromStr := q.Get("from")
	toStr := q.Get("to")

	var valores []struct {
		Fecha    time.Time
		SyncedAt time.Time
		SenalID  float64
		Valor    *float64
	}

	if fromStr != "" && toStr != "" {
		desde, err1 := time.Parse(time.RFC3339, fromStr)
		hasta, err2 := time.Parse(time.RFC3339, toStr)
		if err1 != nil || err2 != nil {
			writeErr(w, 400, "from/to must be RFC3339")
			return
		}
		raw, err := s.valorRepo.FindByRango(ctx, senalID, desde, hasta)
		if err != nil {
			writeErr(w, 500, err.Error())
			return
		}
		for _, v := range raw {
			valores = append(valores, struct {
				Fecha    time.Time
				SyncedAt time.Time
				SenalID  float64
				Valor    *float64
			}{v.Fecha, v.SyncedAt, v.SenalID, v.Valor})
		}
	} else {
		// No range: return ALL values for this signal.
		raw, err := s.valorRepo.FindBySenalID(ctx, senalID)
		if err != nil {
			writeErr(w, 500, err.Error())
			return
		}
		for _, v := range raw {
			valores = append(valores, struct {
				Fecha    time.Time
				SyncedAt time.Time
				SenalID  float64
				Valor    *float64
			}{v.Fecha, v.SyncedAt, v.SenalID, v.Valor})
		}
	}

	out := make([]ValueDTO, 0, len(valores))
	for _, v := range valores {
		out = append(out, ValueDTO{
			Fecha:    v.Fecha.UTC().Format(time.RFC3339),
			SyncedAt: v.SyncedAt.UTC().Format(time.RFC3339),
			SenalID:  v.SenalID,
			Valor:    v.Valor,
		})
	}
	writeJSON(w, 200, out)
}

// OverviewRow is returned by /api/overview — latest value per signal.
type OverviewRow struct {
	SenalID    float64  `json:"senal_id"`
	B1         string   `json:"b1"`
	B2         string   `json:"b2"`
	B3         string   `json:"b3"`
	Element    string   `json:"element"`
	Unidades   string   `json:"unidades"`
	LastFecha  *string  `json:"last_fecha"`
	LastValor  *float64 `json:"last_valor"`
}

// handleOverview returns the latest value for every active signal.
func (s *Server) handleOverview(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	senales, err := s.senalRepo.FindActivas(ctx)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}

	// Dashboard-specific query: latest value per signal in one scan.
	var q string
	if s.db.Dialect == db.DialectSQLite {
		q = `SELECT v.SENAL_ID, v.FECHA, v.VALOR
		     FROM ROC_VALORES v
		     INNER JOIN (
		       SELECT SENAL_ID, MAX(FECHA) AS mf
		       FROM ROC_VALORES GROUP BY SENAL_ID
		     ) latest ON v.SENAL_ID = latest.SENAL_ID AND v.FECHA = latest.mf`
	} else {
		q = `SELECT v.SENAL_ID, v.FECHA, v.VALOR
		     FROM HEPMGA.ROC_VALORES v
		     INNER JOIN (
		       SELECT SENAL_ID, MAX(FECHA) AS mf
		       FROM HEPMGA.ROC_VALORES GROUP BY SENAL_ID
		     ) latest ON v.SENAL_ID = latest.SENAL_ID AND v.FECHA = latest.mf`
	}

	rows, err := s.db.QueryContext(ctx, q)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	defer rows.Close()

	type latestInfo struct {
		fecha string
		valor *float64
	}
	latestByID := make(map[float64]latestInfo)

	for rows.Next() {
		var senalID float64
		var valor sql.NullFloat64
		if s.db.Dialect == db.DialectSQLite {
			var fechaStr string
			if err := rows.Scan(&senalID, &fechaStr, &valor); err != nil {
				writeErr(w, 500, err.Error())
				return
			}
			v := latestInfo{fecha: fechaStr}
			if valor.Valid {
				v.valor = &valor.Float64
			}
			latestByID[senalID] = v
		} else {
			var fecha time.Time
			if err := rows.Scan(&senalID, &fecha, &valor); err != nil {
				writeErr(w, 500, err.Error())
				return
			}
			v := latestInfo{fecha: fecha.UTC().Format(time.RFC3339)}
			if valor.Valid {
				v.valor = &valor.Float64
			}
			latestByID[senalID] = v
		}
	}
	if err := rows.Err(); err != nil {
		writeErr(w, 500, err.Error())
		return
	}

	out := make([]OverviewRow, 0, len(senales))
	for _, sig := range senales {
		row := OverviewRow{
			SenalID:  sig.SenalID,
			B1:       ptrOr(sig.B1, ""),
			B2:       ptrOr(sig.B2, ""),
			B3:       ptrOr(sig.B3, ""),
			Element:  ptrOr(sig.Element, ""),
			Unidades: ptrOr(sig.Unidades, ""),
		}
		if info, ok := latestByID[sig.SenalID]; ok {
			row.LastFecha = &info.fecha
			row.LastValor = info.valor
		}
		out = append(out, row)
	}
	writeJSON(w, 200, out)
}

// StatsDTO is the JSON shape for /api/stats.
type StatsDTO struct {
	TotalSignals  int    `json:"total_signals"`
	ActiveSignals int    `json:"active_signals"`
	TotalValues   int    `json:"total_values"`
	MinFecha      string `json:"min_fecha"`
	MaxFecha      string `json:"max_fecha"`
	StationCount  int    `json:"station_count"`
}

// handleStats returns global statistics.
func (s *Server) handleStats(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	senales, err := s.senalRepo.FindAll(ctx)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}

	total := len(senales)
	active := 0
	stations := make(map[string]struct{})
	for _, sig := range senales {
		if sig.EstaActivo() {
			active++
		}
		if sig.B1 != nil {
			stations[*sig.B1] = struct{}{}
		}
	}

	// Value counts + date range
	var countQ, rangeQ string
	if s.db.Dialect == db.DialectSQLite {
		countQ = `SELECT COUNT(*) FROM ROC_VALORES`
		rangeQ = `SELECT MIN(FECHA), MAX(FECHA) FROM ROC_VALORES`
	} else {
		countQ = `SELECT COUNT(*) FROM HEPMGA.ROC_VALORES`
		rangeQ = `SELECT MIN(FECHA), MAX(FECHA) FROM HEPMGA.ROC_VALORES`
	}

	var totalValues int
	_ = s.db.QueryRowContext(ctx, countQ).Scan(&totalValues)

	dto := StatsDTO{
		TotalSignals:  total,
		ActiveSignals: active,
		TotalValues:   totalValues,
		StationCount:  len(stations),
	}

	if s.db.Dialect == db.DialectSQLite {
		var minF, maxF sql.NullString
		if err := s.db.QueryRowContext(ctx, rangeQ).Scan(&minF, &maxF); err == nil {
			if minF.Valid {
				dto.MinFecha = minF.String
			}
			if maxF.Valid {
				dto.MaxFecha = maxF.String
			}
		}
	} else {
		var minF, maxF sql.NullTime
		if err := s.db.QueryRowContext(ctx, rangeQ).Scan(&minF, &maxF); err == nil {
			if minF.Valid {
				dto.MinFecha = minF.Time.UTC().Format(time.RFC3339)
			}
			if maxF.Valid {
				dto.MaxFecha = maxF.Time.UTC().Format(time.RFC3339)
			}
		}
	}

	writeJSON(w, 200, dto)
}
