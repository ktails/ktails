# KTails TODO

Roadmap and planned features for KTails, organized by priority and version.

## Legend

- 🔴 **HIGH** - Critical functionality
- 🟡 **MEDIUM** - Important improvements
- 🟢 **LOW** - Nice to have
- ✅ **DONE** - Completed
- 🚧 **IN PROGRESS** - Currently being worked on

---

## v0.1.0 - Core Functionality

Focus: Get the basics working - status bar, refresh, error handling.

### 🔴 HIGH Priority

- [x] **Status Bar / Header**
  - Show current context count
  - Show active tab and its row count
  - Show loading indicator per context
  - Make header persistent (always visible)
  - Status: ✅ Done — `renderStatusBar` in `internal/pages/mainPage.go`
  - Still open: namespace-per-pane display, last-refresh timestamp, search match count (no search mode exists yet)

- [x] **Error Handling & Display**
  - Non-blocking error banner (dismissible with `Esc`)
  - Per-context error tracking and summary overlay
  - Handle API errors gracefully (wrapped with `%w`, surfaced not swallowed)
  - Status: ✅ Done — `state.AppState.Errors`, `renderErrorOverlay`/`renderErrorSummaryOverlay`
  - Still open: hints for common issues (permissions, connectivity), debug-file error logging

- [ ] **Manual Refresh**
  - Press `r` to re-fetch **only the active tab's** resource list (across all
    loaded contexts) — not all three resource types, to avoid 3x API load on
    tabs you're not looking at
  - Reuses the existing `LoadDeploymentInfoCmd`/`LoadPodInfoCmd`/`LoadServiceInfoCmd`
  - Visual feedback during refresh (loading indicator already exists for initial load)
  - Preserve cursor position after refresh
  - Status: ❌ Not started (only loads once on context selection) — spec locked via grilling session, ready to implement

- [x] **Basic Log Viewing**
  - Press `l` on a pod to view its logs
  - Display logs in a dedicated pane, with follow/tail-f mode
  - Scroll through logs with arrow keys
  - Status: ✅ Done — see `design-log-viewer.md`

- [ ] **Tab Gating Consistency**
  - Only the Deployments tab currently blocks switching-in before contexts
    are loaded (`nextTab == "Deployments"` check in `mainPage.go`'s tab
    navigation); Pods/svc allow switching in and just show the empty-state
    message instead
  - Extend the same guard to Pods and svc, so all three tabs behave
    identically (chosen over relaxing Deployments to match Pods/svc — see
    grilling session)
  - Status: ❌ Not started — spec locked via grilling session, ready to implement (small, mostly one-line-per-tab)

- [ ] **Checked-Count Indicator**
  - While on the Pods tab with 1+ rows checked (`Space`), show a status bar
    hint: `☑ N checked · l: open merged · Ctrl+X: clear` — mirrors the
    existing `⏳ N loading` status bar pattern
  - Pods-tab only, hidden when nothing's checked
  - Status: ❌ Not started — spec locked via grilling session, ready to implement (tiny)

### 🟡 MEDIUM Priority

- [ ] **Auto-Refresh**
  - Wire up the existing (currently unused) `config.Preferences.RefreshInterval`
    to a ticking `tea.Cmd` that refreshes the active tab on that interval —
    finally puts that config field to use
  - **Pause the tick while the Detail or Log pane is open** — a background
    refresh reordering rows under a pinned/open pane is more disruptive than
    helpful; skip or discard the tick's result in that state
  - Depends on **Manual Refresh** above landing first (reuses the same
    per-tab refresh path)
  - Subtle spinner in status bar during refresh
  - Toggle auto-refresh with a key
  - Status: ❌ Not started — spec locked via grilling session, ready to implement

- [ ] **Status Colors** (Detail pane + focus/selection — done; resource-table phase coloring — spec'd, not started)
  - Color code Detail pane event types (Warning=yellow, Normal=green) — ✅ done
  - Highlight selected row with accent color — ✅ done
  - Show focused pane with distinct border color/thickness — ✅ done
  - **Pods table** — color just the Status *cell* (not the whole row) by phase:
    Running=green, Pending=yellow, Failed/Unknown=red, Succeeded=Overlay1 (dim)
  - **Deployments table** — needs a new `DesiredReplicas` field (from
    `deployment.Spec.Replicas`, currently not fetched into `DeploymentInfo` at
    all); change the replica column from a bare ready-count to a colored
    `ready/desired` fraction (green = fully ready, yellow = partial, red =
    zero ready with desired > 0)
  - **Services table** — no natural phase/status field exists today; out of
    scope for this item
  - Status: ⚠️ Partial — spec locked via grilling session for the Pods/Deployments halves, ready to implement

---

## v0.2.0 - Search & Filtering

Focus: Make it easy to find pods and filter logs in large clusters.

### 🔴 HIGH Priority

- [ ] **Search Mode**
  - Press `S` to enter search mode
  - Live filter pod list as you type
  - Highlight matching text in pod names
  - Optionally highlight matching log lines
  - Press `Esc` to exit search
  - Keep headers visible during search
  - Status: ❌ Not started

- [ ] **Log Filtering**
  - Filter logs by text pattern
  - Case-sensitive/insensitive toggle
  - Regex support (optional)
  - Show match count in status bar
  - Navigate between matches with `n`/`N`
  - Status: ❌ Not started

### 🟡 MEDIUM Priority

- [ ] **Sorting**
  - Press `o` to cycle sort order
  - Sort by: name, status, restarts, age
  - Visual indicator of current sort
  - Ascending/descending toggle
  - Status: ❌ Not started

- [ ] **Quick Filter**
  - Filter pods by substring in search mode
  - Filter by namespace
  - Filter by label selector
  - Status: ❌ Not started

- [ ] **Horizontal Scrolling** (table, Detail, and Log panes — spec locked via
  grilling session, 2026-07-19; expanded well beyond the original bullet
  below, see `plan-horizontal-scroll.md` for the execution plan)
  - **Library migration**: adopt `github.com/evertras/bubble-table` for the
    three resource tables (Pods/Deployments/svc), replacing
    `charmbracelet/bubbles/table`. Retires the ANSI-embedding workarounds
    built for Status/Replica cell coloring (`colorizeStatusColumn`/
    `colorizeReplicaColumn`) in favor of native `StyledCell`/`StyledCellFunc`,
    and replaces positional `table.Row` slices (with hidden width-0 "carrier"
    columns for Context/Namespace/Containers) with real keyed `RowData` maps.
    `WithNoPagination()` — vertical scroll-through-full-list stays exactly as
    today, migration doesn't introduce paging as a side effect.
  - **Tables — narrow/wide mode**: default ("narrow") behavior unchanged —
    flex columns shrink to fit viewport, long values truncated with `…`.
    `Ctrl+W` toggles "wide mode" **per tab, sticky** across tab switches
    (Deployments/Pods/svc each remember their own state independently).
    **Re-scoped (grilling session, same day): wide mode isn't just
    auto-fit-to-content-width of the existing columns (that was the original,
    narrower framing, already built in Track M) — it also reveals genuinely
    new columns not shown in narrow mode at all**, joining the same
    auto-fit-width + scroll mechanism (nothing from Track M is discarded, the
    wide-mode column list just grows):
    - **Pods**: Node, Node IP (`pod.Status.HostIP`), Pod IP
      (`pod.Status.PodIP`), Ready containers (e.g. `2/3`). All free from the
      existing `ListPods` call already backing `LoadPodInfoCmd` — no extra
      API round trip.
    - **Deployments**: Strategy (`deployment.Spec.Strategy.Type`),
      Available/Updated replicas (`deployment.Status.AvailableReplicas`/
      `.UpdatedReplicas`), Selector (`deployment.Spec.Selector`). All free
      from the existing deployments List call.
    - **Services**: Selector (`svc.Spec.Selector`), External IP/LoadBalancer
      ingress (`svc.Status.LoadBalancer.Ingress`) — both free from the
      existing services List call. **Endpoint IPs is the one exception** —
      it lives on a separate resource (EndpointSlices/Endpoints, not
      `Service`), so it needs one extra namespace-wide List call, joined
      client-side by service name. Fetched **lazily** — only the first time
      wide mode toggles on for a given context+namespace, then cached like
      everything else. `Ctrl+W` reveals every other wide-mode column
      immediately; the Endpoint IPs column shows `…` per row until that one
      async fetch resolves.
    All wide-mode columns (old and new) are **auto-fit to the longest
    current value in each column** (recomputed on every `SetRows`/refresh) —
    no truncation. When wide mode's total column width exceeds the viewport,
    `Shift+Left`/`Shift+Right` scroll **one column at a time**
    (`ScrollLeft`/`ScrollRight`). Pods' checkbox (`✓`) column stays **frozen**
    on the left during scroll (`WithHorizontalFreezeColumnCount(1)`) — no
    other column warrants freezing. Status-bar indicator (new bit alongside
    `⏳ N loading`/`☑ N checked`): `◂ col N/M ▸`, shown only when wide mode is
    on and there's more than one column-page. Scroll position + wide-mode
    on/off **survive** manual/auto refresh; **reset** to leftmost/narrow on
    terminal resize. `Ctrl+W` and column-scroll are a no-op outside the three
    resource tables.
  - **Detail & Log panes — horizontal scroll** (free text, no columns, both
    `bubbles/viewport`): always horizontally scrollable when content
    overflows (no on/off toggle — nothing to "widen" from). `Shift+Left`/
    `Shift+Right` scroll by **half the viewport's width** per press.
    Status-bar indicator: percentage scrolled, e.g. `◂ 40% ▸`. Scroll
    position survives refresh/new log lines; resets on resize.
  - Status: ⚠️ Partial — library migration + tables (narrow/wide mode,
    including the wide-mode new-columns re-scope) ✅ done (Tracks M/M2, see
    `plan-horizontal-scroll.md`); Log pane's horizontal scroll ✅ done
    (Track G); Detail pane's horizontal scroll ❌ not started (Track F)

---

## v0.3.0 - Log Enhancements

Focus: Advanced log viewing capabilities.

### 🟡 MEDIUM Priority

- [ ] **Soft Wrap Toggle** (spec locked via grilling session, 2026-07-19 —
  designed together with Horizontal Scrolling above, see
  `plan-horizontal-scroll.md`)
  - `w` toggles soft wrap (free key, no conflicts). **Off by default** — no
    behavior change for existing users on their first log view.
  - **Mutually exclusive** with the Log pane's horizontal scroll (see
    Horizontal Scrolling above): wrap on → no horizontal scroll (nothing to
    scroll to); wrap off → horizontal scroll available.
  - Preference is "remembered per session" — persists across different
    pods/log views for the lifetime of the running process (single in-memory
    flag on the one long-lived `LogPage` instance), **not** written to disk —
    no config-file I/O exists yet (see Configuration File, v0.4.0), so this
    doesn't wait on that landing first.
  - Wrapping must be ANSI-aware (reflow without breaking the per-source-prefix
    colors or the JSON highlighting already shipped) — the main technical
    risk in this item.
  - Status: ✅ Done (Track G, see `plan-horizontal-scroll.md`) —
    `github.com/charmbracelet/x/ansi.Wrap` reflows the already-colored line
    text; `w` wired in `mainPage.go`'s `logsFocused` block next to `c`

- [ ] **Log Export**
  - Press `e` to export current logs to file
  - Configurable export location
  - Include timestamp in filename
  - Status: ❌ Not started

- [x] **Multi-Container Support**
  - All containers of a tailed pod stream at once (merged view), not one at a time
  - `c` isolates the view to one source (cycling through sources), instead of switching which container streams
  - Current isolation state shown in the log pane header
  - Status: ✅ Done — see `design-log-viewer.md`

- [ ] **Timestamp Control**
  - Toggle timestamp display with `t` key
  - Configure timestamp format
  - Show relative or absolute times
  - Status: ❌ Not started

### 🟢 LOW Priority

- [ ] **Log Highlighting (JSON)**
  - Detect JSON **embedded after a prefix** (e.g. `2026-07-19 INFO {"user":"x"}`)
    — scan for the first `{`/`[` and try parsing from there to end of line;
    whole-line-only detection was considered and rejected (misses the common
    `timestamp level {json}` format)
  - Color tokens **in place, single line** — no pretty-print/expand, so the
    existing one-buffered-line-per-entry assumption (ring buffer cap, scroll
    position, viewport line count) doesn't need to change
  - Color scheme: Blue keys / Green strings / Yellow numbers / Mauve
    booleans+null / Overlay1 (dim) punctuation — deliberately distinct from
    the source-prefix color rotation (Sapphire/Sky/Teal/Pink/Flamingo/
    Rosewater/Lavender/Maroon) so a payload's colors never collide with its
    own line's source-prefix color
  - Self-contained, ~100-150 lines, no shared risk with other log-pane code
  - Custom highlight patterns — not scoped, future idea only
  - Status: ❌ Not started — spec locked via grilling session, ready to implement

- [ ] **Bookmarks**
  - Press `b` to bookmark current log line
  - Jump to bookmarks with number keys
  - Status: ❌ Not started

---

## v0.4.0 - Configuration & Persistence

Focus: User preferences and saved state.

### 🟡 MEDIUM Priority

- [ ] **Configuration File**
  - Save/load from `~/.config/ktails/config.yaml`
  - Theme selection (support multiple themes)
  - Default follow mode
  - Max log lines in memory
  - Auto-refresh interval
  - Keyboard shortcuts customization
  - Status: ⚠️ Partial (structs exist, no file I/O)

- [ ] **Session Persistence**
  - Remember last viewed contexts
  - Restore pod selection on restart
  - Save window layout preferences
  - Status: ❌ Not started

- [ ] **Recent Pods History**
  - Keep list of recently viewed pods
  - Quick access with keyboard shortcut
  - Limit to last 20 pods
  - Status: ⚠️ Partial (struct exists, not used)

---

## v1.0.0 - Polish & Performance

Focus: Production-ready stability and performance.

### 🔴 HIGH Priority

- [ ] **Performance Optimization**
  - Efficient log streaming (don't keep all logs in memory)
  - Virtual scrolling for large pod lists
  - Lazy loading for contexts with many namespaces
  - Status: ❌ Not started

- [ ] **Resource Management**
  - Proper cleanup on exit
  - Close goroutines and streams
  - Memory leak prevention
  - Status: ⚠️ Partial (basic cleanup exists)

- [ ] **Comprehensive Testing**
  - Unit tests for models
  - Integration tests for k8s client
  - TUI interaction tests
  - Status: ❌ Not started

### 🟡 MEDIUM Priority

- [x] **Multiple Log Panes** (shipped as a merged single pane, not split screen)
  - View logs from multiple pods simultaneously — done via `Space`-checked rows + `l`, merged into one scrollback with colored per-source prefixes
  - Split screen horizontally/vertically — not done; deliberately chose a single merged pane instead (see `design-log-viewer.md`'s "Future" section); still open if the merged view proves insufficient
  - Sync scroll option — not applicable to a single merged pane
  - Status: ✅ Done (merged-pane version); split-screen variant still open

- [ ] **Metrics Integration**
  - Show pod CPU/memory usage in list
  - Optional metrics-server integration
  - Visual indicators for high resource usage
  - Status: ❌ Not started

- [ ] **Advanced Filtering**
  - Label selector UI
  - Field selector support
  - Save filter presets
  - Status: ❌ Not started

### 🟢 LOW Priority

- [ ] **Plugins/Extensions**
  - Plugin API for custom commands
  - Custom log parsers
  - Status: ❌ Not started

- [ ] **Notifications**
  - Alert on pod state changes
  - Desktop notifications (optional)
  - Status: ❌ Not started

---

## Completed Features ✅

### v0.0.1 - Foundation

- ✅ Multi-context listing
- ✅ Context selection (single and multi-select)
- ✅ Pod listing with detailed info
- ✅ Tab navigation between panes
- ✅ Focus management
- ✅ Catppuccin Mocha theme
- ✅ Help mode (? key)
- ✅ Basic keyboard navigation
- ✅ Window resizing support

### v0.0.2 - Deployments, Services & Resource Detail

- ✅ Deployments tab (list + status)
- ✅ Services (svc) tab (list + status)
- ✅ Cross-cutting Detail pane — press `Enter` on any row in any of the three
  tabs to see Status conditions, recent Events, and full YAML for that
  resource, without it behaving like a peer tab
- ✅ `Ctrl+R` to jump back into an already-open Detail pane without re-fetching;
  `Enter` refocuses instantly when re-entering the same row
- ✅ Per-context error banner + summary overlay
- ✅ Status bar with live per-tab row counts and loading indicator
- ✅ Small-terminal guard (resize prompt below 80x24)

### Performance & Internals

(Folded in from the former `improvements.md`, which is now merged into this file.)

- ✅ Single `AppState` (`internal/state/state.go`) — no more duplicate state tracking
- ✅ Table re-render caching — `rowsSet`/`viewDirty` on `DeploymentPage`/`PodPage`/`ServicePage`;
  `SetRows` no-ops when rows are unchanged (`rowsEqual`)
- ✅ Batched lock reads — `AppState.Snapshot()` uses a cached fast-path under `RLock`, full
  recompute only under `Lock`
- ✅ Parallel context loading — `tea.Batch(cmdSequence...)` in the `ContextsStateMsg` handler
- ✅ Error recovery on success — `AppState.SetDeployments` clears a context's error on a
  successful load
- ✅ Update timing measurement — `logSlowUpdate` logs any `Update()` call over 16ms
- ✅ Dead code removed — `cmd/test-client` no longer exists; `PodPage` is fully integrated

---

## Backlog / Future Ideas

These are ideas for future consideration, not committed to any version:

- [ ] **Context Groups** - Save groups of contexts for quick switching
- [ ] **Log Analytics** - Basic log analysis (error count, patterns)
- [ ] **Exec Mode** - Execute commands in pods (like k9s shell)
- [ ] **Port Forwarding** - Quick port-forward setup from TUI
- [ ] **Resource Editing** - Edit pod/deployment specs in-place
- [ ] **Multiple Themes** - Support different color schemes
- [ ] **Cloud Integration** - EKS/GKE/AKS specific features
- [ ] **Vim Key Bindings** - Full vim-style navigation
- [ ] **Log Streaming to External** - Send logs to file/syslog/etc.
- [ ] **Namespace Filtering** - Filter whole UI by namespace
- [ ] **Log Stream Auto-Retry** - Deliberately deferred (grilling session, 2026-07-19): a dead log
  source today just shows an inline banner and stops (manual `l` restarts it in one keypress); no
  evidence yet that this is actually annoying in practice. Don't build reconnect-with-backoff
  speculatively — revisit only if manual restart proves to be real friction.

---

## Known Issues

Track current bugs and limitations:

- ⚠️ Resource table rows aren't color-coded by status/phase — see **Status Colors** above for the
  locked spec (Pods phase + Deployments ready/desired fraction)
- ⚠️ No way to deselect/remove a single loaded context's resources without deselecting the context itself
- ⚠️ Context switching modifies global k8s client state (not thread-safe)

---

## Contributing

Want to work on a feature? See [`plan-quick-wins.md`](./plan-quick-wins.md) for
a phased, parallelizable execution plan covering the items below whose specs
are already locked (Tab Gating Consistency, Checked-Count Indicator, Manual
Refresh, Auto-Refresh, Status Colors, Log Highlighting (JSON)) — it sequences
them into tracks by file ownership so multiple agents/contributors can pick up
different tracks without stepping on each other.

See [`plan-horizontal-scroll.md`](./plan-horizontal-scroll.md) for the
similarly phased plan covering Horizontal Scrolling (table/Detail/Log panes,
including a `bubbles/table` → `evertras/bubble-table` migration) and the Log
pane's Soft Wrap Toggle.

**Needs design discussion first (not in the plan above):**
1. Search mode implementation

See [CONTRIBUTING.md](./CONTRIBUTING.md) for detailed guidelines.