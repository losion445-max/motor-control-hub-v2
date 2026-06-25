package robot

import (
	"context"
	"fmt"
	"log/slog"
	"math"
	"time"

	"github.com/losion445-max/motor-control-hub-v2/pkg/motion"
	"github.com/losion445-max/motor-control-hub-v2/pkg/t3d"
)


// LineTo moves the camera in a straight Cartesian line to (x1, y1) at
// speedMmPerSec using a continuous closed-loop velocity controller.
//
// # How it works
//
// At each 100 ms tick the controller:
//  1. Reads actual cable lengths from the 4 absolute encoders.
//  2. Computes desired cable lengths at the ideal path position for this tick
//     and one tick ahead (using a trapezoidal velocity profile).
//  3. Derives a feed-forward motor speed = (desiredNext − desiredNow) / dt,
//     which is the exact cable speed required for the commanded Cartesian velocity.
//  4. Adds a proportional correction for accumulated error:
//     corrSpeed = lineCorrGain × (desired − actual) in mm/s cable space.
//  5. Converts the combined cable speed to RPM and writes it to each drive.
//
// Motors run continuously for the duration of the move — no stop-start between
// waypoints — so motion is smooth and the effective speed equals the commanded
// speed minus only the control-loop overhead (~10 %).
//
// The overall speed profile is trapezoidal (accel → cruise → decel) using
// Config.AccelMmPerSec2.  The same ramp is programmed into the drive hardware
// via P-060 / P-061 to smooth out the speed-command steps between ticks.
//
// Sign convention: positive motor RPM = wind in = cable shorter.
// A cable getting longer requires negative RPM (pay-out direction).
func (s *System) LineTo(ctx context.Context, x1, y1, speedMmPerSec float64) error {
	if !s.homed {
		return fmt.Errorf("robot: LineTo called before Home")
	}
	if err := s.checkWorkspace(x1, y1, speedMmPerSec); err != nil {
		return err
	}

	x0, y0 := s.posX, s.posY
	dx, dy := x1-x0, y1-y0
	dist := math.Sqrt(dx*dx + dy*dy)
	if dist < 0.5 {
		s.posX, s.posY = x1, y1
		return nil
	}

	accel := s.cfg.AccelMmPerSec2
	if accel <= 0 {
		accel = DefaultConfig.AccelMmPerSec2
	}
	prof := motion.New(dist, speedMmPerSec, accel)

	// Stop all motors first — kills any active HoldTension before changing torque limits.
	for _, m := range s.motors {
		_ = m.WriteParam(t3d.ParamInternalSpd1, 0)
		_ = m.Disable()
	}

	// Restore full torque capacity. Home and HoldTension leave P-069/P-070 at
	// low values (5-10%) which would prevent normal movement.
	moveTorque := s.cfg.MoveTorquePct
	if moveTorque <= 0 {
		moveTorque = 300
	}
	for i, m := range s.motors {
		if err := m.SetTorqueLimit(moveTorque); err != nil {
			return fmt.Errorf("robot: motor %d restore torque limit: %w", i+1, err)
		}
	}

	// Program hardware accel/decel ramps (P-060 / P-061) so the drive
	// smooths out the speed-command steps between control ticks.
	hwParam := motion.AccelToT3DParam(accel, s.cfg.DrumRadiusMM)
	for i, m := range s.motors {
		if err := m.SetAccelTime(hwParam); err != nil {
			return fmt.Errorf("robot: motor %d set accel: %w", i+1, err)
		}
		if err := m.SetDecelTime(hwParam); err != nil {
			return fmt.Errorf("robot: motor %d set decel: %w", i+1, err)
		}
	}

	// Enable all motors at zero speed; feed-forward ramps them up.
	for i, m := range s.motors {
		if err := m.WriteParam(t3d.ParamInternalSpd1, 0); err != nil {
			return fmt.Errorf("robot: motor %d init: %w", i+1, err)
		}
		if err := m.Enable(); err != nil {
			return fmt.Errorf("robot: motor %d enable: %w", i+1, err)
		}
	}

	circMM := 2 * math.Pi * s.cfg.DrumRadiusMM
	dtSec := s.cfg.LineTickDT.Seconds()
	ppm := pulsesPerMM(s.cfg.DrumRadiusMM, s.cfg.PulsesPerRev)

	// Allow 50 % speed headroom above commanded speed for the correction term.
	capRPM := max(int16(mmPerSecToRPM(speedMmPerSec, s.cfg.DrumRadiusMM))*3/2, 10)

	start := time.Now()
	var settleStart time.Time
	inSettle := false

	for {
		loopStart := time.Now()

		select {
		case <-ctx.Done():
			_ = s.EmergencyStop()
			return ctx.Err()
		default:
		}

		elapsed := time.Since(start).Seconds()
		profileDone := elapsed >= prof.Total

		// Ideal Cartesian position now and one tick ahead (feed-forward derivative).
		posNow := prof.PositionAt(elapsed)
		posNext := prof.PositionAt(elapsed + dtSec)
		txNow := x0 + (posNow/dist)*dx
		tyNow := y0 + (posNow/dist)*dy
		txNext := x0 + (posNext/dist)*dx
		tyNext := y0 + (posNext/dist)*dy

		desiredNow := cableLengths(txNow, tyNow, s.cfg.WidthMM, s.cfg.HeightMM)
		desiredNext := cableLengths(txNext, tyNext, s.cfg.WidthMM, s.cfg.HeightMM)

		// Read actual cable lengths from the absolute encoders.
		actual, err := s.currentCableLengths()
		if err != nil {
			_ = s.EmergencyStop()
			return fmt.Errorf("robot: LineTo read pos: %w", err)
		}

		finalLens := cableLengths(x1, y1, s.cfg.WidthMM, s.cfg.HeightMM)

		// Settle phase: after the velocity profile ends, run pure correction
		// until all cables converge within lineSettleTol pulses of the target.
		if profileDone {
			if !inSettle {
				inSettle = true
				settleStart = time.Now()
			}
			converged := true
			for i := range 4 {
				if int64(math.Abs(finalLens[i]-actual[i])*ppm) > int64(s.cfg.LineSettleTol) {
					converged = false
					break
				}
			}
			select {
			case <-ctx.Done():
				_ = s.EmergencyStop()
				return ctx.Err()
			default:
			}
			if converged {
				break
			}
			if time.Since(settleStart) > s.cfg.LineSettleLim {
				_ = s.EmergencyStop()
				slog.Warn("LineTo: settle timeout", "target_x", x1, "target_y", y1)
				return fmt.Errorf("robot: LineTo settle timeout after %.1fs — target (%.1f, %.1f) not reached",
					s.cfg.LineSettleLim.Seconds(), x1, y1)
			}
		}

		// Compute and apply motor speeds.
		for i, m := range s.motors {
			ffSpeed := (desiredNext[i] - desiredNow[i]) / dtSec
			corrSpeed := s.cfg.LineCorrGain * (desiredNow[i] - actual[i])

			rpmFloat := -(ffSpeed + corrSpeed) / circMM * 60 * float64(s.motorDir(i))
			rpm := int16(math.Round(rpmFloat))
			if rpm > capRPM {
				rpm = capRPM
			} else if rpm < -capRPM {
				rpm = -capRPM
			}

			// During settle the feed-forward is zero and small errors produce sub-1 RPM
			// commands that round to 0 — motors stall and settle never converges.
			// Guarantee at least ±1 RPM whenever there is remaining position error.
			if inSettle && rpm == 0 {
				errPulses := int64(math.Abs(finalLens[i]-actual[i]) * ppm)
				if errPulses > int64(s.cfg.LineSettleTol) {
					if finalLens[i] > actual[i] {
						rpm = 1
					} else {
						rpm = -1
					}
				}
			}

			if err := m.WriteParam(t3d.ParamInternalSpd1, uint16(rpm)); err != nil {
				_ = s.EmergencyStop()
				return fmt.Errorf("robot: motor %d speed: %w", i+1, err)
			}
		}

		// Sleep for the remainder of the control period.
		if rem := s.cfg.LineTickDT - time.Since(loopStart); rem > 0 {
			time.Sleep(rem)
		}
	}

	var disableErr error
	for i, m := range s.motors {
		_ = m.WriteParam(t3d.ParamInternalSpd1, 0)
		if err := m.Disable(); err != nil && disableErr == nil {
			disableErr = fmt.Errorf("robot: motor %d disable after settle: %w", i+1, err)
		}
	}
	if disableErr != nil {
		_ = s.EmergencyStop()
		return disableErr
	}
	time.Sleep(s.cfg.StopSettle)
	s.posX, s.posY = x1, y1
	return nil
}
