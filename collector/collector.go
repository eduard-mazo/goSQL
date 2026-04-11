package collector

import (
	"context"
	"encoding/binary"
	"fmt"
	"log"
	"os"
	"sort"
	"sync"
	"time"

	"gopkg.in/yaml.v3"

	"goSQL/db"
	"goSQL/modbus"
	"goSQL/models"
	"goSQL/repository"
)

const syncTotal = 840

// ─── syncTask ─────────────────────────────────────────────────────────────────

// syncTask represents one independent Modbus polling operation:
// one (IP, Port, UnitID, PointerAddress, DBAddress) → one circular buffer → N signals.
type syncTask struct {
	Key                string            // "STATION / M1" — used in logs and signalIDs map
	IP                 string
	Port               int
	UnitID             byte
	PtrEndian          modbus.Endianness
	DBEndian           modbus.Endianness
	PtrAddr            uint16
	DBAddr             uint16
	DataRegistersCount uint16 // 1 → uint16 pointer (LLANOS), 2 → float32 pointer (rest)
	Signals            []SignalConfig
}

func expandTasks(stations []StationConfig) []syncTask {
	var tasks []syncTask
	for _, s := range stations {
		stPtrEndian := s.PtrEndian
		stDBEndian := s.DBEndian
		drc := s.DataRegistersCount
		if drc == 0 {
			drc = 2
		}

		for _, m := range s.Medidores {
			ptrEndian := stPtrEndian
			if m.PtrEndian != "" {
				ptrEndian = m.PtrEndian
			}
			dbEndian := stDBEndian
			if m.DBEndian != "" {
				dbEndian = m.DBEndian
			}
			tasks = append(tasks, syncTask{
				Key:                fmt.Sprintf("%s / %s", s.Name, m.Name),
				IP:                 s.IP,
				Port:               s.Port,
				UnitID:             s.ID,
				PtrEndian:          ptrEndian,
				DBEndian:           dbEndian,
				PtrAddr:            m.PointerAddress,
				DBAddr:             m.DBAddress,
				DataRegistersCount: drc,
				Signals:            m.Signals,
			})
		}
	}
	return tasks
}

// ─── Collector ────────────────────────────────────────────────────────────────

// Collector polls ROC stations via Modbus TCP and writes hourly values to Oracle.
type Collector struct {
	db        *db.DB
	senalRepo *repository.SenalRepository
	valorRepo *repository.ValorRepository
	tasks     []syncTask
	// signalIDs maps "taskKey:flotante" → SENAL_ID.
	// Populated by EnsureSignals; read-only after that.
	signalIDs map[string]float64
}

// New creates a Collector from config.yaml at configPath.
func New(database *db.DB, configPath string) (*Collector, error) {
	data, err := os.ReadFile(configPath)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", configPath, err)
	}
	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parse %s: %w", configPath, err)
	}

	return &Collector{
		db:        database,
		senalRepo: repository.NewSenalRepository(database),
		valorRepo: repository.NewValorRepository(database),
		tasks:     expandTasks(cfg.Stations),
		signalIDs: make(map[string]float64),
	}, nil
}

// ─── EnsureSignals ────────────────────────────────────────────────────────────

// uniqueKey builds the Oracle lookup key for a signal: "B1|B2|B3|Element".
func uniqueKey(b1, b2, b3, element string) string {
	return fmt.Sprintf("%s|%s|%s|%s", b1, b2, b3, element)
}

// EnsureSignals performs a find-or-create for every signal declared in config.yaml:
//
//  1. Loads ALL existing ROC_SENALES rows into an in-memory map (1 query).
//  2. Gets the current MAX(SENAL_ID) to seed a local counter (1 query).
//  3. For each config signal:
//     - Key = B1|B2|B3|Element (unique composite key).
//     - Hit: reuse existing SENAL_ID.
//     - Miss: INSERT new row with next available ID, increment counter.
//  4. Populates signalIDs cache:  "taskKey:flotante" → SENAL_ID.
//
// Single-threaded at startup; not safe for concurrent callers.
func (c *Collector) EnsureSignals(ctx context.Context) error {
	// Step 1 — load all existing signals.
	existing, err := c.senalRepo.FindAllMap(ctx)
	if err != nil {
		return fmt.Errorf("EnsureSignals/FindAllMap: %w", err)
	}

	// Step 2 — get the next available ID (local counter avoids repeated MAX queries).
	nextID, err := c.senalRepo.NextID(ctx)
	if err != nil {
		return fmt.Errorf("EnsureSignals/NextID: %w", err)
	}

	// Step 3 & 4 — process each configured signal.
	inserted := 0
	for _, t := range c.tasks {
		for _, sig := range t.Signals {
			ukey := uniqueKey(sig.B1, sig.B2, sig.B3, sig.Element)

			sid, found := existing[ukey]
			if !found {
				row := models.RocSenal{
					SenalID:  nextID,
					B1:       models.S(sig.B1),
					B2:       models.S(sig.B2),
					B3:       models.S(sig.B3),
					Element:  models.S(sig.Element),
					Unidades: models.S(sig.Unidades),
					Activo:   "S",
				}
				if err := c.senalRepo.Insert(ctx, row); err != nil {
					return fmt.Errorf("EnsureSignals/Insert %s: %w", ukey, err)
				}
				log.Printf("[collector] nueva señal ID=%.0f: %s", nextID, ukey)
				sid = nextID
				existing[ukey] = sid
				nextID++
				inserted++
			}

			// Cache: taskKey:flotante → SENAL_ID
			c.signalIDs[fmt.Sprintf("%s:%d", t.Key, sig.Flotante)] = sid
		}
	}

	log.Printf("[collector] EnsureSignals: %d insertadas, %d señales en caché", inserted, len(c.signalIDs))
	return nil
}

// ─── SyncAll ─────────────────────────────────────────────────────────────────

// SyncAll runs a delta-sync for all configured tasks concurrently.
// Max 2 simultaneous connections per IP (ROC device limit).
func (c *Collector) SyncAll(ctx context.Context) {
	if len(c.tasks) == 0 {
		return
	}
	log.Printf("[collector] iniciando sync — %d tarea(s)", len(c.tasks))

	// Per-IP semaphore: max 2 concurrent connections.
	ipSems := make(map[string]chan struct{})
	for _, t := range c.tasks {
		if _, ok := ipSems[t.IP]; !ok {
			ipSems[t.IP] = make(chan struct{}, 2)
		}
	}

	var wg sync.WaitGroup
	for _, t := range c.tasks {
		wg.Add(1)
		go func() {
			defer wg.Done()
			defer func() {
				if r := recover(); r != nil {
					log.Printf("[%s] PANIC: %v", t.Key, r)
				}
			}()
			sem := ipSems[t.IP]
			sem <- struct{}{}
			defer func() { <-sem }()
			c.syncStation(ctx, t)
		}()
	}
	wg.Wait()
	log.Printf("[collector] sync completado")
}

// ─── syncStation ─────────────────────────────────────────────────────────────

func (c *Collector) syncStation(ctx context.Context, task syncTask) {
	start := time.Now()

	// Determine the last stored timestamp for this task using the first valid signal.
	var lastTS time.Time
	hasHistory := false
	for _, sig := range task.Signals {
		sid, ok := c.signalIDs[fmt.Sprintf("%s:%d", task.Key, sig.Flotante)]
		if !ok {
			continue
		}
		t, found, err := c.valorRepo.MaxFechaBySenalID(ctx, sid)
		if err != nil {
			log.Printf("[%s] error leyendo MAX(FECHA): %v", task.Key, err)
			break
		}
		if found {
			lastTS = t
			hasHistory = true
		}
		break
	}

	// Connect to the ROC device.
	client := modbus.NewModbusClient(task.IP, task.Port, task.UnitID, task.DBEndian)
	if err := client.Connect(); err != nil {
		log.Printf("[%s] conexión fallida: %v", task.Key, err)
		return
	}
	defer client.Close()

	// Read the pointer register.
	// DataRegistersCount=1 → uint16 big-endian (2 bytes)
	// DataRegistersCount=2 → float32 with PtrEndian (4 bytes)
	currentPtr := -1
	ptrData, _, _, ptrErr := client.Execute(modbus.FCReadHoldingRegisters, task.PtrAddr, task.DataRegistersCount, nil)
	if ptrErr == nil {
		switch len(ptrData) {
		case 4: // float32 pointer
			modes := modbus.DecodeAllModes(ptrData)
			if len(modes) > 0 {
				f := modes[0].Pick(task.PtrEndian)
				v := int(f)
				if f >= 0 && float32(v) == f && v < syncTotal {
					currentPtr = v
				}
			}
		case 2: // uint16 big-endian pointer
			v := int(binary.BigEndian.Uint16(ptrData))
			if v >= 0 && v < syncTotal {
				currentPtr = v
			}
		}
	}
	if currentPtr < 0 {
		log.Printf("[%s] no se pudo leer puntero (err=%v)", task.Key, ptrErr)
		return
	}

	// Read the record at currentPtr to determine T_current for delta calculation.
	var currentPtrData []byte
	if d, _, _, err := client.Execute(modbus.FCReadHoldingRegisters, task.DBAddr, uint16(currentPtr), nil); err == nil {
		currentPtrData = d
	}

	// Compute which circular-buffer slots to fetch.
	var ptrs []int
	if hasHistory && len(currentPtrData) > 0 {
		ptrs = timeDeltaPtrs(lastTS, currentPtr, currentPtrData, task.DBEndian)
	} else {
		ptrs = allPtrs()
	}
	sort.Ints(ptrs)

	if len(ptrs) == 0 {
		log.Printf("[%s] al día (ptr=%d)", task.Key, currentPtr)
		return
	}
	log.Printf("[%s] fetching %d registros (ptr=%d)", task.Key, len(ptrs), currentPtr)

	// Fetch records and build the Oracle batch.
	polledAt := time.Now()
	var batch []models.RocValor

	for _, p := range ptrs {
		var data []byte
		if p == currentPtr && len(currentPtrData) > 0 {
			data = currentPtrData
		} else {
			d, _, _, err := client.Execute(modbus.FCReadHoldingRegisters, task.DBAddr, uint16(p), nil)
			if err != nil {
				log.Printf("[%s] ptr=%d err: %v", task.Key, p, err)
				continue
			}
			data = d
		}
		if len(data) == 0 {
			continue
		}

		// Decode all 10 float32 values from the 40-byte record.
		modes := modbus.DecodeAllModes(data)
		fecha, hora, _, ok := modbus.DecodeROCDateTime(modes, task.DBEndian)
		if !ok {
			continue
		}
		recordTime, err := time.ParseInLocation("2006-01-02 15:04", fecha+" "+hora, time.Local)
		if err != nil {
			continue
		}

		// Map each signal's Flotante to its mode index and SENAL_ID.
		for _, sig := range task.Signals {
			sid, ok := c.signalIDs[fmt.Sprintf("%s:%d", task.Key, sig.Flotante)]
			if !ok {
				continue
			}
			modeIdx := sig.Flotante - 1 // flotante=3 → modes[2], flotante=10 → modes[9]
			if modeIdx < 2 || modeIdx >= len(modes) {
				continue
			}
			val := float64(modbus.SanitizeFloat(modes[modeIdx].Pick(task.DBEndian)))
			batch = append(batch, models.RocValor{
				Fecha:    recordTime,
				SyncedAt: polledAt,
				SenalID:  sid,
				Valor:    models.F(val),
			})
		}
	}

	if len(batch) == 0 {
		log.Printf("[%s] sin valores nuevos", task.Key)
		return
	}

	if err := c.valorRepo.UpsertBatch(ctx, batch); err != nil {
		log.Printf("[%s] error insertando %d valores: %v", task.Key, len(batch), err)
		return
	}
	log.Printf("[%s] %d valores escritos en %.1fs (ptr=%d)",
		task.Key, len(batch), time.Since(start).Seconds(), currentPtr)
}

// ─── Delta helpers ────────────────────────────────────────────────────────────

func allPtrs() []int {
	ptrs := make([]int, syncTotal)
	for i := range ptrs {
		ptrs[i] = i
	}
	return ptrs
}

// timeDeltaPtrs returns the circular-buffer slots that are newer than lastTS.
func timeDeltaPtrs(lastTS time.Time, currentPtr int, currentData []byte, endian modbus.Endianness) []int {
	modes := modbus.DecodeAllModes(currentData)
	_, _, currentUnix, ok := modbus.DecodeROCDateTime(modes, endian)
	if !ok || currentUnix <= 0 {
		return allPtrs()
	}
	deltaHours := int((currentUnix - lastTS.Unix()) / 3600)
	if deltaHours <= 0 {
		return nil
	}
	if deltaHours >= syncTotal {
		return allPtrs()
	}
	ptrs := make([]int, deltaHours)
	for i := range deltaHours {
		ptrs[i] = (currentPtr - deltaHours + 1 + i + syncTotal*10) % syncTotal
	}
	return ptrs
}

