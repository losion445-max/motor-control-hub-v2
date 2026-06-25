package t3d

import (
	"fmt"
	"time"
)

const (
	DefaultPort    = "/dev/ttyUSB0"
	DefaultBaud    = 19200
	DefaultSlaveID = byte(1)
)

// Status is a snapshot of the key FC04 input registers.
type Status struct {
	SpeedRPM       int16  `json:"speed_rpm"`        // ST-000: motor speed (rpm); negative = reverse
	PositionL      uint16 `json:"position_l"`        // ST-01F: absolute motor position low word (pulses, multi-turn)
	PositionH      uint16 `json:"position_h"`        // ST-020: absolute motor position high word (pulses, multi-turn)
	Position32     int32  `json:"position32"`        // ST-01F/020 combined as signed 32-bit pulse count
	TorquePct      int16  `json:"torque_pct"`        // ST-009: torque (% of rated)
	CurrentA10     uint16 `json:"current_a10"`       // ST-00B: instantaneous current (A×10; 12 = 1.2 A)
	SpeedRefRPM    uint16 `json:"speed_ref_rpm"`     // ST-00E: speed setpoint (rpm)
	TorqueRefPct   int16  `json:"torque_ref_pct"`    // ST-00F: torque setpoint (%)
	DIState        uint16 `json:"di_state"`           // ST-012: DI pin states — bit0=DI1 … bit7=DI8
	DOState        uint16 `json:"do_state"`           // ST-013: DO pin states — bit0=DO1 … bit5=DO6
	FaultCode      uint16 `json:"fault_code"`        // ST-01A: fault code (0 = no fault)
	SpeedPrecise10 int16  `json:"speed_precise10"`   // ST-01B: precise speed (×0.1 rpm)
	HeatsinkTempC  int16  `json:"heatsink_temp_c"`   // ST-026: heatsink temperature (°C)
	ModuleTempC    int16  `json:"module_temp_c"`     // ST-027: module temperature (°C)
	BusVoltageV    uint16 `json:"bus_voltage_v"`     // ST-028: DC bus voltage (V; nominal ≈310 V)
}

// Config holds the five parameters that determine how motion commands reach the motor.
type Config struct {
	ControlMode  uint16 // P-004: 0=position, 1=speed, 2=torque
	SpeedSource  uint16 // P-025: 0=analog, 1=internal multi-speed, 3=pulse
	ServoOnMode  uint16 // P-098: 0=external SON pin, 1=always ON
	DI1Func      uint16 // P-100: 1=SON, 10=SP1, 11=SP2, …
	InternalSpd1 int16  // P-137: internal speed preset 1 (rpm); negative = reverse
}

// Driver is a convenience wrapper for single-motor use.
// For multiple motors on the same RS-485 bus, use NewBus + NewMotor directly:
//
//	bus := t3d.NewBus("/dev/ttyUSB0", 19200)
//	bus.Connect()
//	m1 := t3d.NewMotor(bus, 1)
//	m2 := t3d.NewMotor(bus, 2)
type Driver struct {
	bus   *Bus
	motor *Motor
}

// New creates a Driver for the given port, baud rate, and slave ID.
// Call Connect before any read/write operations.
func New(port string, baud int, slaveID byte) *Driver {
	bus := NewBus(port, baud)
	return &Driver{
		bus:   bus,
		motor: NewMotor(bus, slaveID),
	}
}

// Connect opens the serial port.
func (d *Driver) Connect() error { return d.bus.Connect() }

// Close releases the serial port.
func (d *Driver) Close() error { return d.bus.Close() }

// SetSlaveID changes the active slave address for subsequent operations.
// Not safe for concurrent use; prefer separate Motor instances.
func (d *Driver) SetSlaveID(id byte) { d.motor.slaveID = id }

// ReadParam reads one FC03 holding register (P-xxx parameter number).
func (d *Driver) ReadParam(addr uint16) (uint16, error) { return d.motor.ReadParam(addr) }

// WriteParam writes one FC06 holding register.
func (d *Driver) WriteParam(addr, value uint16) error { return d.motor.WriteParam(addr, value) }

// WriteParams writes consecutive FC10 holding registers.
func (d *Driver) WriteParams(startAddr uint16, values []uint16) error {
	return d.motor.WriteParams(startAddr, values)
}

// ReadInputReg reads one FC04 input register.
func (d *Driver) ReadInputReg(addr uint16) (uint16, error) { return d.motor.ReadInputReg(addr) }

// ReadStatus reads all key FC04 input registers.
func (d *Driver) ReadStatus() (*Status, error) { return d.motor.ReadStatus() }

// ReadConfig reads the five parameters governing motion command input.
func (d *Driver) ReadConfig() (*Config, error) { return d.motor.ReadConfig() }

// SetupSpeedMode configures the drive for Modbus-controlled internal speed preset mode.
func (d *Driver) SetupSpeedMode(speedRPM int) error { return d.motor.SetupSpeedMode(speedRPM) }

// SetSpeedPreset changes P-137 (internal speed preset 1).
// If resume is true the servo is re-enabled after the write.
func (d *Driver) SetSpeedPreset(speedRPM int, resume bool) error {
	if err := d.motor.Disable(); err != nil {
		return err
	}
	time.Sleep(50 * time.Millisecond)
	if err := d.motor.WriteParam(ParamInternalSpd1, uint16(int16(speedRPM))); err != nil {
		return err
	}
	if resume {
		return d.motor.Enable()
	}
	return nil
}

// ServoEnable sends FC42 0x55 to enable the servo output.
func (d *Driver) ServoEnable() error { return d.motor.Enable() }

// ServoDisable sends FC42 0xAA to disable the servo output.
func (d *Driver) ServoDisable() error { return d.motor.Disable() }

// SaveEEPROM sends FC41 to persist current parameters to non-volatile memory.
// Allow at least 5 seconds after this call before cutting power.
func (d *Driver) SaveEEPROM() error { return d.motor.SaveEEPROM() }

// SaveEEPROMAlt writes the alternative EEPROM trigger (FC06 addr=0x1001 val=0x1234).
// Required to persist P-025 when the standard FC41 does not save it reliably.
func (d *Driver) SaveEEPROMAlt() error {
	return d.motor.WriteParam(0x1001, 0x1234)
}

// Motor returns the underlying Motor instance for direct access to the full API
// (MoveByPulses, RunUntilTorque, SetTorqueLimit, etc.).
func (d *Driver) Motor() *Motor { return d.motor }

// Bus returns the underlying Bus, useful for creating additional Motor instances
// on the same physical RS-485 port without opening a second connection.
func (d *Driver) Bus() *Bus { return d.bus }

// String returns a human-readable description of the driver connection.
func (d *Driver) String() string {
	return fmt.Sprintf("T3D(slave=%d @ %s)", d.motor.slaveID, d.bus.handler.Address)
}
