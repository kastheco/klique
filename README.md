# kas [![CI](https://github.com/kastheco/kasmos/actions/workflows/build.yml/badge.svg)](https://github.com/kastheco/kasmos/actions/workflows/build.yml) [![GitHub Release](https://img.shields.io/github/v/release/kastheco/kasmos)](https://github.com/kastheco/kasmos/releases/latest) [![License: AGPL v3](https://img.shields.io/badge/License-AGPL_v3-blue.svg)](https://www.gnu.org/licenses/agpl-3.0)

A TUI-based agent-driven IDE that manages multiple [Claude Code](https://github.com/anthropics/claude-code), [Codex](https://github.com/openai/codex), [Gemini](https://github.com/google-gemini/gemini-cli), [Amp](https://github.com/anthropics/amp) and other AI agents in isolated workspaces, so you can work on multiple tasks simultaneously.

![kas Screenshot](assets/screenshot.png)

### Highlights
- Complete tasks in the background (including yolo / auto-accept mode)
- Manage all your agent instances and tasks in one terminal window
- Review changes before applying them, checkout changes before pushing
- Each task gets its own isolated git workspace — no conflicts

### Installation

#### Homebrew (macOS / Linux)

```bash
brew install kastheco/tap/kasmos
```

#### Scoop (Windows)

```powershell
scoop bucket add kastheco https://github.com/kastheco/scoop-bucket
scoop install kasmos
```

#### Go Install

```bash
go install github.com/kastheco/kasmos@latest
```

#### Install Script

```bash
curl -fsSL https://raw.githubusercontent.com/kastheco/kasmos/main/install.sh | bash
```

This installs the `kasmos` binary in `~/.local/bin`.

To install with a custom name (e.g. `kq`):

```bash
curl -fsSL https://raw.githubusercontent.com/kastheco/kasmos/main/install.sh | bash -s -- --name kas
```

#### Download Binary

Pre-built binaries for macOS, Linux, and Windows are available on the [Releases page](https://github.com/kastheco/kasmos/releases/latest).

### Prerequisites

- [tmux](https://github.com/tmux/tmux/wiki/Installing)
- [gh](https://cli.github.com/)

### Usage

```
Usage:
  kas [flags]
  kas [command]

Available Commands:
  completion  Generate the autocompletion script for the specified shell
  debug       Print debug information like config paths
  help        Help about any command
  reset       Reset all stored instances
  version     Print the version number of kas

Flags:
  -y, --autoyes          [experimental] If enabled, all instances will automatically accept prompts
  -h, --help             help for kas
  -p, --program string   Program to run in new instances (e.g. 'aider --model ollama_chat/gemma3:1b')
```

Run the application with:

```bash
kas
```

NOTE: The default program is `claude` and we recommend using the latest version.

<br />

<b>Using kas with other AI assistants:</b>
- For [Codex](https://github.com/openai/codex): Set your API key with `export OPENAI_API_KEY=<your_key>`
- Launch with specific assistants:
   - Codex: `kas -p "codex"`
   - Aider: `kas -p "aider ..."`
   - Gemini: `kas -p "gemini"`
- Make this the default by modifying the config file (locate with `kas debug`)

<br />

#### Menu
The menu at the bottom of the screen shows available commands:

##### Instance/Session Management
- `n` - Create a new session
- `N` - Create a new session with a prompt
- `D` - Kill (delete) the selected session
- `↑/j`, `↓/k` - Navigate between sessions

##### Actions
- `↵/o` - Attach to the selected session to reprompt
- `ctrl-q` - Detach from session
- `s` - Commit and push branch to github
- `c` - Checkout. Commits changes and pauses the session
- `r` - Resume a paused session
- `?` - Show help menu

##### Navigation
- `tab` - Switch between preview tab and diff tab
- `q` - Quit the application
- `shift-↓/↑` - Scroll in diff view

### How It Works

1. **tmux** to create isolated terminal sessions for each agent
2. **git worktrees** to isolate codebases so each session works on its own branch
3. A TUI interface for easy navigation and management

### Attribution

kas is a fork of [claude-squad](https://github.com/smtg-ai/claude-squad) by smtg-ai.

### License

[AGPL-3.0](LICENSE.md)
