package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/kastheco/kasmos/config/planfsm"
	"github.com/kastheco/kasmos/config/planstate"
	"github.com/spf13/cobra"
)

// executePlanList returns a formatted string listing all plans, optionally
// filtered by status. Exported for testing without cobra plumbing.
func executePlanList(plansDir, statusFilter string) string {
	ps, err := planstate.Load(plansDir)
	if err != nil {
		return fmt.Sprintf("error: %v", err)
	}
	var sb strings.Builder
	for _, info := range ps.List() {
		if statusFilter != "" && string(info.Status) != statusFilter {
			continue
		}
		line := fmt.Sprintf("%-14s %-50s %s", info.Status, info.Filename, info.Branch)
		sb.WriteString(strings.TrimRight(line, " ") + "\n")
	}
	return sb.String()
}

// executePlanSetStatus force-overrides a plan's status, bypassing the FSM.
// Requires force=true to prevent accidental misuse.
func executePlanSetStatus(plansDir, planFile, status string, force bool) error {
	if !force {
		return fmt.Errorf("--force required to override plan status (this bypasses the FSM)")
	}
	ps, err := planstate.Load(plansDir)
	if err != nil {
		return err
	}
	return ps.ForceSetStatus(planFile, planstate.Status(status))
}

// executePlanTransition applies a named FSM event to a plan and returns the new status.
func executePlanTransition(plansDir, planFile, event string) (string, error) {
	eventMap := map[string]planfsm.Event{
		"plan_start":         planfsm.PlanStart,
		"planner_finished":   planfsm.PlannerFinished,
		"implement_start":    planfsm.ImplementStart,
		"implement_finished": planfsm.ImplementFinished,
		"review_approved":    planfsm.ReviewApproved,
		"review_changes":     planfsm.ReviewChangesRequested,
		"request_review":     planfsm.RequestReview,
		"start_over":         planfsm.StartOver,
		"reimplement":        planfsm.Reimplement,
		"cancel":             planfsm.Cancel,
		"reopen":             planfsm.Reopen,
	}
	fsmEvent, ok := eventMap[event]
	if !ok {
		names := make([]string, 0, len(eventMap))
		for k := range eventMap {
			names = append(names, k)
		}
		return "", fmt.Errorf("unknown event %q; valid events: %s", event, strings.Join(names, ", "))
	}
	fsm := planfsm.New(plansDir)
	if err := fsm.Transition(planFile, fsmEvent); err != nil {
		return "", err
	}
	ps, err := planstate.Load(plansDir)
	if err != nil {
		return "", err
	}
	entry, _ := ps.Entry(planFile)
	return string(entry.Status), nil
}

// executePlanImplement transitions a plan into implementing state and writes
// a wave signal file so the TUI metadata tick can pick it up.
func executePlanImplement(plansDir, planFile string, wave int) error {
	if wave < 1 {
		return fmt.Errorf("wave number must be >= 1, got %d", wave)
	}
	fsm := planfsm.New(plansDir)
	ps, err := planstate.Load(plansDir)
	if err != nil {
		return err
	}
	entry, ok := ps.Entry(planFile)
	if !ok {
		return fmt.Errorf("plan not found: %s", planFile)
	}
	current := planfsm.Status(entry.Status)
	// If still in planning, finish that phase first (→ ready).
	if current == planfsm.StatusPlanning {
		if err := fsm.Transition(planFile, planfsm.PlannerFinished); err != nil {
			return err
		}
	}
	// Advance to implementing unless already there.
	if current != planfsm.StatusImplementing {
		if err := fsm.Transition(planFile, planfsm.ImplementStart); err != nil {
			return err
		}
	}

	// Write the wave signal file consumed by the TUI metadata tick.
	signalsDir := filepath.Join(plansDir, ".signals")
	if err := os.MkdirAll(signalsDir, 0o755); err != nil {
		return err
	}
	signalName := fmt.Sprintf("implement-wave-%d-%s", wave, planFile)
	return os.WriteFile(filepath.Join(signalsDir, signalName), nil, 0o644)
}

// NewPlanCmd builds the `kq plan` cobra command tree.
func NewPlanCmd() *cobra.Command {
	planCmd := &cobra.Command{
		Use:   "plan",
		Short: "manage plan lifecycle (list, set-status, transition, implement)",
	}

	// kq plan list
	var statusFilter string
	listCmd := &cobra.Command{
		Use:   "list",
		Short: "list all plans with status",
		RunE: func(cmd *cobra.Command, args []string) error {
			plansDir, err := resolvePlansDir()
			if err != nil {
				return err
			}
			fmt.Print(executePlanList(plansDir, statusFilter))
			return nil
		},
	}
	listCmd.Flags().StringVar(&statusFilter, "status", "", "filter by status (ready, planning, implementing, reviewing, done, cancelled)")
	planCmd.AddCommand(listCmd)

	// kq plan set-status
	var forceFlag bool
	setStatusCmd := &cobra.Command{
		Use:   "set-status <plan-file> <status>",
		Short: "force-override a plan's status (bypasses FSM)",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			plansDir, err := resolvePlansDir()
			if err != nil {
				return err
			}
			if err := executePlanSetStatus(plansDir, args[0], args[1], forceFlag); err != nil {
				return err
			}
			fmt.Printf("%s → %s\n", args[0], args[1])
			return nil
		},
	}
	setStatusCmd.Flags().BoolVar(&forceFlag, "force", false, "confirm intent to bypass FSM transition rules")
	planCmd.AddCommand(setStatusCmd)

	// kq plan transition
	transitionCmd := &cobra.Command{
		Use:   "transition <plan-file> <event>",
		Short: "apply an FSM event to a plan",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			plansDir, err := resolvePlansDir()
			if err != nil {
				return err
			}
			newStatus, err := executePlanTransition(plansDir, args[0], args[1])
			if err != nil {
				return err
			}
			fmt.Printf("%s → %s\n", args[0], newStatus)
			return nil
		},
	}
	planCmd.AddCommand(transitionCmd)

	// kq plan implement
	var waveNum int
	implementCmd := &cobra.Command{
		Use:   "implement <plan-file>",
		Short: "trigger implementation of a specific wave",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			plansDir, err := resolvePlansDir()
			if err != nil {
				return err
			}
			if err := executePlanImplement(plansDir, args[0], waveNum); err != nil {
				return err
			}
			fmt.Printf("implementation triggered: %s wave %d\n", args[0], waveNum)
			return nil
		},
	}
	implementCmd.Flags().IntVar(&waveNum, "wave", 1, "wave number to trigger (default: 1)")
	planCmd.AddCommand(implementCmd)

	return planCmd
}

// resolvePlansDir resolves docs/plans/ relative to cwd, returning an error if absent.
func resolvePlansDir() (string, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("get cwd: %w", err)
	}
	dir := filepath.Join(cwd, "docs", "plans")
	if _, err := os.Stat(dir); err != nil {
		return "", fmt.Errorf("plans directory not found: %s", dir)
	}
	return dir, nil
}
