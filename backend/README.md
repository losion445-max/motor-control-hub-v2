# motor-control-hub-v2 — Backend

Go backend for a **4-cable parallel robot (CDPR)** that positions a camera in a
1400 × 2400 mm workspace. Four servo drives pull cables attached to the camera
carriage from the corners of a rectangular frame. Changing all four cable lengths
simultaneously moves the camera to any point in the plane.

---

## Hardware

| Component | Model / Value |
|---|---|
| Servo drives (× 4) | HLTNC T3D-L20A-RABF |
| Motors (× 4) | 80AST-A1C04025 — 2000 RPM, 2.4 Nm, 10 000 ppr |
| Cable drums | Single-layer grooved, r = 67.8 mm |
| Frame width (X) | 1400 mm |
| Frame height (Y) | 2400 mm |
| Bus interface | USB–RS-485 adapter, 19200 baud, 8E1 |
| Slave IDs | M1 = 1, M2 = 2, M3 = 3, M4 = 4 |

---

## Coordinate system

```
M1 (0, 0) ──────────────────── M2 (W, 0)
   │                                  │
   │          X →                     │
   │       Y                          │
   │       ↓         [camera]         │
   │                                  │
M4 (0, H) ──────────────────── M3 (W, H)
```

- Origin at **M1** (top-left corner).
- **X** increases to the right; **Y** increases downward.
- Cable lengths: L1 = √(x²+y²), L2 = √((W-x)²+y²), L3 = √((W-x)²+(H-y)²), L4 = √(x²+(H-y)²).
- Home position: geometric centre (W/2, H/2) — all cables equal length ≈ 1389 mm.

---

## Package architecture

```
cmd/tui          cmd/test
     │                │
     └──── pkg/runner ┘        ← wires gcode + robot; no hardware knowledge
                │
      ┌─────────┴──────────┐
  pkg/gcode            pkg/robot   ← high-level CDPR API + IK
  (pure parser)             │
                    ┌───────┴──────┐
               pkg/motion      pkg/t3d   ← Modbus RTU driver
               (pure math)
```

**Dependency rules** (lower packages never import upper ones):

| Package | Imports |
|---|---|
| `pkg/t3d` | standard library + `github.com/goburrow/modbus` |
| `pkg/motion` | `math` only |
| `pkg/robot` | `pkg/t3d`, `pkg/motion` |
| `pkg/gcode` | standard library only |
| `pkg/runner` | `pkg/gcode` only (accepts `runner.System` interface) |

`pkg/motion` and `pkg/gcode` have **zero hardware dependencies** and can be
imported or tested on any platform without a drive connected.

---

## Quick start

```go
package main

import (
    "context"
    "log"

    "github.com/losion445-max/motor-control-hub-v2/pkg/robot"
    "github.com/losion445-max/motor-control-hub-v2/pkg/gcode"
    "github.com/losion445-max/motor-control-hub-v2/pkg/runner"
)

func main() {
    sys := robot.NewSystem("/dev/ttyUSB0", 19200, robot.DefaultConfig)
    if err := sys.Connect(); err != nil {
        log.Fatal(err)
    }
    defer sys.Close()

    ctx := context.Background()

    // Tension cables and declare home position.
    if err := sys.Home(ctx); err != nil {
        log.Fatal(err)
    }

    // Run a G-code program.
    prog := `
G0 X700 Y1200   ; rapid to centre
G1 F600         ; set feed 600 mm/min = 10 mm/s
X350 Y600       ; straight-line move (interpolated)
X1050 Y1800
G0 X700 Y1200
`
    cmds, err := gcode.Parse(prog)
    if err != nil {
        log.Fatal(err)
    }
    if err := runner.Run(ctx, sys, cmds, runner.DefaultOpts); err != nil {
        log.Fatal(err)
    }
}
```

---

## Building

```bash
cd backend
go build ./...        # build everything
go test ./...         # run all tests (hardware not required)
go run ./cmd/tui      # single-motor TUI (requires drive connected)
go run ./cmd/test     # speed-preset CLI test tool
```

---

## First-time drive setup

Each drive must be pre-configured over Modbus before the software can control it.
Use `SetupSpeedMode` or set the parameters manually via the drive keypad:

| Parameter | Value | Meaning |
|---|---|---|
| P-181 | 1–4 | Slave ID (one per drive) |
| P-182 | 2 | Baud rate = 19200 |
| P-183 | 1 | Parity = 8E1 |
| P-004 | 1 | Control mode = speed |
| P-025 | 1 | Speed source = internal preset |
| P-098 | 1 | Servo-on = always ON (software-controlled) |

Call `SaveEEPROM()` after writing parameters to persist them across power cycles.
