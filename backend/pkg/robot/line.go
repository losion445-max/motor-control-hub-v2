package robot

import (
	"context"
	"fmt"
	"math"
)

// LineTo moves the camera in a straight line from its current position to
// (x, y) at the given speed (mm/s).
//
// The path is subdivided into waypoints spaced Config.InterpStepMM apart.
// At each waypoint the inverse kinematics are recomputed, so the camera
// follows a straight line in Cartesian space rather than the curved arc that
// a single proportional-cable move would produce.
//
// Returns when the camera has reached (x, y). Cancelling ctx stops all motors.
func (s *System) LineTo(ctx context.Context, x, y, speedMmPerSec float64) error {
	if !s.homed {
		return fmt.Errorf("robot: LineTo called before Home")
	}

	startX, startY := s.posX, s.posY
	dx, dy := x-startX, y-startY
	dist := math.Sqrt(dx*dx + dy*dy)

	if dist < 0.5 {
		return nil
	}

	step := s.cfg.InterpStepMM
	if step <= 0 {
		step = 25
	}

	n := int(math.Ceil(dist / step))
	if n < 1 {
		n = 1
	}

	for i := 1; i <= n; i++ {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		t := float64(i) / float64(n)
		wx := startX + t*dx
		wy := startY + t*dy

		if err := s.MoveTo(ctx, wx, wy, speedMmPerSec); err != nil {
			return fmt.Errorf("robot: LineTo step %d/%d: %w", i, n, err)
		}
	}
	return nil
}
