package modbus

import (
	"fmt"
	"math"
	"time"
)

// Float32Modes holds one float32 value decoded under all four endianness conventions.
type Float32Modes struct {
	ABCD float32 // Big-Endian
	DCBA float32 // Little-Endian
	CDAB float32 // Word-Swap (ROC default)
	BADC float32 // Byte-Swap
}

// Pick returns the float32 value for the given endianness.
func (f *Float32Modes) Pick(endian Endianness) float32 {
	switch endian {
	case LittleEndian:
		return f.DCBA
	case WordSwapped:
		return f.CDAB
	case ByteSwapped:
		return f.BADC
	default:
		return f.ABCD
	}
}

// SanitizeFloat replaces NaN or Inf with 0.
func SanitizeFloat(v float32) float32 {
	if math.IsNaN(float64(v)) || math.IsInf(float64(v), 0) {
		return 0
	}
	return v
}

// DecodeAllModes decodes each 4-byte group in data under all four endianness modes.
func DecodeAllModes(data []byte) []Float32Modes {
	out := make([]Float32Modes, 0, len(data)/4)
	for i := 0; i+4 <= len(data); i += 4 {
		b := data[i : i+4]
		out = append(out, Float32Modes{
			ABCD: math.Float32frombits(uint32(b[0])<<24 | uint32(b[1])<<16 | uint32(b[2])<<8 | uint32(b[3])),
			DCBA: math.Float32frombits(uint32(b[3])<<24 | uint32(b[2])<<16 | uint32(b[1])<<8 | uint32(b[0])),
			CDAB: math.Float32frombits(uint32(b[2])<<24 | uint32(b[3])<<16 | uint32(b[0])<<8 | uint32(b[1])),
			BADC: math.Float32frombits(uint32(b[1])<<24 | uint32(b[0])<<16 | uint32(b[3])<<8 | uint32(b[2])),
		})
	}
	return out
}

// DecodeROCDateTime reads the first two Float32Modes entries as ROC date and time.
//
// ROC encodes date as a float whose integer value follows the MMDDYY pattern:
//
//	dateFloat = MM*10000 + DD*100 + YY   (e.g. 030524.0 = March 05, 2024)
//
// Time is encoded as:
//
//	timeFloat = HH*100 + MM             (e.g. 1145.0 = 11:45)
//
// Returns (fecha "YYYY-MM-DD", hora "HH:MM", unix-seconds, ok).
// ok is false if the decoded values are out of plausible range.
func DecodeROCDateTime(modes []Float32Modes, endian Endianness) (fecha, hora string, ts int64, ok bool) {
	if len(modes) < 2 {
		return
	}
	dateVal := modes[0].Pick(endian)
	timeVal := modes[1].Pick(endian)

	dv := int(math.Round(float64(dateVal)))
	tv := int(math.Round(float64(timeVal)))

	month := dv / 10000
	day := (dv % 10000) / 100
	year := dv%100 + 2000
	hour := tv / 100
	minute := tv % 100

	if month < 1 || month > 12 || day < 1 || day > 31 ||
		year < 2000 || year > 2099 || hour > 23 || minute > 59 {
		return
	}

	t := time.Date(year, time.Month(month), day, hour, minute, 0, 0, time.Local)
	fecha = t.Format("2006-01-02")
	hora = fmt.Sprintf("%02d:%02d", hour, minute)
	ts = t.Unix()
	ok = true
	return
}
