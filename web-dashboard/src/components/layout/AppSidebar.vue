<script setup lang="ts">
import { useDashboardStore } from '@/stores/dashboard'
import { RefreshCw } from 'lucide-vue-next'

const store = useDashboardStore()

function totalSignals(): number {
  return store.stations.reduce((s, st) => s + st.signal_count, 0)
}

function totalActive(): number {
  return store.stations.reduce((s, st) => s + st.active_count, 0)
}
</script>

<template>
  <aside class="hidden lg:flex w-64 flex-col border-r border-border bg-white shrink-0">
    <!-- Header -->
    <div class="flex items-center justify-between px-4 py-3 border-b border-border">
      <h2 class="text-xs font-bold uppercase tracking-widest text-epm-gray-500">Estaciones</h2>
      <button
        class="w-7 h-7 flex items-center justify-center rounded-md border border-border text-epm-gray-500 hover:text-epm-forest hover:border-epm-forest transition-colors"
        title="Actualizar"
        @click="store.loadAll()"
      >
        <RefreshCw :size="14" />
      </button>
    </div>

    <!-- Station list -->
    <div class="flex-1 overflow-y-auto p-2 space-y-0.5">
      <!-- All -->
      <button
        class="w-full flex items-center gap-2 px-3 py-2.5 rounded-lg text-left transition-all"
        :class="store.selectedStation === null
          ? 'bg-epm-forest/10 border border-epm-forest/30 text-epm-forest-800'
          : 'border border-transparent hover:bg-epm-gray-50 text-foreground'"
        @click="store.selectStation(null)"
      >
        <span class="w-2 h-2 rounded-full bg-epm-citric shrink-0" />
        <span class="flex-1 text-sm font-semibold truncate">Todas</span>
        <span class="font-mono text-xs text-epm-gray-500 bg-epm-gray-50 px-2 py-0.5 rounded-full">
          {{ totalActive() }}/{{ totalSignals() }}
        </span>
      </button>

      <!-- Each station -->
      <button
        v-for="st in store.stations"
        :key="st.name"
        class="w-full flex items-center gap-2 px-3 py-2.5 rounded-lg text-left transition-all"
        :class="store.selectedStation === st.name
          ? 'bg-epm-forest/10 border border-epm-forest/30 text-epm-forest-800'
          : 'border border-transparent hover:bg-epm-gray-50 text-foreground'"
        @click="store.selectStation(st.name)"
      >
        <span
          class="w-2 h-2 rounded-full shrink-0"
          :class="st.active_count === st.signal_count ? 'bg-epm-citric' : 'bg-amber-400'"
        />
        <div class="flex-1 min-w-0">
          <div class="text-sm font-semibold truncate">{{ st.name }}</div>
          <div class="text-[11px] font-mono text-epm-gray-400">{{ st.active_count }}/{{ st.signal_count }} activas</div>
        </div>
        <span class="font-mono text-xs text-epm-gray-500 bg-epm-gray-50 px-2 py-0.5 rounded-full">
          {{ st.signal_count }}
        </span>
      </button>
    </div>
  </aside>
</template>
