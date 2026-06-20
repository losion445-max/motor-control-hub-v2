// Package t3d implements a Modbus RTU driver for the HLTNC T3D servo driver.
// Protocol: Modbus RTU over RS-485, default 19200/8E1, Slave ID=1.
// Source: Modbus Communication function Description V3.3.
package t3d

// FC03 — Holding Registers (R/W). Address = P-xxx parameter number in decimal.
// Rule: FC03 addr of P-005 is 005 decimal (V3.3 section 4).
const (
	ParamControlMode    uint16 = 4   // P-004: 0=position/pulse, 1=speed, 2=torque
	ParamSpeedKp        uint16 = 5   // P-005: speed loop Kp (Hz, default=50)
	ParamSpeedTi        uint16 = 6   // P-006: speed loop Ti (ms, default=20)
	ParamTorqueFilter   uint16 = 7   // P-007: torque filter (×0.1ms, default=25)
	ParamPositionKp     uint16 = 9   // P-009: position loop Kp (1/s, default=40)
	ParamSpeedSource    uint16 = 25  // P-025: 0=analog, 1=internal multi-speed, 3=pulse
	ParamTorqueSource   uint16 = 26  // P-026: 0=analog, 1=internal multi-segment
	ParamGearNumerH     uint16 = 28  // P-028: electronic gear numerator high digits
	ParamGearNumerL     uint16 = 29  // P-029: electronic gear numerator low digits
	ParamGearDenom      uint16 = 30  // P-030: electronic gear denominator
	ParamPulseMode      uint16 = 35  // P-035: 0=pulse+dir, 1=CW/CCW, 2=AB quadrature
	ParamPulseDir       uint16 = 36  // P-036: 0=normal, 1=reverse
	ParamAccelTime      uint16 = 60  // P-060: accel time (ms per 1000 rpm, default=100)
	ParamDecelTime      uint16 = 61  // P-061: decel time (ms per 1000 rpm, default=100)
	ParamTorqueLimit    uint16 = 69  // P-069: torque limit (% of rated, default=300; set lower to cap tension)
	ParamMaxSpeed       uint16 = 75  // P-075: max speed (rpm, default=3000)
	ParamJogSpeed       uint16 = 76  // P-076: JOG speed (rpm, default=100)
	ParamServoOnMode    uint16 = 98  // P-098: 0=external SON pin, 1=always ON
	ParamDI1Func        uint16 = 100 // P-100: DI1 function (1=SON, 10=SP1, 11=SP2…)
	ParamDI2Func        uint16 = 101 // P-101: DI2 function
	ParamDI3Func        uint16 = 102 // P-102: DI3 function
	ParamDI4Func        uint16 = 103 // P-103: DI4 function
	ParamDO1Func        uint16 = 108 // P-108: DO1 function (2=RDY, 3=ALM, 5=COIN…)
	ParamDO2Func        uint16 = 109 // P-109: DO2 function
	ParamDO3Func        uint16 = 110 // P-110: DO3 function
	ParamDO4Func        uint16 = 111 // P-111: DO4 function
	ParamInternalSpd1   uint16 = 137 // P-137: internal speed preset 1 (rpm)
	ParamInternalSpd2   uint16 = 138 // P-138: internal speed preset 2 (rpm)
	ParamInternalSpd3   uint16 = 139 // P-139: internal speed preset 3 (rpm)
	ParamInternalSpd4   uint16 = 140 // P-140: internal speed preset 4 (rpm)
	ParamInternalSpd5   uint16 = 141 // P-141: internal speed preset 5 (rpm)
	ParamInternalSpd6   uint16 = 142 // P-142: internal speed preset 6 (rpm)
	ParamInternalSpd7   uint16 = 143 // P-143: internal speed preset 7 (rpm)
	ParamInternalSpd8   uint16 = 144 // P-144: internal speed preset 8 (rpm)
	ParamEncoderLines   uint16 = 172 // P-172: encoder lines per rev (default=2500 → 10000 ppr)
	ParamEncoderBits    uint16 = 184 // P-184: encoder resolution (17 or 23 bit)
	ParamSlaveID        uint16 = 181 // P-181: Modbus slave ID (1..32; -1=disabled)
	ParamBaudRate       uint16 = 182 // P-182: 0=4800, 1=9600, 2=19200, 3=38400, 4=57600, 5=115200
	ParamDataFormat     uint16 = 183 // P-183: 0=8N1, 1=8E1, 2=8O1, 3=8N2, 4=8E2, 5=8O2
	ParamPolePairs      uint16 = 201 // P-201: motor pole pairs
	ParamRatedCurrent   uint16 = 204 // P-204: rated current (A)
	ParamRatedSpeed     uint16 = 207 // P-207: rated speed (rpm)
)

// FC04 — Input Registers (read-only). Address range 0x0000..0x0028.
const (
	StatusSpeed          uint16 = 0x00 // motor speed (rpm)
	StatusPulseRefL      uint16 = 0x01 // input pulse position command, low word
	StatusPulseRefH      uint16 = 0x02 // input pulse position command, high word
	StatusPosRefL        uint16 = 0x03 // position command (pulses), low word
	StatusPosRefH        uint16 = 0x04 // position command (pulses), high word
	StatusPositionL      uint16 = 0x05 // current motor position (pulses), low word
	StatusPositionH      uint16 = 0x06 // current motor position (pulses), high word
	StatusDeviationL     uint16 = 0x07 // position deviation (pulses), low word
	StatusDeviationH     uint16 = 0x08 // position deviation (pulses), high word
	StatusTorquePct      uint16 = 0x09 // motor torque (% of rated)
	StatusPeakTorque     uint16 = 0x0A // peak torque in 1s (%)
	StatusCurrentA10     uint16 = 0x0B // instantaneous current (A×10; 12 = 1.2A)
	StatusPeakCurrentA10 uint16 = 0x0C // peak current in 1s (A×10)
	StatusPulseFreqKHz10 uint16 = 0x0D // pulse cmd frequency (×0.1 kHz; 3000 = 300 kHz)
	StatusSpeedRef       uint16 = 0x0E // speed setpoint (rpm)
	StatusTorqueRef      uint16 = 0x0F // torque setpoint (%)
	StatusSpeedVoltmV    uint16 = 0x10 // analog speed cmd voltage (mV)
	StatusTorqueVoltmV   uint16 = 0x11 // analog torque cmd voltage (mV)
	StatusDIState        uint16 = 0x12 // DI pin states: bit0=DI1 … bit7=DI8
	StatusDOState        uint16 = 0x13 // DO pin states: bit0=DO1 … bit5=DO6
	StatusEncAbsPosL     uint16 = 0x14 // abs encoder position within one rev, low
	StatusEncAbsPosH     uint16 = 0x15 // abs encoder position within one rev, high
	StatusEncMultiTurn   uint16 = 0x16 // multi-turn encoder count (0 if single-turn)
	StatusBrakePct       uint16 = 0x17 // braking resistor load (%)
	StatusAvgLoadPct     uint16 = 0x18 // average load (%)
	StatusOutputVoltPct  uint16 = 0x19 // output voltage (%)
	StatusFaultCode      uint16 = 0x1A // fault code (0 = no fault)
	StatusSpeedPrecise10 uint16 = 0x1B // precise speed (×0.1 rpm)
	StatusEnc2PosL       uint16 = 0x1C // 2nd encoder position, low word
	StatusEnc2PosH       uint16 = 0x1D // 2nd encoder position, high word
	StatusAbsMotorPosL   uint16 = 0x1F // absolute motor position (32-bit), low word
	StatusAbsMotorPosH   uint16 = 0x20 // absolute motor position (32-bit), high word
	StatusHeatsinkTempC  uint16 = 0x26 // heatsink temperature (°C)
	StatusModuleTempC    uint16 = 0x27 // module temperature (°C)
	StatusBusVoltageV    uint16 = 0x28 // DC bus voltage (V; nominal ≈310V)
)

// P-004 control mode values.
const (
	ModePosition uint16 = 0 // pulse/step hardware input
	ModeSpeed    uint16 = 1 // speed control
	ModeTorque   uint16 = 2 // torque control
)

// P-025 speed source values.
const (
	SpeedSourceAnalog   uint16 = 0 // analog voltage on speed input
	SpeedSourceInternal uint16 = 1 // internal multi-speed presets P-137..P-144
	SpeedSourcePulse    uint16 = 3 // pulse frequency input
)
