package ui

import (
	"context"
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/ja-carroll/kube-tui/internal/k8s"
)

// Message types for async data fetching.

type namespacesMsg struct {
	namespaces []string
	err        error
}

// resourceMsg now carries []k8s.Resource instead of []string.
// The interface means this single message type works for all resource kinds.
type resourceMsg struct {
	items []k8s.Resource
	err   error
}

// resourceType represents the kind of Kubernetes resource to display.
type resourceType int

const (
	resourcePods resourceType = iota
	resourceDeployments
	resourceServices
	resourceConfigMaps
)

func (rt resourceType) String() string {
	switch rt {
	case resourcePods:
		return "Pods"
	case resourceDeployments:
		return "Deployments"
	case resourceServices:
		return "Services"
	case resourceConfigMaps:
		return "ConfigMaps"
	default:
		return "Unknown"
	}
}

var allResourceTypes = []resourceType{
	resourcePods,
	resourceDeployments,
	resourceServices,
	resourceConfigMaps,
}

// pane tracks which panel is focused.
type pane int

const (
	leftPane  pane = iota
	rightPane
)

// leftSection tracks which section of the left pane the cursor is in.
type leftSection int

const (
	namespacesSection leftSection = iota
	resourcesSection
)

// Model is the top-level application state.
type Model struct {
	client *k8s.Client

	namespaces []string
	items      []k8s.Resource // now holds rich resource objects, not just names

	activePane  pane
	leftSection leftSection

	nsCursor       int
	resCursor      int
	itemCursor     int
	selectedNS     string
	selectedResIdx int
	selectedRes    resourceType

	err error

	width  int
	height int

	viewingLogs bool
	logViewer   logViewer

	// Search/filter state.
	searching   bool
	searchQuery string
	searchScope searchScope // local (active pane only) or global

	// Filtered views — recomputed whenever searchQuery changes.
	filteredNS    []string
	filteredItems []k8s.Resource
}

// searchScope controls what the search filter applies to.
type searchScope int

const (
	searchLocal  searchScope = iota // filter only the active pane
	searchGlobal                    // filter all panes
)

func New(client *k8s.Client) Model {
	return Model{
		client:      client,
		selectedRes: resourcePods,
	}
}

// refilter recomputes the filtered slices based on the current search query.
// This is called whenever the query changes OR when new data arrives.
//
// strings.Contains + strings.ToLower is the simplest approach for
// case-insensitive substring matching. For fancier fuzzy matching (like
// fzf), there are libraries, but this covers 90% of use cases.
func (m *Model) refilter() {
	query := strings.ToLower(m.searchQuery)

	// Determine which lists to filter based on scope and active pane.
	// Local search: only filter the pane you're in.
	// Global search: filter everything.
	filterNS := query != "" && (m.searchScope == searchGlobal ||
		(m.searchScope == searchLocal && m.activePane == leftPane && m.leftSection == namespacesSection))
	filterItems := query != "" && (m.searchScope == searchGlobal ||
		(m.searchScope == searchLocal && m.activePane == rightPane))

	// Filter namespaces
	m.filteredNS = nil
	for _, ns := range m.namespaces {
		if !filterNS || strings.Contains(strings.ToLower(ns), query) {
			m.filteredNS = append(m.filteredNS, ns)
		}
	}

	// Filter resource items
	m.filteredItems = nil
	for _, item := range m.items {
		if !filterItems || strings.Contains(strings.ToLower(item.Name()), query) {
			m.filteredItems = append(m.filteredItems, item)
		}
	}

	// Clamp cursors to prevent out-of-bounds panics
	if m.nsCursor >= len(m.filteredNS) {
		m.nsCursor = max(0, len(m.filteredNS)-1)
	}
	if m.itemCursor >= len(m.filteredItems) {
		m.itemCursor = max(0, len(m.filteredItems)-1)
	}
}

func (m Model) Init() tea.Cmd {
	return m.fetchNamespaces()
}

func (m Model) fetchNamespaces() tea.Cmd {
	client := m.client
	return func() tea.Msg {
		namespaces, err := client.ListNamespaces(context.Background())
		return namespacesMsg{namespaces: namespaces, err: err}
	}
}

// fetchResources now calls the new *Resources methods that return rich types.
func (m Model) fetchResources() tea.Cmd {
	client := m.client
	ns := m.selectedNS
	res := m.selectedRes
	return func() tea.Msg {
		var items []k8s.Resource
		var err error

		switch res {
		case resourcePods:
			items, err = client.ListPodResources(context.Background(), ns)
		case resourceDeployments:
			items, err = client.ListDeploymentResources(context.Background(), ns)
		case resourceServices:
			items, err = client.ListServiceResources(context.Background(), ns)
		case resourceConfigMaps:
			items, err = client.ListConfigMapResources(context.Background(), ns)
		}

		return resourceMsg{items: items, err: err}
	}
}

// Update handles all messages and returns the updated model.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		if m.viewingLogs {
			m.logViewer.width = msg.Width
			m.logViewer.height = msg.Height
		}
		return m, nil

	case logLineMsg:
		m.logViewer.lines = append(m.logViewer.lines, msg.line)
		return m, waitForNextLogLine()

	case logDoneMsg:
		return m, nil

	case logErrMsg:
		m.err = msg.err
		m.viewingLogs = false
		return m, nil

	case namespacesMsg:
		if msg.err != nil {
			m.err = msg.err
			return m, nil
		}
		m.namespaces = msg.namespaces
		m.refilter() // rebuild filtered view with new data
		if len(m.namespaces) > 0 {
			m.selectedNS = m.namespaces[0]
			return m, m.fetchResources()
		}
		return m, nil

	case resourceMsg:
		if msg.err != nil {
			m.err = msg.err
			return m, nil
		}
		m.items = msg.items
		m.itemCursor = 0
		m.refilter() // rebuild filtered view with new data
		return m, nil

	case tea.KeyMsg:
		if m.viewingLogs {
			viewer, closed, cmd := m.logViewer.update(msg)
			m.logViewer = viewer
			if closed {
				m.viewingLogs = false
			}
			return m, cmd
		}
		return m.handleKeyPress(msg)
	}

	return m, nil
}

func (m Model) handleKeyPress(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	// When searching, most keys go to the search input.
	if m.searching {
		return m.handleSearchInput(msg)
	}

	switch msg.String() {
	case "ctrl+c", "q":
		return m, tea.Quit

	case "tab":
		if m.activePane == leftPane && m.leftSection == namespacesSection {
			m.leftSection = resourcesSection
		} else if m.activePane == leftPane && m.leftSection == resourcesSection {
			m.activePane = rightPane
		} else {
			m.activePane = leftPane
			m.leftSection = namespacesSection
		}

	case "up", "k":
		m.moveCursorUp()

	case "down", "j":
		m.moveCursorDown()

	case "enter":
		return m.handleEnter()

	case "esc":
		// If a filter is active (search closed but query kept), clear it
		if m.searchQuery != "" {
			m.searchQuery = ""
			m.refilter()
		}

	case "/":
		// Enter search mode. If a filter is already active, re-open
		// the search bar with the existing query so you can edit it.
		m.searching = true
		if m.searchQuery == "" {
			m.searchScope = searchLocal // default to local for new searches
		}
		m.refilter()

	case "l":
		if m.activePane == rightPane && m.selectedRes == resourcePods && len(m.filteredItems) > 0 {
			podName := m.filteredItems[m.itemCursor].Name()
			viewer, cmd := startLogStream(m.client, m.selectedNS, podName, m.width, m.height)
			m.logViewer = viewer
			m.viewingLogs = true
			return m, cmd
		}
	}

	return m, nil
}

// handleSearchInput processes keystrokes while the search bar is active.
// This is a clean separation — search mode has its own key handling so
// regular keys (like 'j', 'k', 'q') type into the search instead of
// triggering navigation or quit.
func (m Model) handleSearchInput(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		// Close search and clear the filter
		m.searching = false
		m.searchQuery = ""
		m.refilter()

	case "enter":
		// Close search but KEEP the filter active
		m.searching = false

	case "tab":
		// Toggle between local and global search scope
		if m.searchScope == searchLocal {
			m.searchScope = searchGlobal
		} else {
			m.searchScope = searchLocal
		}
		m.refilter()

	case "backspace":
		if len(m.searchQuery) > 0 {
			runes := []rune(m.searchQuery)
			m.searchQuery = string(runes[:len(runes)-1])
			m.refilter()
		}

	case "ctrl+c":
		return m, tea.Quit

	default:
		if len(msg.String()) == 1 {
			m.searchQuery += msg.String()
			m.refilter()
		}
	}

	return m, nil
}

func (m *Model) moveCursorUp() {
	switch {
	case m.activePane == rightPane:
		if m.itemCursor > 0 {
			m.itemCursor--
		}
	case m.leftSection == namespacesSection:
		if m.nsCursor > 0 {
			m.nsCursor--
		}
	case m.leftSection == resourcesSection:
		if m.resCursor > 0 {
			m.resCursor--
		}
	}
}

func (m *Model) moveCursorDown() {
	switch {
	case m.activePane == rightPane:
		if m.itemCursor < len(m.filteredItems)-1 {
			m.itemCursor++
		}
	case m.leftSection == namespacesSection:
		if m.nsCursor < len(m.filteredNS)-1 {
			m.nsCursor++
		}
	case m.leftSection == resourcesSection:
		if m.resCursor < len(allResourceTypes)-1 {
			m.resCursor++
		}
	}
}

func (m Model) handleEnter() (tea.Model, tea.Cmd) {
	if m.activePane == rightPane {
		return m, nil
	}

	// Use the filtered namespace list — the cursor indexes into the
	// filtered view, so we pick the right item even when filtered.
	if m.leftSection == namespacesSection && len(m.filteredNS) > 0 {
		m.selectedNS = m.filteredNS[m.nsCursor]
		m.items = nil
		m.filteredItems = nil
		return m, m.fetchResources()
	}

	if m.leftSection == resourcesSection {
		m.selectedRes = allResourceTypes[m.resCursor]
		m.selectedResIdx = m.resCursor
		m.items = nil
		m.filteredItems = nil
		return m, m.fetchResources()
	}

	return m, nil
}

// View renders the entire UI.
func (m Model) View() string {
	if m.err != nil {
		return fmt.Sprintf("Error: %v\n\nPress q to quit.\n", m.err)
	}

	if m.viewingLogs {
		return m.logViewer.view()
	}

	if m.namespaces == nil {
		return "Loading namespaces...\n"
	}

	sideWidth := m.width/3 - 2
	mainWidth := m.width - sideWidth - 6

	// Height budget calculation.
	// Each box with RoundedBorder + Padding(1,2) adds 4 lines of overhead
	// (1 top border + 1 top padding + 1 bottom padding + 1 bottom border).
	// We have: 2 sidebar boxes (8 overhead) + help bar (2 lines) = 10 lines of chrome.
	// The remaining space is split between the two sidebar content areas.
	boxOverhead := 4           // border + padding per box
	helpBarHeight := 2         // help text + margin
	resBoxContent := len(allResourceTypes) + 2 // items + title + gap
	totalChrome := (boxOverhead * 2) + helpBarHeight
	nsBoxHeight := m.height - totalChrome - resBoxContent
	resBoxHeight := resBoxContent
	if nsBoxHeight < 3 {
		nsBoxHeight = 3
	}

	nsContent := m.renderNamespaceList()
	resContent := m.renderResourceList()

	// Style each box — highlight whichever section is active
	var nsBox, resBox string
	if m.activePane == leftPane && m.leftSection == namespacesSection {
		nsBox = activePanelStyle.Width(sideWidth).Height(nsBoxHeight).Render(nsContent)
	} else {
		nsBox = panelStyle.Width(sideWidth).Height(nsBoxHeight).Render(nsContent)
	}
	if m.activePane == leftPane && m.leftSection == resourcesSection {
		resBox = activePanelStyle.Width(sideWidth).Height(resBoxHeight).Render(resContent)
	} else {
		resBox = panelStyle.Width(sideWidth).Height(resBoxHeight).Render(resContent)
	}

	sidebar := lipgloss.JoinVertical(lipgloss.Left, nsBox, resBox)

	// Main panel — its rendered height must match the sidebar's total rendered height.
	// Sidebar total rendered = (nsBoxHeight + 4) + (resBoxHeight + 4)
	// Main panel rendered = mainHeight + 4
	// So: mainHeight = nsBoxHeight + resBoxHeight + 4
	mainHeight := nsBoxHeight + resBoxHeight + boxOverhead
	mainContent := m.renderMainPanel(mainWidth)

	var mainPanel string
	if m.activePane == rightPane {
		mainPanel = activePanelStyle.Width(mainWidth).Height(mainHeight).Render(mainContent)
	} else {
		mainPanel = panelStyle.Width(mainWidth).Height(mainHeight).Render(mainContent)
	}

	panels := lipgloss.JoinHorizontal(lipgloss.Top, sidebar, mainPanel)

	// Bottom bar
	var bottomBar string
	if m.searching {
		bottomBar = m.renderSearchBar()
	} else if m.searchQuery != "" {
		scope := "local"
		if m.searchScope == searchGlobal {
			scope = "global"
		}
		bottomBar = helpStyle.Render(fmt.Sprintf(
			"filter (%s): \"%s\"  [/] edit  [esc] clear", scope, m.searchQuery,
		))
	} else {
		helpText := "[tab] switch pane  [j/k] navigate  [enter] select  [/] search"
		if m.selectedRes == resourcePods {
			helpText += "  [l] logs"
		}
		helpText += "  [q] quit"
		bottomBar = helpStyle.Render(helpText)
	}

	return panels + "\n" + bottomBar
}

// renderNamespaceList builds the content for the namespace box.
func (m Model) renderNamespaceList() string {
	title := titleStyle.Render("Namespaces")

	var items []string
	for i, ns := range m.filteredNS {
		isCursorHere := m.activePane == leftPane && m.leftSection == namespacesSection && m.nsCursor == i
		isSelected := ns == m.selectedNS

		var line string
		if isCursorHere {
			line = selectedItemStyle.Render("> " + ns)
		} else if isSelected {
			line = itemStyle.Render("* " + ns)
		} else {
			line = dimmedItemStyle.Render("  " + ns)
		}
		items = append(items, line)
	}

	return title + "\n" + strings.Join(items, "\n")
}

// renderResourceList builds the content for the resource type box.
func (m Model) renderResourceList() string {
	title := titleStyle.Render("Resources")

	var items []string
	for i, rt := range allResourceTypes {
		isCursorHere := m.activePane == leftPane && m.leftSection == resourcesSection && m.resCursor == i
		isSelected := rt == m.selectedRes

		var line string
		if isCursorHere {
			line = selectedItemStyle.Render("> " + rt.String())
		} else if isSelected {
			line = itemStyle.Render("* " + rt.String())
		} else {
			line = dimmedItemStyle.Render("  " + rt.String())
		}
		items = append(items, line)
	}

	return title + "\n" + strings.Join(items, "\n")
}

// renderMainPanel builds the right pane with a table list on top
// and details for the selected item on the bottom.
func (m Model) renderMainPanel(width int) string {
	title := titleStyle.Render(fmt.Sprintf("%s (%s)", m.selectedRes, m.selectedNS))

	if m.items == nil {
		return title + "\n" + dimmedItemStyle.Render("Loading...")
	}
	if len(m.filteredItems) == 0 {
		noResult := "No " + strings.ToLower(m.selectedRes.String()) + " found"
		if m.searchQuery != "" {
			noResult += fmt.Sprintf(" matching \"%s\"", m.searchQuery)
		}
		return title + "\n" + dimmedItemStyle.Render(noResult)
	}

	// Build table with columns.
	// First, compute column widths by scanning all rows for the widest value in each column.
	// This is a common pattern when building text tables — you need two passes:
	// 1) measure all values, 2) render with padding.
	headers := m.filteredItems[0].TableHeaders()
	colWidths := make([]int, len(headers))
	for i, h := range headers {
		colWidths[i] = len(h)
	}
	for _, item := range m.filteredItems {
		for i, val := range item.TableRow() {
			if i < len(colWidths) && len(val) > colWidths[i] {
				colWidths[i] = len(val)
			}
		}
	}

	// Render header row
	var headerCells []string
	for i, h := range headers {
		headerCells = append(headerCells, fmt.Sprintf("%-*s", colWidths[i], h))
	}
	headerLine := tableHeaderStyle.Render("  " + strings.Join(headerCells, "  "))

	// Render each resource row with health-based coloring
	var rows []string
	for i, item := range m.filteredItems {
		row := item.TableRow()
		var cells []string
		for j, val := range row {
			if j < len(colWidths) {
				cells = append(cells, fmt.Sprintf("%-*s", colWidths[j], val))
			}
		}
		line := strings.Join(cells, "  ")

		isCursor := m.activePane == rightPane && m.itemCursor == i

		if isCursor {
			// Selected row always uses the highlight style
			line = selectedItemStyle.Render("> " + line)
		} else {
			// Color based on health status
			line = "  " + line
			switch item.Health() {
			case k8s.HealthOK:
				line = healthOKStyle.Render(line)
			case k8s.HealthWarning:
				line = healthWarningStyle.Render(line)
			case k8s.HealthError:
				line = healthErrorStyle.Render(line)
			default:
				line = dimmedItemStyle.Render(line)
			}
		}
		rows = append(rows, line)
	}

	table := headerLine + "\n" + strings.Join(rows, "\n")

	// Detail panel below the table
	detailSep := dimmedItemStyle.Render(strings.Repeat("─", width-2))

	selected := m.filteredItems[m.itemCursor]
	details := selected.Details()

	var detailLines []string
	for _, row := range details {
		key := detailKeyStyle.Render(fmt.Sprintf("%-14s", row.Key))
		val := detailValStyle.Render(row.Value)
		detailLines = append(detailLines, key+val)
	}
	detailContent := strings.Join(detailLines, "\n")

	return title + "\n" + table + "\n\n" + detailSep + "\n\n" + detailContent
}

// renderSearchBar renders the search input at the bottom of the screen.
func (m Model) renderSearchBar() string {
	cursor := searchCursorStyle.Render("█")
	scope := "local"
	if m.searchScope == searchGlobal {
		scope = "global"
	}
	prompt := searchBarStyle.Render("/ " + m.searchQuery + cursor)
	hint := dimmedItemStyle.Render(fmt.Sprintf("  [tab] toggle scope (%s)  [enter] keep filter  [esc] clear", scope))
	return prompt + hint
}
