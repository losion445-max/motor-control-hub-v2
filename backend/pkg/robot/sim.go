package robot

import (
	"sync"
	"time"

	"github.com/losion445-max/motor-control-hub-v2/pkg/t3d"
)

// NewSimSystem creates a System with 4 software-simulated motors.
// Use for testing without physical RS-485 hardware (server -sim flag).
// Connect() and Close() are no-ops on a sim system.
func NewSimSystem(cfg Config) *System {
	s := &System{cfg: cfg}
	for i := range 4 {
		s.motors[i] = newSimMotor()
	}
	return s
}

// simMotor is a software simulation of a T3D servo drive.
//
// Position updates every 10 ms based on the current speed command (P-137).
// Torque simulation: while winding in (negative RPM), after 1 second
// of continuous motion the motor reports −25 % torque to trigger homing
// threshold detection (HomingTorquePct = 20 %).
type simMotor struct {
	mu    sync.Mutex
	pos   int32
	speed int16 // current P-137 value (RPM); negative = reverse

	// homing torque simulation
	windStart  time.Time
	windActive bool
}

const simTick = 10 * time.Millisecond

// pulsesPerRev is baked into the tick math: 10000 PPR, 10 ms period.
//
//	pulses per tick = RPM × 10000 / (60 × 1000/10) = RPM × 10000 / 6000
const simPulsesPerRPMPerTick = 10000.0 / 6000.0

func newSimMotor() *simMotor {
	m := &simMotor{}
	go m.run()
	return m
}

func (m *simMotor) run() {
	t := time.NewTicker(simTick)
	defer t.Stop()
	for range t.C {
		m.mu.Lock()
		if m.speed != 0 {
			delta := int32(float64(m.speed) * simPulsesPerRPMPerTick)
			m.pos += delta

			// Track sustained winding-in (negative speed).
			if m.speed < 0 {
				if !m.windActive {
					m.windActive = true
					m.windStart = time.Now()
				}
			} else {
				m.windActive = false
			}
		} else {
			m.windActive = false
		}
		m.mu.Unlock()
	}
}

func (m *simMotor) torqueLocked() int16 {
	if m.windActive && time.Since(m.windStart) >= time.Second {
		return -25 // cable taut — triggers HomingTorquePct=20 threshold
	}
	if m.speed != 0 {
		return m.speed / 20 // small proportional drag
	}
	return 0
}

// ── driveMotor interface ──────────────────────────────────────────────────────

func (m *simMotor) Enable() error { return nil }

func (m *simMotor) Disable() error {
	m.mu.Lock()
	m.speed = 0
	m.windActive = false
	m.mu.Unlock()
	return nil
}

func (m *simMotor) WriteParam(addr, value uint16) error {
	if addr == t3d.ParamInternalSpd1 {
		m.mu.Lock()
		m.speed = int16(value)
		if m.speed >= 0 {
			m.windActive = false
		}
		m.mu.Unlock()
	}
	return nil
}

func (m *simMotor) ReadParam(_ uint16) (uint16, error) { return 0, nil }

func (m *simMotor) ReadAbsPosition() (int32, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.pos, nil
}

func (m *simMotor) ReadAbsPosAndFault() (int32, uint16, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.pos, 0, nil
}

func (m *simMotor) ReadTorquePct() (int16, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.torqueLocked(), nil
}

func (m *simMotor) ReadFault() (uint16, error) { return 0, nil }

func (m *simMotor) ReadStatus() (*t3d.Status, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return &t3d.Status{
		SpeedRPM:      m.speed,
		TorquePct:     m.torqueLocked(),
		FaultCode:     0,
		Position32:    m.pos,
		BusVoltageV:   310,
		HeatsinkTempC: 35,
		ModuleTempC:   30,
	}, nil
}

func (m *simMotor) ReadMotionState() (int32, int16, uint16, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.pos, m.torqueLocked(), 0, nil
}

func (m *simMotor) SetAccelTime(_ int) error { return nil }
func (m *simMotor) SetDecelTime(_ int) error { return nil }

func (m *simMotor) SetSpeed(rpm int) error {
	m.mu.Lock()
	m.speed = int16(rpm)
	if m.speed >= 0 {
		m.windActive = false
	}
	m.mu.Unlock()
	return nil
}

func (m *simMotor) SetTorqueLimit(_ int) error { return nil }
