package t3d

import (
	"context"
	"encoding/binary"
	"fmt"
	"time"

	"github.com/goburrow/modbus"
)

const (
	PulsesPerRev = 10000 // 80AST-A1C04025: 2500-line encoder × 4
	NominalNm    = 2.4   // rated torque for 80AST-A1C04025 (Nm)

	// MoveByPulses tuning.
	moveApproachRPM  = 30  // final approach speed (RPM)
	moveTolerance    = 50  // stop when remaining ≤ this many pulses (~2 mm at r=67.8 mm)
	moveCorrection   = 150 // run correctTo if post-stop error exceeds this (~6 mm)
	moveTorqueSafety = 80  // emergency stop if |torque| ≥ this (% of rated)

	movePollInterval = 15 * time.Millisecond
	moveStopSettle   = 150 * time.Millisecond
	moveDisableWait  = 80 * time.Millisecond
)

// Motor is a handle for one T3D servo drive on a shared RS-485 Bus.
// All I/O is serialized through the Bus mutex — multiple Motor instances
// sharing the same Bus are safe for concurrent use.
//
// Usage:
//
//	bus := t3d.NewBus("/dev/ttyUSB0", 19200)
//	bus.Connect()
//	m1 := t3d.NewMotor(bus, 1)
//	m2 := t3d.NewMotor(bus, 2)
type Motor struct {
	bus     *Bus
	slaveID byte
}

// NewMotor returns a Motor for the given slave ID on bus.
// slaveID must match the drive's P-181 parameter (factory default: 1).
func NewMotor(bus *Bus, slaveID byte) *Motor {
	return &Motor{bus: bus, slaveID: slaveID}
}

// ── Low-level reads (FC04) ────────────────────────────────────────────────────

// ReadInputReg reads one FC04 input register by address.
func (m *Motor) ReadInputReg(addr uint16) (uint16, error) {
	var v uint16
	err := m.bus.tx(m.slaveID, func(c modbus.Client) error {
		b, e := c.ReadInputRegisters(addr, 1)
		if e != nil {
			return fmt.Errorf("FC04 0x%04X: %w", addr, e)
		}
		v = binary.BigEndian.Uint16(b)
		return nil
	})
	return v, err
}

// ReadSpeed returns the current motor speed in RPM (negative = reverse direction).
func (m *Motor) ReadSpeed() (int16, error) {
	v, err := m.ReadInputReg(StatusSpeed)
	return int16(v), err
}

// ReadAbsPosition returns the absolute 32-bit encoder pulse counter from FC04
// registers 0x1F/0x20 (multi-turn + single-turn combined).
// Accumulates across revolutions from power-on; 10 000 pulses = 1 revolution.
func (m *Motor) ReadAbsPosition() (int32, error) {
	var lo, hi uint16
	err := m.bus.tx(m.slaveID, func(c modbus.Client) error {
		b, e := c.ReadInputRegisters(StatusAbsMotorPosL, 2)
		if e != nil {
			return fmt.Errorf("FC04[abspos]: %w", e)
		}
		lo = binary.BigEndian.Uint16(b[0:])
		hi = binary.BigEndian.Uint16(b[2:])
		return nil
	})
	return int32(uint32(hi)<<16 | uint32(lo)), err
}

// ReadPosition returns the absolute 32-bit encoder position (alias for ReadAbsPosition).
func (m *Motor) ReadPosition() (int32, error) {
	return m.ReadAbsPosition()
}

// ReadTorquePct returns the current torque as a percentage of rated torque.
// Negative values indicate reverse torque direction.
// For 80AST: 100% ≈ 2.4 Nm.  F (N) = (pct/100 × 2.4) / radius_m.
func (m *Motor) ReadTorquePct() (int16, error) {
	v, err := m.ReadInputReg(StatusTorquePct)
	return int16(v), err
}

// ReadFault returns the active fault code (0 = normal operation).
func (m *Motor) ReadFault() (uint16, error) {
	return m.ReadInputReg(StatusFaultCode)
}

// ReadStatus reads all key FC04 input registers in batches (≤8 per request per V3.3).
func (m *Motor) ReadStatus() (*Status, error) {
	var b1, b2, b3, b4, b5, b6 []byte
	err := m.bus.tx(m.slaveID, func(c modbus.Client) error {
		var e error
		if b1, e = c.ReadInputRegisters(0x00, 8); e != nil {
			return fmt.Errorf("FC04[0x00]: %w", e)
		}
		if b2, e = c.ReadInputRegisters(0x08, 8); e != nil {
			return fmt.Errorf("FC04[0x08]: %w", e)
		}
		if b3, e = c.ReadInputRegisters(0x10, 8); e != nil {
			return fmt.Errorf("FC04[0x10]: %w", e)
		}
		if b4, e = c.ReadInputRegisters(0x18, 4); e != nil {
			return fmt.Errorf("FC04[0x18]: %w", e)
		}
		if b5, e = c.ReadInputRegisters(0x26, 3); e != nil {
			return fmt.Errorf("FC04[0x26]: %w", e)
		}
		// Absolute multi-turn position (0x1F / 0x20).
		if b6, e = c.ReadInputRegisters(StatusAbsMotorPosL, 2); e != nil {
			return fmt.Errorf("FC04[abspos]: %w", e)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	u16 := func(buf []byte, byteOff int) uint16 {
		return binary.BigEndian.Uint16(buf[byteOff:])
	}

	absL := u16(b6, 0)
	absH := u16(b6, 2)
	return &Status{
		SpeedRPM:       int16(u16(b1, 0)),
		PositionL:      absL,
		PositionH:      absH,
		Position32:     int32(uint32(absH)<<16 | uint32(absL)),
		TorquePct:      int16(u16(b2, 2)),  // (0x09-0x08)*2 = 2
		CurrentA10:     u16(b2, 6),         // (0x0B-0x08)*2 = 6
		SpeedRefRPM:    u16(b2, 12),        // (0x0E-0x08)*2 = 12
		TorqueRefPct:   int16(u16(b2, 14)), // (0x0F-0x08)*2 = 14
		DIState:        u16(b3, 4),         // (0x12-0x10)*2 = 4
		DOState:        u16(b3, 6),         // (0x13-0x10)*2 = 6
		FaultCode:      u16(b4, 4),         // (0x1A-0x18)*2 = 4
		SpeedPrecise10: int16(u16(b4, 6)),  // (0x1B-0x18)*2 = 6
		HeatsinkTempC:  int16(u16(b5, 0)),  // (0x26-0x26)*2 = 0
		ModuleTempC:    int16(u16(b5, 2)),  // (0x27-0x26)*2 = 2
		BusVoltageV:    u16(b5, 4),         // (0x28-0x26)*2 = 4
	}, nil
}

// ── Parameter read/write (FC03 / FC06 / FC10) ────────────────────────────────

// ReadParam reads one FC03 holding register (P-xxx parameter number).
func (m *Motor) ReadParam(addr uint16) (uint16, error) {
	var v uint16
	err := m.bus.tx(m.slaveID, func(c modbus.Client) error {
		b, e := c.ReadHoldingRegisters(addr, 1)
		if e != nil {
			return fmt.Errorf("FC03 P-%03d: %w", addr, e)
		}
		v = binary.BigEndian.Uint16(b)
		return nil
	})
	return v, err
}

// WriteParam writes one FC06 holding register. Changes go to RAM only;
// call SaveEEPROM to persist across power cycles.
func (m *Motor) WriteParam(addr, value uint16) error {
	return m.bus.tx(m.slaveID, func(c modbus.Client) error {
		_, e := c.WriteSingleRegister(addr, value)
		if e != nil {
			return fmt.Errorf("FC06 P-%03d=%d: %w", addr, value, e)
		}
		return nil
	})
}

// WriteParams writes consecutive FC10 holding registers (up to 10 per V3.3).
func (m *Motor) WriteParams(startAddr uint16, values []uint16) error {
	return m.bus.tx(m.slaveID, func(c modbus.Client) error {
		b := make([]byte, len(values)*2)
		for i, v := range values {
			binary.BigEndian.PutUint16(b[i*2:], v)
		}
		_, e := c.WriteMultipleRegisters(startAddr, uint16(len(values)), b)
		if e != nil {
			return fmt.Errorf("FC10 P-%03d[%d]: %w", startAddr, len(values), e)
		}
		return nil
	})
}

// ReadConfig reads the five parameters that determine how the drive accepts motion commands.
func (m *Motor) ReadConfig() (*Config, error) {
	addrs := []uint16{ParamControlMode, ParamSpeedSource, ParamServoOnMode, ParamDI1Func, ParamInternalSpd1}
	vals := make([]uint16, len(addrs))
	for i, a := range addrs {
		v, err := m.ReadParam(a)
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

// SetupSpeedMode configures the drive for Modbus-controlled internal speed preset mode.
// Writes: P-098=1 (force servo-on), P-004=1 (speed), P-025=1 (internal multi-speed),
// P-100=10 (DI1→SP1), P-137=speedRPM (preset 1).
// Call SaveEEPROM afterwards to persist across power cycles.
func (m *Motor) SetupSpeedMode(speedRPM int) error {
	for _, p := range []struct{ a, v uint16 }{
		{ParamServoOnMode, 1},
		{ParamControlMode, ModeSpeed},
		{ParamSpeedSource, SpeedSourceInternal},
		{ParamDI1Func, 10},
		{ParamInternalSpd1, uint16(int16(speedRPM))},
	} {
		if err := m.WriteParam(p.a, p.v); err != nil {
			return err
		}
	}
	return nil
}

// ── Servo control (FC42 / FC41) ───────────────────────────────────────────────

// Enable sends FC42 0x55 to start the servo output.
func (m *Motor) Enable() error {
	resp, err := m.bus.txRaw(m.slaveID, 0x42, []byte{0x55})
	if err != nil {
		return fmt.Errorf("FC42 enable slave %d: %w", m.slaveID, err)
	}
	if len(resp) < 2 || resp[1] != 0x42 {
		return fmt.Errorf("FC42 enable slave %d: unexpected response % x", m.slaveID, resp)
	}
	return nil
}

// Disable sends FC42 0xAA to stop the servo output.
// The motor decelerates per P-061 before stopping.
func (m *Motor) Disable() error {
	resp, err := m.bus.txRaw(m.slaveID, 0x42, []byte{0xAA})
	if err != nil {
		return fmt.Errorf("FC42 disable slave %d: %w", m.slaveID, err)
	}
	if len(resp) < 2 || resp[1] != 0x42 {
		return fmt.Errorf("FC42 disable slave %d: unexpected response % x", m.slaveID, resp)
	}
	return nil
}

// SaveEEPROM sends FC41 to persist RAM parameters to non-volatile memory.
// Allow at least 5 seconds after this call before cutting power.
func (m *Motor) SaveEEPROM() error {
	resp, err := m.bus.txRaw(m.slaveID, 0x41, nil)
	if err != nil {
		return fmt.Errorf("FC41 slave %d: %w", m.slaveID, err)
	}
	if len(resp) < 2 || resp[1] != 0x41 {
		return fmt.Errorf("FC41 slave %d: unexpected response % x", m.slaveID, resp)
	}
	return nil
}

// ── Speed and torque setpoints ────────────────────────────────────────────────

// SetSpeed changes P-137 (speed preset 1) and resumes the motor.
// The drive must be briefly stopped to write P-137 (returns exception 0x10 while active).
// Negative rpm = reverse direction.
func (m *Motor) SetSpeed(rpm int) error {
	if err := m.Disable(); err != nil {
		return err
	}
	time.Sleep(50 * time.Millisecond)
	if err := m.WriteParam(ParamInternalSpd1, uint16(int16(rpm))); err != nil {
		return err
	}
	return m.Enable()
}

// SetTorqueLimit sets P-069 — the maximum torque the drive will produce as a
// percentage of rated torque (0..300, default 300).
//
// In speed mode with a cable/drum: when the load reaches this limit the motor
// stalls holding that force instead of overshooting. Use as a hardware tension
// cap and combine with a slow winding speed for passive auto-tension:
//
//	m.SetTorqueLimit(40)     // max 40% × 2.4 Nm = 0.96 Nm
//	m.SetSpeed(25)           // slow winding; stalls when cable is taut
func (m *Motor) SetTorqueLimit(pct int) error {
	if pct < 0 {
		pct = 0
	}
	if pct > 300 {
		pct = 300
	}
	return m.WriteParam(ParamTorqueLimit, uint16(pct))
}

// SetAccelTime sets P-060 — the acceleration ramp time in milliseconds per 1000 RPM.
// Lower values = faster acceleration; factory default is 100 ms/1000 rpm.
// Use motion.AccelToT3DParam to convert from mm/s² to this value.
func (m *Motor) SetAccelTime(msPerKRPM int) error {
	if msPerKRPM < 1 {
		msPerKRPM = 1
	}
	if msPerKRPM > 30000 {
		msPerKRPM = 30000
	}
	return m.WriteParam(ParamAccelTime, uint16(msPerKRPM))
}

// SetDecelTime sets P-061 — the deceleration ramp time in milliseconds per 1000 RPM.
// Lower values = faster braking; factory default is 100 ms/1000 rpm.
// Use motion.AccelToT3DParam to convert from mm/s² to this value.
func (m *Motor) SetDecelTime(msPerKRPM int) error {
	if msPerKRPM < 1 {
		msPerKRPM = 1
	}
	if msPerKRPM > 30000 {
		msPerKRPM = 30000
	}
	return m.WriteParam(ParamDecelTime, uint16(msPerKRPM))
}

// ── High-level motion commands ────────────────────────────────────────────────

// MoveByPulses moves the motor by exactly `pulses` encoder counts at `speedRPM`.
// Negative pulses = reverse direction. speedRPM must be positive.
//
// Motion is two-phase:
//  1. Full speed until `approach` pulses remain.
//  2. Slow approach (moveApproachRPM) until within moveTolerance pulses.
//
// After stopping, position is verified; if the error exceeds moveCorrection a
// single low-speed correction pass is made. Torque is monitored throughout —
// if it reaches moveTorqueSafety the move is aborted with an error.
//
// Returns when the motor has stopped. Cancelling ctx stops the motor immediately.
func (m *Motor) MoveByPulses(ctx context.Context, pulses int64, speedRPM int) error {
	if speedRPM <= 0 {
		return fmt.Errorf("t3d MoveByPulses: speedRPM must be positive, got %d", speedRPM)
	}
	if pulses == 0 {
		return nil
	}

	absPulses := pulses
	if absPulses < 0 {
		absPulses = -absPulses
	}

	directedRPM := speedRPM
	if pulses < 0 {
		directedRPM = -speedRPM
	}

	// Switch to approach speed when this many pulses remain.
	// Heuristic: 5 × speedRPM pulses (≈ one motor revolution at that RPM)
	// gives enough distance to decelerate from full speed to moveApproachRPM
	// without overshoot at 19200 baud poll rates (~15 ms per sample).
	approach := int64(speedRPM) * 5
	if approach < 500 {
		approach = 500
	}
	if approach > absPulses/2 {
		approach = absPulses / 2
	}

	// Stop and read a stable start position.
	if err := m.Disable(); err != nil {
		return fmt.Errorf("t3d MoveByPulses: initial disable: %w", err)
	}
	time.Sleep(moveDisableWait)

	startPos, err := m.ReadAbsPosition()
	if err != nil {
		return fmt.Errorf("t3d MoveByPulses: read start pos: %w", err)
	}

	if err := m.WriteParam(ParamInternalSpd1, uint16(int16(directedRPM))); err != nil {
		return fmt.Errorf("t3d MoveByPulses: set speed: %w", err)
	}
	if err := m.Enable(); err != nil {
		return fmt.Errorf("t3d MoveByPulses: enable: %w", err)
	}

	inApproach := false

	for {
		select {
		case <-ctx.Done():
			_ = m.Disable()
			return ctx.Err()
		default:
		}

		pos, torque, fault, err := m.readMotionState()
		if err != nil {
			_ = m.Disable()
			return fmt.Errorf("t3d MoveByPulses: poll: %w", err)
		}
		if fault != 0 {
			_ = m.Disable()
			return fmt.Errorf("t3d MoveByPulses: drive fault %d", fault)
		}
		absT := torque
		if absT < 0 {
			absT = -absT
		}
		if int(absT) >= moveTorqueSafety {
			_ = m.Disable()
			return fmt.Errorf("t3d MoveByPulses: torque safety trip %d%%", torque)
		}

		traveled := int64(pos) - int64(startPos)
		if traveled < 0 {
			traveled = -traveled
		}
		remaining := absPulses - traveled

		if !inApproach && remaining <= approach {
			inApproach = true
			_ = m.Disable()
			time.Sleep(60 * time.Millisecond)
			approachRPM := moveApproachRPM
			if pulses < 0 {
				approachRPM = -moveApproachRPM
			}
			_ = m.WriteParam(ParamInternalSpd1, uint16(int16(approachRPM)))
			_ = m.Enable()
		}

		if remaining <= moveTolerance {
			break
		}

		time.Sleep(movePollInterval)
	}

	if err := m.Disable(); err != nil {
		return err
	}

	// Wait for full mechanical stop, then verify and correct if needed.
	time.Sleep(moveStopSettle)
	return m.correctTo(ctx, int64(startPos)+pulses)
}

// RunUntilTorque runs the motor at speedRPM until the absolute torque reaches
// maxTorquePct (% of rated). Stops the motor and returns when the limit is hit.
//
// Intended for cable/rope winding: motor pulls until the measured torque (= tension
// proxy) reaches the threshold, then stops. Set P-069 via SetTorqueLimit to the same
// or slightly higher value as a hardware safety net.
//
// Returns when stopped. Cancelling ctx stops the motor immediately.
func (m *Motor) RunUntilTorque(ctx context.Context, speedRPM int, maxTorquePct int) error {
	if err := m.SetSpeed(speedRPM); err != nil {
		return fmt.Errorf("t3d RunUntilTorque: %w", err)
	}

	for {
		select {
		case <-ctx.Done():
			_ = m.Disable()
			return ctx.Err()
		default:
		}

		_, torque, fault, err := m.readMotionState()
		if err != nil {
			_ = m.Disable()
			return fmt.Errorf("t3d RunUntilTorque: poll: %w", err)
		}
		if fault != 0 {
			_ = m.Disable()
			return fmt.Errorf("t3d RunUntilTorque: drive fault %d", fault)
		}

		absT := torque
		if absT < 0 {
			absT = -absT
		}
		if int(absT) >= maxTorquePct {
			return m.Disable()
		}

		time.Sleep(10 * time.Millisecond)
	}
}

// ── Internal helpers ──────────────────────────────────────────────────────────

// ReadMotionState reads position, torque, and fault in two optimised FC04 batches.
// Public alias for use by higher-level packages (robot, etc.).
func (m *Motor) ReadMotionState() (pos int32, torque int16, fault uint16, err error) {
	return m.readMotionState()
}

// readMotionState reads position, torque, and fault in two optimised FC04 batches.
//
// Batch 1: torque only (0x09, 1 register).
// Batch 2: fault + gap + absolute position (0x1A..0x20, 7 registers):
//
//	offset 0  (0x1A) = fault code
//	offset 10 (0x1F) = abs pos low word
//	offset 12 (0x20) = abs pos high word
func (m *Motor) readMotionState() (pos int32, torque int16, fault uint16, err error) {
	var b1, b2 []byte
	err = m.bus.tx(m.slaveID, func(c modbus.Client) error {
		var e error
		if b1, e = c.ReadInputRegisters(StatusTorquePct, 1); e != nil {
			return fmt.Errorf("FC04[torque]: %w", e)
		}
		// 7 regs from 0x1A: fault(0x1A), precise_spd(0x1B), enc2L(0x1C),
		// enc2H(0x1D), ?(0x1E), absL(0x1F), absH(0x20).
		if b2, e = c.ReadInputRegisters(StatusFaultCode, 7); e != nil {
			return fmt.Errorf("FC04[fault+abspos]: %w", e)
		}
		return nil
	})
	if err != nil {
		return
	}
	torque = int16(binary.BigEndian.Uint16(b1))
	fault = binary.BigEndian.Uint16(b2[0:])
	absL := binary.BigEndian.Uint16(b2[10:]) // (0x1F-0x1A)*2 = 10
	absH := binary.BigEndian.Uint16(b2[12:]) // (0x20-0x1A)*2 = 12
	pos = int32(uint32(absH)<<16 | uint32(absL))
	return
}

// correctTo performs a single low-speed correction pass if the actual position
// after a move differs from targetPos by more than moveCorrection pulses.
// A best-effort function: errors during the correction read are silently ignored
// so they do not mask the original move's success.
func (m *Motor) correctTo(ctx context.Context, targetPos int64) error {
	actual, err := m.ReadAbsPosition()
	if err != nil {
		return nil // best-effort
	}

	errPulses := targetPos - int64(actual)
	absErr := errPulses
	if absErr < 0 {
		absErr = -absErr
	}
	if absErr <= moveCorrection {
		return nil
	}

	corrRPM := moveApproachRPM
	if errPulses < 0 {
		corrRPM = -moveApproachRPM
	}
	if err := m.WriteParam(ParamInternalSpd1, uint16(int16(corrRPM))); err != nil {
		return err
	}
	if err := m.Enable(); err != nil {
		return err
	}

	for {
		select {
		case <-ctx.Done():
			_ = m.Disable()
			return ctx.Err()
		default:
		}

		pos, _, fault, err := m.readMotionState()
		if err != nil {
			_ = m.Disable()
			return fmt.Errorf("t3d correctTo: poll: %w", err)
		}
		if fault != 0 {
			_ = m.Disable()
			return fmt.Errorf("t3d correctTo: fault %d", fault)
		}

		remaining := targetPos - int64(pos)
		if remaining < 0 {
			remaining = -remaining
		}
		if remaining <= moveTolerance {
			break
		}
		time.Sleep(movePollInterval)
	}

	return m.Disable()
}
