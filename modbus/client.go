package modbus

import (
	"fmt"
	"log"
	"net"
	"time"
)

// ModbusClient manages a single Modbus TCP connection.
type ModbusClient struct {
	Host          string
	Port          int
	UnitID        byte
	Endian        Endianness
	Timeout       time.Duration
	conn          net.Conn
	transactionID uint16
}

func NewModbusClient(host string, port int, unitID byte, endian Endianness) *ModbusClient {
	return &ModbusClient{
		Host:    host,
		Port:    port,
		UnitID:  unitID,
		Endian:  endian,
		Timeout: 15 * time.Second,
	}
}

func (c *ModbusClient) Connect() error {
	address := net.JoinHostPort(c.Host, fmt.Sprintf("%d", c.Port))
	conn, err := net.DialTimeout("tcp", address, 15*time.Second)
	if err != nil {
		return fmt.Errorf("TCP connect %s: %w", address, err)
	}
	c.conn = conn
	return nil
}

func (c *ModbusClient) Close() {
	if c.conn != nil {
		c.conn.Close()
		c.conn = nil
	}
}

// Execute sends a FC03 (Read Holding Registers) request and returns the data payload.
// Returns (data, sentBytes, elapsed, error). data is the payload after MBAP+FC+ByteCount headers.
func (c *ModbusClient) Execute(fc byte, addr uint16, qty uint16, _ []byte) ([]byte, []byte, time.Duration, error) {
	if c.conn == nil {
		return nil, nil, 0, fmt.Errorf("not connected")
	}

	pdu := buildPDU(fc, addr, qty)
	req := buildMBAP(c.transactionID, c.UnitID, pdu)

	c.conn.SetDeadline(time.Now().Add(c.Timeout))
	start := time.Now()

	if _, err := c.conn.Write(req); err != nil {
		return nil, req, 0, fmt.Errorf("TX error FC=0x%02X addr=%d: %w", fc, addr, err)
	}

	buf := make([]byte, 4096)
	n, err := c.conn.Read(buf)
	elapsed := time.Since(start)
	if err != nil {
		return nil, req, elapsed, fmt.Errorf("RX error FC=0x%02X addr=%d [%dms]: %w", fc, addr, elapsed.Milliseconds(), err)
	}

	resp := buf[:n]
	if n < 8 {
		return nil, req, elapsed, fmt.Errorf("incomplete response FC=0x%02X: %d bytes (min 8)", fc, n)
	}

	// Check exception: high bit of FC byte set
	if resp[7] >= 0x80 {
		code := byte(0)
		if n > 8 {
			code = resp[8]
		}
		desc := ModbusExceptionDesc[code]
		if desc == "" {
			desc = "unknown error"
		}
		log.Printf("[modbus] exception 0x%02X addr=%d: %s | TX: %X | RX: %X", code, addr, desc, req, resp)
		return nil, req, elapsed, fmt.Errorf("exception 0x%02X: %s", code, desc)
	}

	c.transactionID++

	// Data payload starts after MBAP(7) + FC(1) + ByteCount(1) = byte 9
	if n >= 9 {
		return resp[9:n], req, elapsed, nil
	}
	return resp[8:n], req, elapsed, nil
}
