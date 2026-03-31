package ui

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	lipgloss "charm.land/lipgloss/v2"
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
	items     []k8s.Resource
	err       error
	isRefresh bool // true for auto-refresh ticks, false for user-initiated fetches
}

// editorFinishedMsg is sent when the external editor process exits.
// tea.Exec sends this through the callback we provide. The err field
// tells us whether the editor exited cleanly (err == nil) or crashed.
type editorFinishedMsg struct {
	err error
}

// yamlAppliedMsg is sent after we've applied edited YAML back to the cluster.
type yamlAppliedMsg struct {
	err error
}

// scaleReadyMsg is sent when we've fetched the current replica count and
// the scale dialog is ready to show.
type scaleReadyMsg struct {
	currentScale int
	resName      string
	namespace    string
	resType      string
	err          error
}

// scaleDoneMsg is sent after scaling completes.
type scaleDoneMsg struct {
	err error
}

// execFinishedMsg is sent when the kubectl exec process exits.
type execFinishedMsg struct {
	err error
}

// clusterStatsMsg carries refreshed cluster statistics.
type clusterStatsMsg struct {
	stats k8s.ClusterStats
}

// tickMsg triggers a periodic refresh of the resource list.
// This is how you do "reactive" UIs in Bubble Tea — there's no
// background watcher or event stream. Instead, you set a timer
// that fires a message, re-fetch the data, and schedule the next tick.
type tickMsg time.Time

// resourceType represents the kind of Kubernetes resource to display.
type resourceType int

const (
	resourcePods resourceType = iota
	resourceDeployments
	resourceStatefulSets
	resourceDaemonSets
	resourceServices
	resourceIngresses
	resourceConfigMaps
	resourceSecrets
	resourceJobs
	resourceCronJobs
	resourcePVCs
)

func (rt resourceType) String() string {
	switch rt {
	case resourcePods:
		return "Pods"
	case resourceDeployments:
		return "Deployments"
	case resourceStatefulSets:
		return "StatefulSets"
	case resourceDaemonSets:
		return "DaemonSets"
	case resourceServices:
		return "Services"
	case resourceIngresses:
		return "Ingresses"
	case resourceConfigMaps:
		return "ConfigMaps"
	case resourceSecrets:
		return "Secrets"
	case resourceJobs:
		return "Jobs"
	case resourceCronJobs:
		return "CronJobs"
	case resourcePVCs:
		return "PVCs"
	default:
		return "Unknown"
	}
}

var allResourceTypes = []resourceType{
	resourcePods,
	resourceDeployments,
	resourceStatefulSets,
	resourceDaemonSets,
	resourceServices,
	resourceIngresses,
	resourceConfigMaps,
	resourceSecrets,
	resourceJobs,
	resourceCronJobs,
	resourcePVCs,
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

	clusterStats k8s.ClusterStats
	err          error

	width  int
	height int

	viewingLogs bool
	logViewer   logViewer

	// Action menu state
	showActions bool
	actionMenu  actionMenu

	// Scale dialog state
	showScale   bool
	scaleDialog scaleDialog

	// YAML editor state — tracks the temp file and resource being edited
	// so we can apply changes when the editor exits.
	editTmpFile  string       // path to the temp file
	editResType  string       // e.g. "Pods", "Deployments"
	editResName  string       // the resource name being edited
	editResNS    string       // namespace of the resource

	// Status message — briefly shown after an action completes
	statusMsg    string

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
	return tea.Batch(m.fetchNamespaces(), m.fetchClusterStats(), m.tickCmd())
}

func (m Model) fetchClusterStats() tea.Cmd {
	client := m.client
	return func() tea.Msg {
		stats := client.GetClusterStats(context.Background())
		return clusterStatsMsg{stats: stats}
	}
}

// tickCmd returns a command that fires a tickMsg after 2 seconds.
func (m Model) tickCmd() tea.Cmd {
	return tea.Tick(2*time.Second, func(t time.Time) tea.Msg {
		return tickMsg(t)
	})
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
		case resourceStatefulSets:
			items, err = client.ListStatefulSetResources(context.Background(), ns)
		case resourceDaemonSets:
			items, err = client.ListDaemonSetResources(context.Background(), ns)
		case resourceServices:
			items, err = client.ListServiceResources(context.Background(), ns)
		case resourceIngresses:
			items, err = client.ListIngressResources(context.Background(), ns)
		case resourceConfigMaps:
			items, err = client.ListConfigMapResources(context.Background(), ns)
		case resourceSecrets:
			items, err = client.ListSecretResources(context.Background(), ns)
		case resourceJobs:
			items, err = client.ListJobResources(context.Background(), ns)
		case resourceCronJobs:
			items, err = client.ListCronJobResources(context.Background(), ns)
		case resourcePVCs:
			items, err = client.ListPVCResources(context.Background(), ns)
		}

		return resourceMsg{items: items, err: err}
	}
}

// refreshResources is like fetchResources but marks the message as a refresh
// so Update() knows not to reset the cursor position.
func (m Model) refreshResources() tea.Cmd {
	client := m.client
	ns := m.selectedNS
	resType := m.selectedRes
	return func() tea.Msg {
		var items []k8s.Resource
		var err error

		switch resType {
		case resourcePods:
			items, err = client.ListPodResources(context.Background(), ns)
		case resourceDeployments:
			items, err = client.ListDeploymentResources(context.Background(), ns)
		case resourceStatefulSets:
			items, err = client.ListStatefulSetResources(context.Background(), ns)
		case resourceDaemonSets:
			items, err = client.ListDaemonSetResources(context.Background(), ns)
		case resourceServices:
			items, err = client.ListServiceResources(context.Background(), ns)
		case resourceIngresses:
			items, err = client.ListIngressResources(context.Background(), ns)
		case resourceConfigMaps:
			items, err = client.ListConfigMapResources(context.Background(), ns)
		case resourceSecrets:
			items, err = client.ListSecretResources(context.Background(), ns)
		case resourceJobs:
			items, err = client.ListJobResources(context.Background(), ns)
		case resourceCronJobs:
			items, err = client.ListCronJobResources(context.Background(), ns)
		case resourcePVCs:
			items, err = client.ListPVCResources(context.Background(), ns)
		}

		return resourceMsg{items: items, err: err, isRefresh: true}
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

	case editorFinishedMsg:
		// The editor has exited. If it exited cleanly, read the temp file
		// and apply the edited YAML back to the cluster.
		if msg.err != nil {
			m.statusMsg = fmt.Sprintf("Editor error: %v", msg.err)
			m.cleanupTmpFile()
			return m, nil
		}
		// Read the edited file and apply it
		return m, m.applyEditedYAML()

	case openEditorMsg:
		// We got the temp file — now launch the editor via tea.Exec.
		// This suspends our TUI and gives the terminal to the editor.
		//
		// os.Getenv("EDITOR") respects the user's preference. Most
		// developers set this in their shell profile. We fall back to
		// "vi" which is available on virtually every Unix system.
		m.editTmpFile = msg.tmpFile
		editor := os.Getenv("EDITOR")
		if editor == "" {
			editor = "vi"
		}
		cmd := exec.Command(editor, msg.tmpFile)
		return m, tea.ExecProcess(cmd, func(err error) tea.Msg {
			return editorFinishedMsg{err: err}
		})

	case yamlAppliedMsg:
		m.cleanupTmpFile()
		if msg.err != nil {
			m.statusMsg = fmt.Sprintf("Apply failed: %v", msg.err)
		} else {
			m.statusMsg = fmt.Sprintf("%s applied successfully", m.editResName)
		}
		return m, m.fetchResources()

	case scaleReadyMsg:
		if msg.err != nil {
			m.statusMsg = fmt.Sprintf("Scale error: %v", msg.err)
			return m, nil
		}
		sd, cmd := newScaleDialog(msg.resName, msg.namespace, msg.resType, msg.currentScale)
		m.scaleDialog = sd
		m.showScale = true
		return m, cmd

	case execFinishedMsg:
		if msg.err != nil {
			m.statusMsg = fmt.Sprintf("Exec error: %v", msg.err)
		}
		// TUI resumes automatically after the shell exits.
		return m, nil

	case scaleDoneMsg:
		m.showScale = false
		if msg.err != nil {
			m.statusMsg = fmt.Sprintf("Scale failed: %v", msg.err)
		} else {
			m.statusMsg = fmt.Sprintf("Scaled %s successfully", m.scaleDialog.resourceName)
		}
		return m, m.fetchResources()

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
		// Only reset cursor on explicit user actions (changing namespace/resource type),
		// not on auto-refresh ticks. We distinguish by checking if the message
		// came with the refresh flag.
		if !msg.isRefresh {
			m.itemCursor = 0
		}
		m.refilter()
		return m, nil

	case clusterStatsMsg:
		m.clusterStats = msg.stats
		return m, nil

	case tickMsg:
		// Don't refresh while overlays are open or logs are streaming —
		// it would cause flicker or interfere with the user's interaction.
		if m.viewingLogs || m.showActions || m.showScale {
			return m, m.tickCmd()
		}
		return m, tea.Batch(m.refreshResources(), m.fetchClusterStats(), m.tickCmd())

	case tea.KeyMsg:
		// Overlay delegation — whichever overlay is active "owns" the input.
		// This is a state machine pattern: check overlays in priority order,
		// delegate, and return early. The main key handler only runs when
		// no overlay is active. This keeps each handler focused and simple.
		if m.viewingLogs {
			viewer, closed, cmd := m.logViewer.update(msg)
			m.logViewer = viewer
			if closed {
				m.viewingLogs = false
			}
			return m, cmd
		}
		if m.showActions {
			return m.handleActionMenu(msg)
		}
		if m.showScale {
			return m.handleScaleDialog(msg)
		}
		return m.handleKeyPress(msg)
	}

	return m, nil
}

func (m Model) handleKeyPress(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	// Clear any status message on the next keypress
	m.statusMsg = ""

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
	if m.activePane == rightPane && len(m.filteredItems) > 0 {
		// Open the action menu for the selected resource.
		// newActionMenu creates the menu with context-appropriate actions
		// based on the resource type — pods get "view logs" and "exec",
		// deployments get "scale", etc.
		selected := m.filteredItems[m.itemCursor]
		m.actionMenu = newActionMenu(selected, m.selectedRes, m.selectedNS)
		m.showActions = true
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

// handleActionMenu delegates keypresses to the action menu overlay.
//
// Notice the three return values from am.update(): the updated menu, whether
// to close, and which action key was selected. This "multi-return" pattern
// lets the caller decide what to do with the results — the action menu itself
// doesn't know anything about pods, logs, or deletion. It just reports back
// "the user picked 'l'" and the parent decides what that means.
// This separation of concerns keeps the action menu reusable.
func (m Model) handleActionMenu(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	menu, closed, selectedKey := m.actionMenu.update(msg)
	m.actionMenu = menu

	if !closed {
		return m, nil
	}

	// Menu is closing — either cancelled (selectedKey == "") or an action was picked
	m.showActions = false

	if selectedKey == "" {
		return m, nil
	}

	// Dispatch the selected action.
	switch selectedKey {
	case "l":
		// View logs — only makes sense for pods and jobs
		if m.selectedRes == resourcePods || m.selectedRes == resourceJobs {
			podName := m.actionMenu.resource.Name()
			viewer, cmd := startLogStream(m.client, m.selectedNS, podName, m.width, m.height)
			m.logViewer = viewer
			m.viewingLogs = true
			return m, cmd
		}

	case "y":
		// View/edit YAML — opens the user's $EDITOR on a temp file.
		// tea.Exec is the key concept here: it SUSPENDS the Bubble Tea
		// program, hands the terminal over to the child process (your
		// editor), and RESUMES the TUI when the process exits. The
		// callback function wraps the result in our editorFinishedMsg
		// so Update() can handle it.
		return m, m.openYAMLEditor()

	case "e":
		// Exec into pod — runs `kubectl exec -it` via tea.ExecProcess.
		// This is the same pattern as the YAML editor: suspend the TUI,
		// hand the terminal to an interactive process, resume when it exits.
		//
		// We use kubectl rather than client-go's remotecommand package
		// because kubectl handles all the terminal setup (TTY allocation,
		// signal forwarding, resize events) that would be complex to
		// replicate. It's the right tool for the job.
		podName := m.actionMenu.resource.Name()
		ns := m.actionMenu.namespace
		cmd := exec.Command("kubectl", "exec", "-it", podName, "-n", ns, "--", "/bin/sh")
		return m, tea.ExecProcess(cmd, func(err error) tea.Msg {
			return execFinishedMsg{err: err}
		})

	case "d":
		// Delete resource
		return m, m.deleteResource()

	case "r":
		// Restart — for pods this deletes the pod (controller recreates it),
		// for deployments/statefulsets/daemonsets it does a rollout restart.
		return m, m.restartResource()

	case "s":
		// Scale — fetch current replicas first, then show the scale dialog.
		// We fetch async because it's an API call that could take time.
		resType := m.selectedRes.String()
		resName := m.actionMenu.resource.Name()
		ns := m.actionMenu.namespace
		client := m.client
		return m, func() tea.Msg {
			replicas, err := client.GetReplicas(context.Background(), resType, ns, resName)
			return scaleReadyMsg{
				currentScale: replicas,
				resName:      resName,
				namespace:    ns,
				resType:      resType,
				err:          err,
			}
		}
	}

	return m, nil
}

// handleScaleDialog delegates keypresses to the scale input overlay.
func (m Model) handleScaleDialog(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	sd, closed, replicas, cmd := m.scaleDialog.update(msg)
	m.scaleDialog = sd

	if !closed {
		return m, cmd
	}

	// Cancelled
	if replicas < 0 {
		m.showScale = false
		return m, nil
	}

	// Apply the new scale
	resType := m.scaleDialog.resType
	resName := m.scaleDialog.resourceName
	ns := m.scaleDialog.namespace
	client := m.client

	return m, func() tea.Msg {
		err := client.ScaleResource(context.Background(), resType, ns, resName, replicas)
		return scaleDoneMsg{err: err}
	}
}

// openYAMLEditor fetches the resource YAML, writes it to a temp file, and
// opens the user's editor. When the editor exits, editorFinishedMsg is sent.
func (m *Model) openYAMLEditor() tea.Cmd {
	resType := m.selectedRes.String()
	resName := m.actionMenu.resource.Name()
	ns := m.actionMenu.namespace

	// Save edit context so we know what to apply when the editor exits
	m.editResType = resType
	m.editResName = resName
	m.editResNS = ns

	client := m.client
	return func() tea.Msg {
		// Fetch the YAML
		yamlContent, err := client.GetResourceYAML(context.Background(), resType, ns, resName)
		if err != nil {
			return editorFinishedMsg{err: err}
		}

		// Write to a temp file. os.CreateTemp gives us a unique filename
		// in the OS temp directory — no collisions even if multiple
		// instances are running.
		tmpFile, err := os.CreateTemp("", fmt.Sprintf("kube-tui-%s-*.yaml", resName))
		if err != nil {
			return editorFinishedMsg{err: fmt.Errorf("creating temp file: %w", err)}
		}

		if _, err := tmpFile.WriteString(yamlContent); err != nil {
			tmpFile.Close()
			return editorFinishedMsg{err: fmt.Errorf("writing YAML: %w", err)}
		}
		tmpFile.Close()

		// We can't use tea.Exec from inside a Cmd (we're already in a
		// goroutine). Instead, return a special message that tells Update
		// to issue the tea.Exec command.
		return openEditorMsg{tmpFile: tmpFile.Name()}
	}
}

// openEditorMsg signals that we need to launch the editor.
// We need this intermediate message because tea.Exec must be returned
// from Update(), not from inside a Cmd goroutine.
type openEditorMsg struct {
	tmpFile string
}

// applyEditedYAML reads the temp file and applies it to the cluster.
func (m Model) applyEditedYAML() tea.Cmd {
	tmpFile := m.editTmpFile
	resType := m.editResType
	resName := m.editResName
	ns := m.editResNS
	client := m.client

	return func() tea.Msg {
		data, err := os.ReadFile(tmpFile)
		if err != nil {
			return yamlAppliedMsg{err: fmt.Errorf("reading edited file: %w", err)}
		}
		err = client.ApplyYAML(context.Background(), resType, ns, resName, data)
		return yamlAppliedMsg{err: err}
	}
}

// cleanupTmpFile removes the temp file used for YAML editing.
func (m *Model) cleanupTmpFile() {
	if m.editTmpFile != "" {
		os.Remove(m.editTmpFile)
		m.editTmpFile = ""
	}
}

// deleteResource deletes the selected resource from the cluster.
func (m Model) deleteResource() tea.Cmd {
	resType := m.selectedRes.String()
	resName := m.actionMenu.resource.Name()
	ns := m.actionMenu.namespace
	client := m.client

	return func() tea.Msg {
		err := client.DeleteResource(context.Background(), resType, ns, resName)
		if err != nil {
			return yamlAppliedMsg{err: err}
		}
		return yamlAppliedMsg{err: nil}
	}
}

// restartResource restarts pods (by deleting) or triggers a rollout restart
// for deployments, statefulsets, and daemonsets.
func (m Model) restartResource() tea.Cmd {
	resType := m.selectedRes.String()
	resName := m.actionMenu.resource.Name()
	ns := m.actionMenu.namespace
	client := m.client

	return func() tea.Msg {
		var err error
		switch resType {
		case "Pods":
			// Restarting a pod = deleting it. The controller (Deployment,
			// ReplicaSet, etc.) will recreate it automatically.
			err = client.DeleteResource(context.Background(), resType, ns, resName)
		case "Deployments", "StatefulSets", "DaemonSets":
			err = client.RolloutRestart(context.Background(), resType, ns, resName)
		}
		return yamlAppliedMsg{err: err}
	}
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

	// Render the header bar — shows the cluster context so you always know
	// where you're connected. This is important in Kubernetes because
	// running commands against the wrong cluster is a classic footgun.
	header := m.renderHeader()

	sideWidth := m.width / 3
	mainWidth := m.width - sideWidth

	// Height budget calculation.
	// renderPanel adds 2 lines of overhead per box (top border + bottom border).
	// We have: header (1) + help bar (1) + 3 panels (6 overhead total for sidebar's 2 + main's 2... but
	// sidebar has 2 panels stacked = 4 lines of border). Main panel = 2 lines of border.
	// Total chrome = 1 (header) + 4 (sidebar borders) + 1 (help) = 6
	headerHeight := 1
	borderOverhead := 2        // top + bottom border per panel
	helpBarHeight := 1
	resBoxContent := len(allResourceTypes) // just the items, no title (it's in the border)
	totalSidebarBorders := borderOverhead * 2 // 2 sidebar panels
	totalChrome := headerHeight + totalSidebarBorders + helpBarHeight
	nsBoxHeight := m.height - totalChrome - resBoxContent
	resBoxHeight := resBoxContent
	if nsBoxHeight < 3 {
		nsBoxHeight = 3
	}

	nsContent := m.renderNamespaceList()
	resContent := m.renderResourceList()

	// Use renderPanel for lazygit-style borders with embedded titles and counters
	nsActive := m.activePane == leftPane && m.leftSection == namespacesSection
	resActive := m.activePane == leftPane && m.leftSection == resourcesSection

	nsCursor := m.nsCursor
	if len(m.filteredNS) == 0 {
		nsCursor = -1
	}
	nsBox := renderPanel("Namespaces", nsContent, sideWidth, nsBoxHeight+borderOverhead, nsActive, nsCursor, len(m.filteredNS))
	resBox := renderPanel("Resources", resContent, sideWidth, resBoxHeight+borderOverhead, resActive, m.resCursor, len(allResourceTypes))

	sidebar := lipgloss.JoinVertical(lipgloss.Left, nsBox, resBox)

	// Main panel — its rendered height must match the sidebar's total rendered height.
	// Sidebar outer = (nsBoxHeight + 2) + (resBoxHeight + 2) = nsBoxHeight + resBoxHeight + 4
	// Main outer = mainHeight + 2, so mainHeight = nsBoxHeight + resBoxHeight + 2
	mainHeight := nsBoxHeight + resBoxHeight + borderOverhead
	mainContent := m.renderMainPanel(mainWidth, mainHeight)

	mainTitle := fmt.Sprintf("%s (%s)", m.selectedRes, m.selectedNS)
	mainActive := m.activePane == rightPane
	mainCursor := m.itemCursor
	if len(m.filteredItems) == 0 {
		mainCursor = -1
	}
	mainPanel := renderPanel(mainTitle, mainContent, mainWidth, mainHeight+borderOverhead, mainActive, mainCursor, len(m.filteredItems))

	panels := lipgloss.JoinHorizontal(lipgloss.Top, sidebar, mainPanel)

	// Bottom bar — contextual help that changes based on state
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
	} else if m.statusMsg != "" {
		bottomBar = helpStyle.Foreground(special).Render(m.statusMsg)
	} else {
		helpText := "[tab] switch pane  [j/k] navigate  [enter] actions  [/] search  [q] quit"
		bottomBar = helpStyle.Render(helpText)
	}

	// Compose the full screen: header + panels + help bar
	screen := header + "\n" + panels + "\n" + bottomBar

	// If the action menu is open, composite it on top of the existing UI
	// using lipgloss v2's Compositor.
	//
	// Why Compositor and not Canvas.Compose? Canvas.Compose calls
	// Layer.Draw(canvas, canvas.Bounds()), which ignores the Layer's own
	// X/Y position. The Compositor, on the other hand, flattens the layer
	// hierarchy, calculates absolute positions, sorts by z-index, and
	// draws each layer at its correct bounds. It's the piece of the API
	// that actually does spatial composition.
	//
	// Think of it like this:
	//   - Layer = "what to draw and where"
	//   - Compositor = "the engine that draws layers in the right order"
	//   - Canvas = "the pixel buffer being drawn onto"
	if m.showActions {
		menuBox := m.actionMenu.view()
		menuW := lipgloss.Width(menuBox)
		menuH := lipgloss.Height(menuBox)

		// Center the menu
		menuX := (m.width - menuW) / 2
		menuY := (m.height - menuH) / 2

		bg := lipgloss.NewLayer(screen)
		fg := lipgloss.NewLayer(menuBox).X(menuX).Y(menuY).Z(1)

		return lipgloss.NewCompositor(bg, fg).Render()
	}

	// Scale dialog overlay — same compositor pattern
	if m.showScale {
		dialogBox := m.scaleDialog.view()
		dw := lipgloss.Width(dialogBox)
		dh := lipgloss.Height(dialogBox)
		dx := (m.width - dw) / 2
		dy := (m.height - dh) / 2

		bg := lipgloss.NewLayer(screen)
		fg := lipgloss.NewLayer(dialogBox).X(dx).Y(dy).Z(1)
		return lipgloss.NewCompositor(bg, fg).Render()
	}

	return screen
}

// renderHeader builds the top bar showing cluster context.
func (m Model) renderHeader() string {
	logo := headerStyle.Render(fmt.Sprintf("%s kube-tui", symbolK8s))
	cluster := headerClusterStyle.Render(m.client.ClusterName)
	ctx := headerDimStyle.Render(fmt.Sprintf("ctx: %s", m.client.ContextName))

	left := fmt.Sprintf("%s  %s  %s", logo, cluster, ctx)

	// Right side: cluster stats
	s := m.clusterStats
	stats := headerDimStyle.Render(fmt.Sprintf(
		"nodes: %d  pods: %d", s.NodeCount, s.PodCount,
	))

	if s.MetricsAvailable && s.CPUTotalMillis > 0 {
		cpuPct := float64(s.CPUUsedMillis) / float64(s.CPUTotalMillis) * 100
		memPct := float64(s.MemUsedBytes) / float64(s.MemTotalBytes) * 100
		stats = headerDimStyle.Render(fmt.Sprintf(
			"nodes: %d  pods: %d  cpu: %.0f%%  mem: %.0f%%",
			s.NodeCount, s.PodCount, cpuPct, memPct,
		))
	}

	// Pad the gap between left and right to fill the full width.
	gap := m.width - lipgloss.Width(left) - lipgloss.Width(stats)
	if gap < 2 {
		gap = 2
	}
	bar := left + strings.Repeat(" ", gap) + stats

	return lipgloss.NewStyle().
		Width(m.width).
		Background(headerBg).
		Render(bar)
}

// renderNamespaceList builds the content for the namespace box (no title — it goes in the border).
func (m Model) renderNamespaceList() string {
	var items []string
	for i, ns := range m.filteredNS {
		isCursorHere := m.activePane == leftPane && m.leftSection == namespacesSection && m.nsCursor == i
		isSelected := ns == m.selectedNS

		var line string
		if isCursorHere {
			line = selectedItemStyle.Render(symbolCursor + " " + ns)
		} else if isSelected {
			line = itemStyle.Render(symbolSelected + " " + ns)
		} else {
			line = dimmedItemStyle.Render("  " + ns)
		}
		items = append(items, line)
	}

	return strings.Join(items, "\n")
}

// renderResourceList builds the content for the resource type box (no title — it goes in the border).
func (m Model) renderResourceList() string {
	var items []string
	for i, rt := range allResourceTypes {
		isCursorHere := m.activePane == leftPane && m.leftSection == resourcesSection && m.resCursor == i
		isSelected := rt == m.selectedRes

		var line string
		if isCursorHere {
			line = selectedItemStyle.Render(symbolCursor + " " + rt.String())
		} else if isSelected {
			line = itemStyle.Render(symbolSelected + " " + rt.String())
		} else {
			line = dimmedItemStyle.Render("  " + rt.String())
		}
		items = append(items, line)
	}

	return strings.Join(items, "\n")
}

// renderMainPanel builds the right pane content — table on top, details
// pushed to the bottom. No title here; it goes in the border.
func (m Model) renderMainPanel(width, height int) string {
	if m.items == nil {
		return dimmedItemStyle.Render("Loading...")
	}
	if len(m.filteredItems) == 0 {
		noResult := "No " + strings.ToLower(m.selectedRes.String()) + " found"
		if m.searchQuery != "" {
			noResult += fmt.Sprintf(" matching \"%s\"", m.searchQuery)
		}
		return dimmedItemStyle.Render(noResult)
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
			line = selectedItemStyle.Render(symbolCursor + " " + line)
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

	// Detail panel — pushed to the bottom of the panel.
	detailSep := dimmedItemStyle.Render(strings.Repeat("─", width-4))

	selected := m.filteredItems[m.itemCursor]
	details := selected.Details()

	var detailLines []string
	for _, row := range details {
		key := detailKeyStyle.Render(fmt.Sprintf("%-14s", row.Key))
		val := detailValStyle.Render(row.Value)
		detailLines = append(detailLines, key+val)
	}
	detailContent := strings.Join(detailLines, "\n")

	// Calculate how many blank lines we need to push details to the bottom.
	// height = total inner height available in the panel
	tableLines := 1 + len(rows) // header + rows
	detailBlockLines := 1 + 1 + len(detailLines) // separator + gap + detail rows
	gap := height - tableLines - detailBlockLines - 1
	if gap < 1 {
		gap = 1
	}

	return table + "\n" + strings.Repeat("\n", gap) + detailSep + "\n" + detailContent
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
