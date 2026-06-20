package motion

import (
	"math"
	"testing"
)

const eps = 1e-6

func near(a, b, tolerance float64) bool {
	return math.Abs(a-b) <= tolerance
}

func TestNew_ZeroAndNegativeInputs(t *testing.T) {
	cases := []struct {
		name         string
		dist, v, acc float64
	}{
		{"zero dist", 0, 300, 1000},
		{"zero vMax", 1000, 0, 1000},
		{"zero accel", 1000, 300, 0},
		{"negative dist", -10, 300, 1000},
		{"negative vMax", 1000, -300, 1000},
		{"negative accel", 1000, 300, -1000},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			p := New(tc.dist, tc.v, tc.acc) // must not panic
			// Zero/degenerate profile should have zero total duration.
			if tc.dist <= 0 || tc.v <= 0 || tc.acc <= 0 {
				if p.Total != 0 {
					t.Errorf("expected Total=0 for degenerate input, got %v", p.Total)
				}
			}
		})
	}
}

func TestNew_TriangleProfile(t *testing.T) {
	// dist=50, vMax=300, accel=1000
	// Full accel+decel distance = 300²/1000 = 90 > 50 → triangle profile
	p := New(50, 300, 1000)

	if p.TCruise != 0 {
		t.Errorf("triangle profile: TCruise = %v, want 0", p.TCruise)
	}
	if p.VMax >= 300 {
		t.Errorf("triangle profile: VMax = %v, should be < 300 (reduced peak)", p.VMax)
	}
	// vPeak = sqrt(1000 * 50) = 223.6...
	wantVMax := math.Sqrt(1000 * 50)
	if !near(p.VMax, wantVMax, 0.01) {
		t.Errorf("triangle VMax = %.4f, want %.4f", p.VMax, wantVMax)
	}
	if !near(p.TAccel, p.TDecel, eps) {
		t.Errorf("triangle profile not symmetric: TAccel=%v TDecel=%v", p.TAccel, p.TDecel)
	}
	if !near(p.PositionAt(p.Total), 50, 0.001) {
		t.Errorf("position at end = %v, want 50", p.PositionAt(p.Total))
	}
}

func TestNew_TrapezoidalProfile(t *testing.T) {
	// dist=1000, vMax=300, accel=1000 → dFull=90 < 1000 → trapezoidal
	p := New(1000, 300, 1000)

	if !near(p.TAccel, 0.3, 0.001) {
		t.Errorf("TAccel = %.4f, want 0.3", p.TAccel)
	}
	if !near(p.TDecel, 0.3, 0.001) {
		t.Errorf("TDecel = %.4f, want 0.3", p.TDecel)
	}
	if p.TCruise <= 0 {
		t.Errorf("TCruise = %v, should be > 0 for trapezoidal", p.TCruise)
	}
	if !near(p.VMax, 300, 0.001) {
		t.Errorf("VMax = %.4f, want 300", p.VMax)
	}
	// DecelDistMM = vMax²/(2·accel) = 300²/2000 = 45
	if !near(p.DecelDistMM, 45, 0.01) {
		t.Errorf("DecelDistMM = %.4f, want 45", p.DecelDistMM)
	}
	// Total = TAccel + TCruise + TDecel
	if !near(p.Total, p.TAccel+p.TCruise+p.TDecel, eps) {
		t.Errorf("Total ≠ TAccel+TCruise+TDecel")
	}
}

func TestVelocityAt(t *testing.T) {
	p := New(1000, 300, 1000) // trapezoidal

	t.Run("before start", func(t *testing.T) {
		if v := p.VelocityAt(-1); v != 0 {
			t.Errorf("v(-1) = %v, want 0", v)
		}
	})
	t.Run("at t=0", func(t *testing.T) {
		if v := p.VelocityAt(0); v != 0 {
			t.Errorf("v(0) = %v, want 0", v)
		}
	})
	t.Run("end of accel phase", func(t *testing.T) {
		if v := p.VelocityAt(p.TAccel); !near(v, p.VMax, 0.01) {
			t.Errorf("v(TAccel) = %v, want %v", v, p.VMax)
		}
	})
	t.Run("mid cruise", func(t *testing.T) {
		mid := p.TAccel + p.TCruise/2
		if v := p.VelocityAt(mid); !near(v, p.VMax, 0.01) {
			t.Errorf("v(mid cruise) = %v, want %v", v, p.VMax)
		}
	})
	t.Run("start of decel phase", func(t *testing.T) {
		if v := p.VelocityAt(p.TAccel + p.TCruise); !near(v, p.VMax, 0.01) {
			t.Errorf("v(start decel) = %v, want %v", v, p.VMax)
		}
	})
	t.Run("at total", func(t *testing.T) {
		if v := p.VelocityAt(p.Total); v != 0 {
			t.Errorf("v(Total) = %v, want 0", v)
		}
	})
	t.Run("beyond total", func(t *testing.T) {
		if v := p.VelocityAt(p.Total + 10); v != 0 {
			t.Errorf("v(Total+10) = %v, want 0", v)
		}
	})
}

func TestPositionAt(t *testing.T) {
	p := New(1000, 300, 1000) // trapezoidal

	t.Run("at 0", func(t *testing.T) {
		if pos := p.PositionAt(0); pos != 0 {
			t.Errorf("pos(0) = %v, want 0", pos)
		}
	})
	t.Run("at total", func(t *testing.T) {
		if pos := p.PositionAt(p.Total); !near(pos, p.Dist, 0.001) {
			t.Errorf("pos(Total) = %v, want %v", pos, p.Dist)
		}
	})
	t.Run("at end of accel = DecelDistMM", func(t *testing.T) {
		// pos(TAccel) = 0.5 * VMax * TAccel = 0.5 * 300 * 0.3 = 45 = DecelDistMM
		got := p.PositionAt(p.TAccel)
		want := 0.5 * p.VMax * p.TAccel
		if !near(got, want, 0.01) {
			t.Errorf("pos(TAccel) = %.4f, want %.4f", got, want)
		}
		if !near(got, p.DecelDistMM, 0.01) {
			t.Errorf("pos(TAccel) = %.4f should equal DecelDistMM=%.4f", got, p.DecelDistMM)
		}
	})
	t.Run("before start clamped to 0", func(t *testing.T) {
		if pos := p.PositionAt(-5); pos != 0 {
			t.Errorf("pos(-5) = %v, want 0", pos)
		}
	})
	t.Run("beyond total clamped to dist", func(t *testing.T) {
		if pos := p.PositionAt(p.Total + 100); !near(pos, p.Dist, 0.001) {
			t.Errorf("pos(Total+100) = %v, want %v", pos, p.Dist)
		}
	})
	t.Run("monotone: position never decreases", func(t *testing.T) {
		prev := 0.0
		for i := 0; i <= 100; i++ {
			t_ := p.Total * float64(i) / 100
			pos := p.PositionAt(t_)
			if pos < prev-eps {
				t.Errorf("position decreased at t=%.3f: %.6f < %.6f", t_, pos, prev)
			}
			prev = pos
		}
	})
}

func TestAccelToT3DParam(t *testing.T) {
	t.Run("known value r=67.8 accel=1000", func(t *testing.T) {
		got := AccelToT3DParam(1000, 67.8)
		// circumference = 2π×67.8 ≈ 425.98 mm
		// accelRPM/s = 1000×60/425.98 ≈ 140.85
		// result = round(1e6/140.85) ≈ 7100
		if got < 7000 || got > 7200 {
			t.Errorf("AccelToT3DParam(1000, 67.8) = %d, want ~7100 (7000-7200)", got)
		}
	})
	t.Run("zero accel returns default 100", func(t *testing.T) {
		if got := AccelToT3DParam(0, 67.8); got != 100 {
			t.Errorf("AccelToT3DParam(0, 67.8) = %d, want 100", got)
		}
	})
	t.Run("zero drum radius returns default 100", func(t *testing.T) {
		if got := AccelToT3DParam(1000, 0); got != 100 {
			t.Errorf("AccelToT3DParam(1000, 0) = %d, want 100", got)
		}
	})
	t.Run("higher accel → smaller param (faster ramp)", func(t *testing.T) {
		slow := AccelToT3DParam(100, 67.8)
		fast := AccelToT3DParam(1000, 67.8)
		if slow <= fast {
			t.Errorf("higher accel should give smaller param: slow=%d fast=%d", slow, fast)
		}
	})
}
