# prompt-via-cli Design

## Problem

When kasmos launches an agent with a `QueuedPrompt`, there's a two-phase wait:

1. `TmuxSession.Start()` polls until the agent TUI is ready (e.g. "Ask anything" for opencode, trust screen for claude)
2. The app tick handler waits for `Ready`/`PromptDetected` status, then delivers the prompt via `tmux send-keys -l` + `TapEnter()`

This is slow and fragile — send-keys can race with TUI input handling, and the agent sits idle between boot and prompt delivery.

## Solution

Both opencode and claude support supplying an initial prompt at launch time:

| Program  | Syntax                       |
|----------|------------------------------|
| opencode | `opencode --prompt '...'`    |
| claude   | `claude '...'` (positional)  |

Bake the `QueuedPrompt` into the CLI command string at launch, so the agent starts working immediately on boot — no idle gap, no send-keys race.

## Design

### New TmuxSession field

```go
type TmuxSession struct {
    // ...existing fields...
    initialPrompt string // baked into CLI command at Start()
}

func (t *TmuxSession) SetInitialPrompt(prompt string) {
    t.initialPrompt = prompt
}
```

### Command construction in Start()

After the existing `--agent` and `--dangerously-skip-permissions` appends, add prompt injection:

```go
if t.initialPrompt != "" {
    escaped := shellEscapeSingleQuote(t.initialPrompt)
    switch {
    case isOpenCodeProgram(t.program):
        program = program + " --prompt " + escaped
    case isClaudeProgram(t.program):
        program = program + " " + escaped
    }
    // aider/gemini: no CLI prompt support, fall through to QueuedPrompt/send-keys
}
```

### Shell escaping

POSIX single-quote escaping: wrap in `'...'`, replace internal `'` with `'\''`. This handles newlines, `$`, backticks, double quotes — everything the markdown prompts contain.

```go
func shellEscapeSingleQuote(s string) string {
    return "'" + strings.ReplaceAll(s, "'", "'\\''") + "'"
}
```

### Caller integration

In `instance_lifecycle.go`, each `Start*()` method already calls `tmuxSession.SetAgentType()`. Add a parallel call:

```go
if i.QueuedPrompt != "" && programSupportsCliPrompt(i.Program) {
    tmuxSession.SetInitialPrompt(i.QueuedPrompt)
    i.QueuedPrompt = "" // prevent send-keys fallback
}
```

The `programSupportsCliPrompt()` helper returns true for opencode and claude, false for everything else. For unsupported programs, `QueuedPrompt` stays set and the existing send-keys path fires as before.

### Ready-wait behavior

The polling loop in `Start()` still runs — it handles the claude trust screen and ensures the TUI has booted before `Start()` returns. The key difference is that `QueuedPrompt` is already empty, so the app tick handler's send-keys delivery path is naturally skipped.

### What doesn't change

- `SendPrompt()` / send-keys — still used for interactive prompts to running instances
- `QueuedPrompt` field — still exists for aider/gemini and for prompts queued to already-running instances
- The ready-wait polling loop — still runs for all programs
