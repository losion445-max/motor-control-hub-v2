# pkg/motion — Trapezoidal Velocity Profile

Pure-math package for computing **trapezoidal velocity profiles** (accel → cruise →
decel) for linear motion axes. It has **no hardware dependencies** and no knowledge
of cable robots, servo drives, or Modbus — it operates in whatever consistent units
the caller provides (typically mm and seconds).

The package also provides a conversion helper that translates mm/s² acceleration
into the T3D drive's P-060/P-061 parameter format so that the hardware ramp and the
software plan agree.

---

## Import

```go
import "github.com/losion445-max/motor-control-hub-v2/pkg/motion"
```

---

## Quick start

```go
package main

import (
    "fmt"
    "github.com/losion445-max/motor-control-hub-v2/pkg/motion"
)

func main() {
    // Plan a 1000 mm move, max speed 300 mm/s, acceleration 1000 mm/s².
    p := motion.New(1000, 300, 1000)

    fmt.Printf("Duration:      %.3f s\n", p.Total)
    fmt.Printf("Accel phase:   %.3f s  (%.1f mm)\n", p.TAccel, p.DecelDistMM)
    fmt.Printf("Cruise phase:  %.3f s\n", p.TCruise)
    fmt.Printf("Decel phase:   %.3f s  (%.1f mm)\n", p.TDecel, p.DecelDistMM)
    fmt.Printf("Speed at 0.5s: %.1f mm/s\n", p.VelocityAt(0.5))

    // Convert to T3D P-060/P-061 value for r = 67.8 mm drum.
    hw := motion.AccelToT3DParam(1000, 67.8)
    fmt.Printf("P-060/P-061:   %d ms/1000rpm\n", hw) // → 7100
}
```

---

## API reference

### `TrapProfile`

```go
type TrapProfile struct {
    Dist        float64 // total distance
    VMax        float64 // cruise velocity (reduced for triangle profiles)
    Accel       float64 // accel/decel magnitude
    TAccel      float64 // duration of acceleration phase (s)
    TCruise     float64 // duration of cruise phase (s); 0 for triangle
    TDecel      float64 // duration of deceleration phase (s) = TAccel
    Total       float64 // total duration (s)
    DecelDistMM float64 // braking distance = VMax²/(2·Accel)
}
```

All fields are exported and read-only after creation via `New()`. Units match
whatever you pass to `New()`.

**`DecelDistMM`** is the key field for motor control integration: it is the distance
the motor will travel while decelerating from VMax to zero. Set P-061 via
`AccelToT3DParam`, then cut motor power when `remaining ≤ DecelDistMM` — the
hardware ramp lands the motor exactly at the target.

### `New(dist, vMax, accel float64) TrapProfile`

Computes the profile. Two cases:

- **Trapezoidal** (`dist > vMax²/accel`): three distinct phases; `TCruise > 0`.
- **Triangle** (`dist ≤ vMax²/accel`): distance is too short to reach `vMax`; peak
  velocity is reduced to `√(accel × dist)` and `TCruise = 0`.

Returns a zero profile without panicking if any input is ≤ 0.

### `VelocityAt(t float64) float64`

Instantaneous velocity at time `t`. Returns 0 for `t ≤ 0` and `t ≥ Total`.
Useful for logging or feed-forward control.

### `PositionAt(t float64) float64`

Distance traveled at time `t`. Returns 0 at `t ≤ 0` and `Dist` at `t ≥ Total`.
Clamped — never returns a value outside `[0, Dist]`.

### `AccelToT3DParam(accelMmPerS2, drumRadiusMM float64) int`

Converts a linear cable acceleration (mm/s²) to the value for the T3D drive's
P-060 and P-061 parameters (milliseconds per 1000 RPM).

```
circumference = 2π × drumRadiusMM
accelRPM/s   = accelMmPerS2 × 60 / circumference
P-060        = round(1 000 000 / accelRPM/s)
```

When `motor.SetAccelTime(AccelToT3DParam(...))` and `motor.SetDecelTime(...)` are
called before a move, the drive's hardware ramp period equals `TrapProfile.TAccel`
(and `TDecel`), so the actual motor behaviour matches the planned profile.

Returns 100 (T3D default) for zero or negative inputs.

---

## Configuration

`TrapProfile` has no mutable state — `New()` computes everything upfront. The only
"configuration" is the three inputs:

| Parameter | Unit | Effect when increased |
|---|---|---|
| `dist` | mm (or any length) | Longer cruise phase (or higher peak for triangle) |
| `vMax` | mm/s | Faster cruise; does not affect TAccel |
| `accel` | mm/s² | Shorter accel/decel phases; larger `DecelDistMM` decreases |

For `AccelToT3DParam`: increasing `accel` decreases the P-060 value (faster ramp).
The T3D clamps P-060 to [1, 30000]; `Motor.SetAccelTime` enforces this range.

---

## Implementation notes

- **Symmetric profile.** `TAccel` always equals `TDecel` (same accel and decel
  magnitude). Asymmetric profiles are not supported — add a wrapper if needed.
- **Units are caller-defined.** The math is dimensionless; just keep dist, vMax, and
  accel in consistent units. The only exception is `AccelToT3DParam`, which requires
  mm and mm/s² specifically (T3D parameters are RPM-based).
- **AccelToT3DParam gives the same P-060 and P-061 value.** For the hardware ramp to
  match the profile, use the same value for both acceleration and deceleration
  parameters (symmetric braking).
- **Multi-axis synchronization caveat.** When multiple motors have different speeds
  (proportional to their cable displacements) but the same P-060 value, the fastest
  motor's hardware ramp matches the profile. Slower motors reach cruise speed sooner,
  causing a small straight-line deviation during the acceleration phase. For the
  1400 × 2400 mm workspace this deviation is typically < 10 mm and is corrected by
  the approach phase in `pkg/robot`.
