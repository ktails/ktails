# Quick wins — execution plan (phased, parallelizable)

Status: ready to execute. Every item below has its full spec locked in `todo.md`
(via a grilling session, 2026-07-19) — this file only sequences *how* to build
them (phases, tracks, file ownership), not *what* to build. Read the linked
`todo.md` entry before starting a track; this file won't repeat that detail.

## How to use this plan

Each **track** is an independent unit of work claimable by one agent. Tracks
within the same phase have no functional dependency on each other and can run
in parallel — ideally each in its own git worktree (`isolation: "worktree"` if
launching via the Agent tool), merged back independently. Where two tracks
touch the same file in different, non-overlapping functions, that's called out
explicitly below; expect a clean merge but rebase if not. Phase 2 tracks must
not start until their named Phase 1 dependency has merged.

---

## Phase 1 — four parallel tracks, no cross-track dependencies

### Track A — mainPage.go bundle: Tab Gating → Checked-Count Indicator → Manual Refresh

Bundled into one track (not three) because all three touch
`internal/pages/mainPage.go` in different regions — bundling avoids two agents
editing the same file concurrently. Build in this order within the track:

1. **Tab Gating Consistency** (`todo.md`, v0.1.0) — extends the existing
   Deployments-only guard (tab-navigation switch, `nextTab == "Deployments"`
   check) to Pods/svc. Smallest, no interaction with the other two.
2. **Checked-Count Indicator** (`todo.md`, v0.1.0) — new status bar hint in
   `renderStatusBar`, Pods-tab only.
3. **Manual Refresh** (`todo.md`, v0.1.0) — new `r` keypress case, reuses
   `LoadDeploymentInfoCmd`/`LoadPodInfoCmd`/`LoadServiceInfoCmd`. Build last —
   Phase 2's Track E depends on this landing.

Files: `internal/pages/mainPage.go` only.

### Track B — Status Colors: Pods

`todo.md`, v0.1.0 "Status Colors" (Pods half). `PodInfo.Status` already carries
the phase string (`internal/k8s/client.go`), so this is render-layer only — no
`cmds.go`/`k8s` changes needed.

Files: `internal/tui/models/pods.go`, `internal/tui/models/table.go`
(`podTableColumns`).

Implementation note: `bubbles/table` applies one `Styles.Cell` uniformly to
every cell — there's no per-cell style hook. Coloring just the Status column
will mean embedding a lipgloss-rendered (ANSI) string directly into that
cell's content (same trick already used for the `✓` checkbox glyph), not
reaching for `table.Styles`. Verify the ANSI doesn't get clobbered when
`bubbles/table` composes/pads the row.

### Track C — Status Colors: Deployments

`todo.md`, v0.1.0 "Status Colors" (Deployments half). Needs a new
`DesiredReplicas` field that doesn't exist on `DeploymentInfo` today.

Files: `internal/k8s/deployments.go` (add `DesiredReplicas int32`, populated
from `deployment.Spec.Replicas` — a `*int32`, nil means the API default of 1),
`internal/tui/cmds/cmds.go` (`LoadDeploymentInfoCmd`'s row building — change
the replica cell from a bare ready-count to a `ready/desired` string),
`internal/tui/models/table.go` (`deploymentTableColumns`) + `internal/tui/models/deployment.go`
(coloring, same ANSI-embedding approach as Track B).

Shares `table.go` with Track B, but a different function
(`deploymentTableColumns` vs `podTableColumns`) — expected to merge cleanly.

### Track D — Log Highlighting (JSON)

`todo.md`, v0.3.0 "Log Highlighting (JSON)". Fully self-contained: new
functions only, no interaction with the existing merge/isolation logic in
`podlogs.go`.

Files: `internal/tui/models/podlogs.go` only.

---

## Phase 2 — depends on Track A landing

### Track E — Auto-Refresh

`todo.md`, v0.1.0 "Auto-Refresh". Explicitly depends on Manual Refresh's
per-tab refresh path (Track A, step 3) already existing — the tick reuses that
same path. Do not start until Track A has merged.

Files: `internal/pages/mainPage.go` (new ticking `tea.Cmd`, `Init()`/`Update()`
wiring — likely a new `msgs.RefreshTickMsg`), reads the existing (currently
unused) `config.Preferences.RefreshInterval`.

---

## Explicitly not scheduled

- **Log Stream Auto-Retry** — deliberately deferred (see `todo.md`'s Backlog
  entry for the full rationale). Not part of this plan; don't build it as a
  side effect of any track above.

## Verification

After each track lands: `make build && make test && make lint && make fmt`
(per `AGENTS.md`/`Makefile`). After all tracks: a manual pass covering each
track's own scenario — checking rows shows the indicator and clears with
`Ctrl+X`, switching tabs before contexts load is blocked on all three tabs,
`r` refreshes only the active tab and preserves cursor position, Pods/Deployments
show correct colors across a few different phases/ready-states, and a
`timestamp level {json}`-shaped log line gets its JSON portion colored. Same
spirit as the verification checklist in `design-log-viewer.md`.
