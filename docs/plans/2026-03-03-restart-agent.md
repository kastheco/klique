# plan: restart agent

**date:** 2026-03-03  
**status:** done  
**branch:** restart-agent

## goal

Add a "restart" action to the instance context menu that kills the current tmux session and starts a fresh one, preserving the worktree, branch, and all instance identity. This allows an agent to be recycled without tearing down the full instance.

## wave 1

### task 1 — `Instance.Restart()` method

Add `Restart()` to `session/instance_lifecycle.go`:
- guard: return error if instance has not been started
- guard: return error if instance is paused (worktree is removed on disk; tmux start would fail)
- kill existing tmux session (best-effort; tolerate already-dead sessions)
- create a fresh `TmuxSession` with the same title/program/agent config
- determine working directory from `i.gitWorktree.GetWorktreePath()` or `i.Path`
- call `ts.Start(workDir)`
- reset ephemeral state: `Exited`, `PromptDetected`, `HasWorked`, `AwaitingWork`, `Notified`, `CachedContentSet`, `CachedContent`
- set status to `Running`

tests: `TestRestart_KillsTmuxAndRestartsSession`, `TestRestart_WorksWhenTmuxAlreadyDead`, `TestRestart_NotStarted_ReturnsError`, `TestRestart_PausedInstance_ReturnsError`

### task 2 — context menu + app action

Add "restart" to the instance context menu in `app/app_actions.go`:
- add `{Label: "restart", Action: "restart_instance"}` to the context menu items
- handle `"restart_instance"` in `handleAction`: call `selected.Restart()` in a goroutine, emit `auditlog.EventAgentRestarted`, call `m.saveAllInstances()` to persist state, return `instanceChangedMsg{}`
- add `EventAgentRestarted` to `config/auditlog/event.go`

tests: `TestHandleAction_RestartInstance`

## implementation notes

- `Restart()` paused guard is essential: `Pause()` removes the git worktree from disk, so starting tmux in a non-existent directory would silently produce a broken session.
- `saveAllInstances()` mirrors every other lifecycle action (`kill`, `pause`, `resume`) to ensure state survives a crash after restart.
