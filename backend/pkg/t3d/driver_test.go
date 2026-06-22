package t3d

import (
	"errors"
	"testing"
)

// newDriverMock returns a Driver with a mockBus-backed Motor.
// The Driver.bus field is left nil because Connect/Close are hardware-only.
func newDriverMock() (*Driver, *mockBus) {
	mc := &mockClient{
		readInput:   make(map[uint16][]byte),
		readHolding: make(map[uint16][]byte),
	}
	mb := &mockBus{
		client:  mc,
		rawResp: []byte{0x01, 0x42},
	}
	m := &Motor{bus: mb, slaveID: 2}
	d := &Driver{motor: m}
	return d, mb
}

func TestDriver_SetSlaveID(t *testing.T) {
	d, _ := newDriverMock()
	d.SetSlaveID(5)
	if d.motor.slaveID != 5 {
		t.Errorf("slaveID = %d, want 5", d.motor.slaveID)
	}
}

func TestDriver_ReadParam(t *testing.T) {
	d, mb := newDriverMock()
	mb.client.readHolding[ParamAccelTime] = u16be(200)
	v, err := d.ReadParam(ParamAccelTime)
	if err != nil {
		t.Fatal(err)
	}
	if v != 200 {
		t.Errorf("ReadParam = %d, want 200", v)
	}
}

func TestDriver_WriteParam(t *testing.T) {
	d, mb := newDriverMock()
	if err := d.WriteParam(ParamDecelTime, 150); err != nil {
		t.Fatal(err)
	}
	if mb.client.lastWriteAddr != ParamDecelTime {
		t.Errorf("addr = %d, want %d", mb.client.lastWriteAddr, ParamDecelTime)
	}
}

func TestDriver_WriteParams(t *testing.T) {
	d, _ := newDriverMock()
	if err := d.WriteParams(ParamAccelTime, []uint16{100, 100}); err != nil {
		t.Fatal(err)
	}
}

func TestDriver_ReadInputReg(t *testing.T) {
	d, mb := newDriverMock()
	mb.client.readInput[StatusSpeed] = u16be(300)
	v, err := d.ReadInputReg(StatusSpeed)
	if err != nil {
		t.Fatal(err)
	}
	if v != 300 {
		t.Errorf("ReadInputReg = %d, want 300", v)
	}
}

func TestDriver_ReadStatus(t *testing.T) {
	d, mb := newDriverMock()
	mb.client.readInput[0x00] = make([]byte, 16)
	mb.client.readInput[0x08] = make([]byte, 16)
	mb.client.readInput[0x10] = make([]byte, 16)
	mb.client.readInput[0x18] = make([]byte, 8)
	mb.client.readInput[0x26] = make([]byte, 6)
	mb.client.readInput[StatusAbsMotorPosL] = make([]byte, 4)

	st, err := d.ReadStatus()
	if err != nil {
		t.Fatalf("ReadStatus: %v", err)
	}
	_ = st
}

func TestDriver_ReadConfig(t *testing.T) {
	d, _ := newDriverMock()
	cfg, err := d.ReadConfig()
	if err != nil {
		t.Fatalf("ReadConfig: %v", err)
	}
	_ = cfg
}

func TestDriver_SetupSpeedMode(t *testing.T) {
	d, _ := newDriverMock()
	if err := d.SetupSpeedMode(50); err != nil {
		t.Fatal(err)
	}
}

func TestDriver_SetSpeedPreset_NoResume(t *testing.T) {
	d, _ := newDriverMock()
	if err := d.SetSpeedPreset(100, false); err != nil {
		t.Fatal(err)
	}
}

func TestDriver_SetSpeedPreset_WithResume(t *testing.T) {
	d, _ := newDriverMock()
	if err := d.SetSpeedPreset(100, true); err != nil {
		t.Fatal(err)
	}
}

func TestDriver_ServoEnable(t *testing.T) {
	d, _ := newDriverMock()
	if err := d.ServoEnable(); err != nil {
		t.Fatal(err)
	}
}

func TestDriver_ServoDisable(t *testing.T) {
	d, _ := newDriverMock()
	if err := d.ServoDisable(); err != nil {
		t.Fatal(err)
	}
}

func TestDriver_SaveEEPROM(t *testing.T) {
	d, mb := newDriverMock()
	mb.rawResp = []byte{0x01, 0x41}
	if err := d.SaveEEPROM(); err != nil {
		t.Fatal(err)
	}
}

func TestDriver_SaveEEPROMAlt(t *testing.T) {
	d, _ := newDriverMock()
	if err := d.SaveEEPROMAlt(); err != nil {
		t.Fatal(err)
	}
}

func TestDriver_Motor(t *testing.T) {
	d, _ := newDriverMock()
	if d.Motor() == nil {
		t.Error("Motor() returned nil")
	}
}

// ── New / Bus / String ────────────────────────────────────────────────────────

func TestNew_FieldsWired(t *testing.T) {
	d := New("/dev/null", 9600, 3)
	if d.motor == nil {
		t.Error("motor is nil after New")
	}
	if d.bus == nil {
		t.Error("bus is nil after New")
	}
	if d.motor.slaveID != 3 {
		t.Errorf("slaveID = %d, want 3", d.motor.slaveID)
	}
}

func TestDriver_Bus(t *testing.T) {
	d := New("/dev/null", 9600, 1)
	if d.Bus() == nil {
		t.Error("Bus() returned nil")
	}
}

func TestDriver_String(t *testing.T) {
	d := New("/dev/null", 9600, 2)
	s := d.String()
	if s == "" {
		t.Error("String() returned empty")
	}
}

func TestDriver_SetSpeedPreset_Error(t *testing.T) {
	d, mb := newDriverMock()
	mb.rawErr = errors.New("disable fail") // Disable fails
	if err := d.SetSpeedPreset(100, false); err == nil {
		t.Fatal("expected error when Disable fails")
	}
}
