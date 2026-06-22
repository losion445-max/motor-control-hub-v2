// Package config defines the application configuration schema and loader.
//
// The canonical config file is backend/config.toml, committed to the
// repository. It is the single source of truth for all hardware parameters,
// motion tuning values, and server settings.
//
// Usage:
//
//	cfg, err := config.Load("config.toml")
//	if err != nil {
//	    log.Fatal(err)
//	}
package config

import (
	"fmt"
	"os"

	"github.com/BurntSushi/toml"
)

// Config is the root configuration object.
type Config struct {
	Server   ServerConfig   `toml:"server"`
	Hardware HardwareConfig `toml:"hardware"`
	Homing   HomingConfig   `toml:"homing"`
	Safety   SafetyConfig   `toml:"safety"`
	Hold     HoldConfig     `toml:"hold_tension"`
	Move     MoveConfig     `toml:"move"`
	Line     LineConfig     `toml:"line"`
	Gcode    GcodeConfig    `toml:"gcode"`
}

// ServerConfig controls the HTTP server and serial transport.
type ServerConfig struct {
	// Addr is the TCP address the HTTP server binds to (e.g. ":8080").
	Addr string `toml:"addr"`

	// SerialPort is the RS-485 device path (e.g. "/dev/ttyUSB0").
	SerialPort string `toml:"serial_port"`

	// BaudRate for Modbus RTU (must match drive P-179 setting).
	BaudRate int `toml:"baud_rate"`

	// StatusIntervalMs is how often (ms) all motors are polled and status
	// events are broadcast to WebSocket subscribers.
	StatusIntervalMs int `toml:"status_interval_ms"`

	// LogFormat is "text" (human-readable) or "json" (machine-readable).
	LogFormat string `toml:"log_format"`

	// LogLevel controls verbosity: "debug", "info", "warn", "error".
	LogLevel string `toml:"log_level"`
}

// HardwareConfig describes the physical cable robot frame.
type HardwareConfig struct {
	// WidthMM is the horizontal distance between motor-1 and motor-2 (mm).
	WidthMM float64 `toml:"width_mm"`

	// HeightMM is the vertical distance between motor-1 and motor-4 (mm).
	HeightMM float64 `toml:"height_mm"`

	// DrumRadiusMM is the effective cable drum radius at the midline of the
	// cable (mm). Determines RPM↔mm/s conversion.
	DrumRadiusMM float64 `toml:"drum_radius_mm"`

	// PulsesPerRev is the encoder resolution in pulses per revolution.
	// 80AST-A1C04025: 2500-line encoder × 4 = 10000 PPR.
	// Together with DrumRadiusMM this determines the mm↔pulse conversion.
	PulsesPerRev int `toml:"pulses_per_rev"`

	// MotorReversed inverts the winding direction for each motor individually.
	// Set to true for a motor whose drum is mounted so that positive RPM
	// pays out instead of winding in (i.e. the cable wraps the opposite way).
	// Index 0 = M1, 1 = M2, 2 = M3, 3 = M4.
	MotorReversed [4]bool `toml:"motor_reversed"`
}

// HomingConfig contains parameters for the homing (calibration) sequence.
type HomingConfig struct {
	// RPM is the winding speed during homing (positive = wind in).
	RPM int `toml:"rpm"`

	// TorquePct is the torque % threshold that signals cable is taut.
	TorquePct int `toml:"torque_pct"`
}

// SafetyConfig sets hard limits enforced during all moves.
type SafetyConfig struct {
	// TorquePct is the emergency-stop threshold (% of rated torque).
	// If any motor exceeds this during a move, all motors are stopped.
	TorquePct int `toml:"torque_pct"`
}

// HoldConfig defines passive tension mode parameters.
type HoldConfig struct {
	// TorquePct is the torque cap for tension-hold mode (P-069, % of rated).
	TorquePct int `toml:"torque_pct"`

	// RPM is the slow winding speed used to maintain tension.
	RPM int `toml:"rpm"`
}

// MoveConfig tunes the MoveTo stop-and-go trapezoidal controller.
type MoveConfig struct {
	// AccelMmPerSec2 is the acceleration for the trapezoidal profile.
	// Set to 0 to leave drive ramps at their current hardware values.
	AccelMmPerSec2 float64 `toml:"accel_mm_per_sec2"`

	// ApproachRPM is the target speed during the final approach phase (RPM).
	ApproachRPM int `toml:"approach_rpm"`

	// ApproachFactor: approach zone = max(ApproachFactor × maxSpeedRPM, MinApproachPulses).
	ApproachFactor int `toml:"approach_factor"`

	// MinApproachPulses is the floor for the approach zone (pulses).
	// Prevents the zone from collapsing to zero for slow or short moves.
	MinApproachPulses int `toml:"min_approach_pulses"`

	// TolerancePulses: a motor stops when remaining ≤ this (pulses, ~2 mm).
	TolerancePulses int `toml:"tolerance_pulses"`

	// PollMs is how often the motor positions are read during a move (ms).
	PollMs int `toml:"poll_ms"`

	// StopSettleMs is the settle time after all motors have stopped (ms).
	StopSettleMs int `toml:"stop_settle_ms"`

	// DisableWaitMs is the wait after disabling motors before reading start
	// positions (ms). Gives the drive time to coast to a full stop.
	DisableWaitMs int `toml:"disable_wait_ms"`

	// ApproachSwitchMs is the wait after disabling motors during the
	// collective approach phase before re-enabling at reduced speed (ms).
	ApproachSwitchMs int `toml:"approach_switch_ms"`
}

// LineConfig tunes the LineTo continuous closed-loop velocity controller.
type LineConfig struct {
	// TickMs is the control loop period (ms).
	TickMs int `toml:"tick_ms"`

	// CorrectionGain is the proportional cable-error gain: (mm/s) per mm
	// of cable-length error added to the feed-forward speed.
	CorrectionGain float64 `toml:"correction_gain"`

	// FaultCheckEvery: fault registers are read every N control ticks.
	FaultCheckEvery int `toml:"fault_check_every"`

	// SettleTolPulses: final settle phase completes when all cable errors
	// are within this many pulses (~2 mm).
	SettleTolPulses int `toml:"settle_tol_pulses"`

	// SettleTimeoutS is the maximum time (seconds) to wait in the settle phase.
	SettleTimeoutS float64 `toml:"settle_timeout_s"`
}

// GcodeConfig sets default feed rates used by the G-code runner.
type GcodeConfig struct {
	// RapidMmPerSec is the speed for G0 (rapid) moves.
	RapidMmPerSec float64 `toml:"rapid_mm_per_sec"`

	// DefaultFeedMmPerSec is the feed rate before any F word appears in the
	// program.
	DefaultFeedMmPerSec float64 `toml:"default_feed_mm_per_sec"`
}

// Load reads and validates a TOML config file at path.
// Returns an error if the file cannot be read or parsed.
func Load(path string) (*Config, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("config: open %q: %w", path, err)
	}
	defer f.Close()

	var cfg Config
	meta, err := toml.NewDecoder(f).Decode(&cfg)
	if err != nil {
		return nil, fmt.Errorf("config: parse %q: %w", path, err)
	}

	// Warn about keys present in the file that don't map to any struct field.
	if keys := meta.Undecoded(); len(keys) > 0 {
		return nil, fmt.Errorf("config: unknown keys in %q: %v", path, keys)
	}

	if err := cfg.validate(); err != nil {
		return nil, fmt.Errorf("config: %w", err)
	}
	return &cfg, nil
}

func (c *Config) validate() error {
	if c.Hardware.WidthMM <= 0 || c.Hardware.HeightMM <= 0 {
		return fmt.Errorf("hardware.width_mm and hardware.height_mm must be > 0")
	}
	if c.Hardware.DrumRadiusMM <= 0 {
		return fmt.Errorf("hardware.drum_radius_mm must be > 0")
	}
	if c.Server.BaudRate <= 0 {
		return fmt.Errorf("server.baud_rate must be > 0")
	}
	if c.Server.LogFormat != "text" && c.Server.LogFormat != "json" {
		return fmt.Errorf("server.log_format must be \"text\" or \"json\", got %q", c.Server.LogFormat)
	}
	return nil
}
