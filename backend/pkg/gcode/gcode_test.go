package gcode

import (
	"math"
	"testing"
)

func TestParse_Empty(t *testing.T) {
	cmds, err := Parse("")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(cmds) != 0 {
		t.Errorf("got %d commands for empty input, want 0", len(cmds))
	}
}

func TestParse_CommentsOnly(t *testing.T) {
	cases := []string{
		"; full line comment",
		"   ; leading space comment",
		"(block comment)",
		"; first\n; second\n\n   ",
	}
	for _, src := range cases {
		cmds, err := Parse(src)
		if err != nil {
			t.Errorf("Parse(%q) error: %v", src, err)
		}
		if len(cmds) != 0 {
			t.Errorf("Parse(%q): got %d cmds, want 0", src, len(cmds))
		}
	}
}

func TestParse_G28(t *testing.T) {
	cmds, err := Parse("G28")
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	if len(cmds) != 1 {
		t.Fatalf("got %d commands, want 1", len(cmds))
	}
	c := cmds[0]
	if c.Motion != Home {
		t.Errorf("Motion = %v, want Home", c.Motion)
	}
	if c.HasX() {
		t.Errorf("G28 should not have X")
	}
	if c.HasY() {
		t.Errorf("G28 should not have Y")
	}
}

func TestParse_G0(t *testing.T) {
	cmds, err := Parse("G0 X100 Y200")
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	if len(cmds) != 1 {
		t.Fatalf("got %d commands, want 1", len(cmds))
	}
	c := cmds[0]
	if c.Motion != Rapid {
		t.Errorf("Motion = %v, want Rapid", c.Motion)
	}
	if c.X != 100 {
		t.Errorf("X = %v, want 100", c.X)
	}
	if c.Y != 200 {
		t.Errorf("Y = %v, want 200", c.Y)
	}
}

func TestParse_G1WithF(t *testing.T) {
	cmds, err := Parse("G1 X100 Y200 F300")
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	if len(cmds) != 1 {
		t.Fatalf("got %d commands, want 1", len(cmds))
	}
	c := cmds[0]
	if c.Motion != Linear {
		t.Errorf("Motion = %v, want Linear", c.Motion)
	}
	if c.X != 100 || c.Y != 200 || c.F != 300 {
		t.Errorf("X=%v Y=%v F=%v, want 100 200 300", c.X, c.Y, c.F)
	}
}

func TestParse_ModalG1(t *testing.T) {
	// G1 set on first line, subsequent lines with X/Y but no G word inherit it.
	src := "G1\nX100 Y200\nX300 Y400"
	cmds, err := Parse(src)
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	// "G1" alone emits nothing (no X/Y/F); lines 2 and 3 emit as Linear.
	if len(cmds) != 2 {
		t.Fatalf("got %d commands, want 2 (modal G1 lines)", len(cmds))
	}
	for i, c := range cmds {
		if c.Motion != Linear {
			t.Errorf("cmd[%d].Motion = %v, want Linear", i, c.Motion)
		}
	}
	if cmds[0].X != 100 || cmds[0].Y != 200 {
		t.Errorf("cmd[0]: X=%v Y=%v, want 100 200", cmds[0].X, cmds[0].Y)
	}
	if cmds[1].X != 300 || cmds[1].Y != 400 {
		t.Errorf("cmd[1]: X=%v Y=%v, want 300 400", cmds[1].X, cmds[1].Y)
	}
}

func TestParse_FOnlyLine(t *testing.T) {
	// Feed-rate-only lines must be emitted so the runner can pick up the change.
	src := "G1 F600"
	cmds, err := Parse(src)
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	if len(cmds) != 1 {
		t.Fatalf("got %d commands, want 1 (F-only)", len(cmds))
	}
	c := cmds[0]
	if c.Motion != Linear {
		t.Errorf("Motion = %v, want Linear", c.Motion)
	}
	if c.F != 600 {
		t.Errorf("F = %v, want 600", c.F)
	}
	if c.HasX() || c.HasY() {
		t.Errorf("F-only line should have no X or Y")
	}
}

func TestParse_CompactSyntax(t *testing.T) {
	cmds, err := Parse("G1X100Y200F300")
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	if len(cmds) != 1 {
		t.Fatalf("got %d commands, want 1", len(cmds))
	}
	c := cmds[0]
	if c.Motion != Linear || c.X != 100 || c.Y != 200 || c.F != 300 {
		t.Errorf("compact: got Motion=%v X=%v Y=%v F=%v, want Linear 100 200 300",
			c.Motion, c.X, c.Y, c.F)
	}
}

func TestParse_MixedCase(t *testing.T) {
	cmds, err := Parse("g1 x100 y200")
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	if len(cmds) != 1 {
		t.Fatalf("got %d commands, want 1", len(cmds))
	}
	if cmds[0].X != 100 || cmds[0].Y != 200 {
		t.Errorf("got X=%v Y=%v, want 100 200", cmds[0].X, cmds[0].Y)
	}
}

func TestParse_InlineComment(t *testing.T) {
	// Text after ';' should be ignored, including Y200.
	cmds, err := Parse("G0 X100 ; go here Y200")
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	if len(cmds) != 1 {
		t.Fatalf("got %d commands, want 1", len(cmds))
	}
	c := cmds[0]
	if c.X != 100 {
		t.Errorf("X = %v, want 100", c.X)
	}
	if c.HasY() {
		t.Errorf("Y should not be set (was inside comment)")
	}
}

func TestParse_BlockComment(t *testing.T) {
	// Text inside (...) should be ignored; Y300 outside the block should be kept.
	cmds, err := Parse("G0 X100 (skip this Y200) Y300")
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	if len(cmds) != 1 {
		t.Fatalf("got %d commands, want 1", len(cmds))
	}
	c := cmds[0]
	if c.X != 100 {
		t.Errorf("X = %v, want 100", c.X)
	}
	if c.Y != 300 {
		t.Errorf("Y = %v, want 300", c.Y)
	}
}

func TestParse_UnknownGCode(t *testing.T) {
	// Unknown G codes should be silently ignored, no error.
	cmds, err := Parse("G5 X100\nG90\nG0 X50 Y50")
	if err != nil {
		t.Fatalf("unknown G code caused error: %v", err)
	}
	// G5 X100: G5 unknown but X present → depends on modal mode (Rapid by default).
	// G90: no X/Y/F → not emitted.
	// G0 X50 Y50 → emitted.
	// Find the G0 X50 Y50 command.
	found := false
	for _, c := range cmds {
		if c.X == 50 && c.Y == 50 {
			found = true
		}
	}
	if !found {
		t.Errorf("G0 X50 Y50 not found in: %+v", cmds)
	}
}

func TestParse_LineNumber(t *testing.T) {
	cmds, err := Parse("N10 G0 X100 Y200")
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	if len(cmds) != 1 {
		t.Fatalf("got %d commands, want 1", len(cmds))
	}
	if cmds[0].X != 100 || cmds[0].Y != 200 {
		t.Errorf("got X=%v Y=%v, want 100 200", cmds[0].X, cmds[0].Y)
	}
}

func TestParse_NegativeCoords(t *testing.T) {
	cmds, err := Parse("G0 X-100 Y-200")
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	if len(cmds) != 1 {
		t.Fatalf("got %d commands, want 1", len(cmds))
	}
	if cmds[0].X != -100 || cmds[0].Y != -200 {
		t.Errorf("got X=%v Y=%v, want -100 -200", cmds[0].X, cmds[0].Y)
	}
}

func TestParse_HasXHasY(t *testing.T) {
	cmds, err := Parse("G0 X100")
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	if len(cmds) != 1 {
		t.Fatalf("got %d commands", len(cmds))
	}
	c := cmds[0]
	if !c.HasX() {
		t.Errorf("HasX() = false, want true")
	}
	if c.HasY() {
		t.Errorf("HasY() = true, want false")
	}
	if !math.IsNaN(c.Y) {
		t.Errorf("unset Y should be NaN, got %v", c.Y)
	}
}

func TestParse_FullProgram(t *testing.T) {
	prog := `
; Home and run a short path
G28
G0 X700 Y1200
G1 F600
X350 Y600
X1050 Y1800
G0 X700 Y1200
`
	cmds, err := Parse(prog)
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	// Expected: G28, G0, G1(F-only), G1(X350 Y600), G1(X1050 Y1800), G0
	if len(cmds) != 6 {
		t.Fatalf("got %d commands, want 6: %+v", len(cmds), cmds)
	}
	if cmds[0].Motion != Home {
		t.Errorf("cmd[0] should be Home, got %v", cmds[0].Motion)
	}
	if cmds[1].Motion != Rapid || cmds[1].X != 700 || cmds[1].Y != 1200 {
		t.Errorf("cmd[1] should be G0 X700 Y1200, got %+v", cmds[1])
	}
	if cmds[2].Motion != Linear || cmds[2].F != 600 {
		t.Errorf("cmd[2] should be G1 F600, got %+v", cmds[2])
	}
	if cmds[3].Motion != Linear || cmds[3].X != 350 || cmds[3].Y != 600 {
		t.Errorf("cmd[3] should be G1 X350 Y600, got %+v", cmds[3])
	}
}
