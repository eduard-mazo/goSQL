// ── API response types ──────────────────────────────────────────────────────

export interface Station {
  name: string
  signal_count: number
  active_count: number
}

export interface Signal {
  senal_id: number
  b1: string
  b2: string
  b3: string
  element: string
  unidades: string
  activo: boolean
}

export interface ValuePoint {
  fecha: string
  synced_at: string
  senal_id: number
  valor: number | null
}

export interface OverviewRow {
  senal_id: number
  b1: string
  b2: string
  b3: string
  element: string
  unidades: string
  last_fecha: string | null
  last_valor: number | null
}

export interface Stats {
  total_signals: number
  active_signals: number
  total_values: number
  min_fecha: string
  max_fecha: string
  station_count: number
}
