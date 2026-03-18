<div align="center">
  <h1>agent-ton-browser</h1>
  <p>CLI tool and AI agent skill for browsing .ton sites through Tonnet Browser.</p>
  <p>
    <a href="https://ton.org"><img src="https://img.shields.io/badge/TON-blockchain-0098EA" alt="TON"></a>
    <a href="https://tonnet.resistance.dog"><img src="https://img.shields.io/badge/Tonnet-Browser-1a1a2e" alt="Tonnet Browser"></a>
    <a href="LICENSE"><img src="https://img.shields.io/badge/license-MIT-green" alt="License"></a>
  </p>
</div>

---

`agent-ton-browser` is a CLI tool for automating [Tonnet Browser](https://github.com/TONresistor/Tonnet-Browser) over the Chrome DevTools Protocol. It can navigate .ton sites, read page content, interact with elements, and take screenshots. A persistent daemon process holds the CDP WebSocket connection between commands, so the browser stays alive across multiple invocations. Written from scratch in Go.

## Table of contents

- [Installation](#installation)
- [Quick start](#quick-start)
- [Architecture](#architecture)
- [Commands](#commands)
- [Global flags](#global-flags)
- [Platform notes](#platform-notes)
- [Development](#development)
- [License](#license)

## Installation

### As a skill (for AI agents)

```sh
npx skills add TONresistor/agent-ton-browser
```

This installs the `tonbrowser` skill into your project under `.agents/skills/tonbrowser/`. Works with Claude Code, Cursor, Codex, Copilot, Cline, Amp, OpenCode, Warp, and 30+ other agents.

### Binary

Download a pre-built binary from the [releases page](https://github.com/TONresistor/agent-ton-browser/releases), or build from source:

```sh
go install github.com/TONresistor/agent-tonbrowser/cmd/agent-tonbrowser@latest
```

**Prerequisite:** [Tonnet Browser](https://github.com/TONresistor/Tonnet-Browser/releases) must be installed separately, or use `agent-tonbrowser install` to download it.

## Quick start

### AI agent

After installing the skill, talk to your agent:

```
> go to piracy.ton and take a screenshot
> what's on skills.ton?
> navigate to how-to-resist.ton and read the full page
```

The agent handles everything: launching the browser, connecting, navigating, and reading content.

### CLI (scripting and advanced use)

```sh
agent-tonbrowser install                          # download Tonnet Browser
agent-tonbrowser launch                           # start with CDP enabled
agent-tonbrowser connect                          # connect the daemon
agent-tonbrowser fill "input" "piracy.ton"        # type in address bar
agent-tonbrowser press Enter                      # navigate
agent-tonbrowser --tab 1 screenshot page.png      # screenshot the .ton page
agent-tonbrowser --tab 1 eval 'document.title'    # read page title
agent-tonbrowser close                            # stop browser and daemon
```

## Architecture

Each CLI command is a short-lived process. When a command needs to interact with the browser, it first ensures a daemon is running, then sends an HTTP request over a Unix socket. The daemon holds a single CDP WebSocket connection to Tonnet Browser. Because the daemon is persistent, the browser stays open and tabs retain their state between commands.

```
CLI cmd -> Unix socket -> daemon -> CDP WebSocket -> Tonnet Browser
```

The daemon shuts itself down after 5 minutes of inactivity. It writes a PID file and socket path under the user config directory. The `daemon start` subcommand is called internally; you rarely need to invoke it directly.

## Commands

### Browser lifecycle

| Command | Description |
|---------|-------------|
| `install [--version VERSION]` | Download and install Tonnet Browser |
| `launch [--port PORT]` | Launch Tonnet Browser with CDP enabled |
| `close` | Close the running Tonnet Browser and stop the daemon |
| `status` | Show browser running state, PID, CDP port, and proxy status |

### Connection

| Command | Description |
|---------|-------------|
| `connect [PORT]` | Connect the daemon to a running browser on the given CDP port |

### Navigation

| Command | Description |
|---------|-------------|
| `goto URL` | Navigate to a .ton URL (aliases: `open`, `navigate`) |
| `back` | Go back in history |
| `forward` | Go forward in history |
| `reload` | Reload the current page |

### Observation

| Command | Description |
|---------|-------------|
| `snapshot [-i] [-c] [-d DEPTH]` | Get the accessibility tree. `-i` interactive only, `-c` compact, `-d` max depth |
| `screenshot [PATH] [-f]` | Take a screenshot. `-f` for full-page. Auto-generates filename if path is omitted |
| `get url\|title` | Print the current URL or page title |
| `tab [INDEX]` | List all tabs, or switch to tab by index |

### Interaction

| Command | Description |
|---------|-------------|
| `click SELECTOR` | Click an element (CSS selector, XPath, or `@eN` ref from snapshot) |
| `fill SELECTOR TEXT` | Clear an input and type new text |
| `type SELECTOR TEXT` | Type text into an element without clearing it first |
| `press KEY` | Press a key: `Enter`, `Tab`, `Escape`, or a single character |
| `scroll up\|down\|left\|right [--pixels N]` | Scroll the page (default: 300 px) |
| `wait TARGET` | Wait for a CSS selector, a duration in ms, `load`, or `network` |
| `eval SCRIPT` | Execute JavaScript and print the result |

### Daemon

| Command | Description |
|---------|-------------|
| `daemon start` | Start the daemon process (called automatically by other commands) |
| `daemon stop` | Stop the running daemon |
| `daemon status` | Show daemon PID, connection state, and current tab |

### Other

| Command | Description |
|---------|-------------|
| `version` | Print the binary version |

## Global flags

| Flag | Default | Env var | Description |
|------|---------|---------|-------------|
| `--cdp-port` | `9222` | `AGENT_TONBROWSER_CDP_PORT` | CDP port to connect to |
| `--tab` | `-1` (active tab) | `AGENT_TONBROWSER_TAB` | Target tab index (0 = main UI, 1 = first .ton tab) |
| `--json` | `false` | `AGENT_TONBROWSER_JSON` | Output results as JSON |
| `--debug` | `false` | `AGENT_TONBROWSER_DEBUG` | Print debug output |
| `--timeout` | `30000` | `AGENT_TONBROWSER_TIMEOUT` | Action timeout in milliseconds |

All flags can also be set via environment variables. This is useful for scripting or when integrating with an AI agent.

## Platform notes

**Linux:** Tonnet Browser is distributed as an AppImage. After `install`, the binary is made executable automatically. No additional steps needed.

**macOS:** The downloaded binary may be quarantined by Gatekeeper. Remove the attribute with:
```sh
xattr -cr /path/to/Tonnet-Browser.app
```

**Windows:** Downloaded binaries may be blocked by Windows SmartScreen. Unblock with:
```powershell
Unblock-File -Path .\agent-tonbrowser.exe
```

## Development

```sh
git clone https://github.com/TONresistor/agent-ton-browser.git
cd agent-ton-browser
make build
make test
make lint
```

The binary is written to `bin/agent-tonbrowser`. The `VERSION` variable can be set at build time:

```sh
make build VERSION=1.2.3
```

Releases are built with [GoReleaser](https://goreleaser.com) for Linux, macOS, and Windows (amd64/arm64, except Windows arm64).

## License

[MIT](LICENSE)
