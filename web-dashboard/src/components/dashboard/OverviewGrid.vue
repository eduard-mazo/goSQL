<script setup lang="ts">
import { useDashboardStore } from '@/stores/dashboard'
import SignalCard from './SignalCard.vue'

const store = useDashboardStore()
</script>

<template>
  <div class="rounded-xl border border-border bg-white overflow-hidden">
    <!-- Header -->
    <div class="flex items-center justify-between px-6 py-4 border-b border-border">
      <h2 class="text-base font-bold font-display text-foreground">
        {{ store.selectedStation || 'Resumen General' }}
      </h2>
      <span class="text-xs font-mono text-epm-gray-400">
        {{ store.filteredOverview.length }} señales
      </span>
    </div>

    <!-- Grid grouped by station -->
    <div class="p-4 space-y-4">
      <div v-for="group in store.overviewGrouped" :key="group.name">
        <!-- Station group header (only in "all" view) -->
        <div
          v-if="!store.selectedStation"
          class="flex items-center gap-2 pb-2 mb-3 border-b border-epm-gray-100"
        >
          <span class="w-1.5 h-1.5 rounded-full bg-epm-forest" />
          <span class="text-xs font-bold uppercase tracking-widest text-epm-gray-500">
            {{ group.name }}
          </span>
          <span class="text-[10px] font-mono text-epm-gray-400">
            {{ group.signals.length }} señales
          </span>
        </div>

        <!-- Signal cards -->
        <div class="grid grid-cols-1 sm:grid-cols-2 xl:grid-cols-3 2xl:grid-cols-4 gap-3">
          <SignalCard
            v-for="row in group.signals"
            :key="row.senal_id"
            :row="row"
          />
        </div>
      </div>

      <!-- Empty state -->
      <div
        v-if="store.filteredOverview.length === 0"
        class="flex items-center justify-center py-12 text-epm-gray-400 text-sm"
      >
        No hay señales disponibles
      </div>
    </div>
  </div>
</template>
