# headless coder execution models

kasmos agent profiles can now set `execution_mode` per role.

## modes

- `tmux` (default): launches the agent in a background tmux session. this stays attachable from the tui and fits interactive debugging.
- `headless`: marks the agent for non-interactive execution. this is intended for automated coder flows where deterministic completion matters more than live attachment.

## recommendation

- keep planners and reviewers on `tmux` when you want to inspect or steer them live.
- use `headless` for coder-heavy wave execution when the agent should finish work and hand control back to kasmos cleanly.

## example

```toml
[agents.coder]
enabled = true
program = "opencode"
execution_mode = "headless"
```
