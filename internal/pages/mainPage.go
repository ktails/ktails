// Package pages, it implements main routing to different pages.
package pages

import (
	"bufio"
	"fmt"
	"io"
	"log"
	"os"
	"sort"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/term"

	"github.com/ktails/ktails/internal/k8s"
	"github.com/ktails/ktails/internal/state"
	"github.com/ktails/ktails/internal/tui/cmds"
	"github.com/ktails/ktails/internal/tui/models"
	"github.com/ktails/ktails/internal/tui/msgs"
	"github.com/ktails/ktails/internal/tui/styles"
	"github.com/ktails/ktails/internal/tui/views"
	"github.com/ktails/ktails/internal/tui/watch"
)

type focusTarget int

const (
	focusLeftPane focusTarget = iota
	focusTabs
)

// leftSection identifies which of the left sidebar's stacked sections
// currently has keyboard focus while focusLeftPane is active — cycled with
// "[" / "]", the same keys that cycle resource-kind tabs while focusTabs is
// active.
type leftSection int

const (
	sectionContexts leftSection = iota
	sectionClusters
	sectionNamespaces
)

type MainPage struct {
	// dimensions
	width  int
	height int

	// tabs — one per resource kind, in msgs.Kinds() order
	tabs      []msgs.ResourceKind
	activeTab int

	// App state — per-context UI status (selection, loading, errors). Rows
	// live in the watch Supervisor's caches, not here.
	appState       *state.AppState
	appStateLoaded bool

	// base models
	contextList *models.ContextsInfo
	// clusterList groups contextList's contexts by kubeconfig cluster for
	// bulk select/deselect — a second left-pane section sharing the
	// Contexts pane's own selection/confirm state, not a separate data
	// source.
	clusterList *models.ClustersInfo
	// namespacesPane lets each selected context's checked namespace set grow
	// beyond the kubeconfig default it starts with (see
	// state.AppState.AddContext) — a display-only filter over each
	// context's already cluster-wide watch (see filteredRows), not
	// something that starts or stops a watch.
	namespacesPane *models.NamespacesInfo
	// activeLeftSection is which stacked left-pane section ("[" / "]"
	// cycle it) is focused while focus == focusLeftPane.
	activeLeftSection leftSection
	// tables holds the one resource table per tab; all three share the
	// models.ResourceTable implementation and differ only by spec.
	tables           map[msgs.ResourceKind]*models.ResourceTable
	deploymentDetail *models.ResourceDetailPage
	focus            focusTarget

	// k8s client
	Client *k8s.Client

	// watchSup owns every watch stream, cache, and reconnect decision —
	// MainPage only starts/stops contexts, forwards watch messages to
	// Handle, and reads rows back out.
	watchSup *watch.Supervisor

	// UI overlays
	errorMessage string
	showHelp     bool

	// Auto-refresh — a self-rescheduling tick. Table data itself is kept
	// current by the watch streams; the tick's only remaining job is to
	// re-render Age text from the local watch caches (no API calls). Paused
	// (tick still reschedules, but the re-render is skipped) while the Detail
	// or Log pane is open, since a background reorder under a pinned pane is
	// more disruptive than helpful.
	autoRefresh     bool
	refreshInterval time.Duration

	// Detail pane — a cross-cutting bottom split opened by Enter from
	// Deployments or Pods. It's not a peer tab: it stays put, splitting
	// whichever top tab's content area you're currently on.
	showDetail    bool
	detailFocused bool

	// Log pane — a second cross-cutting bottom split, reachable with `l` on
	// one or more checked Pods rows (or the row under the cursor, if none
	// are checked). Mutually exclusive with the Detail pane: opening one
	// closes the other. Every checked pod's containers become one source
	// each, merged into a single scrollback; logStreams holds the live
	// stream/scanner/generation per source, keyed the same way as
	// podLogs' sources. Generation guards against messages from a
	// since-superseded stream for that specific source (old pod switched
	// out, or the whole pane closed) without affecting other open sources.
	podLogs     *models.LogPage
	showLogs    bool
	logsFocused bool
	logStreams  map[string]*logStreamState

	tableW, tableH int

	// sortField/sortDir hold the currently active column sort (applied by
	// filteredRows, last, after namespace filtering) — Shift+N/M/A cycle
	// sortField through name/namespace/age and sortDir through
	// ascending/descending/off. sortNone (the zero value) is a true no-op
	// that preserves each cache's existing context/namespace/name order.
	sortField sortField
	sortDir   sortDir

	// lastMetricsFetch throttles fetchMetricsIfNeeded well below the
	// refresh tick's cadence — metrics-server itself only refreshes
	// internally every ~60s by default, so fetching on every tick
	// (default 5s) would be pure waste.
	lastMetricsFetch time.Time
}

// metricsFetchInterval is the minimum time between fetchMetricsIfNeeded
// dispatches — see lastMetricsFetch.
const metricsFetchInterval = 15 * time.Second

// sortField identifies which column, if any, rows are currently ordered by.
type sortField int

const (
	sortNone sortField = iota
	sortByName
	sortByNamespace
	sortByAge
)

// sortDir is the direction of the active sortField.
type sortDir int

const (
	sortAsc sortDir = iota
	sortDesc
)

// logStreamState is the live stream-plumbing state for one open log
// source, keyed by the same context/namespace/pod/container key used in
// models.LogPage.
type logStreamState struct {
	stream     io.ReadCloser
	scanner    *bufio.Scanner
	generation int
}

// NewMainPageModel builds the top-level page model. refreshIntervalSeconds is
// config.Preferences.RefreshInterval — the auto-refresh tick period; values
// below 1 fall back to 5s (the same default as config.DefaultConfig).
func NewMainPageModel(c *k8s.Client, refreshIntervalSeconds int) *MainPage {
	if refreshIntervalSeconds < 1 {
		refreshIntervalSeconds = 5
	}

	tables := make(map[msgs.ResourceKind]*models.ResourceTable)
	for _, kind := range msgs.Kinds() {
		tables[kind] = models.NewResourceTable(kind)
	}

	contextList := models.NewContextInfo(c)

	m := &MainPage{
		Client:           c,
		appState:         state.NewAppState(),
		tabs:             msgs.Kinds(),
		contextList:      contextList,
		clusterList:      models.NewClustersInfo(contextList),
		namespacesPane:   models.NewNamespacesInfo(),
		tables:           tables,
		deploymentDetail: models.NewResourceDetailPage(),
		podLogs:          models.NewLogPage(),
		logStreams:       make(map[string]*logStreamState),
		watchSup:         watch.NewSupervisor(c),
		appStateLoaded:   false,
		focus:            focusLeftPane,
		errorMessage:     "",
		showHelp:         false,
		autoRefresh:      true,
		refreshInterval:  time.Duration(refreshIntervalSeconds) * time.Second,
	}

	m.updateFocusStates()
	return m
}

// activeKind returns the resource kind of the active tab.
func (m *MainPage) activeKind() msgs.ResourceKind {
	return m.tabs[m.activeTab]
}

// activeTable returns the active tab's resource table.
func (m *MainPage) activeTable() *models.ResourceTable {
	return m.tables[m.activeKind()]
}

// renderTabStrip renders a row of tab titles with the active one accented
// (or merely brightened, when the surrounding pane isn't focused) — the
// "Deployments · Pods · svc" look. Shared by the Tab Area (resource kinds)
// and the Context List (Contexts/Namespaces/Clusters), both of which embed
// their tab strip directly in their box's border.
func (m *MainPage) renderTabStrip(titles []string, active int, focused bool) string {
	p := styles.CatppuccinMocha()
	sep := lipgloss.NewStyle().Foreground(p.Overlay0).Render(" · ")
	parts := make([]string, 0, len(titles))
	for i, title := range titles {
		st := lipgloss.NewStyle().Foreground(p.Overlay1)
		if i == active {
			st = st.Bold(true).Foreground(p.Subtext1)
			if focused {
				st = st.Foreground(styles.FocusColor)
			}
		}
		parts = append(parts, st.Render(title))
	}
	return strings.Join(parts, sep)
}

// tabTitles returns the Tab Area's tab labels in tab order.
func (m *MainPage) tabTitles() []string {
	titles := make([]string, len(m.tabs))
	for i, kind := range m.tabs {
		titles[i] = kind.Title()
	}
	return titles
}

// renderTabTitle renders the Tab Area box's top-border tab strip — the full
// "Deployments · Pods · svc · ..." when boxW has room for it (see
// views.FitsTitle), or just the active tab's name with ◂ ▸ hints when it
// doesn't (a narrow terminal with all six resource kinds as tabs). This is
// the tab bar's overflow strategy (§8.5): rather than wrapping to a second
// line (which would compete with the sidebar's vertical space and reflow
// the whole header on resize), a too-narrow strip collapses to just the
// active tab — always legible, never silently truncated mid-strip — and
// [ / ] (already bound to tab switching, see the Update handler) moves
// through the same set one at a time regardless of which mode is showing.
func (m *MainPage) renderTabTitle(focused bool, boxW int) string {
	full := m.renderTabStrip(m.tabTitles(), m.activeTab, focused)
	if views.FitsTitle(full, boxW) {
		return full
	}

	p := styles.CatppuccinMocha()
	activeStyle := lipgloss.NewStyle().Bold(true).Foreground(p.Subtext1)
	if focused {
		activeStyle = activeStyle.Foreground(styles.FocusColor)
	}
	hint := lipgloss.NewStyle().Foreground(p.Overlay0)
	return hint.Render("◂ ") + activeStyle.Render(m.activeKind().Title()) + hint.Render(" ▸")
}

func (m *MainPage) Init() tea.Cmd {
	m.contextList.Init()
	return tea.Batch(m.refreshTickCmd(), recheckStartupSizeCmd())
}

// refreshTickCmd schedules the next RefreshTickMsg one refreshInterval from
// now — the standard bubbletea self-rescheduling tick pattern. Scheduled
// unconditionally, even while auto-refresh is paused or toggled off, so it's
// always running in the background and ready to pick refreshing back up.
func (m *MainPage) refreshTickCmd() tea.Cmd {
	return tea.Tick(m.refreshInterval, func(time.Time) tea.Msg {
		return msgs.RefreshTickMsg{}
	})
}

// startupResizeCheckDelay is how long after Init() to re-verify the real
// terminal size against whatever WindowSizeMsg bubbletea delivered at
// startup.
const startupResizeCheckDelay = 250 * time.Millisecond

// recheckStartupSizeCmd re-queries the terminal's actual size shortly after
// startup and delivers it as a fresh WindowSizeMsg. bubbletea reads the
// initial size via one synchronous ioctl before the first frame renders,
// then only ever corrects it again on a real SIGWINCH. Some terminal
// emulators (observed with Ghostty on macOS, launched directly into a
// tiled/half-screen layout) haven't finished settling their own window
// geometry at that instant, so the ioctl reads a stale size and — because
// no further physical resize happens on its own — the layout stays wrong
// until the user manually triggers one. This fires once to catch that race;
// if the size didn't actually change, the resulting WindowSizeMsg is a
// harmless no-op.
func recheckStartupSizeCmd() tea.Cmd {
	return tea.Tick(startupResizeCheckDelay, func(time.Time) tea.Msg {
		w, h, err := term.GetSize(os.Stdout.Fd())
		if err != nil {
			return nil
		}
		return tea.WindowSizeMsg{Width: w, Height: h}
	})
}

func (m *MainPage) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	start := time.Now()
	defer m.logSlowUpdate(start, msg)

	// Watch messages are the Supervisor's to interpret; MainPage only
	// applies the resulting row/error updates to the UI.
	if upd, cmd, handled := m.watchSup.Handle(msg); handled {
		m.applyWatchUpdate(upd)
		return m, cmd
	}

	switch msg := msg.(type) {
	case tea.KeyPressMsg:
		keypress := msg.String()

		// Help overlay is modal — only ? and esc pass through
		if m.showHelp {
			if keypress == "?" || keypress == "esc" {
				m.showHelp = false
			}
			return m, nil
		}

		// While a resource table is actively capturing filter text (see
		// rowFilter in models/table.go), every keypress must reach it
		// untouched — otherwise single-letter global shortcuts like "r"
		// (refresh) or "l" (open logs) below would get swallowed into a
		// command instead of becoming part of the filter query.
		if m.focus == focusTabs && !m.detailFocused && !m.logsFocused {
			if _, _, typing, ok := m.activeTable().FilterStatus(); ok && typing {
				return m, m.activeTable().Update(msg)
			}
		}

		// Same protection for the Namespaces pane's own "/" filter (see
		// models.NamespacesInfo) — otherwise "R" (auto-refresh toggle) or
		// "?" (help) below would swallow those characters out of the query.
		if m.focus == focusLeftPane && m.activeLeftSection == sectionNamespaces {
			if _, _, typing, ok := m.namespacesPane.FilterStatus(); ok && typing {
				return m, m.namespacesPane.Update(msg)
			}
		}

		// Global keys
		switch keypress {
		case "ctrl+c":
			m.stopLogStream()
			m.watchSup.Shutdown()
			return m, tea.Quit
		case "tab", "shift+tab":
			m.toggleFocus()
			return m, nil
		case "esc":
			// Peel dismissals one at a time: unfocus the detail/log pane, then
			// close it, then inline error, then context errors. Detail and Logs
			// are mutually exclusive, so only one of their branches is ever live.
			if m.detailFocused {
				m.detailFocused = false
				m.updateFocusStates()
			} else if m.logsFocused {
				m.logsFocused = false
				m.updateFocusStates()
			} else if m.showDetail {
				m.showDetail = false
				m.applyContentSizes()
			} else if m.showLogs {
				m.closeLogs()
				m.applyContentSizes()
			} else if m.errorMessage != "" {
				m.errorMessage = ""
			} else {
				m.appState.ClearErrors()
			}
			return m, nil
		case "?":
			m.showHelp = true
			return m, nil
		case "R":
			m.autoRefresh = !m.autoRefresh
			return m, nil
		}

		// Left sidebar keys: "[" / "]" cycle which stacked section has
		// focus (mirroring the same keys' resource-tab-cycling meaning while
		// focusTabs is active instead), everything else forwards to
		// whichever section is currently focused.
		if m.focus == focusLeftPane {
			switch keypress {
			case "]":
				m.activeLeftSection = (m.activeLeftSection + 1) % (sectionNamespaces + 1)
				m.updateFocusStates()
				return m, nil
			case "[":
				m.activeLeftSection = (m.activeLeftSection - 1 + sectionNamespaces + 1) % (sectionNamespaces + 1)
				m.updateFocusStates()
				return m, nil
			}
			switch m.activeLeftSection {
			case sectionClusters:
				return m, m.clusterList.Update(msg)
			case sectionNamespaces:
				return m, m.namespacesPane.Update(msg)
			default:
				return m, m.contextList.Update(msg)
			}
		}

		// While the detail pane has keyboard focus, it captures everything
		// (arrows/j-k/pgup/pgdn/g/G) until Esc hands focus back to the list.
		if m.detailFocused {
			cmd := m.deploymentDetail.Update(msg)
			return m, cmd
		}

		// While the log pane has keyboard focus, it captures everything except
		// 'c' and 'w', which MainPage intercepts directly — both are pure view
		// toggles with no stream side effects (isolate/return-to-merged a
		// single source, and soft-wrap on/off).
		if m.logsFocused {
			switch keypress {
			case "c":
				m.podLogs.CycleIsolation()
				return m, nil
			case "w":
				m.podLogs.ToggleWrap()
				return m, nil
			}
			cmd := m.podLogs.Update(msg)
			return m, cmd
		}

		// Ctrl+R always jumps straight back into an already-open pane — unlike
		// Enter, it never fetches, no matter where the list cursor now sits.
		if keypress == "ctrl+r" && m.showDetail {
			m.detailFocused = true
			m.applyContentSizes()
			m.updateFocusStates()
			return m, nil
		}

		// Tab navigation (tabs focused) — switching tabs while the detail pane
		// is open (but unfocused) is allowed; the pane is cross-cutting and
		// stays put beneath whichever tab you land on. Every tab needs loaded
		// data, so navigation is gated on contexts being selected and loaded.
		switch keypress {
		case "right", "]":
			next := m.activeTab + 1
			if next >= len(m.tabs) {
				return m, nil
			}
			snapshot := m.appState.Snapshot()
			if !m.appStateLoaded || !allContextsLoaded(snapshot.SelectedContexts, snapshot.LoadedKinds[m.tabs[next]], snapshot.Errors) {
				return m, nil
			}
			m.activeTab = next
			m.updateFocusStates()
			// The newly active tab's Age text may be stale — the 5s refresh
			// tick only re-renders the active kind (see
			// reRenderAgeFromWatchCaches), so catch this one up now rather
			// than waiting up to 5s for the next tick.
			m.tables[m.activeKind()].SetRows(m.filteredRows(m.activeKind()))
			return m, nil
		case "left", "[":
			prev := m.activeTab - 1
			if prev < 0 {
				return m, nil
			}
			snapshot := m.appState.Snapshot()
			if !m.appStateLoaded || !allContextsLoaded(snapshot.SelectedContexts, snapshot.LoadedKinds[m.tabs[prev]], snapshot.Errors) {
				return m, nil
			}
			m.activeTab = prev
			m.updateFocusStates()
			m.tables[m.activeKind()].SetRows(m.filteredRows(m.activeKind()))
			return m, nil
		}

		// d on a selected row (re)loads the detail pane for that row and
		// gives it keyboard focus for scrolling — k9s's "describe" key.
		// Detail and Logs share the same bottom slot and are mutually
		// exclusive. Unlike the sidebar panes (Contexts/Clusters/
		// Namespaces), where Enter confirms a checked selection, Enter has
		// no meaning here.
		if m.appStateLoaded && keypress == "d" {
			m.closeLogs()
			if cmd := m.openResourceDetail(m.activeKind()); cmd != nil {
				return m, cmd
			}
			return m, nil
		}

		// Space toggles the row under the cursor for inclusion in the next
		// merged log stream; Ctrl+X clears all checkmarks. Pods-tab only.
		if m.appStateLoaded && m.activeKind() == msgs.KindPods {
			switch keypress {
			case "space":
				pods := m.tables[msgs.KindPods]
				pods.ToggleChecked(models.PodRowKey(pods.SelectedRow()))
				return m, nil
			case "ctrl+x":
				m.tables[msgs.KindPods].ClearChecked()
				return m, nil
			}
		}

		// l reconciles the merged log pane to whatever's currently checked in
		// the Pods tab (or the row under the cursor, if nothing's checked).
		if m.appStateLoaded && keypress == "l" && m.activeKind() == msgs.KindPods {
			if cmd := m.openPodLogs(); cmd != nil {
				return m, cmd
			}
			return m, nil
		}

		// r force-restarts the watch(es) for only the active tab's resource
		// kind, across every selected context — not all three kinds, to
		// avoid tripling API load on tabs the user isn't even looking at.
		// Table cursor is untouched: SetRows reuses the same table.Model, it
		// doesn't reset it.
		if m.appStateLoaded && keypress == "r" {
			if cmd := m.restartActiveTabWatch(); cmd != nil {
				return m, cmd
			}
			return m, nil
		}

		// Shift+N/M/A cycle sorting by Name/Namespace/Age: ascending ->
		// descending -> off, switching to a different column resets to
		// ascending. Applies across every tab (filteredRows), not just the
		// active one, so it stays put across tab switches.
		if m.appStateLoaded {
			switch keypress {
			case "N":
				m.cycleSort(sortByName)
				return m, nil
			case "M":
				m.cycleSort(sortByNamespace)
				return m, nil
			case "A":
				m.cycleSort(sortByAge)
				return m, nil
			}
		}

		// Ctrl+W toggles wide mode on the active tab's table (sticky per tab,
		// reset on resize); Shift+Left/Right scroll one column at a time while
		// wide mode is on.
		if m.appStateLoaded {
			switch keypress {
			case "ctrl+w":
				t := m.activeTable()
				wasWide := t.WideMode()
				t.ToggleWideMode()
				if !wasWide && t.WideMode() && m.activeKind() == msgs.KindServices {
					return m, m.fetchServiceEndpointsIfNeeded()
				}
				return m, nil
			case "shift+left":
				if t := m.activeTable(); t.WideMode() {
					t.ScrollLeft()
				}
				return m, nil
			case "shift+right":
				if t := m.activeTable(); t.WideMode() {
					t.ScrollRight()
				}
				return m, nil
			}
		}

		// Content keys forwarded to the active tab
		if m.appStateLoaded {
			return m, m.activeTable().Update(msg)
		}

		return m, nil

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height

		// views.Solve is the single owner of the layout budget: every pane
		// gets exactly its solved content rectangle.
		r := views.Solve(m.width, m.height)
		m.tableW, m.tableH = r.RightContentW, r.RightContentH
		tier := views.WidthTier(m.width)
		for _, kind := range m.tabs {
			m.tables[kind].SetTier(tier)
		}
		m.applyContentSizes()

		contextsH, namespacesH, clustersH := splitLeftSections(r.LeftContentH)
		m.clusterList.SetSize(r.LeftContentW, clustersH)
		m.namespacesPane.SetSize(r.LeftContentW, namespacesH)
		return m, m.contextList.Update(tea.WindowSizeMsg{Width: r.LeftContentW, Height: contextsH})

	case msgs.ResourceDetailMsg:
		if msg.Err != nil {
			m.deploymentDetail.SetError(msg.Err.Error())
			return m, nil
		}
		m.deploymentDetail.SetDetail(msg.Detail)
		return m, nil

	case msgs.LogStreamOpenedMsg:
		// Stale — this source has since been restarted or closed. Close the
		// stream rather than adopting it; other open sources are unaffected.
		st, ok := m.logStreams[msg.SourceKey]
		if !ok || msg.Generation != st.generation {
			msg.Stream.Close()
			return m, nil
		}
		st.stream = msg.Stream
		st.scanner = cmds.NewLogScanner(msg.Stream)
		return m, cmds.WaitForLogLineCmd(msg.SourceKey, msg.Generation, st.scanner)

	case msgs.LogLineMsg:
		st, ok := m.logStreams[msg.SourceKey]
		if !ok || msg.Generation != st.generation {
			return m, nil
		}
		m.podLogs.AppendLine(msg.SourceKey, msg.Line)
		return m, cmds.WaitForLogLineCmd(msg.SourceKey, msg.Generation, st.scanner)

	case msgs.LogStreamClosedMsg:
		st, ok := m.logStreams[msg.SourceKey]
		if !ok || msg.Generation != st.generation {
			return m, nil
		}
		if st.stream != nil {
			st.stream.Close()
		}
		delete(m.logStreams, msg.SourceKey)
		m.podLogs.SetStreamEnded(msg.SourceKey, msg.Err)
		return m, nil

	case msgs.ContextsStateMsg:
		return m, m.applyContextsState(msg)

	case msgs.NodesAccessMsg:
		return m, m.applyNodesAccess(msg)

	case msgs.NamespacesMsg:
		if msg.Err != nil {
			// Listing every namespace is a cluster-scoped call and commonly
			// needs broader RBAC than watching resources within just this
			// context's own namespace — a RoleBinding scoped to one
			// namespace often grants the latter but not the former. Fall
			// back to just that namespace (already scoped via
			// AppState.DefaultNamespace/SelectedContexts, so resource
			// browsing itself is unaffected) instead of a loud error the
			// user can't act on. A context with no namespace pinned at all
			// (all-namespaces mode, DefaultNamespace=="") has no single
			// namespace to fall back to — that one still surfaces the error,
			// since there's genuinely nothing this pane can offer.
			if defaultNS := m.appState.DefaultNamespace(msg.Context); defaultNS != "" {
				m.namespacesPane.SetContextNamespaces(msg.Context, []string{defaultNS})
				snapshot := m.appState.Snapshot()
				m.namespacesPane.SyncConfirmed(msg.Context, snapshot.SelectedContexts[msg.Context], snapshot.AllNamespaces[msg.Context])
				return m, nil
			}
			m.namespacesPane.SetContextError(msg.Context, msg.Err.Error())
			return m, nil
		}
		m.namespacesPane.SetContextNamespaces(msg.Context, msg.Namespaces)
		snapshot := m.appState.Snapshot()
		m.namespacesPane.SyncConfirmed(msg.Context, snapshot.SelectedContexts[msg.Context], snapshot.AllNamespaces[msg.Context])
		return m, nil

	case msgs.NamespacesStateMsg:
		return m, m.applyNamespacesState(msg)

	case msgs.ServiceEndpointsMsg:
		if msg.Err != nil {
			// Allow a later Ctrl+W toggle to retry instead of getting stuck
			// showing the "…" placeholder forever.
			m.watchSup.ClearEndpointsRequested(msg.Context)
			return m, nil
		}
		m.watchSup.SetEndpoints(msg.Context, msg.Endpoints)
		m.tables[msgs.KindServices].SetRows(m.filteredRows(msgs.KindServices))
		return m, nil

	case msgs.PodMetricsMsg:
		if msg.Err != nil {
			// metrics-server commonly isn't installed; stop retrying this
			// context rather than re-erroring on every tick forever.
			m.watchSup.MarkPodMetricsUnavailable(msg.Context)
			return m, nil
		}
		m.watchSup.SetPodMetrics(msg.Context, msg.Usage)
		m.tables[msgs.KindPods].SetRows(m.filteredRows(msgs.KindPods))
		return m, nil

	case msgs.NodeMetricsMsg:
		if msg.Err != nil {
			m.watchSup.MarkNodeMetricsUnavailable(msg.Context)
			return m, nil
		}
		m.watchSup.SetNodeMetrics(msg.Context, msg.Usage)
		m.tables[msgs.KindNodes].SetRows(m.filteredRows(msgs.KindNodes))
		return m, nil

	case msgs.RefreshTickMsg:
		// Always reschedule, even when auto-refresh is off or paused, so it
		// resumes on its own the moment the pane closes / it's toggled back on.
		// Table data itself is kept current by the watch streams; this tick
		// just re-renders Age text from the local watch caches — purely
		// local, zero API calls.
		next := m.refreshTickCmd()
		if !m.autoRefresh || m.showDetail || m.showLogs || !m.appStateLoaded {
			return m, next
		}
		m.reRenderAgeFromWatchCaches()
		if metricsCmd := m.fetchMetricsIfNeeded(); metricsCmd != nil {
			return m, tea.Batch(next, metricsCmd)
		}
		return m, next
	}

	// Forward non-key messages to the focused component(s)
	if m.focus == focusTabs && m.appStateLoaded {
		forwardCmds := []tea.Cmd{m.activeTable().Update(msg)}
		if m.showDetail {
			forwardCmds = append(forwardCmds, m.deploymentDetail.Update(msg))
		}
		if m.showLogs {
			forwardCmds = append(forwardCmds, m.podLogs.Update(msg))
		}
		return m, tea.Batch(forwardCmds...)
	}

	if m.focus == focusLeftPane {
		switch m.activeLeftSection {
		case sectionClusters:
			return m, m.clusterList.Update(msg)
		case sectionNamespaces:
			return m, m.namespacesPane.Update(msg)
		default:
			return m, m.contextList.Update(msg)
		}
	}

	return m, nil
}

// applyWatchUpdate applies a Supervisor update to the UI: fresh rows for one
// kind's table, or a permanently failed watch surfaced as a context error.
func (m *MainPage) applyWatchUpdate(upd *watch.Update) {
	if upd == nil {
		return
	}
	if upd.GaveUp {
		if upd.Forbidden {
			// An RBAC denial on one resource kind isn't a broken connection
			// worth a loud banner — many clusters simply restrict some kinds
			// (audit-only ServiceAccounts commonly can't see Secrets, Jobs,
			// etc.) — so this fails quietly the same way a denied Nodes
			// watch already does (see applyNodesAccess): clear just this
			// kind's loading indicator and move on, leaving that tab to show
			// as empty rather than context-wide broken.
			m.appState.SetLoading(upd.Kind, upd.Context, false)
			m.syncContextStates()
			return
		}
		errMsg := upd.Err.Error()
		m.appState.SetError(upd.Context, errMsg)
		m.errorMessage = errMsg
		m.syncContextStates()
		return
	}
	if upd.RowsChanged {
		m.appState.MarkLoaded(upd.Kind, upd.Context)
		m.tables[upd.Kind].SetRows(m.filteredRows(upd.Kind))
		m.syncContextStates()
		m.updateFocusStates()
	}
}

// filteredRows returns watchSup.Rows(kind) narrowed to each row's context's
// checked namespaces, or unfiltered for a context in all-namespaces mode.
// The Namespaces pane's checked set is a display-only filter on top of
// whatever the watch itself already returned (which may already be scoped
// to a single namespace — see watch.Supervisor's stateKey doc comment) — so
// this is where "which namespaces are checked" actually takes effect.
// KindNodes is cluster-scoped and carries no namespace column, so it's
// exempt.
func (m *MainPage) filteredRows(kind msgs.ResourceKind) []msgs.RowData {
	return m.filteredRowsWithSnapshot(kind, m.appState.Snapshot())
}

// filteredRowsWithSnapshot is filteredRows with the snapshot passed in
// rather than freshly taken — for callers looping over every kind
// (applyContextsState, applyNamespacesState, cycleSort), so they take one
// snapshot and reuse it instead of one Snapshot() call per kind.
func (m *MainPage) filteredRowsWithSnapshot(kind msgs.ResourceKind, snapshot state.Snapshot) []msgs.RowData {
	rows := m.watchSup.Rows(kind)
	if kind != msgs.KindNodes {
		rows = filterRowsByNamespace(rows, snapshot.SelectedContexts, snapshot.AllNamespaces)
	}
	return sortRows(rows, m.sortField, m.sortDir)
}

// filterRowsByNamespace keeps only rows whose (context, namespace) is
// checked in selected, or whose context is in allNS — a pure function so
// the filtering logic is testable independent of the Supervisor/AppState.
func filterRowsByNamespace(rows []msgs.RowData, selected map[string][]string, allNS map[string]bool) []msgs.RowData {
	checkedSets := make(map[string]map[string]bool, len(selected))
	for ctxName, namespaces := range selected {
		set := make(map[string]bool, len(namespaces))
		for _, ns := range namespaces {
			set[ns] = true
		}
		checkedSets[ctxName] = set
	}

	var filtered []msgs.RowData
	for _, row := range rows {
		ctxName, _ := row[msgs.KeyContext].(string)
		if allNS[ctxName] {
			filtered = append(filtered, row)
			continue
		}
		namespace, _ := row[msgs.KeyNamespace].(string)
		if checkedSets[ctxName][namespace] {
			filtered = append(filtered, row)
		}
	}
	return filtered
}

// sortRows orders rows by field/dir, stably — sortNone is a true no-op,
// preserving whatever order rows already arrived in (each cache's
// namespace/name order — see resourceCache.rows). Rows missing a field
// (e.g. KindNodes has no KeyNamespace) sort as the zero value for that
// field, which is harmless: it just groups them together rather than
// erroring.
func sortRows(rows []msgs.RowData, field sortField, dir sortDir) []msgs.RowData {
	if field == sortNone {
		return rows
	}
	sort.SliceStable(rows, func(i, j int) bool {
		var less bool
		switch field {
		case sortByName:
			less = strings.ToLower(rowString(rows[i], msgs.KeyName)) < strings.ToLower(rowString(rows[j], msgs.KeyName))
		case sortByNamespace:
			less = strings.ToLower(rowString(rows[i], msgs.KeyNamespace)) < strings.ToLower(rowString(rows[j], msgs.KeyNamespace))
		case sortByAge:
			less = rowCreatedAt(rows[i]).Before(rowCreatedAt(rows[j]))
		}
		if dir == sortDesc {
			return !less
		}
		return less
	})
	return rows
}

func rowString(row msgs.RowData, key string) string {
	s, _ := row[key].(string)
	return s
}

func rowCreatedAt(row msgs.RowData) time.Time {
	t, _ := row[msgs.KeyCreatedAt].(time.Time)
	return t
}

// cycleSort advances the active sort for field: switching to a new field
// starts it at ascending; pressing the same field's key again cycles
// ascending -> descending -> off. Every visible tab is re-rendered
// immediately so the change is visible regardless of which tab is active.
func (m *MainPage) cycleSort(f sortField) {
	if m.sortField != f {
		m.sortField = f
		m.sortDir = sortAsc
	} else if m.sortDir == sortAsc {
		m.sortDir = sortDesc
	} else {
		m.sortField = sortNone
		m.sortDir = sortAsc
	}
	if len(m.tabs) == 0 {
		return
	}
	snapshot := m.appState.Snapshot()
	for _, kind := range m.tabs {
		m.tables[kind].SetRows(m.filteredRowsWithSnapshot(kind, snapshot))
	}
}

// applyContextsState reconciles a selection change from the Context List.
// msg.Added and msg.Deselected are already diffed against the previous
// confirm (see ContextsInfo.getAllContextStates) — every context in Added is
// genuinely new here, so there's no second diff to do: one loop starts
// everything a newly-added context needs (AppState bookkeeping, watches
// against its default namespace, and a namespace-list fetch for the
// Namespaces pane) directly.
func (m *MainPage) applyContextsState(msg msgs.ContextsStateMsg) tea.Cmd {
	m.errorMessage = ""

	for _, contextName := range msg.Deselected {
		m.appState.RemoveContext(contextName)
		m.watchSup.StopContext(contextName)
		m.namespacesPane.RemoveContext(contextName)
	}

	var cmdSequence []tea.Cmd
	for _, added := range msg.Added {
		m.appState.AddContext(added.ContextName, added.DefaultNamespace)
		for _, kind := range m.tabs {
			m.appState.SetLoading(kind, added.ContextName, true)
		}
		cmdSequence = append(cmdSequence, m.watchSup.StartContext(added.ContextName, added.DefaultNamespace)...)
		cmdSequence = append(cmdSequence, cmds.LoadNamespacesCmd(m.Client, added.ContextName))
		// Nodes is cluster-scoped and commonly restricted — check access
		// before opening its watch (see applyNodesAccess) instead of
		// starting it unconditionally like every other kind above.
		cmdSequence = append(cmdSequence, cmds.CheckNodesAccessCmd(m.Client, added.ContextName))
	}

	ctxSnapshot := m.appState.Snapshot()
	for _, kind := range m.tabs {
		m.tables[kind].SetRows(m.filteredRowsWithSnapshot(kind, ctxSnapshot))
	}
	m.syncContextStates()

	if len(ctxSnapshot.SelectedContexts) == 0 {
		m.appStateLoaded = false
		for _, kind := range m.tabs {
			m.tables[kind].SetRows([]msgs.RowData{})
		}
		m.contextList.SetContextStates(nil, nil, nil)
		m.updateFocusStates()
		return nil
	}

	m.activeTab = 0 // land on the Deployments tab
	m.syncContextStates()
	m.appStateLoaded = true
	m.updateFocusStates()

	if len(cmdSequence) == 0 {
		return nil
	}
	return tea.Batch(cmdSequence...)
}

// applyNodesAccess resolves one context's Nodes pre-flight check
// (cmds.CheckNodesAccessCmd): if access is confirmed denied, the Nodes watch
// is never opened at all — the loading flag is just cleared, quietly,
// rather than surfacing an RBAC error for a resource that's expected to be
// inaccessible in many clusters. If the check itself failed (as opposed to
// a confirmed denial) or access is allowed, this falls back to starting the
// watch normally — the existing reactive RBAC-detection in watch.Supervisor
// still catches a genuine denial the check missed, just after the fact.
func (m *MainPage) applyNodesAccess(msg msgs.NodesAccessMsg) tea.Cmd {
	if msg.Err == nil && !msg.Allowed {
		m.appState.SetLoading(msgs.KindNodes, msg.Context, false)
		m.syncContextStates()
		return nil
	}
	return m.watchSup.StartKind(msgs.KindNodes, msg.Context)
}

// applyNamespacesState reconciles a checked-namespace change from the
// Namespaces pane for one context. This is purely a display-side filter
// change now — each context's watch is already scoped to its kubeconfig
// default namespace (or cluster-wide, with none pinned) regardless of which
// namespaces are checked (see watch.Supervisor's stateKey doc comment), so
// there's no watch to start or stop here, just AppState bookkeeping
// followed by re-filtering each tab's already-loaded rows. The pane is
// resynced afterward (SyncConfirmed) so it always reflects exactly what
// AppState now reports as selected.
func (m *MainPage) applyNamespacesState(msg msgs.NamespacesStateMsg) tea.Cmd {
	for _, ns := range msg.Removed {
		m.appState.RemoveNamespace(msg.Context, ns)
	}
	for _, ns := range msg.Added {
		m.appState.AddNamespace(msg.Context, ns)
	}
	if msg.AllNamespaces != nil {
		m.appState.SetAllNamespaces(msg.Context, *msg.AllNamespaces)
	}

	// Confirming with zero checked namespaces and not in all-namespaces mode
	// would silently show nothing for this context — rarely what "deselect
	// everything" was meant to produce. Fall back to wherever the context
	// started instead: its original kubeconfig namespace, or all-namespaces
	// if it had none (see AppState.DefaultNamespace).
	snapshot := m.appState.Snapshot()
	if !snapshot.AllNamespaces[msg.Context] && len(snapshot.SelectedContexts[msg.Context]) == 0 {
		if defaultNS := m.appState.DefaultNamespace(msg.Context); defaultNS == "" {
			m.appState.SetAllNamespaces(msg.Context, true)
		} else {
			m.appState.AddNamespace(msg.Context, defaultNS)
		}
		snapshot = m.appState.Snapshot()
	}

	for _, kind := range m.tabs {
		m.tables[kind].SetRows(m.filteredRowsWithSnapshot(kind, snapshot))
	}
	m.namespacesPane.SyncConfirmed(msg.Context, snapshot.SelectedContexts[msg.Context], snapshot.AllNamespaces[msg.Context])
	m.syncContextStates()

	return nil
}

// syncContextStates pushes the current loading/error/loaded status into the
// Context List's per-context indicators.
func (m *MainPage) syncContextStates() {
	s := m.appState.Snapshot()
	m.contextList.SetContextStates(s.LoadingStates, s.Errors, s.LoadedContexts)
	m.contextList.SetContextColors(s.ContextColors)
	for _, kind := range m.tabs {
		m.tables[kind].SetContextColors(s.ContextColors)
	}
}

func (m *MainPage) logSlowUpdate(start time.Time, msg tea.Msg) {
	elapsed := time.Since(start)
	if elapsed > 16*time.Millisecond {
		log.Printf("Slow update: %v msg=%T", elapsed, msg)
	}
}

// logSlowRender is logSlowUpdate's View() counterpart — a render is the
// other half of what can make a frame miss its budget, and previously had
// no equivalent instrumentation at all.
func (m *MainPage) logSlowRender(start time.Time) {
	elapsed := time.Since(start)
	if elapsed > 16*time.Millisecond {
		log.Printf("Slow render: %v", elapsed)
	}
}

func (m *MainPage) toggleFocus() {
	if m.focus == focusLeftPane {
		m.focus = focusTabs
	} else {
		m.focus = focusLeftPane
	}
	m.updateFocusStates()
}

func (m *MainPage) updateFocusStates() {
	m.contextList.SetFocused(m.focus == focusLeftPane && m.activeLeftSection == sectionContexts)
	m.clusterList.SetFocused(m.focus == focusLeftPane && m.activeLeftSection == sectionClusters)
	m.namespacesPane.SetFocused(m.focus == focusLeftPane && m.activeLeftSection == sectionNamespaces)
	listActive := m.focus == focusTabs && !m.detailFocused && !m.logsFocused && m.appStateLoaded
	for _, kind := range m.tabs {
		m.tables[kind].SetFocused(listActive && kind == m.activeKind())
	}
	m.deploymentDetail.SetFocused(m.focus == focusTabs && m.detailFocused)
	m.podLogs.SetFocused(m.focus == focusTabs && m.logsFocused)
}

// detailPaneHeightPercent is the share of the tab content area given to the
// detail pane, out of the remainder after list rows.
const detailPaneHeightPercent = 45

// detailMinRows is the minimum number of usable rows the Detail/Log pane
// needs to be worth opening as a bottom split (§8.4) — below this, two
// half-broken panes are worse than one full-screen one.
const detailMinRows = 8

// listMinRows is the minimum usable rows the resource list needs to stay
// worth showing at all alongside a split Detail/Log pane.
const listMinRows = 5

// wantsDetailOverlay reports whether the Detail/Log pane should replace the
// list entirely (a full-screen overlay) rather than split the tab content
// area, given the current tab area's height: true whenever a split would
// leave the Detail pane with fewer than detailMinRows usable rows once the
// list keeps its own minimum.
func (m *MainPage) wantsDetailOverlay() bool {
	available := m.tableH - 2 // divider + pane header
	return available-listMinRows < detailMinRows
}

// applyContentSizes resizes the resource tables and the bottom pane (Detail
// or Logs — mutually exclusive) to split the tab content area in two
// whenever either is open — or, on a short terminal where a split would
// leave too little room for either half, gives the Detail/Log pane the
// entire tab content area instead (§8.4; see wantsDetailOverlay).
func (m *MainPage) applyContentSizes() {
	listH := m.tableH
	detailH := 0
	if m.showDetail || m.showLogs {
		if m.wantsDetailOverlay() {
			detailH = m.tableH - 1 // 1 line reserved: the pane header
			if detailH < 3 {
				detailH = 3
			}
		} else {
			detailH = m.tableH * detailPaneHeightPercent / 100
			if detailH < detailMinRows {
				detailH = detailMinRows
			}
			listH = m.tableH - detailH - 2 // 2 lines reserved: the divider and the pane header
			if listH < listMinRows {
				listH = listMinRows
			}
		}
	}
	for _, kind := range m.tabs {
		m.tables[kind].SetSize(m.tableW, listH)
	}
	m.deploymentDetail.SetSize(m.tableW, detailH)
	m.podLogs.SetSize(m.tableW, detailH)
}

// openResourceDetail loads detail for the currently selected row on the given
// tab into the shared bottom detail pane. Returns nil if there's no valid
// selection.
func (m *MainPage) openResourceDetail(kind msgs.ResourceKind) tea.Cmd {
	row := m.tables[kind].SelectedRow()
	if row == nil {
		return nil
	}
	name, _ := row[msgs.KeyName].(string)
	namespace, _ := row[msgs.KeyNamespace].(string)
	ctxName, _ := row[msgs.KeyContext].(string)

	// Re-entering the row already shown in the pane just refocuses it instead
	// of re-fetching — e.g. after Esc dropped back to the list to scroll/pick
	// a row, Enter on that same row jumps straight back in.
	if m.showDetail && m.deploymentDetail.Matches(kind.Kind(), name, ctxName) {
		m.detailFocused = true
		m.applyContentSizes()
		m.updateFocusStates()
		return nil
	}

	m.deploymentDetail.StartLoading(kind.Kind(), name, ctxName)
	m.showDetail = true
	m.detailFocused = true
	m.applyContentSizes()
	m.updateFocusStates()

	switch kind {
	case msgs.KindDeployments:
		return cmds.LoadDeploymentDetailCmd(m.Client, ctxName, namespace, name)
	case msgs.KindPods:
		return cmds.LoadPodDetailCmd(m.Client, ctxName, namespace, name)
	case msgs.KindServices:
		return cmds.LoadServiceDetailCmd(m.Client, ctxName, namespace, name)
	case msgs.KindConfigMaps:
		return cmds.LoadConfigMapDetailCmd(m.Client, ctxName, namespace, name)
	case msgs.KindSecrets:
		return cmds.LoadSecretDetailCmd(m.Client, ctxName, namespace, name)
	case msgs.KindJobs:
		return cmds.LoadJobDetailCmd(m.Client, ctxName, namespace, name)
	case msgs.KindCronJobs:
		return cmds.LoadCronJobDetailCmd(m.Client, ctxName, namespace, name)
	case msgs.KindStatefulSets:
		return cmds.LoadStatefulSetDetailCmd(m.Client, ctxName, namespace, name)
	case msgs.KindDaemonSets:
		return cmds.LoadDaemonSetDetailCmd(m.Client, ctxName, namespace, name)
	case msgs.KindIngresses:
		return cmds.LoadIngressDetailCmd(m.Client, ctxName, namespace, name)
	case msgs.KindPodDisruptionBudgets:
		return cmds.LoadPodDisruptionBudgetDetailCmd(m.Client, ctxName, namespace, name)
	case msgs.KindHorizontalPodAutoscalers:
		return cmds.LoadHorizontalPodAutoscalerDetailCmd(m.Client, ctxName, namespace, name)
	case msgs.KindNodes:
		return cmds.LoadNodeDetailCmd(m.Client, ctxName, namespace, name)
	}
	return nil
}

// restartActiveTabWatch force-restarts the watch(es) for only the active
// tab's resource kind, across every selected context — the "r" key.
func (m *MainPage) restartActiveTabWatch() tea.Cmd {
	snapshot := m.appState.Snapshot()
	if len(snapshot.SelectedContexts) == 0 {
		return nil
	}

	kind := m.activeKind()
	contexts := make([]string, 0, len(snapshot.SelectedContexts))
	for context := range snapshot.SelectedContexts {
		contexts = append(contexts, context)
		m.appState.SetLoading(kind, context, true)
	}

	cmdSequence := m.watchSup.Restart(kind, contexts)
	if len(cmdSequence) == 0 {
		return nil
	}

	m.syncContextStates()
	return tea.Batch(cmdSequence...)
}

// reRenderAgeFromWatchCaches recomputes the active tab's rows from the local
// watch caches (re-running the converter/formatDuration against time.Now(),
// refreshing the Age column text) and reapplies them — purely local, zero
// API calls. Driven by RefreshTickMsg. Scoped to the active kind only: every
// kind's actual data (adds/deletes/modifies) already stays live via
// applyWatchUpdate's per-event SetRows regardless of which tab is active —
// this tick's only job is the Age column text, which is only visible on the
// active tab; a background tab's Age catches up on tab switch (see the
// "right"/"]" and "left"/"[" tab-navigation handlers).
func (m *MainPage) reRenderAgeFromWatchCaches() {
	if !m.watchSup.Watching() {
		return
	}
	kind := m.activeKind()
	m.tables[kind].SetRows(m.filteredRows(kind))
	m.syncContextStates()
}

// fetchServiceEndpointsIfNeeded dispatches LoadServiceEndpointsCmd for every
// selected context that hasn't had its Endpoint IPs fetched yet (see
// watch.Supervisor.NeedsEndpoints) — one cluster-wide fetch per context,
// matching the Services watch's own scope (see watch.Supervisor's stateKey
// doc comment), regardless of which namespaces are currently checked in the
// Namespaces pane. Called only when svc wide mode turns on — the Ctrl+W
// toggle itself already revealed the other new columns synchronously; this
// just fills in the one column that needs a network round trip, without
// blocking or repeating that round trip.
func (m *MainPage) fetchServiceEndpointsIfNeeded() tea.Cmd {
	snapshot := m.appState.Snapshot()
	if len(snapshot.SelectedContexts) == 0 {
		return nil
	}

	var cmdSequence []tea.Cmd
	for context := range snapshot.SelectedContexts {
		if !m.watchSup.NeedsEndpoints(context) {
			continue
		}
		m.watchSup.MarkEndpointsRequested(context)
		cmdSequence = append(cmdSequence, cmds.LoadServiceEndpointsCmd(m.Client, context, ""))
	}

	if len(cmdSequence) == 0 {
		return nil
	}
	return tea.Batch(cmdSequence...)
}

// fetchMetricsIfNeeded dispatches a CPU/Memory usage fetch (LoadPodMetricsCmd
// or LoadNodeMetricsCmd) for every selected context, but only while the
// active tab is Pods or Nodes — usage on tabs the user isn't looking at
// isn't worth the API calls — and throttled to metricsFetchInterval via
// lastMetricsFetch. A context whose fetch has ever failed (see
// Supervisor.PodMetricsUnavailable/NodeMetricsUnavailable — most commonly:
// no metrics-server installed) is skipped rather than retried forever.
func (m *MainPage) fetchMetricsIfNeeded() tea.Cmd {
	kind := m.activeKind()
	if kind != msgs.KindPods && kind != msgs.KindNodes {
		return nil
	}
	if time.Since(m.lastMetricsFetch) < metricsFetchInterval {
		return nil
	}
	m.lastMetricsFetch = time.Now()

	snapshot := m.appState.Snapshot()
	var cmdSequence []tea.Cmd
	for context := range snapshot.SelectedContexts {
		switch kind {
		case msgs.KindPods:
			if !m.watchSup.PodMetricsUnavailable(context) {
				cmdSequence = append(cmdSequence, cmds.LoadPodMetricsCmd(m.Client, context, ""))
			}
		case msgs.KindNodes:
			if !m.watchSup.NodeMetricsUnavailable(context) {
				cmdSequence = append(cmdSequence, cmds.LoadNodeMetricsCmd(m.Client, context))
			}
		}
	}

	if len(cmdSequence) == 0 {
		return nil
	}
	return tea.Batch(cmdSequence...)
}

// podLogTarget identifies one pod/container source to be tailed.
type podLogTarget struct {
	key                            string // context/namespace/pod/container
	context, namespace, pod, cntnr string
}

// podLogTargets expands the given raw Pods-table rows into one target per
// container (all containers of each pod are tailed — decision #4).
func podLogTargets(rows []msgs.RowData) []podLogTarget {
	var targets []podLogTarget
	for _, row := range rows {
		containers, _ := row[msgs.PodKeyContainers].(string)
		if containers == "" {
			continue
		}
		name, _ := row[msgs.PodKeyName].(string)
		namespace, _ := row[msgs.PodKeyNamespace].(string)
		ctxName, _ := row[msgs.PodKeyContext].(string)
		for _, container := range strings.Split(containers, ",") {
			targets = append(targets, podLogTarget{
				key:       ctxName + "/" + namespace + "/" + name + "/" + container,
				context:   ctxName,
				namespace: namespace,
				pod:       name,
				cntnr:     container,
			})
		}
	}
	return targets
}

// openPodLogs reconciles the merged log pane to whatever's currently checked
// in the Pods tab — or, if nothing's checked, the single row under the
// cursor (preserving the original single-pod behavior). Sources newly
// present are opened, sources no longer targeted are closed, and unchanged
// sources are left running untouched. An empty target set closes the pane.
func (m *MainPage) openPodLogs() tea.Cmd {
	pods := m.tables[msgs.KindPods]
	var rows []msgs.RowData
	if keys := pods.CheckedKeys(); len(keys) > 0 {
		for _, key := range keys {
			if row := pods.CheckedRow(key); row != nil {
				rows = append(rows, row)
			}
		}
	} else if row := pods.SelectedRow(); row != nil {
		rows = append(rows, row)
	}

	targets := podLogTargets(rows)
	if len(targets) == 0 {
		m.closeLogs()
		m.applyContentSizes()
		return nil
	}

	targetSet := make(map[string]podLogTarget, len(targets))
	for _, t := range targets {
		targetSet[t.key] = t
	}

	// Close sources no longer targeted.
	for key := range m.logStreams {
		if _, wanted := targetSet[key]; !wanted {
			m.closeLogSource(key)
		}
	}

	// Open sources newly targeted; unchanged ones are left running.
	var openCmds []tea.Cmd
	for key, t := range targetSet {
		if _, exists := m.logStreams[key]; exists {
			continue
		}
		m.podLogs.AddSource(key, t.pod, t.namespace, t.context, t.cntnr)
		m.logStreams[key] = &logStreamState{generation: 1}
		openCmds = append(openCmds, cmds.OpenPodLogStreamCmd(m.Client, t.context, t.namespace, t.pod, t.cntnr, key, 1))
	}

	m.closeDetail()
	m.showLogs = true
	m.logsFocused = true
	m.applyContentSizes()
	m.updateFocusStates()

	if len(openCmds) == 0 {
		return nil
	}
	return tea.Batch(openCmds...)
}

// closeLogSource stops one source's stream (if any) and removes it from
// both the stream registry and the render model.
func (m *MainPage) closeLogSource(key string) {
	if st, ok := m.logStreams[key]; ok {
		if st.stream != nil {
			st.stream.Close()
		}
		delete(m.logStreams, key)
	}
	m.podLogs.RemoveSource(key)
}

// closeDetail closes the Detail pane, if open. It holds no external
// resources, so there's nothing to release beyond the flags themselves.
func (m *MainPage) closeDetail() {
	m.showDetail = false
	m.detailFocused = false
}

// closeLogs closes the Log pane, if open, stopping every open source's
// underlying stream.
func (m *MainPage) closeLogs() {
	m.stopLogStream()
	m.podLogs.Clear()
	m.showLogs = false
	m.logsFocused = false
}

// stopLogStream closes every currently open log source's stream. Safe to
// call when nothing is streaming.
func (m *MainPage) stopLogStream() {
	for key, st := range m.logStreams {
		if st.stream != nil {
			st.stream.Close()
		}
		delete(m.logStreams, key)
	}
}

func (m *MainPage) View() tea.View {
	start := time.Now()
	defer m.logSlowRender(start)
	return tea.View{
		Content:   m.renderView(),
		AltScreen: true,
	}
}

func (m *MainPage) renderView() string {
	if m.width < views.MinContentWidth || m.height < views.MinHeight {
		return m.renderTooSmallOverlay()
	}

	snapshot := m.appState.Snapshot()

	// Overlays replace the frame entirely, so check them before doing any
	// frame work — building the full frame (left box with
	// clusterList.Refresh(), table view, status bar) only to immediately
	// discard it wastes a full render on every message while one is open.
	if m.showHelp {
		return m.renderHelpOverlay()
	}
	if m.errorMessage != "" {
		return m.renderErrorOverlay(m.errorMessage)
	}
	if len(snapshot.Errors) > 0 {
		return m.renderErrorSummaryOverlay(snapshot.Errors)
	}

	r := views.Solve(m.width, m.height)
	p := styles.CatppuccinMocha()

	leftFocused := m.focus == focusLeftPane
	leftBorder, rightBorder := styles.BlurColor, styles.FocusColor
	if leftFocused {
		leftBorder, rightBorder = styles.FocusColor, styles.BlurColor
	}

	// Left box: Contexts (the interactive selection list), Namespaces, and
	// Clusters (a "connected" summary) — all three always visible, stacked
	// top to bottom rather than tab-switched.
	leftTitleStyle := lipgloss.NewStyle().Foreground(p.Overlay1).Bold(true)
	if leftFocused {
		leftTitleStyle = leftTitleStyle.Foreground(styles.FocusColor)
	}
	leftBox := views.TitledBox(leftTitleStyle.Render("Contexts"), m.renderLeftBox(r, leftFocused), r.LeftBoxW, r.BoxH, leftBorder)

	// Right box: the active tab's content, with the tab strip in the border.
	var content string
	if !m.appStateLoaded || len(snapshot.SelectedContexts) == 0 {
		emptyMsg := "No contexts selected\n\nPress Tab to focus contexts\nSpace to select • Enter to load"
		empty := styles.HelpBoxStyle().Align(lipgloss.Center).Render(emptyMsg)
		content = lipgloss.Place(r.RightContentW, r.RightContentH, lipgloss.Center, lipgloss.Center, empty)
	} else {
		content = m.activeTable().View()

		// Loading indicator (inline — it's brief and doesn't break layout).
		// Only shown on the active tab's first load (no rows yet) — a
		// background refresh of already-populated data relies on the subtle
		// "⏳ N loading" status bar hint instead, so auto-refresh doesn't
		// reflow the tab.
		if m.activeTable().RowCount() == 0 && hasLoading(snapshot.LoadingStates) {
			content = m.renderLoadingIndicator(snapshot.LoadingStates) + "\n\n" + content
		}

		// The bottom pane (Detail or Logs — mutually exclusive) is
		// cross-cutting: it splits whichever top tab's content area is
		// active in two, rather than being a peer tab of its own — unless
		// the terminal is too short for a split to be worth it (§8.4), in
		// which case it replaces the list entirely.
		if m.showDetail || m.showLogs {
			var header, body string
			if m.showDetail {
				header = m.deploymentDetail.Header(r.RightContentW, snapshot.ContextColors[m.deploymentDetail.Context()])
				body = m.deploymentDetail.View()
			} else {
				header = m.podLogs.Header(r.RightContentW)
				body = m.podLogs.View()
			}
			if m.wantsDetailOverlay() {
				content = lipgloss.JoinVertical(lipgloss.Left, header, body)
			} else {
				divider := lipgloss.NewStyle().Foreground(p.Overlay0).Render(strings.Repeat("─", r.RightContentW))
				content = lipgloss.JoinVertical(lipgloss.Left, content, divider, header, body)
			}
		}
	}
	rightBox := views.TitledBox(m.renderTabTitle(!leftFocused, r.RightBoxW), content, r.RightBoxW, r.BoxH, rightBorder)

	fullView := lipgloss.JoinVertical(lipgloss.Left,
		lipgloss.JoinHorizontal(lipgloss.Top, leftBox, rightBox),
		m.renderStatusBar(snapshot),
	)

	return fullView
}

func (m *MainPage) renderStatusBar(snapshot state.Snapshot) string {
	p := styles.CatppuccinMocha()
	// Every segment carries the bar's background itself — a nested style's
	// ANSI reset would otherwise punch a hole in an outer background.
	leftStyle := lipgloss.NewStyle().Foreground(p.Rosewater).Background(p.Mantle).Padding(0, 1)
	midStyle := lipgloss.NewStyle().Foreground(p.Sapphire).Background(p.Mantle).Bold(true)
	rightStyle := lipgloss.NewStyle().Foreground(p.Green).Background(p.Mantle).Padding(0, 1)

	selectedCtx := len(snapshot.SelectedContexts)
	errCount := len(snapshot.Errors)
	loadingCount := 0
	for _, l := range snapshot.LoadingStates {
		if l {
			loadingCount++
		}
	}
	activeTabName := m.activeKind().Title()
	activeCount := m.activeTable().RowCount()

	focusStr := "Left Pane"
	if m.focus == focusTabs {
		focusStr = "Tabs"
	}

	left := leftStyle.Render(fmt.Sprintf("Contexts: %d", selectedCtx))
	mid := midStyle.Render(fmt.Sprintf("Tab: %s | Focus: %s", activeTabName, focusStr))

	// Everything below is state — what's currently selected/loading/focused
	// — and belongs on the left (§3.5). The right zone is reserved entirely
	// for keybind hints so the two zones never both report the same kind of
	// thing.
	var statusBits []string
	if loadingCount > 0 {
		statusBits = append(statusBits, fmt.Sprintf("⏳ %d loading", loadingCount))
	} else if activeCount > 0 {
		statusBits = append(statusBits, fmt.Sprintf("%s: %d", activeTabName, activeCount))
	}
	if errCount > 0 {
		statusBits = append(statusBits, fmt.Sprintf("⚠ %d error(s)", errCount))
	}
	if m.activeKind() == msgs.KindPods {
		if checkedCount := len(m.tables[msgs.KindPods].CheckedKeys()); checkedCount > 0 {
			statusBits = append(statusBits, fmt.Sprintf("☑ %d checked", checkedCount))
		}
	}
	if offset, total, ok := m.activeTable().ScrollStatus(); ok {
		statusBits = append(statusBits, fmt.Sprintf("◂ col %d/%d ▸", offset, total))
	}
	if query, matches, typing, ok := m.activeTable().FilterStatus(); ok {
		cursor := ""
		if typing {
			cursor = "_"
		}
		statusBits = append(statusBits, fmt.Sprintf("/%s%s (%d match(es))", query, cursor, matches))
	}
	if m.focus == focusLeftPane && m.activeLeftSection == sectionNamespaces {
		if query, matches, typing, ok := m.namespacesPane.FilterStatus(); ok {
			cursor := ""
			if typing {
				cursor = "_"
			}
			statusBits = append(statusBits, fmt.Sprintf("/%s%s (%d match(es))", query, cursor, matches))
		}
	}
	if m.showDetail {
		if percent, ok := m.deploymentDetail.HScrollStatus(); ok {
			statusBits = append(statusBits, fmt.Sprintf("◂ %d%% ▸", percent))
		}
	}
	if m.showLogs {
		if percent, ok := m.podLogs.ScrollStatus(); ok {
			statusBits = append(statusBits, fmt.Sprintf("◂ %d%% ▸", percent))
		}
	}
	if len(statusBits) == 0 {
		statusBits = append(statusBits, "Ready")
	}
	status := rightStyle.Render(strings.Join(statusBits, "  |  "))

	// Hints are the right zone's only content — a context-sensitive
	// keybind list (§4), never state, so the two footer zones never
	// duplicate the same kind of information.
	hints := lipgloss.NewStyle().Foreground(p.Overlay1).Background(p.Mantle).Faint(true).Render(m.footerHints() + " ")

	gap := styles.StatusBar.Render("  ")
	leftMid := lipgloss.JoinHorizontal(lipgloss.Top, left, gap, mid, gap, status)
	rightSection := hints
	spacerWidth := m.width - lipgloss.Width(leftMid) - lipgloss.Width(rightSection)
	if spacerWidth < 1 {
		spacerWidth = 1
	}
	spacer := styles.StatusBar.Render(strings.Repeat(" ", spacerWidth))

	return leftMid + spacer + rightSection
}

// footerHints returns the right footer zone's keybind hint list (§4),
// picking the 4-6 keys relevant to whatever currently has focus rather than
// always showing the same fixed set — e.g. "d delete" makes no sense while
// the context picker has focus.
func (m *MainPage) footerHints() string {
	switch {
	case m.showLogs:
		return "↑↓ scroll  w wrap  c isolate  esc close  ctrl+c quit"
	case m.showDetail:
		return "↑↓ scroll  y yaml  ctrl+r focus  esc close  ctrl+c quit"
	case m.focus == focusLeftPane:
		return "space select  ↵ confirm  tab focus  ?  help  ctrl+c quit"
	case m.activeKind() == msgs.KindPods:
		return "/ filter  [ ] tabs  space check  l logs  d describe  ?  help"
	default:
		return "/ filter  [ ] tabs  d describe  r refresh  ?  help  ctrl+c quit"
	}
}

func (m *MainPage) renderHelpOverlay() string {
	p := styles.CatppuccinMocha()

	titleStyle := lipgloss.NewStyle().Foreground(p.Mauve).Bold(true)
	keyStyle := lipgloss.NewStyle().Foreground(p.Blue).Bold(true).Width(22)
	descStyle := lipgloss.NewStyle().Foreground(p.Text)
	sepStyle := lipgloss.NewStyle().Foreground(p.Overlay0)
	boxStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(p.Mauve).
		Background(p.Mantle).
		Padding(1, 3)

	type binding struct{ key, desc string }
	bindings := []binding{
		{"Tab / Shift+Tab", "Switch pane focus"},
		{"[ / ]", "Navigate resource tabs, or cycle Contexts/Namespaces/Clusters sections when the sidebar has focus"},
		{"Space / Enter (Namespaces pane)", "Check a namespace to watch it in addition to the default; Enter applies the change"},
		{"a (Namespaces pane)", "Toggle all namespaces under the cursor's context checked, or back to what was checked before"},
		{"/ (Namespaces pane)", "Filter namespace rows by name; Enter to keep it, Esc to clear"},
		{"Space / Enter (Clusters pane)", "Bulk select/deselect every context under one cluster; Enter applies the change"},
		{"← / →", "Navigate tabs (alias)"},
		{"↑ / ↓   j / k", "Move up / down"},
		{"PgUp / PgDn   Ctrl+U / Ctrl+D", "Move up / down a page (any resource tab)"},
		{"g / Home   G / End", "Jump to first / last row (any resource tab)"},
		{"/", "Filter the active table by name across all rows, not just the visible ones; Enter to keep it, Esc to clear"},
		{"Space", "Toggle context selection / check a Pods row for log tailing"},
		{"Enter (Contexts pane)", "Confirm selection & load"},
		{"d", "Open + focus the detail pane for the row under the cursor (refocuses instantly if already loaded)"},
		{"l (Pods tab)", "Open/reconcile the merged log pane for checked rows (or the row under the cursor)"},
		{"Ctrl+X (Pods tab)", "Clear all checked rows"},
		{"r", "Refresh the active tab's resource list across all selected contexts"},
		{"c (log pane focused)", "Isolate one source's view, or return to the full merge"},
		{"Ctrl+R", "Jump back into an open detail pane without changing its resource"},
		{"R", "Toggle auto-refresh on/off"},
		{"Shift+N / Shift+M / Shift+A", "Cycle sorting by Name / Namespace / Age: ascending -> descending -> off"},
		{"↑/↓ j/k PgUp/PgDn", "Scroll detail/log pane (while it has focus)"},
		{"y (detail pane focused)", "Jump straight to the YAML section"},
		{"Home / End", "Jump to top / bottom of detail/log pane"},
		{"Esc", "Unfocus detail/log pane, then close it / overlay / dismiss error"},
		{"?", "Toggle this help"},
		{"Ctrl+C", "Quit"},
	}

	var lines []string
	lines = append(lines, titleStyle.Render("Keybindings"))
	lines = append(lines, sepStyle.Render(strings.Repeat("─", 38)))
	for _, b := range bindings {
		lines = append(lines, lipgloss.JoinHorizontal(lipgloss.Top,
			keyStyle.Render(b.key),
			descStyle.Render(b.desc),
		))
	}

	box := boxStyle.Render(strings.Join(lines, "\n"))
	return lipgloss.Place(m.width, m.height-2, lipgloss.Center, lipgloss.Center, box)
}

// renderTooSmallOverlay replaces the whole TUI with a plain message when the
// terminal is below views.MinContentWidth x views.MinHeight — below that, the
// real layout doesn't have room to render without breaking, so we don't try.
func (m *MainPage) renderTooSmallOverlay() string {
	p := styles.CatppuccinMocha()
	msg := fmt.Sprintf(
		"Terminal window is too small\n\nCurrent size: %d x %d\nMinimum size: %d x %d\n\nPlease resize your terminal",
		m.width, m.height, views.MinContentWidth, views.MinHeight,
	)
	body := lipgloss.NewStyle().Foreground(p.Text).Align(lipgloss.Center).Render(msg)

	// Below a certain point there isn't even room for the box border/padding
	// — fall back to bare text rather than let lipgloss mangle it further.
	if m.width < 20 || m.height < 6 {
		return body
	}

	box := lipgloss.NewStyle().
		Foreground(p.Text).
		Background(p.Surface0).
		Border(lipgloss.RoundedBorder()).
		BorderForeground(p.Yellow).
		Padding(1, 3).
		Align(lipgloss.Center).
		Render(body)

	return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, box)
}

func (m *MainPage) renderErrorOverlay(msg string) string {
	p := styles.CatppuccinMocha()
	maxW := m.width - 16
	if maxW < 40 {
		maxW = 40
	}
	box := lipgloss.NewStyle().
		Foreground(p.Text).
		Background(p.Surface0).
		Border(lipgloss.RoundedBorder()).
		BorderForeground(p.Red).
		Padding(1, 3).
		Width(maxW).
		Align(lipgloss.Center)

	title := lipgloss.NewStyle().Foreground(p.Red).Bold(true).Render("⚠  Error")
	sep := lipgloss.NewStyle().Foreground(p.Overlay0).Render(strings.Repeat("─", maxW-2))
	body := lipgloss.NewStyle().Foreground(p.Text).Render(msg)
	hint := lipgloss.NewStyle().Foreground(p.Overlay1).Faint(true).Render("Esc to dismiss")

	content := strings.Join([]string{title, sep, body, "", hint}, "\n")
	return lipgloss.Place(m.width, m.height-2, lipgloss.Center, lipgloss.Center, box.Render(content))
}

func (m *MainPage) renderErrorSummaryOverlay(errors map[string]string) string {
	p := styles.CatppuccinMocha()
	maxW := m.width - 16
	if maxW < 40 {
		maxW = 40
	}
	box := lipgloss.NewStyle().
		Foreground(p.Text).
		Background(p.Surface0).
		Border(lipgloss.RoundedBorder()).
		BorderForeground(p.Red).
		Padding(1, 3).
		Width(maxW).
		Align(lipgloss.Center)

	title := lipgloss.NewStyle().Foreground(p.Red).Bold(true).Render("⚠  Errors encountered")
	sep := lipgloss.NewStyle().Foreground(p.Overlay0).Render(strings.Repeat("─", maxW-2))
	// Map iteration order is randomized per Go's spec — without sorting,
	// this list would reshuffle every single frame.
	ctxNames := make([]string, 0, len(errors))
	for ctx := range errors {
		ctxNames = append(ctxNames, ctx)
	}
	sort.Strings(ctxNames)
	var bodyLines []string
	for _, ctx := range ctxNames {
		bodyLines = append(bodyLines, fmt.Sprintf("• %s: %s", ctx, errors[ctx]))
	}
	body := lipgloss.NewStyle().Foreground(p.Text).Render(strings.Join(bodyLines, "\n"))
	hint := lipgloss.NewStyle().Foreground(p.Overlay1).Faint(true).Render("Esc to dismiss")

	content := strings.Join([]string{title, sep, body, "", hint}, "\n")
	return lipgloss.Place(m.width, m.height-2, lipgloss.Center, lipgloss.Center, box.Render(content))
}

func (m *MainPage) renderLoadingIndicator(loading map[string]bool) string {
	p := styles.CatppuccinMocha()
	loadingStyle := lipgloss.NewStyle().
		Foreground(p.Blue).
		Background(p.Surface0).
		Padding(0, 1)

	var loadingContexts []string
	for ctx, isLoading := range loading {
		if isLoading {
			loadingContexts = append(loadingContexts, ctx)
		}
	}
	// Map iteration order is randomized per Go's spec — without sorting,
	// this list would reshuffle every single frame.
	sort.Strings(loadingContexts)

	if len(loadingContexts) == 0 {
		return ""
	}

	return loadingStyle.Render(fmt.Sprintf("⏳ Loading: %s...", strings.Join(loadingContexts, ", ")))
}

// leftSections is how many always-visible sections the Context List box is
// divided into, and how many one-line headers that costs.
const leftSections = 3

// minLeftSectionHeight is the minimum content height any one section is
// left with, even on a barely-tall-enough terminal.
const minLeftSectionHeight = 1

// splitLeftSections divides the Context List box's content height across
// its three always-visible, stacked sections — Contexts (the interactive
// list, given the most room), Namespaces, and Clusters — each preceded by
// a one-line header. total is the box's full content height (views.Rects'
// LeftContentH); the three results plus leftSections header lines always
// sum back to exactly total.
func splitLeftSections(total int) (contextsH, namespacesH, clustersH int) {
	remaining := total - leftSections
	if remaining < leftSections*minLeftSectionHeight {
		remaining = leftSections * minLeftSectionHeight
	}

	namespacesH = remaining / 4
	if namespacesH < minLeftSectionHeight {
		namespacesH = minLeftSectionHeight
	}
	clustersH = remaining / 4
	if clustersH < minLeftSectionHeight {
		clustersH = minLeftSectionHeight
	}
	contextsH = remaining - namespacesH - clustersH
	if contextsH < minLeftSectionHeight {
		contextsH = minLeftSectionHeight
	}
	return contextsH, namespacesH, clustersH
}

// renderLeftBox renders the Context List box's content: the interactive
// Contexts list, then the interactive Namespaces and Clusters lists (per-
// context multi-select namespaces, and grouping Contexts' rows by
// kubeconfig cluster for bulk select/deselect), each clipped
// (views.FitBlock) to its allotted height so one section can never push the
// others out of place. Only Namespaces and Clusters get their own header
// rendered here — Contexts' header is the outer box's border title instead.
func (m *MainPage) renderLeftBox(r views.Rects, focused bool) string {
	contextsH, namespacesH, clustersH := splitLeftSections(r.LeftContentH)

	p := styles.CatppuccinMocha()
	dimHeaderStyle := lipgloss.NewStyle().Foreground(p.Overlay1).Bold(true)
	// Each section's header only picks up the focus colour when it's
	// genuinely the section with keyboard focus (see activeLeftSection) —
	// otherwise every section would look focused whenever the sidebar has
	// focus at all.
	namespacesHeaderStyle := dimHeaderStyle
	if focused && m.activeLeftSection == sectionNamespaces {
		namespacesHeaderStyle = namespacesHeaderStyle.Foreground(styles.FocusColor)
	}
	clustersHeaderStyle := dimHeaderStyle
	if focused && m.activeLeftSection == sectionClusters {
		clustersHeaderStyle = clustersHeaderStyle.Foreground(styles.FocusColor)
	}

	m.clusterList.Refresh()

	blocks := []string{
		views.FitBlock(m.contextList.View(), r.LeftContentW, contextsH),
		namespacesHeaderStyle.Render("Namespaces"),
		views.FitBlock(m.namespacesPane.View(), r.LeftContentW, namespacesH),
		clustersHeaderStyle.Render("Clusters"),
		views.FitBlock(m.clusterList.View(), r.LeftContentW, clustersH),
	}
	return lipgloss.JoinVertical(lipgloss.Left, blocks...)
}

func hasLoading(loading map[string]bool) bool {
	for _, isLoading := range loading {
		if isLoading {
			return true
		}
	}
	return false
}

// allContextsLoaded reports whether every selected context has settled for
// one specific tab's kind: it either completed a successful load for that
// kind, or gave up permanently (e.g. an RBAC denial recorded in errors —
// that context will never load, so it must not hold tab navigation hostage
// forever). Tab navigation passes the target tab's own snapshot.LoadedKinds
// entry as loaded, so each tab unlocks independently as its own List/watch
// settles rather than every tab waiting on Deployments+Pods together, while
// a context this user can't view doesn't stick every tab shut.
func allContextsLoaded(selected map[string][]string, loaded map[string]bool, errors map[string]string) bool {
	if len(selected) == 0 {
		return false
	}
	for ctx := range selected {
		if !loaded[ctx] && errors[ctx] == "" {
			return false
		}
	}
	return true
}
