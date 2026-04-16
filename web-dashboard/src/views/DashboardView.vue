<script setup lang="ts">
import { onMounted } from 'vue'
import { useDashboardStore } from '@/stores/dashboard'
import OverviewGrid from '@/components/dashboard/OverviewGrid.vue'
import TimeSeriesChart from '@/components/dashboard/TimeSeriesChart.vue'
import DataTable from '@/components/dashboard/DataTable.vue'

const store = useDashboardStore()

onMounted(() => {
  store.loadAll()
})
</script>

<template>
  <div class="flex flex-col gap-6 p-6">
    <!-- Loading state -->
    <div
      v-if="store.loading && store.overview.length === 0"
      class="flex items-center justify-center py-24 text-epm-gray-400"
    >
      <svg class="animate-spin h-6 w-6 mr-3 text-epm-forest" viewBox="0 0 24 24">
        <circle class="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" stroke-width="4" fill="none" />
        <path class="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4z" />
      </svg>
      Cargando datos...
    </div>

    <!-- Content -->
    <template v-else>
      <OverviewGrid />
      <TimeSeriesChart />
      <DataTable />
    </template>
  </div>
</template>
