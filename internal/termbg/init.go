package termbg

import "github.com/charmbracelet/lipgloss"

func init() {
	// Bubble Tea's package init calls lipgloss.HasDarkBackground, which
	// makes termenv ask the terminal for its background and wait up to
	// five seconds for an answer: every skd command would hang that long
	// on a terminal that does not answer (`script`, some embedded
	// consoles, CI with a pty). Setting the value first turns that query
	// into a no-op. This init runs before Bubble Tea's because Go
	// initialises ready packages in import-path order and
	// github.com/Maybe404/… sorts before github.com/charmbracelet/…;
	// TestNoQueryBeforeDetect in internal/cli checks it on a real pty.
	// The CLI then asks with Detect, which gives up after Timeout.
	lipgloss.DefaultRenderer().SetHasDarkBackground(true)
}
