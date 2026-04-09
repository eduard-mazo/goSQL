package collector

import (
	"context"
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

type syncTask struct {
	Key                string // "STATION" or "STATION / M1"
	Station            string // station name → B1 in ROC_SENALES
	MeterName          string // meter name → B2 in ROC_SENALES; "" if no medidores
	IP                 string
	Port               int
	UnitID             byte
	PtrEndian          modbus.Endianness
	DBEndian           modbus.Endianness
	PtrAddr            uint16
	DBAddr             uint16
	DataRegistersCount uint16
	SignalNames        [8]string // signal names → B3 in ROC_SENALES; "" means unused channel
}

func expandTasks(stations []StationConfig) []syncTask {
	var tasks []syncTask
	for _, s := range stations {
		stPtrEndian := s.PtrEndian
		if stPtrEndian == "" {
			stPtrEndian = s.Endian
		}
		stDBEndian := s.DBEndian
		if stDBEndian == "" {
			stDBEndian = s.Endian
		}
		drc := s.DataRegistersCount
		if drc == 0 {
			drc = 1
		}

		stSigs := toFixed8(s.SignalNames)

		if len(s.Medidores) > 0 {
			for _, m := range s.Medidores {
				ptrEndian := stPtrEndian
				if m.PtrEndian != "" {
					ptrEndian = m.PtrEndian
				}
				dbEndian := stDBEndian
				if m.DBEndian != "" {
					dbEndian = m.DBEndian
				}
				sigs := stSigs
				if len(m.SignalNames) > 0 {
					sigs = toFixed8(m.SignalNames)
				}
				tasks = append(tasks, syncTask{
					Key:                fmt.Sprintf("%s / %s", s.Name, m.Name),
					Station:            s.Name,
					MeterName:          m.Name,
					IP:                 s.IP,
					Port:               s.Port,
					UnitID:             s.ID,
					PtrEndian:          ptrEndian,
					DBEndian:           dbEndian,
					PtrAddr:            m.PointerAddress,
					DBAddr:             m.DBAddress,
					DataRegistersCount: drc,
					SignalNames:        sigs,
				})
			}
		} else {
			tasks = append(tasks, syncTask{
				Key:                s.Name,
				Station:            s.Name,
				MeterName:          "",
				IP:                 s.IP,
				Port:               s.Port,
				UnitID:             s.ID,
				PtrEndian:          stPtrEndian,
				DBEndian:           stDBEndian,
				PtrAddr:            s.PointerAddress,
				DBAddr:             s.DBAddress,
				DataRegistersCount: drc,
				SignalNames:        stSigs,
			})
		}
	}
	return tasks
}

func toFixed8(names []string) [8]string {
	var out [8]string
	for i := 0; i < 8 && i < len(names); i++ {
		out[i] = names[i]
	}
	return out
}

// ─── Collector ────────────────────────────────────────────────────────────────

// Collector polls ROC stations via Modbus TCP and writes to Oracle ROC_VALORES.
type Collector struct {
	db        *db.DB
	senalRepo *repository.SenalRepository
	valorRepo *repository.ValorRepository
	tasks     []syncTask
	signalIDs map[string]float64 // "taskKey:signalIdx(0-7)" → SENAL_ID
}

// New creates a Collector by loading station configuration from configPath (YAML).
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

// EnsureSignals checks ROC_SENALES for every configured signal and inserts missing ones.
// It then populates the internal signalIDs cache used during sync.
// Safe to call multiple times; existing rows are never modified.
func (c *Collector) EnsureSignals(ctx context.Context) error {
	inserted := 0
	for _, t := range c.tasks {
		for j, sigName := range t.SignalNames {
			if sigName == "" {
				continue
			}

			existing, err := c.senalRepo.FindByKeys(ctx, t.Station, t.MeterName, sigName)
			if err != nil {
				return fmt.Errorf("FindByKeys %s sig[%d]: %w", t.Key, j, err)
			}

			var sid float64
			if existing != nil {
				sid = existing.SenalID
			} else {
				nextID, err := c.senalRepo.NextID(ctx)
				if err != nil {
					return fmt.Errorf("NextID: %w", err)
				}
				s := models.RocSenal{
					SenalID: nextID,
					B1:      models.S(t.Station),
					B3:      models.S(sigName),
					Activo:  "S",
				}
				if t.MeterName != "" {
					s.B2 = models.S(t.MeterName)
				}
				if err := c.senalRepo.Insert(ctx, s); err != nil {
					return fmt.Errorf("insert senal %s sig[%d]: %w", t.Key, j, err)
				}
				sid = nextID
				inserted++
				log.Printf("[collector] nueva señal ID=%.0f: %q / %q / %q", sid, t.Station, t.MeterName, sigName)
			}

			c.signalIDs[fmt.Sprintf("%s:%d", t.Key, j)] = sid
		}
	}
	log.Printf("[collector] EnsureSignals: %d insertadas, %d en caché", inserted, len(c.signalIDs))
	return nil
}

// ─── SyncAll ─────────────────────────────────────────────────────────────────

// SyncAll runs a full delta-sync for all configured stations concurrently.
// Max 2 concurrent connections per IP (same as the ROC device limit).
func (c *Collector) SyncAll(ctx context.Context) {
	if len(c.tasks) == 0 {
		return
	}
	log.Printf("[collector] iniciando sync — %d tarea(s)", len(c.tasks))

	ipSems := make(map[string]chan struct{})
	for _, t := range c.tasks {
		if _, ok := ipSems[t.IP]; !ok {
			ipSems[t.IP] = make(chan struct{}, 2)
		}
	}

	var wg sync.WaitGroup
	for _, t := range c.tasks {
		t := t
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

	// Get last stored FECHA for this task (use first valid SENAL_ID).
	var lastTS time.Time
	hasHistory := false
	for j := range task.SignalNames {
		sid, ok := c.signalIDs[fmt.Sprintf("%s:%d", task.Key, j)]
		if !ok {
			continue
		}
		t, found, err := c.valorRepo.MaxFechaBySenalID(ctx, sid)
		if err != nil {
			log.Printf("[%s] error leyendo max fecha: %v", task.Key, err)
			break
		}
		if found {
			lastTS = t
			hasHistory = true
		}
		break
	}

	// Connect to device.
	client := modbus.NewModbusClient(task.IP, task.Port, task.UnitID, task.DBEndian)
	if err := client.Connect(); err != nil {
		log.Printf("[%s] conexión fallida: %v", task.Key, err)
		return
	}
	defer client.Close()

	// Read current pointer register.
	currentPtr := -1
	ptrData, _, _, ptrErr := client.Execute(modbus.FCReadHoldingRegisters, task.PtrAddr, task.DataRegistersCount, nil)
	if ptrErr == nil && len(ptrData) == 4 {
		modes := modbus.DecodeAllModes(ptrData)
		if len(modes) > 0 {
			f := modes[0].Pick(task.PtrEndian)
			v := int(f)
			if f >= 0 && float32(v) == f && v < syncTotal {
				currentPtr = v
			}
		}
	}
	if currentPtr < 0 {
		log.Printf("[%s] no se pudo leer puntero (err=%v)", task.Key, ptrErr)
		return
	}

	// Read the record at currentPtr to get T_current for delta calculation.
	var currentPtrData []byte
	if d, _, _, err := client.Execute(modbus.FCReadHoldingRegisters, task.DBAddr, uint16(currentPtr), nil); err == nil {
		currentPtrData = d
	}

	// Compute which pointers to fetch.
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

	// Fetch records sequentially.
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

		modes := modbus.DecodeAllModes(data)
		fecha, hora, _, ok := modbus.DecodeROCDateTime(modes, task.DBEndian)
		if !ok {
			continue
		}

		recordTime, err := time.ParseInLocation("2006-01-02 15:04", fecha+" "+hora, time.Local)
		if err != nil {
			continue
		}

		// Signals are at modes[2..9] (modes[0]=date float, modes[1]=time float).
		for j := 0; j < 8; j++ {
			if task.SignalNames[j] == "" {
				continue
			}
			sid, ok := c.signalIDs[fmt.Sprintf("%s:%d", task.Key, j)]
			if !ok {
				continue
			}
			modeIdx := j + 2
			if modeIdx >= len(modes) {
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

// timeDeltaPtrs calculates which circular-buffer pointers are newer than lastTS.
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
	for i := 0; i < deltaHours; i++ {
		ptrs[i] = (currentPtr - deltaHours + 1 + i + syncTotal*10) % syncTotal
	}
	return ptrs
}
