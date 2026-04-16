<script setup lang="ts">
import type { OverviewRow } from '@/types'
import { useDashboardStore, CHART_COLORS } from '@/stores/dashboard'

const props = defineProps<{ row: OverviewRow }>()
const store = useDashboardStore()

function accentColor(): string {
  let hash = 0
  const str = props.row.element || ''
  for (let i = 0; i < str.length; i++) {
    hash = ((hash << 5) - hash + str.charCodeAt(i)) | 0
  }
  return CHART_COLORS[Math.abs(hash) % CHART_COLORS.length]
}

function formatValor(v: number): string {
  if (Math.abs(v) >= 1000) return v.toLocaleString('es-CO', { maximumFractionDigits: 2 })
  if (Math.abs(v) >= 1) return v.toLocaleString('es-CO', { maximumFractionDigits: 4 })
  return v.toLocaleString('es-CO', { maximumFractionDigits: 6 })
}

function formatTimestamp(iso: string): string {
  const d = new Date(iso)
  return d.toLocaleDateString('es-CO', { day: '2-digit', month: 'short', year: 'numeric' })
    + ' ' + d.toLocaleTimeString('es-CO', { hour: '2-digit', minute: '2-digit' })
}

const isSelected = () => store.isSignalSelected(props.row.senal_id)
</script>

<template>
  <button
    class="relative text-left w-full p-4 rounded-xl border transition-all duration-200 overflow-hidden group"
    :class="isSelected()
      ? 'border-epm-forest bg-epm-forest-50 shadow-md shadow-epm-forest/10'
      : 'border-border bg-white hover:border-epm-gray-300 hover:shadow-sm hover:-translate-y-0.5'"
    @click="store.toggleSignal(row)"
  >
    <!-- Accent bar -->
    <div
      class="absolute top-0 left-0 w-1 h-full rounded-l-xl transition-opacity"
      :style="{ backgroundColor: accentColor() }"
      :class="isSelected() ? 'opacity-100' : 'opacity-40 group-hover:opacity-70'"
    />

    <!-- Header -->
    <div class="flex items-start justify-between mb-1 pl-2">
      <span class="font-mono text-sm font-bold text-foreground">{{ row.element }}</span>
      <span class="text-[10px] font-mono text-epm-gray-400 bg-epm-gray-50 px-1.5 py-0.5 rounded">{{ row.unidades || '—' }}</span>
    </div>

    <!-- Path -->
    <div class="text-[11px] text-epm-gray-400 truncate pl-2 mb-2">
      {{ row.b1 }} › {{ row.b2 }} › {{ row.b3 }}
    </div>

    <!-- Value -->
    <div class="pl-2">
      <div
        v-if="row.last_valor != null"
        class="font-mono text-2xl font-bold leading-tight"
        :style="{ color: accentColor() }"
      >
        {{ formatValor(row.last_valor) }}
      </div>
      <div v-else class="font-mono text-base text-epm-gray-300">
        Sin datos
      </div>
    </div>

    <!-- Timestamp -->
    <div v-if="row.last_fecha" class="text-[10px] font-mono text-epm-gray-400 mt-1 pl-2">
      {{ formatTimestamp(row.last_fecha) }}
    </div>
  </button>
</template>
