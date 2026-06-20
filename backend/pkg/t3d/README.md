# pkg/t3d — HLTNC T3D Modbus RTU Driver

Low-level Go driver for the **HLTNC T3D series AC servo drives** over Modbus RTU /
RS-485. It handles frame assembly, CRC, bus arbitration, and all function codes
used by the drive (FC03 read params, FC04 read status, FC06 write single, FC10
write multiple, FC41 save EEPROM, FC42 servo on/off).

This package knows nothing about robots, cable lengths, or motion profiles. It is
the only package in the stack that touches serial hardware directly.

---

## Import

```go
import "github.com/losion445-max/motor-control-hub-v2/pkg/t3d"
```

---

## Quick start — two motors on one bus

```go
package main

import (
    "context"
    "fmt"
    "log"
    "time"

    "github.com/losion445-max/motor-control-hub-v2/pkg/t3d"
)

func main() {
    // One Bus owns the RS-485 port. All motors share it.
    bus := t3d.NewBus("/dev/ttyUSB0", 19200)
    if err := bus.Connect(); err != nil {
        log.Fatal(err)
    }
    defer bus.Close()

    m1 := t3d.NewMotor(bus, 1) // slave ID = P-181 value
    m2 := t3d.NewMotor(bus, 2)

    ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
    defer cancel()

    // Move motor 1 forward 5000 pulses (= 0.5 rev) at 100 RPM.
    if err := m1.MoveByPulses(ctx, 5000, 100); err != nil {
        log.Fatal(err)
    }

    // Read combined status snapshot.
    st, err := m2.ReadStatus()
    if err != nil {
        log.Fatal(err)
    }
    fmt.Printf("M2: speed=%d RPM, torque=%d%%, bus=%dV\n",
        st.SpeedRPM, st.TorquePct, st.BusVoltageV)
}
```

---

## API reference

### `Bus`

`NewBus(port string, baud int) *Bus` — create a bus for one RS-485 port. Parity is
fixed at **8E1** (T3D factory default P-183=1). Call `Connect()` before any motor
operations; call `Close()` at shutdown. All motor I/O is serialized through a
`sync.Mutex` inside Bus — you can safely call methods on different `Motor` instances
from different goroutines.

### `Motor`

`NewMotor(bus *Bus, slaveID byte) *Motor` — create a handle for one drive on the bus.
`slaveID` must match the drive's P-181 parameter (factory default: 1). Does not open
any connection — all I/O goes through the shared `Bus`.

**Status reads (FC04)**

| Method | Returns | Register |
|---|---|---|
| `ReadAbsPosition()` | int32 pulses (multi-turn, cumulative) | 0x1F/0x20 |
| `ReadPosition()` | alias for ReadAbsPosition | — |
| `ReadSpeed()` | int16 RPM (negative = reverse) | 0x00 |
| `ReadTorquePct()` | int16 % of rated torque | 0x09 |
| `ReadFault()` | uint16 fault code (0 = OK) | 0x1A |
| `ReadMotionState()` | pos, torque, fault (optimised 2-batch) | 0x09, 0x1A–0x20 |
| `ReadStatus()` | full `*Status` snapshot | all key registers |

Prefer `ReadMotionState()` inside control loops — it reads only the three values
needed for motion monitoring in two batched FC04 requests.

**Parameter access (FC03/FC06/FC10)**

| Method | FC | Notes |
|---|---|---|
| `ReadParam(addr)` | FC03 | Reads one P-xxx holding register |
| `WriteParam(addr, val)` | FC06 | Writes to RAM; call SaveEEPROM to persist |
| `WriteParams(start, vals)` | FC10 | Writes up to 10 consecutive registers |
| `ReadConfig()` | FC03 × 5 | Reads the 5 motion-control params into `*Config` |
| `SetupSpeedMode(rpm)` | FC06 × 5 | Sets P-098/004/025/100/137 for Modbus speed control |

**Servo on/off (FC42)**

`Enable()` and `Disable()` send the T3D proprietary FC42 command. `Disable` causes the
drive to decelerate per P-061 and then remove torque. Always disable before writing
P-137 (the drive returns exception 0x10 if you try to write it while running).

**Speed and torque setpoints**

`SetSpeed(rpm int)` — changes P-137 and re-enables the servo. Issues a brief
Disable → write → Enable cycle because P-137 is read-only while the drive is active.

`SetTorqueLimit(pct int)` — sets P-069 (0–300 % of rated, default 300). In speed
mode with a cable drum this acts as a hardware tension cap: the motor stalls when
the cable becomes taut and resumes if slack develops.

`SetAccelTime(msPerKRPM int)` — sets P-060 (acceleration ramp, ms per 1000 RPM).
Use `motion.AccelToT3DParam()` to convert from mm/s² to this unit.

`SetDecelTime(msPerKRPM int)` — sets P-061 (deceleration ramp, same unit).

**High-level motion**

`MoveByPulses(ctx, pulses, speedRPM)` — closed-loop move by a signed pulse count.
Two-phase: full speed until the approach zone, then 30 RPM for accurate stopping,
followed by a post-stop position verification and correction pass if error > 6 mm.
Torque is monitored throughout; a trip at 80 % rated aborts the move.

`RunUntilTorque(ctx, rpm, pct)` — runs at `rpm` until `|torque| ≥ pct`. Used for
cable tensioning (homing): motor winds until the cable is taut, then stops.

**EEPROM**

`SaveEEPROM()` — FC41: persists RAM parameters to non-volatile storage. Allow at
least 5 s before cutting power after this call.

### `Driver`

Single-motor convenience wrapper that bundles one `Bus` and one `Motor`. Use
`NewBus` + `NewMotor` directly when you need multiple motors — `Driver` is for
quick single-motor scripts or the TUI tool.

`Driver.Motor()` and `Driver.Bus()` return the underlying objects if you need to
escape the wrapper.

### `Status`

Snapshot struct returned by `ReadStatus()`. All fields are direct register values;
see `registers.go` for the mapping. Key fields: `SpeedRPM`, `TorquePct`,
`Position32` (= absolute multi-turn pulse count), `FaultCode`.

### `Config`

Five-field struct for the motion-control parameters (P-004/025/098/100/137).
Read with `ReadConfig()`.

---

## Configuration — register constants (`registers.go`)

The file exports every FC03 and FC04 address used by the driver as named constants
(`ParamXxx` and `StatusXxx`). Use these instead of raw numbers when calling
`ReadParam`/`WriteParam` directly.

Commonly tuned parameters:

| Constant | P-xxx | Default | Notes |
|---|---|---|---|
| `ParamAccelTime` | P-060 | 100 | ms / 1000 RPM ramp up |
| `ParamDecelTime` | P-061 | 100 | ms / 1000 RPM ramp down |
| `ParamTorqueLimit` | P-069 | 300 | % of rated; lower for tension cap |
| `ParamMaxSpeed` | P-075 | 3000 | RPM hard ceiling |
| `ParamInternalSpd1` | P-137 | — | active speed setpoint (signed RPM) |
| `ParamSlaveID` | P-181 | 1 | Modbus node address |
| `ParamBaudRate` | P-182 | 2 | 0=4800 1=9600 2=19200 3=38400 5=115200 |

---

## Physical setup

### RS-485 wiring

```
USB–RS-485 adapter
    A+ ──┬── A+ (drive 1) ── A+ (drive 2) ── … ── A+ (drive 4)
    B-  ──┴── B- (drive 1) ── B- (drive 2) ── … ── B- (drive 4)
    GND ──── GND (all drives)
```

- Use twisted-pair cable. Keep the bus stub short.
- Add a 120 Ω termination resistor between A+ and B- at the far end of the bus.
- Maximum cable length at 19200 baud: ~200 m (typical).

### First-time drive configuration

Set these parameters on each drive before connecting to this software:

```
P-181 = <slave ID>   ; 1, 2, 3, 4 for the four motors
P-182 = 2            ; 19200 baud
P-183 = 1            ; 8E1
P-004 = 1            ; speed control mode
P-025 = 1            ; internal multi-speed presets
P-098 = 1            ; servo-on always active (software controlled via FC42)
```

Then call `motor.SaveEEPROM()` (or FC41) to persist. Alternatively use
`motor.SetupSpeedMode(0)` + `motor.SaveEEPROM()` from code.

### Position register note

Use FC04 registers **0x1F / 0x20** (absolute multi-turn position) — not 0x05/0x06
(single-turn, resets each revolution). The `ReadAbsPosition()` method reads the
correct pair. The multi-turn counter accumulates from power-on.

---

## Implementation notes

- **P-137 is write-protected while running.** `SetSpeed()` issues Disable → write →
  Enable. Attempting to write it while enabled returns Modbus exception 0x10.
- **FC42 is non-standard.** The T3D uses function code 0x42 for servo on/off, outside
  the normal Modbus spec. `Bus.txRaw()` assembles the raw ADU for it.
- **CRC is little-endian.** Modbus RTU CRC bytes are transmitted LSB first (contrary
  to what some documentation implies). `buildADU` appends `[crc_lo, crc_hi]`.
- **Bus serialization.** The `Bus.tx()` and `Bus.txRaw()` calls hold `Bus.mu` for the
  entire round-trip. Multiple goroutines can call different motors safely, but
  requests are processed one at a time — no pipelining.
