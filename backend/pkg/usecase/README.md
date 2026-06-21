# pkg/usecase

Orchestration layer that composes lower-level robot primitives into named
operation scenarios. Sits between `pkg/robot` / `pkg/runner` (below) and
`pkg/api` (above).

## Responsibilities

- **Serialises concurrent access** — only one motion operation runs at a time.
  Any second caller receives an immediate `error` event with the message
  `"robot busy"`.
- **Cancellation** — `Stop()` cancels the active operation context *and* calls
  `EmergencyStop()` on the hardware in the same call.
- **Status fan-out** — `RunStatusBroadcast` polls all four motors every N ms
  and pushes `status` events to every registered subscriber channel (one per
  WebSocket connection).

## Architecture

```
pkg/t3d ──► pkg/robot ──► pkg/usecase ──► pkg/api ──► cmd/server
                │                  ▲
                └── pkg/runner ────┘
```

The `Robot` interface is declared in this package so the orchestrator can be
tested with a mock without importing `pkg/robot`.

## Event stream

Every operation function accepts an `out chan<- Event` parameter. Events are
written synchronously during execution and the function returns only after the
terminal event (`done` or `error`) has been sent.

| `kind`     | When                                        |
|------------|---------------------------------------------|
| `progress` | Intermediate step started (homing, moving…) |
| `done`     | Operation completed successfully            |
| `error`    | Operation failed or robot was busy          |
| `status`   | Periodic snapshot from `RunStatusBroadcast` |

```go
type Event struct {
    Kind    EventKind `json:"kind"`
    Message string    `json:"message,omitempty"`
    Payload any       `json:"payload,omitempty"` // SystemStatus for "status"
}
```

## Usage

```go
import (
    "github.com/losion445-max/motor-control-hub-v2/pkg/robot"
    "github.com/losion445-max/motor-control-hub-v2/pkg/usecase"
    "github.com/losion445-max/motor-control-hub-v2/pkg/runner"
)

sys := robot.NewSystem("/dev/ttyUSB0", 19200, robot.DefaultConfig)
sys.Connect()

orch := usecase.New(sys)

// Background status broadcast (cancel ctx to stop)
go orch.RunStatusBroadcast(ctx, 200*time.Millisecond)

// Subscribe a channel to receive status events
ch := make(chan usecase.Event, 16)
orch.Subscribe(ch)
defer orch.Unsubscribe(ch)

// Run a use case; out receives progress/done/error
out := make(chan usecase.Event, 32)
go func() {
    orch.Calibrate(ctx, out)
    close(out)
}()
for ev := range out {
    fmt.Println(ev.Kind, ev.Message)
}

// Execute a G-code program
orch.RunGcode(ctx, "G28\nG0 X700 Y1200\nG1 F1200 X140 Y2160", runner.DefaultOpts, out)

// Emergency stop (cancels active op + disables motors)
orch.Stop()
```

## Use cases

| Method         | Description                                          |
|----------------|------------------------------------------------------|
| `Calibrate`    | Runs homing sequence; sets the declared home position |
| `Move`         | Rapid move (G0-style) — no straight-line guarantee   |
| `Line`         | Straight-line move at constant feed rate (G1-style)  |
| `RunGcode`     | Parse and execute a G-code program string            |
| `Stop`         | Cancel current op + emergency stop all motors        |
| `HoldTension`  | Enable passive cable tension (no active positioning) |
| `Status`       | One-shot system status snapshot                      |

## SystemStatus payload

```go
type SystemStatus struct {
    Homed  bool          `json:"homed"`
    X      float64       `json:"x"`
    Y      float64       `json:"y"`
    Busy   bool          `json:"busy"`
    Motors []MotorStatus `json:"motors"` // one per drive, IDs 1–4
}

type MotorStatus struct {
    ID        int    `json:"id"`
    SpeedRPM  int    `json:"speed_rpm"`
    TorquePct int    `json:"torque_pct"`
    FaultCode uint16 `json:"fault_code"`
    Err       string `json:"err,omitempty"`
}
```

## Testing

The `Robot` interface makes the orchestrator fully testable with a mock,
without any serial port or physical hardware.

```
go test ./pkg/usecase/...
```

Tests cover: success paths, error propagation, busy-lock rejection for every
operation, `Stop` cancelling a blocked operation, `HoldTension`, `Status`
field verification, and broadcast subscriber fan-out / unsubscribe / slow
subscriber drop.
