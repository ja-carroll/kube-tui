package ui

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
)

// nsPicker is a floating overlay that lets the user jump between namespaces
// from anywhere in the app without tabbing back to the namespaces panel.
// It supports live filter-as-you-type and j/k navigation.
type nsPicker struct {
	namespaces []string // full list
	filter     string   // filter-as-you-type query
	cursor     int      // index into the filtered list
}

// newNSPicker creates a picker pre-positioned on the current namespace
// so the user can hit enter to keep it or start typing to jump.
func newNSPicker(all []string, current string) nsPicker {
	np := nsPicker{namespaces: all}
	for i, ns := range np.filtered() {
		if ns == current {
			np.cursor = i
			break
		}
	}
	return np
}

// filtered returns the namespaces matching the current filter,
// using case-insensitive substring matching.
func (np nsPicker) filtered() []string {
	if np.filter == "" {
		return np.namespaces
	}
	q := strings.ToLower(np.filter)
	var out []string
	for _, ns := range np.namespaces {
		if strings.Contains(strings.ToLower(ns), q) {
			out = append(out, ns)
		}
	}
	return out
}

// update handles keypresses. Returns the updated picker, whether to close,
// and the selected namespace (empty string means cancelled).
func (np nsPicker) update(msg tea.KeyPressMsg) (nsPicker, bool, string) {
	switch msg.String() {
	case "esc":
		return np, true, ""

	case "enter":
		filtered := np.filtered()
		if np.cursor < len(filtered) {
			return np, true, filtered[np.cursor]
		}
		return np, true, ""

	case "up", "ctrl+k":
		if np.cursor > 0 {
			np.cursor--
		}

	case "down", "ctrl+j":
		if np.cursor < len(np.filtered())-1 {
			np.cursor++
		}

	case "backspace":
		if len(np.filter) > 0 {
			runes := []rune(np.filter)
			np.filter = string(runes[:len(runes)-1])
			np.cursor = 0
		}

	default:
		// Single-character keys type into the filter.
		s := msg.String()
		if len(s) == 1 {
			np.filter += s
			np.cursor = 0
		}
	}

	return np, false, ""
}

// view renders the picker box. Fixed width keeps it from jumping as the
// user types. Height is capped to avoid overflowing the screen.
func (np nsPicker) view() string {
	title := actionMenuTitleStyle.Render(
		fmt.Sprintf("%s Switch namespace", symbolK8s),
	)

	cursor := searchCursorStyle.Render("█")
	prompt := "  " + searchBarStyle.Render("/ "+np.filter+cursor)

	filtered := np.filtered()
	var items []string

	const maxRows = 12
	start := 0
	if np.cursor >= maxRows {
		start = np.cursor - maxRows + 1
	}
	end := start + maxRows
	if end > len(filtered) {
		end = len(filtered)
	}

	for i := start; i < end; i++ {
		ns := filtered[i]
		if i == np.cursor {
			items = append(items, selectedItemStyle.Render(symbolCursor+" "+ns))
		} else {
			items = append(items, actionDescStyle.Render("  "+ns))
		}
	}

	if len(filtered) == 0 {
		items = append(items, actionDimStyle.Render("  (no matches)"))
	}

	hint := "\n  " + keyHint("enter", "select") + "  " + keyHint("esc", "cancel")
	content := title + "\n" + prompt + "\n\n" + strings.Join(items, "\n") + hint

	return actionMenuStyle.Width(40).Render(content)
}
