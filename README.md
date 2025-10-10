# ktails

Multi-context Kubernetes log viewer

Tail logs from multiple pods across different contexts, side by side.

## The Problem

When managing multi-tenant Kubernetes clusters (namespace-per-tenant),
operators need to constantly switch contexts to compare logs between tenants.
This is slow, breaks flow, and makes debugging painful.

## The Solution

`ktails` lets you view logs from multiple pods/namespaces/contexts
simultaneously in a split-pane TUI.

## MVP Features (v0.1)

- [ ] Interactive pod selection (context → namespace → pod)
- [ ] Split-pane view (2 panes side-by-side)
- [ ] Real-time log streaming
- [ ] Pod info header (status, restarts, age)
- [ ] Independent scrolling per pane
- [ ] Focus switching (Tab key)
- [ ] Follow mode (auto-scroll)

---

## **Step 6: Task Breakdown**

## Phase 1: Bubble Tea Fundamentals (Week 1)

### Day 1-2: Split Pane Static Viewer

- [ ] Create basic split pane layout (2 columns)
- [ ] Show static text in each pane
- [ ] Tab key switches focus
- [ ] Visual focus indicator (border color)
- [ ] Arrow keys scroll focused pane

**Success criteria:** Can navigate between two panes showing different text

### Day 3-4: Async Updates

- [ ] Background goroutine sends messages every 1s
- [ ] Messages update specific pane
- [ ] Text appears to "stream" into viewport
- [ ] Auto-scroll to bottom in follow mode
- [ ] 'f' key toggles follow mode

**Success criteria:** Text streams into panes from background process

### Day 5-7: Selection UI

- [ ] Create list component with sample data
- [ ] Arrow keys navigate list
- [ ] Enter selects item
- [ ] Show selection result
- [ ] Esc cancels selection

**Success criteria:** Can select from a list and see the result

---

## Phase 2: K8s Integration (Week 2)

### Day 1-2: K8s Client Setup

- [ ] Load kubeconfig
- [ ] List available contexts
- [ ] Switch context programmatically
- [ ] Handle missing kubeconfig gracefully

### Day 3-4: Pod Operations

- [ ] List pods in namespace
- [ ] Get pod details (status, restarts, age)
- [ ] Handle pod not found
- [ ] Handle context not available

### Day 5-7: Log Streaming

- [ ] Stream logs from pod
- [ ] Parse log lines
- [ ] Handle log stream errors
- [ ] Reconnect on failure
- [ ] Tail last N lines

**Success criteria:** CLI tool that tails logs from any pod

---

## Phase 3: Single Pane TUI (Week 3)

### Integration

- [ ] Connect selection UI → K8s client
- [ ] Show real contexts/namespaces/pods
- [ ] Stream real logs into viewport
- [ ] Display real pod info

### Features

- [ ] Search logs (/ key)
- [ ] Copy log line (y key)
- [ ] Refresh pod info (r key)
- [ ] Toggle timestamps
- [ ] Color-coded log levels

**Success criteria:** Usable single-pane log viewer

---

## Phase 4: Split Pane (Week 4)

### Core

- [ ] Duplicate single pane logic
- [ ] Route messages to correct pane
- [ ] Independent streaming per pane
- [ ] Tab switches focus

### UX

- [ ] 's' key opens pod selector for second pane
- [ ] Clear visual distinction between panes
- [ ] Sync scroll option (= key)
- [ ] Compare mode (highlight differences)

**Success criteria:** Can view two pod logs side-by-side

---

## Phase 5: Polish (Week 5)

- [ ] Syntax highlighting for log levels
- [ ] Better error messages
- [ ] Loading spinners
- [ ] Keyboard shortcuts help (?)
- [ ] Config file support
- [ ] Recent pods history
- [ ] Save/load sessions

**Success criteria:** Tool feels polished and professional
