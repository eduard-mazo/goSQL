<script setup lang="ts">
import { computed } from 'vue'
import type { OverviewRow } from '@/types'
import { useDashboardStore, CHART_COLORS } from '@/stores/dashboard'
import { Activity } from 'lucide-vue-next'

const store = useDashboardStore()

/** Group signals by station (b1) → meter (b3) */
interface MeterGroup {
  meter: string       // b3 value (BRAZO1, BRAZO2, …)
  subsystem: string   // b2 value (MEDICION, DS, RINT, …)
  signals: OverviewRow[]
}
interface StationGroup {
  station: string
  meters: MeterGroup[]
}

const grouped = computed<StationGroup[]>(() => {
  const stationMap = new Map<string, Map<string, MeterGroup>>()
  const stationOrder: string[] = []

  for (const row of store.filteredOverview) {
    const st = row.b1 || '?'
    const meter = row.b3 || '?'
    const sub = row.b2 || ''

    if (!stationMap.has(st)) {
      stationMap.set(st, new Map())
      stationOrder.push(st)
    }
    const meterMap = stationMap.get(st)!
    if (!meterMap.has(meter)) {
      meterMap.set(meter, { meter, subsystem: sub, signals: [] })
    }
    meterMap.get(meter)!.signals.push(row)
  }

  return stationOrder.map(st => ({
    station: st,
    meters: Array.from(stationMap.get(st)!.values()),
  }))
})

function chipColor(element: string): string {
  let hash = 0
  for (let i = 0; i < element.length; i++) {
    hash = ((hash << 5) - hash + element.charCodeAt(i)) | 0
  }
  return CHART_COLORS[Math.abs(hash) % CHART_COLORS.length]
}

function formatValor(v: number | null): string {
  if (v == null) return '—'
  if (Math.abs(v) >= 1000) return v.toLocaleString('es-CO', { maximumFractionDigits: 1 })
  if (Math.abs(v) >= 1) return v.toLocaleString('es-CO', { maximumFractionDigits: 2 })
  return v.toLocaleString('es-CO', { maximumFractionDigits: 4 })
}

function timeAgo(iso: string | null): string {
  if (!iso) return ''
  const diff = Date.now() - new Date(iso).getTime()
  const hours = Math.floor(diff / 3600000)
  if (hours < 1) return 'ahora'
  if (hours < 24) return `${hours}h`
  return `${Math.floor(hours / 24)}d`
}
</script>

<template>
  <div class="rounded-xl border border-border bg-white overflow-hidden">
    <!-- Header -->
    <div class="flex items-center justify-between px-5 py-3.5 border-b border-border">
      <div class="flex items-center gap-2">
        <Activity :size="15" class="text-epm-forest" />
        <h2 class="text-sm font-bold font-display text-foreground">
          {{ store.selectedStation || 'Señales' }}
        </h2>
      </div>
      <div class="flex items-center gap-3">
        <span
          v-if="store.selectedSignals.length > 0"
          class="text-[10px] font-mono font-semibold bg-epm-forest text-white px-2 py-0.5 rounded-full"
        >
          {{ store.selectedSignals.length }}/8 seleccionadas
        </span>
        <span class="text-[10px] font-mono text-epm-gray-400">
          {{ store.filteredOverview.length }} señales
        </span>
      </div>
    </div>

    <!-- Grouped chips -->
    <div class="p-4 space-y-4">
      <div v-for="stGroup in grouped" :key="stGroup.station">
        <!-- Station header (only in "all" view) -->
        <div
          v-if="!store.selectedStation"
          class="flex items-center gap-2 mb-2.5"
        >
          <span class="w-1.5 h-1.5 rounded-full bg-epm-forest shrink-0" />
          <span class="text-[11px] font-bold uppercase tracking-[0.15em] text-epm-gray-500">
            {{ stGroup.station }}
          </span>
          <div class="flex-1 h-px bg-epm-gray-100" />
        </div>

        <!-- Meter groups -->
        <div class="space-y-2.5" :class="!store.selectedStation ? 'ml-3' : ''">
          <div
            v-for="mGroup in stGroup.meters"
            :key="mGroup.meter"
            class="group"
          >
            <!-- Meter label -->
            <div class="flex items-center gap-2 mb-1.5">
              <span class="text-[10px] font-mono font-semibold text-epm-gray-400 uppercase tracking-wider">
                {{ mGroup.meter }}
              </span>
              <span class="text-[9px] font-mono text-epm-gray-300">
                {{ mGroup.subsystem }}
              </span>
            </div>

            <!-- Signal chips -->
            <div class="flex flex-wrap gap-1.5">
              <button
                v-for="sig in mGroup.signals"
                :key="sig.senal_id"
                class="inline-flex items-center gap-1.5 pl-2 pr-2.5 py-1 rounded-lg text-[11px] font-mono font-medium transition-all duration-150 border cursor-pointer select-none"
                :class="store.isSignalSelected(sig.senal_id)
                  ? 'shadow-sm scale-[1.02]'
                  : 'border-epm-gray-200 bg-white hover:border-epm-gray-300 hover:shadow-sm text-epm-gray-700'"
                :style="store.isSignalSelected(sig.senal_id)
                  ? {
                      borderColor: chipColor(sig.element),
                      backgroundColor: chipColor(sig.element) + '14',
                      color: chipColor(sig.element),
                      boxShadow: `0 1px 4px ${chipColor(sig.element)}22`,
                    }
                  : undefined"
                :title="`${sig.b1} › ${sig.b2} › ${sig.b3} › ${sig.element}\nÚltimo: ${formatValor(sig.last_valor)} ${sig.unidades || ''}`"
                @click="store.toggleSignal(sig)"
              >
                <!-- Color dot -->
                <span
                  class="w-1.5 h-1.5 rounded-full shrink-0 transition-colors"
                  :style="{ backgroundColor: chipColor(sig.element) }"
                />
                <!-- Element name -->
                <span class="font-semibold">{{ sig.element }}</span>
                <!-- Latest value -->
                <span
                  v-if="sig.last_valor != null"
                  class="text-[10px] opacity-60"
                >
                  {{ formatValor(sig.last_valor) }}
                </span>
                <!-- Time ago -->
                <span
                  v-if="sig.last_fecha"
                  class="text-[9px] opacity-40"
                >
                  {{ timeAgo(sig.last_fecha) }}
                </span>
              </button>
            </div>
          </div>
        </div>
      </div>

      <!-- Empty state -->
      <div
        v-if="store.filteredOverview.length === 0"
        class="flex items-center justify-center py-10 text-epm-gray-400 text-sm"
      >
        No hay señales disponibles
      </div>
    </div>
  </div>
</template>
