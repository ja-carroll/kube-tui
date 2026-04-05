package ui

import (
	"fmt"
	"path/filepath"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/ja-carroll/kube-tui/internal/k8s"
)

// ctxPicker is an overlay that lists every context found across all
// kubeconfigs and lets the user jump to another cluster without restart.
type ctxPicker struct {
	contexts []k8s.ContextInfo
	filter   string
	cursor   int
	current  string // name of the currently-connected context
}

func newCtxPicker(contexts []k8s.ContextInfo, current string) ctxPicker {
	cp := ctxPicker{contexts: contexts, current: current}
	// Position the cursor on the current context so the user can see
	// where they are and quickly skip past it.
	for i, c := range cp.filtered() {
		if c.Name == current {
			cp.cursor = i
			break
		}
	}
	return cp
}

// filtered returns contexts matching the current filter. Matches against
// both the context name and the cluster name — either is searchable.
func (cp ctxPicker) filtered() []k8s.ContextInfo {
	if cp.filter == "" {
		return cp.contexts
	}
	q := strings.ToLower(cp.filter)
	var out []k8s.ContextInfo
	for _, c := range cp.contexts {
		if strings.Contains(strings.ToLower(c.Name), q) ||
			strings.Contains(strings.ToLower(c.Cluster), q) {
			out = append(out, c)
		}
	}
	return out
}

// update handles keypresses. Returns the updated picker, whether to close,
// and the selected context (empty name means cancelled).
func (cp ctxPicker) update(msg tea.KeyPressMsg) (ctxPicker, bool, k8s.ContextInfo) {
	switch msg.String() {
	case "esc":
		return cp, true, k8s.ContextInfo{}

	case "enter":
		filtered := cp.filtered()
		if cp.cursor < len(filtered) {
			return cp, true, filtered[cp.cursor]
		}
		return cp, true, k8s.ContextInfo{}

	case "up", "ctrl+k":
		if cp.cursor > 0 {
			cp.cursor--
		}

	case "down", "ctrl+j":
		if cp.cursor < len(cp.filtered())-1 {
			cp.cursor++
		}

	case "backspace":
		if len(cp.filter) > 0 {
			runes := []rune(cp.filter)
			cp.filter = string(runes[:len(runes)-1])
			cp.cursor = 0
		}

	default:
		s := msg.String()
		if len(s) == 1 {
			cp.filter += s
			cp.cursor = 0
		}
	}
	return cp, false, k8s.ContextInfo{}
}

// view renders the picker. Each row shows context name, cluster, and the
// kubeconfig file it came from — helpful when you have overlapping names.
func (cp ctxPicker) view() string {
	title := actionMenuTitleStyle.Render(
		fmt.Sprintf("%s Switch context", symbolK8s),
	)

	cursor := searchCursorStyle.Render("█")
	prompt := "  " + searchBarStyle.Render("/ "+cp.filter+cursor)

	filtered := cp.filtered()
	var rows []string

	const maxRows = 10
	start := 0
	if cp.cursor >= maxRows {
		start = cp.cursor - maxRows + 1
	}
	end := start + maxRows
	if end > len(filtered) {
		end = len(filtered)
	}

	for i := start; i < end; i++ {
		c := filtered[i]
		marker := " "
		if c.Name == cp.current {
			marker = symbolSelected
		}

		// Two-line entry: name + cluster on top, source file dimmed below.
		nameLine := fmt.Sprintf("%s %s", marker, c.Name)
		if c.Cluster != "" && c.Cluster != c.Name {
			nameLine += actionDimStyle.Render(" · "+c.Cluster)
		}
		fileLine := "    " + actionDimStyle.Render("from "+filepath.Base(c.File))

		var block string
		if i == cp.cursor {
			block = selectedItemStyle.Render(symbolCursor+" "+strings.TrimPrefix(nameLine, " ")) + "\n" + fileLine
		} else {
			block = "  " + actionDescStyle.Render(nameLine) + "\n" + fileLine
		}
		rows = append(rows, block)
	}

	if len(filtered) == 0 {
		rows = append(rows, actionDimStyle.Render("  (no matches)"))
	}

	hint := "\n  " + keyHint("enter", "connect") + "  " + keyHint("esc", "cancel")
	content := title + "\n" + prompt + "\n\n" + strings.Join(rows, "\n") + hint

	return actionMenuStyle.Width(60).Render(content)
}
