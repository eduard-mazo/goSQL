import type { Station, Signal, OverviewRow, Stats, ValuePoint } from '@/types'

const BASE = import.meta.env.VITE_API_BASE ?? ''

async function get<T>(path: string): Promise<T> {
  const res = await fetch(`${BASE}${path}`)
  if (!res.ok) {
    const body = await res.text()
    throw new Error(`API ${res.status}: ${body}`)
  }
  return res.json() as Promise<T>
}

export function fetchStations(): Promise<Station[]> {
  return get<Station[]>('/api/stations')
}

export function fetchSignals(station?: string): Promise<Signal[]> {
  const qs = station ? `?station=${encodeURIComponent(station)}` : ''
  return get<Signal[]>(`/api/signals${qs}`)
}

export function fetchOverview(): Promise<OverviewRow[]> {
  return get<OverviewRow[]>('/api/overview')
}

export function fetchStats(): Promise<Stats> {
  return get<Stats>('/api/stats')
}

export function fetchValues(
  senalId: number,
  from?: string,
  to?: string,
): Promise<ValuePoint[]> {
  const params = new URLSearchParams({ senal_id: String(senalId) })
  if (from) params.set('from', from)
  if (to) params.set('to', to)
  return get<ValuePoint[]>(`/api/values?${params}`)
}
