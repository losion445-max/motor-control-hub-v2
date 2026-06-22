# motor-control-hub-v2

Control system for a 4-cable suspended cable-driven parallel robot (CDPR) — a camera/sensor platform that moves in a 2D vertical plane via four servo-driven cables. The system consists of a Go backend (Modbus RTU over RS-485) and a React web control panel.

```
M1(0,0) ─────────────────── M2(W,0)
  │   \                   /   │
  │     \    [camera]   /     │
  │       \           /       │
M4(0,H) ─────────────────── M3(W,H)
```

---

## Contents

- [Hardware](#hardware)
- [Software architecture](#software-architecture)
- [Quick start](#quick-start)
- [Installation](#installation)
- [Configuration reference](#configuration-reference)
- [Calibration procedure](#calibration-procedure)
- [Control modes](#control-modes)
- [Speed limits](#speed-limits)
- [WebSocket API](#websocket-api)
- [G-code reference](#g-code-reference)
- [Makefile targets](#makefile-targets)
- [Test coverage](#test-coverage)
- [Upgrading baud rate to 115200](#upgrading-baud-rate-to-115200)

---

## Hardware

### Servo motors — 80AST-A1C04025

| Parameter | Value |
|-----------|-------|
| Rated power | 400 W |
| Rated speed | 2000 RPM |
| Max speed | 3000 RPM |
| Rated torque | 2.4 Nm |
| Encoder | 2500-line incremental × 4 = **10 000 PPR** |
| Max cable linear speed | 2000 × 2π × 67.8 / 60 ≈ **237 mm/s** |

### Servo drives — HLTNC T3D-L20A-RABF

| Parameter | Value |
|-----------|-------|
| Communication | Modbus RTU over RS-485 |
| Default baud | 19 200 / 8E1 |
| Max baud | 115 200 |
| Slave IDs | 1 (M1) · 2 (M2) · 3 (M3) · 4 (M4) |
| Control mode | Speed (P-004 = 1), internal preset P-137 |

### Frame

| Parameter | Value |
|-----------|-------|
| Width (M1 → M2) | 1400 mm |
| Height (M1 → M4) | 2400 mm |
| Cable drum radius | 67.8 mm (effective, mid-cable) |

### Motor layout — slave IDs (clockwise from top-left)

```
M1 (slave 1)  ──────  M2 (slave 2)
     │                      │
     │       [camera]       │
     │                      │
M4 (slave 4)  ──────  M3 (slave 3)
```

Positive RPM = wind cable in (cable gets shorter). Negative RPM = pay out.

### Wiring

```
PC/SBC ──[USB]── USB-RS485 adapter ──[RS-485 bus]── T3D #1
                                                  ├── T3D #2
                                                  ├── T3D #3
                                                  └── T3D #4
```

Use a twisted-pair RS-485 cable. Add a 120 Ω termination resistor between A/B at each end of the bus if the bus is longer than ~3 m or if you experience communication errors.

---

## Software architecture

```
backend/
  cmd/server/         — entry point: loads config, connects drives, starts HTTP+WS
  pkg/config/         — TOML config loader (strict: unknown keys rejected)
  pkg/t3d/            — Modbus RTU driver for T3D drives
    bus.go            — RS-485 bus, mutex-serialised transactions
    motor.go          — per-motor API: Enable/Disable, MoveByPulses, RunUntilTorque…
    driver.go         — thin wrapper (Driver) that adds slave-ID awareness
    registers.go      — complete FC03/FC04 register map (P-numbers + status)
  pkg/robot/          — CDPR kinematics and motion controllers
    kinematics.go     — cable length ↔ Cartesian conversion, pulsesPerMM
    system.go         — System: Home, MoveTo, EmergencyStop, HoldTension…
    line.go           — LineTo: continuous closed-loop velocity controller
    config.go         — Config struct + DefaultConfig
  pkg/motion/         — trapezoidal velocity profile (hardware-independent)
  pkg/gcode/          — G-code parser (G0/G1/G28, X/Y/F words)
  pkg/runner/         — G-code executor (wraps robot.System)
  pkg/usecase/        — Orchestrator: serialises ops, fans out status events
  pkg/api/            — WebSocket handler + HTTP server

frontend/
  src/api/            — WsClient singleton, TypeScript types
  src/hooks/          — useCommand, useRobotStatus, useWsConnection
  src/context/        — WsProvider
  src/components/     — shadcn/ui primitives + layout, control, motor, gcode panels
  src/pages/          — ControlPage, GcodePage, SettingsPage
```

### Layer rules

```
t3d → robot → usecase → api → cmd/server
              gcode ─┘
              motion ─┘
              runner ─┘
```

No layer may import a layer above it. `pkg/t3d` and `pkg/motion` have zero cross-dependencies.

### Dependency versions

| Component | Version |
|-----------|---------|
| Go | 1.26 |
| `goburrow/modbus` | 0.1.0 |
| `gorilla/websocket` | 1.5.3 |
| `BurntSushi/toml` | 1.6.0 |
| React | 19 |
| Vite | 8 |
| TypeScript | 6 |
| Tailwind CSS | 3 |

---

## Quick start

```bash
# 1. Clone
git clone <repo> motor-control-hub-v2
cd motor-control-hub-v2

# 2. Install frontend dependencies (once)
make install

# 3. Add your user to the dialout group (once — then re-login)
sudo usermod -aG uucp $USER   # Arch / Manjaro
# or
sudo usermod -aG dialout $USER  # Debian / Ubuntu

# 4. Plug in the USB-RS485 adapter → /dev/ttyUSB0

# 5. Start everything
make start
```

Open **http://localhost:5173** in a browser. The backend runs on `:8080`, WebSocket on `ws://localhost:8080/ws`.

To stop:

```bash
Ctrl-C          # stops the frontend dev server
make stop       # kills the backend (uses /tmp/mch-backend.pid)
```

### Override serial port / baud at runtime

```bash
make start PORT=/dev/ttyUSB1
make start PORT=/dev/ttyUSB0 BAUD=115200
```

This patches a temporary copy of `config.toml` — the file on disk is not modified.

---

## Installation

### Prerequisites

| Tool | Min version | Install |
|------|-------------|---------|
| Go | 1.22 | `pacman -S go` / [go.dev](https://go.dev/dl/) |
| Node.js | 20 | `pacman -S nodejs npm` |
| golangci-lint | any | optional, for `make lint` |

### Build

```bash
make build           # → backend/bin/server
make frontend-build  # → frontend/dist/
```

### Run (production — backend only, serve frontend as static files)

After `make frontend-build` you can serve `frontend/dist/` from any static HTTP server, or add a static-file handler to the Go server (not currently wired — the dev proxy in Vite covers this for now).

```bash
make run                         # uses backend/config.toml
make run CONFIG=my-config.toml   # custom config file
```

---

## Configuration reference

All configuration lives in `backend/config.toml`. The server rejects unknown keys at startup (prevents silent misconfiguration).

```toml
[server]
addr               = ":8080"        # HTTP listen address (WebSocket: /ws, health: /health)
serial_port        = "/dev/ttyUSB0" # RS-485 device node
baud_rate          = 19200          # must match drive P-182 (see baud table below)
status_interval_ms = 200            # how often motor state is polled and broadcast (ms)
log_format         = "text"         # "text" (dev/human) | "json" (production/systemd)
log_level          = "info"         # "debug" | "info" | "warn" | "error"

[hardware]
width_mm       = 1400.0   # frame width: horizontal M1→M2 distance (mm)
height_mm      = 2400.0   # frame height: vertical M1→M4 distance (mm)
drum_radius_mm = 67.8     # effective cable drum radius at mid-cable (mm)
pulses_per_rev = 10000    # encoder PPR (80AST-A1C04025: 2500-line × 4)

[homing]
rpm        = 25   # cable winding speed during calibration (RPM)
torque_pct = 18   # torque threshold that signals cable is fully taut (% of rated)

[safety]
torque_pct = 75   # emergency stop threshold during any move (% of rated 2.4 Nm)

[hold_tension]
torque_pct = 30   # torque cap applied via P-069 during tension-hold mode (%)
rpm        = 20   # slow winding speed to maintain tension without moving (RPM)

[move]
# MoveTo — stop-and-go trapezoidal controller (used for rapid/non-linear moves)
accel_mm_per_sec2   = 1000.0  # trapezoidal accel/decel magnitude (0 = drive default)
approach_rpm        = 30      # slow final-approach speed for precision stopping (RPM)
approach_factor     = 5       # approach zone = max(factor × maxRPM, min_approach_pulses)
min_approach_pulses = 50      # minimum approach zone in pulses (≈ 2 mm)
tolerance_pulses    = 50      # stop condition: remaining error ≤ this (≈ 2 mm)
poll_ms             = 15      # encoder read interval during move (ms)
stop_settle_ms      = 150     # wait after all motors stop before declaring done (ms)
disable_wait_ms     = 80      # pause after disable before reading starting positions (ms)
approach_switch_ms  = 30      # pause between disable and re-enable at approach speed (ms)

[line]
# LineTo — continuous closed-loop velocity controller (straight Cartesian lines)
tick_ms           = 100   # control loop period (ms) — limited by Modbus read time at 19200 baud
correction_gain   = 3.0   # proportional gain: (mm/s cable speed) per mm cable-length error
fault_check_every = 20    # read fault registers every N ticks (≈ 2 s at 100 ms tick)
settle_tol_pulses = 50    # settle condition: all cable errors < this (≈ 2 mm)
settle_timeout_s  = 3.0   # max time in settle phase before LineTo gives up (s)

[gcode]
rapid_mm_per_sec        = 200.0  # G0 rapid move speed (mm/s)
default_feed_mm_per_sec = 20.0   # G1 feed rate before first F word (mm/s)
```

### Baud rate table (P-182)

| P-182 value | Baud rate |
|-------------|-----------|
| 0 | 4 800 |
| 1 | 9 600 |
| 2 | **19 200** (factory default) |
| 3 | 38 400 |
| 4 | 57 600 |
| 5 | 115 200 |

---

## Calibration procedure

Calibration (homing) must be run once after power-on before any motion command.

**What happens internally:**

1. All 4 motors are set to `torque_limit = homing.torque_pct` (18 % of rated ≈ 0.43 Nm).
2. All 4 motors are enabled at `homing.rpm = 25` RPM in the wind-in direction.
3. Each motor is monitored independently. When its torque reaches or exceeds the threshold, the cable is considered taut and the motor is disabled.
4. The absolute encoder position is recorded as the home reference.
5. The camera position is declared `(0, 0)` — top-left corner of the frame.

**From the web panel:**  
Control tab → **Calibrate / Home** button.

**From G-code:**  
```
G28
```

**After homing**, the `HOMED` badge appears in the top bar and all motion commands become available.

---

## Control modes

### MoveTo — trapezoidal stop-and-go

Used for rapid repositioning. Each motor is driven independently to its target cable length using a trapezoidal speed profile. Motion is not guaranteed to follow a straight Cartesian line.

```
Phase 1 (cruise):   all motors run at commanded RPM toward their targets
Phase 2 (approach): when remaining < approach_zone, speed drops to approach_rpm
Phase 3 (stop):     when remaining < tolerance_pulses, motors disabled
Phase 4 (settle):   stop_settle_ms wait
```

Triggered by G-code `G0`, or the **Rapid Move** button in the web panel.

### LineTo — closed-loop velocity controller

Used for straight-line Cartesian motion. All 4 cable speeds are computed simultaneously every tick via inverse kinematics with feed-forward + P-correction.

```
Each tick (default 100 ms):
  1. Read absolute encoder positions for all 4 motors (≈ 48 ms at 19200 baud)
  2. Compute ideal cable lengths at profile position now + 1 tick ahead
  3. Feed-forward speed = (len_next − len_now) / dt  [mm/s cable space]
  4. Correction = correction_gain × (len_desired − len_actual)  [mm/s]
  5. Combined speed → RPM → write to each drive via Modbus
  6. Sleep remainder of tick period
Settle phase: after velocity profile ends, run pure correction until
  all cable errors < settle_tol_pulses, or settle_timeout_s elapses
```

Triggered by G-code `G1`, or the **Line Move** / **Jog** buttons in the web panel.

### HoldTension

Passive mode that keeps cables taut without commanded motion. All motors wind in slowly at `hold_tension.rpm` with `hold_tension.torque_pct` as the torque cap. Safe to leave running indefinitely.

---

## Speed limits

| Limit | Value | Cause |
|-------|-------|-------|
| Motor max cable speed | **237 mm/s** | 2000 RPM × 2π × 67.8 mm / 60 |
| Camera max at workspace edge | **~237 mm/s** | cables nearly horizontal at top/bottom rows |
| Camera max at workspace centre | **~470 mm/s** | geometric advantage (cable angle ≈ 60°) but motor-limited |
| Practical LineTo max (19200 baud) | **~150 mm/s** | Modbus poll takes ≈ 48 ms of the 100 ms tick |
| Practical LineTo max (115200 baud) | **~200 mm/s** | Modbus poll drops to ≈ 9 ms; tick can be reduced to 25 ms |
| G-code rapid default | 200 mm/s | `gcode.rapid_mm_per_sec` in config |
| G-code feed default | 20 mm/s | `gcode.default_feed_mm_per_sec` in config |

**Serpentine (snake) scan — full field at 50 mm row pitch:**

| Speed | Time per row | Total (1400×2400, 48 rows) |
|-------|-------------|---------------------------|
| 80 mm/s | 17.6 s | ≈ 14 min |
| 150 mm/s | 9.5 s | ≈ 8 min |
| 200 mm/s | 7.2 s | ≈ 6 min |

At 100 ms tick the position command steps 20 mm per iteration at 200 mm/s, which is at the practical accuracy limit. For coverage scans requiring < 5 mm repeatability, use ≤ 100 mm/s.

---

## WebSocket API

Connect to `ws://<host>:8080/ws`.

### Commands (client → server)

All commands carry a client-generated `id` (UUID). The server echoes the same `id` on all response events for that command.

```jsonc
// Calibrate / home all cables
{"id":"<uuid>","cmd":"home"}

// Rapid move (non-straight path, motor-independent)
{"id":"<uuid>","cmd":"move","x":700,"y":1200,"speed":50}

// Straight-line move (closed-loop Cartesian)
{"id":"<uuid>","cmd":"line","x":350,"y":600,"speed":20}

// Execute a G-code program
{"id":"<uuid>","cmd":"gcode","program":"G28\nG0 X700 Y1200\nG1 X0 Y0 F1200"}

// Emergency stop — always safe to call; cancels current operation
{"id":"<uuid>","cmd":"stop"}

// Enable passive cable tension hold
{"id":"<uuid>","cmd":"hold_tension"}

// Request a one-shot status snapshot
{"id":"<uuid>","cmd":"status"}
```

`speed` is in mm/s. Omit or set to `0` to use the configured default.

### Events (server → client)

```jsonc
// Operation in progress
{"id":"<uuid>","kind":"progress","message":"homing: tensioning all cables…"}

// Operation completed successfully
{"id":"<uuid>","kind":"done","message":"arrived (700, 1200)"}

// Operation failed
{"id":"<uuid>","kind":"error","message":"robot busy"}

// Periodic status broadcast (no id — sent to all connected clients)
{
  "kind": "status",
  "payload": {
    "homed": true,
    "x": 700.0,
    "y": 1200.0,
    "busy": false,
    "motors": [
      {"id":1,"speed_rpm":0,"torque_pct":5,"fault_code":0},
      {"id":2,"speed_rpm":0,"torque_pct":4,"fault_code":0},
      {"id":3,"speed_rpm":0,"torque_pct":6,"fault_code":0},
      {"id":4,"speed_rpm":0,"torque_pct":5,"fault_code":0}
    ]
  }
}
```

Status is broadcast every `status_interval_ms` (default 200 ms) to all connected clients.

### REST

```
GET /health   → 200 OK
```

### Concurrency

The orchestrator serialises all motion commands. Sending a second motion command while one is running returns `{"kind":"error","message":"robot busy"}`. `stop` and `hold_tension` are always accepted regardless of busy state.

---

## G-code reference

The parser implements a subset of RS-274/NGC. Unknown codes are silently ignored (forward-compatible).

| Code | Meaning | Example |
|------|---------|---------|
| `G0` | Rapid move (fast, non-straight) | `G0 X700 Y1200` |
| `G1` | Linear move at feed rate | `G1 X350 Y600 F1200` |
| `G28` | Return to home position | `G28` |

**Words:**

| Word | Meaning | Unit |
|------|---------|------|
| `X` | Target X position | mm |
| `Y` | Target Y position | mm |
| `F` | Feed rate (modal, G1 only) | mm/min |
| `N` | Line number | — (ignored) |

**Notes:**

- G0/G1 are **modal** — the last active code applies to subsequent lines with X/Y but no G word.
- F is modal — persists until the next F word.
- F is in **mm/min** (standard G-code convention). Divide by 60 to get mm/s.  
  Example: `F1200` = 20 mm/s.
- Comments: `;` to end of line, or `(` … `)` blocks.
- Both spaced (`G1 X100 Y200`) and compact (`G1X100Y200`) syntax accepted.

**Example program:**

```gcode
; Serpentine scan, 200 mm row pitch
G28              ; home
G0 X0 Y0        ; rapid to top-left
G1 X1400 Y0 F1200   ; scan row 0 → right at 20 mm/s
G1 X1400 Y200       ; step down
G1 X0 Y200          ; scan row 200 ← left
G1 X0 Y400
G1 X1400 Y400
; ... etc
```

---

## Makefile targets

```
make help              Show this list

make build             Compile backend → backend/bin/server
make frontend-build    Build frontend → frontend/dist/
make install           npm install for the frontend (run once)

make run               Build + run backend  [CONFIG=…  PORT=…  BAUD=…]
make dev               Run backend with race detector
make ui                Run frontend dev server → http://localhost:5173
make start             Start backend (background) + frontend dev server (foreground)
make stop              Kill background backend started by 'make start'

make test              Run all backend tests
make test-v            Run all backend tests (verbose)
make test-race         Run all backend tests with race detector
make lint              Run golangci-lint
make tidy              go mod tidy

make clean             Remove backend binary and frontend/dist/
```

**Override variables:**

```bash
make run CONFIG=custom.toml          # different config file
make run PORT=/dev/ttyUSB1           # different serial device
make run PORT=/dev/ttyUSB0 BAUD=115200  # different baud rate
```

PORT and BAUD patch a temporary copy of `config.toml` in `/tmp` — the original is not modified.

---

## Test coverage

All testable packages maintain ≥ 80 % statement coverage. Hardware-dependent code (`Bus.Connect`, `Bus.tx`, `Bus.txRaw`) is excluded from unit tests by design — it requires a live RS-485 port.

| Package | Coverage | Notes |
|---------|----------|-------|
| `pkg/config` | 95.5 % | full Load path including validation |
| `pkg/motion` | 97.4 % | trapezoidal profile, AccelToT3DParam |
| `pkg/usecase` | 90.2 % | orchestrator, broadcast, all use cases |
| `pkg/robot` | 88.1 % | system, homing, LineTo, kinematics |
| `pkg/api` | 86.6 % | WS handler, health endpoint |
| `pkg/t3d` | 80.4 % | motor methods via `busTransport` mock |

### Testability interfaces

Two interfaces allow full unit testing without hardware:

**`busTransport`** (`pkg/t3d/bus.go`) — decouples `Motor` from the physical RS-485 bus:
```go
type busTransport interface {
    tx(slaveID byte, fn func(modbus.Client) error) error
    txRaw(slaveID, fc byte, data []byte) ([]byte, error)
}
```

**`driveMotor`** (`pkg/robot/system.go`) — decouples `System` from `*t3d.Motor`:
```go
type driveMotor interface {
    Enable() error
    Disable() error
    WriteParam(addr, value uint16) error
    ReadAbsPosition() (int32, error)
    ReadTorquePct() (int16, error)
    ReadFault() (uint16, error)
    ReadStatus() (*t3d.Status, error)
    ReadMotionState() (int32, int16, uint16, error)
    SetAccelTime(msPerKRPM int) error
    SetDecelTime(msPerKRPM int) error
    SetSpeed(rpm int) error
    SetTorqueLimit(pct int) error
}
```

Run tests:
```bash
make test        # all packages, no race detector
make test-race   # with -race flag
```

---

## Upgrading baud rate to 115200

At 19200 baud, reading 4 motor positions (4 × `ReadAbsPosition`) takes ≈ 48 ms — over half the 100 ms control tick budget. At 115200 baud the same reads take ≈ 9 ms, enabling a tighter control loop.

### Step 1 — reprogram all 4 drives (do this at current baud rate)

```go
// For each drive at its current 19200 baud:
m.WriteParam(t3d.ParamBaudRate, 5) // P-182 = 5 → 115200
m.SaveEEPROM()
// Drive stops responding at 19200 baud immediately after SaveEEPROM.
// Reconnect at 115200 to confirm.
```

### Step 2 — update config.toml

```toml
[server]
baud_rate = 115200

[line]
tick_ms         = 25    # was 100 — now Modbus read takes only ≈18 ms per tick
correction_gain = 2.0   # reduce slightly; tighter loop is more responsive
```

### What this unlocks

| | 19200 baud | 115200 baud |
|-|-----------|------------|
| Modbus read (4 motors) | ≈ 48 ms | ≈ 9 ms |
| Control loop frequency | 10 Hz | **40 Hz** |
| Position step at 200 mm/s | 20 mm / tick | **5 mm / tick** |
| Practical max LineTo speed | ~150 mm/s | **~200 mm/s** |

**Cable quality note:** at 115200 baud, reflections on RS-485 lines longer than ~3 m can cause framing errors. Ensure 120 Ω termination resistors are present at both ends of the bus.

---

## Fault codes

Fault codes are returned in the `fault_code` field of the status payload. The value maps to T3D P-fault register (FC04 address `0x1A`). Common codes:

| Code | Cause |
|------|-------|
| 0 | No fault |
| 1 | Overcurrent |
| 2 | Overvoltage |
| 3 | Undervoltage |
| 4 | Motor overload |
| 5 | Drive overtemperature |
| 6 | Encoder error |
| 11 | Position deviation too large |

A non-zero fault code causes `LineTo` to issue an emergency stop immediately. Faults are checked every `line.fault_check_every` ticks (default: every 20 ticks ≈ 2 s).

Clear faults by power-cycling the drive or writing the fault-clear command (refer to the T3D Modbus manual, section on fault handling).

---

## Project layout

```
motor-control-hub-v2/
├── Makefile
├── README.md
├── backend/
│   ├── config.toml                  ← single source of truth for all parameters
│   ├── go.mod
│   ├── cmd/
│   │   └── server/main.go           ← entry point
│   └── pkg/
│       ├── api/                     ← WebSocket handler, HTTP server
│       ├── config/                  ← TOML config loader + validation
│       ├── gcode/                   ← G-code parser
│       ├── motion/                  ← trapezoidal velocity profile
│       ├── robot/                   ← CDPR kinematics + motion controllers
│       ├── runner/                  ← G-code executor
│       ├── t3d/                     ← Modbus RTU driver for T3D drives
│       └── usecase/                 ← orchestrator, event broadcast
└── frontend/
    ├── package.json
    ├── vite.config.ts
    ├── tailwind.config.js
    └── src/
        ├── api/                     ← WsClient, TypeScript types
        ├── context/                 ← WsProvider
        ├── hooks/                   ← useCommand, useRobotStatus, useWsConnection
        ├── components/
        │   ├── ui/                  ← shadcn/ui primitives
        │   ├── layout/              ← TopBar
        │   ├── control/             ← WorkspaceView, MovePanel, JogPad, buttons
        │   ├── motors/              ← MotorCard, MotorGrid
        │   └── gcode/               ← upload, file list, viewer, controls
        └── pages/                   ← ControlPage, GcodePage, SettingsPage
```
