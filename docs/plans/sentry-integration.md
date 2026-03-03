# Sentry Integration Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Add Sentry crash reporting and error forwarding so panics and log.ErrorLog calls are automatically reported to Sentry, with opt-out via config.

**Architecture:** A thin `internal/sentry` package wraps the SDK lifecycle and provides an `io.Writer` adapter that bridges existing loggers to Sentry events/breadcrumbs. Panic recovery at `main()` and `app.Run()`. Config opt-out mirrors `NotificationsEnabled` pattern.

**Tech Stack:** sentry-go v0.43.0 (already in go.mod as indirect), stdlib log, bubbletea

**Design doc:** `docs/plans/2026-02-24-sentry-integration-design.md`

---

### Task 1: Config — add TelemetryEnabled field

**Files:**
- Modify: `config/config.go`
- Modify: `config/toml.go`
- Modify: `config/config_test.go`

**Step 1: Write the failing test**

Add to `config/config_test.go`:

```go
func TestIsTelemetryEnabled(t *testing.T) {
	tests := []struct {
		name     string
		field    *bool
		expected bool
	}{
		{"nil defaults to true", nil, true},
		{"explicit true", boolPtr(true), true},
		{"explicit false", boolPtr(false), false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &Config{TelemetryEnabled: tt.field}
			assert.Equal(t, tt.expected, cfg.IsTelemetryEnabled())
		})
	}
}

func boolPtr(b bool) *bool { return &b }
```

**Step 2: Run test to verify it fails**

Run: `go test ./config/ -run TestIsTelemetryEnabled -v`
Expected: FAIL — `IsTelemetryEnabled` not defined

**Step 3: Add TelemetryEnabled to Config struct and method**

In `config/config.go`, add to the `Config` struct:

```go
// TelemetryEnabled controls whether crash reporting via Sentry is active.
// Defaults to true when not set.
TelemetryEnabled *bool `json:"telemetry_enabled,omitempty"`
```

Add method:

```go
// IsTelemetryEnabled returns whether Sentry telemetry is enabled.
// Defaults to true when the field is not set.
func (c *Config) IsTelemetryEnabled() bool {
	if c.TelemetryEnabled == nil {
		return true
	}
	return *c.TelemetryEnabled
}
```

**Step 4: Add TOML telemetry table**

In `config/toml.go`, add to `TOMLConfig`:

```go
type TOMLTelemetryConfig struct {
	Enabled *bool `toml:"enabled,omitempty"`
}
```

Add field to `TOMLConfig`:

```go
Telemetry TOMLTelemetryConfig `toml:"telemetry"`
```

In `LoadTOMLConfigFrom`, add to the result mapping:

```go
if tc.Telemetry.Enabled != nil {
	result.TelemetryEnabled = tc.Telemetry.Enabled
}
```

Add `TelemetryEnabled *bool` to `TOMLConfigResult`.

In `config/config.go` `LoadConfig()`, apply the TOML overlay:

```go
if tomlResult.TelemetryEnabled != nil {
	config.TelemetryEnabled = tomlResult.TelemetryEnabled
}
```

**Step 5: Run test to verify it passes**

Run: `go test ./config/ -run TestIsTelemetryEnabled -v`
Expected: PASS

**Step 6: Commit**

```bash
git add config/config.go config/toml.go config/config_test.go
git commit -m "feat(config): add telemetry_enabled opt-out field for sentry"
```

---

### Task 2: internal/sentry package — SDK lifecycle

**Files:**
- Create: `internal/sentry/sentry.go`
- Create: `internal/sentry/sentry_test.go`

**Step 1: Write the failing test**

Create `internal/sentry/sentry_test.go`:

```go
package sentry

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestInit_Disabled(t *testing.T) {
	// When telemetry is disabled, Init should succeed but not initialize sentry
	err := Init("1.0.0", false)
	assert.NoError(t, err)
	// Flush and RecoverPanic should be safe no-ops
	Flush()
	// No panic expected
}

func TestInit_EmptyDSN(t *testing.T) {
	// With empty DSN, sentry silently no-ops
	origDSN := dsn
	dsn = ""
	defer func() { dsn = origDSN }()

	err := Init("1.0.0", true)
	assert.NoError(t, err)
	Flush()
}

func TestIsEnabled(t *testing.T) {
	enabled = false
	assert.False(t, IsEnabled())
	enabled = true
	assert.True(t, IsEnabled())
	enabled = false // reset
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/sentry/ -v`
Expected: FAIL — package doesn't exist

**Step 3: Implement sentry.go**

Create `internal/sentry/sentry.go`:

```go
package sentry

import (
	"runtime"
	"time"

	gosentry "github.com/getsentry/sentry-go"
)

const sentryDSN = "https://69b5854d8f6099c818b46b7ebaf45acb@o4510947765190656.ingest.us.sentry.io/4510947771088896"

// dsn is a package-level var so tests can override it.
var dsn = sentryDSN

// enabled tracks whether sentry was successfully initialized.
var enabled bool

// Init initializes the Sentry SDK. When telemetryEnabled is false or dsn is
// empty, it no-ops silently — all other functions in this package become safe
// no-ops.
func Init(version string, telemetryEnabled bool) error {
	if !telemetryEnabled || dsn == "" {
		enabled = false
		return nil
	}

	err := gosentry.Init(gosentry.ClientOptions{
		Dsn:              dsn,
		Release:          "kasmos@" + version,
		AttachStacktrace: true,
		SampleRate:       1.0,
	})
	if err != nil {
		return err
	}

	gosentry.ConfigureScope(func(scope *gosentry.Scope) {
		scope.SetTag("os", runtime.GOOS)
		scope.SetTag("arch", runtime.GOARCH)
		scope.SetTag("go_version", runtime.Version())
		scope.SetTag("version", version)
	})

	enabled = true
	return nil
}

// IsEnabled returns whether sentry is active.
func IsEnabled() bool {
	return enabled
}

// Flush waits up to 2 seconds for buffered events to be sent.
func Flush() {
	if !enabled {
		return
	}
	gosentry.Flush(2 * time.Second)
}

// RecoverPanic captures a panic to Sentry, flushes, then re-panics.
// Usage: defer sentry.RecoverPanic()
func RecoverPanic() {
	if !enabled {
		return
	}
	if err := recover(); err != nil {
		gosentry.CurrentHub().Recover(err)
		gosentry.Flush(2 * time.Second)
		panic(err)
	}
}

// SetContext adds app-level context to the current scope.
func SetContext(program string, autoYes bool, repoBasename string) {
	if !enabled {
		return
	}
	gosentry.ConfigureScope(func(scope *gosentry.Scope) {
		scope.SetTag("program", program)
		scope.SetTag("auto_yes", boolStr(autoYes))
		scope.SetContext("app", map[string]interface{}{
			"program":     program,
			"auto_yes":    autoYes,
			"active_repo": repoBasename,
		})
	})
}

func boolStr(b bool) string {
	if b {
		return "true"
	}
	return "false"
}
```

**Step 4: Run test to verify it passes**

Run: `go test ./internal/sentry/ -v`
Expected: PASS

**Step 5: Commit**

```bash
git add internal/sentry/
git commit -m "feat(sentry): add SDK lifecycle package with init, flush, panic recovery"
```

---

### Task 3: Sentry writer adapter

**Files:**
- Create: `internal/sentry/writer.go`
- Create: `internal/sentry/writer_test.go`

**Step 1: Write the failing test**

Create `internal/sentry/writer_test.go`:

```go
package sentry

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestWriter_PassthroughToInner(t *testing.T) {
	var buf bytes.Buffer
	w := NewWriter(&buf, LevelError)

	msg := []byte("test error message\n")
	n, err := w.Write(msg)

	assert.NoError(t, err)
	assert.Equal(t, len(msg), n)
	assert.Equal(t, string(msg), buf.String())
}

func TestWriter_DisabledPassthrough(t *testing.T) {
	// When sentry is not enabled, writer should still pass through to inner
	enabled = false
	var buf bytes.Buffer
	w := NewWriter(&buf, LevelError)

	msg := []byte("test message\n")
	n, err := w.Write(msg)

	assert.NoError(t, err)
	assert.Equal(t, len(msg), n)
	assert.Equal(t, string(msg), buf.String())
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/sentry/ -run TestWriter -v`
Expected: FAIL — `NewWriter` not defined

**Step 3: Implement writer.go**

Create `internal/sentry/writer.go`:

```go
package sentry

import (
	"io"
	"strings"

	gosentry "github.com/getsentry/sentry-go"
)

// Level represents the severity level for the sentry writer.
type Level int

const (
	LevelInfo Level = iota
	LevelWarning
	LevelError
)

// Writer wraps an io.Writer and forwards log messages to Sentry.
// Errors become Sentry events; warnings and info become breadcrumbs.
type Writer struct {
	inner io.Writer
	level Level
}

// NewWriter creates a Writer that tees to inner and forwards to Sentry.
func NewWriter(inner io.Writer, level Level) *Writer {
	return &Writer{inner: inner, level: level}
}

func (w *Writer) Write(p []byte) (int, error) {
	// Always write to the original destination first.
	n, err := w.inner.Write(p)

	if !enabled {
		return n, err
	}

	msg := strings.TrimSpace(string(p))
	if msg == "" {
		return n, err
	}

	switch w.level {
	case LevelError:
		gosentry.CaptureMessage(msg)
	case LevelWarning:
		gosentry.AddBreadcrumb(&gosentry.Breadcrumb{
			Level:    gosentry.LevelWarning,
			Category: "log",
			Message:  msg,
		})
	case LevelInfo:
		gosentry.AddBreadcrumb(&gosentry.Breadcrumb{
			Level:    gosentry.LevelInfo,
			Category: "log",
			Message:  msg,
		})
	}

	return n, err
}
```

**Step 4: Run test to verify it passes**

Run: `go test ./internal/sentry/ -run TestWriter -v`
Expected: PASS

**Step 5: Commit**

```bash
git add internal/sentry/writer.go internal/sentry/writer_test.go
git commit -m "feat(sentry): add io.Writer adapter for log-to-sentry bridging"
```

---

### Task 4: Wire sentry writer into log package

**Files:**
- Modify: `log/log.go`

**Step 1: Modify log.Initialize to accept sentry flag and wrap writers**

Update `log/log.go`:

```go
package log

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"time"

	sentrypkg "github.com/kastheco/kasmos/internal/sentry"
)

// Initialize sets up logging. When telemetryEnabled is true and sentry is
// active, log writers are wrapped to forward errors/warnings to Sentry.
func Initialize(daemon bool, telemetryEnabled bool) {
	f, err := os.OpenFile(logFileName, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0666)
	if err != nil {
		panic(fmt.Sprintf("could not open log file: %s", err))
	}

	log.SetFlags(log.LstdFlags | log.Lshortfile)

	fmtS := "%s"
	if daemon {
		fmtS = "[DAEMON] %s"
	}

	var infoW, warnW, errW io.Writer = f, f, f
	if telemetryEnabled && sentrypkg.IsEnabled() {
		infoW = sentrypkg.NewWriter(f, sentrypkg.LevelInfo)
		warnW = sentrypkg.NewWriter(f, sentrypkg.LevelWarning)
		errW = sentrypkg.NewWriter(f, sentrypkg.LevelError)
	}

	InfoLog = log.New(infoW, fmt.Sprintf(fmtS, "INFO:"), log.Ldate|log.Ltime|log.Lshortfile)
	WarningLog = log.New(warnW, fmt.Sprintf(fmtS, "WARNING:"), log.Ldate|log.Ltime|log.Lshortfile)
	ErrorLog = log.New(errW, fmt.Sprintf(fmtS, "ERROR:"), log.Ldate|log.Ltime|log.Lshortfile)

	globalLogFile = f
}
```

Add `"io"` to the imports.

**Step 2: Update all Initialize callers**

There are multiple call sites for `log.Initialize(bool)`. All need the second arg.

In `main.go` (rootCmd RunE): `log.Initialize(daemonFlag, cfg.IsTelemetryEnabled())`
In `main.go` (resetCmd RunE): `log.Initialize(false, false)` (no sentry needed for reset)
In `main.go` (debugCmd RunE): `log.Initialize(false, false)`

Note: sentry must be initialized before `log.Initialize` is called with `telemetryEnabled=true`, because the writer checks `sentrypkg.IsEnabled()`.

**Step 3: Run tests**

Run: `go test ./... -v -count=1`
Expected: PASS (existing tests should still work — Initialize signature changed but tests call it)

**Step 4: Commit**

```bash
git add log/log.go main.go
git commit -m "feat(log): wire sentry writer into log package for automatic error forwarding"
```

---

### Task 5: Wire sentry init and panic recovery into main.go and app.go

**Files:**
- Modify: `main.go`
- Modify: `app/app.go`

**Step 1: Wire sentry into main.go**

In `main.go`, in the root command's `RunE`, before `log.Initialize`:

```go
cfg := config.LoadConfig()
sentrypkg.Init(version, cfg.IsTelemetryEnabled())
defer sentrypkg.Flush()
defer sentrypkg.RecoverPanic()
```

Add import: `sentrypkg "github.com/kastheco/kasmos/internal/sentry"`

After `app.Run` context is set up, add context enrichment:

```go
sentrypkg.SetContext(program, autoYes, filepath.Base(currentDir))
```

**Step 2: Add panic recovery wrapper in app.Run**

In `app/app.go`, wrap `p.Run()`:

```go
func Run(ctx context.Context, program string, autoYes bool) (retErr error) {
	restore := ui.SetTerminalBackground(string(ui.ColorBase))
	defer restore()

	defer func() {
		if r := recover(); r != nil {
			sentrypkg.RecoverPanic() // won't fire — we already recovered
			// Capture manually
			gosentry.CurrentHub().Recover(r)
			sentrypkg.Flush()
			retErr = fmt.Errorf("panic: %v", r)
		}
	}()

	zone.NewGlobal()
	p := tea.NewProgram(
		newHome(ctx, program, autoYes),
		tea.WithAltScreen(),
		tea.WithMouseAllMotion(),
	)
	_, err := p.Run()
	return err
}
```

Actually, simpler — just use `defer sentrypkg.RecoverPanic()` which re-panics after capturing. The re-panic will be caught by main's defer. So:

```go
func Run(ctx context.Context, program string, autoYes bool) error {
	restore := ui.SetTerminalBackground(string(ui.ColorBase))
	defer restore()
	defer sentrypkg.RecoverPanic()

	zone.NewGlobal()
	p := tea.NewProgram(
		newHome(ctx, program, autoYes),
		tea.WithAltScreen(),
		tea.WithMouseAllMotion(),
	)
	_, err := p.Run()
	return err
}
```

**Step 3: Run full test suite**

Run: `go test ./... -v -count=1`
Expected: PASS

**Step 4: Build and smoke test**

Run: `go build -o kasmos . && ./kasmos version`
Expected: prints version, no sentry errors

**Step 5: Commit**

```bash
git add main.go app/app.go
git commit -m "feat: wire sentry init and panic recovery into main and TUI entry"
```

---

### Task 6: Promote sentry-go from indirect to direct dependency

**Files:**
- Modify: `go.mod`

**Step 1: Run go mod tidy**

Run: `go mod tidy`

This should promote `sentry-go` from `// indirect` to a direct dependency since `internal/sentry` now imports it directly.

**Step 2: Verify**

Run: `grep sentry go.mod`
Expected: `github.com/getsentry/sentry-go v0.43.0` (without `// indirect`)

**Step 3: Run full test suite**

Run: `go test ./... -count=1`
Expected: PASS

**Step 4: Commit**

```bash
git add go.mod go.sum
git commit -m "deps: promote sentry-go to direct dependency"
```
