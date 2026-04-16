<script setup lang="ts">
import { computed, ref, watch, onMounted } from 'vue'
import { Chart, registerables } from 'chart.js'
import 'chartjs-adapter-date-fns'
import { Line } from 'vue-chartjs'
import { useDashboardStore } from '@/stores/dashboard'
import { X } from 'lucide-vue-next'

Chart.register(...registerables)

const store = useDashboardStore()
const activeRange = ref(7)

const hasData = computed(() => store.selectedSignals.length > 0)

const chartData = computed(() => ({
  datasets: store.selectedSignals.map(sig => {
    const values = store.chartData[sig.senalId] || []
    return {
      label: sig.label + (sig.unidades ? ` (${sig.unidades})` : ''),
      data: values.map(v => ({
        x: new Date(v.fecha).getTime(),
        y: v.valor,
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
      ticks: { color: '#8B8D8E', font: { family: "'JetBrains Mono', monospace", size: 10 } },
      border: { color: '#e4e4e4' },
    },
  },
  animation: { duration: 500 },
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

// Re-sync range when store range changes externally
watch(() => store.rangeDays, (v) => { activeRange.value = v })

onMounted(() => { activeRange.value = store.rangeDays })
</script>

<template>
  <div class="rounded-xl border border-border bg-white overflow-hidden">
    <!-- Header -->
    <div class="flex items-center justify-between px-6 py-4 border-b border-border flex-wrap gap-3">
      <h2 class="text-base font-bold font-display text-foreground">Serie Temporal</h2>
      <div class="flex items-center gap-4 flex-wrap">
        <!-- Date pickers -->
        <div class="flex items-center gap-2 text-sm text-epm-gray-500">
          <label>Desde</label>
          <input
            v-model="dateFrom"
            type="date"
            class="font-mono text-xs bg-epm-gray-50 border border-border rounded-md px-2 py-1.5 text-foreground focus:border-epm-forest focus:outline-none"
            @change="onDateChange"
          />
          <label>Hasta</label>
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

    <!-- Chart -->
    <div class="relative h-[380px] p-4">
      <Line v-if="hasData" :data="chartData" :options="chartOptions" />
      <div v-else class="absolute inset-0 flex flex-col items-center justify-center gap-3 text-epm-gray-400">
        <svg viewBox="0 0 48 48" fill="none" width="48" height="48" class="opacity-30">
          <path d="M6 36l10-14 8 10 8-18 10 22" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"/>
        </svg>
        <p class="text-sm">Selecciona una señal para visualizar</p>
      </div>
    </div>

    <!-- Signal chips -->
    <div v-if="store.selectedSignals.length > 0" class="flex flex-wrap gap-2 px-6 pb-4">
      <button
        v-for="sig in store.selectedSignals"
        :key="sig.senalId"
        class="inline-flex items-center gap-1.5 px-3 py-1 rounded-full text-xs font-mono font-medium border transition-colors hover:opacity-80"
        :style="{
          borderColor: sig.color,
          backgroundColor: sig.color + '12',
          color: sig.color,
        }"
        @click="store.removeSignal(sig.senalId)"
      >
        <span>{{ sig.label }}</span>
        <X :size="12" />
      </button>
    </div>
  </div>
</template>
