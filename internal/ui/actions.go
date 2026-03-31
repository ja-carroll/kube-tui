package ui

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/ja-carroll/kube-tui/internal/k8s"
)

// action represents a single thing you can do to a resource.
type action struct {
	key  string // the key to press (e.g. "l", "d")
	name string // display name (e.g. "View logs")
	icon string // a unicode symbol for flair
}

// actionsForResource returns the available actions based on resource type.
// This is a plain function, not a method — in Go, not everything needs to
// live on a struct. Free functions are perfectly idiomatic when there's no
// state to carry.
//
// An interesting Go pattern here: we return a SLICE OF STRUCTS rather than,
// say, a map. This preserves ordering (maps in Go have random iteration order)
// which matters for UI — we want actions in a consistent, logical order.
func actionsForResource(res resourceType) []action {
	// Common actions that apply to all resources
	common := []action{
		{"y", "View YAML", symbolArrow},
		{"d", "Delete", symbolCross},
	}

	switch res {
	case resourcePods:
		return append([]action{
			{"l", "View logs", symbolArrow},
			{"e", "Exec into pod", symbolArrow},
			{"r", "Restart (delete pod)", symbolCross},
		}, common...)

	case resourceDeployments:
		return append([]action{
			{"s", "Scale", symbolArrow},
			{"r", "Restart rollout", symbolArrow},
		}, common...)

	case resourceStatefulSets:
		return append([]action{
			{"s", "Scale", symbolArrow},
			{"r", "Restart rollout", symbolArrow},
		}, common...)

	case resourceDaemonSets:
		return append([]action{
			{"r", "Restart rollout", symbolArrow},
		}, common...)

	case resourceConfigMaps, resourceSecrets:
		return append([]action{
			{"v", "View data", symbolArrow},
		}, common...)

	case resourceJobs:
		return append([]action{
			{"l", "View logs", symbolArrow},
		}, common...)

	default:
		return common
	}
}

// actionMenu holds the state of the action overlay.
type actionMenu struct {
	resource     k8s.Resource
	resourceType resourceType
	namespace    string
	actions      []action
	cursor       int
}

// newActionMenu creates an action menu for a given resource.
func newActionMenu(res k8s.Resource, resType resourceType, namespace string) actionMenu {
	return actionMenu{
		resource:     res,
		resourceType: resType,
		namespace:    namespace,
		actions:      actionsForResource(resType),
	}
}

// update handles keypresses in the action menu.
// Returns: updated menu, whether to close, the selected action key (if any), and a cmd.
func (am actionMenu) update(msg tea.KeyMsg) (actionMenu, bool, string) {
	switch msg.String() {
	case "esc", "q":
		return am, true, ""

	case "up", "k":
		if am.cursor > 0 {
			am.cursor--
		}

	case "down", "j":
		if am.cursor < len(am.actions)-1 {
			am.cursor++
		}

	case "enter":
		// Return the key of the selected action
		if am.cursor < len(am.actions) {
			return am, true, am.actions[am.cursor].key
		}

	default:
		// Direct key press — check if it matches an action
		for _, a := range am.actions {
			if msg.String() == a.key {
				return am, true, a.key
			}
		}
	}

	return am, false, ""
}

// view renders the action menu box (without positioning — the caller
// handles placing it on screen). Returning just the box keeps this
// method focused on content, while the parent handles composition.
// This is the single-responsibility principle at work.
func (am actionMenu) view() string {
	title := actionMenuTitleStyle.Render(
		fmt.Sprintf("%s Actions: %s", symbolK8s, am.resource.Name()),
	)

	var items []string
	for i, a := range am.actions {
		key := actionKeyStyle.Render(fmt.Sprintf("[%s]", a.key))
		desc := actionDescStyle.Render(a.name)
		line := fmt.Sprintf("  %s  %s %s", key, a.icon, desc)

		if i == am.cursor {
			line = selectedItemStyle.Render(
				fmt.Sprintf("%s %s  %s %s", symbolCursor, actionKeyStyle.Render(fmt.Sprintf("[%s]", a.key)), a.icon, a.name),
			)
		}

		items = append(items, line)
	}

	hint := actionDimStyle.Render("\n  [esc] cancel")
	content := title + "\n" + strings.Join(items, "\n") + hint

	return actionMenuStyle.Render(content)
}

// scaleDialog is a small overlay that lets the user type a new replica count.
// It wraps Charm's textinput.Model — a pre-built, styled text input component
// that handles cursor rendering, focus, and blinking for us. Rather than
// building our own input from scratch (like we did with the search bar),
// we lean on the ecosystem. This is a good Go habit: use well-maintained
// libraries for UI primitives, write your own logic for domain stuff.
type scaleDialog struct {
	input        textinput.Model
	resourceName string
	namespace    string
	resType      string
	currentScale int
}

// newScaleDialog creates a scale input dialog pre-filled with the current
// replica count. The textinput.Model handles all the text editing — cursor
// movement, backspace, selection — so we just configure and style it.
func newScaleDialog(resName, namespace, resType string, currentScale int) (scaleDialog, tea.Cmd) {
	ti := textinput.New()
	ti.Placeholder = "replicas"
	ti.SetValue(strconv.Itoa(currentScale))
	ti.CharLimit = 4
	ti.Width = 10
	ti.Prompt = fmt.Sprintf("%s ", symbolK8s)

	// Validate that input is a non-negative integer. textinput calls this
	// on every keystroke. If it returns an error, the input rejects the key.
	ti.Validate = func(s string) error {
		if s == "" {
			return nil
		}
		n, err := strconv.Atoi(s)
		if err != nil {
			return fmt.Errorf("must be a number")
		}
		if n < 0 {
			return fmt.Errorf("must be >= 0")
		}
		return nil
	}

	// Focus returns a tea.Cmd that starts the cursor blinking.
	cmd := ti.Focus()

	return scaleDialog{
		input:        ti,
		resourceName: resName,
		namespace:    namespace,
		resType:      resType,
		currentScale: currentScale,
	}, cmd
}

// update handles keypresses. Returns the updated dialog, whether it closed,
// the new replica count (or -1 if cancelled), and a tea.Cmd.
func (sd scaleDialog) update(msg tea.KeyMsg) (scaleDialog, bool, int, tea.Cmd) {
	switch msg.String() {
	case "esc":
		return sd, true, -1, nil
	case "enter":
		val := sd.input.Value()
		if val == "" {
			return sd, true, -1, nil
		}
		n, err := strconv.Atoi(val)
		if err != nil || n < 0 {
			return sd, false, -1, nil
		}
		return sd, true, n, nil
	}

	// Pass all other keys to the textinput — it handles cursor movement,
	// typing, backspace, etc. This delegation pattern is the same one
	// Bubble Tea uses everywhere: pass the message to child components
	// and collect their commands.
	var cmd tea.Cmd
	sd.input, cmd = sd.input.Update(msg)
	return sd, false, -1, cmd
}

// view renders the scale dialog box with a fixed width so it doesn't
// jump around as the user types.
func (sd scaleDialog) view() string {
	title := actionMenuTitleStyle.Render(
		fmt.Sprintf("%s Scale: %s", symbolK8s, sd.resourceName),
	)

	current := actionDimStyle.Render(
		fmt.Sprintf("  Current replicas: %d", sd.currentScale),
	)

	inputLine := fmt.Sprintf("  New replicas: %s", sd.input.View())

	hint := actionDimStyle.Render("\n  [enter] apply  [esc] cancel")

	content := title + "\n" + current + "\n\n" + inputLine + hint

	// Fixed width prevents the dialog from resizing as digits are typed.
	return actionMenuStyle.Width(40).Render(content)
}
