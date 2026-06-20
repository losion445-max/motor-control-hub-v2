// Package runner executes a parsed G-code program on a cable robot.
// It imports pkg/gcode but not pkg/robot — it accepts any value that satisfies
// the System interface, which *robot.System happens to implement. This keeps
// pkg/robot and pkg/gcode independent of each other.
//
// Usage:
//
//	cmds, err := gcode.Parse(programText)
//	err = runner.Run(ctx, sys, cmds, runner.DefaultOpts)
package runner

import (
	"context"
	"fmt"
	"math"

	"github.com/losion445-max/motor-control-hub-v2/pkg/gcode"
)

// System is the interface that runner requires. *robot.System satisfies it.
type System interface {
	MoveTo(ctx context.Context, x, y, speedMmPerSec float64) error
	LineTo(ctx context.Context, x, y, speedMmPerSec float64) error
	Home(ctx context.Context) error
	EmergencyStop() error
	Position() (float64, float64)
}

// Opts controls execution behaviour.
type Opts struct {
	// RapidMmPerSec is the speed used for G0 (rapid) moves.
	// Default: 200 mm/s.
	RapidMmPerSec float64

	// DefaultFeedMmPerSec is the feed rate used before any F word appears
	// in the program. Default: 20 mm/s.
	DefaultFeedMmPerSec float64
}

// DefaultOpts are sensible defaults for the 1400×2400 mm frame.
var DefaultOpts = Opts{
	RapidMmPerSec:       200,
	DefaultFeedMmPerSec: 20,
}

// Run executes the G-code command list on sys.
// The system must already be homed. Feed rate is modal — the last F value
// seen in the program persists for subsequent G1 moves until changed.
// Cancelling ctx performs an emergency stop.
func Run(ctx context.Context, sys System, cmds []gcode.Cmd, opts Opts) error {
	if opts.RapidMmPerSec <= 0 {
		opts.RapidMmPerSec = DefaultOpts.RapidMmPerSec
	}
	feedMmPerSec := opts.DefaultFeedMmPerSec
	if feedMmPerSec <= 0 {
		feedMmPerSec = DefaultOpts.DefaultFeedMmPerSec
	}

	for i, cmd := range cmds {
		select {
		case <-ctx.Done():
			_ = sys.EmergencyStop()
			return ctx.Err()
		default:
		}

		// Update modal feed rate if this command carries an F word.
		if cmd.F > 0 {
			feedMmPerSec = cmd.F / 60 // mm/min → mm/s
		}

		curX, curY := sys.Position()
		x, y := resolveTarget(cmd, curX, curY)

		switch cmd.Motion {
		case gcode.Rapid:
			if err := sys.MoveTo(ctx, x, y, opts.RapidMmPerSec); err != nil {
				return fmt.Errorf("runner: command %d G0: %w", i, err)
			}

		case gcode.Linear:
			if err := sys.LineTo(ctx, x, y, feedMmPerSec); err != nil {
				return fmt.Errorf("runner: command %d G1: %w", i, err)
			}

		case gcode.Home:
			if err := sys.Home(ctx); err != nil {
				return fmt.Errorf("runner: command %d G28: %w", i, err)
			}
		}
	}
	return nil
}

// resolveTarget returns the absolute target position for a command, falling
// back to the current position for any axis that was not specified.
func resolveTarget(cmd gcode.Cmd, curX, curY float64) (x, y float64) {
	x, y = curX, curY
	if !math.IsNaN(cmd.X) {
		x = cmd.X
	}
	if !math.IsNaN(cmd.Y) {
		y = cmd.Y
	}
	return
}
