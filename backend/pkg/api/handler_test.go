package api_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/losion445-max/motor-control-hub-v2/pkg/api"
	"github.com/losion445-max/motor-control-hub-v2/pkg/robot"
	"github.com/losion445-max/motor-control-hub-v2/pkg/runner"
	"github.com/losion445-max/motor-control-hub-v2/pkg/t3d"
	"github.com/losion445-max/motor-control-hub-v2/pkg/usecase"
)

// ── mock robot (mirrors usecase_test mock) ────────────────────────────────────

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
	block    bool
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
	return [4]robot.MotorState{{ID: 1}, {ID: 2}, {ID: 3}, {ID: 4}}
}
func (m *mockRobot) JogMotor(_ int, _ int) error                    { return nil }
func (m *mockRobot) JogStop(_ int) error                            { return nil }
func (m *mockRobot) ReadMotorStatus(_ int) (*t3d.Status, error)     { return nil, nil }
func (m *mockRobot) WriteMotorParam(_ int, _, _ uint16) error       { return nil }
func (m *mockRobot) ReadMotorParam(_ int, _ uint16) (uint16, error) { return 0, nil }
func (m *mockRobot) SetHome(_, _ float64) error                     { return nil }

// ── test helpers ──────────────────────────────────────────────────────────────

type outMsg struct {
	ID      string            `json:"id"`
	Kind    usecase.EventKind `json:"kind"`
	Message string            `json:"message"`
	Payload json.RawMessage   `json:"payload"`
}

// testServer starts an httptest server backed by orch, returns the WebSocket URL.
func testServer(t *testing.T, r usecase.Robot) (*httptest.Server, *websocket.Conn) {
	t.Helper()
	orch := usecase.New(r)
	h := api.NewHandler(orch, runner.DefaultOpts)
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)

	wsURL := "ws" + strings.TrimPrefix(srv.URL, "http")
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("dial ws: %v", err)
	}
	t.Cleanup(func() { conn.Close() })
	return srv, conn
}

// readUntilTerminal reads messages until a done/error event arrives for id,
// skipping status events that are unrelated to the command. Fails on timeout.
func readUntilTerminal(t *testing.T, conn *websocket.Conn, id string) []outMsg {
	t.Helper()
	conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	var out []outMsg
	for {
		var msg outMsg
		if err := conn.ReadJSON(&msg); err != nil {
			t.Fatalf("read: %v", err)
		}
		if msg.Kind == usecase.KindStatus && msg.ID == "" {
			continue // background status, skip
		}
		out = append(out, msg)
		if msg.ID == id && (msg.Kind == usecase.KindDone || msg.Kind == usecase.KindError) {
			return out
		}
	}
}

func send(t *testing.T, conn *websocket.Conn, v any) {
	t.Helper()
	if err := conn.WriteJSON(v); err != nil {
		t.Fatalf("write: %v", err)
	}
}

// ── command tests ─────────────────────────────────────────────────────────────

func TestHandler_Home(t *testing.T) {
	r := &mockRobot{}
	_, conn := testServer(t, r)

	send(t, conn, map[string]any{"id": "1", "cmd": "home"})
	msgs := readUntilTerminal(t, conn, "1")

	last := msgs[len(msgs)-1]
	if last.Kind != usecase.KindDone {
		t.Errorf("want done, got %q: %s", last.Kind, last.Message)
	}
}

func TestHandler_HomeError(t *testing.T) {
	r := &mockRobot{homeErr: errors.New("cable slack")}
	_, conn := testServer(t, r)

	send(t, conn, map[string]any{"id": "2", "cmd": "home"})
	msgs := readUntilTerminal(t, conn, "2")

	last := msgs[len(msgs)-1]
	if last.Kind != usecase.KindError {
		t.Errorf("want error, got %q", last.Kind)
	}
	if !strings.Contains(last.Message, "cable slack") {
		t.Errorf("message %q should contain 'cable slack'", last.Message)
	}
}

func TestHandler_Move(t *testing.T) {
	r := &mockRobot{}
	_, conn := testServer(t, r)

	send(t, conn, map[string]any{"id": "3", "cmd": "move", "x": 350.0, "y": 600.0, "speed": 50.0})
	msgs := readUntilTerminal(t, conn, "3")

	if last := msgs[len(msgs)-1]; last.Kind != usecase.KindDone {
		t.Errorf("want done, got %q: %s", last.Kind, last.Message)
	}
	x, y := r.Position()
	if x != 350 || y != 600 {
		t.Errorf("position = (%.0f,%.0f), want (350,600)", x, y)
	}
}

func TestHandler_MoveDefaultSpeed(t *testing.T) {
	r := &mockRobot{}
	_, conn := testServer(t, r)

	// speed=0 should fall back to RapidMmPerSec.
	send(t, conn, map[string]any{"id": "4", "cmd": "move", "x": 100.0, "y": 200.0})
	msgs := readUntilTerminal(t, conn, "4")

	if msgs[len(msgs)-1].Kind != usecase.KindDone {
		t.Fatal("move with zero speed should still succeed")
	}
}

func TestHandler_Line(t *testing.T) {
	r := &mockRobot{}
	_, conn := testServer(t, r)

	send(t, conn, map[string]any{"id": "5", "cmd": "line", "x": 1050.0, "y": 1800.0, "speed": 20.0})
	msgs := readUntilTerminal(t, conn, "5")

	if msgs[len(msgs)-1].Kind != usecase.KindDone {
		t.Errorf("want done, got %q", msgs[len(msgs)-1].Kind)
	}
}

func TestHandler_Gcode(t *testing.T) {
	r := &mockRobot{homedVal: true, x: 700, y: 1200}
	_, conn := testServer(t, r)

	send(t, conn, map[string]any{
		"id":      "6",
		"cmd":     "gcode",
		"program": "G0 X350 Y600\nG0 X700 Y1200",
	})
	msgs := readUntilTerminal(t, conn, "6")

	if msgs[len(msgs)-1].Kind != usecase.KindDone {
		t.Errorf("want done, got %q: %s", msgs[len(msgs)-1].Kind, msgs[len(msgs)-1].Message)
	}
}

func TestHandler_Stop(t *testing.T) {
	r := &mockRobot{}
	_, conn := testServer(t, r)

	send(t, conn, map[string]any{"id": "7", "cmd": "stop"})
	msgs := readUntilTerminal(t, conn, "7")

	if msgs[len(msgs)-1].Kind != usecase.KindDone {
		t.Errorf("want done, got %q", msgs[len(msgs)-1].Kind)
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
		t.Error("EmergencyStop should have been called")
	}
}

func TestHandler_HoldTension(t *testing.T) {
	r := &mockRobot{}
	_, conn := testServer(t, r)

	send(t, conn, map[string]any{"id": "8", "cmd": "hold_tension"})
	msgs := readUntilTerminal(t, conn, "8")

	if msgs[len(msgs)-1].Kind != usecase.KindDone {
		t.Errorf("want done, got %q", msgs[len(msgs)-1].Kind)
	}
}

func TestHandler_Status(t *testing.T) {
	r := &mockRobot{homedVal: true, x: 700, y: 1200}
	_, conn := testServer(t, r)

	send(t, conn, map[string]any{"id": "9", "cmd": "status"})
	conn.SetReadDeadline(time.Now().Add(3 * time.Second))

	// Read until we see a status event for our id.
	for {
		var msg outMsg
		if err := conn.ReadJSON(&msg); err != nil {
			t.Fatalf("read: %v", err)
		}
		if msg.ID == "9" && msg.Kind == usecase.KindStatus {
			// Decode the payload and verify fields.
			var s usecase.SystemStatus
			if err := json.Unmarshal(msg.Payload, &s); err != nil {
				t.Fatalf("decode payload: %v", err)
			}
			if !s.Homed {
				t.Error("status.Homed should be true")
			}
			if s.X != 700 || s.Y != 1200 {
				t.Errorf("status position = (%.0f,%.0f), want (700,1200)", s.X, s.Y)
			}
			return
		}
	}
}

func TestHandler_UnknownCommand(t *testing.T) {
	r := &mockRobot{}
	_, conn := testServer(t, r)

	send(t, conn, map[string]any{"id": "10", "cmd": "fly"})
	msgs := readUntilTerminal(t, conn, "10")

	if msgs[len(msgs)-1].Kind != usecase.KindError {
		t.Errorf("want error for unknown command, got %q", msgs[len(msgs)-1].Kind)
	}
}

func TestHandler_BadJSON(t *testing.T) {
	r := &mockRobot{}
	_, conn := testServer(t, r)

	// Raw text message — not valid JSON.
	conn.WriteMessage(websocket.TextMessage, []byte("{broken json"))
	conn.SetReadDeadline(time.Now().Add(2 * time.Second))

	for {
		var msg outMsg
		if err := conn.ReadJSON(&msg); err != nil {
			t.Fatalf("read: %v", err)
		}
		if msg.Kind == usecase.KindError {
			if !strings.Contains(msg.Message, "bad json") {
				t.Errorf("expected 'bad json' in error, got %q", msg.Message)
			}
			return
		}
	}
}

func TestHandler_BusyRejection(t *testing.T) {
	r := &mockRobot{block: true}
	_, conn := testServer(t, r)

	// First command blocks.
	send(t, conn, map[string]any{"id": "11", "cmd": "home"})
	time.Sleep(30 * time.Millisecond)

	// Second command should be rejected immediately.
	send(t, conn, map[string]any{"id": "12", "cmd": "move", "x": 100.0, "y": 100.0, "speed": 50.0})

	conn.SetReadDeadline(time.Now().Add(3 * time.Second))
	for {
		var msg outMsg
		if err := conn.ReadJSON(&msg); err != nil {
			t.Fatalf("read: %v", err)
		}
		if msg.ID == "12" && msg.Kind == usecase.KindError {
			if !strings.Contains(msg.Message, "busy") {
				t.Errorf("expected 'busy' in error, got %q", msg.Message)
			}
			return
		}
	}
}

func TestHandler_MultipleClients(t *testing.T) {
	r := &mockRobot{}
	orch := usecase.New(r)
	h := api.NewHandler(orch, runner.DefaultOpts)
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)

	wsURL := "ws" + strings.TrimPrefix(srv.URL, "http")
	dial := func() *websocket.Conn {
		conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
		if err != nil {
			t.Fatalf("dial: %v", err)
		}
		return conn
	}

	c1 := dial()
	c2 := dial()
	t.Cleanup(func() { c1.Close(); c2.Close() })

	// Both clients independently run commands.
	send(t, c1, map[string]any{"id": "a", "cmd": "home"})
	send(t, c2, map[string]any{"id": "b", "cmd": "stop"})

	m1 := readUntilTerminal(t, c1, "a")
	m2 := readUntilTerminal(t, c2, "b")

	if m1[len(m1)-1].Kind != usecase.KindDone {
		t.Errorf("client1 home: want done, got %q", m1[len(m1)-1].Kind)
	}
	if m2[len(m2)-1].Kind != usecase.KindDone {
		t.Errorf("client2 stop: want done, got %q", m2[len(m2)-1].Kind)
	}
}

// ── NewServer / Shutdown / health endpoint ────────────────────────────────────

func TestNewServer_HealthEndpoint(t *testing.T) {
	h := api.NewHandler(usecase.New(&mockRobot{}), runner.Opts{})
	srv := api.NewServer(":0", h)

	ts := httptest.NewServer(srv.Handler)
	t.Cleanup(ts.Close)

	resp, err := ts.Client().Get(ts.URL + "/health")
	if err != nil {
		t.Fatalf("GET /health: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Errorf("/health status = %d, want 200", resp.StatusCode)
	}
}

func TestShutdown(t *testing.T) {
	h := api.NewHandler(usecase.New(&mockRobot{}), runner.Opts{})
	srv := api.NewServer(":0", h)
	ts := httptest.NewServer(srv.Handler)
	t.Cleanup(ts.Close)

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	// Shutdown on an httptest server will return an error ("Server closed") but
	// the call itself must not panic or block.
	_ = api.Shutdown(ctx, srv)
}
