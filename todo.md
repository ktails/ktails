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
  - Press `r` to refresh the current tab's resource list
  - Visual feedback during refresh (loading indicator already exists for initial load)
  - Preserve cursor position after refresh
  - Status: ❌ Not started (only loads once on context selection)

- [ ] **Basic Log Viewing**
  - Press a key on a pod to view its logs
  - Display logs in a dedicated pane, with follow/tail-f mode
  - Scroll through logs with arrow keys
  - Status: ❌ Not started — the app currently shows Pod *metadata and detail* (Status/Events/YAML via the Detail pane), not log streams

### 🟡 MEDIUM Priority

- [ ] **Auto-Refresh**
  - Configurable refresh interval (default: 5s)
  - Subtle spinner in status bar during refresh
  - Toggle auto-refresh with a key
  - Status: ❌ Not started

- [x] **Status Colors**
  - Color code Detail pane event types (Warning=yellow, Normal=green)
  - Highlight selected row with accent color
  - Show focused pane with distinct border color/thickness
  - Status: ✅ Done — Catppuccin palette used consistently; resource-table status text itself is not yet color-coded by phase (Running/Pending/Failed)

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

- [ ] **Horizontal Scrolling**
  - Left/Right arrow keys to scroll horizontally
  - Useful for long pod names or wide tables
  - Show scroll position indicator
  - Status: ❌ Not started

---

## v0.3.0 - Log Enhancements

Focus: Advanced log viewing capabilities.

### 🟡 MEDIUM Priority

- [ ] **Soft Wrap Toggle**
  - Press `w` to toggle soft wrap for long log lines
  - Remember preference per session
  - Status: ❌ Not started

- [ ] **Log Export**
  - Press `e` to export current logs to file
  - Configurable export location
  - Include timestamp in filename
  - Status: ❌ Not started

- [ ] **Multi-Container Support**
  - Select container when pod has multiple containers
  - Switch between container logs with `c` key
  - Show current container in status bar
  - Status: ❌ Not started

- [ ] **Timestamp Control**
  - Toggle timestamp display with `t` key
  - Configure timestamp format
  - Show relative or absolute times
  - Status: ❌ Not started

### 🟢 LOW Priority

- [ ] **Log Highlighting**
  - Syntax highlighting for common log formats (JSON, etc.)
  - Custom highlight patterns
  - Status: ❌ Not started

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

- [ ] **Multiple Log Panes**
  - View logs from multiple pods simultaneously
  - Split screen horizontally/vertically
  - Sync scroll option
  - Status: ❌ Not started

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

---

## Known Issues

Track current bugs and limitations:

- ⚠️ No manual refresh — resource lists load once on context selection and don't update
- ⚠️ Resource table rows aren't color-coded by status/phase (Running/Pending/Failed)
- ⚠️ No log viewing yet — Pod detail shows metadata/Status/Events/YAML, not log streams
- ⚠️ No way to deselect/remove a single loaded context's resources without deselecting the context itself
- ⚠️ Context switching modifies global k8s client state (not thread-safe)

---

## Contributing

Want to work on a feature? Here's the suggested order:

**For New Contributors:**
1. Status bar implementation (good first task)
2. Error display panel
3. Manual refresh (r key)

**Core Features (need design discussion first):**
1. Log viewing architecture
2. Search mode implementation
3. Auto-refresh strategy

See [CONTRIBUTING.md](./CONTRIBUTING.md) for detailed guidelines.