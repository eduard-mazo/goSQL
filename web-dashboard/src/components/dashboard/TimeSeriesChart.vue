<script setup lang="ts">
import { computed, ref, watch, onMounted } from 'vue'
import { Chart, registerables } from 'chart.js'
import 'chartjs-adapter-date-fns'
import { Line } from 'vue-chartjs'
import { useDashboardStore } from '@/stores/dashboard'
import { X, Maximize2, Minimize2, Layers, Minus } from 'lucide-vue-next'

Chart.register(...registerables)

const store = useDashboardStore()
const activeRange = ref(7)

const hasData = computed(() => store.selectedSignals.length > 0)
const multiSignal = computed(() => store.selectedSignals.length > 1)

/** Compute the visible time span in days to pick the right X-axis format */
const visibleSpanDays = computed(() => {
  let min = Infinity
  let max = -Infinity
  for (const sig of store.selectedSignals) {
    const values = store.chartData[sig.senalId] || []
    for (const v of values) {
      const t = new Date(v.fecha).getTime()
      if (t < min) min = t
      if (t > max) max = t
    }
  }
  if (min === Infinity) return 7
  return (max - min) / 86400000
})

const chartData = computed(() => ({
  datasets: store.selectedSignals.map(sig => {
    const values = store.chartData[sig.senalId] || []
    return {
      label: sig.label + (sig.unidades ? ` (${sig.unidades})` : ''),
      data: values.map(v => ({
        x: new Date(v.fecha).getTime(),
        y: store.normalized
          ? store.normalizeValue(sig.senalId, v.valor)
          : v.valor,
        _raw: v.valor,
      })),
      borderColor: sig.color,
      backgroundColor: sig.color + '20',
      fill: store.showFill,
      pointRadius: values.length < 150 ? 2 : 0,
      pointHoverRadius: 4,
      borderWidth: 2,
      tension: 0.25,
    }
  }),
}))

const chartOptions = computed(() => {
  const span = visibleSpanDays.value

  // Pick time unit and format based on visible span
  let timeUnit: 'hour' | 'day' | 'week' | 'month' = 'hour'
  let tooltipFormat = 'dd MMM yyyy HH:mm'
  if (span > 180) {
    timeUnit = 'month'
    tooltipFormat = 'MMM yyyy'
  } else if (span > 30) {
    timeUnit = 'week'
    tooltipFormat = 'dd MMM yyyy'
  } else if (span > 3) {
    timeUnit = 'day'
    tooltipFormat = 'dd MMM yyyy HH:mm'
  }

  return {
    responsive: true,
    maintainAspectRatio: false,
    interaction: { mode: 'index' as const, intersect: false },
    plugins: {
      legend: {
        display: true,
        position: 'top' as const,
        labels: {
          color: '#6d6d6d',
          font: { family: "'JetBrains Mono', monospace", size: 11 },
          boxWidth: 12, boxHeight: 2, padding: 16,
          usePointStyle: true, pointStyle: 'line' as const,
        },
      },
      tooltip: {
        backgroundColor: '#1a1a1a',
        titleColor: '#ffffff',
        bodyColor: '#d1d1d1',
        borderColor: '#4f4f4f',
        borderWidth: 1,
        titleFont: { family: "'JetBrains Mono', monospace", size: 11 },
        bodyFont: { family: "'JetBrains Mono', monospace", size: 11 },
        padding: 10,
        callbacks: {
          label(ctx: any) {
            const raw = ctx.raw?._raw
            const lbl = ctx.dataset.label || ''
            // Truncate label for readability
            const short = lbl.length > 30 ? lbl.slice(0, 28) + '…' : lbl
            if (store.normalized && raw != null) {
              const pct = ctx.parsed.y?.toFixed(1)
              return `${short}: ${raw.toLocaleString('es-CO', { maximumFractionDigits: 4 })} (${pct}%)`
            }
            const val = ctx.parsed.y
            if (val == null) return `${short}: —`
            return `${short}: ${val.toLocaleString('es-CO', { maximumFractionDigits: 4 })}`
          },
        },
      },
    },
    scales: {
      x: {
        type: 'time' as const,
        time: {
          unit: timeUnit,
          tooltipFormat,
          displayFormats: {
            hour: 'HH:mm',
            day: 'dd MMM',
            week: 'dd MMM',
            month: 'MMM yyyy',
          },
        },
        grid: { color: '#f0f0f0' },
        ticks: {
          color: '#8B8D8E',
          font: { family: "'JetBrains Mono', monospace", size: 10 },
          maxRotation: 45,
          autoSkip: true,
          maxTicksLimit: 18,
        },
        border: { color: '#e4e4e4' },
      },
      y: {
        grid: { color: '#f0f0f0' },
        ticks: {
          color: '#8B8D8E',
          font: { family: "'JetBrains Mono', monospace", size: 10 },
          callback: store.normalized
            ? (v: string | number) => `${Number(v).toFixed(0)}%`
            : undefined,
        },
        border: { color: '#e4e4e4' },
        min: store.normalized ? 0 : undefined,
        max: store.normalized ? 100 : undefined,
        title: store.normalized
          ? { display: true, text: 'Normalizado 0–100%', color: '#8B8D8E', font: { family: "'JetBrains Mono', monospace", size: 10 } }
          : { display: false },
      },
    },
    animation: { duration: 400 },
  }
})

const rangeBtns = [
  { label: '24h', days: 1 },
  { label: '7d', days: 7 },
  { label: '30d', days: 30 },
  { label: '90d', days: 90 },
  { label: '1a', days: 365 },
  { label: 'Todo', days: 0 },
]

const dateFrom = ref('')
const dateTo = ref('')

async function selectRange(days: number) {
  activeRange.value = days
  dateFrom.value = ''
  dateTo.value = ''
  await store.setRange(days)
}

async function onDateChange() {
  if (dateFrom.value && dateTo.value) {
    activeRange.value = -1
    await store.setCustomRange(dateFrom.value, dateTo.value)
  }
}

watch(() => store.rangeDays, (v) => { activeRange.value = v })
onMounted(() => { activeRange.value = store.rangeDays })
</script>

<template>
  <div class="rounded-xl border border-border bg-white overflow-hidden">
    <!-- Header -->
    <div class="flex items-center justify-between px-5 py-3 border-b border-border flex-wrap gap-2">
      <div class="flex items-center gap-2">
        <h2 class="text-sm font-bold font-display text-foreground">Serie Temporal</h2>

        <!-- Normalize toggle -->
        <button
          v-if="multiSignal"
          class="inline-flex items-center gap-1 px-2 py-1 rounded-md text-[11px] font-mono font-semibold border transition-all duration-150"
          :class="store.normalized
            ? 'bg-epm-forest text-white border-epm-forest'
            : 'border-epm-gray-200 text-epm-gray-500 hover:border-epm-gray-300'"
          @click="store.toggleNormalized()"
        >
          <component :is="store.normalized ? Minimize2 : Maximize2" :size="11" />
          {{ store.normalized ? '0–100%' : 'Normalizar' }}
        </button>

        <!-- Fill toggle -->
        <button
          v-if="hasData"
          class="inline-flex items-center gap-1 px-2 py-1 rounded-md text-[11px] font-mono font-semibold border transition-all duration-150"
          :class="store.showFill
            ? 'bg-epm-gray-700 text-white border-epm-gray-700'
            : 'border-epm-gray-200 text-epm-gray-500 hover:border-epm-gray-300'"
          @click="store.toggleFill()"
        >
          <component :is="store.showFill ? Layers : Minus" :size="11" />
          {{ store.showFill ? 'Área' : 'Línea' }}
        </button>
      </div>

      <div class="flex items-center gap-3 flex-wrap">
        <!-- Date pickers -->
        <div class="flex items-center gap-1.5 text-sm text-epm-gray-500">
          <label class="text-[10px] font-mono">Desde</label>
          <input
            v-model="dateFrom"
            type="date"
            class="font-mono text-[11px] bg-epm-gray-50 border border-border rounded-md px-1.5 py-1 text-foreground focus:border-epm-forest focus:outline-none"
            @change="onDateChange"
          />
          <label class="text-[10px] font-mono">Hasta</label>
          <input
            v-model="dateTo"
            type="date"
            class="font-mono text-[11px] bg-epm-gray-50 border border-border rounded-md px-1.5 py-1 text-foreground focus:border-epm-forest focus:outline-none"
            @change="onDateChange"
          />
        </div>
        <!-- Range buttons -->
        <div class="flex gap-0.5 bg-epm-gray-50 rounded-lg p-0.5">
          <button
            v-for="btn in rangeBtns"
            :key="btn.days"
            class="font-mono text-[11px] font-medium px-2 py-1 rounded-md transition-all"
            :class="activeRange === btn.days
              ? 'bg-epm-forest text-white shadow-sm'
              : 'text-epm-gray-500 hover:text-foreground hover:bg-white'"
            @click="selectRange(btn.days)"
          >
            {{ btn.label }}
          </button>
        </div>
      </div>
    </div>

    <!-- Normalization bounds banner -->
    <div
      v-if="store.normalized && hasData"
      class="px-5 py-1.5 bg-epm-gray-50 border-b border-epm-gray-100 flex items-center gap-3 text-[10px] font-mono overflow-x-auto"
    >
      <span class="text-epm-gray-500 font-semibold shrink-0">MIN–MAX:</span>
      <span
        v-for="sig in store.selectedSignals"
        :key="sig.senalId"
        class="inline-flex items-center gap-1 shrink-0"
        :style="{ color: sig.color }"
      >
        <span class="w-1.5 h-1.5 rounded-full" :style="{ backgroundColor: sig.color }" />
        {{ sig.label.split(' > ').pop() }}
        <template v-if="store.signalBounds[sig.senalId]">
          [{{ store.signalBounds[sig.senalId].min.toLocaleString('es-CO', { maximumFractionDigits: 2 }) }}
          →
          {{ store.signalBounds[sig.senalId].max.toLocaleString('es-CO', { maximumFractionDigits: 2 }) }}]
        </template>
      </span>
    </div>

    <!-- Chart -->
    <div class="relative h-[400px] p-4">
      <Line v-if="hasData" :data="chartData" :options="chartOptions" />
      <div v-else class="absolute inset-0 flex flex-col items-center justify-center gap-3 text-epm-gray-400">
        <svg viewBox="0 0 48 48" fill="none" width="48" height="48" class="opacity-30">
          <path d="M6 36l10-14 8 10 8-18 10 22" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"/>
        </svg>
        <p class="text-sm">Selecciona señales para visualizar</p>
        <p class="text-xs text-epm-gray-300">Con múltiples señales activa "Normalizar" para comparar</p>
      </div>
    </div>

    <!-- Selected signal chips -->
    <div v-if="store.selectedSignals.length > 0" class="flex flex-wrap gap-1.5 px-5 pb-3">
      <button
        v-for="sig in store.selectedSignals"
        :key="sig.senalId"
        class="inline-flex items-center gap-1.5 px-2.5 py-1 rounded-full text-[11px] font-mono font-medium border transition-colors hover:opacity-80"
        :style="{
          borderColor: sig.color,
          backgroundColor: sig.color + '12',
          color: sig.color,
        }"
        @click="store.removeSignal(sig.senalId)"
      >
        <span>{{ sig.label }}</span>
        <X :size="11" />
      </button>
    </div>
  </div>
</template>
