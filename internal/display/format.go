package display

import (
	"fmt"

	"net_proxy_tools/internal/core"
)

// Verbosity levels for frame display (inspired by mitmdump --flow-detail).
const (
	LevelSilent  = 0 // no output
	LevelSummary = 1 // connection events only
	LevelFrames  = 2 // one line per frame: direction, type, size
	LevelPreview = 3 // frame + truncated payload
	LevelFull    = 4 // frame + full payload dump
)

// OpcodeNames maps WebSocket opcodes to short display names.
var OpcodeNames = map[uint8]string{
	1:  "TEXT",
	2:  "BIN",
	8:  "CLOSE",
	9:  "PING",
	10: "PONG",
}

// FormatFrame renders a single WebSocket frame as a terminal-friendly string.
// The level parameter controls verbosity (see Level* constants).
//
// TODO(human): Implement the core formatting logic for levels 2-4.
//
// For reference, here are the target outputs from the design spec:
//
// Level 2 (-v):    → TEXT    13B
// Level 3 (-vv):   → TEXT    13B │ Hello, World!
// Level 4 (-vvv):  → TEXT 13B\n  Hello, World!  (or hex dump for binary)
//
// Design decisions to make:
//   - Direction indicator: → / ← are nice but may not render in all terminals.
//     Alternatives: > / <, ▶ / ◀, C> / S> (client/server)
//   - Size formatting: raw bytes "4218B" vs human "4.2KB"?
//   - Payload truncation for level 3: truncate to terminal width? fixed 60 chars?
//   - Binary at level 3: "[binary 4,218 bytes]" vs first 16 bytes as hex?
//   - Control frames: show "PING 0B" or just "PING"?
func FormatFrame(f *core.Frame, level int) string {
	if level <= LevelSummary {
		return ""
	}

	// TODO(human): replace the placeholder below with your formatting logic
	return fmt.Sprintf("%d %d %dB", f.Direction, f.Opcode, len(f.Payload))
}

// FormatConnectionEvent renders connection lifecycle events.
// Used at all verbosity levels >= LevelSummary.
func FormatConnectionEvent(event string, target string) string {
	return fmt.Sprintf("── %s %s", event, target)
}
