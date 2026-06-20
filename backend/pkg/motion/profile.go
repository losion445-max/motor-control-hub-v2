// Package motion provides trapezoidal velocity profile calculations for
// multi-axis cable robots. It has no dependencies on hardware packages —
// it operates in consistent user-defined units (e.g. mm and seconds).
//
// Typical usage in a robot layer:
//
//	p := motion.New(distanceMM, maxSpeedMMperSec, accelMMperSecSq)
//	hwParam := motion.AccelToT3DParam(accelMMperSecSq, drumRadiusMM)
//	motor.SetAccelTime(hwParam)
//	motor.SetDecelTime(hwParam)
//	// run motors; trigger deceleration when remaining ≤ p.DecelDistMM
package motion

import "math"

// TrapProfile is a precomputed trapezoidal velocity profile.
// All fields are in consistent units (e.g. mm and seconds).
//
//	velocity
//	 ▲    ╔═══════════╗
//	 │   ╔╝           ╚╗
//	VMax ╫╝             ╚╗
//	 │  ╔╝               ╚╗
//	 │──╚──────────────────╚──▶ time
//	    TAccel  TCruise  TDecel
//
// If the distance is too short to reach VMax, a triangle profile is used
// (TCruise = 0, VMax is the reduced peak velocity).
type TrapProfile struct {
	Dist  float64 // total distance (mm or any length unit)
	VMax  float64 // cruise velocity (same unit/s); may be < requested vMax for triangle
	Accel float64 // acceleration and deceleration magnitude

	TAccel  float64 // duration of acceleration phase (s)
	TCruise float64 // duration of cruise phase (s); 0 for triangle profile
	TDecel  float64 // duration of deceleration phase (s); equals TAccel

	// Total duration of the profile (s).
	Total float64

	// DecelDistMM is the distance covered during the deceleration phase alone.
	// Use this as the approach-zone threshold: when remaining ≤ DecelDistMM,
	// cutting motor power lets the hardware ramp (P-061) land exactly at target.
	DecelDistMM float64
}

// New computes a TrapProfile for the given distance, maximum velocity, and
// symmetric acceleration/deceleration magnitude.
//
// Returns a zero TrapProfile for zero or negative inputs.
func New(dist, vMax, accel float64) TrapProfile {
	if dist <= 0 || vMax <= 0 || accel <= 0 {
		return TrapProfile{Dist: dist, VMax: vMax, Accel: accel}
	}

	// Full accel + decel distance (no cruise).
	dFull := vMax * vMax / accel

	var p TrapProfile
	p.Dist = dist
	p.Accel = accel

	if dist <= dFull {
		// Triangle: peak velocity reduced so that accel + decel = dist exactly.
		vPeak := math.Sqrt(accel * dist)
		p.VMax = vPeak
		p.TAccel = vPeak / accel
		p.TCruise = 0
		p.TDecel = p.TAccel
	} else {
		// Trapezoidal.
		p.VMax = vMax
		p.TAccel = vMax / accel
		p.TCruise = (dist - dFull) / vMax
		p.TDecel = p.TAccel
	}

	p.Total = p.TAccel + p.TCruise + p.TDecel
	p.DecelDistMM = 0.5 * p.VMax * p.TDecel // = VMax² / (2·Accel)
	return p
}

// VelocityAt returns the instantaneous velocity at time t.
func (p TrapProfile) VelocityAt(t float64) float64 {
	switch {
	case t <= 0 || p.Total == 0:
		return 0
	case t >= p.Total:
		return 0
	case t < p.TAccel:
		return p.Accel * t
	case t < p.TAccel+p.TCruise:
		return p.VMax
	default:
		return p.VMax - p.Accel*(t-(p.TAccel+p.TCruise))
	}
}

// PositionAt returns the distance traveled at time t.
func (p TrapProfile) PositionAt(t float64) float64 {
	switch {
	case t <= 0:
		return 0
	case t >= p.Total:
		return p.Dist
	case t < p.TAccel:
		return 0.5 * p.Accel * t * t
	case t < p.TAccel+p.TCruise:
		return 0.5*p.VMax*p.TAccel + p.VMax*(t-p.TAccel)
	default:
		posAC := 0.5*p.VMax*p.TAccel + p.VMax*p.TCruise
		dt := t - (p.TAccel + p.TCruise)
		return posAC + p.VMax*dt - 0.5*p.Accel*dt*dt
	}
}

// AccelToT3DParam converts a linear cable acceleration (mm/s²) to the value
// for the T3D drive's P-060/P-061 parameters (ms per 1000 RPM).
//
// The T3D P-060 parameter defines how many milliseconds it takes to accelerate
// the motor by 1000 RPM. Setting P-060/P-061 using this function makes the
// hardware ramp match the planned TrapProfile, so cutting power at DecelDistMM
// stops the motor at the intended target.
//
// drumRadiusMM is the effective radius of the cable drum (mm).
func AccelToT3DParam(accelMmPerS2, drumRadiusMM float64) int {
	if accelMmPerS2 <= 0 || drumRadiusMM <= 0 {
		return 100 // drive default
	}
	circumference := 2 * math.Pi * drumRadiusMM
	// Cable acceleration in RPM/s → time to accelerate 1000 RPM in ms.
	accelRPMperS := accelMmPerS2 * 60 / circumference
	return int(math.Round(1e6 / accelRPMperS))
}
