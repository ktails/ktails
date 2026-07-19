# Horizontal scroll + log wrap — execution plan (phased, parallelizable)

Status: ready to execute. Full spec locked in `todo.md` (via a grilling
session, 2026-07-19) under "Horizontal Scrolling" and "Soft Wrap Toggle" —
this file only sequences *how* to build it (phases, tracks, file ownership),
not *what* to build. Read both `todo.md` entries before starting a track;
this file won't repeat that detail.

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

### Track M — bubble-table migration + table wide-mode/scroll/freeze

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

### Track G — Log pane wrap toggle + horizontal scroll

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

After all tracks: a manual pass — `Ctrl+W` on each of the three tabs widens
columns to fit content with no clipping, is sticky across tab switches,
resets on resize; `Shift+Left/Right` pans one column at a time with the
checkbox column frozen on Pods; the `◂ col N/M ▸` indicator appears/hides
correctly; Detail and Log's `Shift+Left/Right` jump by half-viewport-width
with a percentage indicator; `w` toggles Log wrap (off by default), confirm
wrap and horizontal scroll are never both active, and that wrapped JSON-
highlighted and per-source-colored lines still render correctly across the
wrap.
