package robot

import "math"

// cableLengths returns the 4 cable lengths (mm) for camera at position (x, y)
// measured from the top-left corner M1.
//
//	M1(0,0)      M2(W,0)
//	M4(0,H)      M3(W,H)
func cableLengths(x, y, W, H float64) [4]float64 {
	return [4]float64{
		math.Sqrt(x*x + y*y),                      // M1 (0,0)
		math.Sqrt((W-x)*(W-x) + y*y),              // M2 (W,0)
		math.Sqrt((W-x)*(W-x) + (H-y)*(H-y)),     // M3 (W,H)
		math.Sqrt(x*x + (H-y)*(H-y)),              // M4 (0,H)
	}
}

// homeLength returns the cable length (mm) at the centre of a W×H rectangle.
func homeLength(W, H float64) float64 {
	return math.Sqrt((W/2)*(W/2) + (H/2)*(H/2))
}

// pulsesPerMM returns encoder pulses per mm of cable travel.
//
//	pulses/mm = pulsesPerRev / circumference = pulsesPerRev / (2π × r)
func pulsesPerMM(drumRadiusMM float64, pulsesPerRev int) float64 {
	return float64(pulsesPerRev) / (2 * math.Pi * drumRadiusMM)
}

// mmToPulses converts a cable length delta (mm) to encoder pulse delta.
//
// Sign convention (positive RPM = wind in = encoder increases):
//
//	mm > 0  →  cable shorter  →  motor wound in  →  positive pulses
//	mm < 0  →  cable longer   →  motor paid out  →  negative pulses
func mmToPulses(mm, drumRadiusMM float64, pulsesPerRev int) int64 {
	return int64(math.Round(mm * pulsesPerMM(drumRadiusMM, pulsesPerRev)))
}

// rpmToMMperSec converts motor RPM to cable linear speed (mm/s).
func rpmToMMperSec(rpm, drumRadiusMM float64) float64 {
	return rpm * 2 * math.Pi * drumRadiusMM / 60
}
