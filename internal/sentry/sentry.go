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
