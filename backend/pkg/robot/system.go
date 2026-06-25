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


// driveMotor is the set of *t3d.Motor methods used by System.
// Defined as an interface so System can be tested without a real RS-485 port.
type driveMotor interface {
	Enable() error
	Disable() error
	WriteParam(addr, value uint16) error
	ReadParam(addr uint16) (uint16, error)
	ReadAbsPosition() (int32, error)
	ReadAbsPosAndFault() (pos int32, fault uint16, err error)
	ReadTorquePct() (int16, error)
	ReadFault() (uint16, error)
	ReadStatus() (*t3d.Status, error)
	ReadMotionState() (pos int32, torque int16, fault uint16, err error)
	SetAccelTime(msPerKRPM int) error
	SetDecelTime(msPerKRPM int) error
	SetSpeed(rpm int) error
	SetTorqueLimit(pct int) error
}

// MotorState is a snapshot returned by ReadAllStatus.
type MotorState struct {
	ID     int // slave ID (1..4)
	Status *t3d.Status
	Err    error
}

// System drives a 4-cable parallel robot.
// Create with NewSystem, call Connect before any motion, Close when done.
type System struct {
	bus    *t3d.Bus
	motors [4]driveMotor // index 0=M1, 1=M2, 2=M3, 3=M4; *t3d.Motor in production
	cfg    Config

	// set by Home()
	homed    bool
	homePos  [4]int32  // encoder readings at home
	homeLenMM float64  // cable length at home (mm); same for all 4 at centre
	posX, posY float64 // last known position (mm from top-left)
}

// NewSystem creates a System for the given RS-485 port.
// Call Connect before any motor operations.
func NewSystem(port string, baud int, cfg Config) *System {
	bus := t3d.NewBus(port, baud)
	s := &System{bus: bus, cfg: cfg}
	for i := range 4 {
		s.motors[i] = t3d.NewMotor(bus, byte(i+1)) // slave IDs 1..4
	}
	return s
}

// Connect opens the serial port.
func (s *System) Connect() error { return s.bus.Connect() }

// Close releases the serial port.
func (s *System) Close() error { return s.bus.Close() }

// ── Homing ────────────────────────────────────────────────────────────────────

// Home tensions all 4 cables until each reaches the torque threshold, then
// declares the camera to be at the centre of the workspace (W/2, H/2).
//
// The camera must be placed physically near the centre before calling Home.
// Each motor stops independently as soon as its cable becomes taut.
func (s *System) Home(ctx context.Context) error {
	hwLimit := s.cfg.HomingTorquePct + 5
	slog.Info("home: start",
		"rpm", s.cfg.HomingRPM,
		"torque_threshold_pct", s.cfg.HomingTorquePct,
		"hw_torque_limit_pct", hwLimit,
	)

	// Safety cap during homing.
	for i, m := range s.motors {
		if err := m.SetTorqueLimit(hwLimit); err != nil {
			return fmt.Errorf("home: motor %d set torque limit: %w", i+1, err)
		}
	}

	// Start all motors winding in slowly (direction-corrected per drum orientation).
	for i, m := range s.motors {
		rpm := s.cfg.HomingRPM * s.motorDir(i)
		if err := m.WriteParam(t3d.ParamInternalSpd1, uint16(int16(rpm))); err != nil {
			return fmt.Errorf("home: motor %d set speed: %w", i+1, err)
		}
		if err := m.Enable(); err != nil {
			return fmt.Errorf("home: motor %d enable: %w", i+1, err)
		}
	}
	slog.Info("home: all motors enabled, winding in")

	done := [4]bool{}
	pollN := 0
	for {
		select {
		case <-ctx.Done():
			s.EmergencyStop()
			return ctx.Err()
		default:
		}

		allDone := true
		for i, m := range s.motors {
			if done[i] {
				continue
			}
			allDone = false

			torque, err := m.ReadTorquePct()
			if err != nil {
				s.EmergencyStop()
				return fmt.Errorf("home: motor %d read torque: %w", i+1, err)
			}

			// Log every ~500ms (every 50 polls at 10ms interval) so we can
			// see torque climbing toward the threshold without flooding the log.
			if pollN%50 == 0 {
				slog.Debug("home: poll", "motor", i+1, "torque_pct", torque)
			}

			absT := torque
			if absT < 0 {
				absT = -absT
			}
			if int(absT) >= s.cfg.HomingTorquePct {
				if err := m.Disable(); err != nil {
					return fmt.Errorf("home: motor %d disable: %w", i+1, err)
				}
				// Let motor coast to rest before reading encoder; respect ctx cancellation.
				select {
				case <-time.After(30 * time.Millisecond):
				case <-ctx.Done():
					s.EmergencyStop()
					return ctx.Err()
				}
				pos, err := m.ReadAbsPosition()
				if err != nil {
					return fmt.Errorf("home: motor %d read pos: %w", i+1, err)
				}
				s.homePos[i] = pos
				done[i] = true
				slog.Info("home: motor taut",
					"motor", i+1,
					"torque_pct", torque,
					"encoder_pos", pos,
				)
			}
		}
		pollN++
		if allDone {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}

	// Record home geometry (centre of workspace).
	s.homeLenMM = homeLength(s.cfg.WidthMM, s.cfg.HeightMM)
	s.posX = s.cfg.WidthMM / 2
	s.posY = s.cfg.HeightMM / 2
	s.homed = true
	slog.Info("home complete",
		"len_mm", s.homeLenMM,
		"home_pos", s.homePos,
		"x", s.posX,
		"y", s.posY,
	)
	return nil
}

// ── Motion ────────────────────────────────────────────────────────────────────

// MoveTo moves the camera to (x, y) mm from the top-left corner at the given
// speed (mm/s). All 4 motors run simultaneously with proportional speeds so
// that they all finish at the same time, keeping the camera on a straight line.
//
// Motion is two-phase: full proportional speed → collective slowdown to
// multiApproachRPM when the leading motor enters its approach zone.
//
// Returns when all motors have stopped. Cancelling ctx stops all motors.
func (s *System) MoveTo(ctx context.Context, x, y, speedMmPerSec float64) error {
	if !s.homed {
		return fmt.Errorf("robot: MoveTo called before Home")
	}
	if err := s.checkWorkspace(x, y, speedMmPerSec); err != nil {
		return err
	}

	targets := cableLengths(x, y, s.cfg.WidthMM, s.cfg.HeightMM)
	currLens, err := s.currentCableLengths()
	if err != nil {
		return fmt.Errorf("robot: MoveTo read cable lengths: %w", err)
	}

	// Compute delta pulses for each motor.
	// ΔL > 0 means cable must get longer (pay out) → negative pulses.
	// ΔL < 0 means cable must get shorter (wind in) → positive pulses.
	// motorDir negates for drums mounted in the opposite winding orientation.
	var deltaPulses [4]int64
	var maxAbsMM float64
	for i := range 4 {
		deltaL := targets[i] - currLens[i]
		deltaPulses[i] = mmToPulses(-deltaL, s.cfg.DrumRadiusMM, s.cfg.PulsesPerRev) * int64(s.motorDir(i))
		if absMM := math.Abs(deltaL); absMM > maxAbsMM {
			maxAbsMM = absMM
		}
	}

	if maxAbsMM < 0.5 {
		s.posX, s.posY = x, y
		return nil
	}

	// Per-motor speeds proportional to distance so all finish simultaneously.
	// The motor with the largest displacement runs at the requested speed;
	// others run slower.
	maxSpeedRPM := mmPerSecToRPM(speedMmPerSec, s.cfg.DrumRadiusMM)
	var speeds [4]int
	for i := range 4 {
		absPulses := deltaPulses[i]
		if absPulses < 0 {
			absPulses = -absPulses
		}
		maxAbsPulses := mmToPulses(maxAbsMM, s.cfg.DrumRadiusMM, s.cfg.PulsesPerRev)
		if maxAbsPulses < 0 {
			maxAbsPulses = -maxAbsPulses
		}
		if maxAbsPulses == 0 {
			continue
		}
		speeds[i] = int(int64(maxSpeedRPM) * absPulses / maxAbsPulses)
		if speeds[i] < 1 && absPulses > 0 {
			speeds[i] = 1
		}
	}

	return s.movePulses(ctx, deltaPulses, speeds, maxSpeedRPM, x, y)
}

// ── Tension hold ──────────────────────────────────────────────────────────────

// HoldTension switches all 4 motors into passive tension mode:
// slow winding speed + torque cap. The drive hardware stalls at the cap
// when the cable is taut, and resumes winding if slack develops.
// No goroutine needed — the drive controller handles it autonomously.
//
// Call after a MoveTo to keep cables taut while stationary.
func (s *System) HoldTension() error {
	for i, m := range s.motors {
		if err := m.SetTorqueLimit(s.cfg.HoldTensionPct); err != nil {
			return fmt.Errorf("robot: hold tension motor %d: %w", i+1, err)
		}
		if err := m.SetSpeed(s.cfg.HoldTensionRPM * s.motorDir(i)); err != nil {
			return fmt.Errorf("robot: hold tension motor %d: %w", i+1, err)
		}
	}
	return nil
}

// ── Emergency ─────────────────────────────────────────────────────────────────

// EmergencyStop disables all 4 motors immediately. Errors are collected but
// do not prevent the remaining motors from being stopped.
func (s *System) EmergencyStop() error {
	slog.Warn("emergency stop")
	var first error
	for _, m := range s.motors {
		if err := m.Disable(); err != nil && first == nil {
			first = err
		}
	}
	return first
}

// ── Status ────────────────────────────────────────────────────────────────────

// ReadAllStatus reads FC04 status from all 4 motors sequentially.
func (s *System) ReadAllStatus() [4]MotorState {
	var out [4]MotorState
	for i, m := range s.motors {
		st, err := m.ReadStatus()
		out[i] = MotorState{ID: i + 1, Status: st, Err: err}
	}
	return out
}

// Position returns the last known (x, y) camera position in mm from top-left.
// Only valid after a successful Home call.
func (s *System) Position() (x, y float64) { return s.posX, s.posY }

// Homed reports whether Home has completed successfully at least once.
func (s *System) Homed() bool { return s.homed }

// ── Internal ──────────────────────────────────────────────────────────────────

// movePulses executes a synchronised multi-motor move.
//
// Phase 1 — proportional full speed, with trapezoidal accel/decel via P-060/P-061
// (set from Config.AccelMmPerSec2). The leading motor's approach zone is computed
// from the motion profile's deceleration distance instead of a heuristic.
//
// Phase 2 — collective slowdown to multiApproachRPM (proportional), each motor
// stops independently when within multiTolerance pulses of its target.
func (s *System) movePulses(ctx context.Context, pulses [4]int64, speeds [4]int, maxSpeedRPM int, finalX, finalY float64) error {
	// Restore full torque capacity before motion. Home and HoldTension leave
	// P-069/P-070 at low values (5-10%) which prevent normal movement.
	moveTorque := s.cfg.MoveTorquePct
	if moveTorque <= 0 {
		moveTorque = 300
	}
	for i, m := range s.motors {
		if err := m.SetTorqueLimit(moveTorque); err != nil {
			return fmt.Errorf("robot: motor %d restore torque limit: %w", i+1, err)
		}
	}

	// Disable all and read stable start positions.
	for _, m := range s.motors {
		_ = m.Disable()
	}
	select {
	case <-time.After(s.cfg.DisableWait):
	case <-ctx.Done():
		return ctx.Err()
	}

	var starts [4]int32
	for i, m := range s.motors {
		p, err := m.ReadAbsPosition()
		if err != nil {
			return fmt.Errorf("robot: motor %d read start: %w", i+1, err)
		}
		starts[i] = p
	}

	// Compute motion profile and set hardware accel/decel ramps (P-060/P-061).
	// The profile is based on the master axis (largest displacement).
	maxSpeedMMperSec := rpmToMMperSec(float64(maxSpeedRPM), s.cfg.DrumRadiusMM)
	var prof motion.TrapProfile
	if s.cfg.AccelMmPerSec2 > 0 {
		// Longest cable travel for this move (master axis).
		var maxAbsPulses int64
		for _, p := range pulses {
			if p < 0 {
				p = -p
			}
			if p > maxAbsPulses {
				maxAbsPulses = p
			}
		}
		masterDistMM := float64(maxAbsPulses) / pulsesPerMM(s.cfg.DrumRadiusMM, s.cfg.PulsesPerRev)
		prof = motion.New(masterDistMM, maxSpeedMMperSec, s.cfg.AccelMmPerSec2)

		hwParam := motion.AccelToT3DParam(s.cfg.AccelMmPerSec2, s.cfg.DrumRadiusMM)
		for i, m := range s.motors {
			if pulses[i] == 0 {
				continue
			}
			if err := m.SetAccelTime(hwParam); err != nil {
				return fmt.Errorf("robot: motor %d set accel time: %w", i+1, err)
			}
			if err := m.SetDecelTime(hwParam); err != nil {
				return fmt.Errorf("robot: motor %d set decel time: %w", i+1, err)
			}
		}
	}

	// Set speeds and enable all active motors.
	for i, m := range s.motors {
		if pulses[i] == 0 {
			continue
		}
		rpm := speeds[i]
		if pulses[i] < 0 {
			rpm = -rpm
		}
		if err := m.WriteParam(t3d.ParamInternalSpd1, uint16(int16(rpm))); err != nil {
			return fmt.Errorf("robot: motor %d set speed: %w", i+1, err)
		}
	}
	for i, m := range s.motors {
		if pulses[i] == 0 {
			continue
		}
		if err := m.Enable(); err != nil {
			return fmt.Errorf("robot: motor %d enable: %w", i+1, err)
		}
	}

	// Approach zone: deceleration distance from the motion profile, or heuristic fallback.
	var collectiveApproach int64
	if prof.DecelDistMM > 0 {
		collectiveApproach = mmToPulses(prof.DecelDistMM, s.cfg.DrumRadiusMM, s.cfg.PulsesPerRev)
		if collectiveApproach < 0 {
			collectiveApproach = -collectiveApproach
		}
	}
	if collectiveApproach < s.cfg.MinApproachPulses {
		// Fallback: heuristic (covers low-speed or zero-accel config cases).
		collectiveApproach = max(int64(s.cfg.ApproachFactor*maxSpeedRPM), s.cfg.MinApproachPulses)
	}

	done := [4]bool{}
	inApproach := false

	for {
		select {
		case <-ctx.Done():
			_ = s.EmergencyStop()
			return ctx.Err()
		default:
		}

		// Poll all motors.
		allDone := true
		for i, m := range s.motors {
			if done[i] || pulses[i] == 0 {
				done[i] = true
				continue
			}
			allDone = false

			pos, torque, fault, err := m.ReadMotionState()
			if err != nil {
				_ = s.EmergencyStop()
				return fmt.Errorf("robot: motor %d poll: %w", i+1, err)
			}
			if fault != 0 {
				slog.Warn("drive fault during move", "motor", i+1, "fault", fault)
				_ = s.EmergencyStop()
				return fmt.Errorf("robot: motor %d fault %d", i+1, fault)
			}
			absT := torque
			if absT < 0 {
				absT = -absT
			}
			if int(absT) >= s.cfg.TorqueSafetyPct {
				slog.Warn("torque safety trip", "motor", i+1, "torque_pct", torque, "limit_pct", s.cfg.TorqueSafetyPct)
				_ = s.EmergencyStop()
				return fmt.Errorf("robot: motor %d torque safety trip %d%%", i+1, torque)
			}

			traveled := int64(pos) - int64(starts[i])
			if traveled < 0 {
				traveled = -traveled
			}
			absPulses := pulses[i]
			if absPulses < 0 {
				absPulses = -absPulses
			}
			remaining := absPulses - traveled

			// Collective slowdown: triggered by whichever motor leads first.
			// Using any motor (not just the master) as the trigger is conservative:
			// a cable that is nearly done must not overshoot while waiting for
			// slower cables, as slack causes loss of position.
			if !inApproach && remaining <= collectiveApproach {
				inApproach = true
				if err := s.collectiveSlowdown(done, pulses, speeds, maxSpeedRPM); err != nil {
					_ = s.EmergencyStop()
					return fmt.Errorf("robot: slowdown: %w", err)
				}
			}

			if remaining <= s.cfg.TolerancePulses {
				_ = m.Disable()
				done[i] = true
			}
		}

		if allDone {
			break
		}
		time.Sleep(s.cfg.PollInterval)
	}

	time.Sleep(s.cfg.StopSettle)
	s.posX, s.posY = finalX, finalY
	return nil
}

// collectiveSlowdown disables all still-running motors simultaneously, then
// restarts them at proportional approach speeds.
func (s *System) collectiveSlowdown(done [4]bool, pulses [4]int64, speeds [4]int, maxSpeedRPM int) error {
	for i, m := range s.motors {
		if done[i] || pulses[i] == 0 {
			continue
		}
		_ = m.Disable()
	}
	time.Sleep(s.cfg.ApproachSwitch)

	for i, m := range s.motors {
		if done[i] || pulses[i] == 0 {
			continue
		}
		// Proportional approach speed: same ratio as full-speed, scaled to ApproachRPM.
		approachRPM := s.cfg.ApproachRPM
		if speeds[i] > 0 && maxSpeedRPM > 0 {
			approachRPM = s.cfg.ApproachRPM * speeds[i] / maxSpeedRPM
		}
		// Never exceed the commanded speed — at low G1 feedrates multiApproachRPM
		// can be higher than speeds[i], which would accelerate instead of slow down.
		if approachRPM > speeds[i] {
			approachRPM = speeds[i]
		}
		if approachRPM < 1 {
			approachRPM = 1
		}
		if pulses[i] < 0 {
			approachRPM = -approachRPM
		}
		if err := m.WriteParam(t3d.ParamInternalSpd1, uint16(int16(approachRPM))); err != nil {
			return fmt.Errorf("motor %d set approach speed: %w", i+1, err)
		}
		if err := m.Enable(); err != nil {
			return fmt.Errorf("motor %d approach enable: %w", i+1, err)
		}
	}
	return nil
}

// currentCableLengths reads the absolute encoder position and fault code from each
// motor in a single FC04 transaction, applies motorDir, and returns cable lengths
// in mm. Returns error on read failure or if any motor reports a non-zero fault.
func (s *System) currentCableLengths() ([4]float64, error) {
	ppm := pulsesPerMM(s.cfg.DrumRadiusMM, s.cfg.PulsesPerRev)
	var lengths [4]float64
	for i, m := range s.motors {
		pos, fault, err := m.ReadAbsPosAndFault()
		if err != nil {
			return lengths, fmt.Errorf("motor %d: read: %w", i+1, err)
		}
		if fault != 0 {
			slog.Warn("drive fault detected", "motor", i+1, "fault", fmt.Sprintf("0x%04X", fault))
			return lengths, fmt.Errorf("motor %d: fault 0x%04X", i+1, fault)
		}
		deltaEncoder := float64(int64(pos)-int64(s.homePos[i])) * float64(s.motorDir(i))
		lengths[i] = s.homeLenMM - deltaEncoder/ppm
	}
	return lengths, nil
}

// motorDir returns the direction multiplier for motor i: +1 for normal, -1 for reversed.
// Multiply any RPM or pulse-count command by this value to account for drums mounted
// so that positive RPM pays out instead of winding in.
func (s *System) motorDir(i int) int {
	if s.cfg.MotorReversed[i] {
		return -1
	}
	return 1
}

const maxCableSpeedMmPerSec = 250.0 // physical drum limit: ~237 mm/s at 2000 RPM

func (s *System) checkWorkspace(x, y, speed float64) error {
	if x < 0 || x > s.cfg.WidthMM || y < 0 || y > s.cfg.HeightMM {
		return fmt.Errorf("robot: position (%.1f, %.1f) outside workspace [0,%.0f]×[0,%.0f]",
			x, y, s.cfg.WidthMM, s.cfg.HeightMM)
	}
	if speed <= 0 || speed > maxCableSpeedMmPerSec {
		return fmt.Errorf("robot: speed %.1f out of range (0, %.0f]", speed, maxCableSpeedMmPerSec)
	}
	return nil
}

// mmPerSecToRPM converts a cable speed (mm/s) to motor RPM for the given drum radius.
//
//	RPM = (mm/s) / circumference_mm × 60
func mmPerSecToRPM(mmPerSec, drumRadiusMM float64) int {
	circumference := 2 * math.Pi * drumRadiusMM
	return int(math.Round(mmPerSec / circumference * 60))
}

// ── Debug / manual motor control ──────────────────────────────────────────────

// JogMotor enables motor motorIdx (0-based) at the given cable speed.
// rpm > 0 winds in cable (shortens); rpm < 0 pays out (lengthens).
// motorDir is applied so the physical cable direction is always correct.
func (s *System) JogMotor(motorIdx, rpm int) error {
	if motorIdx < 0 || motorIdx > 3 {
		return fmt.Errorf("robot: invalid motor index %d", motorIdx)
	}
	return s.motors[motorIdx].SetSpeed(rpm * s.motorDir(motorIdx))
}

// JogStop disables motor motorIdx immediately.
func (s *System) JogStop(motorIdx int) error {
	if motorIdx < 0 || motorIdx > 3 {
		return fmt.Errorf("robot: invalid motor index %d", motorIdx)
	}
	return s.motors[motorIdx].Disable()
}

// ReadMotorStatus reads all FC04 status registers for motor motorIdx (0-based).
func (s *System) ReadMotorStatus(motorIdx int) (*t3d.Status, error) {
	if motorIdx < 0 || motorIdx > 3 {
		return nil, fmt.Errorf("robot: invalid motor index %d", motorIdx)
	}
	return s.motors[motorIdx].ReadStatus()
}

// WriteMotorParam writes a FC03 holding register (parameter) to motor motorIdx.
func (s *System) WriteMotorParam(motorIdx int, addr, value uint16) error {
	if motorIdx < 0 || motorIdx > 3 {
		return fmt.Errorf("robot: invalid motor index %d", motorIdx)
	}
	return s.motors[motorIdx].WriteParam(addr, value)
}

// ReadMotorParam reads a FC03 holding register (parameter) from motor motorIdx.
func (s *System) ReadMotorParam(motorIdx int, addr uint16) (uint16, error) {
	if motorIdx < 0 || motorIdx > 3 {
		return 0, fmt.Errorf("robot: invalid motor index %d", motorIdx)
	}
	return s.motors[motorIdx].ReadParam(addr)
}
