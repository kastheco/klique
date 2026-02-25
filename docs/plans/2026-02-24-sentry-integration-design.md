# Sentry Integration Design

**Goal:** Add Sentry crash reporting and error visibility to kasmos so errors and panics are automatically reported without requiring users to share log files.

**Decisions:**
- Both crash reporting (panic recovery) and error forwarding (log.ErrorLog → Sentry events)
- DSN hardcoded in source (ingest-only, not a secret)
- Opt-out via `telemetry_enabled` config field (default true)
- Writer adapter approach: existing 94+ log.ErrorLog call sites automatically report to Sentry with zero changes

## Architecture

A new `internal/sentry` package owns the SDK lifecycle and provides an `io.Writer` adapter that bridges the existing `log` package to Sentry. Errors become Sentry events, warnings become breadcrumbs (context leading up to the next error), and info logs become info-level breadcrumbs.

Panic recovery is layered at two points: `main()` and around `bubbletea.Program.Run()`. Panics anywhere in the process — including goroutines that crash the process — are captured before the program exits.

```
main.go                          internal/sentry/
  ├─ Init(version, enabled)  ──►  sentry.Init(ClientOptions{...})
  ├─ defer Flush()           ──►  sentry.Flush(2s)
  ├─ defer RecoverPanic()    ──►  sentry.Recover() + flush
  │
log/log.go                       internal/sentry/writer.go
  ErrorLog   = New(Writer{Error})  ──► Write() → sentry.CaptureMessage()
  WarningLog = New(Writer{Warning}) ──► Write() → sentry.AddBreadcrumb()
  InfoLog    = New(Writer{Info})    ──► Write() → sentry.AddBreadcrumb()
```

## Components

### 1. `internal/sentry/sentry.go` — SDK lifecycle

Hardcoded DSN constant. Three public functions:

- `Init(version string, telemetryEnabled bool)` — calls `sentry.Init` with DSN, release tag, stack traces enabled, sample rate 1.0. Sets global tags (os, arch, version, go_version). No-ops when `telemetryEnabled` is false.
- `Flush()` — calls `sentry.Flush(2 * time.Second)`.
- `RecoverPanic()` — calls `sentry.Recover()`, flushes, then re-panics so the user still sees crash output.

Init options:
- `Dsn`: hardcoded constant
- `Release`: `"kasmos@" + version`
- `AttachStacktrace`: true
- `SampleRate`: 1.0

### 2. `internal/sentry/writer.go` — log writer adapter

An `io.Writer` that wraps the original log file writer. Every `Write()` call:
1. Writes to the original file (preserves `/tmp/kas.log` behavior)
2. Based on configured level:
   - `Error` → `sentry.CaptureMessage(msg)` (creates a Sentry event)
   - `Warning` → `sentry.AddBreadcrumb(...)` (warning-level breadcrumb)
   - `Info` → `sentry.AddBreadcrumb(...)` (info-level breadcrumb)

The writer is a no-op passthrough when sentry is not initialized (telemetry disabled or empty DSN).

### 3. `log/log.go` — wire writers

`Initialize()` accepts an optional sentry-enabled flag. When enabled, wraps the file writer with the sentry writer for each log level before constructing the `log.Logger` instances. The change is ~6 lines in `Initialize()`.

### 4. Panic recovery points

1. **`main()`** — `defer sentrypkg.RecoverPanic()` after init. Catches anything that bubbles up.
2. **`app.Run()`** — wrap `p.Run()` with a deferred recover that captures to sentry and flushes before returning the error. This ensures panics inside the bubbletea event loop are captured before alt-screen teardown.
3. Existing `panic()` sites in `tmux_attach.go` are left as-is — they bubble up to #1 or #2.

### 5. Config opt-out

Mirrors the `NotificationsEnabled` pattern:

**JSON** (`config.json`):
```json
{ "telemetry_enabled": true }
```

**TOML** (`config.toml`):
```toml
[telemetry]
enabled = true
```

`*bool` field, defaults to `true` when nil. `IsTelemetryEnabled()` method on Config.

### 6. Context enrichment

Global tags set once at init:
- `os` (runtime.GOOS)
- `arch` (runtime.GOARCH)
- `version`
- `go_version` (runtime.Version())

Scope context set when the app starts:
- `program` (default agent program)
- `auto_yes` (autoyes mode)
- `active_repo` (basename only, not full path)

## Privacy

- No user paths beyond repo basename
- No file contents or agent output
- Opt-out via config
- DSN is ingest-only (cannot read events)
- `BeforeSend` strips any accidental home directory paths

## Files touched

| File | Change |
|------|--------|
| `internal/sentry/sentry.go` | New — SDK lifecycle |
| `internal/sentry/writer.go` | New — io.Writer adapter |
| `log/log.go` | Modify — wrap loggers with sentry writer |
| `main.go` | Modify — init sentry, defer flush + recover |
| `app/app.go` | Modify — recovery wrapper around p.Run() |
| `config/config.go` | Modify — TelemetryEnabled field + method |
| `config/toml.go` | Modify — telemetry TOML table |
