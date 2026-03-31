package ui

import (
	"fmt"
	"strings"

	lipgloss "charm.land/lipgloss/v2"
)

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

	// Counter style for "X of Y" in the bottom border
	counterStyle = lipgloss.NewStyle().
			Foreground(subtle)
)

// renderPanel draws a box with a title embedded in the top border and an
// optional counter in the bottom-right border, like lazygit does.
//
// Example output:
//
//	╭─Namespaces──────────────╮
//	│  default                │
//	│  kube-system            │
//	╰────────────────1 of 4──╯
//
// We draw the border manually because lipgloss's built-in Border() doesn't
// support embedding text in the border line.
func renderPanel(title, content string, width, height int, active bool, cursor, total int) string {
	borderColor := dimGrey
	titleColor := subtle
	if active {
		borderColor = highlight
		titleColor = highlight
	}

	// Characters for rounded box-drawing
	const (
		topLeft     = "╭"
		topRight    = "╮"
		bottomLeft  = "╰"
		bottomRight = "╯"
		horizontal  = "─"
		vertical    = "│"
	)

	bc := lipgloss.NewStyle().Foreground(borderColor)
	tc := lipgloss.NewStyle().Foreground(titleColor).Bold(true)

	innerWidth := width - 2 // subtract left + right border chars

	// Top border: ╭─ Title ────────────╮
	titleText := tc.Render(title)
	titleLen := lipgloss.Width(titleText)
	topPadding := innerWidth - titleLen - 3 // -1 leading ─, -2 for spaces around title
	if topPadding < 0 {
		topPadding = 0
	}
	topBorder := bc.Render(topLeft+horizontal+" ") + titleText + bc.Render(" "+strings.Repeat(horizontal, topPadding)+topRight)

	// Bottom border: ╰───────── 1 of 4 ─╯
	counter := ""
	if total > 0 {
		counter = fmt.Sprintf("%d of %d", cursor+1, total)
	} else {
		counter = "0 of 0"
	}
	counterText := counterStyle.Render(counter)
	counterLen := lipgloss.Width(counterText)
	bottomPadding := innerWidth - counterLen - 3 // -1 trailing ─, -2 for spaces around counter
	if bottomPadding < 0 {
		bottomPadding = 0
	}
	bottomBorder := bc.Render(bottomLeft+strings.Repeat(horizontal, bottomPadding)+" ") + counterText + bc.Render(" "+horizontal+bottomRight)

	// Content lines — pad each to innerWidth and add border chars
	contentLines := strings.Split(content, "\n")

	// Pad or trim to fill the height (subtract 2 for top/bottom borders)
	contentHeight := height - 2
	for len(contentLines) < contentHeight {
		contentLines = append(contentLines, "")
	}
	if len(contentLines) > contentHeight {
		contentLines = contentLines[:contentHeight]
	}

	var body strings.Builder
	for _, line := range contentLines {
		lineWidth := lipgloss.Width(line)
		pad := innerWidth - lineWidth - 2 // -2 for left/right padding
		if pad < 0 {
			pad = 0
		}
		body.WriteString(bc.Render(vertical) + " " + line + strings.Repeat(" ", pad) + " " + bc.Render(vertical) + "\n")
	}

	return topBorder + "\n" + body.String() + bottomBorder
}
