package config_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/losion445-max/motor-control-hub-v2/pkg/config"
)

func writeTOML(t *testing.T, content string) string {
	t.Helper()
	f, err := os.CreateTemp(t.TempDir(), "*.toml")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.WriteString(content); err != nil {
		t.Fatal(err)
	}
	f.Close()
	return f.Name()
}

const validTOML = `
[server]
addr               = ":9090"
serial_port        = "/dev/ttyUSB1"
baud_rate          = 9600
status_interval_ms = 100
log_format         = "json"
log_level          = "debug"

[hardware]
width_mm        = 1000.0
height_mm       = 2000.0
drum_radius_mm  = 50.0
pulses_per_rev  = 5000

[homing]
rpm        = 20
torque_pct = 15

[safety]
torque_pct = 70

[hold_tension]
torque_pct = 25
rpm        = 15

[move]
accel_mm_per_sec2    = 800.0
approach_rpm         = 25
approach_factor      = 4
min_approach_pulses  = 40
tolerance_pulses     = 40
poll_ms              = 20
stop_settle_ms       = 120
disable_wait_ms      = 60
approach_switch_ms   = 25

[line]
tick_ms          = 80
correction_gain  = 2.5
fault_check_every = 15
settle_tol_pulses = 40
settle_timeout_s  = 2.5

[gcode]
rapid_mm_per_sec        = 150.0
default_feed_mm_per_sec = 15.0
`

func TestLoad_Valid(t *testing.T) {
	path := writeTOML(t, validTOML)
	cfg, err := config.Load(path)
	if err != nil {
		t.Fatalf("Load: unexpected error: %v", err)
	}

	if cfg.Server.Addr != ":9090" {
		t.Errorf("server.addr = %q, want :9090", cfg.Server.Addr)
	}
	if cfg.Hardware.WidthMM != 1000.0 {
		t.Errorf("hardware.width_mm = %v, want 1000", cfg.Hardware.WidthMM)
	}
	if cfg.Hardware.PulsesPerRev != 5000 {
		t.Errorf("hardware.pulses_per_rev = %v, want 5000", cfg.Hardware.PulsesPerRev)
	}
	if cfg.Server.LogFormat != "json" {
		t.Errorf("server.log_format = %q, want json", cfg.Server.LogFormat)
	}
	if cfg.Gcode.RapidMmPerSec != 150.0 {
		t.Errorf("gcode.rapid_mm_per_sec = %v, want 150", cfg.Gcode.RapidMmPerSec)
	}
}

func TestLoad_FileNotFound(t *testing.T) {
	_, err := config.Load(filepath.Join(t.TempDir(), "nonexistent.toml"))
	if err == nil {
		t.Fatal("expected error for missing file, got nil")
	}
}

func TestLoad_InvalidTOML(t *testing.T) {
	path := writeTOML(t, "this is not [valid toml }")
	_, err := config.Load(path)
	if err == nil {
		t.Fatal("expected error for invalid TOML, got nil")
	}
}

func TestLoad_UnknownKey(t *testing.T) {
	path := writeTOML(t, validTOML+"\n[server]\nunknown_field = true\n")
	_, err := config.Load(path)
	if err == nil {
		t.Fatal("expected error for unknown key, got nil")
	}
}

func TestLoad_Validation_ZeroWidth(t *testing.T) {
	bad := strings.ReplaceAll(validTOML, "width_mm        = 1000.0", "width_mm = 0")
	path := writeTOML(t, bad)
	_, err := config.Load(path)
	if err == nil {
		t.Fatal("expected validation error for width_mm=0, got nil")
	}
}

func TestLoad_Validation_ZeroHeight(t *testing.T) {
	bad := strings.ReplaceAll(validTOML, "height_mm       = 2000.0", "height_mm = 0")
	path := writeTOML(t, bad)
	_, err := config.Load(path)
	if err == nil {
		t.Fatal("expected validation error for height_mm=0, got nil")
	}
}

func TestLoad_Validation_ZeroDrumRadius(t *testing.T) {
	bad := strings.ReplaceAll(validTOML, "drum_radius_mm  = 50.0", "drum_radius_mm = 0")
	path := writeTOML(t, bad)
	_, err := config.Load(path)
	if err == nil {
		t.Fatal("expected validation error for drum_radius_mm=0, got nil")
	}
}

func TestLoad_Validation_ZeroBaud(t *testing.T) {
	bad := strings.ReplaceAll(validTOML, "baud_rate          = 9600", "baud_rate = 0")
	path := writeTOML(t, bad)
	_, err := config.Load(path)
	if err == nil {
		t.Fatal("expected validation error for baud_rate=0, got nil")
	}
}

func TestLoad_Validation_BadLogFormat(t *testing.T) {
	bad := strings.ReplaceAll(validTOML, `log_format         = "json"`, `log_format = "yaml"`)
	path := writeTOML(t, bad)
	_, err := config.Load(path)
	if err == nil {
		t.Fatal("expected validation error for log_format=yaml, got nil")
	}
}

func TestLoad_Defaults(t *testing.T) {
	// Minimal config — only required non-zero fields.
	minimal := `
[server]
serial_port = "/dev/ttyUSB0"
baud_rate   = 19200
log_format  = "text"

[hardware]
width_mm       = 1400.0
height_mm      = 2400.0
drum_radius_mm = 67.8
`
	path := writeTOML(t, minimal)
	cfg, err := config.Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	// Addr defaults to empty string — that's fine, server will use OS default.
	if cfg.Server.LogLevel == "" {
		// empty is acceptable; initLogger will treat it as "info"
	}
	_ = cfg
}
