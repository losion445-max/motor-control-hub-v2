# cmd/server

Entry point for the robot WebSocket API server.

## Build

```bash
cd backend
go build -o server ./cmd/server
```

## Run

```bash
./server -port /dev/ttyUSB0 -addr :8080
```

| Flag     | Default        | Description                         |
|----------|----------------|-------------------------------------|
| `-port`  | `/dev/ttyUSB0` | RS-485 serial device for Modbus RTU |
| `-addr`  | `:8080`        | TCP address the HTTP server binds to |

## Startup sequence

1. Open serial port and connect to all four servo drives (Modbus RTU 19200/8E1)
2. Create `usecase.Orchestrator` wrapping the robot system
3. Start background goroutine: poll all motors every **200 ms**, broadcast
   `status` events to connected WebSocket clients
4. Start HTTP server (WebSocket at `/ws`, liveness probe at `/health`)
5. Block until `SIGINT` or `SIGTERM`

## Shutdown sequence (Ctrl-C)

1. `SIGINT` / `SIGTERM` received
2. HTTP server drained (5 s timeout)
3. `EmergencyStop` disables all four motors
4. Serial port closed

## Endpoints

See [pkg/api/README.md](../../pkg/api/README.md) for the full WebSocket
protocol, command reference, and event stream documentation.

```
GET /ws      WebSocket — robot control and event stream
GET /health  200 OK    — Kubernetes / Docker liveness probe
```

## Quick smoke test (requires `websocat`)

```bash
# Terminal 1
./server -port /dev/ttyUSB0

# Terminal 2
websocat ws://localhost:8080/ws
{"id":"1","cmd":"status"}
{"id":"2","cmd":"home"}
{"id":"3","cmd":"move","x":700,"y":1200,"speed":50}
```

## Architecture

```
cmd/server
    │
    ├── pkg/robot      (Modbus RTU, inverse kinematics, motion profiles)
    ├── pkg/usecase    (busy-lock, operation serialisation, status broadcast)
    ├── pkg/api        (WebSocket upgrade, JSON protocol, write loop)
    └── pkg/runner     (G-code execution, default feed rates)
```

Each layer imports only the layer directly below it; `cmd/server` is the only
place where all layers are wired together.
