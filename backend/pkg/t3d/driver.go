package t3d

import (
	"encoding/binary"
	"fmt"
	"sync"
	"time"

	"github.com/goburrow/modbus"
)

const (
	DefaultPort    = "/dev/ttyUSB0"
	DefaultBaud    = 19200
	DefaultSlaveID = byte(1)
)

// Driver communicates with the HLTNC T3D servo driver over Modbus RTU / RS-485.
//
// Standard function codes (FC03/04/06/10) go through goburrow/modbus.
// Non-standard codes FC41 (save EEPROM) and FC42 (servo on/off) use handler.Send
// with a manually assembled RTU ADU, since goburrow/modbus has no hook for custom FCs.
//
// All public methods acquire mu before touching the serial port, so concurrent
// calls (e.g. background status poll vs. a write from a key press) are safe.
type Driver struct {
	mu      sync.Mutex
	handler *modbus.RTUClientHandler
	client  modbus.Client
}

// Status is a snapshot of the key FC04 input registers.
type Status struct {
	SpeedRPM       int16  // ST-000: motor speed (rpm); negative = reverse
	PositionL      uint16 // ST-005: current position low word (pulses)
	PositionH      uint16 // ST-006: current position high word (pulses)
	Position32     int32  // ST-005/006 combined as signed 32-bit pulse count
	TorquePct      int16  // ST-009: torque (% of rated)
	CurrentA10     uint16 // ST-00B: instantaneous current (A×10; 12 = 1.2 A)
	SpeedRefRPM    uint16 // ST-00E: speed setpoint (rpm)
	TorqueRefPct   int16  // ST-00F: torque setpoint (%)
	DIState        uint16 // ST-012: DI pin states — bit0=DI1 … bit7=DI8
	DOState        uint16 // ST-013: DO pin states — bit0=DO1 … bit5=DO6
	FaultCode      uint16 // ST-01A: fault code (0 = no fault)
	SpeedPrecise10 int16  // ST-01B: precise speed (×0.1 rpm)
	HeatsinkTempC  int16  // ST-026: heatsink temperature (°C)
	ModuleTempC    int16  // ST-027: module temperature (°C)
	BusVoltageV    uint16 // ST-028: DC bus voltage (V; nominal ≈310 V)
}

// New creates a Driver for the given port, baud rate, and slave ID.
// Call Connect before any read/write operations.
func New(port string, baud int, slaveID byte) *Driver {
	h := modbus.NewRTUClientHandler(port)
	h.BaudRate = baud
	h.DataBits = 8
	h.Parity = "E" // 8E1 is the T3D default (P-183=1)
	h.StopBits = 1
	h.SlaveId = slaveID
	h.Timeout = 500 * time.Millisecond
	return &Driver{handler: h}
}

// SetSlaveID changes the active slave address without reconnecting.
func (d *Driver) SetSlaveID(id byte) {
	d.handler.SlaveId = id
}

// Connect opens the serial port. Must be called before any operations.
func (d *Driver) Connect() error {
	if err := d.handler.Connect(); err != nil {
		return fmt.Errorf("t3d: connect %s: %w", d.handler.Address, err)
	}
	d.client = modbus.NewClient(d.handler)
	return nil
}

// Close releases the serial port.
func (d *Driver) Close() error {
	return d.handler.Close()
}

// ReadParam reads one holding register (FC03).
// addr is the P-xxx parameter number in decimal (e.g. ParamControlMode = 4).
func (d *Driver) ReadParam(addr uint16) (uint16, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	b, err := d.client.ReadHoldingRegisters(addr, 1)
	if err != nil {
		return 0, fmt.Errorf("t3d FC03 P-%03d: %w", addr, err)
	}
	return binary.BigEndian.Uint16(b), nil
}

// WriteParam writes one holding register (FC06).
// Writes go to RAM only; call SaveEEPROM to persist across power cycles.
func (d *Driver) WriteParam(addr, value uint16) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	_, err := d.client.WriteSingleRegister(addr, value)
	if err != nil {
		return fmt.Errorf("t3d FC06 P-%03d=%d: %w", addr, value, err)
	}
	return nil
}

// WriteParams writes consecutive holding registers (FC10).
// T3D accepts up to 10 registers per request.
func (d *Driver) WriteParams(startAddr uint16, values []uint16) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	b := make([]byte, len(values)*2)
	for i, v := range values {
		binary.BigEndian.PutUint16(b[i*2:], v)
	}
	_, err := d.client.WriteMultipleRegisters(startAddr, uint16(len(values)), b)
	if err != nil {
		return fmt.Errorf("t3d FC10 P-%03d[%d]: %w", startAddr, len(values), err)
	}
	return nil
}

// ReadInputReg reads one input register (FC04).
// addr is a StatusXxx constant.
func (d *Driver) ReadInputReg(addr uint16) (uint16, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	b, err := d.client.ReadInputRegisters(addr, 1)
	if err != nil {
		return 0, fmt.Errorf("t3d FC04 0x%04X: %w", addr, err)
	}
	return binary.BigEndian.Uint16(b), nil
}

// ReadStatus reads all key FC04 registers in batches of ≤8 (device limit per V3.3).
func (d *Driver) ReadStatus() (*Status, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	read := func(start, count uint16) ([]byte, error) {
		b, err := d.client.ReadInputRegisters(start, count)
		if err != nil {
			return nil, fmt.Errorf("t3d FC04 0x%02X[%d]: %w", start, count, err)
		}
		return b, nil
	}
	// Extract register value at absolute address from a batch buffer.
	reg := func(buf []byte, batchStart, addr uint16) uint16 {
		off := (addr - batchStart) * 2
		return binary.BigEndian.Uint16(buf[off:])
	}

	// 0x00..0x07: speed, position refs, current position, deviation
	b1, err := read(0x00, 8)
	if err != nil {
		return nil, err
	}
	// 0x08..0x0F: torque, current, pulse freq, speed/torque refs, analog voltages
	b2, err := read(0x08, 8)
	if err != nil {
		return nil, err
	}
	// 0x10..0x17: DI/DO state, encoder abs position, brake load
	b3, err := read(0x10, 8)
	if err != nil {
		return nil, err
	}
	// 0x18..0x1B: avg load, output voltage, fault code, precise speed
	b4, err := read(0x18, 4)
	if err != nil {
		return nil, err
	}
	// 0x26..0x28: temperatures, DC bus voltage
	b5, err := read(0x26, 3)
	if err != nil {
		return nil, err
	}

	posL := reg(b1, 0x00, StatusPositionL)
	posH := reg(b1, 0x00, StatusPositionH)

	return &Status{
		SpeedRPM:       int16(reg(b1, 0x00, StatusSpeed)),
		PositionL:      posL,
		PositionH:      posH,
		Position32:     int32(uint32(posH)<<16 | uint32(posL)),
		TorquePct:      int16(reg(b2, 0x08, StatusTorquePct)),
		CurrentA10:     reg(b2, 0x08, StatusCurrentA10),
		SpeedRefRPM:    reg(b2, 0x08, StatusSpeedRef),
		TorqueRefPct:   int16(reg(b2, 0x08, StatusTorqueRef)),
		DIState:        reg(b3, 0x10, StatusDIState),
		DOState:        reg(b3, 0x10, StatusDOState),
		FaultCode:      reg(b4, 0x18, StatusFaultCode),
		SpeedPrecise10: int16(reg(b4, 0x18, StatusSpeedPrecise10)),
		HeatsinkTempC:  int16(reg(b5, 0x26, StatusHeatsinkTempC)),
		ModuleTempC:    int16(reg(b5, 0x26, StatusModuleTempC)),
		BusVoltageV:    reg(b5, 0x26, StatusBusVoltageV),
	}, nil
}

// Config holds the five parameters that determine how motion commands reach the motor.
type Config struct {
	ControlMode  uint16 // P-004: 0=position, 1=speed, 2=torque
	SpeedSource  uint16 // P-025: 0=analog, 1=internal multi-speed, 3=pulse
	ServoOnMode  uint16 // P-098: 0=external SON pin, 1=always ON
	DI1Func      uint16 // P-100: 1=SON, 10=SP1, 11=SP2, …
	InternalSpd1 int16  // P-137: internal speed preset 1 (rpm); negative = reverse
}

// readParamLocked reads one holding register without acquiring the mutex.
// Caller must hold d.mu.
func (d *Driver) readParamLocked(addr uint16) (uint16, error) {
	b, err := d.client.ReadHoldingRegisters(addr, 1)
	if err != nil {
		return 0, fmt.Errorf("t3d FC03 P-%03d: %w", addr, err)
	}
	return binary.BigEndian.Uint16(b), nil
}

// ReadConfig reads the five parameters that govern how the drive accepts motion commands.
func (d *Driver) ReadConfig() (*Config, error) {
	addrs := []uint16{ParamControlMode, ParamSpeedSource, ParamServoOnMode, ParamDI1Func, ParamInternalSpd1}
	vals := make([]uint16, len(addrs))
	d.mu.Lock()
	defer d.mu.Unlock()
	for i, a := range addrs {
		v, err := d.readParamLocked(a)
		if err != nil {
			return nil, err
		}
		vals[i] = v
	}
	return &Config{
		ControlMode:  vals[0],
		SpeedSource:  vals[1],
		ServoOnMode:  vals[2],
		DI1Func:      vals[3],
		InternalSpd1: int16(vals[4]),
	}, nil
}

// writeParamLocked writes one holding register without acquiring the mutex.
// Caller must hold d.mu.
func (d *Driver) writeParamLocked(addr, value uint16) error {
	_, err := d.client.WriteSingleRegister(addr, value)
	if err != nil {
		return fmt.Errorf("t3d FC06 P-%03d=%d: %w", addr, value, err)
	}
	return nil
}

// SetupSpeedMode configures the drive for Modbus-controlled internal speed preset mode.
// Writes: P-098=1 (force servo-on), P-004=1 (speed), P-025=1 (internal multi-speed),
// P-100=10 (DI1→SP1), P-137=speedRPM (preset 1).
// If DI1 is already wired HIGH the motor will start running immediately.
// Call SaveEEPROM afterwards to persist across power cycles.
func (d *Driver) SetupSpeedMode(speedRPM int) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	for _, p := range []struct{ a, v uint16 }{
		{ParamServoOnMode, 1},
		{ParamControlMode, ModeSpeed},
		{ParamSpeedSource, SpeedSourceInternal},
		{ParamDI1Func, 10},
		{ParamInternalSpd1, uint16(int16(speedRPM))},
	} {
		if err := d.writeParamLocked(p.a, p.v); err != nil {
			return err
		}
	}
	return nil
}

// SetSpeedPreset changes P-137 (internal speed preset 1, rpm; negative = reverse).
// The servo is briefly stopped to allow the write (drive rejects FC06 to P-137 while active).
// If resume is true the servo is re-enabled after the write.
func (d *Driver) SetSpeedPreset(speedRPM int, resume bool) error {
	if err := d.ServoDisable(); err != nil {
		return err
	}
	time.Sleep(50 * time.Millisecond)
	if err := d.WriteParam(ParamInternalSpd1, uint16(int16(speedRPM))); err != nil {
		return err
	}
	if resume {
		return d.ServoEnable()
	}
	return nil
}

// ServoEnable sends FC42 0x55 to enable the servo output.
func (d *Driver) ServoEnable() error {
	return d.sendFC42(0x55)
}

// ServoDisable sends FC42 0xAA to disable the servo output.
func (d *Driver) ServoDisable() error {
	return d.sendFC42(0xAA)
}

// SaveEEPROM sends FC41 to persist current parameters to non-volatile memory.
// Allow at least 5 seconds after this call before cutting power.
func (d *Driver) SaveEEPROM() error {
	d.mu.Lock()
	defer d.mu.Unlock()
	adu := buildADU(d.handler.SlaveId, 0x41, nil)
	resp, err := d.handler.Send(adu)
	if err != nil {
		return fmt.Errorf("t3d FC41 save EEPROM: %w", err)
	}
	if len(resp) < 2 || resp[1] != 0x41 {
		return fmt.Errorf("t3d FC41: unexpected response % x", resp)
	}
	return nil
}

func (d *Driver) sendFC42(cmd byte) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	adu := buildADU(d.handler.SlaveId, 0x42, []byte{cmd})
	resp, err := d.handler.Send(adu)
	if err != nil {
		return fmt.Errorf("t3d FC42 0x%02X: %w", cmd, err)
	}
	if len(resp) < 2 || resp[1] != 0x42 {
		return fmt.Errorf("t3d FC42: unexpected response % x", resp)
	}
	return nil
}

// buildADU assembles a full RTU ADU: [slaveID, fc, data…, crc_lo, crc_hi].
// goburrow/modbus only handles standard function codes in its client, so we
// build the ADU manually for FC41/FC42 and pass it to handler.Send directly.
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
