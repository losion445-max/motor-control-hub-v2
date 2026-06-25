// Package robot implements a high-level API for a 4-cable parallel robot (CDPR).
//
// Motor layout (clockwise from top-left, slave IDs 1-4):
//
//	M1(0,0) ─────────── M2(W,0)
//	  │                      │
//	  │       [camera]       │
//	  │                      │
//	M4(0,H) ─────────── M3(W,H)
//
// Usage:
//
//	sys := robot.NewSystem("/dev/ttyUSB0", 19200, robot.DefaultConfig)
//	sys.Connect()
//	defer sys.Close()
//	sys.Home(ctx)
//	sys.MoveTo(ctx, 700, 1200, 50) // move to centre at 50 mm/s
package robot

import "time"

// Config holds all physical and operational parameters of the cable robot.
// All values are populated from config.toml at startup; DefaultConfig
// contains the same values as the shipped config file.
type Config struct {
	// ── Physical frame ────────────────────────────────────────────────────────

	WidthMM      float64 // W: horizontal distance M1→M2 (mm)
	HeightMM     float64 // H: vertical distance M1→M4 (mm)
	DrumRadiusMM float64 // effective cable drum radius at mid-cable (mm)
	PulsesPerRev  int      // encoder PPR (10000 for 80AST-A1C04025: 2500-line × 4)
	MotorReversed [4]bool  // invert winding direction per motor (index 0=M1…3=M4)

	// ── Homing ────────────────────────────────────────────────────────────────

	HomingRPM       int // winding speed during homing (positive = wind in)
	HomingTorquePct int // torque % threshold that signals cable is taut

	// ── Safety ────────────────────────────────────────────────────────────────

	TorqueSafetyPct int // emergency stop if |torque| ≥ this during any move
	MoveTorquePct   int // hardware torque limit during normal motion (P-069/P-070); 0 = 300% (full)

	// ── Passive tension hold ──────────────────────────────────────────────────

	HoldTensionPct int // torque cap for tension-hold mode (P-069/P-070, % of rated)
	HoldTensionRPM int // slow winding speed for tension-hold mode

	// ── MoveTo controller (stop-and-go trapezoidal) ───────────────────────────

	AccelMmPerSec2 float64 // trapezoidal profile acceleration (mm/s²); 0 = use drive default

	ApproachRPM       int           // final approach speed for all motors (RPM)
	ApproachFactor    int           // approach zone = max(Factor × maxRPM, MinApproachPulses)
	MinApproachPulses int64         // floor for approach zone (pulses); prevents collapse on short moves
	TolerancePulses   int64         // stop when remaining ≤ this (pulses)
	PollInterval      time.Duration // how often motor positions are read
	StopSettle        time.Duration // settle time after all motors have stopped
	DisableWait       time.Duration // wait after disable before reading start positions
	ApproachSwitch    time.Duration // wait between collective disable and re-enable at approach speed

	// ── LineTo controller (continuous closed-loop) ────────────────────────────

	LineTickDT    time.Duration // control loop period
	LineCorrGain  float64       // proportional cable-error gain: (mm/s) per mm error
	LineSettleTol int32         // settle phase done when all errors < this (pulses)
	LineSettleLim time.Duration // max wait in settle phase
}

// DefaultConfig matches the values in backend/config.toml exactly.
// Used directly in tests (no file I/O required).
var DefaultConfig = Config{
	WidthMM:      1400,
	HeightMM:     2400,
	DrumRadiusMM: 67.8,
	PulsesPerRev: 10000,

	HomingRPM:       25,
	HomingTorquePct: 5,

	TorqueSafetyPct: 70,
	MoveTorquePct:   300,

	HoldTensionPct: 1,
	HoldTensionRPM: 20,

	AccelMmPerSec2: 1000,

	ApproachRPM:       30,
	ApproachFactor:    5,
	MinApproachPulses: 50,
	TolerancePulses:   50,
	PollInterval:      10 * time.Millisecond,
	StopSettle:        150 * time.Millisecond,
	DisableWait:       80 * time.Millisecond,
	ApproachSwitch:    30 * time.Millisecond,

	LineTickDT:    25 * time.Millisecond,
	LineCorrGain:  2.0,
	LineSettleTol: 50,
	LineSettleLim: 3 * time.Second,
}
