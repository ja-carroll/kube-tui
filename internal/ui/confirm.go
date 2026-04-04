package ui

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
)

// confirmDialog is a small y/N modal for destructive actions. It holds
// only the text to display — the action to run on confirm lives on the
// Model, since tea.Cmd doesn't compare/copy cleanly inside a struct.
type confirmDialog struct {
	title   string
	message string
	danger  bool // if true, styles the title red
}

// update handles keypresses. Returns the dialog, whether it closed, and
// whether the user confirmed.
func (cd confirmDialog) update(msg tea.KeyPressMsg) (confirmDialog, bool, bool) {
	switch msg.String() {
	case "y", "Y", "enter":
		return cd, true, true
	case "n", "N", "esc", "q":
		return cd, true, false
	}
	return cd, false, false
}

// view renders the dialog. The message typically names the resource being
// acted on so the user can double-check before confirming.
func (cd confirmDialog) view() string {
	var titleStyle = actionMenuTitleStyle
	if cd.danger {
		titleStyle = titleStyle.Foreground(red)
	}
	title := titleStyle.Render(fmt.Sprintf("%s %s", symbolCross, cd.title))

	msgLines := strings.Split(cd.message, "\n")
	var body []string
	for _, l := range msgLines {
		body = append(body, "  "+actionDescStyle.Render(l))
	}

	hint := "\n  " + keyHint("y", "confirm") + "  " + keyHint("n", "cancel")
	content := title + "\n\n" + strings.Join(body, "\n") + hint

	return actionMenuStyle.Width(50).Render(content)
}
