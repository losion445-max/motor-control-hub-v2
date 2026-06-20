# pkg/robot — Cable Robot High-Level API

High-level Go API for controlling a **4-cable parallel robot (CDPR)**. It handles
inverse kinematics, multi-motor synchronization, homing, straight-line
interpolation, and passive cable tension.

This package imports `pkg/t3d` (hardware) and `pkg/motion` (profile math). It does
not import `pkg/gcode` or `pkg/runner` — those live above it in the stack.

---

## Motor layout

```
M1 (0, 0) ──────────────────── M2 (W, 0)
   │  slave 1                  slave 2  │
   │                                    │
   │               [camera]             │
   │                                    │
M4 (0, H) ──────────────────── M3 (W, H)
   slave 4                     slave 3
```

Origin at M1 (top-left). X increases right; Y increases downward.

---

## Import

```go
import "github.com/losion445-max/motor-control-hub-v2/pkg/robot"
```

---

## Quick start

```go
package main

import (
    "context"
    "log"
    "github.com/losion445-max/motor-control-hub-v2/pkg/robot"
)

func main() {
    sys := robot.NewSystem("/dev/ttyUSB0", 19200, robot.DefaultConfig)
    if err := sys.Connect(); err != nil {
        log.Fatal(err)
    }
    defer sys.Close()

    ctx := context.Background()

    // Tension all cables; declare current position as home (centre).
    if err := sys.Home(ctx); err != nil {
        log.Fatal(err)
    }

    // Proportional-speed move (curves slightly in Cartesian space at long distances).
    if err := sys.MoveTo(ctx, 350, 600, 100); err != nil { // 100 mm/s
        log.Fatal(err)
    }

    // Straight-line interpolated move (correct for IK non-linearity).
    if err := sys.LineTo(ctx, 1050, 1800, 30); err != nil { // 30 mm/s
        log.Fatal(err)
    }

    // Keep cables taut while stationary (hardware-controlled, no goroutine).
    sys.HoldTension()

    // Emergency stop (call from any goroutine).
    // sys.EmergencyStop()
}
```

---

## API reference

### `Config` and `DefaultConfig`

`Config` holds all physical and operational parameters. `DefaultConfig` is
pre-filled for the 1400 × 2400 mm frame with 67.8 mm drums. Copy and modify it
to adjust individual fields:

```go
cfg := robot.DefaultConfig
cfg.HomingTorquePct = 22  // tighter homing tension
sys := robot.NewSystem("/dev/ttyUSB0", 19200, cfg)
```

See the [Configuration](#configuration) section for all fields.

### `NewSystem(port string, baud int, cfg Config) *System`

Creates a `System` with four motors (slave IDs 1–4) on one RS-485 bus. Does not
open the serial port. Call `Connect()` before any motor operations.

### `Connect() error` / `Close() error`

Open and close the underlying serial port. `Close` should be called in a `defer`
or shutdown handler.

### `Home(ctx context.Context) error`

Winds all four cables simultaneously at `Config.HomingRPM` RPM with a torque cap
of `Config.HomingTorquePct + 5`. Each motor stops independently as soon as its
cable becomes taut (torque reaches `HomingTorquePct`). When all four have stopped,
the camera is declared to be at the geometric centre (W/2, H/2).

**Prerequisite:** place the camera physically near the centre before calling `Home`.
If a cable is already slack the motor will wind indefinitely until ctx is cancelled.

### `MoveTo(ctx context.Context, x, y, speedMmPerSec float64) error`

Moves the camera to `(x, y)` mm from M1 (top-left). Runs all four motors
simultaneously with proportional speeds so the fastest cable finishes at
`speedMmPerSec` and the others finish at the same time. Before starting, sets
P-060/P-061 on all motors from `Config.AccelMmPerSec2` for smooth acceleration.

The camera path in Cartesian space is approximately straight for short moves
(< 100 mm) but curves for long moves due to IK non-linearity. Use `LineTo` when
a geometrically straight path is required.

Returns when all motors have stopped. Cancelling ctx calls `EmergencyStop`.

### `LineTo(ctx context.Context, x, y, speedMmPerSec float64) error`

Same as `MoveTo` but subdivides the Cartesian path into `Config.InterpStepMM`
segments and recomputes inverse kinematics at each waypoint. The camera follows
a straight line. At the default 25 mm step size, the path error is < 0.1 mm
anywhere in the 1400 × 2400 mm workspace.

Use `LineTo` for all G1 (feed) moves. Use `MoveTo` for G0 (rapid) moves where
path accuracy is not important.

### `HoldTension() error`

Switches all four motors to passive tension mode by setting `Config.HoldTensionPct`
as the torque limit and `Config.HoldTensionRPM` as the winding speed. The drive
hardware stalls when the cable is taut (torque limit reached) and automatically
resumes if slack develops. No goroutine is needed — the drive controller handles
it entirely in hardware.

Call after `MoveTo` or `LineTo` to keep cables taut while the camera is stationary.

### `EmergencyStop() error`

Sends FC42 Disable to all four motors immediately. Errors from individual motors
are suppressed so all four are attempted even if one fails. Safe to call from any
goroutine.

### `ReadAllStatus() [4]MotorState`

Reads the full `t3d.Status` from each motor sequentially. Returns a `[4]MotorState`
slice where `MotorState.Err` is non-nil for any motor that could not be read.
Useful for monitoring dashboards.

### `Position() (x, y float64)`

Returns the last known camera position (mm from M1). Updated after every successful
`MoveTo` or `LineTo`. Only valid after a successful `Home` call.

---

## Configuration

| Field | Type | Default | Notes |
|---|---|---|---|
| `WidthMM` | float64 | 1400 | Horizontal distance M1→M2 (mm) |
| `HeightMM` | float64 | 2400 | Vertical distance M1→M4 (mm) |
| `DrumRadiusMM` | float64 | 67.8 | Effective cable drum radius (mm) |
| `HomingRPM` | int | 25 | Cable winding speed during homing |
| `HomingTorquePct` | int | 18 | Torque % that signals cable taut |
| `AccelMmPerSec2` | float64 | 1000 | Accel/decel for trapezoidal profile; 0 = leave P-060/P-061 unchanged |
| `InterpStepMM` | float64 | 25 | Waypoint spacing for `LineTo` (mm) |
| `TorqueSafetyPct` | int | 75 | Emergency stop threshold during moves (% of rated) |
| `HoldTensionPct` | int | 30 | Torque cap for passive tension hold (P-069) |
| `HoldTensionRPM` | int | 20 | Winding speed for tension hold |

**`AccelMmPerSec2 = 0`** leaves the drive's P-060/P-061 unchanged. The approach
zone falls back to the heuristic `5 × maxSpeedRPM` pulses.

**`InterpStepMM`** trades path accuracy for Modbus transaction count. Each step
requires one full `MoveTo` cycle (disable → read → move → settle ≈ 300–500 ms).
At 25 mm steps and 30 mm/s feed, a 1000 mm move completes in ≈ 50 s.

---

## Implementation notes

**Inverse kinematics** (`kinematics.go`) computes cable lengths from Cartesian
position using the four corner distances. The IK is exact — no approximation.

**Sign convention:** positive encoder change = motor wound in = cable shorter.
`mmToPulses(+d)` gives positive pulses; `mmToPulses(-d)` gives negative.

**Proportional speeds** in `movePulses`: the motor with the largest cable delta runs
at `maxSpeedRPM`; others scale proportionally so all reach their targets
simultaneously. This keeps the camera on a straight line *in cable space*, which is
approximately a straight line in Cartesian space for moves shorter than ≈ 100 mm.

**Collective approach trigger**: the slowdown to 30 RPM is triggered when *any* motor
enters its approach zone (not just the fastest). Using "whichever leads" is
conservative — any cable that is nearly done must not overshoot while waiting for
slower cables, as cable slack causes loss of position.

**Approach zone size**: when `AccelMmPerSec2 > 0`, the approach zone equals the
profile's `DecelDistMM` (physics-derived braking distance). Otherwise the heuristic
`max(5 × maxRPM, 500)` pulses is used.

**`currentCableLengths`**: reconstructs cable lengths from encoder readings relative
to the home snapshot. If the camera is moved by hand while powered off, call `Home`
again before the next `MoveTo`.

**`LineTo` pauses between steps**: each waypoint is a full `MoveTo` call including
disable/enable cycles and settle times. Motion is therefore stop-and-go at each
25 mm waypoint, not continuous. This is acceptable at G1 feed rates (10–50 mm/s)
but would be unacceptably slow at rapid speeds — use `MoveTo` for G0.
