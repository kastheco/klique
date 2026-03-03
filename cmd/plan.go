package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/kastheco/kasmos/config"
	"github.com/kastheco/kasmos/config/planfsm"
	"github.com/kastheco/kasmos/config/planstate"
	"github.com/kastheco/kasmos/config/planstore"
	"github.com/kastheco/kasmos/internal/clickup"
	"github.com/spf13/cobra"
)

// executePlanRegister registers a plan file that exists on disk but isn't
// tracked in plan state yet. It extracts a description from the first markdown
// heading and uses the conventional branch name format.
func executePlanRegister(plansDir, planFile, branch, topic, description string, store planstore.Store) error {
	fullPath := filepath.Join(plansDir, planFile)
	if _, err := os.Stat(fullPath); err != nil {
		return fmt.Errorf("plan file not found on disk: %s", fullPath)
	}
	ps, err := loadPlanState(plansDir, store)
	if err != nil {
		return err
	}
	if description == "" {
		data, err := os.ReadFile(fullPath)
		if err != nil {
			return err
		}
		description = strings.TrimSuffix(planFile, ".md")
		for _, line := range strings.Split(string(data), "\n") {
			if strings.HasPrefix(line, "# ") {
				description = strings.TrimPrefix(line, "# ")
				break
			}
		}
	}
	// Default branch: plan/<slug> derived from the plan filename.
	if branch == "" {
		slug := strings.TrimSuffix(planFile, ".md")
		branch = "plan/" + slug
	}
	info, _ := os.Stat(fullPath)
	createdAt := info.ModTime()
	return ps.Create(planFile, description, branch, topic, createdAt)
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
		current = planfsm.StatusReady
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

// executePlanLinkClickUp iterates all plans in the given project, reads their
// content, parses the ClickUp task ID from the "**Source:** ClickUp <ID>" line,
// and stores it in the clickup_task_id field for any plan that has an ID in its
// content but not yet in the store. Returns the count of plans updated.
func executePlanLinkClickUp(project string, store planstore.Store) (int, error) {
	plans, err := store.List(project)
	if err != nil {
		return 0, fmt.Errorf("list plans: %w", err)
	}

	updated := 0
	for _, plan := range plans {
		// Skip plans that already have a ClickUp task ID.
		if plan.ClickUpTaskID != "" {
			continue
		}

		content, err := store.GetContent(project, plan.Filename)
		if err != nil {
			// Non-fatal: skip plans whose content can't be read.
			continue
		}

		taskID := clickup.ParseClickUpTaskID(content)
		if taskID == "" {
			continue
		}

		if err := store.SetClickUpTaskID(project, plan.Filename, taskID); err != nil {
			return updated, fmt.Errorf("set clickup task id for %s: %w", plan.Filename, err)
		}
		updated++
	}

	return updated, nil
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
	var branchFlag, topicFlag, descriptionFlag string
	registerCmd := &cobra.Command{
		Use:   "register <plan-file>",
		Short: "register an untracked plan file (sets status to ready)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			plansDir, err := resolvePlansDir()
			if err != nil {
				return err
			}
			if err := executePlanRegister(plansDir, args[0], branchFlag, topicFlag, descriptionFlag, resolveStore(plansDir)); err != nil {
				return err
			}
			fmt.Printf("registered: %s → ready\n", args[0])
			return nil
		},
	}
	registerCmd.Flags().StringVar(&branchFlag, "branch", "", "override branch name (default: plan/<slug>)")
	registerCmd.Flags().StringVar(&topicFlag, "topic", "", "assign plan to a topic group (auto-creates topic if needed)")
	registerCmd.Flags().StringVar(&descriptionFlag, "description", "", "override description (default: extracted from first # heading)")
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

	// kq plan link-clickup
	var linkProject string
	linkClickUpCmd := &cobra.Command{
		Use:   "link-clickup",
		Short: "backfill ClickUp task IDs from plan content (parses **Source:** ClickUp <ID> lines)",
		RunE: func(cmd *cobra.Command, args []string) error {
			store, project := resolveStoreConfig("")
			if store == nil {
				var err error
				store, err = localSQLiteStore()
				if err != nil {
					return fmt.Errorf("open local plan store: %w", err)
				}
				defer store.Close()
			}
			if linkProject != "" {
				project = linkProject
			}
			if project == "" {
				plansDir, err := resolvePlansDir()
				if err != nil {
					return err
				}
				project = projectFromPlansDir(plansDir)
			}
			n, err := executePlanLinkClickUp(project, store)
			if err != nil {
				return err
			}
			fmt.Printf("linked %d plan(s) to ClickUp tasks\n", n)
			return nil
		},
	}
	linkClickUpCmd.Flags().StringVar(&linkProject, "project", "", "project name (default: derived from current directory)")
	planCmd.AddCommand(linkClickUpCmd)

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
// canonical path returned by planstore.ResolvedDBPath(). Used as a fallback
// when no remote store is configured.
func localSQLiteStore() (planstore.Store, error) {
	dbPath := planstore.ResolvedDBPath()
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

// resolvePlansDir resolves docs/plans/ relative to cwd. When the cwd doesn't
// contain docs/plans/ (e.g. when running from a git worktree), it falls back
// to resolving the main repo root via resolveRepoRoot and looks for docs/plans/
// there.
func resolvePlansDir() (string, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("get cwd: %w", err)
	}

	// Fast path: docs/plans/ exists relative to cwd (main repo case).
	dir := filepath.Join(cwd, "docs", "plans")
	if _, err := os.Stat(dir); err == nil {
		return dir, nil
	}

	// Fallback: resolve the main repo root (worktree case) and try there.
	root, err := resolveRepoRoot(cwd)
	if err != nil {
		return "", fmt.Errorf("plans directory not found in cwd and cannot resolve repo root: %w", err)
	}
	dir = filepath.Join(root, "docs", "plans")
	if _, err := os.Stat(dir); err != nil {
		return "", fmt.Errorf("plans directory not found: %s", dir)
	}
	return dir, nil
}

// resolveRepoRoot returns the root directory of the git repository that owns
// dir. It handles both regular repositories (where .git is a directory) and
// git worktrees (where .git is a file with a "gitdir:" pointer). On failure it
// falls back to shelling out to `git rev-parse --show-toplevel`.
func resolveRepoRoot(dir string) (string, error) {
	gitPath := filepath.Join(dir, ".git")
	info, err := os.Stat(gitPath)
	if err != nil {
		// .git not found — try git CLI as last resort.
		return resolveRepoRootViaGit(dir)
	}

	if info.IsDir() {
		// Regular repo: .git is a directory, so dir IS the repo root.
		return dir, nil
	}

	// Worktree: .git is a file with content "gitdir: <path>"
	data, err := os.ReadFile(gitPath)
	if err != nil {
		return resolveRepoRootViaGit(dir)
	}
	line := strings.TrimSpace(string(data))
	if !strings.HasPrefix(line, "gitdir: ") {
		return resolveRepoRootViaGit(dir)
	}

	// worktreeGitDir is the per-worktree git dir (e.g. /repo/.git/worktrees/name)
	worktreeGitDir := strings.TrimPrefix(line, "gitdir: ")
	if !filepath.IsAbs(worktreeGitDir) {
		worktreeGitDir = filepath.Join(dir, worktreeGitDir)
	}
	worktreeGitDir = filepath.Clean(worktreeGitDir)

	// commondir contains a relative (or absolute) path back to the main .git dir.
	commondirPath := filepath.Join(worktreeGitDir, "commondir")
	commondirData, err := os.ReadFile(commondirPath)
	if err != nil {
		return resolveRepoRootViaGit(dir)
	}
	commondir := strings.TrimSpace(string(commondirData))

	var mainGitDir string
	if filepath.IsAbs(commondir) {
		mainGitDir = commondir
	} else {
		mainGitDir = filepath.Clean(filepath.Join(worktreeGitDir, commondir))
	}

	// The repo root is the parent of the main .git directory.
	return filepath.Dir(mainGitDir), nil
}

// resolveRepoRootViaGit shells out to git to find the main repository root.
// It uses --git-common-dir (which always points to the main repo's .git) rather
// than --show-toplevel (which returns the worktree checkout path in worktrees).
func resolveRepoRootViaGit(dir string) (string, error) {
	out, err := exec.Command("git", "-C", dir, "rev-parse", "--git-common-dir").Output()
	if err != nil {
		return "", fmt.Errorf("resolve repo root for %s: %w", dir, err)
	}
	gitDir := strings.TrimSpace(string(out))
	if !filepath.IsAbs(gitDir) {
		gitDir = filepath.Join(dir, gitDir)
	}
	// --git-common-dir returns the .git directory; repo root is its parent.
	return filepath.Dir(filepath.Clean(gitDir)), nil
}
