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

// logViewer holds the state for the full-screen log overlay.
type logViewer struct {
	podName   string
	namespace string
	lines     []string
	offset    int
	width     int
	height    int
	cancel    context.CancelFunc

	// Save state
	saving   bool   // true when the filename prompt is open
	filename string // the filename being edited

	// Status message shown briefly after save
	statusMsg string
}

// defaultFilename generates a sensible default like "nginx-abc123-2026-03-29T20-15-30.log"
func (lv logViewer) defaultFilename() string {
	// time.Now().Format uses Go's unique approach to time formatting.
	// Instead of strftime tokens like %Y-%m-%d, Go uses a "reference time":
	// Mon Jan 2 15:04:05 MST 2006
	// You rearrange that specific date/time to define your format.
	// Why that date? Because it's 01/02 03:04:05 06 -07 — each component
	// is a different number (1-7), making it unambiguous.
	// It's weird but you get used to it!
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

func (lv logViewer) update(msg tea.KeyPressMsg) (logViewer, bool, tea.Cmd) {
	// If we're in the save prompt, handle filename input
	if lv.saving {
		return lv.handleSaveInput(msg), false, nil
	}

	maxScroll := len(lv.lines) - lv.viewableHeight()
	if maxScroll < 0 {
		maxScroll = 0
	}

	switch msg.String() {
	case "esc", "q":
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

	case "g":
		lv.offset = maxScroll

	case "G":
		lv.offset = 0

	case "s":
		// Open the save prompt with the default filename pre-filled
		lv.saving = true
		lv.filename = lv.defaultFilename()
		lv.statusMsg = ""
	}

	return lv, false, nil
}

// handleSaveInput processes keystrokes in the filename prompt.
func (lv logViewer) handleSaveInput(msg tea.KeyPressMsg) logViewer {
	switch msg.String() {
	case "esc":
		// Cancel the save
		lv.saving = false
		lv.filename = ""

	case "enter":
		// Write the file. os.WriteFile is Go's one-liner for writing a file.
		// It creates the file if it doesn't exist, truncates it if it does.
		// 0644 is the Unix file permission: owner read/write, others read-only.
		// In Go, file permissions use os.FileMode which is just a uint32.
		lv.saving = false
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

	// ctrl+u clears the whole input — a common terminal shortcut
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

func (lv logViewer) viewableHeight() int {
	// Total height budget: terminal height
	// - 4 for box overhead (border + padding)
	// - 2 for help bar
	// - 3 for title + scroll info + gaps inside the box
	h := lv.height - 9
	if h < 1 {
		h = 1
	}
	return h
}

func (lv logViewer) view() string {
	viewHeight := lv.viewableHeight()

	title := titleStyle.Render(fmt.Sprintf("Logs: %s/%s", lv.namespace, lv.podName))

	totalLines := len(lv.lines)
	end := totalLines - lv.offset
	start := end - viewHeight
	if start < 0 {
		start = 0
	}
	if end < 0 {
		end = 0
	}

	var visible []string
	if totalLines > 0 && start < totalLines {
		visible = lv.lines[start:end]
	}

	content := strings.Join(visible, "\n")
	lineCount := len(visible)
	if lineCount < viewHeight {
		content += strings.Repeat("\n", viewHeight-lineCount)
	}

	scrollInfo := dimmedItemStyle.Render(fmt.Sprintf(
		"%d lines total | offset: %d",
		totalLines, lv.offset,
	))

	// Set explicit height to prevent overflow. Content height = terminal - overhead.
	panelContentHeight := lv.height - 6 // 4 box overhead + 2 help bar
	panel := activePanelStyle.
		Width(lv.width - 4).
		Height(panelContentHeight).
		Render(lipgloss.JoinVertical(lipgloss.Left, title, content, scrollInfo))

	// Bottom bar: save prompt, status message, or key badge help
	var bottomBar string
	if lv.saving {
		cursor := searchCursorStyle.Render("█")
		prompt := searchBarStyle.Render("Save as: " + lv.filename + cursor)
		hint := "  " + keyHint("enter", "save") + "  " + keyHint("esc", "cancel") + "  " + keyHint("ctrl+u", "clear")
		bottomBar = prompt + hint
	} else if lv.statusMsg != "" {
		bottomBar = lipgloss.NewStyle().Foreground(special).Render(symbolCheck+" "+lv.statusMsg) +
			"  " + keyHint("s", "save again")
	} else {
		bottomBar = strings.Join([]string{
			keyHint("j/k", "scroll"),
			keyHint("g/G", "top/bottom"),
			keyHint("s", "save"),
			keyHint("esc", "back"),
		}, "  ")
	}

	return panel + "\n" + bottomBar
}
