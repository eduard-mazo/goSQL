<script setup lang="ts">
import { ref, onMounted, onUnmounted } from 'vue'
import { useDashboardStore } from '@/stores/dashboard'
import EpmLogo from './EpmLogo.vue'

const store = useDashboardStore()

const clock = ref('')
let timer: ReturnType<typeof setInterval>

function tick() {
  clock.value = new Date().toLocaleTimeString('es-CO', {
    hour: '2-digit', minute: '2-digit', second: '2-digit',
  })
}

function formatCount(n: number): string {
  if (n >= 1_000_000) return (n / 1_000_000).toFixed(1) + 'M'
  if (n >= 1_000) return (n / 1_000).toFixed(1) + 'k'
  return String(n)
}

function coverageDays(): string {
  if (!store.stats?.min_fecha || !store.stats?.max_fecha) return '—'
  const from = new Date(store.stats.min_fecha)
  const to = new Date(store.stats.max_fecha)
  const days = Math.round((to.getTime() - from.getTime()) / 86400000)
  return `${days}d`
}

onMounted(() => {
  tick()
  timer = setInterval(tick, 1000)
})

onUnmounted(() => clearInterval(timer))
</script>

<template>
  <header class="sticky top-0 z-50 flex items-center justify-between gap-6 px-6 py-3 bg-white border-b border-border backdrop-blur-sm bg-white/95">
    <!-- Brand -->
    <div class="flex items-center gap-4 shrink-0">
      <EpmLogo :size="100" />
      <div class="hidden sm:block border-l border-epm-gray-200 pl-4">
        <h1 class="text-base font-bold font-display text-epm-forest-800 leading-tight">ROC Monitor</h1>
        <span class="text-xs text-epm-gray-500">Panel de Control</span>
      </div>
    </div>

    <!-- Stats pills -->
    <div class="hidden lg:flex items-center gap-6">
      <div class="flex flex-col items-center gap-0.5">
        <span class="font-mono text-lg font-bold text-epm-forest">{{ store.stats?.station_count ?? '—' }}</span>
        <span class="text-[10px] uppercase tracking-widest text-epm-gray-500">Estaciones</span>
      </div>
      <div class="flex flex-col items-center gap-0.5">
        <span class="font-mono text-lg font-bold text-epm-forest">{{ store.stats?.active_signals ?? '—' }}</span>
        <span class="text-[10px] uppercase tracking-widest text-epm-gray-500">Señales</span>
      </div>
      <div class="flex flex-col items-center gap-0.5">
        <span class="font-mono text-lg font-bold text-epm-forest">{{ store.stats ? formatCount(store.stats.total_values) : '—' }}</span>
        <span class="text-[10px] uppercase tracking-widest text-epm-gray-500">Registros</span>
      </div>
      <div class="flex flex-col items-center gap-0.5">
        <span class="font-mono text-lg font-bold text-epm-forest">{{ coverageDays() }}</span>
        <span class="text-[10px] uppercase tracking-widest text-epm-gray-500">Cobertura</span>
      </div>
    </div>

    <!-- Clock -->
    <div class="flex items-center gap-2 shrink-0">
      <span class="w-2 h-2 rounded-full bg-epm-citric animate-pulse" />
      <span class="font-mono text-sm text-epm-gray-500 min-w-[68px]">{{ clock }}</span>
    </div>
  </header>
</template>
