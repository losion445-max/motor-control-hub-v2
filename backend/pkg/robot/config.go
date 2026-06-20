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

// Config holds the physical and operational parameters of the cable robot.
type Config struct {
	WidthMM  float64 // W: horizontal distance M1→M2 (mm)
	HeightMM float64 // H: vertical distance M1→M4 (mm)

	DrumRadiusMM float64 // effective cable drum radius at the midline of the cable (mm)

	// Homing
	HomingRPM       int // winding speed during homing (positive = wind in)
	HomingTorquePct int // torque % threshold that signals cable is taut

	// Motion safety
	TorqueSafetyPct int // emergency stop if |torque| ≥ this during any move (% of rated)

	// Passive tension hold (after a move or on demand)
	HoldTensionPct int // torque cap for the tension-hold mode (P-069, % of rated)
	HoldTensionRPM int // slow winding speed for tension-hold mode
}

// DefaultConfig is ready-to-use for the 1400×2400 mm frame with 67.8 mm drums.
var DefaultConfig = Config{
	WidthMM:      1400,
	HeightMM:     2400,
	DrumRadiusMM: 67.8,

	HomingRPM:       25,
	HomingTorquePct: 18,

	TorqueSafetyPct: 75,

	HoldTensionPct: 30,
	HoldTensionRPM: 20,
}
