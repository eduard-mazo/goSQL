// ═══════════════════════════════════════════════════════════════════════
//  ROC Monitor — Dashboard Application
// ═══════════════════════════════════════════════════════════════════════

const API_BASE = window.location.origin;

// Chart.js palette matching CSS variables
const CHART_COLORS = [
  '#00d4aa', '#58a6ff', '#d29922', '#bc8cff',
  '#f0883e', '#f85149', '#3fb950', '#79c0ff',
];

// ── STATE ─────────────────────────────────────────────────────────────

const state = {
  stations: [],      // StationDTO[]
  signals: [],       // SignalDTO[]
  overview: [],      // OverviewRow[]
  selectedStation: null,
  selectedSignals: [], // {senalID, label, color}[]
  chartData: {},     // senalID → ValueDTO[]
  rangeDays: 7,
  dateFrom: null,
  dateTo: null,
};

let mainChart = null;

// ── INIT ──────────────────────────────────────────────────────────────

document.addEventListener('DOMContentLoaded', async () => {
  startClock();
  initChart();
  bindEvents();
  await loadAll();
});

async function loadAll() {
  try {
    const [stations, overview, stats] = await Promise.all([
      fetchJSON('/api/stations'),
      fetchJSON('/api/overview'),
      fetchJSON('/api/stats'),
    ]);
    state.stations = stations;
    state.overview = overview;
    renderStats(stats);
    renderStations();
    renderOverview();
  } catch (err) {
    console.error('Failed to load:', err);
  }
}

// ── FETCH ─────────────────────────────────────────────────────────────

async function fetchJSON(path) {
  const res = await fetch(API_BASE + path);
  if (!res.ok) throw new Error(`${res.status} ${res.statusText}`);
  return res.json();
}

// ── CLOCK ─────────────────────────────────────────────────────────────

function startClock() {
  const el = document.getElementById('clock');
  function tick() {
    const now = new Date();
    el.textContent = now.toLocaleTimeString('es-CO', { hour: '2-digit', minute: '2-digit', second: '2-digit' });
  }
  tick();
  setInterval(tick, 1000);
}

// ── STATS ─────────────────────────────────────────────────────────────

function renderStats(stats) {
  document.getElementById('statStations').textContent = stats.station_count;
  document.getElementById('statSignals').textContent = stats.active_signals;
  document.getElementById('statValues').textContent = formatCount(stats.total_values);

  if (stats.min_fecha && stats.max_fecha) {
    const from = new Date(stats.min_fecha);
    const to = new Date(stats.max_fecha);
    const days = Math.round((to - from) / 86400000);
    document.getElementById('statRange').textContent = `${days}d`;
  }
}

function formatCount(n) {
  if (n >= 1_000_000) return (n / 1_000_000).toFixed(1) + 'M';
  if (n >= 1_000) return (n / 1_000).toFixed(1) + 'k';
  return String(n);
}

// ── STATIONS SIDEBAR ──────────────────────────────────────────────────

function renderStations() {
  const container = document.getElementById('stationList');
  container.innerHTML = '';

  // "All" item
  const allItem = createStationItem({
    name: 'Todas',
    signal_count: state.stations.reduce((s, st) => s + st.signal_count, 0),
    active_count: state.stations.reduce((s, st) => s + st.active_count, 0),
  }, null);
  if (state.selectedStation === null) allItem.classList.add('active');
  container.appendChild(allItem);

  for (const st of state.stations) {
    const item = createStationItem(st, st.name);
    if (state.selectedStation === st.name) item.classList.add('active');
    container.appendChild(item);
  }
}

function createStationItem(st, key) {
  const el = document.createElement('div');
  el.className = 'station-item';
  const dotClass = st.active_count === st.signal_count
    ? 'station-item__dot--active'
    : 'station-item__dot--partial';

  el.innerHTML = `
    <div class="station-item__dot ${dotClass}"></div>
    <div class="station-item__info">
      <div class="station-item__name">${st.name}</div>
      <div class="station-item__meta">${st.active_count}/${st.signal_count} activas</div>
    </div>
    <span class="station-item__count">${st.signal_count}</span>
  `;
  el.addEventListener('click', () => selectStation(key));
  return el;
}

function selectStation(stationName) {
  state.selectedStation = stationName;
  renderStations();
  renderOverview();

  if (stationName) {
    document.getElementById('panelTitle').textContent = stationName;
  } else {
    document.getElementById('panelTitle').textContent = 'Resumen General';
  }
}

// ── OVERVIEW GRID ─────────────────────────────────────────────────────

function renderOverview() {
  const container = document.getElementById('overviewGrid');
  container.innerHTML = '';

  let filtered = state.overview;
  if (state.selectedStation) {
    filtered = filtered.filter(r => r.b1 === state.selectedStation);
  }

  const groupBy = document.getElementById('groupBy').value;

  if (groupBy === 'station') {
    // Group by B1 (station)
    const groups = {};
    const groupOrder = [];
    for (const row of filtered) {
      const key = row.b1 || '?';
      if (!groups[key]) {
        groups[key] = [];
        groupOrder.push(key);
      }
      groups[key].push(row);
    }

    for (const station of groupOrder) {
      const rows = groups[station];
      if (!state.selectedStation) {
        // Show station header only in "all" view
        const header = document.createElement('div');
        header.className = 'station-group-header';
        header.innerHTML = `
          <span class="station-group-header__name">${station}</span>
          <span class="station-group-header__count">${rows.length} señales</span>
        `;
        container.appendChild(header);
      }
      for (const row of rows) {
        container.appendChild(createSignalCard(row));
      }
    }
  } else {
    // Group by element
    const groups = {};
    const groupOrder = [];
    for (const row of filtered) {
      const key = row.element || '?';
      if (!groups[key]) {
        groups[key] = [];
        groupOrder.push(key);
      }
      groups[key].push(row);
    }
    for (const element of groupOrder) {
      const rows = groups[element];
      const header = document.createElement('div');
      header.className = 'station-group-header';
      header.innerHTML = `
        <span class="station-group-header__name">${element}</span>
        <span class="station-group-header__count">${rows.length} señales</span>
      `;
      container.appendChild(header);
      for (const row of rows) {
        container.appendChild(createSignalCard(row));
      }
    }
  }
}

function createSignalCard(row) {
  const card = document.createElement('div');
  card.className = 'signal-card';

  // Assign accent color based on element type
  const colorIdx = hashCode(row.element) % CHART_COLORS.length;
  card.style.setProperty('--card-accent', CHART_COLORS[colorIdx]);

  const isSelected = state.selectedSignals.some(s => s.senalID === row.senal_id);
  if (isSelected) card.classList.add('selected');

  const valor = row.last_valor != null
    ? formatValor(row.last_valor)
    : null;
  const valorClass = valor != null ? '' : ' signal-card__value--null';
  const valorText = valor != null ? valor : 'Sin datos';

  let timestamp = '';
  if (row.last_fecha) {
    const d = new Date(row.last_fecha);
    timestamp = d.toLocaleDateString('es-CO', { day: '2-digit', month: 'short', year: 'numeric' })
      + ' ' + d.toLocaleTimeString('es-CO', { hour: '2-digit', minute: '2-digit' });
  }

  card.innerHTML = `
    <div class="signal-card__header">
      <span class="signal-card__element">${row.element}</span>
      <span class="signal-card__unit">${row.unidades || '—'}</span>
    </div>
    <div class="signal-card__path">${row.b1} › ${row.b2} › ${row.b3}</div>
    <div class="signal-card__value${valorClass}">${valorText}</div>
    <div class="signal-card__timestamp">${timestamp}</div>
  `;

  card.addEventListener('click', () => toggleSignal(row));
  return card;
}

function formatValor(v) {
  if (v == null) return null;
  // Smart formatting: show up to 4 decimals but strip trailing zeros
  if (Math.abs(v) >= 1000) return v.toLocaleString('es-CO', { maximumFractionDigits: 2 });
  if (Math.abs(v) >= 1) return v.toLocaleString('es-CO', { maximumFractionDigits: 4 });
  return v.toLocaleString('es-CO', { maximumFractionDigits: 6 });
}

// ── SIGNAL SELECTION & CHARTING ───────────────────────────────────────

async function toggleSignal(row) {
  const idx = state.selectedSignals.findIndex(s => s.senalID === row.senal_id);
  if (idx >= 0) {
    state.selectedSignals.splice(idx, 1);
    delete state.chartData[row.senal_id];
  } else {
    if (state.selectedSignals.length >= 8) {
      state.selectedSignals.shift(); // max 8 signals
    }
    const colorIdx = state.selectedSignals.length % CHART_COLORS.length;
    state.selectedSignals.push({
      senalID: row.senal_id,
      label: `${row.b1} › ${row.b3} › ${row.element}`,
      color: CHART_COLORS[colorIdx],
      unidades: row.unidades || '',
    });

    // Fetch data for this signal
    await loadSignalData(row.senal_id);
  }

  renderOverview();
  renderSelectedChips();
  updateChart();
  updateTable();
}

async function loadSignalData(senalID) {
  const { from, to } = getDateRange();
  let url = `/api/values?senal_id=${senalID}`;
  if (from && to) {
    url += `&from=${from.toISOString()}&to=${to.toISOString()}`;
  }
  try {
    state.chartData[senalID] = await fetchJSON(url);
  } catch (err) {
    console.error(`Failed to load signal ${senalID}:`, err);
    state.chartData[senalID] = [];
  }
}

async function reloadAllSignalData() {
  const promises = state.selectedSignals.map(s => loadSignalData(s.senalID));
  await Promise.all(promises);
  updateChart();
  updateTable();
}

function getDateRange() {
  // Custom date inputs override range buttons
  const fromInput = document.getElementById('dateFrom').value;
  const toInput = document.getElementById('dateTo').value;
  if (fromInput && toInput) {
    return {
      from: new Date(fromInput + 'T00:00:00Z'),
      to: new Date(toInput + 'T23:59:59Z'),
    };
  }

  if (state.rangeDays === 0) {
    return { from: null, to: null }; // all data
  }

  const to = new Date();
  const from = new Date();
  from.setDate(from.getDate() - state.rangeDays);
  return { from, to };
}

function renderSelectedChips() {
  const container = document.getElementById('selectedSignals');
  container.innerHTML = '';
  for (const sig of state.selectedSignals) {
    const chip = document.createElement('span');
    chip.className = 'signal-chip';
    chip.style.setProperty('--chip-color', sig.color);
    chip.innerHTML = `
      <span>${sig.label}</span>
      <span class="signal-chip__remove">×</span>
    `;
    chip.addEventListener('click', () => {
      // Find the overview row and toggle it off
      const row = state.overview.find(r => r.senal_id === sig.senalID);
      if (row) toggleSignal(row);
    });
    container.appendChild(chip);
  }
}

// ── CHART ─────────────────────────────────────────────────────────────

function initChart() {
  const ctx = document.getElementById('mainChart').getContext('2d');
  mainChart = new Chart(ctx, {
    type: 'line',
    data: { datasets: [] },
    options: {
      responsive: true,
      maintainAspectRatio: false,
      interaction: {
        mode: 'index',
        intersect: false,
      },
      plugins: {
        legend: {
          display: true,
          position: 'top',
          labels: {
            color: '#8b949e',
            font: { family: "'JetBrains Mono', monospace", size: 11 },
            boxWidth: 12,
            boxHeight: 2,
            padding: 16,
            usePointStyle: true,
            pointStyle: 'line',
          },
        },
        tooltip: {
          backgroundColor: '#161b22',
          titleColor: '#e6edf3',
          bodyColor: '#8b949e',
          borderColor: '#30363d',
          borderWidth: 1,
          titleFont: { family: "'JetBrains Mono', monospace", size: 12 },
          bodyFont: { family: "'JetBrains Mono', monospace", size: 11 },
          padding: 12,
          displayColors: true,
          callbacks: {
            title: (items) => {
              if (!items.length) return '';
              const d = new Date(items[0].parsed.x);
              return d.toLocaleDateString('es-CO', { day: '2-digit', month: 'short', year: 'numeric' })
                + '  ' + d.toLocaleTimeString('es-CO', { hour: '2-digit', minute: '2-digit' });
            },
            label: (item) => {
              const val = item.parsed.y != null ? formatValor(item.parsed.y) : 'null';
              return ` ${item.dataset.label}: ${val}`;
            },
          },
        },
      },
      scales: {
        x: {
          type: 'time',
          time: {
            tooltipFormat: 'PPpp',
            displayFormats: {
              hour: 'HH:mm',
              day: 'dd MMM',
              month: 'MMM yyyy',
            },
          },
          grid: { color: '#21262d', lineWidth: 0.5 },
          ticks: {
            color: '#484f58',
            font: { family: "'JetBrains Mono', monospace", size: 10 },
            maxRotation: 0,
          },
          border: { color: '#30363d' },
        },
        y: {
          grid: { color: '#21262d', lineWidth: 0.5 },
          ticks: {
            color: '#484f58',
            font: { family: "'JetBrains Mono', monospace", size: 10 },
          },
          border: { color: '#30363d' },
        },
      },
      elements: {
        point: { radius: 0, hitRadius: 8, hoverRadius: 4 },
        line: { tension: 0.2, borderWidth: 1.5 },
      },
      animation: {
        duration: 600,
        easing: 'easeOutQuart',
      },
    },
  });
}

function updateChart() {
  const emptyEl = document.getElementById('chartEmpty');
  const tablePanel = document.getElementById('tablePanel');

  if (state.selectedSignals.length === 0) {
    emptyEl.classList.remove('hidden');
    mainChart.data.datasets = [];
    mainChart.update();
    tablePanel.style.display = 'none';
    return;
  }
  emptyEl.classList.add('hidden');
  tablePanel.style.display = '';

  const datasets = state.selectedSignals.map((sig, i) => {
    const values = state.chartData[sig.senalID] || [];
    return {
      label: sig.label + (sig.unidades ? ` (${sig.unidades})` : ''),
      data: values.map(v => ({
        x: new Date(v.fecha).getTime(),
        y: v.valor,
      })),
      borderColor: sig.color,
      backgroundColor: sig.color + '18',
      fill: state.selectedSignals.length === 1,
      pointRadius: values.length < 200 ? 2 : 0,
      pointHoverRadius: 4,
    };
  });

  mainChart.data.datasets = datasets;
  mainChart.update();
}

// ── DATA TABLE ────────────────────────────────────────────────────────

function updateTable() {
  const tbody = document.getElementById('dataTableBody');
  tbody.innerHTML = '';

  if (state.selectedSignals.length === 0) return;

  // Merge all selected signals' data into one sorted array
  const allRows = [];
  for (const sig of state.selectedSignals) {
    const values = state.chartData[sig.senalID] || [];
    for (const v of values) {
      allRows.push({ ...v, label: sig.label, unidades: sig.unidades });
    }
  }

  // Sort by fecha descending, limit to 500
  allRows.sort((a, b) => new Date(b.fecha) - new Date(a.fecha));
  const limited = allRows.slice(0, 500);

  for (const row of limited) {
    const tr = document.createElement('tr');
    const d = new Date(row.fecha);
    const fechaStr = d.toLocaleDateString('es-CO', { day: '2-digit', month: 'short', year: 'numeric' })
      + ' ' + d.toLocaleTimeString('es-CO', { hour: '2-digit', minute: '2-digit' });
    const valorStr = row.valor != null ? formatValor(row.valor) : '—';

    tr.innerHTML = `
      <td>${fechaStr}</td>
      <td>${row.label}</td>
      <td>${valorStr}</td>
      <td>${row.unidades || '—'}</td>
    `;
    tbody.appendChild(tr);
  }
}

// ── EXPORT ────────────────────────────────────────────────────────────

function exportCSV() {
  if (state.selectedSignals.length === 0) return;

  const allRows = [];
  for (const sig of state.selectedSignals) {
    const values = state.chartData[sig.senalID] || [];
    for (const v of values) {
      allRows.push({
        fecha: v.fecha,
        senal: sig.label,
        valor: v.valor,
        unidades: sig.unidades,
      });
    }
  }

  allRows.sort((a, b) => new Date(a.fecha) - new Date(b.fecha));

  let csv = 'FECHA,SENAL,VALOR,UNIDADES\n';
  for (const row of allRows) {
    csv += `${row.fecha},"${row.senal}",${row.valor ?? ''},${row.unidades}\n`;
  }

  const blob = new Blob([csv], { type: 'text/csv;charset=utf-8;' });
  const url = URL.createObjectURL(blob);
  const a = document.createElement('a');
  a.href = url;
  a.download = `roc-monitor-${new Date().toISOString().slice(0, 10)}.csv`;
  a.click();
  URL.revokeObjectURL(url);
}

// ── EVENTS ────────────────────────────────────────────────────────────

function bindEvents() {
  document.getElementById('btnRefresh').addEventListener('click', loadAll);
  document.getElementById('groupBy').addEventListener('change', renderOverview);
  document.getElementById('btnExport').addEventListener('click', exportCSV);

  // Range buttons
  document.querySelectorAll('.range-btn').forEach(btn => {
    btn.addEventListener('click', async () => {
      document.querySelectorAll('.range-btn').forEach(b => b.classList.remove('active'));
      btn.classList.add('active');
      state.rangeDays = parseInt(btn.dataset.days, 10);
      // Clear custom dates
      document.getElementById('dateFrom').value = '';
      document.getElementById('dateTo').value = '';
      await reloadAllSignalData();
    });
  });

  // Custom date range
  const dateFrom = document.getElementById('dateFrom');
  const dateTo = document.getElementById('dateTo');
  const onDateChange = async () => {
    if (dateFrom.value && dateTo.value) {
      document.querySelectorAll('.range-btn').forEach(b => b.classList.remove('active'));
      await reloadAllSignalData();
    }
  };
  dateFrom.addEventListener('change', onDateChange);
  dateTo.addEventListener('change', onDateChange);
}

// ── UTILS ─────────────────────────────────────────────────────────────

function hashCode(str) {
  let hash = 0;
  for (let i = 0; i < (str || '').length; i++) {
    hash = ((hash << 5) - hash + str.charCodeAt(i)) | 0;
  }
  return Math.abs(hash);
}
