# Design: Multi-pod merged log viewer

Status: proposed, not yet implemented.
Tracks: `todo.md` v1.0.0 → "Multiple Log Panes" (this ships the merged
single-pane version, not the originally-imagined split-screen version).
Supersedes/extends: `design-pod-log-viewer.md` (single-pod log pane).

## Summary

Extend the existing single-pod live log viewer (`l` key, mutually
exclusive with the Detail pane) into a **multi-pod, multi-container merged**
log viewer, inspired by [`atombender/ktail`](https://github.com/atombender/ktail)
— a CLI that tails many pods/containers by regex/label, merging them into
one colorized stream. ktails adapts this to its TUI/row-selection model:
you check pods in the Pods tab (`Space`), then `l` opens one merged pane
streaming every container of every checked pod, each line prefixed with a
colored `pod/container` tag.

## Decisions (from grilling session)

| # | Decision | Rationale |
|---|---|---|
| 1 | Scope: multi-pod merged tailing (not single-pod + niceties) | This is ktail's headline, distinguishing feature |
| 2 | Selection: `Space`-multiselect rows in the Pods tab (reuses the context-list pattern) | Familiar interaction already in the app; no new pattern/label input UI needed |
| 3 | No auto-discovery of new pods matching a pattern/label | Static snapshot is simpler and matches "you picked specific rows," not a live query; can revisit later |
| 4 | All containers per checked pod are tailed (not one container per pod) | Matches ktail's "all containers in a pod tailed by default" |
| 5 | Single merged pane, one scrollback (not separate panes per source) | Matches ktail's actual output model (one interleaved stream); reuses existing viewport/ring-buffer machinery |
| 6 | Colored `pod/container` prefix per line, distinct color per source | Matches ktail's colorized multi-source output; colors drawn from unreserved Catppuccin Mocha accents |
| 7 | No auto-retry on stream failure — inline banner only | Simpler; per-source reconnect logic deferred until there's evidence it's needed |
| 8 | JSON syntax highlighting deferred to a separate follow-up | Separable, moderate-sized addition with its own edge cases; keeps this change scoped to the merge itself |
| 9 | `l` with nothing checked falls back to today's single-row behavior | Backward compatible — no muscle-memory break for existing single-pod usage |
| 10 | `c` repurposed from "cycle container" to "isolate to one source" | No single "current container" concept once all containers stream at once; keeps the key useful |
| 11 | Isolating via `c` is view-only — all sources keep streaming in the background | Returning to full merge shows no gaps; only the rendered view changes |
| 12 | Ring buffer cap (5000 lines) applies **per source**, not globally | A noisy container can't evict a quiet one's history |
| 13 | Checkmarks persist after opening the pane, until explicitly cleared | Lets you reopen/adjust the same set without rechecking from scratch |
| 14 | `l` while the pane is open reconciles it to current checkmarks (opens new, closes unchecked, leaves rest alone) | Matches how you'd naturally iterate on a selection |

## Data model changes

- `internal/tui/models/pods.go` (`PodPage`) gains a `checkedPods map[string]bool`
  keyed by `context+"/"+namespace+"/"+name`, plus `ToggleChecked`,
  `ClearChecked`, `CheckedKeys`, `IsChecked`. A new narrow "✓" column (first,
  visible) renders the checkbox glyph per row; existing hidden `Context`/
  `Containers` columns shift right by one index accordingly.
- `internal/tui/models/podlogs.go` (`LogPage`) becomes a merged multi-source
  pane:
  - `logSource{Key, PodName, Namespace, Context, Container, Color, lines []string, streaming, streamErr}`
    — its own 5000-line ring buffer, trimmed from the front independently.
  - `sources map[string]*logSource`, `order []string` (insertion order, for
    stable color assignment and isolate-cycle order), `isolatedIdx int`
    (`-1` = full merge, else index into `order`).
  - Merged (non-isolated) rendering is a k-way merge of each source's
    already-chronologically-ordered `lines` (cheap: sources are few, each
    slice internally ordered since bubbletea processes one `LogLineMsg` at
    a time) — no global sequence counter needed.
  - Per-source `SetStreamEnded`-equivalent appends the existing red banner
    into just that source's `lines`, no retry.
- `internal/tui/msgs/msgs.go`: `LogStreamOpenedMsg`/`LogLineMsg`/
  `LogStreamClosedMsg` swap `SessionID int` for `SourceKey string` +
  `Generation int`, so stale-message detection is scoped per source instead
  of one global counter.
- `internal/tui/cmds/cmds.go`: `OpenPodLogStreamCmd`/`WaitForLogLineCmd`
  parameterized by `(sourceKey, generation)` instead of a single
  `sessionID`; `k8s.Client.StreamLogs` is unchanged (already single
  pod/container per call) — `MainPage` just issues one call per source.

## `MainPage` wiring

- `logStreams map[string]*logStreamState{stream, scanner, generation}`
  replaces the single `logStream`/`logScanner`/`logSessionID` fields.
- `Space` (Pods tab, list focus) toggles the row under the cursor;
  `Ctrl+X` clears all checkmarks.
- `l` reconciles the merged pane to `podList.CheckedKeys()` (or the cursor
  row if nothing's checked): opens sources newly present, closes sources no
  longer checked, leaves the rest running. Empty target set closes the pane
  entirely.
- `c` becomes a pure view toggle (`podLogs.CycleIsolation()`), no stream
  side effects.
- Message handling (`LogStreamOpenedMsg`/`LogLineMsg`/`LogStreamClosedMsg`)
  looks up `logStreams[msg.SourceKey]` and compares `msg.Generation`,
  dropping stale messages per source.
- Mutual exclusion with the Detail pane, `applyContentSizes`/`View` split
  logic, and quit-time cleanup are unchanged in shape — just operating over
  the `logStreams` map instead of a single stream.

## Future (explicitly out of scope for this change)

- Auto-discovery of new pods matching a label/pattern (decision #3) —
  would need a pod-watch mechanism the TUI doesn't have yet.
- Per-source reconnect/retry on failure (decision #7).
- JSON syntax highlighting (decision #8).
- True split-screen multiple log panes, as originally sketched in
  `todo.md`'s v1.0.0 "Multiple Log Panes" entry — this design deliberately
  chose a single merged pane instead; split-screen remains a possible
  future direction if the merged view proves insufficient.
