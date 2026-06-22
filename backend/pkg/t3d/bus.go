package t3d

import (
	"fmt"
	"sync"
	"time"

	"github.com/goburrow/modbus"
)

// busTransport abstracts RS-485 bus operations so Motor can be tested without
// a real serial port. *Bus is the production implementation.
type busTransport interface {
	tx(slaveID byte, fn func(modbus.Client) error) error
	txRaw(slaveID, fc byte, data []byte) ([]byte, error)
}

// Bus owns a single RS-485 serial port shared by all motors on the network.
// Create one Bus per port; then create Motor instances that reference it.
// All motor operations serialize through Bus.mu — only one Modbus frame is
// in flight at a time, which is the correct behavior for a shared RS-485 bus.
type Bus struct {
	mu      sync.Mutex
	handler *modbus.RTUClientHandler
	client  modbus.Client
}

// NewBus creates a Bus for the given port and baud rate.
// Parity is fixed at 8E1 — the T3D factory default (P-183=1).
// Call Connect before creating Motor instances or using Driver.
func NewBus(port string, baud int) *Bus {
	h := modbus.NewRTUClientHandler(port)
	h.BaudRate = baud
	h.DataBits = 8
	h.Parity = "E"
	h.StopBits = 1
	h.Timeout = 500 * time.Millisecond
	return &Bus{handler: h}
}

// Connect opens the serial port. Must be called before any motor operations.
func (b *Bus) Connect() error {
	if err := b.handler.Connect(); err != nil {
		return fmt.Errorf("t3d bus: connect %s: %w", b.handler.Address, err)
	}
	b.client = modbus.NewClient(b.handler)
	return nil
}

// Close releases the serial port.
func (b *Bus) Close() error { return b.handler.Close() }

// tx acquires the bus, activates slaveID, and calls fn with the shared Modbus client.
// Use for standard FC03/04/06/10 transactions.
func (b *Bus) tx(slaveID byte, fn func(modbus.Client) error) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.handler.SlaveId = slaveID
	return fn(b.client)
}

// txRaw acquires the bus, activates slaveID, and sends a raw ADU for non-standard
// function codes (FC41 EEPROM save, FC42 servo on/off). Returns the raw response.
func (b *Bus) txRaw(slaveID, fc byte, data []byte) ([]byte, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.handler.SlaveId = slaveID
	adu := buildADU(slaveID, fc, data)
	return b.handler.Send(adu)
}

// buildADU assembles a full RTU ADU: [slaveID, fc, data…, crc_lo, crc_hi].
func buildADU(slaveID, fc byte, data []byte) []byte {
	frame := []byte{slaveID, fc}
	frame = append(frame, data...)
	crc := crc16modbus(frame)
	return append(frame, byte(crc), byte(crc>>8)) // RTU CRC is little-endian
}

func crc16modbus(data []byte) uint16 {
	var crc uint16 = 0xFFFF
	for _, b := range data {
		crc ^= uint16(b)
		for range 8 {
			if crc&1 != 0 {
				crc = (crc >> 1) ^ 0xA001
			} else {
				crc >>= 1
			}
		}
	}
	return crc
}
