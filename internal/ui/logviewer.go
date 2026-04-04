package ui

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	lipgloss "charm.land/lipgloss/v2"
	"github.com/ja-carroll/kube-tui/internal/k8s"
)

// Log viewer message types.

type logLineMsg struct {
	line string
}

type logDoneMsg struct{}

type logErrMsg struct {
	err error
}

// logSavedMsg is sent after a log file has been written (or failed).
type logSavedMsg struct {
	path string
	err  error
}

// maxLogLines caps the in-memory buffer so log streaming doesn't grow
// unbounded. When we hit this, we drop the oldest lines.
const maxLogLines = 5000

// logViewer holds the state for the full-screen log overlay.
type logViewer struct {
	podName   string
	namespace string
	lines     []string
	offset    int // 0 = at bottom (following), N = N lines scrolled up
	width     int
	height    int
	cancel    context.CancelFunc

	// Save state
	saving   bool   // true when the filename prompt is open
	filename string // the filename being edited

	// Filter state — hides non-matching lines from the view
	filtering bool   // true when the filter prompt is open (typing)
	filter    string // active filter query (may be non-empty when not typing)

	// Status message shown briefly after save
	statusMsg string
}

// defaultFilename generates a sensible default like "nginx-abc123-2026-03-29T20-15-30.log"
func (lv logViewer) defaultFilename() string {
	ts := time.Now().Format("2006-01-02T15-04-05")
	return fmt.Sprintf("%s-%s.log", lv.podName, ts)
}

func startLogStream(client *k8s.Client, namespace, podName string, width, height int) (logViewer, tea.Cmd) {
	ctx, cancel := context.WithCancel(context.Background())

	viewer := logViewer{
		podName:   podName,
		namespace: namespace,
		width:     width,
		height:    height,
		cancel:    cancel,
	}

	return viewer, func() tea.Msg {
		lines, err := client.StreamPodLogs(ctx, namespace, podName)
		if err != nil {
			return logErrMsg{err: err}
		}
		line, ok := <-lines
		if !ok {
			return logDoneMsg{}
		}
		activeLogChan = lines
		return logLineMsg{line: line}
	}
}

var activeLogChan <-chan string

func waitForNextLogLine() tea.Cmd {
	return func() tea.Msg {
		if activeLogChan == nil {
			return logDoneMsg{}
		}
		line, ok := <-activeLogChan
		if !ok {
			return logDoneMsg{}
		}
		return logLineMsg{line: line}
	}
}

// appendLine adds a line to the buffer, caps the buffer size, and keeps
// the scroll offset anchored so scrolled-up views don't drift when new
// lines arrive. Returns the updated viewer.
func (lv logViewer) appendLine(line string) logViewer {
	lv.lines = append(lv.lines, line)

	// Drop oldest lines if we exceed the cap.
	if len(lv.lines) > maxLogLines {
		drop := len(lv.lines) - maxLogLines
		lv.lines = lv.lines[drop:]
		// If the user is scrolled up, compensate so their viewport stays put.
		if lv.offset > 0 {
			lv.offset -= drop
			if lv.offset < 0 {
				lv.offset = 0
			}
		}
	}

	// If the user is scrolled up (not following), keep their viewport
	// anchored by incrementing the offset. When offset == 0 they're at
	// the bottom, so the new line just appears naturally.
	if lv.offset > 0 {
		lv.offset++
		// Clamp — if filter is on, the visible count may cap this.
		max := lv.maxScroll()
		if lv.offset > max {
			lv.offset = max
		}
	}

	return lv
}

// visibleLines returns the lines to actually show, after applying the filter.
// Building this once per render is cheap and keeps the rendering code clean.
func (lv logViewer) visibleLines() []string {
	if lv.filter == "" {
		return lv.lines
	}
	q := strings.ToLower(lv.filter)
	out := make([]string, 0, len(lv.lines))
	for _, l := range lv.lines {
		if strings.Contains(strings.ToLower(l), q) {
			out = append(out, l)
		}
	}
	return out
}

// maxScroll returns the maximum valid offset given current buffer + filter.
func (lv logViewer) maxScroll() int {
	m := len(lv.visibleLines()) - lv.viewableHeight()
	if m < 0 {
		return 0
	}
	return m
}

func (lv logViewer) update(msg tea.KeyPressMsg) (logViewer, bool, tea.Cmd) {
	if lv.saving {
		return lv.handleSaveInput(msg), false, nil
	}
	if lv.filtering {
		return lv.handleFilterInput(msg), false, nil
	}

	maxScroll := lv.maxScroll()

	switch msg.String() {
	case "esc":
		// esc clears an active filter first; if no filter, closes the viewer.
		if lv.filter != "" {
			lv.filter = ""
			lv.offset = 0
			return lv, false, nil
		}
		lv.cancel()
		activeLogChan = nil
		return lv, true, nil

	case "q":
		lv.cancel()
		activeLogChan = nil
		return lv, true, nil

	case "up", "k":
		if lv.offset < maxScroll {
			lv.offset++
		}

	case "down", "j":
		if lv.offset > 0 {
			lv.offset--
		}

	case "pgup", "ctrl+u":
		lv.offset += lv.viewableHeight() / 2
		if lv.offset > maxScroll {
			lv.offset = maxScroll
		}

	case "pgdown", "ctrl+d":
		lv.offset -= lv.viewableHeight() / 2
		if lv.offset < 0 {
			lv.offset = 0
		}

	case "g":
		// Jump to top of buffer
		lv.offset = maxScroll

	case "G":
		// Jump to bottom (resume following)
		lv.offset = 0

	case "s":
		lv.saving = true
		lv.filename = lv.defaultFilename()
		lv.statusMsg = ""

	case "/":
		lv.filtering = true
		lv.statusMsg = ""
	}

	return lv, false, nil
}

// handleSaveInput processes keystrokes in the filename prompt.
func (lv logViewer) handleSaveInput(msg tea.KeyPressMsg) logViewer {
	switch msg.String() {
	case "esc":
		lv.saving = false
		lv.filename = ""

	case "enter":
		lv.saving = false
		// Save the full unfiltered buffer by default — the user probably
		// wants the complete log file, not just what's on screen.
		content := strings.Join(lv.lines, "\n") + "\n"
		err := os.WriteFile(lv.filename, []byte(content), 0644)
		if err != nil {
			lv.statusMsg = fmt.Sprintf("Error: %v", err)
		} else {
			lv.statusMsg = fmt.Sprintf("Saved %d lines to %s", len(lv.lines), lv.filename)
		}
		lv.filename = ""

	case "backspace":
		if len(lv.filename) > 0 {
			runes := []rune(lv.filename)
			lv.filename = string(runes[:len(runes)-1])
		}

	case "ctrl+u":
		lv.filename = ""

	case "ctrl+c":
		lv.saving = false

	default:
		if len(msg.String()) == 1 {
			lv.filename += msg.String()
		}
	}

	return lv
}

// handleFilterInput processes keystrokes in the filter prompt.
func (lv logViewer) handleFilterInput(msg tea.KeyPressMsg) logViewer {
	switch msg.String() {
	case "esc":
		// Cancel the filter entirely
		lv.filtering = false
		lv.filter = ""
		lv.offset = 0

	case "enter":
		// Keep the filter active; close the prompt so keys navigate again.
		lv.filtering = false
		lv.offset = 0

	case "backspace":
		if len(lv.filter) > 0 {
			runes := []rune(lv.filter)
			lv.filter = string(runes[:len(runes)-1])
			lv.offset = 0
		}

	case "ctrl+u":
		lv.filter = ""
		lv.offset = 0

	case "ctrl+c":
		lv.filtering = false

	default:
		if len(msg.String()) == 1 {
			lv.filter += msg.String()
			lv.offset = 0
		}
	}
	return lv
}

// logPanelStyle is a dedicated panel style for the log viewer — no
// padding, just a gradient border. Removing padding makes height math
// exact, so the bottom border always lands where we expect it to.
var logPanelStyle = lipgloss.NewStyle().
	Border(lipgloss.RoundedBorder()).
	BorderForegroundBlend(gradientPurple, gradientPink, gradientCyan)

func (lv logViewer) viewableHeight() int {
	// Height budget: terminal height
	// - 2 for border (top + bottom)
	// - 1 for title row (inside the box)
	// - 1 for status bar at the bottom
	h := lv.height - 4
	if h < 1 {
		h = 1
	}
	return h
}

// innerWidth returns the usable width inside the panel (after border).
// We truncate log lines to this so they never wrap — wrapping would add
// visual rows beyond our height budget and spill past the terminal.
func (lv logViewer) innerWidth() int {
	// logPanelStyle: border(1+1) = 2 chars of frame. We add a 1-char
	// left gutter for visual breathing room, so subtract 3 total.
	w := lv.width - 3
	if w < 10 {
		w = 10
	}
	return w
}

// addLeftGutter prefixes each line with a single space so log content has
// a bit of breathing room from the panel border.
func addLeftGutter(s string) string {
	lines := strings.Split(s, "\n")
	for i, l := range lines {
		lines[i] = " " + l
	}
	return strings.Join(lines, "\n")
}

// truncate clips s to n runes, appending "…" if truncated. Works on runes
// (not bytes) so multi-byte characters don't get cut mid-glyph.
func truncate(s string, n int) string {
	if n <= 1 {
		return ""
	}
	runes := []rune(s)
	if len(runes) <= n {
		return s
	}
	return string(runes[:n-1]) + "…"
}

func (lv logViewer) view() string {
	viewHeight := lv.viewableHeight()
	innerW := lv.innerWidth()

	titleText := fmt.Sprintf("Logs: %s/%s", lv.namespace, lv.podName)
	title := titleStyle.Render(truncate(titleText, innerW))

	visible := lv.visibleLines()
	totalLines := len(visible)
	end := totalLines - lv.offset
	start := end - viewHeight
	if start < 0 {
		start = 0
	}
	if end < 0 {
		end = 0
	}

	var window []string
	if totalLines > 0 && start < totalLines {
		window = visible[start:end]
	}

	// Truncate every visible line to the inner width to prevent wrapping.
	// This is what actually fixes the overflow bug.
	truncated := make([]string, 0, len(window))
	for _, line := range window {
		truncated = append(truncated, truncate(line, innerW))
	}

	// Pad out to the full height so the box renders at a stable size.
	content := strings.Join(truncated, "\n")
	if len(truncated) < viewHeight {
		content += strings.Repeat("\n", viewHeight-len(truncated))
	}

	// Render the panel with exact dimensions. Inner height must equal
	// title(1) + content(viewHeight) = lv.height - 3, so outer = lv.height - 1.
	panelInnerHeight := 1 + viewHeight // title + content rows
	panel := logPanelStyle.
		Width(lv.width - 2).
		Height(panelInnerHeight).
		MaxHeight(panelInnerHeight).
		MaxWidth(lv.width - 2).
		Render(lipgloss.JoinVertical(lipgloss.Left, " "+title, addLeftGutter(content)))

	// Bottom bar: context-sensitive prompt, status message, or a status
	// strip combining follow state + line counts + key hints.
	var bottomBar string
	switch {
	case lv.saving:
		cursor := searchCursorStyle.Render("█")
		prompt := searchBarStyle.Render("Save as: " + lv.filename + cursor)
		hint := "  " + keyHint("enter", "save") + "  " + keyHint("esc", "cancel") + "  " + keyHint("ctrl+u", "clear")
		bottomBar = prompt + hint

	case lv.filtering:
		cursor := searchCursorStyle.Render("█")
		prompt := searchBarStyle.Render("filter: " + lv.filter + cursor)
		hint := "  " + keyHint("enter", "apply") + "  " + keyHint("esc", "clear") + "  " + keyHint("ctrl+u", "reset")
		bottomBar = prompt + hint

	case lv.statusMsg != "":
		bottomBar = lipgloss.NewStyle().Foreground(special).Render(symbolCheck+" "+lv.statusMsg) +
			"  " + keyHint("s", "save again")

	default:
		// Status strip: follow indicator + line counter on the left,
		// key hints on the right — classic statusline layout.
		var followIndicator string
		if lv.offset == 0 {
			followIndicator = lipgloss.NewStyle().Foreground(green).Bold(true).Render("● FOLLOWING")
		} else {
			followIndicator = lipgloss.NewStyle().Foreground(yellow).Bold(true).Render("■ PAUSED")
		}

		counter := fmt.Sprintf("%d lines", len(lv.lines))
		if lv.filter != "" {
			counter = fmt.Sprintf("%d / %d matching \"%s\"", totalLines, len(lv.lines), lv.filter)
		}
		left := followIndicator + "  " + dimmedItemStyle.Render(counter)

		right := strings.Join([]string{
			keyHint("j/k", "scroll"),
			keyHint("g/G", "top/bottom"),
			keyHint("/", "filter"),
			keyHint("s", "save"),
			keyHint("esc", "back"),
		}, "  ")

		// Pad the gap so hints sit flush right, like the main header bar.
		gap := lv.width - lipgloss.Width(left) - lipgloss.Width(right)
		if gap < 2 {
			gap = 2
		}
		bottomBar = left + strings.Repeat(" ", gap) + right
	}

	return panel + "\n" + bottomBar
}
