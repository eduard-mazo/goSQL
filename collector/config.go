package collector

import "goSQL/modbus"

// Config is the top-level structure of config.yaml.
type Config struct {
	Stations []StationConfig `yaml:"stations"`
}

// StationConfig holds the Modbus communication parameters for a ROC device.
// All signal definitions live inside each Medidor.
type StationConfig struct {
	Name               string            `yaml:"name"`                 // display name used in task key and logs
	IP                 string            `yaml:"ip"`
	Port               int               `yaml:"port"`
	ID                 byte              `yaml:"id"`                   // Modbus UnitID
	PtrEndian          modbus.Endianness `yaml:"ptr_endian,omitempty"` // endian for pointer register decode
	DBEndian           modbus.Endianness `yaml:"db_endian,omitempty"`  // endian for historical data decode
	DataRegistersCount uint16            `yaml:"data_registers_count"` // 1=uint16 ptr, 2=float32 ptr
	Medidores          []MedidorConfig   `yaml:"medidores"`
}

// MedidorConfig represents one independent circular buffer within a ROC device.
// Each medidor has its own pointer address, data base address, and signal list.
type MedidorConfig struct {
	Label          int               `yaml:"label"`
	Name           string            `yaml:"name"`             // e.g. "M1", used in task key suffix
	PointerAddress uint16            `yaml:"pointer_address"`
	DBAddress      uint16            `yaml:"base_data_address"`
	PtrEndian      modbus.Endianness `yaml:"ptr_endian,omitempty"` // overrides station-level when set
	DBEndian       modbus.Endianness `yaml:"db_endian,omitempty"`  // overrides station-level when set
	Signals        []SignalConfig    `yaml:"signals"`
}

// SignalConfig maps one float position in the Modbus record to a ROC_SENALES row.
//
// Flotante (3-10) is the 1-based index of the float32 value inside the 40-byte
// ROC hourly record:
//   - index 1 = date float  (MMDDYY) — handled internally, never stored here
//   - index 2 = time float  (HHMM)   — handled internally, never stored here
//   - index 3-10 = measurement signals
//
// The corresponding modes index is:  modes[Flotante-1]
//
// The unique Oracle key is the concatenation B1|B2|B3|Element.
type SignalConfig struct {
	Flotante    int    `yaml:"flotante"`     // float position in ROC record (3-10)
	B1          string `yaml:"b1"`           // station code  → ROC_SENALES.B1
	B2          string `yaml:"b2"`           // system code   → ROC_SENALES.B2
	B3          string `yaml:"b3"`           // element group → ROC_SENALES.B3
	Element     string `yaml:"element"`      // signal code   → ROC_SENALES.ELEMENT
	Descripcion string `yaml:"descripcion"`  // human-readable description (informational)
	Unidades    string `yaml:"unidades"`     // measurement units → ROC_SENALES.UNIDADES
}
