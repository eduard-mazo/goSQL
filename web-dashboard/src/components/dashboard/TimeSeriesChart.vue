<script setup lang="ts">
import { computed, ref, watch, onMounted } from 'vue'
import { Chart, registerables } from 'chart.js'
import 'chartjs-adapter-date-fns'
import { Line } from 'vue-chartjs'
import { useDashboardStore } from '@/stores/dashboard'
import { X, Maximize2, Minimize2 } from 'lucide-vue-next'

Chart.register(...registerables)

const store = useDashboardStore()
const activeRange = ref(7)

const hasData = computed(() => store.selectedSignals.length > 0)
const multiSignal = computed(() => store.selectedSignals.length > 1)

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
      backgroundColor: sig.color + '18',
      fill: store.selectedSignals.length === 1,
      pointRadius: values.length < 200 ? 2 : 0,
      pointHoverRadius: 4,
      borderWidth: 2,
      tension: 0.25,
    }
  }),
}))

const chartOptions = computed(() => ({
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
      backgroundColor: '#ffffff',
      titleColor: '#1a1a1a',
      bodyColor: '#6d6d6d',
      borderColor: '#e4e4e4',
      borderWidth: 1,
      titleFont: { family: "'JetBrains Mono', monospace", size: 12 },
      bodyFont: { family: "'JetBrains Mono', monospace", size: 11 },
      padding: 12,
      callbacks: {
        label(ctx: any) {
          const raw = ctx.raw?._raw
          const label = ctx.dataset.label || ''
          if (store.normalized && raw != null) {
            const pct = ctx.parsed.y?.toFixed(1)
            return `${label}: ${raw.toLocaleString('es-CO', { maximumFractionDigits: 4 })} (${pct}%)`
          }
          const val = ctx.parsed.y
          if (val == null) return `${label}: —`
          return `${label}: ${val.toLocaleString('es-CO', { maximumFractionDigits: 4 })}`
        },
      },
    },
  },
  scales: {
    x: {
      type: 'time' as const,
      time: {
        displayFormats: { hour: 'HH:mm', day: 'dd MMM', month: 'MMM yyyy' },
      },
      grid: { color: '#f0f0f0' },
      ticks: { color: '#8B8D8E', font: { family: "'JetBrains Mono', monospace", size: 10 }, maxRotation: 0 },
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
}))

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
    <div class="flex items-center justify-between px-5 py-3.5 border-b border-border flex-wrap gap-3">
      <div class="flex items-center gap-3">
        <h2 class="text-sm font-bold font-display text-foreground">Serie Temporal</h2>

        <!-- Normalize toggle -->
        <button
          v-if="multiSignal"
          class="inline-flex items-center gap-1.5 px-2.5 py-1 rounded-lg text-[11px] font-mono font-semibold border transition-all duration-150"
          :class="store.normalized
            ? 'bg-epm-forest text-white border-epm-forest shadow-sm'
            : 'border-epm-gray-200 text-epm-gray-500 hover:border-epm-gray-300 hover:text-epm-gray-700'"
          :title="store.normalized ? 'Mostrando valores normalizados 0–100%' : 'Normalizar para comparar señales de diferentes escalas'"
          @click="store.toggleNormalized()"
        >
          <component :is="store.normalized ? Minimize2 : Maximize2" :size="12" />
          {{ store.normalized ? 'Normalizado' : 'Normalizar' }}
        </button>
      </div>

      <div class="flex items-center gap-4 flex-wrap">
        <!-- Date pickers -->
        <div class="flex items-center gap-2 text-sm text-epm-gray-500">
          <label class="text-[11px]">Desde</label>
          <input
            v-model="dateFrom"
            type="date"
            class="font-mono text-xs bg-epm-gray-50 border border-border rounded-md px-2 py-1.5 text-foreground focus:border-epm-forest focus:outline-none"
            @change="onDateChange"
          />
          <label class="text-[11px]">Hasta</label>
          <input
            v-model="dateTo"
            type="date"
            class="font-mono text-xs bg-epm-gray-50 border border-border rounded-md px-2 py-1.5 text-foreground focus:border-epm-forest focus:outline-none"
            @change="onDateChange"
          />
        </div>
        <!-- Range buttons -->
        <div class="flex gap-0.5 bg-epm-gray-50 rounded-lg p-0.5">
          <button
            v-for="btn in rangeBtns"
            :key="btn.days"
            class="font-mono text-xs font-medium px-2.5 py-1.5 rounded-md transition-all"
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

    <!-- Normalization info banner -->
    <div
      v-if="store.normalized && hasData"
      class="px-5 py-2 bg-epm-forest-50 border-b border-epm-forest-200 flex items-center gap-4 text-[10px] font-mono"
    >
      <span class="text-epm-forest-700 font-semibold">MIN–MAX → 0–100%</span>
      <span
        v-for="sig in store.selectedSignals"
        :key="sig.senalId"
        class="inline-flex items-center gap-1 text-epm-gray-600"
      >
        <span class="w-1.5 h-1.5 rounded-full" :style="{ backgroundColor: sig.color }" />
        {{ sig.label.split(' > ').pop() }}:
        <template v-if="store.signalBounds[sig.senalId]">
          {{ store.signalBounds[sig.senalId].min.toLocaleString('es-CO', { maximumFractionDigits: 2 }) }}
          →
          {{ store.signalBounds[sig.senalId].max.toLocaleString('es-CO', { maximumFractionDigits: 2 }) }}
        </template>
      </span>
    </div>

    <!-- Chart -->
    <div class="relative h-[380px] p-4">
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
    <div v-if="store.selectedSignals.length > 0" class="flex flex-wrap gap-1.5 px-5 pb-4">
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
