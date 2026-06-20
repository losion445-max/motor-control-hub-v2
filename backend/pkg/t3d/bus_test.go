package t3d

import (
	"bytes"
	"testing"
)

func TestCRC16Modbus(t *testing.T) {
	t.Run("empty input", func(t *testing.T) {
		got := crc16modbus([]byte{})
		if got != 0xFFFF {
			t.Errorf("crc16modbus([]) = 0x%04X, want 0xFFFF", got)
		}
	})

	t.Run("known frame FC03 read 1 register", func(t *testing.T) {
		// Slave 1, FC03, addr 0x0000, count 1 → CRC 0x0A84
		frame := []byte{0x01, 0x03, 0x00, 0x00, 0x00, 0x01}
		got := crc16modbus(frame)
		if got != 0x0A84 {
			t.Errorf("crc16modbus = 0x%04X, want 0x0A84", got)
		}
	})

	t.Run("consistency: same input same output", func(t *testing.T) {
		data := []byte{0x01, 0x06, 0x00, 0x89, 0x00, 0x01}
		a := crc16modbus(data)
		b := crc16modbus(data)
		if a != b {
			t.Errorf("crc not deterministic: 0x%04X != 0x%04X", a, b)
		}
	})

	t.Run("single byte", func(t *testing.T) {
		// Known: CRC of [0x01] with Modbus polynomial
		got := crc16modbus([]byte{0x01})
		// Verify it doesn't panic and produces a stable result.
		got2 := crc16modbus([]byte{0x01})
		if got != got2 {
			t.Errorf("single-byte CRC not deterministic")
		}
	})
}

func TestBuildADU(t *testing.T) {
	t.Run("FC03 read 1 register", func(t *testing.T) {
		// buildADU(1, 3, [0x00,0x00,0x00,0x01]) →
		// CRC of [01 03 00 00 00 01] = 0x0A84
		// RTU appends lo first: [... 0x84, 0x0A]
		data := []byte{0x00, 0x00, 0x00, 0x01}
		got := buildADU(1, 3, data)
		want := []byte{0x01, 0x03, 0x00, 0x00, 0x00, 0x01, 0x84, 0x0A}
		if !bytes.Equal(got, want) {
			t.Errorf("buildADU = % X, want % X", got, want)
		}
	})

	t.Run("nil data produces header plus CRC only", func(t *testing.T) {
		got := buildADU(2, 0x42, nil)
		// Length must be exactly 4: slaveID + fc + 2 CRC bytes
		if len(got) != 4 {
			t.Errorf("buildADU with nil data: len=%d, want 4", len(got))
		}
		if got[0] != 2 || got[1] != 0x42 {
			t.Errorf("buildADU header wrong: % X", got[:2])
		}
		// Verify embedded CRC matches recomputed value.
		crc := crc16modbus(got[:2])
		if got[2] != byte(crc) || got[3] != byte(crc>>8) {
			t.Errorf("buildADU CRC mismatch in nil-data frame")
		}
	})

	t.Run("CRC bytes are little-endian", func(t *testing.T) {
		// RTU appends CRC lo byte first, hi byte second.
		// CRC of [01 03 00 00 00 01] = 0x0A84, so lo=0x84 hi=0x0A.
		adu := buildADU(1, 3, []byte{0x00, 0x00, 0x00, 0x01})
		if adu[len(adu)-2] != 0x84 || adu[len(adu)-1] != 0x0A {
			t.Errorf("CRC byte order wrong: got %02X %02X, want 84 0A",
				adu[len(adu)-2], adu[len(adu)-1])
		}
	})

	t.Run("data is not modified", func(t *testing.T) {
		data := []byte{0xAA, 0xBB}
		original := []byte{0xAA, 0xBB}
		buildADU(1, 6, data)
		if !bytes.Equal(data, original) {
			t.Errorf("buildADU modified input data slice")
		}
	})
}
