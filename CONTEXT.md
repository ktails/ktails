# ktails — Glossary

## Pane Focus
Which of the two main areas — **Context List** (left) or **Tab Area** (right) — currently receives keyboard input. Switched with `Tab` / `Shift+Tab`.

## Context List
The left pane showing available Kubernetes contexts. Items are toggled with `Space` and confirmed with `Enter`.

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
