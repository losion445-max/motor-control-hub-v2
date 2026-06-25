package robot

import (
	"context"
	"errors"
	"math"
	"testing"
	"time"
)

func lineTestSys() (*System, [4]*mockMotor) {
	cfg := defaultCfg()
	cfg.LineTickDT = 1 * time.Millisecond // fast tick
	cfg.LineSettleTol = 500               // generous settle tolerance
	cfg.LineSettleLim = 5 * time.Second   // plenty of settle time
	cfg.LineCorrGain = 0                  // no correction term, pure feed-forward

	s, mocks := newTestSystem(cfg)
	s.homed = true
	s.posX = cfg.WidthMM / 2
	s.posY = cfg.HeightMM / 2
	s.homeLenMM = homeLength(cfg.WidthMM, cfg.HeightMM)
	for _, m := range mocks {
		m.absPos = 0
	}
	return s, mocks
}

// ── LineTo ────────────────────────────────────────────────────────────────────

func TestLineTo_NotHomed(t *testing.T) {
	s, _ := newTestSystem(defaultCfg())
	if err := s.LineTo(context.Background(), 700, 1200, 50); err == nil {
		t.Fatal("expected error when not homed")
	}
}

func TestLineTo_NearTarget(t *testing.T) {
	// dist < 0.5 mm → no-op path.
	s, _ := lineTestSys()
	if err := s.LineTo(context.Background(), s.posX+0.1, s.posY, 50); err != nil {
		t.Fatalf("LineTo near target: %v", err)
	}
	if s.posX != s.cfg.WidthMM/2+0.1 {
		t.Errorf("posX not updated on near-target, got %v", s.posX)
	}
}

func TestLineTo_SetAccelError(t *testing.T) {
	s, mocks := lineTestSys()
	mocks[0].accelErr = errors.New("accel fail")
	if err := s.LineTo(context.Background(), s.posX+100, s.posY, 50); err == nil {
		t.Fatal("expected error from SetAccelTime")
	}
}

func TestLineTo_EnableError(t *testing.T) {
	s, mocks := lineTestSys()
	mocks[0].enableErr = errors.New("enable fail")
	if err := s.LineTo(context.Background(), s.posX+100, s.posY, 50); err == nil {
		t.Fatal("expected error from Enable")
	}
}

func TestLineTo_ContextCancelled(t *testing.T) {
	s, _ := lineTestSys()
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // already cancelled
	if err := s.LineTo(ctx, s.posX+100, s.posY, 50); err == nil {
		t.Fatal("expected cancellation error")
	}
}

func TestLineTo_Completes(t *testing.T) {
	// High speed so the motion profile completes quickly.
	// ReadAbsPosition returns 0 (actual ≈ homeLen for each cable).
	// finalLens ≈ homeLen + tiny delta; |delta|*ppm < LineSettleTol(500) → converges.
	s, _ := lineTestSys()

	target := s.posX + 1.0 // 1 mm move
	if err := s.LineTo(context.Background(), target, s.posY, 200); err != nil {
		t.Fatalf("LineTo: %v", err)
	}
	if s.posX != target {
		t.Errorf("posX = %v, want %v", s.posX, target)
	}
}

func TestLineTo_SettleTimeout(t *testing.T) {
	// Very tight settle tolerance (0 pulses) so convergence never happens.
	// The settle timeout should fire instead.
	cfg := defaultCfg()
	cfg.LineTickDT = 1 * time.Millisecond
	cfg.LineSettleTol = 0                    // never converge
	cfg.LineSettleLim = 2 * time.Millisecond // very short timeout → break fast
	cfg.LineCorrGain = 0

	s, mocks := newTestSystem(cfg)
	s.homed = true
	s.posX = cfg.WidthMM / 2
	s.posY = cfg.HeightMM / 2
	s.homeLenMM = homeLength(cfg.WidthMM, cfg.HeightMM)
	for _, m := range mocks {
		m.absPos = 0
	}

	// Move 1 mm; profile finishes quickly, then settle loop runs until
	// LineSettleLim (2ms) expires and LineTo returns a timeout error.
	target := cfg.WidthMM/2 + 1.0
	err := s.LineTo(context.Background(), target, cfg.HeightMM/2, 200)
	if err == nil {
		t.Fatal("expected settle timeout error, got nil")
	}
}

func TestLineTo_WriteSpeedError(t *testing.T) {
	s, mocks := lineTestSys()
	// WriteParam fails during the speed-command phase of the control loop.
	// First several WriteParams succeed (SetAccelTime/SetDecelTime/Enable setup),
	// then the speed-write in the loop fails.
	callCount := 0
	mocks[0].writeErr = nil
	// Inject a counting mock that fails after 2 writes.
	orig := mocks[0]
	cm := &countingMock{mockMotor: orig, failAfter: 2, failErr: errors.New("speed write fail")}
	s.motors[0] = cm

	err := s.LineTo(context.Background(), s.posX+100, s.posY, 50)
	if err == nil {
		t.Fatal("expected error from WriteParam in control loop")
	}
	_ = callCount
}

// countingMock wraps mockMotor and causes WriteParam to fail after N calls.
type countingMock struct {
	*mockMotor
	calls     int
	failAfter int
	failErr   error
}

func (c *countingMock) WriteParam(addr, value uint16) error {
	c.calls++
	if c.calls > c.failAfter {
		return c.failErr
	}
	return nil
}
func (c *countingMock) ReadParam(_ uint16) (uint16, error) { return 0, nil }

// ── LineTo ReadAbsPosition (currentCableLengths) error ───────────────────────

func TestLineTo_ReadPosError(t *testing.T) {
	s, mocks := lineTestSys()
	// currentCableLengths fails in the control loop.
	// Fail immediately even before the motion profile starts.
	mocks[0].readPosErr = errors.New("pos fail")

	err := s.LineTo(context.Background(), s.posX+100, s.posY, 50)
	if err == nil {
		t.Fatal("expected error from currentCableLengths")
	}
}

// ── Fault check ───────────────────────────────────────────────────────────────

func TestLineTo_FaultDetected(t *testing.T) {
	cfg := defaultCfg()
	cfg.LineTickDT = 1 * time.Millisecond
	cfg.LineSettleTol = 500
	cfg.LineSettleLim = 5 * time.Second
	cfg.LineCorrGain = 0

	s, mocks := newTestSystem(cfg)
	s.homed = true
	s.posX = cfg.WidthMM / 2
	s.posY = cfg.HeightMM / 2
	s.homeLenMM = homeLength(cfg.WidthMM, cfg.HeightMM)
	for _, m := range mocks {
		m.absPos = 0
	}
	mocks[0].fault = 7 // motor 1 reports fault

	err := s.LineTo(context.Background(), s.posX+100, s.posY, 50)
	if err == nil {
		t.Fatal("expected fault error")
	}
}

// ── mmPerSecToRPM ─────────────────────────────────────────────────────────────

func TestMmPerSecToRPM(t *testing.T) {
	// 1 RPM = 2π × 67.8 / 60 ≈ 7.1 mm/s, so 100 mm/s → ≈ 14 RPM.
	rpm := mmPerSecToRPM(100, 67.8)
	if rpm < 10 || rpm > 20 {
		t.Errorf("mmPerSecToRPM(100, 67.8) = %d, expected ~14", rpm)
	}
}

func TestMmPerSecToRPM_ZeroSpeed(t *testing.T) {
	if rpm := mmPerSecToRPM(0, 67.8); rpm != 0 {
		t.Errorf("mmPerSecToRPM(0) = %d, want 0", rpm)
	}
}

func TestMmPerSecToRPM_RoundTripHigh(t *testing.T) {
	// Only check for higher speeds where integer rounding is small relative to value.
	for _, mm := range []float64{50, 100, 200} {
		rpm := mmPerSecToRPM(mm, 67.8)
		back := rpmToMMperSec(float64(rpm), 67.8)
		// Allow ±1 RPM rounding error.
		tol := rpmToMMperSec(1, 67.8)
		if math.Abs(back-mm) > tol {
			t.Errorf("round-trip mm/s=%.1f → rpm=%d → mm/s=%.2f (err %.2f > tol %.2f)",
				mm, rpm, back, math.Abs(back-mm), tol)
		}
	}
}
