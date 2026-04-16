<script setup lang="ts">
import { computed } from 'vue'
import { useDashboardStore } from '@/stores/dashboard'
import { Download } from 'lucide-vue-next'

const store = useDashboardStore()

const hasData = computed(() => store.selectedSignals.length > 0)

interface TableRow {
  fecha: string
  label: string
  valor: number | null
  unidades: string
}

const rows = computed<TableRow[]>(() => {
  const all: TableRow[] = []
  for (const sig of store.selectedSignals) {
    const values = store.chartData[sig.senalId] || []
    for (const v of values) {
      all.push({ fecha: v.fecha, label: sig.label, valor: v.valor, unidades: sig.unidades })
    }
  }
  all.sort((a, b) => new Date(b.fecha).getTime() - new Date(a.fecha).getTime())
  return all.slice(0, 500)
})

function formatFecha(iso: string): string {
  const d = new Date(iso)
  return d.toLocaleDateString('es-CO', { day: '2-digit', month: 'short', year: 'numeric' })
    + ' ' + d.toLocaleTimeString('es-CO', { hour: '2-digit', minute: '2-digit' })
}

function formatValor(v: number | null): string {
  if (v == null) return '—'
  if (Math.abs(v) >= 1000) return v.toLocaleString('es-CO', { maximumFractionDigits: 2 })
  return v.toLocaleString('es-CO', { maximumFractionDigits: 4 })
}

function exportCSV() {
  let csv = 'FECHA,SENAL,VALOR,UNIDADES\n'
  for (const sig of store.selectedSignals) {
    const values = store.chartData[sig.senalId] || []
    for (const v of values) {
      csv += `${v.fecha},"${sig.label}",${v.valor ?? ''},${sig.unidades}\n`
    }
  }
  const blob = new Blob([csv], { type: 'text/csv;charset=utf-8;' })
  const url = URL.createObjectURL(blob)
  const a = document.createElement('a')
  a.href = url
  a.download = `roc-monitor-${new Date().toISOString().slice(0, 10)}.csv`
  a.click()
  URL.revokeObjectURL(url)
}
</script>

<template>
  <div v-if="hasData" class="rounded-xl border border-border bg-white overflow-hidden">
    <!-- Header -->
    <div class="flex items-center justify-between px-6 py-4 border-b border-border">
      <h2 class="text-base font-bold font-display text-foreground">Datos</h2>
      <button
        class="inline-flex items-center gap-1.5 px-3 py-1.5 rounded-lg border border-border text-sm text-epm-gray-500 hover:text-epm-forest hover:border-epm-forest transition-colors"
        @click="exportCSV"
      >
        <Download :size="14" />
        Exportar CSV
      </button>
    </div>

    <!-- Table -->
    <div class="overflow-x-auto max-h-[400px] overflow-y-auto">
      <table class="w-full text-sm">
        <thead>
          <tr class="bg-epm-gray-50 sticky top-0 z-10">
            <th class="text-left px-4 py-2.5 text-[11px] font-bold uppercase tracking-widest text-epm-gray-500">Fecha</th>
            <th class="text-left px-4 py-2.5 text-[11px] font-bold uppercase tracking-widest text-epm-gray-500">Señal</th>
            <th class="text-right px-4 py-2.5 text-[11px] font-bold uppercase tracking-widest text-epm-gray-500">Valor</th>
            <th class="text-left px-4 py-2.5 text-[11px] font-bold uppercase tracking-widest text-epm-gray-500">Unidades</th>
          </tr>
        </thead>
        <tbody>
          <tr
            v-for="(row, i) in rows"
            :key="i"
            class="border-t border-epm-gray-100 hover:bg-epm-gray-50/50 transition-colors"
          >
            <td class="px-4 py-2 font-mono text-xs text-epm-gray-500 whitespace-nowrap">{{ formatFecha(row.fecha) }}</td>
            <td class="px-4 py-2 text-foreground">{{ row.label }}</td>
            <td class="px-4 py-2 font-mono font-semibold text-right text-epm-forest">{{ formatValor(row.valor) }}</td>
            <td class="px-4 py-2 text-epm-gray-400">{{ row.unidades || '—' }}</td>
          </tr>
        </tbody>
      </table>
    </div>
  </div>
</template>
