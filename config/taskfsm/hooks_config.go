package taskfsm

import "log/slog"

// HookConfig is the configuration shape for a single FSM hook entry.
// It is intentionally a mirror of config.TOMLHook to avoid an import cycle
// between package config and package taskfsm.
type HookConfig struct {
	Type    string            `json:"type"              toml:"type"`
	URL     string            `json:"url,omitempty"     toml:"url,omitempty"`
	Headers map[string]string `json:"headers,omitempty" toml:"headers,omitempty"`
	Command string            `json:"command,omitempty" toml:"command,omitempty"`
	Events  []string          `json:"events,omitempty"  toml:"events,omitempty"`
}

// parseHookEvents converts a slice of raw event name strings to typed Events.
// Unknown strings produce a slog.Warn and are omitted from the result.
func parseHookEvents(raw []string) []Event {
	out := make([]Event, 0, len(raw))
	for _, s := range raw {
		switch Event(s) {
		case PlanStart, PlannerFinished, ImplementStart, ImplementFinished,
			ReviewApproved, ReviewChangesRequested, RequestReview,
			StartOver, Reimplement, Cancel, Reopen:
			out = append(out, Event(s))
		default:
			slog.Warn("hook config: unknown event name, skipping", "event", s)
		}
	}
	return out
}

// BuildHookRegistry instantiates a HookRegistry from a slice of HookConfig
// entries. It always returns a non-nil registry. Invalid or incomplete entries
// are skipped with a slog.Warn log line.
func BuildHookRegistry(configs []HookConfig) *HookRegistry {
	reg := NewHookRegistry()
	for _, cfg := range configs {
		events := parseHookEvents(cfg.Events)
		switch cfg.Type {
		case "webhook":
			if cfg.URL == "" {
				slog.Warn("hook config: webhook entry has empty url, skipping")
				continue
			}
			reg.Add(NewWebhookHook(cfg.URL, cfg.Headers), events)

		case "notify":
			// notify takes no extra config; ignore URL/Command if supplied.
			reg.Add(NewNotifyHook(), events)

		case "command":
			if cfg.Command == "" {
				slog.Warn("hook config: command entry has empty command, skipping")
				continue
			}
			reg.Add(NewCommandHook(cfg.Command), events)

		default:
			slog.Warn("hook config: unknown hook type, skipping", "type", cfg.Type)
		}
	}
	return reg
}
