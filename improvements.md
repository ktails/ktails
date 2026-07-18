# KTails - Priority Improvements

Most of the items originally tracked here have since been implemented. Kept for history, with each
marked against the current codebase, plus a fresh list of what's actually still open.

## ✅ Previously Tracked, Now Done

1. **Duplicate AppState** — resolved; only `internal/state/state.go` exists now, `mainPage.go` uses
   it exclusively.
2. **Table re-render optimization** — `rowsSet`/`viewDirty` caching exists on `DeploymentPage`,
   `PodPage`, and `ServicePage`; `SetRows` no-ops when rows are unchanged (`rowsEqual`).
3. **Batched lock reads** — `AppState.Snapshot()` does this, with a cached-fast-path under an
   `RLock` and a recompute-path under a full `Lock`.
4. **Parallel context loading** — `tea.Batch(cmdSequence...)` in `mainPage.go`'s
   `ContextsStateMsg` handler.
5. **Loading state visibility** — status bar shows `⏳ N loading`; per-context loading also
   reflected in the context list itself.
6. **Empty state messages** — "No contexts selected / Press Tab to focus contexts / Space to
   select • Enter to load" shown per tab.
7. **Error recovery on success** — `AppState.SetDeployments` deletes the context's error on a
   successful load.
8. **Keyboard hints in status bar** — `Tab:focus [ ]:tabs ?:help q:quit` rendered faint on the
   right of the status bar.
9. **Remove unused code** — `cmd/test-client` no longer exists (only `cmd/page-client`); `PodPage`
   is fully integrated as the Pods tab.
10. **Focus indicator polish** — `LeftPane` (double border, focused) vs `LeftPaneBlur` (normal
    border, blurred); same pattern for `WindowStyle`/`WindowBlurStyle`.
11. **Tab switching gating** — the Deployments tab refuses to activate until contexts are loaded
    (`mainPage.go`, the `nextTab == "Deployments"` check in tab navigation).
12. **Update timing measurement** — `logSlowUpdate` logs any `Update()` call over 16ms.

## 🎯 Actually Open Now

1. **Color-code resource status** — Deployments/Pods/Services table rows aren't colored by
   phase/status (Running=green, Pending=yellow, Failed=red as originally suggested). The Detail
   pane already does this for Events (Warning=yellow, Normal=green) — extend the same treatment to
   the list tables.

2. **Manual + auto refresh** — resource lists load once on context selection and never update.
   Worth adding a refresh key and/or a polling interval, reusing the existing
   `LoadDeploymentInfoCmd`/`LoadPodInfoCmd`/`LoadServiceInfoCmd` commands.

3. **Extend tab gating to Pods/svc** — only the Deployments tab currently blocks switching-in
   before contexts are loaded; Pods and svc don't have the same guard (harmless today since they
   just show the empty-state message, but inconsistent).

4. **Per-context resource removal** — no way to drop just one selected context's rows without
   deselecting the context in the left pane (which reloads everything on reselection).

5. **Test coverage** — no `_test.go` files exist yet anywhere in `internal/`. The layout-height
   reconciliation logic in `mainPage.go` (`lineCount`/measure-and-correct in `View()`) and the
   `k8s.ResourceDetail` YAML/Events rendering are both good first candidates — they're pure
   functions once given synthetic input, no live cluster needed.

6. **Log viewing** — still the biggest gap relative to the original "log viewer" framing: Pods
   currently expose metadata/Status/Events/YAML via the Detail pane, not log streams.
