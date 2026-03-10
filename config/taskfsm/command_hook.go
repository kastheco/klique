package taskfsm

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// CommandHook executes a shell command on each FSM transition. The command is
// run via `sh -c` so shell expansion works as expected. Several KASMOS_HOOK_*
// environment variables are injected so that the command can introspect the
// transition that fired it.
type CommandHook struct {
	command string
}

// NewCommandHook returns a CommandHook that runs command on every transition.
func NewCommandHook(command string) *CommandHook {
	return &CommandHook{command: command}
}

// Name satisfies HookRunner.
func (c *CommandHook) Name() string { return "command" }

// Run executes the hook command in a shell subprocess. It returns an error if
// the command is empty, if the process exits non-zero, or if ctx is cancelled
// before the process finishes.
func (c *CommandHook) Run(ctx context.Context, ev TransitionEvent) error {
	if strings.TrimSpace(c.command) == "" {
		return fmt.Errorf("command hook: empty command")
	}

	cmd := exec.CommandContext(ctx, "sh", "-c", c.command)
	cmd.Env = append(os.Environ(),
		"KASMOS_HOOK_PLAN="+ev.PlanFile,
		"KASMOS_HOOK_EVENT="+string(ev.Event),
		"KASMOS_HOOK_FROM="+string(ev.FromStatus),
		"KASMOS_HOOK_TO="+string(ev.ToStatus),
		"KASMOS_HOOK_PROJECT="+ev.Project,
	)

	out, err := cmd.CombinedOutput()
	if err != nil {
		trimmed := strings.TrimSpace(string(out))
		if trimmed != "" {
			return fmt.Errorf("command hook: %w: %s", err, trimmed)
		}
		return fmt.Errorf("command hook: %w", err)
	}
	return nil
}
