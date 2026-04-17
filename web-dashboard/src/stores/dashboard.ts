import { ref, computed } from 'vue'
import { defineStore } from 'pinia'
import type { Station, OverviewRow, Stats, ValuePoint } from '@/types'
import {
  fetchStations,
  fetchOverview,
  fetchStats,
  fetchValues,
} from '@/api/client'

/** Chart colors — high contrast, perceptually distinct for overlaid lines */
export const CHART_COLORS = [
  '#009653', // EPM forest green
  '#e04040', // red
  '#0082b6', // cerulean blue
  '#d56b00', // burnt orange
  '#8b5cf6', // violet
  '#d29922', // amber/gold
  '#ec4899', // pink
  '#1a9090', // teal
]

export interface SelectedSignal {
  senalId: number
  label: string
  color: string
  unidades: string
}

export const useDashboardStore = defineStore('dashboard', () => {
  // ── state ──────────────────────────────────────────────────────────
  const stations = ref<Station[]>([])
  const overview = ref<OverviewRow[]>([])
  const stats = ref<Stats | null>(null)
  const selectedStation = ref<string | null>(null)
  const selectedSignals = ref<SelectedSignal[]>([])
  const chartData = ref<Record<number, ValuePoint[]>>({})
  const rangeDays = ref(7)
  const customFrom = ref<string | null>(null)
  const customTo = ref<string | null>(null)
  const loading = ref(false)
  const normalized = ref(false)
  const showFill = ref(false)

  // ── getters ────────────────────────────────────────────────────────
  const filteredOverview = computed(() => {
    if (!selectedStation.value) return overview.value
    return overview.value.filter(r => r.b1 === selectedStation.value)
  })

  const overviewGrouped = computed(() => {
    const groups: Record<string, OverviewRow[]> = {}
    const order: string[] = []
    for (const row of filteredOverview.value) {
      const key = row.b1 || '?'
      if (!groups[key]) {
        groups[key] = []
        order.push(key)
      }
      groups[key].push(row)
    }
    return order.map(name => ({ name, signals: groups[name] }))
  })

  /** Per-signal min/max for normalization */
  const signalBounds = computed(() => {
    const bounds: Record<number, { min: number; max: number }> = {}
    for (const sig of selectedSignals.value) {
      const values = chartData.value[sig.senalId] || []
      let min = Infinity
      let max = -Infinity
      for (const v of values) {
        if (v.valor != null) {
          if (v.valor < min) min = v.valor
          if (v.valor > max) max = v.valor
        }
      }
      if (min !== Infinity && max !== -Infinity) {
        bounds[sig.senalId] = { min, max }
      }
    }
    return bounds
  })

  /** Normalize a value to 0–100 range using its signal's min/max, clamped */
  function normalizeValue(senalId: number, valor: number | null): number | null {
    if (valor == null) return null
    const b = signalBounds.value[senalId]
    if (!b || b.max === b.min) return 50 // flat line → middle
    const pct = ((valor - b.min) / (b.max - b.min)) * 100
    return Math.max(0, Math.min(100, pct))
  }

  function toggleNormalized() {
    normalized.value = !normalized.value
  }

  function toggleFill() {
    showFill.value = !showFill.value
  }

  // ── helpers ────────────────────────────────────────────────────────
  function getDateRange(): { from?: string; to?: string } {
    if (customFrom.value && customTo.value) {
      return {
        from: new Date(customFrom.value + 'T00:00:00Z').toISOString(),
        to: new Date(customTo.value + 'T23:59:59Z').toISOString(),
      }
    }
    if (rangeDays.value === 0) return {}
    const to = new Date()
    const from = new Date()
    from.setDate(from.getDate() - rangeDays.value)
    return { from: from.toISOString(), to: to.toISOString() }
  }

  // ── actions ────────────────────────────────────────────────────────
  async function loadAll() {
    loading.value = true
    try {
      const [s, o, st] = await Promise.all([
        fetchStations(),
        fetchOverview(),
        fetchStats(),
      ])
      stations.value = s
      overview.value = o
      stats.value = st
    } finally {
      loading.value = false
    }
  }

  function selectStation(name: string | null) {
    selectedStation.value = name
  }

  function isSignalSelected(senalId: number): boolean {
    return selectedSignals.value.some(s => s.senalId === senalId)
  }

  async function toggleSignal(row: OverviewRow) {
    const idx = selectedSignals.value.findIndex(s => s.senalId === row.senal_id)
    if (idx >= 0) {
      selectedSignals.value.splice(idx, 1)
      delete chartData.value[row.senal_id]
      return
    }
    if (selectedSignals.value.length >= 8) {
      const removed = selectedSignals.value.shift()!
      delete chartData.value[removed.senalId]
    }
    const color = CHART_COLORS[selectedSignals.value.length % CHART_COLORS.length]
    selectedSignals.value.push({
      senalId: row.senal_id,
      label: `${row.b1} > ${row.b3} > ${row.element}`,
      color,
      unidades: row.unidades || '',
    })
    await loadSignalData(row.senal_id)
  }

  function removeSignal(senalId: number) {
    const idx = selectedSignals.value.findIndex(s => s.senalId === senalId)
    if (idx >= 0) {
      selectedSignals.value.splice(idx, 1)
      delete chartData.value[senalId]
    }
  }

  async function loadSignalData(senalId: number) {
    const { from, to } = getDateRange()
    try {
      chartData.value[senalId] = await fetchValues(senalId, from, to)
    } catch {
      chartData.value[senalId] = []
    }
  }

  async function reloadAllSignalData() {
    await Promise.all(
      selectedSignals.value.map(s => loadSignalData(s.senalId))
    )
  }

  async function setRange(days: number) {
    rangeDays.value = days
    customFrom.value = null
    customTo.value = null
    await reloadAllSignalData()
  }

  async function setCustomRange(from: string, to: string) {
    customFrom.value = from
    customTo.value = to
    await reloadAllSignalData()
  }

  return {
    // state
    stations,
    overview,
    stats,
    selectedStation,
    selectedSignals,
    chartData,
    rangeDays,
    customFrom,
    customTo,
    loading,
    normalized,
    showFill,
    // getters
    filteredOverview,
    overviewGrouped,
    signalBounds,
    // actions
    loadAll,
    selectStation,
    isSignalSelected,
    toggleSignal,
    removeSignal,
    setRange,
    setCustomRange,
    reloadAllSignalData,
    normalizeValue,
    toggleNormalized,
    toggleFill,
  }
})
