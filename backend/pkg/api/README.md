# pkg/api

WebSocket transport layer. Upgrades HTTP connections, decodes JSON commands,
dispatches to `pkg/usecase.Orchestrator`, and streams events back to the
client. Also exposes a `/health` liveness probe.

## Endpoints

| Path      | Protocol  | Description                    |
|-----------|-----------|--------------------------------|
| `/ws`     | WebSocket | Robot control and event stream |
| `/health` | HTTP GET  | Returns `200 OK` (liveness)    |

## Wire protocol

All messages are JSON text frames.

### Client → Server (commands)

```json
{"id": "<opaque-id>", "cmd": "<command>", ...}
```

| Field     | Type   | Description                                      |
|-----------|--------|--------------------------------------------------|
| `id`      | string | Caller-chosen ID; echoed back on all response events |
| `cmd`     | string | Command name (see table below)                   |
| `x`, `y`  | number | Target coordinates in mm                         |
| `speed`   | number | Feed rate in mm/s (0 → use default)              |
| `program` | string | G-code program source (for `gcode` command)      |

#### Commands

| `cmd`          | Extra fields     | Description                                |
|----------------|------------------|--------------------------------------------|
| `home`         | —                | Run homing / calibration sequence          |
| `move`         | `x`, `y`, `speed` | Rapid move (no straight-line guarantee)   |
| `line`         | `x`, `y`, `speed` | Straight-line feed move                   |
| `gcode`        | `program`        | Execute a G-code program string            |
| `stop`         | —                | Cancel current op + emergency stop motors  |
| `hold_tension` | —                | Enable passive cable tension mode          |
| `status`       | —                | Request a one-shot system status snapshot  |

### Server → Client (events)

```json
{"id": "<cmd-id>", "kind": "<kind>", "message": "...", "payload": {...}}
```

| `kind`     | `id` present | `payload` type  | Description                       |
|------------|:------------:|-----------------|-----------------------------------|
| `progress` | yes          | —               | Intermediate step started         |
| `done`     | yes          | —               | Operation succeeded               |
| `error`    | yes (or no)  | —               | Operation failed / robot busy     |
| `status`   | no           | `SystemStatus`  | Periodic broadcast (every 200 ms) |

Status events are broadcast independently of any command; `id` is absent.
Command events carry the `id` of the originating command so the client can
correlate multiple concurrent in-flight operations.

### SystemStatus payload

```json
{
  "homed": true,
  "x": 700.0,
  "y": 1200.0,
  "busy": false,
  "motors": [
    {"id": 1, "speed_rpm": 0, "torque_pct": 12, "fault_code": 0},
    {"id": 2, "speed_rpm": 0, "torque_pct": 11, "fault_code": 0},
    {"id": 3, "speed_rpm": 0, "torque_pct": 13, "fault_code": 0},
    {"id": 4, "speed_rpm": 0, "torque_pct": 10, "fault_code": 0}
  ]
}
```

## Example session (pseudocode)

```
client → {"id":"1","cmd":"home"}
server → {"id":"1","kind":"progress","message":"homing: tensioning all cables…"}
server → {"id":"1","kind":"done","message":"homed — position declared (700, 1200)"}
server → {"kind":"status","payload":{…}}   ← background, every 200 ms

client → {"id":"2","cmd":"move","x":350,"y":600,"speed":50}
server → {"id":"2","kind":"progress","message":"rapid move → (350, 600)"}
server → {"id":"2","kind":"done","message":"arrived (350, 600)"}

client → {"id":"3","cmd":"stop"}
server → {"id":"3","kind":"done","message":"all motors stopped"}
```

## Internals

```
ServeHTTP
├── upgrade HTTP → WebSocket
├── goroutine: status broadcast → send channel
├── goroutine: write loop (serialised; gorilla requires single writer)
└── read loop
    └── per command: goroutine dispatch + goroutine forward events → send
```

The `send` channel (buffer 64) decouples the write loop from command
goroutines. If the client is too slow to read and `send` fills up, the
connection is closed by the write goroutine returning an error, not by
blocking the robot control path.

## Usage

```go
import "github.com/losion445-max/motor-control-hub-v2/pkg/api"

h := api.NewHandler(orch, runner.DefaultOpts)
srv := api.NewServer(":8080", h)
srv.ListenAndServe()

// Graceful shutdown
api.Shutdown(ctx, srv)
```

## Testing

```
go test ./pkg/api/...
```

Tests use `net/http/httptest.Server` and `gorilla/websocket` dialer so no
real hardware or port is needed. Covered cases: all commands (home, move,
line, gcode, stop, hold_tension, status), default speed fallback, unknown
command, malformed JSON, busy rejection, and two independent clients.
