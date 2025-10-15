# KTails

A beautiful, terminal-based Kubernetes pod log viewer built with Go and [Bubble Tea](https://github.com/charmbracelet/bubbletea).

## Features

### Currently Implemented ✅

- **Multi-Context Support** - View pods from multiple Kubernetes contexts simultaneously
- **Interactive TUI** - Clean, keyboard-driven interface with focus management
- **Context Switching** - Easy selection and switching between Kubernetes contexts
- **Pod Listing** - View detailed pod information (name, namespace, status, restarts, age, image, container, node)
- **Multi-Selection** - Select multiple contexts to view their pods side-by-side
- **Beautiful Theming** - Catppuccin Mocha color scheme with focus-aware styling
- **Tab Navigation** - Cycle through contexts and pod panes with Tab/Shift+Tab
- **Help Mode** - Press `?` for keyboard shortcuts and help

### In Development 🚧

See [TODO.md](./todo.md) for planned features.

## Installation

### Prerequisites

- Go 1.21 or later
- kubectl configured with access to your Kubernetes clusters
- Valid kubeconfig file (default: `~/.kube/config`)

### Build from Source

```bash
# Clone the repository
git clone https://github.com/yourusername/ktails.git
cd ktails

# Build the binary
go build -o ktails cmd/test-client/main.go

# Run
./ktails
```

### Debug Mode

Enable debug logging to troubleshoot issues:

```bash
KTAILS_DEBUG=1 ./ktails
# Logs will be written to messages.log
```

## Usage

### Basic Usage

```bash
# Start ktails
./ktails

# It will automatically use your default kubeconfig
# Or specify a custom kubeconfig path (future feature)
```

### Keyboard Shortcuts

#### Global

- `q` or `Ctrl+C` - Quit the application
- `?` - Toggle help screen
- `Tab` - Cycle forward through panes
- `Shift+Tab` - Cycle backward through panes

#### Context Pane (Left)

- `↑/↓` - Navigate through contexts
- `Space` - Toggle selection (select multiple contexts)
- `Enter` - Load pods for selected context(s)
- `Esc` - Clear all selections
- `/` - Filter contexts (builtin list filter)

#### Pod Pane (Right)

- `↑/↓` - Navigate through pod list
- `j/k` - Vim-style navigation (alternative to arrows)

## Project Structure

```
ktails/
├── cmd/
│   └── test-client/
│       └── main.go          # Entry point
├── internal/
│   ├── config/
│   │   └── config.go        # Configuration management
│   ├── k8s/
│   │   └── client.go        # Kubernetes client wrapper
│   └── tui/
│       ├── tui.go           # Main TUI orchestration
│       ├── cmds/
│       │   └── cmds.go      # Bubble Tea commands
│       ├── models/
│       │   ├── contexts.go  # Context list model
│       │   ├── pods.go      # Pod table model
│       │   └── table.go     # Table column definitions
│       ├── msgs/
│       │   └── msgs.go      # Bubble Tea messages
│       ├── styles/
│       │   └── style.go     # Catppuccin theme and styles
│       └── views/
│           └── panes.go     # Layout management
└── README.md
```

## Architecture

KTails follows the [Elm Architecture](https://guide.elm-lang.org/architecture/) via Bubble Tea:

1. **Model** - Application state (contexts, pods, focus, dimensions)
2. **Update** - State transitions based on messages (keyboard, data loading)
3. **View** - Render the current state to the terminal

### Key Components

- **SimpleTui** - Root model managing mode, focus, and layout
- **ContextsInfo** - Context list with multi-select capability
- **Pods** - Pod table with filtering and navigation
- **K8s Client** - Wraps kubectl operations for context switching and pod listing

## Configuration

Configuration is managed via `internal/config/config.go`. Future features will include:

- Themes (dark/light)
- Auto-follow logs
- Max log lines
- Refresh intervals
- Recent pod history

## Development

### Running Tests

```bash
go test ./...
```

### Code Formatting

```bash
# Format all Go files
go fmt ./...

# Or use gofmt directly
gofmt -w .
```

### Adding a New Feature

1. Define the message type in `internal/tui/msgs/`
2. Add command in `internal/tui/cmds/`
3. Update model in appropriate `internal/tui/models/` file
4. Handle message in `internal/tui/tui.go` Update()
5. Update view rendering if needed

## Dependencies

- [Bubble Tea](https://github.com/charmbracelet/bubbletea) - TUI framework
- [Bubbles](https://github.com/charmbracelet/bubbles) - TUI components (table, list)
- [Lip Gloss](https://github.com/charmbracelet/lipgloss) - Styling and layout
- [client-go](https://github.com/kubernetes/client-go) - Kubernetes client library

## Roadmap

See [TODO.md](./todo.md) for detailed roadmap and planned features.

### Short Term (v0.1.0)

- [ ] Status bar with context/namespace info
- [ ] Manual refresh (r key)
- [ ] Error display panel
- [ ] Basic log viewing

### Medium Term (v0.2.0)

- [ ] Search mode (S key)
- [ ] Auto-refresh with interval
- [ ] Sort pods by name/status/restarts
- [ ] Log filtering and search

### Long Term (v1.0.0)

- [ ] Log export
- [ ] Multiple log panes
- [ ] Metrics integration
- [ ] Configuration file support

## Contributing

Contributions are welcome! Please feel free to submit a Pull Request.

### Guidelines

1. Follow Go best practices and idioms
2. Run `go fmt` before committing
3. Add tests for new features
4. Update documentation as needed
5. Keep commits atomic and well-described

## License

 GNU GENERAL PUBLIC LICENSE V3

## Acknowledgments

- Inspired by [k9s](https://k9scli.io/) and similar Kubernetes TUI tools
- Built with the amazing [Charm](https://charm.sh/) TUI libraries
- Uses the [Catppuccin](https://github.com/catppuccin/catppuccin) color scheme

## Screenshots

_Coming soon - Add screenshots of the TUI in action_

## Troubleshooting

### No contexts showing up

- Verify your kubeconfig is valid: `kubectl config view`
- Check context access: `kubectl config get-contexts`
- Enable debug mode: `KTAILS_DEBUG=1 ./ktails`

### Connection errors

- Ensure you can connect to your clusters: `kubectl cluster-info`
- Check your kubeconfig file permissions
- Verify network connectivity to Kubernetes API servers

### Application crashes

- Enable debug mode to see detailed logs
- Check `messages.log` for error details
- Report issues on GitHub with log output

## Support

- GitHub Issues: [Report bugs or request features](https://github.com/yourusername/ktails/issues)
- Discussions: [Ask questions and share ideas](https://github.com/yourusername/ktails/discussions)