package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/kastheco/kasmos/config"
	"github.com/kastheco/kasmos/config/taskfsm"
	"github.com/kastheco/kasmos/config/taskstate"
	"github.com/kastheco/kasmos/config/taskstore"
	"github.com/kastheco/kasmos/internal/clickup"
	"github.com/kastheco/kasmos/session/git"
	"github.com/spf13/cobra"
)

// executeTaskRegister registers a plan file into the task store. The filePath
// is resolved relative to the caller's working directory.
func executeTaskRegister(project, filePath, branch, topic, description string, store taskstore.Store) error {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return fmt.Errorf("task file not found: %s", filePath)
	}
	ps, err := loadTaskStateByProject(project, store)
	if err != nil {
		return err
	}
	planFile := filepath.Base(filePath)
	if description == "" {
		description = strings.TrimSuffix(planFile, ".md")
		for _, line := range strings.Split(string(data), "\n") {
			if strings.HasPrefix(line, "# ") {
				description = strings.TrimPrefix(line, "# ")
				break
			}
		}
	}
	if branch == "" {
		slug := strings.TrimSuffix(planFile, ".md")
		branch = "plan/" + slug
	}
	info, _ := os.Stat(filePath)
	createdAt := info.ModTime()
	return ps.CreateWithContent(planFile, description, branch, topic, createdAt, string(data))
}

// executeTaskList returns a formatted string listing all plans, optionally
// filtered by status. Exported for testing without cobra plumbing.
func executeTaskList(project, statusFilter string, store taskstore.Store) string {
	ps, err := loadTaskStateByProject(project, store)
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

// executeTaskListWithStore returns a formatted string listing all plans from a
// remote store backend. storeURL is the base URL of the task store server
// (e.g. "http://athena:7433") and project is the project name to query.
func executeTaskListWithStore(storeURL, project string) string {
	store := taskstore.NewHTTPStore(storeURL, project)
	ps, err := taskstate.Load(store, project, "")
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

// executeTaskSetStatus force-overrides a plan's status, bypassing the FSM.
// Requires force=true to prevent accidental misuse.
func executeTaskSetStatus(project, planFile, status string, force bool, store taskstore.Store) error {
	if !force {
		return fmt.Errorf("--force required to override task status (this bypasses the FSM)")
	}
	ps, err := loadTaskStateByProject(project, store)
	if err != nil {
		return err
	}
	return ps.ForceSetStatus(planFile, taskstate.Status(status))
}

// executeTaskTransition applies a named FSM event to a plan and returns the new status.
func executeTaskTransition(project, planFile, event string, store taskstore.Store) (string, error) {
	eventMap := map[string]taskfsm.Event{
		"plan_start":         taskfsm.PlanStart,
		"planner_finished":   taskfsm.PlannerFinished,
		"implement_start":    taskfsm.ImplementStart,
		"implement_finished": taskfsm.ImplementFinished,
		"review_approved":    taskfsm.ReviewApproved,
		"review_changes":     taskfsm.ReviewChangesRequested,
		"request_review":     taskfsm.RequestReview,
		"start_over":         taskfsm.StartOver,
		"reimplement":        taskfsm.Reimplement,
		"cancel":             taskfsm.Cancel,
		"reopen":             taskfsm.Reopen,
	}
	fsmEvent, ok := eventMap[event]
	if !ok {
		names := make([]string, 0, len(eventMap))
		for k := range eventMap {
			names = append(names, k)
		}
		return "", fmt.Errorf("unknown event %q; valid events: %s", event, strings.Join(names, ", "))
	}
	fsm := newFSMByProject(project, store)
	if err := fsm.Transition(planFile, fsmEvent); err != nil {
		return "", err
	}
	ps, err := loadTaskStateByProject(project, store)
	if err != nil {
		return "", err
	}
	entry, _ := ps.Entry(planFile)
	return string(entry.Status), nil
}

// executeTaskImplement transitions a plan into implementing state and writes
// a wave signal file so the TUI metadata tick can pick it up.
func executeTaskImplement(repoRoot, project, planFile string, wave int, store taskstore.Store) error {
	if wave < 1 {
		return fmt.Errorf("wave number must be >= 1, got %d", wave)
	}
	fsm := newFSMByProject(project, store)
	ps, err := loadTaskStateByProject(project, store)
	if err != nil {
		return err
	}
	entry, ok := ps.Entry(planFile)
	if !ok {
		return fmt.Errorf("task not found: %s", planFile)
	}
	current := taskfsm.Status(entry.Status)
	// If still in planning, finish that phase first (→ ready).
	if current == taskfsm.StatusPlanning {
		if err := fsm.Transition(planFile, taskfsm.PlannerFinished); err != nil {
			return err
		}
		current = taskfsm.StatusReady
	}
	// Advance to implementing unless already there.
	if current != taskfsm.StatusImplementing {
		if err := fsm.Transition(planFile, taskfsm.ImplementStart); err != nil {
			return err
		}
	}

	// Write the wave signal file consumed by the TUI metadata tick.
	signalsDir := filepath.Join(repoRoot, ".kasmos", "signals")
	if err := os.MkdirAll(signalsDir, 0o755); err != nil {
		return err
	}
	signalName := fmt.Sprintf("implement-wave-%d-%s", wave, planFile)
	return os.WriteFile(filepath.Join(signalsDir, signalName), nil, 0o644)
}

// executeTaskShow retrieves plan content from the task store and returns it
// as raw markdown. Returns an error if the plan doesn't exist or has no content.
func executeTaskShow(project, planFile string, store taskstore.Store) (string, error) {
	ps, err := loadTaskStateByProject(project, store)
	if err != nil {
		return "", err
	}
	if _, ok := ps.Entry(planFile); !ok {
		return "", fmt.Errorf("task not found: %s", planFile)
	}
	content, err := ps.GetContent(planFile)
	if err != nil {
		return "", fmt.Errorf("get content for %s: %w", planFile, err)
	}
	if strings.TrimSpace(content) == "" {
		return "", fmt.Errorf("no content stored for %s", planFile)
	}
	return content, nil
}

// executeTaskLinkClickUp iterates all plans in the given project, reads their
// content, parses the ClickUp task ID from the "**Source:** ClickUp <ID>" line,
// and stores it in the clickup_task_id field for any plan that has an ID in its
// content but not yet in the store. Returns the count of plans updated.
func executeTaskLinkClickUp(project string, store taskstore.Store) (int, error) {
	plans, err := store.List(project)
	if err != nil {
		return 0, fmt.Errorf("list tasks: %w", err)
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

// resolveTaskEntry loads task state for the given project, looks up the entry
// by filename, and backfills the branch name if it is empty (using
// git.TaskBranchFromFile). Returns an error if the entry is not found.
func resolveTaskEntry(project, filename string, store taskstore.Store) (taskstate.TaskEntry, error) {
	ps, err := loadTaskStateByProject(project, store)
	if err != nil {
		return taskstate.TaskEntry{}, err
	}
	entry, ok := ps.Entry(filename)
	if !ok {
		return taskstate.TaskEntry{}, fmt.Errorf("task not found: %s", filename)
	}
	if entry.Branch == "" {
		entry.Branch = git.TaskBranchFromFile(filename)
	}
	return entry, nil
}

// executeTaskCreate creates a new task entry in the store. name is the plan
// name without the .md extension. branch defaults to "plan/<name>" when empty.
// If content is non-empty, it is stored alongside the metadata.
func executeTaskCreate(project, name, description, branch, topic, content string, store taskstore.Store) error {
	filename := name + ".md"
	if branch == "" {
		branch = "plan/" + name
	}
	ps, err := loadTaskStateByProject(project, store)
	if err != nil {
		return err
	}
	createdAt := time.Now()
	if content != "" {
		return ps.CreateWithContent(filename, description, branch, topic, createdAt, content)
	}
	return ps.Create(filename, description, branch, topic, createdAt)
}

// executeTaskStart transitions a plan to implementing status and sets up the
// git branch + worktree. It walks planning → ready → implementing via the FSM
// if the plan is currently in planning state. Returns the worktree path.
func executeTaskStart(repoRoot, project, planFile string, store taskstore.Store) (string, error) {
	fsm := newFSMByProject(project, store)

	ps, err := loadTaskStateByProject(project, store)
	if err != nil {
		return "", err
	}
	entry, ok := ps.Entry(planFile)
	if !ok {
		return "", fmt.Errorf("task not found: %s", planFile)
	}

	current := taskfsm.Status(entry.Status)
	// Walk planning → ready first if needed.
	if current == taskfsm.StatusPlanning {
		if err := fsm.Transition(planFile, taskfsm.PlannerFinished); err != nil {
			return "", err
		}
		current = taskfsm.StatusReady
	}
	// Advance to implementing.
	if current != taskfsm.StatusImplementing {
		if err := fsm.Transition(planFile, taskfsm.ImplementStart); err != nil {
			return "", err
		}
	}

	// Resolve branch — backfill if not set.
	branch := entry.Branch
	if branch == "" {
		branch = git.TaskBranchFromFile(planFile)
	}

	// Set up the git branch and worktree.
	if err := git.EnsureTaskBranch(repoRoot, branch); err != nil {
		return "", fmt.Errorf("ensure task branch: %w", err)
	}
	wt := git.NewSharedTaskWorktree(repoRoot, branch)
	if err := wt.Setup(); err != nil {
		return "", fmt.Errorf("setup worktree: %w", err)
	}
	return wt.GetWorktreePath(), nil
}

// executeTaskPush resolves the task entry and its branch, constructs a
// GitWorktree from stored state, commits any dirty changes, and pushes to
// origin. The commit message defaults to "update from kas" when empty.
func executeTaskPush(repoRoot, project, planFile, message string, store taskstore.Store) error {
	if message == "" {
		message = "update from kas"
	}
	entry, err := resolveTaskEntry(project, planFile, store)
	if err != nil {
		return err
	}
	branch := entry.Branch
	worktreePath := git.TaskWorktreePath(repoRoot, branch)
	wt := git.NewGitWorktreeFromStorage(repoRoot, worktreePath, "push", branch, "")
	return wt.PushChanges(message, false)
}

// executeTaskMerge merges the plan branch into the current branch (typically
// main), then walks the FSM to done. If the task is not yet in reviewing state,
// it transitions through ImplementFinished first. Returns an error if the git
// merge fails.
func executeTaskMerge(repoRoot, project, planFile string, store taskstore.Store) error {
	entry, err := resolveTaskEntry(project, planFile, store)
	if err != nil {
		return err
	}
	branch := entry.Branch

	// Validate repoRoot before attempting git operations.
	if _, serr := os.Stat(repoRoot); serr != nil {
		return fmt.Errorf("invalid repo root %q: %w", repoRoot, serr)
	}

	// Git merge first — only advance the FSM on success.
	if err := git.MergeTaskBranch(repoRoot, branch); err != nil {
		return err
	}

	// Walk FSM to done.
	fsm := newFSMByProject(project, store)
	current := taskfsm.Status(entry.Status)
	if current != taskfsm.StatusReviewing {
		if current == taskfsm.StatusImplementing {
			if err := fsm.Transition(planFile, taskfsm.ImplementFinished); err != nil {
				return err
			}
		} else {
			// Force to reviewing so ReviewApproved can proceed.
			ps, lerr := loadTaskStateByProject(project, store)
			if lerr != nil {
				return lerr
			}
			if ferr := ps.ForceSetStatus(planFile, taskstate.StatusReviewing); ferr != nil {
				return ferr
			}
		}
	}
	return fsm.Transition(planFile, taskfsm.ReviewApproved)
}

// executeTaskStartOver removes the plan worktree, deletes and recreates the
// branch from HEAD (via git.ResetTaskBranch), then transitions the FSM to
// planning. Uses StartOver FSM event for states where it is valid; falls back
// to ForceSetStatus for all other states.
func executeTaskStartOver(repoRoot, project, planFile string, store taskstore.Store) error {
	entry, err := resolveTaskEntry(project, planFile, store)
	if err != nil {
		return err
	}
	branch := entry.Branch

	// Validate repoRoot before attempting git operations.
	if _, serr := os.Stat(repoRoot); serr != nil {
		return fmt.Errorf("invalid repo root %q: %w", repoRoot, serr)
	}

	// Git reset first — only touch FSM on success.
	if err := git.ResetTaskBranch(repoRoot, branch); err != nil {
		return err
	}

	// Try the FSM StartOver event; fall back to ForceSetStatus if not valid
	// from the current state (StartOver is only defined from done in the FSM).
	fsm := newFSMByProject(project, store)
	if ferr := fsm.Transition(planFile, taskfsm.StartOver); ferr != nil {
		ps, lerr := loadTaskStateByProject(project, store)
		if lerr != nil {
			return lerr
		}
		return ps.ForceSetStatus(planFile, taskstate.StatusPlanning)
	}
	return nil
}

// executeTaskPR resolves the task entry, derives the PR title from the task
// description when title is empty, generates a PR body from the git log, and
// creates (or reopens) the PR via the GitHub CLI. The PR URL is printed to
// stdout by the gh CLI; the returned string is currently always empty.
func executeTaskPR(repoRoot, project, planFile, title string, store taskstore.Store) (string, error) {
	entry, err := resolveTaskEntry(project, planFile, store)
	if err != nil {
		return "", err
	}
	branch := entry.Branch
	if title == "" {
		title = entry.Description
	}
	if title == "" {
		// Last-resort: use the filename stem as the title.
		title = strings.TrimSuffix(planFile, ".md")
	}
	worktreePath := git.TaskWorktreePath(repoRoot, branch)
	wt := git.NewGitWorktreeFromStorage(repoRoot, worktreePath, "pr", branch, "")
	body, _ := wt.GeneratePRBody()
	if err := wt.CreatePR(title, body, "update from kas"); err != nil {
		return "", err
	}
	return "", nil
}

// NewTaskCmd builds the `kq plan` cobra command tree.
func NewTaskCmd() *cobra.Command {
	planCmd := &cobra.Command{
		Use:   "task",
		Short: "manage task lifecycle (list, set-status, transition, implement)",
	}

	// kq plan list
	var statusFilter string
	listCmd := &cobra.Command{
		Use:   "list",
		Short: "list all tasks with status",
		RunE: func(cmd *cobra.Command, args []string) error {
			_, project, err := resolveRepoInfo()
			if err != nil {
				return err
			}
			fmt.Print(executeTaskList(project, statusFilter, resolveStore(project)))
			return nil
		},
	}
	listCmd.Flags().StringVar(&statusFilter, "status", "", "filter by status (ready, planning, implementing, reviewing, done, cancelled)")
	planCmd.AddCommand(listCmd)

	// kq plan register
	var branchFlag, topicFlag, descriptionFlag string
	registerCmd := &cobra.Command{
		Use:   "register <plan-file>",
		Short: "register an untracked task file (sets status to ready)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			_, project, err := resolveRepoInfo()
			if err != nil {
				return err
			}
			if err := executeTaskRegister(project, args[0], branchFlag, topicFlag, descriptionFlag, resolveStore(project)); err != nil {
				return err
			}
			fmt.Printf("registered: %s → ready\n", filepath.Base(args[0]))
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
		Short: "force-override a task's status (bypasses FSM)",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			_, project, err := resolveRepoInfo()
			if err != nil {
				return err
			}
			if err := executeTaskSetStatus(project, args[0], args[1], forceFlag, resolveStore(project)); err != nil {
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
		Short: "apply an FSM event to a task",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			_, project, err := resolveRepoInfo()
			if err != nil {
				return err
			}
			newStatus, err := executeTaskTransition(project, args[0], args[1], resolveStore(project))
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
			repoRoot, project, err := resolveRepoInfo()
			if err != nil {
				return err
			}
			if err := executeTaskImplement(repoRoot, project, args[0], waveNum, resolveStore(project)); err != nil {
				return err
			}
			fmt.Printf("implementation triggered: %s wave %d\n", args[0], waveNum)
			return nil
		},
	}
	implementCmd.Flags().IntVar(&waveNum, "wave", 1, "wave number to trigger (default: 1)")
	planCmd.AddCommand(implementCmd)

	// kq task show
	showCmd := &cobra.Command{
		Use:   "show <plan-file>",
		Short: "print plan content from the task store",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			_, project, err := resolveRepoInfo()
			if err != nil {
				return err
			}
			content, err := executeTaskShow(project, args[0], resolveStore(project))
			if err != nil {
				return err
			}
			fmt.Print(content)
			return nil
		},
	}
	planCmd.AddCommand(showCmd)

	// kq task update-content <plan-file> < content.md
	updateContentCmd := &cobra.Command{
		Use:   "update-content <plan-file>",
		Short: "replace plan content in the task store (reads from stdin or --file)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			_, project, err := resolveRepoInfo()
			if err != nil {
				return err
			}
			store, storeProject := resolveStoreConfig(project)
			if store == nil {
				store, err = localSQLiteStore()
				if err != nil {
					return fmt.Errorf("open local task store: %w", err)
				}
				defer store.Close()
				storeProject = project
			}
			filename := args[0]
			if !strings.HasSuffix(filename, ".md") {
				filename += ".md"
			}
			contentFile, _ := cmd.Flags().GetString("file")
			var data []byte
			if contentFile != "" {
				data, err = os.ReadFile(contentFile)
			} else {
				data, err = os.ReadFile("/dev/stdin")
			}
			if err != nil {
				return fmt.Errorf("read content: %w", err)
			}
			if err := store.SetContent(storeProject, filename, string(data)); err != nil {
				return err
			}
			fmt.Printf("updated content for %s\n", filename)
			return nil
		},
	}
	updateContentCmd.Flags().String("file", "", "read content from file instead of stdin")
	planCmd.AddCommand(updateContentCmd)

	// kas task create
	var (
		createDescription string
		createBranch      string
		createTopic       string
		createContent     string
	)
	createCmd := &cobra.Command{
		Use:   "create <name>",
		Short: "create a new task entry in the store",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			_, project, err := resolveRepoInfo()
			if err != nil {
				return err
			}
			name := args[0]
			if err := executeTaskCreate(project, name, createDescription, createBranch, createTopic, createContent, resolveStore(project)); err != nil {
				return err
			}
			fmt.Printf("created: %s.md → ready\n", name)
			return nil
		},
	}
	createCmd.Flags().StringVar(&createDescription, "description", "", "task description")
	createCmd.Flags().StringVar(&createBranch, "branch", "", "git branch name (default: plan/<name>)")
	createCmd.Flags().StringVar(&createTopic, "topic", "", "topic group")
	createCmd.Flags().StringVar(&createContent, "content", "", "initial plan content (markdown)")
	planCmd.AddCommand(createCmd)

	// kas task start
	startCmd := &cobra.Command{
		Use:   "start <plan-file>",
		Short: "transition a task to implementing and set up the git worktree",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			repoRoot, project, err := resolveRepoInfo()
			if err != nil {
				return err
			}
			worktreePath, err := executeTaskStart(repoRoot, project, args[0], resolveStore(project))
			if err != nil {
				return err
			}
			fmt.Printf("started: %s → implementing\nworktree: %s\n", args[0], worktreePath)
			return nil
		},
	}
	planCmd.AddCommand(startCmd)

	// kas task push
	var pushMessage string
	pushCmd := &cobra.Command{
		Use:   "push <plan-file>",
		Short: "commit dirty changes and push the task branch to origin",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			repoRoot, project, err := resolveRepoInfo()
			if err != nil {
				return err
			}
			if err := executeTaskPush(repoRoot, project, args[0], pushMessage, resolveStore(project)); err != nil {
				return err
			}
			fmt.Printf("pushed: %s\n", args[0])
			return nil
		},
	}
	pushCmd.Flags().StringVar(&pushMessage, "message", "update from kas", "commit message for dirty changes")
	planCmd.AddCommand(pushCmd)

	// kas task pr
	var prTitle string
	prCmd := &cobra.Command{
		Use:   "pr <plan-file>",
		Short: "push and open a pull request for the task branch",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			repoRoot, project, err := resolveRepoInfo()
			if err != nil {
				return err
			}
			url, err := executeTaskPR(repoRoot, project, args[0], prTitle, resolveStore(project))
			if err != nil {
				return err
			}
			if url != "" {
				fmt.Println(url)
			}
			return nil
		},
	}
	prCmd.Flags().StringVar(&prTitle, "title", "", "PR title (default: task description)")
	planCmd.AddCommand(prCmd)

	// kas task merge
	mergeCmd := &cobra.Command{
		Use:   "merge <plan-file>",
		Short: "merge the task branch into main and transition to done",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			repoRoot, project, err := resolveRepoInfo()
			if err != nil {
				return err
			}
			if err := executeTaskMerge(repoRoot, project, args[0], resolveStore(project)); err != nil {
				return err
			}
			fmt.Printf("merged: %s → done\n", args[0])
			return nil
		},
	}
	planCmd.AddCommand(mergeCmd)

	// kas task start-over
	startOverCmd := &cobra.Command{
		Use:   "start-over <plan-file>",
		Short: "reset the task branch and transition back to planning",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			repoRoot, project, err := resolveRepoInfo()
			if err != nil {
				return err
			}
			if err := executeTaskStartOver(repoRoot, project, args[0], resolveStore(project)); err != nil {
				return err
			}
			fmt.Printf("reset: %s → planning\n", args[0])
			return nil
		},
	}
	planCmd.AddCommand(startOverCmd)

	// kq plan link-clickup
	var linkProject string
	linkClickUpCmd := &cobra.Command{
		Use:   "link-clickup",
		Short: "backfill ClickUp task IDs from task content (parses **Source:** ClickUp <ID> lines)",
		RunE: func(cmd *cobra.Command, args []string) error {
			_, repoProject, err := resolveRepoInfo()
			if err != nil {
				return err
			}
			store, project := resolveStoreConfig(repoProject)
			if store == nil {
				store, err = localSQLiteStore()
				if err != nil {
					return fmt.Errorf("open local task store: %w", err)
				}
				defer store.Close()
				project = repoProject
			}
			if linkProject != "" {
				project = linkProject
			}
			n, err := executeTaskLinkClickUp(project, store)
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
func resolveStoreConfig(project string) (taskstore.Store, string) {
	cfg := config.LoadConfig()
	if cfg.DatabaseURL == "" {
		return nil, ""
	}
	store, err := taskstore.NewStoreFromConfig(cfg.DatabaseURL, project)
	if err != nil || store == nil {
		return nil, ""
	}
	return store, project
}

// localSQLiteStore opens (or creates) the local SQLite task store at the
// canonical path returned by taskstore.ResolvedDBPath(). Used as a fallback
// when no remote store is configured.
func localSQLiteStore() (taskstore.Store, error) {
	dbPath := taskstore.ResolvedDBPath()
	if err := os.MkdirAll(filepath.Dir(dbPath), 0o755); err != nil {
		return nil, fmt.Errorf("create kasmos config dir: %w", err)
	}
	return taskstore.NewSQLiteStore(dbPath)
}

// loadTaskStateByProject loads task state using the store backend and a project name.
// When store is nil, falls back to the local SQLite store.
func loadTaskStateByProject(project string, store taskstore.Store) (*taskstate.TaskState, error) {
	if store == nil {
		var err error
		store, err = localSQLiteStore()
		if err != nil {
			return nil, fmt.Errorf("open local task store: %w", err)
		}
	}
	return taskstate.Load(store, project, "")
}

// newFSMByProject creates a TaskStateMachine backed by the given store and project name.
// When store is nil, falls back to the local SQLite store.
func newFSMByProject(project string, store taskstore.Store) *taskfsm.TaskStateMachine {
	if store == nil {
		var err error
		store, err = localSQLiteStore()
		if err != nil {
			panic("newFSMByProject: open local task store: " + err.Error())
		}
	}
	return taskfsm.New(store, project, "")
}

// resolveStore returns the remote task store from config, or nil if not
// configured or unreachable.
func resolveStore(project string) taskstore.Store {
	store, _ := resolveStoreConfig(project)
	if store != nil && store.Ping() == nil {
		return store
	}
	return nil
}

// resolveRepoInfo resolves the main repository root and derives the project
// name from it. Handles both regular repos and git worktrees. The project name
// is the basename of the repo root directory (e.g. "kasmos" for /home/kas/dev/kasmos).
func resolveRepoInfo() (repoRoot, project string, err error) {
	cwd, err := os.Getwd()
	if err != nil {
		return "", "", fmt.Errorf("get cwd: %w", err)
	}

	root, err := resolveRepoRoot(cwd)
	if err != nil {
		return "", "", fmt.Errorf("cannot resolve repo root: %w", err)
	}
	return root, filepath.Base(root), nil
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
