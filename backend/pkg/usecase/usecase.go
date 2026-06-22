// Package usecase orchestrates robot operation scenarios.
//
// It sits above pkg/robot and pkg/runner: it composes their primitives into
// named use cases (Calibrate, Move, RunGcode, …), serialises concurrent
// access via a busy-lock, propagates cancellation to hardware, and broadcasts
// periodic status snapshots to all connected clients.
//
// Layer rule: usecase imports robot/gcode/runner; nothing below imports usecase.
package usecase

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/losion445-max/motor-control-hub-v2/pkg/gcode"
	"github.com/losion445-max/motor-control-hub-v2/pkg/robot"
	"github.com/losion445-max/motor-control-hub-v2/pkg/runner"
)

// ── Event types ───────────────────────────────────────────────────────────────

// EventKind classifies a streamed event.
type EventKind string

const (
	KindProgress EventKind = "progress" // operation is in progress
	KindDone     EventKind = "done"     // operation completed successfully
	KindError    EventKind = "error"    // operation failed
	KindStatus   EventKind = "status"   // periodic system status broadcast
)

// Event is a single message emitted during or after an operation.
type Event struct {
	Kind    EventKind `json:"kind"`
	Message string    `json:"message,omitempty"`
	Payload any       `json:"payload,omitempty"`
}

// ── Status snapshot ───────────────────────────────────────────────────────────

// MotorStatus is one motor's runtime snapshot, safe to serialise to JSON.
type MotorStatus struct {
	ID        int    `json:"id"`
	SpeedRPM  int    `json:"speed_rpm"`
	TorquePct int    `json:"torque_pct"`
	FaultCode uint16 `json:"fault_code"`
	Err       string `json:"err,omitempty"`
}

// SystemStatus is broadcast to all clients every StatusInterval.
type SystemStatus struct {
	Homed  bool          `json:"homed"`
	X      float64       `json:"x"`
	Y      float64       `json:"y"`
	Busy   bool          `json:"busy"`
	Motors []MotorStatus `json:"motors"`
}

// ── Robot interface ───────────────────────────────────────────────────────────

// Robot is the subset of *robot.System consumed by the orchestrator.
// Defined here so pkg/usecase can be tested with a mock.
type Robot interface {
	Home(ctx context.Context) error
	MoveTo(ctx context.Context, x, y, speedMmPerSec float64) error
	LineTo(ctx context.Context, x, y, speedMmPerSec float64) error
	EmergencyStop() error
	HoldTension() error
	Position() (float64, float64)
	Homed() bool
	ReadAllStatus() [4]robot.MotorState
}

// ── Orchestrator ──────────────────────────────────────────────────────────────

// Orchestrator serialises robot operations and fans out status events.
//
// Only one motion operation runs at a time. Calling Stop cancels the current
// operation context and immediately disables all motors.
type Orchestrator struct {
	robot Robot

	mu       sync.Mutex
	busy     bool
	cancelOp context.CancelFunc // cancels the running operation, if any

	subsMu sync.RWMutex
	subs   map[chan<- Event]struct{}
}

// New creates an Orchestrator wrapping r.
func New(r Robot) *Orchestrator {
	return &Orchestrator{
		robot: r,
		subs:  make(map[chan<- Event]struct{}),
	}
}

// Subscribe registers ch to receive periodic status broadcasts.
// The caller must call Unsubscribe when the channel is no longer needed.
func (o *Orchestrator) Subscribe(ch chan<- Event) {
	o.subsMu.Lock()
	o.subs[ch] = struct{}{}
	o.subsMu.Unlock()
}

// Unsubscribe removes ch from the broadcast set.
func (o *Orchestrator) Unsubscribe(ch chan<- Event) {
	o.subsMu.Lock()
	delete(o.subs, ch)
	o.subsMu.Unlock()
}

// RunStatusBroadcast polls the robot every interval and broadcasts a status
// event to all subscribers. Blocks until ctx is cancelled.
func (o *Orchestrator) RunStatusBroadcast(ctx context.Context, interval time.Duration) {
	t := time.NewTicker(interval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			o.broadcast(o.statusEvent())
		}
	}
}

func (o *Orchestrator) broadcast(ev Event) {
	o.subsMu.RLock()
	defer o.subsMu.RUnlock()
	for ch := range o.subs {
		select {
		case ch <- ev:
		default: // drop if subscriber is slow; never block here
		}
	}
}

func (o *Orchestrator) statusEvent() Event {
	states := o.robot.ReadAllStatus()
	x, y := o.robot.Position()

	motors := make([]MotorStatus, 4)
	for i, s := range states {
		ms := MotorStatus{ID: s.ID}
		if s.Err != nil {
			ms.Err = s.Err.Error()
		} else if s.Status != nil {
			ms.SpeedRPM = int(s.Status.SpeedRPM)
			ms.TorquePct = int(s.Status.TorquePct)
			ms.FaultCode = s.Status.FaultCode
		}
		motors[i] = ms
	}

	o.mu.Lock()
	busy := o.busy
	o.mu.Unlock()

	return Event{
		Kind: KindStatus,
		Payload: SystemStatus{
			Homed:  o.robot.Homed(),
			X:      x,
			Y:      y,
			Busy:   busy,
			Motors: motors,
		},
	}
}

// acquire marks the orchestrator as busy and stores cancel for Stop.
// Returns false if already busy.
func (o *Orchestrator) acquire(cancel context.CancelFunc) bool {
	o.mu.Lock()
	defer o.mu.Unlock()
	if o.busy {
		return false
	}
	o.busy = true
	o.cancelOp = cancel
	return true
}

func (o *Orchestrator) release() {
	o.mu.Lock()
	o.busy = false
	o.cancelOp = nil
	o.mu.Unlock()
}

// ── Use cases ─────────────────────────────────────────────────────────────────

// Calibrate runs the homing sequence. Progress and result are sent to out.
// The caller must drain out until it is closed.
func (o *Orchestrator) Calibrate(ctx context.Context, out chan<- Event) {
	opCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	if !o.acquire(cancel) {
		slog.Warn("calibrate rejected: robot busy")
		out <- Event{Kind: KindError, Message: "robot busy"}
		return
	}
	defer o.release()

	slog.Info("calibrate started")
	out <- Event{Kind: KindProgress, Message: "homing: tensioning all cables…"}
	if err := o.robot.Home(opCtx); err != nil {
		slog.Warn("calibrate failed", "err", err)
		out <- Event{Kind: KindError, Message: err.Error()}
		return
	}
	x, y := o.robot.Position()
	slog.Info("calibrate done", "x", x, "y", y)
	out <- Event{Kind: KindDone, Message: fmt.Sprintf("homed — position declared (%.0f, %.0f)", x, y)}
}

// Move rapids the camera to (x, y) at speed mm/s (no straight-line guarantee).
func (o *Orchestrator) Move(ctx context.Context, x, y, speed float64, out chan<- Event) {
	opCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	if !o.acquire(cancel) {
		slog.Warn("move rejected: robot busy", "x", x, "y", y)
		out <- Event{Kind: KindError, Message: "robot busy"}
		return
	}
	defer o.release()

	slog.Info("move started", "x", x, "y", y, "speed_mm_s", speed)
	out <- Event{Kind: KindProgress, Message: fmt.Sprintf("rapid move → (%.0f, %.0f)", x, y)}
	if err := o.robot.MoveTo(opCtx, x, y, speed); err != nil {
		slog.Warn("move failed", "x", x, "y", y, "err", err)
		out <- Event{Kind: KindError, Message: err.Error()}
		return
	}
	slog.Info("move done", "x", x, "y", y)
	out <- Event{Kind: KindDone, Message: fmt.Sprintf("arrived (%.0f, %.0f)", x, y)}
}

// Line moves the camera in a straight Cartesian line to (x, y) at speed mm/s.
func (o *Orchestrator) Line(ctx context.Context, x, y, speed float64, out chan<- Event) {
	opCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	if !o.acquire(cancel) {
		slog.Warn("line rejected: robot busy", "x", x, "y", y)
		out <- Event{Kind: KindError, Message: "robot busy"}
		return
	}
	defer o.release()

	slog.Info("line started", "x", x, "y", y, "speed_mm_s", speed)
	out <- Event{Kind: KindProgress, Message: fmt.Sprintf("line → (%.0f, %.0f) at %.0f mm/s", x, y, speed)}
	if err := o.robot.LineTo(opCtx, x, y, speed); err != nil {
		slog.Warn("line failed", "x", x, "y", y, "err", err)
		out <- Event{Kind: KindError, Message: err.Error()}
		return
	}
	slog.Info("line done", "x", x, "y", y)
	out <- Event{Kind: KindDone, Message: fmt.Sprintf("arrived (%.0f, %.0f)", x, y)}
}

// RunGcode parses and executes src as a G-code program.
// Progress events are emitted before each command and on completion.
func (o *Orchestrator) RunGcode(ctx context.Context, src string, opts runner.Opts, out chan<- Event) {
	opCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	cmds, err := gcode.Parse(src)
	if err != nil {
		slog.Warn("gcode parse error", "err", err)
		out <- Event{Kind: KindError, Message: "gcode parse: " + err.Error()}
		return
	}

	if !o.acquire(cancel) {
		slog.Warn("gcode rejected: robot busy", "commands", len(cmds))
		out <- Event{Kind: KindError, Message: "robot busy"}
		return
	}
	defer o.release()

	slog.Info("gcode started", "commands", len(cmds))
	out <- Event{Kind: KindProgress, Message: fmt.Sprintf("running %d commands", len(cmds))}

	if err := runner.Run(opCtx, o.robot, cmds, opts); err != nil {
		slog.Warn("gcode failed", "err", err)
		out <- Event{Kind: KindError, Message: err.Error()}
		return
	}
	x, y := o.robot.Position()
	slog.Info("gcode done", "x", x, "y", y)
	out <- Event{Kind: KindDone, Message: fmt.Sprintf("program complete — position (%.0f, %.0f)", x, y)}
}

// Stop cancels any running operation and immediately disables all motors.
func (o *Orchestrator) Stop() error {
	o.mu.Lock()
	if o.cancelOp != nil {
		o.cancelOp()
	}
	o.mu.Unlock()
	slog.Info("stop: emergency stop requested")
	return o.robot.EmergencyStop()
}

// HoldTension enables passive cable tension on all motors.
func (o *Orchestrator) HoldTension() error {
	slog.Info("hold tension started")
	if err := o.robot.HoldTension(); err != nil {
		slog.Warn("hold tension failed", "err", err)
		return err
	}
	slog.Info("hold tension active")
	return nil
}

// Status returns a one-shot system status snapshot.
func (o *Orchestrator) Status() SystemStatus {
	ev := o.statusEvent()
	return ev.Payload.(SystemStatus)
}
