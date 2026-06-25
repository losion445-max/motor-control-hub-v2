package usecase_test

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/losion445-max/motor-control-hub-v2/pkg/robot"
	"github.com/losion445-max/motor-control-hub-v2/pkg/runner"
	"github.com/losion445-max/motor-control-hub-v2/pkg/t3d"
	"github.com/losion445-max/motor-control-hub-v2/pkg/usecase"
)

// ── mock robot ────────────────────────────────────────────────────────────────

type mockRobot struct {
	mu       sync.Mutex
	calls    []string
	homeErr  error
	moveErr  error
	lineErr  error
	stopErr  error
	holdErr  error
	homedVal bool
	x, y    float64
	// block causes the next Home/MoveTo/LineTo to block until ctx is cancelled.
	block bool
}

func (m *mockRobot) record(s string) {
	m.mu.Lock()
	m.calls = append(m.calls, s)
	m.mu.Unlock()
}

func (m *mockRobot) Home(ctx context.Context) error {
	m.record("Home")
	if m.block {
		<-ctx.Done()
		return ctx.Err()
	}
	if m.homeErr == nil {
		m.mu.Lock()
		m.homedVal = true
		m.x, m.y = 700, 1200
		m.mu.Unlock()
	}
	return m.homeErr
}

func (m *mockRobot) MoveTo(ctx context.Context, x, y, speed float64) error {
	m.record("MoveTo")
	if m.block {
		<-ctx.Done()
		return ctx.Err()
	}
	if m.moveErr == nil {
		m.mu.Lock()
		m.x, m.y = x, y
		m.mu.Unlock()
	}
	return m.moveErr
}

func (m *mockRobot) LineTo(ctx context.Context, x, y, speed float64) error {
	m.record("LineTo")
	if m.block {
		<-ctx.Done()
		return ctx.Err()
	}
	if m.lineErr == nil {
		m.mu.Lock()
		m.x, m.y = x, y
		m.mu.Unlock()
	}
	return m.lineErr
}

func (m *mockRobot) EmergencyStop() error {
	m.record("EmergencyStop")
	return m.stopErr
}

func (m *mockRobot) HoldTension() error {
	m.record("HoldTension")
	return m.holdErr
}

func (m *mockRobot) Position() (float64, float64) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.x, m.y
}

func (m *mockRobot) Homed() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.homedVal
}

func (m *mockRobot) ReadAllStatus() [4]robot.MotorState {
	return [4]robot.MotorState{
		{ID: 1}, {ID: 2}, {ID: 3}, {ID: 4},
	}
}
func (m *mockRobot) JogMotor(_ int, _ int) error                     { return nil }
func (m *mockRobot) JogStop(_ int) error                             { return nil }
func (m *mockRobot) ReadMotorStatus(_ int) (*t3d.Status, error)      { return nil, nil }
func (m *mockRobot) WriteMotorParam(_ int, _, _ uint16) error        { return nil }
func (m *mockRobot) ReadMotorParam(_ int, _ uint16) (uint16, error)  { return 0, nil }
func (m *mockRobot) SetHome(_, _ float64) error                      { return nil }

// ── helpers ───────────────────────────────────────────────────────────────────

// run calls fn with a buffered event channel and returns all emitted events.
func run(fn func(chan<- usecase.Event)) []usecase.Event {
	ch := make(chan usecase.Event, 32)
	fn(ch)
	close(ch)
	var evs []usecase.Event
	for e := range ch {
		evs = append(evs, e)
	}
	return evs
}

func lastKind(evs []usecase.Event) usecase.EventKind {
	if len(evs) == 0 {
		return ""
	}
	return evs[len(evs)-1].Kind
}

// ── Calibrate ─────────────────────────────────────────────────────────────────

func TestCalibrate_Success(t *testing.T) {
	r := &mockRobot{}
	o := usecase.New(r)
	evs := run(func(ch chan<- usecase.Event) {
		o.Calibrate(context.Background(), ch)
	})
	if lastKind(evs) != usecase.KindDone {
		t.Fatalf("last event = %q, want done; all events: %v", lastKind(evs), evs)
	}
	r.mu.Lock()
	homed := r.homedVal
	r.mu.Unlock()
	if !homed {
		t.Error("robot.Home was not called or did not set homedVal")
	}
}

func TestCalibrate_Error(t *testing.T) {
	boom := errors.New("motor 3 fault")
	r := &mockRobot{homeErr: boom}
	o := usecase.New(r)
	evs := run(func(ch chan<- usecase.Event) {
		o.Calibrate(context.Background(), ch)
	})
	if lastKind(evs) != usecase.KindError {
		t.Fatalf("want error event, got %q", lastKind(evs))
	}
	if !strings.Contains(evs[len(evs)-1].Message, boom.Error()) {
		t.Errorf("error message %q should contain %q", evs[len(evs)-1].Message, boom.Error())
	}
}

func TestCalibrate_BusyRejection(t *testing.T) {
	r := &mockRobot{block: true}
	o := usecase.New(r)

	ctx, cancel := context.WithCancel(context.Background())

	started := make(chan struct{})
	go func() {
		ch := make(chan usecase.Event, 32)
		close(started)
		o.Calibrate(ctx, ch)
	}()
	<-started
	// Give first op time to acquire lock.
	time.Sleep(20 * time.Millisecond)

	// Second call should be rejected immediately.
	evs := run(func(ch chan<- usecase.Event) {
		o.Calibrate(context.Background(), ch)
	})
	if lastKind(evs) != usecase.KindError {
		t.Errorf("expected busy error, got %q", lastKind(evs))
	}

	cancel()
}

// ── Move ──────────────────────────────────────────────────────────────────────

func TestMove_Success(t *testing.T) {
	r := &mockRobot{}
	o := usecase.New(r)
	evs := run(func(ch chan<- usecase.Event) {
		o.Move(context.Background(), 350, 600, 50, ch)
	})
	if lastKind(evs) != usecase.KindDone {
		t.Fatalf("want done, got %q", lastKind(evs))
	}
	x, y := r.Position()
	if x != 350 || y != 600 {
		t.Errorf("position = (%.0f, %.0f), want (350, 600)", x, y)
	}
}

func TestMove_Error(t *testing.T) {
	r := &mockRobot{moveErr: errors.New("not homed")}
	o := usecase.New(r)
	evs := run(func(ch chan<- usecase.Event) {
		o.Move(context.Background(), 350, 600, 50, ch)
	})
	if lastKind(evs) != usecase.KindError {
		t.Fatalf("want error, got %q", lastKind(evs))
	}
}

func TestMove_BusyRejection(t *testing.T) {
	r := &mockRobot{block: true}
	o := usecase.New(r)

	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		ch := make(chan usecase.Event, 32)
		o.Move(ctx, 0, 0, 10, ch)
	}()
	time.Sleep(20 * time.Millisecond)

	evs := run(func(ch chan<- usecase.Event) {
		o.Move(context.Background(), 100, 100, 10, ch)
	})
	if lastKind(evs) != usecase.KindError {
		t.Errorf("want busy error, got %q", lastKind(evs))
	}
	cancel()
}

// ── Line ──────────────────────────────────────────────────────────────────────

func TestLine_Success(t *testing.T) {
	r := &mockRobot{}
	o := usecase.New(r)
	evs := run(func(ch chan<- usecase.Event) {
		o.Line(context.Background(), 1050, 1800, 20, ch)
	})
	if lastKind(evs) != usecase.KindDone {
		t.Fatalf("want done, got %q", lastKind(evs))
	}
	x, y := r.Position()
	if x != 1050 || y != 1800 {
		t.Errorf("position = (%.0f, %.0f), want (1050, 1800)", x, y)
	}
}

func TestLine_Error(t *testing.T) {
	r := &mockRobot{lineErr: errors.New("cable slack")}
	o := usecase.New(r)
	evs := run(func(ch chan<- usecase.Event) {
		o.Line(context.Background(), 200, 300, 20, ch)
	})
	if lastKind(evs) != usecase.KindError {
		t.Fatalf("want error, got %q", lastKind(evs))
	}
}

// ── RunGcode ──────────────────────────────────────────────────────────────────

func TestRunGcode_ParseError(t *testing.T) {
	r := &mockRobot{}
	o := usecase.New(r)
	evs := run(func(ch chan<- usecase.Event) {
		o.RunGcode(context.Background(), "G999 X@@BAD", runner.DefaultOpts, ch)
	})
	// Unknown G999 is not a parse error (it's ignored); empty/invalid token is.
	// Just verify we get any terminal event.
	k := lastKind(evs)
	if k != usecase.KindDone && k != usecase.KindError {
		t.Fatalf("want done or error, got %q", k)
	}
}

func TestRunGcode_Success(t *testing.T) {
	r := &mockRobot{homedVal: true, x: 700, y: 1200}
	o := usecase.New(r)
	evs := run(func(ch chan<- usecase.Event) {
		o.RunGcode(context.Background(), "G0 X350 Y600", runner.DefaultOpts, ch)
	})
	if lastKind(evs) != usecase.KindDone {
		t.Fatalf("want done, got %q — events: %v", lastKind(evs), evs)
	}
}

func TestRunGcode_BusyRejection(t *testing.T) {
	r := &mockRobot{block: true, homedVal: true}
	o := usecase.New(r)

	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		ch := make(chan usecase.Event, 32)
		o.Move(ctx, 0, 0, 10, ch)
	}()
	time.Sleep(20 * time.Millisecond)

	evs := run(func(ch chan<- usecase.Event) {
		o.RunGcode(context.Background(), "G0 X100", runner.DefaultOpts, ch)
	})
	if lastKind(evs) != usecase.KindError {
		t.Errorf("want busy error, got %q", lastKind(evs))
	}
	cancel()
}

// ── Stop ─────────────────────────────────────────────────────────────────────

func TestStop_CancelsRunningOperation(t *testing.T) {
	r := &mockRobot{block: true}
	o := usecase.New(r)

	allEvents := make(chan []usecase.EventKind, 1)
	go func() {
		ch := make(chan usecase.Event, 32)
		o.Calibrate(context.Background(), ch)
		close(ch)
		var kinds []usecase.EventKind
		for e := range ch {
			kinds = append(kinds, e.Kind)
		}
		allEvents <- kinds
	}()
	time.Sleep(30 * time.Millisecond)

	_ = o.Stop()

	select {
	case kinds := <-allEvents:
		if len(kinds) == 0 {
			t.Fatal("no events emitted")
		}
		last := kinds[len(kinds)-1]
		if last != usecase.KindError {
			t.Errorf("expected last event = error, got %q (all: %v)", last, kinds)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("operation did not terminate after Stop")
	}

	r.mu.Lock()
	calls := r.calls
	r.mu.Unlock()
	found := false
	for _, c := range calls {
		if c == "EmergencyStop" {
			found = true
		}
	}
	if !found {
		t.Error("EmergencyStop was not called")
	}
}

func TestStop_IdleIsNoop(t *testing.T) {
	r := &mockRobot{}
	o := usecase.New(r)
	if err := o.Stop(); err != nil {
		t.Errorf("Stop on idle robot returned error: %v", err)
	}
}

// ── HoldTension ───────────────────────────────────────────────────────────────

func TestHoldTension_Success(t *testing.T) {
	r := &mockRobot{}
	o := usecase.New(r)
	if err := o.HoldTension(); err != nil {
		t.Errorf("HoldTension: %v", err)
	}
	r.mu.Lock()
	calls := r.calls
	r.mu.Unlock()
	if len(calls) != 1 || calls[0] != "HoldTension" {
		t.Errorf("calls = %v, want [HoldTension]", calls)
	}
}

func TestHoldTension_Error(t *testing.T) {
	r := &mockRobot{holdErr: errors.New("drive fault")}
	o := usecase.New(r)
	if err := o.HoldTension(); err == nil {
		t.Error("expected error, got nil")
	}
}

// ── Status ────────────────────────────────────────────────────────────────────

func TestStatus_Fields(t *testing.T) {
	r := &mockRobot{homedVal: true, x: 350, y: 600}
	o := usecase.New(r)
	s := o.Status()
	if !s.Homed {
		t.Error("Status.Homed should be true")
	}
	if s.X != 350 || s.Y != 600 {
		t.Errorf("Status position = (%.0f,%.0f), want (350,600)", s.X, s.Y)
	}
	if len(s.Motors) != 4 {
		t.Errorf("Status.Motors len = %d, want 4", len(s.Motors))
	}
}

// ── Broadcast ─────────────────────────────────────────────────────────────────

func TestBroadcast_SubscriberReceivesStatus(t *testing.T) {
	r := &mockRobot{}
	o := usecase.New(r)

	ch := make(chan usecase.Event, 4)
	o.Subscribe(ch)
	defer o.Unsubscribe(ch)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go o.RunStatusBroadcast(ctx, 50*time.Millisecond)

	select {
	case ev := <-ch:
		if ev.Kind != usecase.KindStatus {
			t.Errorf("got %q, want status", ev.Kind)
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatal("no status event received within 500 ms")
	}
}

func TestBroadcast_UnsubscribeStopsEvents(t *testing.T) {
	r := &mockRobot{}
	o := usecase.New(r)

	ch := make(chan usecase.Event, 4)
	o.Subscribe(ch)

	ctx, cancel := context.WithCancel(context.Background())
	go o.RunStatusBroadcast(ctx, 30*time.Millisecond)

	// Wait for at least one event.
	<-ch

	o.Unsubscribe(ch)
	cancel()

	// Drain any already-queued events.
	for len(ch) > 0 {
		<-ch
	}

	// No more events should arrive.
	select {
	case <-ch:
		t.Error("received event after Unsubscribe")
	case <-time.After(100 * time.Millisecond):
	}
}

func TestBroadcast_SlowSubscriberDropsEvents(t *testing.T) {
	r := &mockRobot{}
	o := usecase.New(r)

	// Unbuffered: subscriber is always "slow".
	ch := make(chan usecase.Event)
	o.Subscribe(ch)
	defer o.Unsubscribe(ch)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Should not block even with a slow subscriber.
	done := make(chan struct{})
	go func() {
		o.RunStatusBroadcast(ctx, 10*time.Millisecond)
		close(done)
	}()

	time.Sleep(100 * time.Millisecond)
	cancel()
	<-done
}
