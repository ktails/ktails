Got it. Here are KubeNodeUsage-style improvements you can add to your TUI:
### todos

- Persistent header/status
  - Show current context/namespace per pane, metric-like info (follow on/off), last refresh time, match count (when searching).
- Search mode (S)
  - Press S to enter search; live filter pod list and optionally highlight log lines; ESC to exit. Keep headers visible.
- Horizontal navigation
  - Left/Right to horizontally scroll pod lists; for logs add SoftWrap toggle (w) and/or simple xOffset slicing for long lines.
- Sorting and filtering for pods
  - Cycle sort (o) by name/status/restarts; quick filter by substring in search mode.
- Refresh/tick
  - r to refresh pod list; optional auto-refresh interval with a subtle spinner in the status bar.
- Better errors
  - Non-blocking error panel area instead of replacing the view; show actionable hints (e.g., metrics, permissions).
- Theming and color
  - Consistent status colors (Running/Pending/Failed), selection highlighting, focused pane accent.
