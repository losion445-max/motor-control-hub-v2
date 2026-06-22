package robot

import (
	"context"
	"errors"
	"testing"

	"github.com/losion445-max/motor-control-hub-v2/pkg/t3d"
)

// ── mock driveMotor ───────────────────────────────────────────────────────────

type mockMotor struct {
	absPos    int32
	torque    int16
	fault     uint16
	status    *t3d.Status
	enableErr error
	disableErr error
	writeErr  error
	readPosErr error
	readTorqueErr error
	readFaultErr  error
	readStatusErr error
	motionErr error
	accelErr  error
	decelErr  error
	speedErr  error
	torqLimErr error
	callsDisable int
}

func (m *mockMotor) Enable() error                                           { return m.enableErr }
func (m *mockMotor) Disable() error                                          { m.callsDisable++; return m.disableErr }
func (m *mockMotor) WriteParam(_ uint16, _ uint16) error                     { return m.writeErr }
func (m *mockMotor) ReadAbsPosition() (int32, error)                         { return m.absPos, m.readPosErr }
func (m *mockMotor) ReadTorquePct() (int16, error)                           { return m.torque, m.readTorqueErr }
func (m *mockMotor) ReadFault() (uint16, error)                              { return m.fault, m.readFaultErr }
func (m *mockMotor) ReadStatus() (*t3d.Status, error)                        { return m.status, m.readStatusErr }
func (m *mockMotor) ReadMotionState() (int32, int16, uint16, error) {
	return m.absPos, m.torque, m.fault, m.motionErr
}
func (m *mockMotor) SetAccelTime(_ int) error  { return m.accelErr }
func (m *mockMotor) SetDecelTime(_ int) error  { return m.decelErr }
func (m *mockMotor) SetSpeed(_ int) error      { return m.speedErr }
func (m *mockMotor) SetTorqueLimit(_ int) error { return m.torqLimErr }

// newTestSystem builds a System with 4 identical mockMotors (no real Bus).
func newTestSystem(cfg Config) (*System, [4]*mockMotor) {
	s := &System{cfg: cfg}
	var mocks [4]*mockMotor
	for i := range 4 {
		mocks[i] = &mockMotor{}
		s.motors[i] = mocks[i]
	}
	return s, mocks
}

func defaultCfg() Config {
	c := DefaultConfig
	// Short timeouts so tests finish quickly.
	c.PollInterval = 0
	c.StopSettle = 0
	c.DisableWait = 0
	c.ApproachSwitch = 0
	return c
}

// ── Position / Homed getters ──────────────────────────────────────────────────

func TestPosition_Initial(t *testing.T) {
	s, _ := newTestSystem(defaultCfg())
	x, y := s.Position()
	if x != 0 || y != 0 {
		t.Errorf("Position() = (%v,%v), want (0,0)", x, y)
	}
}

func TestHomed_Initial(t *testing.T) {
	s, _ := newTestSystem(defaultCfg())
	if s.Homed() {
		t.Error("Homed() = true before Home, want false")
	}
}

// ── EmergencyStop ─────────────────────────────────────────────────────────────

func TestEmergencyStop_AllDisabled(t *testing.T) {
	s, mocks := newTestSystem(defaultCfg())
	if err := s.EmergencyStop(); err != nil {
		t.Fatalf("EmergencyStop: %v", err)
	}
	for i, m := range mocks {
		if m.callsDisable != 1 {
			t.Errorf("motor %d: Disable called %d times, want 1", i+1, m.callsDisable)
		}
	}
}

func TestEmergencyStop_FirstError(t *testing.T) {
	s, mocks := newTestSystem(defaultCfg())
	mocks[0].disableErr = errors.New("motor 1 off")

	err := s.EmergencyStop()
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	// All motors must still be attempted despite error on motor 0.
	for i, m := range mocks {
		if m.callsDisable != 1 {
			t.Errorf("motor %d: Disable called %d times, want 1", i+1, m.callsDisable)
		}
	}
}

// ── ReadAllStatus ─────────────────────────────────────────────────────────────

func TestReadAllStatus_OK(t *testing.T) {
	s, mocks := newTestSystem(defaultCfg())
	mocks[0].status = &t3d.Status{SpeedRPM: 42}
	mocks[1].status = &t3d.Status{TorquePct: 10}

	states := s.ReadAllStatus()
	if states[0].Status.SpeedRPM != 42 {
		t.Errorf("motor 1 speed = %d, want 42", states[0].Status.SpeedRPM)
	}
	if states[1].Status.TorquePct != 10 {
		t.Errorf("motor 2 torque = %d, want 10", states[1].Status.TorquePct)
	}
	for i, st := range states {
		if st.ID != i+1 {
			t.Errorf("state[%d].ID = %d, want %d", i, st.ID, i+1)
		}
	}
}

func TestReadAllStatus_WithError(t *testing.T) {
	s, mocks := newTestSystem(defaultCfg())
	mocks[2].readStatusErr = errors.New("read fail")

	states := s.ReadAllStatus()
	if states[2].Err == nil {
		t.Error("state[2].Err should be non-nil")
	}
	if states[2].Status != nil {
		t.Error("state[2].Status should be nil on error")
	}
}

// ── Home ─────────────────────────────────────────────────────────────────────

func TestHome_Success(t *testing.T) {
	s, mocks := newTestSystem(defaultCfg())

	// Torque immediately above threshold for all motors so homing completes in one poll.
	torqueThreshold := int16(s.cfg.HomingTorquePct) + 5
	for _, m := range mocks {
		m.torque = torqueThreshold
		m.absPos = 1000
	}

	if err := s.Home(context.Background()); err != nil {
		t.Fatalf("Home: %v", err)
	}
	if !s.Homed() {
		t.Error("Homed() = false after successful Home")
	}
	x, y := s.Position()
	if x != s.cfg.WidthMM/2 || y != s.cfg.HeightMM/2 {
		t.Errorf("Position after home = (%v,%v), want (%v,%v)",
			x, y, s.cfg.WidthMM/2, s.cfg.HeightMM/2)
	}
}

func TestHome_SetTorqueLimitError(t *testing.T) {
	s, mocks := newTestSystem(defaultCfg())
	mocks[0].torqLimErr = errors.New("torque limit fail")

	if err := s.Home(context.Background()); err == nil {
		t.Fatal("expected error when SetTorqueLimit fails")
	}
}

func TestHome_WriteParamError(t *testing.T) {
	s, mocks := newTestSystem(defaultCfg())
	mocks[0].writeErr = errors.New("write fail")

	if err := s.Home(context.Background()); err == nil {
		t.Fatal("expected error when WriteParam fails")
	}
}

func TestHome_EnableError(t *testing.T) {
	s, mocks := newTestSystem(defaultCfg())
	mocks[0].enableErr = errors.New("enable fail")

	if err := s.Home(context.Background()); err == nil {
		t.Fatal("expected error when Enable fails")
	}
}

func TestHome_ReadTorqueError(t *testing.T) {
	s, mocks := newTestSystem(defaultCfg())
	mocks[1].readTorqueErr = errors.New("torque read fail")

	if err := s.Home(context.Background()); err == nil {
		t.Fatal("expected error when ReadTorquePct fails")
	}
}

func TestHome_ContextCancelled(t *testing.T) {
	s, mocks := newTestSystem(defaultCfg())
	// Keep torque below threshold so homing never completes naturally.
	for _, m := range mocks {
		m.torque = 0
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := s.Home(ctx)
	if err == nil {
		t.Fatal("expected cancellation error")
	}
}

// ── HoldTension ───────────────────────────────────────────────────────────────

func TestHoldTension_OK(t *testing.T) {
	s, _ := newTestSystem(defaultCfg())
	if err := s.HoldTension(); err != nil {
		t.Fatalf("HoldTension: %v", err)
	}
}

func TestHoldTension_TorqueLimitError(t *testing.T) {
	s, mocks := newTestSystem(defaultCfg())
	mocks[0].torqLimErr = errors.New("torque fail")

	if err := s.HoldTension(); err == nil {
		t.Fatal("expected error from SetTorqueLimit")
	}
}

func TestHoldTension_SetSpeedError(t *testing.T) {
	s, mocks := newTestSystem(defaultCfg())
	mocks[0].speedErr = errors.New("speed fail")

	if err := s.HoldTension(); err == nil {
		t.Fatal("expected error from SetSpeed")
	}
}

// ── MoveTo ────────────────────────────────────────────────────────────────────

func TestMoveTo_NotHomed(t *testing.T) {
	s, _ := newTestSystem(defaultCfg())
	if err := s.MoveTo(context.Background(), 700, 1200, 50); err == nil {
		t.Fatal("expected error when not homed")
	}
}

func TestMoveTo_NearTarget(t *testing.T) {
	// If already at the target position, MoveTo should be a no-op.
	s, mocks := newTestSystem(defaultCfg())
	s.homed = true
	s.posX = 700
	s.posY = 1200
	s.homeLenMM = homeLength(s.cfg.WidthMM, s.cfg.HeightMM)

	// homePos=0, homeLenMM computed from centre → currentCableLengths
	// will compute position relative to home. Set absPos=0 so current lengths
	// equal home lengths; then cableLengths at (700,1200) vs home (700,1200) → delta≈0.
	for _, m := range mocks {
		m.absPos = 0
	}

	// Move to essentially the same spot (< 0.5 mm away).
	if err := s.MoveTo(context.Background(), 700.0, 1200.0, 50); err != nil {
		t.Fatalf("MoveTo near target: %v", err)
	}
}

func TestMoveTo_ReadCableLengthsError(t *testing.T) {
	s, mocks := newTestSystem(defaultCfg())
	s.homed = true
	mocks[0].readPosErr = errors.New("pos read fail")

	if err := s.MoveTo(context.Background(), 700, 1200, 50); err == nil {
		t.Fatal("expected error from ReadAbsPosition")
	}
}

func TestMoveTo_ExecutesMove(t *testing.T) {
	// Move 2 mm right of centre — each cable changes ~24 pulses, well within
	// TolerancePulses so the move completes in a single poll iteration.
	cfg := defaultCfg()
	s, mocks := newTestSystem(cfg)
	s.homed = true
	s.posX = cfg.WidthMM / 2
	s.posY = cfg.HeightMM / 2
	s.homeLenMM = homeLength(cfg.WidthMM, cfg.HeightMM)
	for _, m := range mocks {
		m.absPos = 0
	}

	target := cfg.WidthMM/2 + 2.0
	if err := s.MoveTo(context.Background(), target, cfg.HeightMM/2, 50); err != nil {
		t.Fatalf("MoveTo: %v", err)
	}
	x, _ := s.Position()
	if x != target {
		t.Errorf("Position().X = %v, want %v", x, target)
	}
}

func TestMoveTo_ContextCancelled(t *testing.T) {
	cfg := defaultCfg()
	// Large TolerancePulses so the loop won't exit naturally; rely on ctx cancel.
	// We need large move pulses that don't complete immediately.
	// But context is already cancelled, so first select should pick it up.
	s, mocks := newTestSystem(cfg)
	s.homed = true
	s.posX = cfg.WidthMM / 2
	s.posY = cfg.HeightMM / 2
	s.homeLenMM = homeLength(cfg.WidthMM, cfg.HeightMM)
	for _, m := range mocks {
		m.absPos = 0
	}
	// Override tolerance to 0 so remaining is never ≤ tolerance without traveling.
	cfg.TolerancePulses = 0
	s.cfg = cfg

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := s.MoveTo(ctx, cfg.WidthMM/2+200, cfg.HeightMM/2, 50)
	if err == nil {
		t.Fatal("expected cancellation error")
	}
}

func TestMoveTo_PollError(t *testing.T) {
	cfg := defaultCfg()
	cfg.TolerancePulses = 0 // never auto-complete
	s, mocks := newTestSystem(cfg)
	s.homed = true
	s.posX = cfg.WidthMM / 2
	s.posY = cfg.HeightMM / 2
	s.homeLenMM = homeLength(cfg.WidthMM, cfg.HeightMM)
	// ReadAbsPosition succeeds, but ReadMotionState fails.
	for _, m := range mocks {
		m.absPos = 0
		m.motionErr = errors.New("poll fail")
	}

	if err := s.MoveTo(context.Background(), cfg.WidthMM/2+200, cfg.HeightMM/2, 50); err == nil {
		t.Fatal("expected poll error")
	}
}

// ── collectiveSlowdown ────────────────────────────────────────────────────────

func TestCollectiveSlowdown_OK(t *testing.T) {
	s, _ := newTestSystem(defaultCfg())
	done := [4]bool{false, false, true, true}
	pulses := [4]int64{1000, -500, 0, 0}
	speeds := [4]int{50, 25, 0, 0}

	if err := s.collectiveSlowdown(done, pulses, speeds, 50); err != nil {
		t.Fatalf("collectiveSlowdown: %v", err)
	}
}

func TestCollectiveSlowdown_WriteError(t *testing.T) {
	s, mocks := newTestSystem(defaultCfg())
	mocks[0].writeErr = errors.New("write fail")
	done := [4]bool{}
	pulses := [4]int64{1000, 0, 0, 0}
	speeds := [4]int{50, 0, 0, 0}

	if err := s.collectiveSlowdown(done, pulses, speeds, 50); err == nil {
		t.Fatal("expected error from WriteParam")
	}
}

func TestCollectiveSlowdown_EnableError(t *testing.T) {
	s, mocks := newTestSystem(defaultCfg())
	mocks[0].enableErr = errors.New("enable fail")
	done := [4]bool{}
	pulses := [4]int64{1000, 0, 0, 0}
	speeds := [4]int{50, 0, 0, 0}

	if err := s.collectiveSlowdown(done, pulses, speeds, 50); err == nil {
		t.Fatal("expected error from Enable")
	}
}
