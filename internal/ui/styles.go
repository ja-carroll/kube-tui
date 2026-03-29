package ui

import "github.com/charmbracelet/lipgloss"

// Defining styles as package-level variables is the Lip Gloss convention.
// Each style is immutable — methods like .Border() return a NEW style,
// they don't mutate the original. This is similar to how strings work in Go.

// Color palette
var (
	subtle    = lipgloss.Color("#626262")
	highlight = lipgloss.Color("#7D56F4")
	special   = lipgloss.Color("#73F59F")
	white     = lipgloss.Color("#FFFFFF")
	green     = lipgloss.Color("#73F59F")
	yellow    = lipgloss.Color("#FDFF90")
	red       = lipgloss.Color("#FF6B6B")
)

// Panel styles
var (
	// The base style for panels. We'll copy this and customize per-panel.
	// .Border() adds a border — lipgloss has several built-in border styles:
	// NormalBorder, RoundedBorder, ThickBorder, DoubleBorder, etc.
	panelStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(subtle).
			Padding(1, 2)

	// Active panel gets a highlighted border so you can see which pane has focus.
	activePanelStyle = lipgloss.NewStyle().
				Border(lipgloss.RoundedBorder()).
				BorderForeground(highlight).
				Padding(1, 2)

	// Title styling for panel headers
	titleStyle = lipgloss.NewStyle().
			Foreground(highlight).
			Bold(true).
			MarginBottom(1)

	// The cursor line when an item is selected
	selectedItemStyle = lipgloss.NewStyle().
				Foreground(special).
				Bold(true)

	// Normal (unselected) items
	itemStyle = lipgloss.NewStyle().
			Foreground(white)

	// Dimmed items (e.g., inactive pane cursor)
	dimmedItemStyle = lipgloss.NewStyle().
			Foreground(subtle)

	// The help bar at the bottom
	helpStyle = lipgloss.NewStyle().
			Foreground(subtle).
			MarginTop(1)

	// Detail panel key labels (left column)
	detailKeyStyle = lipgloss.NewStyle().
			Foreground(highlight).
			Bold(true)

	// Detail panel values (right column)
	detailValStyle = lipgloss.NewStyle().
			Foreground(white)

	// Search bar
	searchBarStyle = lipgloss.NewStyle().
			Foreground(special).
			Bold(true)

	// Search cursor (the blinking block)
	searchCursorStyle = lipgloss.NewStyle().
				Foreground(special)

	// Health status row styles
	healthOKStyle = lipgloss.NewStyle().
			Foreground(green)

	healthWarningStyle = lipgloss.NewStyle().
				Foreground(yellow)

	healthErrorStyle = lipgloss.NewStyle().
				Foreground(red)

	// Table header style
	tableHeaderStyle = lipgloss.NewStyle().
				Foreground(highlight).
				Bold(true)
)
