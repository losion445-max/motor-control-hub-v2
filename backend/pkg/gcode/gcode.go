// Package gcode parses a subset of RS-274/NGC G-code into a flat list of
// motion commands. It has no dependencies on hardware or robot packages.
//
// Supported codes:
//
//	G0  — rapid positioning (fastest available speed)
//	G1  — linear interpolation at the active feed rate (F, mm/min)
//	G28 — return to home
//
// Words: X (mm), Y (mm), F (mm/min), N (line number, ignored).
// All other G/M/S/T codes are silently skipped (forward-compatible).
// Comments: `;` through end of line, or `(` … `)` blocks.
//
// G0 and G1 are modal — the last active code applies to subsequent lines
// that contain X/Y but no new G word.
//
// Both spaced (`G1 X100 Y200`) and compact (`G1X100Y200`) syntax are accepted.
package gcode

import (
	"bufio"
	"fmt"
	"math"
	"strconv"
	"strings"
)

// Motion is the type of motion for a command.
type Motion int

const (
	Rapid  Motion = 0  // G0: move at maximum speed; path is not guaranteed straight
	Linear Motion = 1  // G1: straight-line move at the commanded feed rate
	Home   Motion = 28 // G28: return to the machine home position
)

// Cmd is a single resolved G-code motion command.
// Fields X and Y are NaN when the axis was not specified on that line.
// F is 0 when the feed rate was not updated on that line.
type Cmd struct {
	Motion Motion
	X, Y   float64 // target position (mm); math.NaN() if not specified
	F      float64 // feed rate (mm/min); 0 = keep current rate
}

// HasX reports whether X was specified on this command's source line.
func (c Cmd) HasX() bool { return !math.IsNaN(c.X) }

// HasY reports whether Y was specified on this command's source line.
func (c Cmd) HasY() bool { return !math.IsNaN(c.Y) }

// Parse parses a complete G-code program and returns the list of motion
// commands in order. Only G0, G1, and G28 commands with at least one of
// X or Y (or G28 with neither) are emitted; other words are ignored.
func Parse(src string) ([]Cmd, error) {
	var cmds []Cmd
	curMotion := Rapid // modal: starts in G0 mode

	scanner := bufio.NewScanner(strings.NewReader(src))
	lineNum := 0

	for scanner.Scan() {
		lineNum++
		line := strip(scanner.Text())
		if line == "" {
			continue
		}

		words, err := tokenize(line)
		if err != nil {
			return nil, fmt.Errorf("gcode line %d: %w", lineNum, err)
		}
		if len(words) == 0 {
			continue
		}

		cmd, nextMotion, err := interpret(words, curMotion)
		if err != nil {
			return nil, fmt.Errorf("gcode line %d: %w", lineNum, err)
		}
		if nextMotion >= 0 {
			curMotion = Motion(nextMotion)
		}

		// Emit if the command carries any useful payload.
		// F-only lines (e.g. "G1 F600") are emitted so the runner can pick
		// up the feed rate update even without a position change.
		if cmd.Motion == Home || cmd.HasX() || cmd.HasY() || cmd.F > 0 {
			cmds = append(cmds, cmd)
		}
	}

	return cmds, scanner.Err()
}

// ── internal ─────────────────────────────────────────────────────────────────

// word is a parsed G-code word (letter + number).
type word struct {
	Letter byte
	Value  float64
}

// strip removes `;` line comments and `(...)` block comments, then trims space.
func strip(line string) string {
	// Block comments (parentheses).
	for {
		open := strings.IndexByte(line, '(')
		if open < 0 {
			break
		}
		close := strings.IndexByte(line[open:], ')')
		if close < 0 {
			line = line[:open]
			break
		}
		line = line[:open] + " " + line[open+close+1:]
	}
	// Inline comment.
	if idx := strings.IndexByte(line, ';'); idx >= 0 {
		line = line[:idx]
	}
	return strings.TrimSpace(line)
}

// tokenize splits a G-code line into words.
// Each word is one letter followed by an optional sign and decimal number.
// Words may be separated by spaces or written compactly (G1X100Y200).
func tokenize(line string) ([]word, error) {
	line = strings.ToUpper(line)
	var words []word
	i := 0
	for i < len(line) {
		ch := line[i]
		if ch == ' ' || ch == '\t' {
			i++
			continue
		}
		if ch < 'A' || ch > 'Z' {
			i++ // skip unknown punctuation
			continue
		}
		letter := ch
		i++
		// Consume optional sign and digits.
		j := i
		if j < len(line) && (line[j] == '-' || line[j] == '+') {
			j++
		}
		for j < len(line) && (line[j] == '.' || (line[j] >= '0' && line[j] <= '9')) {
			j++
		}
		numStr := line[i:j]
		i = j

		if numStr == "" || numStr == "-" || numStr == "+" {
			// No number following letter — treat as 0 (handles plain "G28", "G0").
			numStr = "0"
		}
		val, err := strconv.ParseFloat(numStr, 64)
		if err != nil {
			return nil, fmt.Errorf("bad number after %c: %q", letter, numStr)
		}
		words = append(words, word{letter, val})
	}
	return words, nil
}

// interpret converts a list of words into a Cmd, given the current modal motion.
// Returns the new modal motion index (or -1 if unchanged).
func interpret(words []word, curMotion Motion) (Cmd, int, error) {
	cmd := Cmd{
		Motion: curMotion,
		X:      math.NaN(),
		Y:      math.NaN(),
	}
	nextMotion := -1

	for _, w := range words {
		switch w.Letter {
		case 'G':
			g := int(w.Value)
			switch g {
			case 0:
				nextMotion = int(Rapid)
				cmd.Motion = Rapid
			case 1:
				nextMotion = int(Linear)
				cmd.Motion = Linear
			case 28:
				nextMotion = int(Home)
				cmd.Motion = Home
			default:
				// Unknown G code — ignore (forward-compatible).
			}
		case 'X':
			cmd.X = w.Value
		case 'Y':
			cmd.Y = w.Value
		case 'F':
			cmd.F = w.Value
		case 'N':
			// Line number — ignore.
		// M, S, T, etc. — ignored.
		}
	}

	return cmd, nextMotion, nil
}
