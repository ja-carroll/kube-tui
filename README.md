
<p align="center">
<pre align="center">
 ██╗  ██╗██╗   ██╗██████╗ ███████╗   ████████╗██╗   ██╗██╗
 ██║ ██╔╝██║   ██║██╔══██╗██╔════╝   ╚══██╔══╝██║   ██║██║
 █████╔╝ ██║   ██║██████╔╝█████╗  █████╗██║   ██║   ██║██║
 ██╔═██╗ ██║   ██║██╔══██╗██╔══╝  ╚════╝██║   ██║   ██║██║
 ██║  ██╗╚██████╔╝██████╔╝███████╗      ██║   ╚██████╔╝██║
 ╚═╝  ╚═╝ ╚═════╝ ╚═════╝ ╚══════╝      ╚═╝    ╚═════╝ ╚═╝
</pre>
</p>

<p align="center">
<strong>A terminal UI for Kubernetes — navigate your clusters without leaving the shell.</strong>
</p>

<p align="center">
<a href="#installation">Installation</a> · <a href="#features">Features</a> · <a href="#usage">Usage</a> · <a href="#keybindings">Keybindings</a> · <a href="#built-with">Built With</a>
</p>

---

## What is kube-tui?

kube-tui is a fast, keyboard-driven terminal interface for managing Kubernetes clusters. It gives you a live, navigable view of your namespaces, workloads, and resources — with actions like viewing logs, exec-ing into pods, scaling deployments, and editing YAML — all without typing `kubectl` commands.

It reads your `~/.kube/config` and lets you pick a context on launch, so switching between clusters is instant.

## Installation

### Homebrew (macOS / Linux)

```sh
# Coming soon
```

### Go

```sh
go install github.com/ja-carroll/kube-tui@latest
```

### From source

```sh
git clone https://github.com/ja-carroll/kube-tui.git
cd kube-tui
go build -o kube-tui .
./kube-tui
```

### Pre-built binaries

Grab a release from the [Releases](https://github.com/ja-carroll/kube-tui/releases) page. Binaries are available for **Linux**, **macOS**, and **Windows** on both `amd64` and `arm64`.

## Features

- **Landing page** — ASCII art logo with kubeconfig context selector and connection spinner
- **Live resource view** — auto-refreshes every 2 seconds with cursor preservation
- **Lazygit-style panels** — bordered panels with embedded titles and item counters
- **11 resource types** — Pods, Deployments, StatefulSets, DaemonSets, Services, Ingresses, ConfigMaps, Secrets, Jobs, CronJobs, PVCs
- **Pod metrics** — CPU and memory columns when metrics-server is available (degrades gracefully when it's not)
- **Cluster stats** — node count, pod count, CPU% and memory% in the header bar
- **Exec into pods** — drop into a shell inside any running pod
- **Log streaming** — real-time pod logs with scroll, search, and save-to-file
- **YAML editor** — view and edit resource YAML in your `$EDITOR`, applied on save
- **Scale deployments** — inline replica count dialog
- **Delete & restart** — delete resources or trigger rollout restarts
- **Search / filter** — `/` to filter resources by name, scoped locally or globally
- **Floating overlays** — action menus and dialogs composited over the main UI
- **Cross-platform** — Linux, macOS, Windows

## Usage

```sh
kube-tui
```

On launch you'll see the context selector. Pick a kubeconfig context and hit `enter` — kube-tui connects and drops you into the main interface.

The UI is split into three panels:

| Panel | Contents |
|---|---|
| **Namespaces** (top-left) | Your cluster namespaces — select one to filter resources |
| **Resources** (bottom-left) | Resource type picker — Pods, Deployments, Services, etc. |
| **Main** (right) | Resource list with details for the selected item |

Press `enter` on any resource to open the action menu.

## Keybindings

| Key | Action |
|---|---|
| `tab` | Cycle between panels |
| `j` / `k` | Navigate up / down |
| `enter` | Open action menu for selected resource |
| `/` | Search / filter (press `tab` in search to toggle local/global) |
| `esc` | Clear filter / close overlay |
| `q` | Quit |

**Action menu** (when a resource is selected):

| Key | Action |
|---|---|
| `l` | View logs (pods / jobs) |
| `e` | Exec into pod |
| `y` | View / edit YAML |
| `s` | Scale replicas (deployments / statefulsets) |
| `r` | Restart (delete pod / rollout restart) |
| `d` | Delete resource |

**Log viewer:**

| Key | Action |
|---|---|
| `j` / `k` | Scroll down / up |
| `g` / `G` | Jump to top / bottom |
| `s` | Save logs to file |
| `esc` | Back to main view |

## Built With

kube-tui is built with Go and the wonderful [Charm](https://charm.sh) ecosystem:

- [Bubble Tea](https://github.com/charmbracelet/bubbletea) — the TUI framework (Elm-inspired, model-update-view)
- [Lip Gloss](https://github.com/charmbracelet/lipgloss) — styling, layout, and compositing
- [Bubbles](https://github.com/charmbracelet/bubbles) — pre-built TUI components (list, spinner, text input)

If you're building terminal apps in Go, Charm's tools are outstanding. Check them out at [charm.sh](https://charm.sh).

## License

[MIT](LICENSE)
