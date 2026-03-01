package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/kastheco/kasmos/config"
	"github.com/kastheco/kasmos/config/planfsm"
	"github.com/kastheco/kasmos/config/planstate"
	"github.com/kastheco/kasmos/config/planstore"
	"github.com/spf13/cobra"
)

// executePlanRegister registers a plan file that exists on disk but isn't
// tracked in plan state yet. It extracts a description from the first markdown
// heading and uses the conventional branch name format.
func executePlanRegister(plansDir, planFile, branch string, store planstore.Store) error {
	fullPath := filepath.Join(plansDir, planFile)
	if _, err := os.Stat(fullPath); err != nil {
		return fmt.Errorf("plan file not found on disk: %s", fullPath)
	}
	ps, err := loadPlanState(plansDir, store)
	if err != nil {
		return err
	}
	// Extract description from first H1 line.
	data, err := os.ReadFile(fullPath)
	if err != nil {
		return err
	}
	desc := strings.TrimSuffix(planFile, ".md")
	for _, line := range strings.Split(string(data), "\n") {
		if strings.HasPrefix(line, "# ") {
			desc = strings.TrimPrefix(line, "# ")
			break
		}
	}
	// Default branch: plan/<slug> where slug strips the date prefix.
	if branch == "" {
		slug := planFile
		if len(slug) > 11 && slug[4] == '-' && slug[7] == '-' && slug[10] == '-' {
			slug = slug[11:]
		}
		slug = strings.TrimSuffix(slug, ".md")
		branch = "plan/" + slug
	}
	info, _ := os.Stat(fullPath)
	createdAt := info.ModTime()
	return ps.Register(planFile, desc, branch, createdAt)
}

// executePlanList returns a formatted string listing all plans, optionally
// filtered by status. Exported for testing without cobra plumbing.
func executePlanList(plansDir, statusFilter string, store planstore.Store) string {
	ps, err := loadPlanState(plansDir, store)
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

// executePlanListWithStore returns a formatted string listing all plans from a
// remote store backend. storeURL is the base URL of the plan store server
// (e.g. "http://athena:7433") and project is the project name to query.
func executePlanListWithStore(storeURL, project string) string {
	store := planstore.NewHTTPStore(storeURL, project)
	ps, err := planstate.Load(store, project, "")
	if err != nil {
		return fmt.Sprintf("error: %v", err)
	}
	var sb strings.Builder
	for _, info := range ps.List() {
		line := fmt.Sprintf("%-14s %-50s %s", info.Status, info.Filename, info.Branch)
		sb.WriteString(strings.TrimRight(line, " ") + "\n")
	}
	return sb.String()
}

// executePlanSetStatus force-overrides a plan's status, bypassing the FSM.
// Requires force=true to prevent accidental misuse.
func executePlanSetStatus(plansDir, planFile, status string, force bool, store planstore.Store) error {
	if !force {
		return fmt.Errorf("--force required to override plan status (this bypasses the FSM)")
	}
	ps, err := loadPlanState(plansDir, store)
	if err != nil {
		return err
	}
	return ps.ForceSetStatus(planFile, planstate.Status(status))
}

// executePlanTransition applies a named FSM event to a plan and returns the new status.
func executePlanTransition(plansDir, planFile, event string, store planstore.Store) (string, error) {
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
	fsm := newFSM(plansDir, store)
	if err := fsm.Transition(planFile, fsmEvent); err != nil {
		return "", err
	}
	ps, err := loadPlanState(plansDir, store)
	if err != nil {
		return "", err
	}
	entry, _ := ps.Entry(planFile)
	return string(entry.Status), nil
}

// executePlanImplement transitions a plan into implementing state and writes
// a wave signal file so the TUI metadata tick can pick it up.
func executePlanImplement(plansDir, planFile string, wave int, store planstore.Store) error {
	if wave < 1 {
		return fmt.Errorf("wave number must be >= 1, got %d", wave)
	}
	fsm := newFSM(plansDir, store)
	ps, err := loadPlanState(plansDir, store)
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
	// plansDir is <repo>/docs/plans — go up two levels to reach the repo root.
	repoRoot := filepath.Dir(filepath.Dir(plansDir))
	signalsDir := filepath.Join(repoRoot, ".kasmos", "signals")
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
			fmt.Print(executePlanList(plansDir, statusFilter, resolveStore(plansDir)))
			return nil
		},
	}
	listCmd.Flags().StringVar(&statusFilter, "status", "", "filter by status (ready, planning, implementing, reviewing, done, cancelled)")
	planCmd.AddCommand(listCmd)

	// kq plan register
	var branchFlag string
	registerCmd := &cobra.Command{
		Use:   "register <plan-file>",
		Short: "register an untracked plan file (sets status to ready)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			plansDir, err := resolvePlansDir()
			if err != nil {
				return err
			}
			if err := executePlanRegister(plansDir, args[0], branchFlag, resolveStore(plansDir)); err != nil {
				return err
			}
			fmt.Printf("registered: %s → ready\n", args[0])
			return nil
		},
	}
	registerCmd.Flags().StringVar(&branchFlag, "branch", "", "override branch name (default: plan/<slug>)")
	planCmd.AddCommand(registerCmd)

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
			if err := executePlanSetStatus(plansDir, args[0], args[1], forceFlag, resolveStore(plansDir)); err != nil {
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
			newStatus, err := executePlanTransition(plansDir, args[0], args[1], resolveStore(plansDir))
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
			if err := executePlanImplement(plansDir, args[0], waveNum, resolveStore(plansDir)); err != nil {
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

// resolveStoreConfig returns the remote store and project name from config.
// Returns (nil, "") when no remote store is configured.
func resolveStoreConfig(plansDir string) (planstore.Store, string) {
	cfg := config.LoadConfig()
	if cfg.PlanStore == "" {
		return nil, ""
	}
	project := projectFromPlansDir(plansDir)
	store, err := planstore.NewStoreFromConfig(cfg.PlanStore, project)
	if err != nil || store == nil {
		return nil, ""
	}
	return store, project
}

// projectFromPlansDir extracts the repo/project name from a plansDir path.
// plansDir is typically <repo>/docs/plans — go up two levels.
func projectFromPlansDir(plansDir string) string {
	return filepath.Base(filepath.Dir(filepath.Dir(plansDir)))
}

// localSQLiteStore opens (or creates) the local SQLite plan store at the
// default path ($HOME/.config/kasmos/plans.db). Used as a fallback when no
// remote store is configured.
func localSQLiteStore() (planstore.Store, error) {
	dbPath := os.ExpandEnv("$HOME/.config/kasmos/plans.db")
	if err := os.MkdirAll(filepath.Dir(dbPath), 0o755); err != nil {
		return nil, fmt.Errorf("create kasmos config dir: %w", err)
	}
	return planstore.NewSQLiteStore(dbPath)
}

// loadPlanState loads plan state using the store backend.
// When store is nil, falls back to the local SQLite store.
func loadPlanState(plansDir string, store planstore.Store) (*planstate.PlanState, error) {
	if store == nil {
		var err error
		store, err = localSQLiteStore()
		if err != nil {
			return nil, fmt.Errorf("open local plan store: %w", err)
		}
	}
	return planstate.Load(store, projectFromPlansDir(plansDir), plansDir)
}

// newFSM creates a PlanStateMachine backed by the given store.
// When store is nil, falls back to the local SQLite store.
func newFSM(plansDir string, store planstore.Store) *planfsm.PlanStateMachine {
	if store == nil {
		var err error
		store, err = localSQLiteStore()
		if err != nil {
			// Panic is acceptable here — this is a CLI tool and the store
			// is required for all operations.
			panic("newFSM: open local plan store: " + err.Error())
		}
	}
	project := projectFromPlansDir(plansDir)
	return planfsm.New(store, project, plansDir)
}

// resolveStore returns the remote plan store from config, or nil if not
// configured or unreachable.
func resolveStore(plansDir string) planstore.Store {
	store, _ := resolveStoreConfig(plansDir)
	if store != nil && store.Ping() == nil {
		return store
	}
	return nil
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
