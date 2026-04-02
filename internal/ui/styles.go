package ui

import (
	"fmt"
	"image/color"
	"strings"

	lipgloss "charm.land/lipgloss/v2"
)

// Color palette — rich gradient-friendly spectrum for a modern TUI.
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
	magenta   = lipgloss.Color("#FF6AD5") // hot pink — gradient midpoint

	// Status colors
	green  = lipgloss.Color("#73F59F")
	yellow = lipgloss.Color("#FDFF90")
	red    = lipgloss.Color("#FF6B6B")

	// Header / branding
	headerPurple = lipgloss.Color("#9B72FF")
	headerBg     = lipgloss.Color("#1A1A2E")

	// Gradient palette for active borders, logo, and accent rendering.
	// This purple → pink → cyan gradient is the signature Charm look.
	gradientPurple = lipgloss.Color("#AD6FFF") // bright lavender
	gradientPink   = lipgloss.Color("#FF6AD5") // hot pink
	gradientCyan   = lipgloss.Color("#00D7FF") // electric cyan

	// UI element accents
	keyBadgeBg = lipgloss.Color("#3A3A5C") // dark indigo for key hint badges
)

// Unicode symbols — these small touches make a big visual difference.
const (
	symbolCursor   = "▸"
	symbolSelected = "◆"
	symbolDot      = "●"
	symbolArrow    = "→"
	symbolCheck    = "✓"
	symbolCross    = "✗"
	symbolK8s      = "⎈"
)

// Resource type icons — unique geometric shapes that give each kind its own character.
var resourceIcons = map[resourceType]string{
	resourcePods:         "●", // dot — individual unit
	resourceDeployments:  "▲", // triangle — deploy upward
	resourceStatefulSets: "■", // square — solid, stateful
	resourceDaemonSets:   "◉", // target — runs everywhere
	resourceServices:     "○", // circle — network endpoint
	resourceIngresses:    "▷", // play — routing inbound
	resourceConfigMaps:   "≡", // triple bar — configuration
	resourceSecrets:      "◇", // diamond outline — hidden value
	resourceJobs:         "▶", // filled play — run once
	resourceCronJobs:     "↻", // cycle — recurring
	resourcePVCs:         "□", // open square — storage volume
}

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

	headerValStyle = lipgloss.NewStyle().
			Background(headerBg).
			Foreground(cream).
			Bold(true)
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

	// Key badge — dark background pill for keyboard shortcut hints
	keyBadgeStyle = lipgloss.NewStyle().
			Foreground(white).
			Background(keyBadgeBg).
			Bold(true).
			Padding(0, 1)

	// Key description — text after the badge
	keyDescStyle = lipgloss.NewStyle().
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

	// Table header style — underlined for a clean separation from data rows
	tableHeaderStyle = lipgloss.NewStyle().
				Foreground(cyan).
				Bold(true).
				Underline(true)

	// Action menu styles — gradient border for the floating overlay
	actionMenuStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForegroundBlend(gradientPurple, gradientPink, gradientCyan).
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

// gradientString renders each rune of s with a color from the gradient,
// starting at the given offset position. This is used to paint border
// characters with a flowing color transition.
func gradientString(s string, gradient []color.Color, offset int) string {
	var b strings.Builder
	for i, r := range s {
		idx := offset + i
		if idx < 0 {
			idx = 0
		}
		if idx >= len(gradient) {
			idx = len(gradient) - 1
		}
		b.WriteString(lipgloss.NewStyle().Foreground(gradient[idx]).Render(string(r)))
	}
	return b.String()
}

// renderGradientSep renders a horizontal separator line with a gradient.
func renderGradientSep(width int) string {
	if width <= 0 {
		return ""
	}
	gradient := lipgloss.Blend1D(width, gradientPurple, gradientPink, gradientCyan)
	return gradientString(strings.Repeat("─", width), gradient, 0)
}

// keyHint renders a key badge with a description for the help bar.
// The key gets a dark background pill, making it look like a physical key.
func keyHint(key, desc string) string {
	return keyBadgeStyle.Render(key) + " " + keyDescStyle.Render(desc)
}

// renderPanel draws a box with a title embedded in the top border and an
// optional counter in the bottom-right border, like lazygit does.
//
// Active panels get gradient borders (purple → pink → cyan) that flow
// across the box-drawing characters. Inactive panels use flat grey.
func renderPanel(title, content string, width, height int, active bool, cursor, total int) string {
	const (
		topLeft     = "╭"
		topRight    = "╮"
		bottomLeft  = "╰"
		bottomRight = "╯"
		horizontal  = "─"
		vertical    = "│"
	)

	innerWidth := width - 2

	// Title styling — bright white for active (contrast against gradient), subtle for inactive
	var tc lipgloss.Style
	if active {
		tc = lipgloss.NewStyle().Foreground(white).Bold(true)
	} else {
		tc = lipgloss.NewStyle().Foreground(subtle).Bold(true)
	}
	titleText := tc.Render(title)
	titleVisualWidth := lipgloss.Width(titleText)

	// Counter text — cream for active, subtle for inactive
	counter := "0 of 0"
	if total > 0 {
		counter = fmt.Sprintf("%d of %d", cursor+1, total)
	}
	var counterText string
	if active {
		counterText = lipgloss.NewStyle().Foreground(cream).Render(counter)
	} else {
		counterText = counterStyle.Render(counter)
	}
	counterVisualWidth := lipgloss.Width(counterText)

	// Padding calculations
	topPadding := innerWidth - titleVisualWidth - 3
	if topPadding < 0 {
		topPadding = 0
	}
	bottomPadding := innerWidth - counterVisualWidth - 3
	if bottomPadding < 0 {
		bottomPadding = 0
	}

	// Content lines — pad or trim to fill the height
	contentLines := strings.Split(content, "\n")
	contentHeight := height - 2
	for len(contentLines) < contentHeight {
		contentLines = append(contentLines, "")
	}
	if len(contentLines) > contentHeight {
		contentLines = contentLines[:contentHeight]
	}

	var topBorder, bottomBorder string
	var body strings.Builder

	if active {
		// ─── GRADIENT BORDERS ───
		// Generate a horizontal gradient across the full border width
		// and a vertical gradient for the side bars.
		totalW := innerWidth + 2
		hGradient := lipgloss.Blend1D(max(1, totalW), gradientPurple, gradientPink, gradientCyan)
		vGradient := lipgloss.Blend1D(max(1, contentHeight), gradientPurple, gradientCyan)

		// Top border: ╭─ Title ────────────╮
		topPrefix := topLeft + horizontal + " "
		topSuffix := " " + strings.Repeat(horizontal, topPadding) + topRight
		topBorder = gradientString(topPrefix, hGradient, 0) +
			titleText +
			gradientString(topSuffix, hGradient, 3+titleVisualWidth)

		// Bottom border: ╰───────── 1 of 4 ─╯
		botPrefix := bottomLeft + strings.Repeat(horizontal, bottomPadding) + " "
		botSuffix := " " + horizontal + bottomRight
		botSuffixOffset := len([]rune(botPrefix)) + counterVisualWidth
		bottomBorder = gradientString(botPrefix, hGradient, 0) +
			counterText +
			gradientString(botSuffix, hGradient, botSuffixOffset)

		// Content rows with gradient side borders
		for row, line := range contentLines {
			lineWidth := lipgloss.Width(line)
			pad := innerWidth - lineWidth - 2
			if pad < 0 {
				pad = 0
			}
			sc := lipgloss.NewStyle().Foreground(vGradient[row])
			body.WriteString(sc.Render(vertical) + " " + line + strings.Repeat(" ", pad) + " " + sc.Render(vertical) + "\n")
		}
	} else {
		// ─── FLAT BORDERS (inactive) ───
		bc := lipgloss.NewStyle().Foreground(dimGrey)

		topBorder = bc.Render(topLeft+horizontal+" ") + titleText + bc.Render(" "+strings.Repeat(horizontal, topPadding)+topRight)
		bottomBorder = bc.Render(bottomLeft+strings.Repeat(horizontal, bottomPadding)+" ") + counterText + bc.Render(" "+horizontal+bottomRight)

		for _, line := range contentLines {
			lineWidth := lipgloss.Width(line)
			pad := innerWidth - lineWidth - 2
			if pad < 0 {
				pad = 0
			}
			body.WriteString(bc.Render(vertical) + " " + line + strings.Repeat(" ", pad) + " " + bc.Render(vertical) + "\n")
		}
	}

	return topBorder + "\n" + body.String() + bottomBorder
}
