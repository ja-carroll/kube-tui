package ui

import (
	"fmt"

	"charm.land/bubbles/v2/list"
	"charm.land/bubbles/v2/spinner"
	tea "charm.land/bubbletea/v2"
	lipgloss "charm.land/lipgloss/v2"
	"github.com/ja-carroll/kube-tui/internal/k8s"
)

// ASCII art logo — block-style Unicode box-drawing characters.
const logo = `
 ██╗  ██╗██╗   ██╗██████╗ ███████╗   ████████╗██╗   ██╗██╗
 ██║ ██╔╝██║   ██║██╔══██╗██╔════╝   ╚══██╔══╝██║   ██║██║
 █████╔╝ ██║   ██║██████╔╝█████╗  █████╗██║   ██║   ██║██║
 ██╔═██╗ ██║   ██║██╔══██╗██╔══╝  ╚════╝██║   ██║   ██║██║
 ██║  ██╗╚██████╔╝██████╔╝███████╗      ██║   ╚██████╔╝██║
 ╚═╝  ╚═╝ ╚═════╝ ╚═════╝ ╚══════╝      ╚═╝    ╚═════╝ ╚═╝`

// Landing page styles (using lipgloss v2 for our rendered text)
var (
	logoStyle = lipgloss.NewStyle().
			Foreground(headerPurple).
			Bold(true)

	subtitleStyle = lipgloss.NewStyle().
			Foreground(subtle)

	connectingMsgStyle = lipgloss.NewStyle().
				Foreground(cyan).
				Bold(true)

	landingErrorStyle = lipgloss.NewStyle().
				Foreground(red).
				Bold(true)
)

// landingState tracks the current phase of the landing page.
type landingState int

const (
	stateSelectContext landingState = iota
	stateConnecting
	stateError
)

// contextItem implements list.DefaultItem for the context selector.
type contextItem struct {
	name    string
	cluster string
}

func (i contextItem) Title() string       { return i.name }
func (i contextItem) Description() string { return fmt.Sprintf("cluster: %s", i.cluster) }
func (i contextItem) FilterValue() string { return i.name }

// connectedMsg signals that the k8s client is ready.
type connectedMsg struct {
	client *k8s.Client
}

// connectErrMsg signals a connection failure.
type connectErrMsg struct {
	err error
}

// LandingModel is the Bubble Tea model for the landing/welcome screen.
type LandingModel struct {
	state   landingState
	list    list.Model
	spinner spinner.Model
	errMsg  string
	width   int
	height  int

	// Client is the result — read by main.go after the program exits.
	Client *k8s.Client
}

// NewLanding creates the landing page model. It reads kubeconfig to discover
// available contexts and presents them in a styled selection list.
func NewLanding() LandingModel {
	contexts := k8s.ListContexts()

	items := make([]list.Item, len(contexts))
	for i, ctx := range contexts {
		items[i] = contextItem{name: ctx.Name, cluster: ctx.Cluster}
	}

	// The default delegate from bubbles/list renders items with title +
	// description and a left-border highlight on the selected item.
	// It looks great out of the box with Charm's default theme.
	delegate := list.NewDefaultDelegate()
	l := list.New(items, delegate, 50, 14)
	l.Title = "Select a context"
	l.SetShowStatusBar(false)
	l.SetShowHelp(false)
	l.DisableQuitKeybindings()

	// Spinner — the Dot style gives a nice animated braille pattern.
	s := spinner.New(spinner.WithSpinner(spinner.Dot))

	return LandingModel{
		state:   stateSelectContext,
		list:    l,
		spinner: s,
	}
}

func (m LandingModel) Init() tea.Cmd {
	return nil
}

func (m LandingModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		// Size the list to fit nicely below the logo
		listWidth := min(60, m.width-4)
		listHeight := min(14, m.height/3)
		m.list.SetSize(listWidth, listHeight)
		return m, nil

	case tea.KeyPressMsg:
		switch msg.String() {
		case "ctrl+c", "q":
			if m.state != stateConnecting {
				return m, tea.Quit
			}
		case "enter":
			if m.state == stateSelectContext {
				item, ok := m.list.SelectedItem().(contextItem)
				if !ok {
					return m, nil
				}
				m.state = stateConnecting
				return m, tea.Batch(
					m.spinner.Tick,
					connectToCluster(item.name),
				)
			}
			if m.state == stateError {
				m.state = stateSelectContext
				m.errMsg = ""
				return m, nil
			}
		}

	case connectedMsg:
		m.Client = msg.client
		return m, tea.Quit

	case connectErrMsg:
		m.state = stateError
		m.errMsg = msg.err.Error()
		return m, nil

	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		return m, cmd
	}

	// Pass messages to the list when in selection state
	if m.state == stateSelectContext {
		var cmd tea.Cmd
		m.list, cmd = m.list.Update(msg)
		return m, cmd
	}

	return m, nil
}

func (m LandingModel) View() tea.View {
	renderedLogo := logoStyle.Render(logo)
	subtitle := subtitleStyle.Render("Kubernetes Terminal User Interface")

	var content string
	switch m.state {
	case stateSelectContext:
		content = m.list.View()

	case stateConnecting:
		item, _ := m.list.SelectedItem().(contextItem)
		content = connectingMsgStyle.Render(
			fmt.Sprintf("\n  %s Connecting to %s...\n", m.spinner.View(), item.name),
		)

	case stateError:
		content = landingErrorStyle.Render(
			fmt.Sprintf("\n  %s Connection failed: %s\n\n  Press [enter] to try again\n", symbolCross, m.errMsg),
		)
	}

	// Stack vertically: logo, subtitle, content — then center on screen
	page := lipgloss.JoinVertical(lipgloss.Center,
		renderedLogo,
		subtitle,
		"",
		content,
	)

	return altView(lipgloss.Place(
		m.width, m.height,
		lipgloss.Center, lipgloss.Center,
		page,
	))
}

// connectToCluster creates a k8s client for the selected context.
func connectToCluster(contextName string) tea.Cmd {
	return func() tea.Msg {
		client, err := k8s.NewClientForContext(contextName)
		if err != nil {
			return connectErrMsg{err: err}
		}
		return connectedMsg{client: client}
	}
}
