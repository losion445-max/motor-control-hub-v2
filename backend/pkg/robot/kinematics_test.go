package robot

import (
	"math"
	"testing"
)

const (
	W    = 1400.0
	H    = 2400.0
	r    = 67.8
	epsK = 0.01 // mm tolerance for geometry calculations
)

func nearK(a, b float64) bool { return math.Abs(a-b) <= epsK }

// ── cableLengths ─────────────────────────────────────────────────────────────

func TestCableLengths_Center(t *testing.T) {
	// At the workspace centre (W/2, H/2) all four cable lengths must be equal
	// and match homeLength.
	lens := cableLengths(W/2, H/2, W, H)
	home := homeLength(W, H)
	for i, l := range lens {
		if !nearK(l, home) {
			t.Errorf("lens[%d] = %.4f, want homeLength=%.4f", i, l, home)
		}
	}
}

func TestCableLengths_TopLeftCorner(t *testing.T) {
	// At (0, 0) = M1's anchor:
	//   L1 = 0
	//   L2 = W
	//   L3 = sqrt(W²+H²)
	//   L4 = H
	lens := cableLengths(0, 0, W, H)
	want := [4]float64{
		0,
		W,
		math.Sqrt(W*W + H*H),
		H,
	}
	for i, w := range want {
		if !nearK(lens[i], w) {
			t.Errorf("lens[%d] = %.4f, want %.4f", i, lens[i], w)
		}
	}
}

func TestCableLengths_AllPositive(t *testing.T) {
	// For any interior point all cable lengths must be strictly positive.
	points := [][2]float64{
		{350, 600}, {700, 1200}, {1050, 1800}, {200, 200}, {1200, 2200},
	}
	for _, pt := range points {
		lens := cableLengths(pt[0], pt[1], W, H)
		for i, l := range lens {
			if l <= 0 {
				t.Errorf("cableLengths(%.0f,%.0f)[%d] = %v, want > 0",
					pt[0], pt[1], i, l)
			}
		}
	}
}

// ── homeLength ────────────────────────────────────────────────────────────────

func TestHomeLength(t *testing.T) {
	got := homeLength(W, H)
	want := math.Sqrt((W/2)*(W/2) + (H/2)*(H/2))
	if !nearK(got, want) {
		t.Errorf("homeLength = %.4f, want %.4f", got, want)
	}
	// For 1400×2400 the exact value ≈ 1389.24.
	if !nearK(got, 1389.24) {
		t.Errorf("homeLength(1400,2400) = %.4f, want ≈1389.24", got)
	}
}

// ── pulsesPerMM ───────────────────────────────────────────────────────────────

func TestPulsesPerMM(t *testing.T) {
	got := pulsesPerMM(r, 10000)
	// 10000 / (2π × 67.8) ≈ 23.47 pulses/mm
	want := 10000.0 / (2 * math.Pi * r)
	if math.Abs(got-want) > 0.001 {
		t.Errorf("pulsesPerMM(%.1f) = %.4f, want %.4f", r, got, want)
	}
	if !nearK(got, 23.47) {
		t.Errorf("pulsesPerMM(67.8) = %.4f, expected ~23.47", got)
	}
}

// ── mmToPulses ────────────────────────────────────────────────────────────────

func TestMmToPulses(t *testing.T) {
	t.Run("zero distance", func(t *testing.T) {
		if p := mmToPulses(0, r, 10000); p != 0 {
			t.Errorf("mmToPulses(0) = %d, want 0", p)
		}
	})
	t.Run("one pulse round-trip", func(t *testing.T) {
		oneMM := 1.0 / pulsesPerMM(r, 10000)
		if p := mmToPulses(oneMM, r, 10000); p != 1 {
			t.Errorf("mmToPulses(1 pulse in mm) = %d, want 1", p)
		}
	})
	t.Run("negative distance gives negative pulses", func(t *testing.T) {
		if p := mmToPulses(-10, r, 10000); p >= 0 {
			t.Errorf("mmToPulses(-10) = %d, expected negative", p)
		}
	})
	t.Run("positive distance gives positive pulses", func(t *testing.T) {
		if p := mmToPulses(10, r, 10000); p <= 0 {
			t.Errorf("mmToPulses(10) = %d, expected positive", p)
		}
	})
	t.Run("antisymmetry", func(t *testing.T) {
		pos := mmToPulses(100, r, 10000)
		neg := mmToPulses(-100, r, 10000)
		if pos != -neg {
			t.Errorf("mmToPulses(100)=%d != -mmToPulses(-100)=%d", pos, neg)
		}
	})
}

// ── rpmToMMperSec ─────────────────────────────────────────────────────────────

func TestRpmToMMperSec(t *testing.T) {
	t.Run("zero RPM", func(t *testing.T) {
		if v := rpmToMMperSec(0, r); v != 0 {
			t.Errorf("rpmToMMperSec(0) = %v, want 0", v)
		}
	})
	t.Run("1 RPM = one revolution per minute", func(t *testing.T) {
		got := rpmToMMperSec(1, r)
		want := 2 * math.Pi * r / 60
		if math.Abs(got-want) > 0.001 {
			t.Errorf("rpmToMMperSec(1) = %.4f, want %.4f", got, want)
		}
	})
	t.Run("proportional to RPM", func(t *testing.T) {
		v1 := rpmToMMperSec(100, r)
		v2 := rpmToMMperSec(200, r)
		if math.Abs(v2-2*v1) > 0.001 {
			t.Errorf("rpmToMMperSec not linear: v1=%.4f v2=%.4f", v1, v2)
		}
	})
}

// ── round-trip mmPerSecToRPM ↔ rpmToMMperSec ─────────────────────────────────

func TestRpmMMRoundTrip(t *testing.T) {
	for _, rpm := range []float64{10, 50, 100, 500} {
		mmPerSec := rpmToMMperSec(rpm, r)
		back := mmPerSecToRPM(mmPerSec, r)
		if math.Abs(float64(back)-rpm) > 1 {
			t.Errorf("round-trip RPM=%.0f → mm/s=%.4f → RPM=%d (want %.0f ±1)",
				rpm, mmPerSec, back, rpm)
		}
	}
}
