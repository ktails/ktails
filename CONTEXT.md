# ktails — Glossary

## Resource Kind
The identity of one watched resource type — Deployments, Pods, svc, ConfigMaps, Secrets, or Nodes — as a value (`msgs.ResourceKind`), not a string. It names a tab in the Tab Area, keys that tab's Resource Table, and tags every watch message. Anywhere the code needs "which kind," it dispatches on a Resource Kind. Nodes are the one cluster-scoped kind — no namespace, watched once per context regardless of which namespace is selected.

## Resource Table
The single table module (`models.ResourceTable`) behind every Tab Area tab. Cursor/window management, the "/" filter, wide mode, column scroll, and view caching are its implementation; what differs per Resource Kind (columns, row rendering, the Pods checkbox) lives in its per-kind spec. There is one instance per tab, but one implementation.

## Secret Redaction
Secrets never carry their Data/StringData values past the Kubernetes API response: `watch.secretRow` only ever emits a key count, and `k8s.GetSecretDetail` overwrites every value with `<redacted>` before rendering YAML for the Detail Pane. A terminal's scrollback is not a safe place for cluster secrets, even base64-encoded ones — this is a load-bearing security decision, not an oversight, and any future Secret-related feature (e.g. viewing a specific key) must decide deliberately whether to punch a hole in it.

## Watch Supervisor
The module (`watch.Supervisor`) owning the full watch lifecycle: opening Watch() streams per selected context, applying events to local caches, reconnect backoff, generation-based staleness guards, and the svc Endpoint-IPs overlay. Its seam to the cluster is the `watch.Cluster` interface (`*k8s.Client` in production, a fake event stream in tests). Rows live only in its caches — `MainPage` and `state.AppState` never store them.

## Layout Solver
The single owner of the terminal's width/height budget (`views.Solve`): a pure function from terminal size to every pane's exact box and content rectangle. Panes render into pre-fitted blocks (`views.FitBlock` / `views.TitledBox`), so the two boxes' borders align by construction — there is no post-render height reconciliation, and no layout constant lives outside `views`.

## Pane Focus
Which of the two main areas — **Context List** (left) or **Tab Area** (right) — currently receives keyboard input. Switched with `Tab` / `Shift+Tab`.

## Context List
The left pane, divided into three always-visible, stacked sections — **Contexts** (the interactive selection list; items toggled with `Space`, confirmed with `Enter`), **Namespaces**, and **Clusters** — sized by `pages.splitLeftSections` (Contexts gets the most room). Only Contexts is interactive and populated; Namespaces and Clusters are currently just header-over-blank-space placeholders, deliberately not driven by the selected contexts — no data pipeline to remove/rewire if they grow real content later.

## Tab Area
The right pane containing named tabs (Deployments, Pods, svc). Active tab is switched with `[` / `]` or `←` / `→`. Gated: a tab that needs loaded data (e.g. Deployments) refuses to become active until contexts are selected and loaded.

## Detail Pane
A cross-cutting bottom split-pane showing a single resource's Status conditions, recent Events, and full YAML. Opened by pressing `Enter` on a row in *any* of the three tabs — it is not a fourth peer tab, it just splits whichever tab's content area is currently active in two, and stays open when you switch tabs. Backed by `k8s.ResourceDetail` (kind-agnostic: Deployment, Pod, or Service) and rendered by `models.ResourceDetailPage`.

## Detail Focus
Within Tab Area focus, whether keyboard input goes to the row list (`ListFocus`) or to the Detail Pane's scrollable viewport (`DetailFocus`). `Enter` on a row opens the pane and grants it focus; `Esc` first returns focus to the list, a second `Esc` closes the pane; `Ctrl+R` jumps back into an already-open pane without touching focus semantics of a fresh fetch (i.e. no re-fetch).

## Log Pane
A cross-cutting bottom split-pane showing live-tailing logs, merged from one or more pod/container sources. Reachable with `l` on the Pods tab: it opens (or reconciles) a stream for every container of every *checked* row (toggled with `Space`), falling back to the row under the cursor if nothing's checked. Mutually exclusive with the Detail Pane — opening one closes the other. Backed by `models.LogPage`, which renders either the full chronological merge of all open sources (each line prefixed with a colored `pod/container` tag) or a single isolated source, toggled with `c`.

## Log Focus
Within Tab Area focus, whether keyboard input goes to the Pods row list (`ListFocus`) or to the Log Pane's scrollable viewport (`LogFocus`). `l` opens/reconciles the pane and grants it focus; while focused, `c` isolates the view to one source (cycling through sources, then back to the full merge) without affecting any source's underlying stream — every source keeps streaming into its own buffer regardless of what's currently isolated. `Esc` first returns focus to the list, a second `Esc` closes the pane (stopping every open source's stream); `Ctrl+R` is not currently wired to the Log Pane (unlike the Detail Pane).

## Help Overlay
A modal display of all keybindings, toggled by `?`. While open it blocks all other keys; dismissed with `Esc` or `?`.

## Error Banner
A dismissible notification rendered in the Tab Area when a context fails to load. Dismissed with `Esc` (only after the Help Overlay is closed if both are visible).

## Too-Small Guard
Below `views.MinContentWidth` x `views.MinHeight` (80x24), `MainPage.View()` renders a "resize your terminal" message instead of attempting the normal layout.
