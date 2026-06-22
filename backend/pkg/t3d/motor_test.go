package t3d

import (
	"context"
	"encoding/binary"
	"errors"
	"testing"

	"github.com/goburrow/modbus"
)

// ── mock busTransport ─────────────────────────────────────────────────────────

// mockBus implements busTransport. tx dispatches to a mockClient whose
// read/write responses are preconfigured via regMap and holdMap.
type mockBus struct {
	client *mockClient
	rawErr error
	rawResp []byte
}

func (b *mockBus) tx(slaveID byte, fn func(modbus.Client) error) error {
	return fn(b.client)
}

func (b *mockBus) txRaw(_ byte, _ byte, _ []byte) ([]byte, error) {
	return b.rawResp, b.rawErr
}

// ── mock modbus.Client ────────────────────────────────────────────────────────

// mockClient returns preset bytes for specific (address, quantity) pairs.
// readInput covers FC04, readHolding covers FC03.
// All unrecognised reads return zeroed bytes of the expected length.
type mockClient struct {
	readInput   map[uint16][]byte // FC04 responses by start address
	readHolding map[uint16][]byte // FC03 responses by start address
	writeErr    error
	lastWriteAddr uint16
	lastWriteVal  uint16
}

func (c *mockClient) ReadInputRegisters(addr, qty uint16) ([]byte, error) {
	if c.readInput != nil {
		if b, ok := c.readInput[addr]; ok {
			return b, nil
		}
	}
	return make([]byte, int(qty)*2), nil
}

func (c *mockClient) ReadHoldingRegisters(addr, qty uint16) ([]byte, error) {
	if c.readHolding != nil {
		if b, ok := c.readHolding[addr]; ok {
			return b, nil
		}
	}
	return make([]byte, int(qty)*2), nil
}

func (c *mockClient) WriteSingleRegister(addr, value uint16) ([]byte, error) {
	c.lastWriteAddr = addr
	c.lastWriteVal = value
	return []byte{0, 0, 0, 0}, c.writeErr
}

func (c *mockClient) WriteMultipleRegisters(addr, qty uint16, val []byte) ([]byte, error) {
	return val, c.writeErr
}

// Remaining Client methods — not used in Motor.
func (c *mockClient) ReadCoils(_, _ uint16) ([]byte, error)                               { return nil, nil }
func (c *mockClient) ReadDiscreteInputs(_, _ uint16) ([]byte, error)                      { return nil, nil }
func (c *mockClient) WriteSingleCoil(_, _ uint16) ([]byte, error)                         { return nil, nil }
func (c *mockClient) WriteMultipleCoils(_, _ uint16, _ []byte) ([]byte, error)            { return nil, nil }
func (c *mockClient) ReadWriteMultipleRegisters(_, _, _, _ uint16, _ []byte) ([]byte, error) { return nil, nil }
func (c *mockClient) MaskWriteRegister(_, _, _ uint16) ([]byte, error)                    { return nil, nil }
func (c *mockClient) ReadFIFOQueue(_ uint16) ([]byte, error)                              { return nil, nil }

// ── helpers ───────────────────────────────────────────────────────────────────

func newMotorMock() (*Motor, *mockBus) {
	mc := &mockClient{
		readInput:   make(map[uint16][]byte),
		readHolding: make(map[uint16][]byte),
	}
	bus := &mockBus{
		client:  mc,
		rawResp: []byte{0x01, 0x42}, // default: valid FC42 response
	}
	m := &Motor{bus: bus, slaveID: 1}
	return m, bus
}

func u16be(v uint16) []byte {
	b := make([]byte, 2)
	binary.BigEndian.PutUint16(b, v)
	return b
}

// ── ReadInputReg ──────────────────────────────────────────────────────────────

func TestReadInputReg_OK(t *testing.T) {
	m, bus := newMotorMock()
	bus.client.readInput[0x09] = u16be(42)
	v, err := m.ReadInputReg(0x09)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if v != 42 {
		t.Errorf("ReadInputReg = %d, want 42", v)
	}
}

func TestReadInputReg_Error(t *testing.T) {
	m, bus := newMotorMock()
	boom := errors.New("bus error")
	// Override tx to return error
	bus.client.readInput = nil
	bus2 := &errorBus{err: boom}
	m.bus = bus2

	_, err := m.ReadInputReg(0x00)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

// errorBus always returns an error from tx.
type errorBus struct{ err error }

func (b *errorBus) tx(_ byte, fn func(modbus.Client) error) error {
	return fn(&errClient{err: b.err})
}
func (b *errorBus) txRaw(_, _ byte, _ []byte) ([]byte, error) { return nil, b.err }

type errClient struct{ err error }

func (c *errClient) ReadInputRegisters(_, _ uint16) ([]byte, error)                          { return nil, c.err }
func (c *errClient) ReadHoldingRegisters(_, _ uint16) ([]byte, error)                         { return nil, c.err }
func (c *errClient) WriteSingleRegister(_, _ uint16) ([]byte, error)                          { return nil, c.err }
func (c *errClient) WriteMultipleRegisters(_, _ uint16, _ []byte) ([]byte, error)             { return nil, c.err }
func (c *errClient) ReadCoils(_, _ uint16) ([]byte, error)                                    { return nil, c.err }
func (c *errClient) ReadDiscreteInputs(_, _ uint16) ([]byte, error)                           { return nil, c.err }
func (c *errClient) WriteSingleCoil(_, _ uint16) ([]byte, error)                              { return nil, c.err }
func (c *errClient) WriteMultipleCoils(_, _ uint16, _ []byte) ([]byte, error)                 { return nil, c.err }
func (c *errClient) ReadWriteMultipleRegisters(_, _, _, _ uint16, _ []byte) ([]byte, error)   { return nil, c.err }
func (c *errClient) MaskWriteRegister(_, _, _ uint16) ([]byte, error)                         { return nil, c.err }
func (c *errClient) ReadFIFOQueue(_ uint16) ([]byte, error)                                   { return nil, c.err }

// ── ReadAbsPosition ───────────────────────────────────────────────────────────

func TestReadAbsPosition(t *testing.T) {
	m, bus := newMotorMock()
	// Encode pos = 0x00010002 → hi=0x0001 lo=0x0002
	// The function reads StatusAbsMotorPosL (0x1F), qty=2 → 4 bytes: lo then hi.
	b := make([]byte, 4)
	binary.BigEndian.PutUint16(b[0:], 0x0002) // lo
	binary.BigEndian.PutUint16(b[2:], 0x0001) // hi
	bus.client.readInput[StatusAbsMotorPosL] = b

	pos, err := m.ReadAbsPosition()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// pos = int32(hi<<16 | lo) = int32(0x00010002) = 65538
	if pos != 65538 {
		t.Errorf("ReadAbsPosition = %d, want 65538", pos)
	}
}

func TestReadAbsPosition_Negative(t *testing.T) {
	m, bus := newMotorMock()
	// Encode -1 = 0xFFFFFFFF → lo=0xFFFF hi=0xFFFF
	b := make([]byte, 4)
	binary.BigEndian.PutUint16(b[0:], 0xFFFF) // lo
	binary.BigEndian.PutUint16(b[2:], 0xFFFF) // hi
	bus.client.readInput[StatusAbsMotorPosL] = b

	pos, err := m.ReadAbsPosition()
	if err != nil {
		t.Fatal(err)
	}
	if pos != -1 {
		t.Errorf("ReadAbsPosition = %d, want -1", pos)
	}
}

// ── ReadTorquePct ─────────────────────────────────────────────────────────────

func TestReadTorquePct(t *testing.T) {
	m, bus := newMotorMock()
	var neg35 int16 = -35
	bus.client.readInput[StatusTorquePct] = u16be(uint16(neg35)) // -35 % encoded as two's complement
	torque, err := m.ReadTorquePct()
	if err != nil {
		t.Fatal(err)
	}
	if torque != -35 {
		t.Errorf("ReadTorquePct = %d, want -35", torque)
	}
}

// ── ReadFault ─────────────────────────────────────────────────────────────────

func TestReadFault_NoFault(t *testing.T) {
	m, bus := newMotorMock()
	bus.client.readInput[StatusFaultCode] = u16be(0)
	f, err := m.ReadFault()
	if err != nil {
		t.Fatal(err)
	}
	if f != 0 {
		t.Errorf("ReadFault = %d, want 0", f)
	}
}

func TestReadFault_WithCode(t *testing.T) {
	m, bus := newMotorMock()
	bus.client.readInput[StatusFaultCode] = u16be(7)
	f, err := m.ReadFault()
	if err != nil {
		t.Fatal(err)
	}
	if f != 7 {
		t.Errorf("ReadFault = %d, want 7", f)
	}
}

// ── WriteParam ────────────────────────────────────────────────────────────────

func TestWriteParam_OK(t *testing.T) {
	m, bus := newMotorMock()
	if err := m.WriteParam(ParamInternalSpd1, 100); err != nil {
		t.Fatalf("WriteParam: %v", err)
	}
	if bus.client.lastWriteAddr != ParamInternalSpd1 {
		t.Errorf("write addr = %d, want %d", bus.client.lastWriteAddr, ParamInternalSpd1)
	}
	if bus.client.lastWriteVal != 100 {
		t.Errorf("write val = %d, want 100", bus.client.lastWriteVal)
	}
}

func TestWriteParam_Error(t *testing.T) {
	m, bus := newMotorMock()
	bus.client.writeErr = errors.New("write failed")
	if err := m.WriteParam(0, 0); err == nil {
		t.Fatal("expected error, got nil")
	}
}

// ── Enable / Disable ──────────────────────────────────────────────────────────

func TestEnable_OK(t *testing.T) {
	m, bus := newMotorMock()
	bus.rawResp = []byte{0x01, 0x42} // valid FC42 echo
	if err := m.Enable(); err != nil {
		t.Fatalf("Enable: %v", err)
	}
}

func TestEnable_ShortResponse(t *testing.T) {
	m, bus := newMotorMock()
	bus.rawResp = []byte{0x01} // only 1 byte — too short
	if err := m.Enable(); err == nil {
		t.Fatal("expected error for short response, got nil")
	}
}

func TestEnable_WrongFC(t *testing.T) {
	m, bus := newMotorMock()
	bus.rawResp = []byte{0x01, 0x41} // wrong FC byte (0x41 instead of 0x42)
	if err := m.Enable(); err == nil {
		t.Fatal("expected error for wrong FC, got nil")
	}
}

func TestEnable_BusError(t *testing.T) {
	m, bus := newMotorMock()
	bus.rawErr = errors.New("serial timeout")
	if err := m.Enable(); err == nil {
		t.Fatal("expected error from bus, got nil")
	}
}

func TestDisable_OK(t *testing.T) {
	m, bus := newMotorMock()
	bus.rawResp = []byte{0x01, 0x42}
	if err := m.Disable(); err != nil {
		t.Fatalf("Disable: %v", err)
	}
}

func TestDisable_Error(t *testing.T) {
	m, bus := newMotorMock()
	bus.rawErr = errors.New("timeout")
	if err := m.Disable(); err == nil {
		t.Fatal("expected error, got nil")
	}
}

// ── SetTorqueLimit ────────────────────────────────────────────────────────────

func TestSetTorqueLimit_Clamp(t *testing.T) {
	m, bus := newMotorMock()

	// Below 0 → clamps to 0.
	if err := m.SetTorqueLimit(-10); err != nil {
		t.Fatal(err)
	}
	if bus.client.lastWriteVal != 0 {
		t.Errorf("clamped below 0: got %d, want 0", bus.client.lastWriteVal)
	}

	// Above 300 → clamps to 300.
	if err := m.SetTorqueLimit(400); err != nil {
		t.Fatal(err)
	}
	if bus.client.lastWriteVal != 300 {
		t.Errorf("clamped above 300: got %d, want 300", bus.client.lastWriteVal)
	}

	// In range → passed through.
	if err := m.SetTorqueLimit(75); err != nil {
		t.Fatal(err)
	}
	if bus.client.lastWriteVal != 75 {
		t.Errorf("in-range: got %d, want 75", bus.client.lastWriteVal)
	}
}

// ── SetAccelTime / SetDecelTime ───────────────────────────────────────────────

func TestSetAccelTime_Clamp(t *testing.T) {
	m, bus := newMotorMock()

	if err := m.SetAccelTime(0); err != nil {
		t.Fatal(err)
	}
	if bus.client.lastWriteVal != 1 {
		t.Errorf("clamp below 1: got %d, want 1", bus.client.lastWriteVal)
	}

	if err := m.SetAccelTime(99999); err != nil {
		t.Fatal(err)
	}
	if bus.client.lastWriteVal != 30000 {
		t.Errorf("clamp above 30000: got %d, want 30000", bus.client.lastWriteVal)
	}

	if err := m.SetAccelTime(500); err != nil {
		t.Fatal(err)
	}
	if bus.client.lastWriteVal != 500 {
		t.Errorf("in-range: got %d, want 500", bus.client.lastWriteVal)
	}
}

func TestSetDecelTime_Clamp(t *testing.T) {
	m, bus := newMotorMock()

	if err := m.SetDecelTime(0); err != nil {
		t.Fatal(err)
	}
	if bus.client.lastWriteVal != 1 {
		t.Errorf("clamp below 1: got %d, want 1", bus.client.lastWriteVal)
	}
}

// ── ReadMotionState ───────────────────────────────────────────────────────────

func TestReadMotionState(t *testing.T) {
	m, bus := newMotorMock()

	// Batch 1 (FC04 @ 0x09, qty=1): torque = 55
	bus.client.readInput[StatusTorquePct] = u16be(uint16(int16(55)))

	// Batch 2 (FC04 @ 0x1A, qty=7): 7 registers = 14 bytes
	// offset 0  (0x1A) = fault code = 0
	// offset 10 (0x1F) = absL = 0x0010 = 16
	// offset 12 (0x20) = absH = 0x0000
	b2 := make([]byte, 14)
	binary.BigEndian.PutUint16(b2[0:], 0)      // fault
	binary.BigEndian.PutUint16(b2[10:], 0x0010) // absL = 16
	binary.BigEndian.PutUint16(b2[12:], 0x0000) // absH = 0
	bus.client.readInput[StatusFaultCode] = b2

	pos, torque, fault, err := m.ReadMotionState()
	if err != nil {
		t.Fatalf("ReadMotionState: %v", err)
	}
	if torque != 55 {
		t.Errorf("torque = %d, want 55", torque)
	}
	if fault != 0 {
		t.Errorf("fault = %d, want 0", fault)
	}
	if pos != 16 {
		t.Errorf("pos = %d, want 16", pos)
	}
}

func TestReadMotionState_Error(t *testing.T) {
	m, _ := newMotorMock()
	m.bus = &errorBus{err: errors.New("bus failure")}
	_, _, _, err := m.ReadMotionState()
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

// ── ReadParam ─────────────────────────────────────────────────────────────────

func TestReadParam_OK(t *testing.T) {
	m, bus := newMotorMock()
	bus.client.readHolding[ParamInternalSpd1] = u16be(1500)
	v, err := m.ReadParam(ParamInternalSpd1)
	if err != nil {
		t.Fatalf("ReadParam: %v", err)
	}
	if v != 1500 {
		t.Errorf("ReadParam = %d, want 1500", v)
	}
}

func TestReadParam_Error(t *testing.T) {
	m, _ := newMotorMock()
	m.bus = &errorBus{err: errors.New("fail")}
	_, err := m.ReadParam(0)
	if err == nil {
		t.Fatal("expected error")
	}
}

// ── ReadStatus ────────────────────────────────────────────────────────────────

func TestReadStatus_OK(t *testing.T) {
	m, bus := newMotorMock()

	// Provide the 6 batches. Unset addresses return zeroed bytes.
	// SpeedRPM = 120 at offset 0 within batch 0x00.
	b1 := make([]byte, 16)
	binary.BigEndian.PutUint16(b1[0:], 120) // SpeedRPM
	bus.client.readInput[0x00] = b1

	// TorquePct = 40 at offset (0x09-0x08)*2 = 2 within batch 0x08.
	b2 := make([]byte, 16)
	binary.BigEndian.PutUint16(b2[2:], uint16(int16(40)))
	bus.client.readInput[0x08] = b2

	bus.client.readInput[0x10] = make([]byte, 16)
	bus.client.readInput[0x18] = make([]byte, 8)
	bus.client.readInput[0x26] = make([]byte, 6)

	// AbsPos = 999
	bpos := make([]byte, 4)
	binary.BigEndian.PutUint16(bpos[0:], 999) // lo
	binary.BigEndian.PutUint16(bpos[2:], 0)   // hi
	bus.client.readInput[StatusAbsMotorPosL] = bpos

	st, err := m.ReadStatus()
	if err != nil {
		t.Fatalf("ReadStatus: %v", err)
	}
	if st.SpeedRPM != 120 {
		t.Errorf("SpeedRPM = %d, want 120", st.SpeedRPM)
	}
	if st.TorquePct != 40 {
		t.Errorf("TorquePct = %d, want 40", st.TorquePct)
	}
	if st.Position32 != 999 {
		t.Errorf("Position32 = %d, want 999", st.Position32)
	}
}

func TestReadStatus_Error(t *testing.T) {
	m, _ := newMotorMock()
	m.bus = &errorBus{err: errors.New("fail")}
	_, err := m.ReadStatus()
	if err == nil {
		t.Fatal("expected error")
	}
}

// ── SetSpeed ──────────────────────────────────────────────────────────────────

func TestSetSpeed_OK(t *testing.T) {
	m, bus := newMotorMock()
	bus.rawResp = []byte{0x01, 0x42} // valid FC42 for Disable + Enable

	if err := m.SetSpeed(100); err != nil {
		t.Fatalf("SetSpeed: %v", err)
	}
	// P-137 = ParamInternalSpd1 must be written with 100.
	if bus.client.lastWriteAddr != ParamInternalSpd1 {
		t.Errorf("write addr = %d, want %d", bus.client.lastWriteAddr, ParamInternalSpd1)
	}
	if bus.client.lastWriteVal != 100 {
		t.Errorf("write val = %d, want 100", bus.client.lastWriteVal)
	}
}

func TestSetSpeed_NegativeRPM(t *testing.T) {
	m, bus := newMotorMock()
	bus.rawResp = []byte{0x01, 0x42}

	if err := m.SetSpeed(-50); err != nil {
		t.Fatalf("SetSpeed: %v", err)
	}
	// uint16(int16(-50)) = 0xFFCE = 65486
	var neg50 int16 = -50
	want := uint16(neg50)
	if bus.client.lastWriteVal != want {
		t.Errorf("write val = %d, want %d", bus.client.lastWriteVal, want)
	}
}

// ── NewMotor / ReadSpeed / ReadPosition ───────────────────────────────────────

func TestNewMotor(t *testing.T) {
	_, bus := newMotorMock()
	m := NewMotor(&Bus{}, 3)
	// Just verify field wiring.
	_ = m
	_ = bus
	if m.slaveID != 3 {
		t.Errorf("slaveID = %d, want 3", m.slaveID)
	}
}

func TestReadSpeed(t *testing.T) {
	m, bus := newMotorMock()
	bus.client.readInput[StatusSpeed] = u16be(500)
	v, err := m.ReadSpeed()
	if err != nil {
		t.Fatal(err)
	}
	if v != 500 {
		t.Errorf("ReadSpeed = %d, want 500", v)
	}
}

func TestReadPosition(t *testing.T) {
	m, bus := newMotorMock()
	b := make([]byte, 4)
	binary.BigEndian.PutUint16(b[0:], 42) // lo
	binary.BigEndian.PutUint16(b[2:], 0)  // hi
	bus.client.readInput[StatusAbsMotorPosL] = b
	pos, err := m.ReadPosition()
	if err != nil {
		t.Fatal(err)
	}
	if pos != 42 {
		t.Errorf("ReadPosition = %d, want 42", pos)
	}
}

// ── WriteParams ───────────────────────────────────────────────────────────────

func TestWriteParams_OK(t *testing.T) {
	m, _ := newMotorMock()
	if err := m.WriteParams(ParamAccelTime, []uint16{100, 200}); err != nil {
		t.Fatalf("WriteParams: %v", err)
	}
}

func TestWriteParams_Error(t *testing.T) {
	m, bus := newMotorMock()
	bus.client.writeErr = errors.New("write failed")
	if err := m.WriteParams(0, []uint16{0}); err == nil {
		t.Fatal("expected error")
	}
}

// ── ReadConfig ────────────────────────────────────────────────────────────────

func TestReadConfig(t *testing.T) {
	m, bus := newMotorMock()
	bus.client.readHolding[ParamInternalSpd1] = u16be(uint16(int16(300)))
	cfg, err := m.ReadConfig()
	if err != nil {
		t.Fatalf("ReadConfig: %v", err)
	}
	if cfg.InternalSpd1 != 300 {
		t.Errorf("InternalSpd1 = %d, want 300", cfg.InternalSpd1)
	}
}

// ── SetupSpeedMode ────────────────────────────────────────────────────────────

func TestSetupSpeedMode_OK(t *testing.T) {
	m, _ := newMotorMock()
	if err := m.SetupSpeedMode(100); err != nil {
		t.Fatalf("SetupSpeedMode: %v", err)
	}
}

func TestSetupSpeedMode_Error(t *testing.T) {
	m, bus := newMotorMock()
	bus.client.writeErr = errors.New("fail")
	if err := m.SetupSpeedMode(100); err == nil {
		t.Fatal("expected error")
	}
}

// ── SaveEEPROM ────────────────────────────────────────────────────────────────

func TestSaveEEPROM_OK(t *testing.T) {
	m, bus := newMotorMock()
	bus.rawResp = []byte{0x01, 0x41}
	if err := m.SaveEEPROM(); err != nil {
		t.Fatalf("SaveEEPROM: %v", err)
	}
}

func TestSaveEEPROM_Error(t *testing.T) {
	m, bus := newMotorMock()
	bus.rawErr = errors.New("fail")
	if err := m.SaveEEPROM(); err == nil {
		t.Fatal("expected error")
	}
}

func TestSaveEEPROM_WrongFC(t *testing.T) {
	m, bus := newMotorMock()
	bus.rawResp = []byte{0x01, 0x42} // wrong FC
	if err := m.SaveEEPROM(); err == nil {
		t.Fatal("expected error for wrong FC in SaveEEPROM")
	}
}

// ── MoveByPulses edge cases ───────────────────────────────────────────────────

func TestMoveByPulses_ZeroRPM(t *testing.T) {
	m, _ := newMotorMock()
	if err := m.MoveByPulses(context.TODO(), 100, 0); err == nil {
		t.Fatal("expected error for speedRPM=0")
	}
}

func TestMoveByPulses_NegativeRPM(t *testing.T) {
	m, _ := newMotorMock()
	if err := m.MoveByPulses(context.TODO(), 100, -5); err == nil {
		t.Fatal("expected error for negative speedRPM")
	}
}

func TestMoveByPulses_ZeroPulses(t *testing.T) {
	m, _ := newMotorMock()
	// pulses=0 returns nil immediately without any bus calls.
	if err := m.MoveByPulses(context.TODO(), 0, 100); err != nil {
		t.Fatalf("MoveByPulses(0 pulses): unexpected error: %v", err)
	}
}

func TestMoveByPulses_ContextCancelled(t *testing.T) {
	m, bus := newMotorMock()
	bus.rawResp = []byte{0x01, 0x42}

	// Give a position that keeps remaining > 50 so the loop won't break naturally.
	// readMotionState: pos=0, so remaining=1000-0=1000 > 50.
	bus.client.readInput[StatusAbsMotorPosL] = make([]byte, 4)
	bus.client.readInput[StatusFaultCode] = make([]byte, 14)
	bus.client.readInput[StatusTorquePct] = make([]byte, 2)

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately so the loop exits on first iteration

	err := m.MoveByPulses(ctx, 1000, 100)
	if err == nil {
		t.Fatal("expected cancellation error")
	}
}

func TestMoveByPulses_TorqueSafetyTrip(t *testing.T) {
	m, bus := newMotorMock()
	bus.rawResp = []byte{0x01, 0x42}
	bus.client.readInput[StatusAbsMotorPosL] = make([]byte, 4)
	bus.client.readInput[StatusFaultCode] = make([]byte, 14)

	// torque = 85% >= moveTorqueSafety(80) → safety trip
	var t85 int16 = 85
	bus.client.readInput[StatusTorquePct] = u16be(uint16(t85))

	err := m.MoveByPulses(context.Background(), 1000, 100)
	if err == nil {
		t.Fatal("expected torque safety trip error")
	}
}

func TestMoveByPulses_FaultCode(t *testing.T) {
	m, bus := newMotorMock()
	bus.rawResp = []byte{0x01, 0x42}
	bus.client.readInput[StatusAbsMotorPosL] = make([]byte, 4)
	bus.client.readInput[StatusTorquePct] = make([]byte, 2)

	// fault = 7
	b2 := make([]byte, 14)
	binary.BigEndian.PutUint16(b2[0:], 7) // fault code
	bus.client.readInput[StatusFaultCode] = b2

	err := m.MoveByPulses(context.Background(), 1000, 100)
	if err == nil {
		t.Fatal("expected fault error")
	}
}

func TestMoveByPulses_NormalPath(t *testing.T) {
	// Small move (10 pulses) so readMotionState returns already-at-target position.
	// Total sleep budget: 80 ms (disableWait) + 60 ms (approach) + 150 ms (stopSettle) = ~290 ms.
	m, bus := newMotorMock()
	bus.rawResp = []byte{0x01, 0x42}

	// ReadAbsPosition (StatusAbsMotorPosL) → start pos = 0.
	bus.client.readInput[StatusAbsMotorPosL] = make([]byte, 4)

	// readMotionState batch 2 (StatusFaultCode, 7 regs = 14 bytes):
	//   bytes[0:2]  = fault = 0
	//   bytes[10:12] = absL = 10  (position 10, within tolerance 50)
	//   bytes[12:14] = absH = 0
	b2 := make([]byte, 14)
	binary.BigEndian.PutUint16(b2[10:], 10)
	bus.client.readInput[StatusFaultCode] = b2

	// readMotionState batch 1 (StatusTorquePct, 1 reg = 2 bytes): torque = 0.
	bus.client.readInput[StatusTorquePct] = make([]byte, 2)

	if err := m.MoveByPulses(context.Background(), 10, 100); err != nil {
		t.Fatalf("MoveByPulses: %v", err)
	}
}

func TestMoveByPulses_WithCorrectionPass(t *testing.T) {
	// 1000-pulse move. readMotionState returns pos=950 (remaining=50, within
	// tolerance) so the main loop exits immediately.
	// correctTo finds ReadAbsPosition=0 (error=1000>150) and runs its loop
	// once (readMotionState pos=950, remaining=50≤50 → break).
	// Total sleep: 80+60+150 ms = ~290 ms (no poll sleeps, loop exits immediately).
	m, bus := newMotorMock()
	bus.rawResp = []byte{0x01, 0x42}

	// ReadAbsPosition → 0 (used for startPos and correctTo actual).
	bus.client.readInput[StatusAbsMotorPosL] = make([]byte, 4)

	// readMotionState: pos=950 (absL=950), torque=0, fault=0.
	b2 := make([]byte, 14)
	binary.BigEndian.PutUint16(b2[10:], 950) // absL
	bus.client.readInput[StatusFaultCode] = b2
	bus.client.readInput[StatusTorquePct] = make([]byte, 2)

	if err := m.MoveByPulses(context.Background(), 1000, 100); err != nil {
		t.Fatalf("MoveByPulses with correction: %v", err)
	}
}

// ── RunUntilTorque ────────────────────────────────────────────────────────────

func TestRunUntilTorque_CancelledImmediately(t *testing.T) {
	m, bus := newMotorMock()
	bus.rawResp = []byte{0x01, 0x42}

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel before calling

	err := m.RunUntilTorque(ctx, 25, 50)
	if err == nil {
		t.Fatal("expected context cancellation error")
	}
}

func TestRunUntilTorque_SetSpeedError(t *testing.T) {
	m, bus := newMotorMock()
	// txRaw succeeds (Disable in SetSpeed), but tx fails (WriteParam in SetSpeed).
	bus.rawResp = []byte{0x01, 0x42}
	bus.client.writeErr = errors.New("param write fail")

	err := m.RunUntilTorque(context.Background(), 25, 50)
	if err == nil {
		t.Fatal("expected error when SetSpeed fails")
	}
}

func TestRunUntilTorque_TorqueReached(t *testing.T) {
	m, bus := newMotorMock()
	bus.rawResp = []byte{0x01, 0x42} // valid FC42

	// torque = 60 >= maxTorquePct=50 → immediate stop on first poll
	bus.client.readInput[StatusTorquePct] = u16be(60)
	bus.client.readInput[StatusFaultCode] = make([]byte, 14) // no fault, zero pos

	err := m.RunUntilTorque(context.Background(), 25, 50)
	if err != nil {
		t.Fatalf("RunUntilTorque: %v", err)
	}
}
