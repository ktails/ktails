# Design: Log viewer (single-pod → merged multi-pod)

Status: **Implemented** (both phases below), manually verified against a live cluster.
Tracks: `todo.md` v0.1.0 → "Basic Log Viewing", v0.3.0 → "Multi-Container Support",
v1.0.0 → "Multiple Log Panes" (shipped as the merged single-pane version).

This consolidates two design docs written across two grilling sessions:
Phase 1 (single-pod log pane) and Phase 2 (merged multi-pod tailing, inspired by
[`atombender/ktail`](https://github.com/atombender/ktail)). Kept as one file since
Phase 2 fully supersedes Phase 1's data model — no reason to read them separately.

## Summary

The Pods tab has a live-tailing log pane, reachable with `l`. It shares the same
cross-cutting bottom-split mechanics as the Detail pane (`Enter`) and is mutually
exclusive with it. `Space` checks one or more Pods rows; `l` opens (or reconciles)
a single merged pane streaming **every container of every checked pod**, each line
prefixed with a colored `pod/container` tag. `c` isolates the view to one source at
a time (cycling through sources, then back to the full merge) — all sources keep
streaming in the background regardless of what's currently isolated.

## Phase 1 decisions — single-pod pane

| # | Decision | Rationale |
|---|---|---|
| 1 | Trigger key: `l` | Mnemonic, unused in current keymap |
| 2 | Multi-container: cycle with `c` | Deferred container picker rejected — cycling is simple enough to build now |
| 3 | Container list sourced from a hidden column on the Pods table, populated at list-load time | Matches the existing hidden-column pattern (`Context`); avoids a second async round-trip when `l` is pressed |
| 4 | Backfill `TailLines: 200` + `Follow: true` | Same convention as `kubectl logs -f --tail=200`; avoids streaming a huge log from the start |
| 5 | Ring buffer cap: 5000 lines | Bounds memory for a long-lived, chatty stream |
| 6 | Auto-scroll only if already at bottom (`viewport.AtBottom()`) | `tail -f` feel without yanking the user back down mid-read |
| 7 | Log pane pinned to the pod it was opened for; `l` on same pod refocuses, `l` on a different pod restreams | Matches the Detail pane's pinning behavior |
| 8 | Opening Detail closes Logs and vice versa | Simplest correct behavior; explicitly not final — see Phase 2 |
| 9 | Esc peel order: logs slot alongside Detail's unfocus→close chain; `c` only fires while log pane focused | Consistent with existing scroll-key handling scoping |
| 10 | Stream errors append an inline banner line, scrollback preserved | Losing scrollback mid-read is worse than a full-pane error |

## Phase 2 decisions — merged multi-pod pane

Extends the pane to tail multiple pods/containers at once, merged into one
scrollback, reached via a grilling session:

| # | Decision | Rationale |
|---|---|---|
| 1 | Scope: multi-pod merged tailing (not single-pod + niceties) | This is ktail's headline, distinguishing feature |
| 2 | Selection: `Space`-multiselect rows in the Pods tab | Reuses the context-list pattern already in the app; no new pattern/label input UI |
| 3 | No auto-discovery of new pods matching a pattern/label | Static snapshot is simpler; a live query is a bigger, separate feature |
| 4 | All containers per checked pod are tailed (not one per pod) | Matches ktail's "all containers tailed by default" |
| 5 | Single merged pane, one scrollback (not separate panes per source) | Matches ktail's actual output model; reuses existing viewport/buffer machinery |
| 6 | Colored `pod/container` prefix per line, distinct color per source | Matches ktail's colorized output; colors from unreserved Catppuccin Mocha accents |
| 7 | No auto-retry on stream failure — inline banner only | Simpler; kept deferred in `todo.md` (Backlog) until proven to be real friction |
| 8 | JSON syntax highlighting deferred to a separate follow-up | Separable, own edge cases; full spec now lives in `todo.md`'s "Log Highlighting (JSON)" |
| 9 | `l` with nothing checked falls back to the Phase 1 single-row behavior | Backward compatible — no muscle-memory break |
| 10 | `c` repurposed from "cycle container" to "isolate to one source" | No single "current container" concept once all containers stream at once |
| 11 | Isolating via `c` is view-only — all sources keep streaming in the background | Returning to full merge shows no gaps |
| 12 | Ring buffer cap (5000 lines) applies **per source**, not globally | A noisy container can't evict a quiet one's history |
| 13 | Checkmarks persist after opening the pane, until explicitly cleared | Lets you reopen/adjust the same set without rechecking from scratch |
| 14 | `l` while the pane is open reconciles it to current checkmarks | Matches how you'd naturally iterate on a selection |

## Current implementation

- **`internal/tui/models/pods.go`** (`PodPage`) — `checkedPods map[string]bool` keyed
  by `context/namespace/name`; `ToggleChecked`/`ClearChecked`/`CheckedKeys`/`CheckedRow`/
  `PodRowKey`. A dedicated `✓` column (first, visible) renders the checkbox glyph;
  `SelectedRow`/raw rows stay unprefixed (`context/namespace/name/…/Containers`), so
  identity lookups are unaffected by the checkbox column.
- **`internal/tui/models/podlogs.go`** (`LogPage`) — merged multi-source pane:
  - `logSource{key, podName, namespace, context, container, color, lines []logLine, streaming, streamErr}`
    — its own 5000-line ring buffer, trimmed from the front independently.
  - `sources map[string]*logSource`, `order []string` (insertion order — stable color
    assignment + isolate-cycle order), `isolatedIdx int` (`-1` = full merge).
  - Each `logLine` carries a monotonic `seq` (via `LogPage.nextSeq`), so the merged
    (non-isolated) view can interleave all sources back into true chronological order
    — sources are buffered independently, so this sequence is the only thing that
    lets the merge be correct rather than just concatenated per-source.
  - Colors drawn round-robin from Blue/Lavender/Sapphire/Sky/Teal/Pink/Flamingo/
    Rosewater/Yellow/Maroon (Red/Mauve/Green/Peach excluded — already reserved
    elsewhere in the UI).
  - Per-source `SetStreamEnded` appends the red banner into just that source's
    `lines`, no retry.
- **`internal/tui/msgs/msgs.go`** — `LogStreamOpenedMsg`/`LogLineMsg`/
  `LogStreamClosedMsg` carry `SourceKey string` + `Generation int` (not a single
  global `SessionID`), so stale-message detection is scoped per source.
- **`internal/tui/cmds/cmds.go`** — `OpenPodLogStreamCmd`/`WaitForLogLineCmd`
  parameterized by `(sourceKey, generation)`; `k8s.Client.StreamLogs` unchanged
  (already single pod/container per call) — `MainPage` issues one call per source.
- **`internal/pages/mainPage.go`** — `logStreams map[string]*logStreamState{stream,
  scanner, generation}` replaces the single-stream fields. `Space` toggles a Pods
  row; `Ctrl+X` clears all checks. `openPodLogs()` reconciles the merged pane to
  `podList.CheckedKeys()` (or the cursor row if nothing's checked): opens newly
  targeted sources, closes ones no longer checked, leaves the rest running; an
  empty target set closes the whole pane. `c` calls `podLogs.CycleIsolation()` —
  a pure view toggle, no stream side effects.

## Next steps (not yet implemented)

Fully specified via a follow-up grilling session (2026-07-19); full detail lives in
`todo.md` under the entries named below — this is just a pointer so the next agent
knows where to look, not a duplicate spec:

- **Checked-Count Indicator** (`todo.md`, v0.1.0) — status bar hint while pods are checked.
- **Log Highlighting (JSON)** (`todo.md`, v0.3.0) — embedded-JSON detection, in-place token coloring.
- **Log Stream Auto-Retry** (`todo.md`, Backlog) — deliberately deferred, not scheduled.
- **Tab Gating Consistency** (`todo.md`, v0.1.0) — unrelated to log streaming itself, but
  was grilled in the same session; extends the Deployments-only tab guard to Pods/svc.

## Future (explicitly out of scope)

- Auto-discovery of new pods matching a label/pattern (Phase 2, decision #3) — would
  need a pod-watch mechanism the TUI doesn't have yet.
- True split-screen multiple log panes, as originally sketched in `todo.md`'s v1.0.0
  "Multiple Log Panes" entry — deliberately chose a single merged pane instead; still
  open if the merged view proves insufficient in practice.
