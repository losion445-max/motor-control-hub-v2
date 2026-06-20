# pkg/runner — G-code Executor

Executes a parsed `[]gcode.Cmd` program on a cable robot. This package is the
**junction point** between `pkg/gcode` and `pkg/robot`: it imports `pkg/gcode`
for the command type, but accepts any value that satisfies the `runner.System`
interface rather than `*robot.System` directly. Neither `pkg/gcode` nor `pkg/robot`
imports the other.

---

## Import

```go
import "github.com/losion445-max/motor-control-hub-v2/pkg/runner"
```

---

## Quick start

```go
package main

import (
    "context"
    "log"

    "github.com/losion445-max/motor-control-hub-v2/pkg/gcode"
    "github.com/losion445-max/motor-control-hub-v2/pkg/robot"
    "github.com/losion445-max/motor-control-hub-v2/pkg/runner"
)

func main() {
    sys := robot.NewSystem("/dev/ttyUSB0", 19200, robot.DefaultConfig)
    if err := sys.Connect(); err != nil {
        log.Fatal(err)
    }
    defer sys.Close()

    ctx := context.Background()
    if err := sys.Home(ctx); err != nil {
        log.Fatal(err)
    }

    prog := `
G0 X700 Y1200     ; rapid to centre
G1 F600           ; feed = 600 mm/min = 10 mm/s
X350 Y600
X1050 Y1800
X700 Y1200
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

## API reference

### `System` interface

```go
type System interface {
    MoveTo(ctx context.Context, x, y, speedMmPerSec float64) error
    LineTo(ctx context.Context, x, y, speedMmPerSec float64) error
    Home(ctx context.Context) error
    EmergencyStop() error
    Position() (float64, float64)
}
```

`*robot.System` satisfies this interface. You can provide your own implementation
for testing or for a different robot backend.

### `Opts`

```go
type Opts struct {
    RapidMmPerSec       float64 // speed for G0 moves (default 200 mm/s)
    DefaultFeedMmPerSec float64 // feed rate before first F word (default 20 mm/s)
}
```

`DefaultOpts` provides sensible defaults for the 1400 × 2400 mm frame.

### `Run(ctx context.Context, sys System, cmds []gcode.Cmd, opts Opts) error`

Executes the command list in order. Behaviour per command type:

| G-code | Action |
|---|---|
| `G0` + X/Y | `sys.MoveTo(ctx, x, y, opts.RapidMmPerSec)` |
| `G1` + X/Y | `sys.LineTo(ctx, x, y, feedMmPerSec)` |
| `G28` | `sys.Home(ctx)` |
| `G1 F600` (F-only) | Updates modal feed rate; no physical movement |

**Feed rate is modal.** Once set with `F`, it persists for all subsequent `G1`
moves until changed. The unit in the G-code file is **mm/min**; `Run` converts to
mm/s internally.

**Position is modal.** If only `X` or only `Y` is specified on a line, the other
axis stays at the current position. This matches standard G-code behaviour.

Cancelling `ctx` interrupts the current motor command and propagates the context
error upward. Each motor command (`MoveTo`/`LineTo`) calls `EmergencyStop`
internally on cancel.

---

## Configuration

| Field | Default | Effect |
|---|---|---|
| `RapidMmPerSec` | 200 | G0 move speed. Reduce if rapid moves feel unsafe |
| `DefaultFeedMmPerSec` | 20 | Feed before first `F` word. 20 mm/s is intentionally slow |

Fields ≤ 0 in a passed `Opts` are replaced with the `DefaultOpts` values.

---

## Implementation notes

**Interface over concrete type.** `Run` accepts `runner.System` instead of
`*robot.System` so that `pkg/runner` does not import `pkg/robot`. This preserves
layer independence and makes unit testing straightforward — provide a mock struct
that records calls.

**F-only commands.** The G-code parser emits a `Cmd` even for lines like `G1 F600`
that carry no position. `Run` updates `feedMmPerSec` from it and then calls
`sys.Position()` → `sys.LineTo(x, x, feedMmPerSec)` with the current position,
which is a no-op move (distance < 0.5 mm).

**G28 resets internal state.** After `sys.Home()` returns, `sys.Position()` returns
`(W/2, H/2)`. Subsequent moves are relative to that new home.

**No look-ahead.** Each command is executed to completion before the next begins.
For smooth high-speed paths between many waypoints, a streaming or buffered approach
would be needed — this is a future enhancement.
