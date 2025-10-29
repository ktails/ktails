# KTails - Priority Improvements

## 🚀 Performance & State Management

### 1. Eliminate Duplicate AppState
**Problem:** You have two identical `AppState` implementations:
- `internal/pages/state.go` 
- `internal/state/state.go`

**Fix:** Delete `internal/pages/state.go` and use only `internal/state/state.go`. Update imports in `mainPage.go`.

### 2. Optimize Table Rendering
**Current Issue:** Table re-renders completely on every update.

**Improvements:**
- Only call `SetRows()` when data actually changes
- Cache the rendered table view when data hasn't changed
- Use `deploymentsDirty` flag pattern for table rendering too

### 3. Reduce Lock Contention
**Problem:** Heavy mutex usage in AppState for reads.

**Fix:**
```go
// Use RWMutex more effectively - batch reads
func (a *AppState) GetState() (contexts map[string]string, deployments []table.Row, loading bool) {
    a.mu.RLock()
    defer a.mu.RUnlock()
    // Return everything in one call
}
```

### 4. Async Loading Improvements
**Current:** All contexts load sequentially in `tea.Sequence()`

**Better:**
```go
// Load contexts in parallel
return tea.Batch(cmdSequence...)
```

---

## ✨ Minimal Polish

### 5. Loading State Visibility
**Add:** Simple spinner next to context name while loading
```go
"⏳ context-name" // when loading
"✓ context-name"  // when done
```

### 6. Empty State Messages
**Current:** Shows "No contexts selected..." but unclear

**Better:**
```go
if !m.appStateLoaded {
    return styles.HelpStyle.Render(
        "No contexts selected\n\n" +
        "Press Ctrl+T to focus context list\n" +
        "Use Space to select, Enter to load"
    )
}
```

### 7. Error Recovery
**Add:** Auto-clear errors after viewing deployments for different context
```go
case msgs.DeploymentTableMsg:
    if msg.Err == nil {
        // Success - clear this context's error
        delete(m.appState.Errors, msg.Context)
    }
```

### 8. Keyboard Hints in Status Bar
**Current:** Status bar shows state, but no action hints

**Add:**
```go
hints := "Ctrl+T:Focus  Ctrl+E:Clear Errors  q:Quit"
rightBits = append(rightBits, lipgloss.NewStyle().Faint(true).Render(hints))
```

---

## 🎯 Quick Wins (< 30 min each)

### 9. Remove Unused Code
- `internal/tui/models/pods.go` - skeleton import unused
- `cmd/test-client/main.go` exists but use `cmd/page-client/main.go`
- `PodPage` is defined but never integrated into main view

### 10. Focus Indicator Polish
**Current:** Hard to see which pane is focused

**Add border thickness differentiation:**
```go
// Focused border = double line
LeftPane = lipgloss.NewStyle().Border(lipgloss.DoubleBorder(), true)
// Blurred border = normal line  
LeftPaneBlur = lipgloss.NewStyle().Border(lipgloss.NormalBorder(), true)
```

### 11. Tab Switching Without Content
**Current:** Can switch to "Deployments" tab even with no contexts selected

**Fix:**
```go
case "right", "tab":
    if m.activeTab+1 >= len(m.tabs) {
        return m, nil // Don't wrap
    }
    // Only allow switch if content ready
    if m.tabs[m.activeTab+1] == "Deployments" && !m.appStateLoaded {
        return m, nil // Block switch
    }
    m.activeTab++
```

---

## 📊 Measurement Suggestions

Add simple timing to see impact:
```go
// In mainPage.go Update()
start := time.Now()
defer func() {
    if time.Since(start) > 16*time.Millisecond {
        log.Printf("Slow update: %v", time.Since(start))
    }
}()
```

---

## Impact Priority

1. **Fix #1** (duplicate AppState) - ⚡ Immediate cleanup
2. **Fix #4** (parallel loading) - ⚡ Noticeable speed boost
3. **Fix #6** (empty states) - ✨ Much better UX
4. **Fix #8** (keyboard hints) - ✨ Reduces confusion
5. **Fix #2** (table caching) - ⚡ Smoother rendering

Start with these 5 and you'll have a noticeably snappier, more polished app.
