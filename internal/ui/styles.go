package ui

import lipgloss "charm.land/lipgloss/v2"

// Color palette — a richer set for visual hierarchy.
var (
	// Base greys
	subtle  = lipgloss.Color("#626262")
	dimGrey = lipgloss.Color("#4A4A4A")

	// Brand / accent colors
	highlight = lipgloss.Color("#7D56F4") // purple — primary accent
	special   = lipgloss.Color("#73F59F") // green — selected items
	cyan      = lipgloss.Color("#00D7FF") // cyan — secondary accent
	white     = lipgloss.Color("#FFFFFF")
	cream     = lipgloss.Color("#EEEEEE") // softer than pure white

	// Status colors
	green  = lipgloss.Color("#73F59F")
	yellow = lipgloss.Color("#FDFF90")
	red    = lipgloss.Color("#FF6B6B")

	// Header / branding
	headerPurple = lipgloss.Color("#9B72FF")
	headerBg     = lipgloss.Color("#1A1A2E")
)

// Unicode symbols — these small touches make a big visual difference.
// Go source files are UTF-8 by default, so you can use Unicode literals
// directly in your code. No encoding declarations needed (unlike Python 2).
const (
	symbolCursor   = "▸"
	symbolSelected = "◆"
	symbolDot      = "●"
	symbolArrow    = "→"
	symbolCheck    = "✓"
	symbolCross    = "✗"
	symbolK8s      = "⎈"
)

// Header bar — shown at the top of the screen with cluster info.
var (
	headerStyle = lipgloss.NewStyle().
			Background(headerBg).
			Foreground(headerPurple).
			Bold(true).
			Padding(0, 2)

	headerClusterStyle = lipgloss.NewStyle().
				Background(headerBg).
				Foreground(cyan).
				Bold(true)

	headerDimStyle = lipgloss.NewStyle().
			Background(headerBg).
			Foreground(subtle)
)

// Panel styles
var (
	panelStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(dimGrey).
			Padding(1, 2)

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
			Foreground(cream)

	// Dimmed items
	dimmedItemStyle = lipgloss.NewStyle().
			Foreground(subtle)

	// The help bar at the bottom
	helpStyle = lipgloss.NewStyle().
			Foreground(subtle)

	// Detail panel
	detailKeyStyle = lipgloss.NewStyle().
			Foreground(cyan).
			Bold(true)

	detailValStyle = lipgloss.NewStyle().
			Foreground(cream)

	// Search bar
	searchBarStyle = lipgloss.NewStyle().
			Foreground(special).
			Bold(true)

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
				Foreground(cyan).
				Bold(true)

	// Action menu styles
	actionMenuStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(highlight).
			Padding(1, 2).
			Background(lipgloss.Color("#1A1A2E"))

	actionMenuTitleStyle = lipgloss.NewStyle().
				Foreground(highlight).
				Bold(true).
				MarginBottom(1)

	actionKeyStyle = lipgloss.NewStyle().
			Foreground(special).
			Bold(true)

	actionDescStyle = lipgloss.NewStyle().
			Foreground(cream)

	actionDimStyle = lipgloss.NewStyle().
			Foreground(subtle)
)
