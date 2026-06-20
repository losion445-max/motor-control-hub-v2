package runner

import (
	"context"
	"errors"
	"fmt"
	"math"
	"testing"

	"github.com/losion445-max/motor-control-hub-v2/pkg/gcode"
)

// ── mock ─────────────────────────────────────────────────────────────────────

type mockSystem struct {
	x, y    float64
	moveErr error
	homeErr error
	calls   []string // log: "MoveTo x y speed", "LineTo x y speed", "Home", "EmergencyStop"
}

func (m *mockSystem) MoveTo(_ context.Context, x, y, speed float64) error {
	m.calls = append(m.calls, fmt.Sprintf("MoveTo %.2f %.2f %.2f", x, y, speed))
	m.x, m.y = x, y
	return m.moveErr
}
func (m *mockSystem) LineTo(_ context.Context, x, y, speed float64) error {
	m.calls = append(m.calls, fmt.Sprintf("LineTo %.2f %.2f %.2f", x, y, speed))
	m.x, m.y = x, y
	return m.moveErr
}
func (m *mockSystem) Home(_ context.Context) error {
	m.calls = append(m.calls, "Home")
	return m.homeErr
}
func (m *mockSystem) EmergencyStop() error {
	m.calls = append(m.calls, "EmergencyStop")
	return nil
}
func (m *mockSystem) Position() (float64, float64) { return m.x, m.y }

// ── helpers ───────────────────────────────────────────────────────────────────

func mustParse(t *testing.T, src string) []gcode.Cmd {
	t.Helper()
	cmds, err := gcode.Parse(src)
	if err != nil {
		t.Fatalf("gcode.Parse: %v", err)
	}
	return cmds
}

// ── tests ─────────────────────────────────────────────────────────────────────

func TestRun_G28(t *testing.T) {
	sys := &mockSystem{}
	cmds := mustParse(t, "G28")
	if err := Run(context.Background(), sys, cmds, DefaultOpts); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(sys.calls) != 1 || sys.calls[0] != "Home" {
		t.Errorf("calls = %v, want [Home]", sys.calls)
	}
}

func TestRun_G0UsesRapidSpeed(t *testing.T) {
	sys := &mockSystem{}
	cmds := mustParse(t, "G0 X100 Y200")
	if err := Run(context.Background(), sys, cmds, DefaultOpts); err != nil {
		t.Fatalf("Run: %v", err)
	}
	want := fmt.Sprintf("MoveTo %.2f %.2f %.2f", 100.0, 200.0, DefaultOpts.RapidMmPerSec)
	if len(sys.calls) != 1 || sys.calls[0] != want {
		t.Errorf("calls = %v, want [%s]", sys.calls, want)
	}
}

func TestRun_G1FeedRate(t *testing.T) {
	sys := &mockSystem{}
	// G1 F600 → feed = 600/60 = 10 mm/s; then G1 X100 Y200 uses that rate.
	cmds := mustParse(t, "G1 F600\nX100 Y200")
	if err := Run(context.Background(), sys, cmds, DefaultOpts); err != nil {
		t.Fatalf("Run: %v", err)
	}
	want := fmt.Sprintf("LineTo %.2f %.2f %.2f", 100.0, 200.0, 10.0)
	// The F-only cmd produces no motion; the second cmd is the LineTo.
	found := false
	for _, c := range sys.calls {
		if c == want {
			found = true
		}
	}
	if !found {
		t.Errorf("calls = %v, want entry %q", sys.calls, want)
	}
}

func TestRun_G1InlineF(t *testing.T) {
	// Feed specified on the same line as position.
	sys := &mockSystem{}
	cmds := mustParse(t, "G1 X100 Y200 F1200")
	if err := Run(context.Background(), sys, cmds, DefaultOpts); err != nil {
		t.Fatalf("Run: %v", err)
	}
	want := fmt.Sprintf("LineTo %.2f %.2f %.2f", 100.0, 200.0, 20.0) // 1200/60=20
	if len(sys.calls) != 1 || sys.calls[0] != want {
		t.Errorf("calls = %v, want [%s]", sys.calls, want)
	}
}

func TestRun_ModalFeedRate(t *testing.T) {
	sys := &mockSystem{}
	// Two G1 moves: first sets F=600, second inherits.
	cmds := mustParse(t, "G1 X100 Y200 F600\nX300 Y400")
	if err := Run(context.Background(), sys, cmds, DefaultOpts); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(sys.calls) != 2 {
		t.Fatalf("calls = %v, want 2", sys.calls)
	}
	// Both should use 600/60=10 mm/s
	want0 := fmt.Sprintf("LineTo %.2f %.2f %.2f", 100.0, 200.0, 10.0)
	want1 := fmt.Sprintf("LineTo %.2f %.2f %.2f", 300.0, 400.0, 10.0)
	if sys.calls[0] != want0 {
		t.Errorf("calls[0] = %q, want %q", sys.calls[0], want0)
	}
	if sys.calls[1] != want1 {
		t.Errorf("calls[1] = %q, want %q", sys.calls[1], want1)
	}
}

func TestRun_PropagatesMoveError(t *testing.T) {
	sys := &mockSystem{moveErr: errors.New("motor fault")}
	cmds := mustParse(t, "G0 X100 Y200")
	err := Run(context.Background(), sys, cmds, DefaultOpts)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !errors.Is(err, sys.moveErr) {
		t.Errorf("error does not wrap moveErr: %v", err)
	}
}

func TestRun_CancelledContext(t *testing.T) {
	sys := &mockSystem{}
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	cmds := mustParse(t, "G0 X100 Y200\nG0 X200 Y300")
	err := Run(ctx, sys, cmds, DefaultOpts)
	if err == nil {
		t.Fatal("expected context error, got nil")
	}
	if !errors.Is(err, context.Canceled) {
		t.Errorf("error = %v, want context.Canceled", err)
	}
}

func TestRun_DefaultOptsZeroFallback(t *testing.T) {
	sys := &mockSystem{}
	cmds := mustParse(t, "G0 X100 Y200")
	// Zero Opts should fall back to DefaultOpts values.
	if err := Run(context.Background(), sys, cmds, Opts{}); err != nil {
		t.Fatalf("Run: %v", err)
	}
	want := fmt.Sprintf("MoveTo %.2f %.2f %.2f", 100.0, 200.0, DefaultOpts.RapidMmPerSec)
	if len(sys.calls) != 1 || sys.calls[0] != want {
		t.Errorf("calls = %v, want [%s]", sys.calls, want)
	}
}

// ── resolveTarget ─────────────────────────────────────────────────────────────

func TestResolveTarget(t *testing.T) {
	nan := math.NaN()

	t.Run("both specified", func(t *testing.T) {
		cmd := gcode.Cmd{X: 100, Y: 200}
		x, y := resolveTarget(cmd, 0, 0)
		if x != 100 || y != 200 {
			t.Errorf("got %v %v, want 100 200", x, y)
		}
	})
	t.Run("NaN X keeps current X", func(t *testing.T) {
		cmd := gcode.Cmd{X: nan, Y: 200}
		x, y := resolveTarget(cmd, 50, 0)
		if x != 50 {
			t.Errorf("NaN X: got x=%v, want 50 (current)", x)
		}
		if y != 200 {
			t.Errorf("NaN X: got y=%v, want 200", y)
		}
	})
	t.Run("NaN Y keeps current Y", func(t *testing.T) {
		cmd := gcode.Cmd{X: 100, Y: nan}
		x, y := resolveTarget(cmd, 0, 75)
		if x != 100 {
			t.Errorf("NaN Y: got x=%v, want 100", x)
		}
		if y != 75 {
			t.Errorf("NaN Y: got y=%v, want 75 (current)", y)
		}
	})
	t.Run("both NaN keeps current position", func(t *testing.T) {
		cmd := gcode.Cmd{X: nan, Y: nan}
		x, y := resolveTarget(cmd, 33, 44)
		if x != 33 || y != 44 {
			t.Errorf("both NaN: got %v %v, want 33 44", x, y)
		}
	})
}
