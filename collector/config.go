package collector

import "goSQL/modbus"

// Config is the top-level structure of config.yaml.
type Config struct {
	Stations []StationConfig `yaml:"stations"`
}

// StationConfig describes a pre-configured ROC device.
// If Medidores is non-empty, per-medidor addresses take precedence over
// the station-level PointerAddress / DBAddress.
// PtrEndian / DBEndian allow different endianness for pointer vs. data registers;
// both fall back to Endian when not set.
type StationConfig struct {
	Name               string            `yaml:"name"`
	IP                 string            `yaml:"ip"`
	Port               int               `yaml:"port"`
	ID                 byte              `yaml:"id"`
	Endian             modbus.Endianness `yaml:"endian,omitempty"`
	PtrEndian          modbus.Endianness `yaml:"ptr_endian,omitempty"`
	DBEndian           modbus.Endianness `yaml:"db_endian,omitempty"`
	PointerAddress     uint16            `yaml:"pointer_address"`
	DBAddress          uint16            `yaml:"base_data_address"`
	DataRegistersCount uint16            `yaml:"data_registers_count"`
	DataType           string            `yaml:"data_type,omitempty"`
	Medidores          []MedidorConfig   `yaml:"medidores,omitempty"`
	SignalNames        []string          `yaml:"signal_names,omitempty"`
}

// MedidorConfig describes a single meter (medidor) within a station.
// Used when one ROC device contains multiple independent flow meters,
// each with its own circular-buffer base address and/or pointer register.
type MedidorConfig struct {
	Label          int               `yaml:"label"`
	Name           string            `yaml:"name"`
	PointerAddress uint16            `yaml:"pointer_address"`
	DBAddress      uint16            `yaml:"base_data_address"`
	PtrEndian      modbus.Endianness `yaml:"ptr_endian,omitempty"`
	DBEndian       modbus.Endianness `yaml:"db_endian,omitempty"`
	SignalNames    []string          `yaml:"signal_names,omitempty"`
}
