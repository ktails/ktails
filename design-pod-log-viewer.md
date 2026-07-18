# Design: Pod Log Viewer (second bottom pane)

Status: proposed, not yet implemented.
Tracks: `todo.md` v0.1.0 → "Basic Log Viewing".

## Summary

Add a live-tailing log pane for Pods, reachable with `l`. It shares the same
cross-cutting bottom-split mechanics as the existing Detail pane (`Enter`),
but is an independent pane — the two are mutually exclusive in v0.1.0 (opening
one closes the other), never combined. `c` cycles through a pod's containers
while the log pane has focus.

## Decisions (from grilling session)

| # | Decision | Rationale |
|---|---|---|
| 1 | Trigger key: `l` | Mnemonic, unused in current keymap |
| 2 | Multi-container: cycle with `c` | Deferred container picker rejected — cycling is simple enough to build now |
| 3 | Container list sourced from a new hidden column on the Pods table, populated at list-load time | Matches the existing hidden-column pattern (`Context`); avoids a second async round-trip when `l` is pressed |
| 4 | Backfill `TailLines: 200` + `Follow: true` | Same convention as `kubectl logs -f --tail=200`; avoids streaming a huge log from the start |
| 5 | Ring buffer cap: 5000 lines | Bounds memory for a long-lived, chatty stream |
| 6 | Auto-scroll only if already at bottom (`viewport.AtBottom()`) | `tail -f` feel without yanking the user back down mid-read |
| 7 | Log pane pinned to the pod it was opened for; `l` on same pod refocuses, `l` on a different pod restreams | Matches existing Detail pane pinning behavior (`ResourceDetailPage.Matches`) |
| 8 | Opening Detail closes Logs and vice versa (v0.1.0) | Simplest correct behavior; **explicitly not** the final architecture — see "Future" below |
| 9 | Esc peel order: logs slot in alongside Detail's existing unfocus→close chain; `c` only fires while log pane is focused | Consistent with `Ctrl+R`/scroll-key handling already scoped to `detailFocused` |
| 10 | Stream errors append an inline banner line, scrollback is preserved (not replaced by a full-pane error) | Losing scrollback mid-read is worse here than in the one-shot YAML/status case |

## Data model changes

- `k8s.PodInfo` gains `Containers []string`, populated in `podToPodInfo` from
  `pod.Spec.Containers`.
- `podTableColumns()` gains a new hidden column (`Width: 0`), analogous to the
  existing hidden `Context` column, carrying `strings.Join(containers, ",")`.
- `cmds.LoadPodInfoCmd` appends this joined string as the row's new last
  column.

## Streaming architecture

- `k8s.Client.StreamLogs` (already exists) called with
  `&v1.PodLogOptions{Follow: true, TailLines: int64Ptr(200), Container: name}`.
- New `cmds.OpenPodLogStreamCmd(client, ctx, ns, pod, container, sessionID)`
  opens the stream and returns `msgs.LogStreamOpenedMsg{SessionID, Stream}`
  (or `msgs.LogStreamClosedMsg{SessionID, Err}` if opening itself fails).
- New `waitForLogLineCmd(sessionID, scanner)` reads one line via
  `bufio.Scanner` (buffer sized up to 1MB/line) and returns
  `msgs.LogLineMsg{SessionID, Line}`, re-issuing itself in `MainPage.Update`
  to keep the read loop going; `msgs.LogStreamClosedMsg{SessionID, Err}` on
  EOF/scanner error.
- `SessionID` is a counter on `MainPage`, bumped on every stream (re)start —
  lets `MainPage.Update` silently drop messages from a since-superseded
  stream (old pod, old container, or a closed pane) instead of corrupting the
  live view.

## `models.LogPage` (new, pure render/state model — mirrors `ResourceDetailPage`)

- Ring-buffered `[]string` (cap 5000) + `viewport.Model`.
- `containers []string`, `containerIdx int`.
- `Matches(pod, namespace, context string) bool` — pinning check, ignores
  container (container cycling happens within an already-open pane).
- `AppendLine(line string)` — auto-`GotoBottom()` only if the viewport was
  already at the bottom before appending.
- `SetStreamEnded(err error)` — appends a styled banner line, keeps existing
  scrollback, marks `streaming = false`.
- `Header(width int)` — banner line, includes pod/context, current container
  index/total, and key hints (`c`: cycle container, scroll/Esc hints).
- No knowledge of `io.ReadCloser`/scanners — all stream I/O orchestration
  lives in `cmds` + `MainPage`, same separation `ResourceDetailPage` already
  has from `cmds.LoadPodDetailCmd`.

## `MainPage` wiring

New fields: `podLogs *models.LogPage`, `showLogs`, `logsFocused bool`,
`logSessionID int`, `logStream io.ReadCloser`, `logScanner *bufio.Scanner`.

- `l` (Pods tab, list focus, `appStateLoaded`): same pod → refocus only, no
  restream. Different pod → close current stream (if any), bump
  `logSessionID`, reset `podLogs`, close `showDetail` if open, issue
  `OpenPodLogStreamCmd` for `containers[0]`.
- `Enter` (open Detail): additionally closes the log stream/pane if open —
  mutual exclusion is symmetric.
- `c`: intercepted at `MainPage` level *before* forwarding to
  `podLogs.Update`, and only while `logsFocused` — closes the current
  stream, advances `containerIdx` circularly, bumps `logSessionID`, opens a
  new stream for the next container. No-op if the pod has only one
  container.
- Esc peel order gains a branch parallel to the existing Detail one
  (`logsFocused → false`, then `showLogs → false` + stream close); since the
  two panes are mutually exclusive only one branch is ever live.
- `applyContentSizes` / `View` / non-key message forwarding extended in
  parallel with the existing `showDetail` handling — whichever of
  Detail/Logs is open gets the bottom split.
- `logStream.Close()` called defensively before `tea.Quit` as well as on
  every pane-close/restream path.

## Implementation phases

### Phase 1 — Container plumbing (no UI yet)
- `k8s.PodInfo.Containers`, `podToPodInfo` population.
- Hidden `Containers` column on `podTableColumns()` + `LoadPodInfoCmd`.
- No behavior change yet; existing Pods tab/detail pane unaffected.
- Verify: `go build ./...`, run app, confirm Pods tab renders identically.

### Phase 2 — Streaming plumbing (`msgs` + `cmds`)
- `msgs.LogStreamOpenedMsg`, `msgs.LogLineMsg`, `msgs.LogStreamClosedMsg`.
- `cmds.OpenPodLogStreamCmd`, `waitForLogLineCmd`, `int64Ptr` helper.
- No wiring into `MainPage` yet — compiles, unused until Phase 4.

### Phase 3 — `models.LogPage`
- New file `internal/tui/models/podlogs.go`: struct, `NewLogPage`,
  `Matches`, `Reset`, `AppendLine`, `SetStreamEnded`, `Header`, `View`,
  `Update` (scroll keys, `home`/`g`, `end`/`G`), `SetSize`, `SetFocused`,
  `Containers`/`CurrentContainer`/`NextContainer` accessors.
- Unit-testable in isolation (no k8s/io dependency).

### Phase 4 — `MainPage` wiring
- New fields, `l`/`c` key handling, Esc peel branch, mutual exclusion with
  Detail, `applyContentSizes`/`View`/forwarding updates, session-ID staleness
  checks, quit-time cleanup.
- Update `CONTEXT.md` glossary with a "Log Pane" / "Log Focus" entry
  mirroring the existing "Detail Pane" / "Detail Focus" entries.
- Update `todo.md`: mark "Basic Log Viewing" done, add multi-container item
  as done, note "Multiple Log Panes" (v1.0.0 backlog) as the deferred future
  work from decision #8.

### Phase 5 — Manual verification
- Multi-container and single-container pods, container cycling, pod
  switching while pane open/unfocused, Esc peel chain, mutual exclusion with
  Detail pane in both directions, stream-error banner (e.g. delete the pod
  being tailed), quit while a stream is active (no goroutine/fd leak
  warnings), small-terminal guard still holds.

## Future (explicitly out of scope for v0.1.0)

Decision #8 accepts single-instance/mutually-exclusive panes for now, but
`LogPage`'s stream orchestration is kept out of the model itself (living in
`cmds`/`MainPage`) specifically so a future multi-tab log viewer (`todo.md`
v1.0.0 "Multiple Log Panes") can generalize `MainPage`'s single
`podLogs`/`logStream` fields into a slice/map of sessions without having to
redesign `LogPage` itself.
