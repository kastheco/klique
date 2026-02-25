# kasmos [![CI](https://github.com/kastheco/kasmos/actions/workflows/build.yml/badge.svg)](https://github.com/kastheco/kasmos/actions/workflows/build.yml) [![GitHub Release](https://img.shields.io/github/v/release/kastheco/kasmos)](https://github.com/kastheco/kasmos/releases/latest) [![License: AGPL v3](https://img.shields.io/badge/License-AGPL_v3-blue.svg)](https://www.gnu.org/licenses/agpl-3.0)

> harness & model-agnostic ai orchestration tool with automated wave-based implementation — powered by superpowers, tmux, and git worktrees.

![kasmos screenshot](assets/screenshot.gif)

---

## what it does

kasmos turns your terminal into a multi-agent control center. each task gets its own isolated git worktree and a fresh tmux session at every lifecycle stage: a planner agent writes the implementation plan, coder agents execute it wave by wave, and a reviewer agent validates the result — all managed from a single tui.

- **plan-centric workflow** — create plans with name + description, organize into topics, track status through the full lifecycle (planning → implementing → reviewing → done)
- **wave orchestration** — plans are split into waves; kasmos automatically runs parallel agents per wave, advancing only when all tasks pass
- **isolated workspaces** — every plan gets a dedicated git worktree and tmux session; no branch conflicts, no shared state
- **live agent preview** — the center pane embeds a live terminal so you can watch agents work without leaving kasmos
- **diff + git views** — review changes and git history before merging, right inside the TUI
- **auto-accept mode** — run agents unattended with a background daemon handling permission prompts

---

## installation

#### homebrew *(coming soon)*

```bash
brew install kastheco/tap/kasmos
```

#### go install

```bash
go install github.com/kastheco/kasmos@latest
```

#### install script

```bash
curl -fsSL https://raw.githubusercontent.com/kastheco/kasmos/main/install.sh | bash
```

installs the `kasmos` binary to `~/.local/bin`. to install with a custom name:

```bash
curl -fsSL https://raw.githubusercontent.com/kastheco/kasmos/main/install.sh | bash -s -- --name kq
```

#### download binary

pre-built binaries for macOS, linux, and windows are on the [releases page](https://github.com/kastheco/kasmos/releases/latest).

---

## prerequisites

- [tmux](https://github.com/tmux/tmux/wiki/Installing)
- [gh](https://cli.github.com/)
- at least one supported AI CLI: **[opencode](https://github.com/sst/opencode)**, [claude code](https://github.com/anthropics/claude-code), [codex](https://github.com/openai/codex), [gemini CLI](https://github.com/google-gemini/gemini-cli), [amp](https://ampcode.com), or [aider](https://aider.chat)

---

## getting started

run from within a git repository:

```bash
kasmos
```

on first run, use the setup wizard to configure your agent harnesses and install skills:

```bash
kasmos init
```

the wizard detects installed agent CLIs, lets you assign roles (planner / coder / reviewer), and scaffolds the project files kasmos needs.

---

## usage

```
usage:
  kasmos [flags]
  kasmos [command]

available commands:
  init        configure agent harnesses, install superpowers, and scaffold project files
  reset       reset all stored instances and clean up tmux sessions and worktrees
  debug       print debug information like config paths
  version     print the version number

flags:
  -p, --program string   agent to use for new instances (e.g. 'opencode', 'codex', 'aider --model ...')
  -y, --autoyes          automatically accept all agent prompts (experimental)
  -h, --help             help for kasmos
```

### keybindings

| key | action |
|-----|--------|
| `n` | new plan |
| `/` | search plans |
| `space` | open context menu |
| `tab` | cycle focus (sidebar → list → preview) |
| `↑ / ↓` or `j / k` | navigate |
| `i` | interactive mode (focus agent pane) |
| `ctrl-q` | exit interactive mode |
| `?` | help |
| `q` | quit |

---

## how it works

1. **plans** live in `docs/plans/` as markdown files — kasmos creates and tracks them in `plan-state.json`
2. **topics** group related plans and act as collision domains (only one plan per topic can implement at a time)
3. **waves** divide implementation into phases — kasmos parses `## Wave N` headers and runs each wave's tasks in parallel
4. **agents** are spawned in isolated tmux sessions with dedicated git worktrees; the TUI shows live output in the preview pane
5. **review** is automated — a reviewer agent checks the implementation, and kasmos prompts for merge/PR approval before closing the plan

---

## configuration

config lives at `~/.config/kasmos/config.toml`. locate it with:

```bash
kasmos debug
```

key settings:

```toml
default_program = "opencode"   # default agent CLI
auto_yes = false               # auto-accept mode
```

---

## attribution

kasmos is a fork of [claude-squad](https://github.com/smtg-ai/claude-squad) by smtg-ai.

---

## license

[AGPL-3.0](LICENSE.md)
