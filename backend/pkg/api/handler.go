// Package api implements the WebSocket transport layer.
//
// Each connected client receives a continuous stream of status events
// and can send command messages to control the robot. All commands
// are dispatched to the usecase.Orchestrator.
//
// Wire protocol (JSON over WebSocket):
//
// Client → Server (commands):
//
//	{"id":"<uuid>","cmd":"home"}
//	{"id":"<uuid>","cmd":"move","x":700,"y":1200,"speed":50}
//	{"id":"<uuid>","cmd":"line","x":350,"y":600,"speed":20}
//	{"id":"<uuid>","cmd":"gcode","program":"G28\nG0 X700 Y1200"}
//	{"id":"<uuid>","cmd":"stop"}
//	{"id":"<uuid>","cmd":"hold_tension"}
//	{"id":"<uuid>","cmd":"status"}
//
// Server → Client (events):
//
//	{"id":"<uuid>","kind":"progress","message":"homing…"}
//	{"id":"<uuid>","kind":"done","message":"arrived (700, 1200)"}
//	{"id":"<uuid>","kind":"error","message":"robot busy"}
//	{"kind":"status","payload":{"homed":true,"x":700,"y":1200,"busy":false,"motors":[…]}}
package api

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/gorilla/websocket"
	"github.com/losion445-max/motor-control-hub-v2/pkg/runner"
	"github.com/losion445-max/motor-control-hub-v2/pkg/usecase"
)

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 4096,
	CheckOrigin:     func(*http.Request) bool { return true },
}

// inMsg is a command sent by the client.
type inMsg struct {
	ID      string  `json:"id"`
	Cmd     string  `json:"cmd"`
	X       float64 `json:"x"`
	Y       float64 `json:"y"`
	Speed   float64 `json:"speed"`
	Program string  `json:"program"`
}

// outMsg is sent to the client.
type outMsg struct {
	ID      string            `json:"id,omitempty"`
	Kind    usecase.EventKind `json:"kind"`
	Message string            `json:"message,omitempty"`
	Payload any               `json:"payload,omitempty"`
}

// Handler handles WebSocket connections.
type Handler struct {
	orch *usecase.Orchestrator
	opts runner.Opts
}

// NewHandler returns a handler backed by orch.
func NewHandler(orch *usecase.Orchestrator, opts runner.Opts) *Handler {
	return &Handler{orch: orch, opts: opts}
}

// ServeHTTP upgrades the connection to WebSocket and runs the client loop.
func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		slog.Warn("ws upgrade failed", "remote", r.RemoteAddr, "err", err)
		return
	}
	defer conn.Close()

	slog.Info("ws client connected", "remote", r.RemoteAddr)
	defer slog.Info("ws client disconnected", "remote", r.RemoteAddr)

	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()

	// Buffered channel so the write goroutine never blocks the control path.
	send := make(chan outMsg, 64)

	// Subscribe to broadcast status events.
	statusCh := make(chan usecase.Event, 16)
	h.orch.Subscribe(statusCh)
	defer h.orch.Unsubscribe(statusCh)

	// Forward status broadcasts → send.
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case ev := <-statusCh:
				select {
				case send <- outMsg{Kind: ev.Kind, Payload: ev.Payload}:
				default:
				}
			}
		}
	}()

	// Serialised write loop — gorilla/websocket is not concurrent-write-safe.
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case msg := <-send:
				if err := conn.WriteJSON(msg); err != nil {
					slog.Debug("ws write error", "remote", r.RemoteAddr, "err", err)
					cancel()
					return
				}
			}
		}
	}()

	// Read loop: receive commands from the client.
	for {
		_, raw, err := conn.ReadMessage()
		if err != nil {
			cancel()
			return
		}
		var in inMsg
		if err := json.Unmarshal(raw, &in); err != nil {
			slog.Warn("ws bad json", "remote", r.RemoteAddr, "err", err)
			select {
			case send <- outMsg{Kind: usecase.KindError, Message: "bad json: " + err.Error()}:
			case <-ctx.Done():
			}
			continue
		}

		slog.Debug("ws command", "remote", r.RemoteAddr, "cmd", in.Cmd, "id", in.ID)
		evCh := make(chan usecase.Event, 32)

		// Run the command in a goroutine; forward its events to the client.
		go func(msg inMsg) {
			defer close(evCh)
			h.dispatch(ctx, msg, evCh)
		}(in)

		go func(id string) {
			for ev := range evCh {
				select {
				case send <- outMsg{ID: id, Kind: ev.Kind, Message: ev.Message, Payload: ev.Payload}:
				case <-ctx.Done():
					return
				}
			}
		}(in.ID)
	}
}

func (h *Handler) dispatch(ctx context.Context, msg inMsg, out chan<- usecase.Event) {
	switch msg.Cmd {
	case "home":
		h.orch.Calibrate(ctx, out)

	case "move":
		spd := msg.Speed
		if spd <= 0 {
			spd = float64(h.opts.RapidMmPerSec)
		}
		h.orch.Move(ctx, msg.X, msg.Y, spd, out)

	case "line":
		spd := msg.Speed
		if spd <= 0 {
			spd = float64(h.opts.DefaultFeedMmPerSec)
		}
		h.orch.Line(ctx, msg.X, msg.Y, spd, out)

	case "gcode":
		h.orch.RunGcode(ctx, msg.Program, h.opts, out)

	case "stop":
		if err := h.orch.Stop(); err != nil {
			out <- usecase.Event{Kind: usecase.KindError, Message: err.Error()}
		} else {
			out <- usecase.Event{Kind: usecase.KindDone, Message: "all motors stopped"}
		}

	case "hold_tension":
		if err := h.orch.HoldTension(); err != nil {
			out <- usecase.Event{Kind: usecase.KindError, Message: err.Error()}
		} else {
			out <- usecase.Event{Kind: usecase.KindDone, Message: "passive tension active"}
		}

	case "status":
		out <- usecase.Event{
			Kind:    usecase.KindStatus,
			Payload: h.orch.Status(),
		}

	default:
		out <- usecase.Event{Kind: usecase.KindError, Message: "unknown command: " + msg.Cmd}
	}
}
