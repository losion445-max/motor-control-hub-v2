package robot

import (
	"math"

	"github.com/losion445-max/motor-control-hub-v2/pkg/t3d"
)

// cableLengths returns the 4 cable lengths (mm) for camera at position (x, y)
// measured from the top-left corner M1.
//
//	M1(0,0)      M2(W,0)
//	M4(0,H)      M3(W,H)
func cableLengths(x, y, W, H float64) [4]float64 {
	return [4]float64{
		math.Sqrt(x*x + y*y),                       // M1 (0,0)
		math.Sqrt((W-x)*(W-x) + y*y),               // M2 (W,0)
		math.Sqrt((W-x)*(W-x) + (H-y)*(H-y)),      // M3 (W,H)
		math.Sqrt(x*x + (H-y)*(H-y)),               // M4 (0,H)
	}
}

// homeLength returns the cable length (mm) at the centre of a W×H rectangle.
// For any rectangle the centre is equidistant from all four corners.
func homeLength(W, H float64) float64 {
	return math.Sqrt((W/2)*(W/2) + (H/2)*(H/2))
}

// pulsesPerMM returns encoder pulses per mm of cable for the given drum radius.
//
//	pulses/mm = PulsesPerRev / circumference = 10000 / (2π × r)
func pulsesPerMM(drumRadiusMM float64) float64 {
	return float64(t3d.PulsesPerRev) / (2 * math.Pi * drumRadiusMM)
}

// mmToPulses converts a cable length delta (mm) to encoder pulse delta.
//
// Sign convention (assumes positive RPM = wind in = encoder increases):
//
//	mm > 0  →  cable is shorter  →  motor wound in  →  positive pulses
//	mm < 0  →  cable is longer   →  motor paid out  →  negative pulses
func mmToPulses(mm, drumRadiusMM float64) int64 {
	return int64(math.Round(mm * pulsesPerMM(drumRadiusMM)))
}

// rpmToMMperSec converts motor RPM to cable linear speed (mm/s).
//
//	mm/s = RPM × circumference / 60  =  RPM × 2π × r / 60
func rpmToMMperSec(rpm, drumRadiusMM float64) float64 {
	return rpm * 2 * math.Pi * drumRadiusMM / 60
}
