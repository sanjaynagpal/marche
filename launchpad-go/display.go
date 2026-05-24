package main

import (
	"fmt"
	"strings"
)

const (
	colorReset  = "\033[0m"
	colorGreen  = "\033[32m"
	colorYellow = "\033[33m"
	colorRed    = "\033[31m"

	// clearScreen moves the cursor to the top-left then erases the display.
	clearScreen = "\033[2J\033[H"

	// rulerChar is the box-drawing horizontal bar used for table dividers.
	rulerChar = "─"
)

// RenderTable clears the terminal and redraws the full status table followed
// by the command help line.
//
// Column layout (all widths are visual / rune counts, not byte counts):
//
//	" " + "#"(3) + "  " + Module(nameWidth) + "  " + Status(8) + "  " + Port(6) + "  " + PID(6)
func RenderTable(statuses []ServiceStatus, root, env string) {
	// Compute the minimum nameWidth that fits all module names.
	nameWidth := len("Module") // header minimum
	for _, s := range statuses {
		if n := len(s.Entry.Name); n > nameWidth {
			nameWidth = n
		}
	}

	// Build the header row first so we can derive the ruler width from it.
	// All header tokens are pure ASCII so len() == visual width.
	header := fmt.Sprintf(" %-3s  %-*s  %-8s  %-6s  %-6s",
		"#", nameWidth, "Module", "Status", "Port", "PID")
	// ruler must match the visual width of header.
	// rulerChar is a 3-byte UTF-8 sequence but renders as 1 column, so we
	// repeat it len(header) times to get the same visual column count.
	ruler := strings.Repeat(rulerChar, len(header))

	// Sub-header row: column-level dividers separated by single spaces.
	// Constructed manually to avoid fmt width padding of multi-byte chars.
	subHeader := fmt.Sprintf(" %s  %s  %s  %s  %s",
		strings.Repeat(rulerChar, 3),
		strings.Repeat(rulerChar, nameWidth),
		strings.Repeat(rulerChar, 8),
		strings.Repeat(rulerChar, 6),
		strings.Repeat(rulerChar, 6),
	)

	fmt.Print(clearScreen)
	fmt.Printf("Launchpad   root=%s   env=%s\n", root, env)
	fmt.Println(ruler)
	fmt.Println(header)
	fmt.Println(subHeader)

	for i, s := range statuses {
		pid := "—"
		if s.PID > 0 {
			pid = fmt.Sprintf("%d", s.PID)
		}
		port := s.Entry.Port
		if port == "" {
			port = "—"
		}
		// colorizeState returns an ANSI-wrapped string that is visually 8
		// columns wide; %s emits it without additional padding.
		fmt.Printf(" %-3d  %-*s  %s  %-6s  %s\n",
			i+1, nameWidth, s.Entry.Name,
			colorizeState(s.State), port, pid)
	}

	fmt.Println(ruler)
	fmt.Println("Commands: [s]tart <#>  [k]ill <#>  [r]efresh  [q]uit")
}

// PrintError prints a red-highlighted error message on its own line.
func PrintError(msg string) {
	fmt.Printf("%s! %s%s\n", colorRed, msg, colorReset)
}

// colorizeState returns state wrapped in ANSI colour codes and padded to
// exactly 8 visual columns so the Status column aligns in all rows.
//
//	RUNNING → green
//	STOPPED → yellow
func colorizeState(state string) string {
	switch state {
	case "RUNNING":
		return fmt.Sprintf("%s%-8s%s", colorGreen, state, colorReset)
	case "STOPPED":
		return fmt.Sprintf("%s%-8s%s", colorYellow, state, colorReset)
	default:
		return fmt.Sprintf("%-8s", state)
	}
}
