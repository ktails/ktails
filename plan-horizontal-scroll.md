# Horizontal scroll + log wrap — execution plan (phased, parallelizable)

Full spec locked in `todo.md` (via a grilling session, 2026-07-19) under
"Horizontal Scrolling" and "Soft Wrap Toggle" — this file only sequences
*how* to build it (phases, tracks, file ownership), not *what* to build.
Read both `todo.md` entries before starting a track; this file won't repeat
that detail.

## Status

- ✅ **Track M** — bubble-table migration + table wide-mode/scroll/freeze.
  Done, merged.
- ✅ **Track M2** — wide mode reveals new columns (Pods: Ready/Node/Node
  IP/Pod IP; Deployments: Available/Updated/Strategy/Selector; Services:
  Selector/External IP/Endpoint IPs). Done, merged.
- ⬜ **Track F** — Detail pane horizontal scroll. Not started.
- ✅ **Track G** — Log pane wrap toggle + horizontal scroll. Done, merged.

F and G are independent of each other and of M/M2 (different files —
`resourcedetail.go` and `podlogs.go` respectively) and can run in parallel
whenever picked up.

## How to use this plan

Each **track** is an independent unit of work claimable by one agent. Tracks
within the same phase have no functional dependency on each other and can run
in parallel — ideally each in its own git worktree (`isolation: "worktree"`
if launching via the Agent tool), merged back independently. Track M is
larger and riskier than F/G (a full table-library migration) — merge it
first even though F/G don't technically depend on it, since it's the biggest
diff and easiest to reconcile into a clean history before smaller changes
land on top.

---

## Phase 1 — three parallel tracks, no cross-track dependencies

### Track M — bubble-table migration + table wide-mode/scroll/freeze — ✅ done, merged

Landed as-scoped below. **Superseded in part**: the "wide mode" framing here
(auto-fit-to-content-width of the *existing* columns) turned out to be too
narrow — see Track M2 below, which re-scopes wide mode to also reveal new
columns, reusing everything built in this track rather than replacing it.
Kept here for the historical record of what Track M itself built.

Bundled into one track (not two) because the wide-mode/scroll/freeze work is
built directly on top of the migration in the same files — splitting them
across two agents would mean the second either blocks on the first landing
first (defeating the parallelism) or fights over `pods.go`/`deployment.go`/
`services.go`/`table.go` mid-migration. Build in this order:

1. **Migrate Pods/Deployments/svc tables from `charmbracelet/bubbles/table`
   to `github.com/evertras/bubble-table`**, preserving all existing behavior
   exactly: `WithNoPagination()` (no paging — full list, vertical scroll via
   up/down, matching today), row data moves from positional `table.Row`
   slices to keyed `RowData` maps (drop the hidden width-0 "carrier" columns
   used today for Context/Namespace/Containers — carry that data as
   non-displayed map keys instead), Status/Replica cell coloring moves from
   the post-render ANSI-recoloring hack (`colorizeStatusColumn`/
   `colorizeReplicaColumn`) to native `StyledCell`/`StyledCellFunc` (delete
   the old hack once replaced), Pods' checkbox column becomes a real bubble-
   table column. Every call site in `internal/pages/mainPage.go` that reads
   `SelectedRow()`/row data by positional index (Enter → detail, `l` → open
   logs, Space/Ctrl+X → checked rows, `r` → refresh preserving cursor) needs
   updating to the new keyed-row API. Verify manually against the existing
   verification checklist below before moving on — this step must not change
   any user-visible behavior.
2. **Wide-mode toggle** (`Ctrl+W`, per-tab, sticky across tab switches): swap
   between flex columns (today's shrink-to-fit) and fixed-width columns
   auto-fit to the longest current value per column, recomputed on every
   `SetRows`. `WithMaxTotalWidth` engages horizontal scroll once wide columns
   exceed the viewport.
3. **Column scroll** (`Shift+Left`/`Shift+Right`, one column at a time via
   `ScrollLeft`/`ScrollRight`) and **frozen checkbox column**
   (`WithHorizontalFreezeColumnCount(1)`, Pods only).
4. **Status-bar indicator** (`◂ col N/M ▸`, alongside the existing
   `⏳ N loading`/`☑ N checked` bits) and **persistence**: scroll position +
   wide-mode on/off survive manual/auto refresh, reset to leftmost/narrow on
   resize.

Files: `internal/tui/models/pods.go`, `internal/tui/models/deployment.go`,
`internal/tui/models/services.go`, `internal/tui/models/table.go`,
`internal/tui/cmds/cmds.go` (row-building, now keyed), `internal/pages/mainPage.go`
(row-access call sites, `Ctrl+W` handling, status bar indicator — different
regions than Tracks F/G touch, expect clean merge).

### Track M2 — wide mode: reveal new columns (builds on Track M) — ✅ done, merged

`todo.md`'s "Horizontal Scrolling" entry, "Tables — narrow/wide mode"
paragraph, re-scope note (grilling session, 2026-07-19, same day as Track
M). Depended on Track M having landed (it had). Landed as scoped below:
Pods wide mode gained Ready/Node/Node IP/Pod IP; Deployments gained
Available/Updated/Strategy/Selector; Services gained Selector/External
IP/Endpoint IPs (the last fetched lazily via one namespace-wide
`EndpointSlices` List call, joined client-side by the
`kubernetes.io/service-name` label, triggered only on the svc tab's
off→on `Ctrl+W` transition per context+namespace — see
`AppState.NeedsServiceEndpoints`/`MarkServiceEndpointsRequested` in
`internal/state/state.go` and `fetchServiceEndpointsIfNeeded` in
`internal/pages/mainPage.go`). Build order that was followed:

1. **New `*Info` fields, all free from the existing `List` calls** (no new
   API call):
   - `internal/k8s/client.go`'s `PodInfo`/`podToPodInfo`: add `NodeIP`
     (`pod.Status.HostIP`), `PodIP` (`pod.Status.PodIP`), and a
     ready-containers count derived from `pod.Status.ContainerStatuses`
     (e.g. rendered as `"2/3"` the same way `DeploymentInfo`'s
     ready/desired is today). `Node` already exists on `PodInfo` — it's
     just never been surfaced into a table row.
   - `internal/k8s/deployments.go`'s `DeploymentInfo`/`GetDeploymentInfo`:
     add `Strategy` (`deployment.Spec.Strategy.Type`), `AvailableReplicas`,
     `UpdatedReplicas`, `Selector` (render `deployment.Spec.Selector` via
     `metav1.LabelSelectorAsSelector(...).String()` or equivalent).
   - `internal/k8s/services.go`'s `ServiceInfo`/`GetServiceInfo`: add
     `Selector` (`svc.Spec.Selector`, a `map[string]string` — render as
     `key=value,key=value`), `ExternalIP` (from
     `svc.Status.LoadBalancer.Ingress`, hostname or IP, joined if multiple).
2. **Endpoint IPs for Services — the one field needing a new API call**:
   add a method alongside `GetServiceInfo` in `internal/k8s/services.go`
   that lists `EndpointSlices` (or `Endpoints`) for the whole namespace in
   one call and returns a `map[string][]string` (service name → endpoint
   IPs) for client-side joining — never one call per service. Wire this
   into `internal/tui/cmds/cmds.go` as a separate `tea.Cmd`
   (`LoadServiceEndpointsCmd` or similar) fetched lazily: only issued the
   first time wide mode toggles on for svc on a given context+namespace
   (track a per-context+namespace "already fetched" flag, same spirit as
   the existing per-context loading-state tracking in `AppState`), then
   cached — don't refetch on every wide-mode toggle or every refresh.
   `Ctrl+W` must reveal every other new svc column immediately regardless
   of whether this fetch has completed; the Endpoint IPs column shows `…`
   per row until the async result lands, then repopulates via the normal
   `SetRows` path.
3. **Row-building**: extend `LoadPodInfoCmd`/`LoadDeploymentInfoCmd`/
   `LoadServiceInfoCmd` in `internal/tui/cmds/cmds.go` (and the new
   endpoints-join step for svc) to carry these fields as additional
   `msgs.RowData` keys — always populated (cheap, same List call already
   made), regardless of whether wide mode is currently on. Add the new key
   constants next to the existing `PodKey*`/`DeployKey*`/`SvcKey*` ones in
   `internal/tui/msgs/msgs.go`.
4. **New wide-mode-only columns**: extend `podWideColumns`/
   `deploymentWideColumns`/`svcWideColumns` in `internal/tui/models/table.go`
   to include the new fields alongside the existing ones, using the same
   `paddedColumn`/`widestValue` auto-fit helpers already built — this is
   additive to those functions, not a new mechanism. Narrow-mode column
   functions (`podNarrowColumns` etc.) are untouched — the new fields are
   wide-mode-only. Pick a sensible column order (e.g. group Node/Node
   IP/Pod IP together for Pods, Ready-containers near Status/Restarts) —
   cosmetic, adjust if it doesn't read well.
5. Confirm the existing `◂ col N/M ▸` status-bar indicator, frozen Pods
   checkbox column, and refresh-survives/resize-resets persistence all
   still behave correctly now that `M` (total wide-mode columns) is larger
   per tab — this should fall out of Track M's existing logic without
   changes, since it already operates generically over "however many
   columns are in wide mode," but verify rather than assume.

Files: `internal/k8s/client.go`, `internal/k8s/deployments.go`,
`internal/k8s/services.go` (new fields + the new endpoints-list method),
`internal/tui/cmds/cmds.go` (row-building + new lazy-fetch `tea.Cmd`),
`internal/tui/msgs/msgs.go` (new row-data key constants),
`internal/tui/models/table.go` (wide-column functions only — narrow-column
functions and the auto-fit/scroll/freeze machinery are untouched),
`internal/state/state.go` (per-context+namespace "endpoints already
fetched" tracking, if it doesn't fit naturally into existing loading-state
tracking), `internal/pages/mainPage.go` (wiring the new lazy-fetch `tea.Cmd`
into the `Ctrl+W` handler for the svc tab specifically).

### Track F — Detail pane horizontal scroll

`todo.md`, Horizontal Scrolling spec, Detail-pane half. No toggle — Detail
(`bubbles/viewport`) is always horizontally scrollable when content overflows.
`Shift+Left`/`Shift+Right` scroll by half the viewport's width per press
(adapts to terminal size automatically). Status-bar indicator: percentage
scrolled (`◂ 40% ▸`). Scroll position survives refresh, resets on resize.

Wire the new keys into `mainPage.go`'s existing `m.detailFocused` block (which
already forwards everything to `m.deploymentDetail.Update(msg)` — either
handle `shift+left`/`shift+right` inside `resourcedetail.go`'s own `Update`,
or intercept it in `mainPage.go` next to the existing `ctrl+r`/ detail-focus
handling, whichever keeps the horizontal-offset state closest to the
viewport it affects).

Files: `internal/tui/models/resourcedetail.go`, `internal/pages/mainPage.go`
(detail-focused key handling + status bar — different region than Track M/G).

### Track G — Log pane wrap toggle + horizontal scroll — ✅ done, merged

Landed as scoped below. `w` (off by default) soft-wraps via
`github.com/charmbracelet/x/ansi.Wrap`, operating on the already-colored,
already-merged line text (`LogPage.rawLines`) so neither the source-prefix
nor JSON-highlight colors need to be re-derived. Unwrapped horizontal scroll
turned out to already be native to the vendored `bubbles/viewport`
(`v0.21.0` ships `xOffset`/`ScrollLeft`/`ScrollRight`/
`HorizontalScrollPercent`, cropping each visible line via the same
`ansi.Cut` under the hood) — `Shift+Left`/`Shift+Right` call straight
through to it rather than hand-rolling cropping. Wrap/scroll mutual
exclusivity and the reset-on-resize/reset-on-wrap-on rules are enforced in
`LogPage.ToggleWrap`/`SetSize`. See `internal/tui/models/podlogs_test.go`
for the ANSI-survival tests (wrap reflow, horizontal crop) that back this.


`todo.md`, Soft Wrap Toggle + Horizontal Scrolling's Log-pane half. `w` toggles
soft wrap (off by default); mutually exclusive with horizontal scroll (wrap
on disables scroll, wrap off enables it). Wrap preference persists in-memory
on the one long-lived `LogPage` instance for the session (not written to
disk). `Shift+Left`/`Shift+Right` scroll by half the viewport's width per
press when unwrapped. Status-bar indicator: percentage scrolled.

The main technical risk: wrapping must be ANSI-aware so it doesn't break the
per-source-prefix colors or the JSON-highlighting already shipped
(`highlightJSONLine`/`colorizeJSON` in `podlogs.go`) — reflow the line's
*rendered* (already-colored) content rather than re-deriving colors after
wrapping, similar in spirit to how the JSON highlighter already tracks byte
spans via `json.Decoder`.

Wire `w` into `mainPage.go`'s existing `m.logsFocused` block next to the
special-cased `c` (isolate/return-to-merged) key — same pattern, same
reasoning (a pure view toggle, no stream side effects).

Files: `internal/tui/models/podlogs.go`, `internal/pages/mainPage.go`
(logs-focused key handling + status bar — different region than Track M/F).

---

## Verification

After each track lands: `make build && make test && make lint && make fmt`
(per `AGENTS.md`/`Makefile`; note `staticcheck` is known to fail in this dev
environment due to a missing mise shim, unrelated to any of these changes —
verify by reproducing the identical failure on unmodified code if it comes
up again).

After Track M specifically (before F/G rely on a clean base): confirm nothing
regressed from the quick-wins work already shipped — Status colors (Pods
phase, Deployments ready/desired) still render correctly with no truncation
artifacts at narrow *and* half-terminal widths (this is exactly the bug class
that motivated the migration — verify it's actually gone, not just moved),
checked-count indicator and multi-pod log selection still work, tab gating,
manual/auto refresh still preserve cursor position.

After Track M2: confirm the new columns appear only in wide mode (never in
narrow), values match `kubectl get -o wide`/`kubectl describe` for a live
cluster, the Endpoint IPs lazy-fetch only fires once per context+namespace
(not on every `Ctrl+W` toggle or every refresh), and toggling wide mode on
svc doesn't block waiting for that fetch — every other new column appears
immediately, Endpoint IPs fills in a beat later.

After all tracks: a manual pass — `Ctrl+W` on each of the three tabs widens
columns to fit content with no clipping, is sticky across tab switches,
resets on resize; `Shift+Left/Right` pans one column at a time with the
checkbox column frozen on Pods; the `◂ col N/M ▸` indicator appears/hides
correctly; Detail and Log's `Shift+Left/Right` jump by half-viewport-width
with a percentage indicator; `w` toggles Log wrap (off by default), confirm
wrap and horizontal scroll are never both active, and that wrapped JSON-
highlighted and per-source-colored lines still render correctly across the
wrap.
