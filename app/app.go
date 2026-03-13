package app

import (
	"context"
	"errors"
	"fmt"
	"reflect"

	cmd2 "github.com/kastheco/kasmos/cmd"
	"github.com/kastheco/kasmos/config"
	"github.com/kastheco/kasmos/config/auditlog"
	"github.com/kastheco/kasmos/config/taskfsm"
	"github.com/kastheco/kasmos/config/taskparser"
	"github.com/kastheco/kasmos/config/taskstate"
	"github.com/kastheco/kasmos/config/taskstore"
	daemonpkg "github.com/kastheco/kasmos/daemon"
	"github.com/kastheco/kasmos/internal/clickup"
	"github.com/kastheco/kasmos/internal/mcpclient"
	sentrypkg "github.com/kastheco/kasmos/internal/sentry"
	"github.com/kastheco/kasmos/log"
	"github.com/kastheco/kasmos/orchestration"
	"github.com/kastheco/kasmos/orchestration/loop"
	"github.com/kastheco/kasmos/session"
	gitpkg "github.com/kastheco/kasmos/session/git"
	"github.com/kastheco/kasmos/session/tmux"
	"github.com/kastheco/kasmos/ui"
	"github.com/kastheco/kasmos/ui/overlay"
	"os"
	"path/filepath"
	"time"

	"charm.land/bubbles/v2/spinner"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	zone "github.com/lrstanley/bubblezone/v2"
)

const GlobalInstanceLimit = 20

const clickUpOpTimeout = 30 * time.Second

var repoManagedByDaemon = func(repoPath string) bool {
	if repoPath == "" {
		return false
	}

	cfg, err := daemonpkg.LoadDaemonConfig("")
	if err != nil {
		return false
	}
	socketPath := cfg.SocketPath
	if socketPath == "" {
		socketPath = defaultDaemonSocketPath()
	}

	client := daemonpkg.NewSocketClient(socketPath)
	repos, err := client.ListRepos()
	if err != nil {
		return false
	}

	cleanRepoPath := filepath.Clean(repoPath)
	for _, repo := range repos {
		if filepath.Clean(repo.Path) == cleanRepoPath {
			return true
		}
	}
	return false
}

func defaultDaemonSocketPath() string {
	if xdg := os.Getenv("XDG_RUNTIME_DIR"); xdg != "" {
		return filepath.Join(xdg, "kasmos", "kas.sock")
	}
	return filepath.Join(os.TempDir(), fmt.Sprintf("kasmos-%d", os.Getuid()), "kas.sock")
}

// RunOptions holds optional configuration for the Run entrypoint.
// NavOnly causes the TUI to render only the navigation panel, deferring agent
// output to a native tmux pane on the right (the new two-pane layout).
type RunOptions struct {
	NavOnly bool
}

// Run is the main entrypoint into the application.
func Run(ctx context.Context, program string, autoYes bool, version string, opts RunOptions) error {
	// Set the terminal's default background to the theme base color so every
	// ANSI reset and unstyled cell falls back to #232136 instead of black.
	restore := ui.SetTerminalBackground("#232136")
	defer restore()
	defer sentrypkg.RecoverPanic()

	zone.NewGlobal()
	h := newHome(ctx, program, autoYes, version, opts)
	defer h.embeddedServer.Stop()
	defer h.auditLogger.Close()
	if h.permissionStore != nil {
		defer h.permissionStore.Close()
	}
	p := tea.NewProgram(h)
	_, err := p.Run()
	return err
}

type state int

const (
	stateDefault state = iota
	// stateNew is the state when the user is creating a new instance.
	stateNew
	// statePrompt is the state when the user is entering a prompt.
	statePrompt
	// stateHelp is the state when a help screen is displayed.
	stateHelp
	// stateConfirm is the state when a confirmation modal is displayed.
	stateConfirm
	// stateSearch is the state when the user is searching topics/instances.
	stateSearch
	// stateNewPlan is the state when the user is creating a new plan (name + description form).
	stateNewPlan
	// stateNewPlanDeriving is the state when the AI is deriving a plan title.
	stateNewPlanDeriving
	// stateNewPlanTopic is the state when the user is picking a topic for a new plan.
	stateNewPlanTopic
	// stateSpawnAgent is the state when the user is spawning an ad-hoc agent session.
	stateSpawnAgent
	// statePRTitle is the state when the user is entering a PR title.
	statePRTitle
	// statePRBody is the state when the user is editing the PR body/description.
	statePRBody
	// stateRenameInstance is the state when the user is renaming an instance.
	stateRenameInstance
	// stateRenameTask is the state when the user is renaming a plan.
	stateRenameTask
	// stateRenameTopic is the state when the user is renaming a topic.
	stateRenameTopic
	// stateSendPrompt is the state when the user is sending a prompt via text overlay.
	stateSendPrompt
	// stateFocusAgent is the state when the user is typing directly into the agent pane.
	stateFocusAgent
	// stateContextMenu is the state when a right-click context menu is shown.
	stateContextMenu
	// stateChangeTopic is the state when the user is changing a plan's topic via picker.
	stateChangeTopic
	// stateSetStatus is the state when the user is force-overriding a plan's status via picker.
	stateSetStatus
	// stateClickUpSearch is the state when the user is typing a ClickUp search query.
	stateClickUpSearch
	// stateClickUpPicker is the state when the user is picking from ClickUp search results.
	stateClickUpPicker
	// stateClickUpFetching is when kasmos is fetching a full task from ClickUp.
	stateClickUpFetching
	// stateClickUpWorkspacePicker is when the user must pick a ClickUp workspace.
	stateClickUpWorkspacePicker
	// statePermission is when an opencode permission prompt is detected and the modal is shown.
	statePermission
	// stateTmuxBrowser is the state when the tmux session browser overlay is shown.
	stateTmuxBrowser
	// stateChatAboutTask is the state when the user is typing a question about a plan.
	stateChatAboutTask
	// stateAuditCursor is the state when the user is navigating log lines in the
	// audit pane to open per-line context menus.
	stateAuditCursor
)

type home struct {
	ctx context.Context

	// -- Storage and Configuration --

	program string
	version string
	autoYes bool
	// navOnly is true when running as the left tmux pane in the two-pane layout.
	// Later waves use this to suppress the embedded VT preview and status bar.
	navOnly bool

	// activeRepoPath is the currently active repository path for filtering and new instances
	activeRepoPath string

	// storage is the interface for saving/loading data to/from the app's state
	storage *session.Storage
	// appConfig stores persistent application configuration
	appConfig *config.Config
	// appState stores persistent application state like seen help screens
	appState config.AppState

	// -- State --

	// allInstances stores every instance across all repos (master list)
	allInstances []*session.Instance

	// state is the current discrete state of the application
	state state
	// seenNotified tracks an instance whose Notified flag was visible when selected.
	// The flag is cleared when the user navigates away, not immediately on focus.
	seenNotified *session.Instance

	// instanceFinalizers stores per-instance finalizers keyed by instance pointer.
	// Each finalizer registers the repo name after the instance has started.
	// Supports concurrent batch spawns (e.g. wave tasks) where multiple
	// instances start in parallel. Lazily initialized via addInstanceFinalizer.
	instanceFinalizers map[*session.Instance]func()
	// newInstance is the instance currently being named in stateNew.
	// Set when entering stateNew, cleared on Enter/Esc/ctrl+c.
	newInstance *session.Instance

	// promptAfterName tracks if we should enter prompt mode after naming
	promptAfterName bool

	// keySent is used to manage underlining menu items
	keySent bool

	// -- UI Components --

	// nav displays plans + instances
	nav *ui.NavigationPanel
	// auditPane displays recent audit events below the nav panel
	auditPane         *ui.AuditPane
	auditBootstrapped bool // true after first audit query on boot
	// menu displays the bottom menu
	menu *ui.Menu
	// statusBar displays the top contextual status bar
	statusBar *ui.StatusBar
	// tabbedWindow displays the tabbed window with preview and info panes
	tabbedWindow *ui.TabbedWindow
	// toastManager manages toast notifications
	toastManager *overlay.ToastManager
	// global spinner instance. we plumb this down to where it's needed
	spinner spinner.Model
	// overlays manages the single active modal overlay.
	overlays *overlay.Manager
	// pendingConfirmAction stores the tea.Cmd to run asynchronously when confirmed
	pendingConfirmAction tea.Cmd

	// nav handles unified navigation state
	// focusSlot tracks which pane has keyboard focus in the Tab ring:
	// 0=nav, 1=info tab, 2=agent tab
	focusSlot int
	// pendingPlanName stores the plan name during the two-step plan creation flow
	pendingPlanName string
	// pendingPlanDesc stores the plan description during the two-step plan creation flow
	pendingPlanDesc string
	// pendingPRTitle stores the PR title during the two-step PR creation flow
	pendingPRTitle string
	// pendingPRWorktree is a GitWorktree built from taskState for plan-level PR
	// creation flows where no running instance is available. Cleared after use.
	pendingPRWorktree *gitpkg.GitWorktree
	// pendingChangeTopicTask stores the plan filename during the change-topic flow
	pendingChangeTopicTask string
	// pendingSetStatusTask stores the plan filename during the set-status flow
	pendingSetStatusTask string
	// pendingChatAboutTask stores the plan filename during the chat-about-plan flow
	pendingChatAboutTask string
	// pendingLogEvent stores the audit event that triggered the log-action context
	// menu. Consumed by executeContextAction for "log_*" actions.
	pendingLogEvent *ui.AuditEventDisplay
	// pendingPRToastID stores the toast ID for the in-progress PR creation
	pendingPRToastID string
	// pendingAttachInstance is the instance queued for tea.Exec attach after the
	// help overlay is dismissed. Set in the keys.KeyEnter handler; consumed and
	// cleared in handleHelpState once the user acknowledges the attach help screen.
	pendingAttachInstance *session.Instance

	// tmuxSessionCount is the latest count of kas_-prefixed tmux sessions.
	tmuxSessionCount int
	// clickUpConfig stores the detected ClickUp MCP server config (nil if not detected)
	clickUpConfig *clickup.MCPServerConfig
	// clickUpImporter handles search/fetch via MCP (nil until first use)
	clickUpImporter *clickup.Importer
	// clickUpCommenter handles posting progress comments to ClickUp tasks (nil until first use)
	clickUpCommenter *clickup.Commenter
	// clickUpMCPClient is the raw MCP caller shared by importer and commenter
	clickUpMCPClient clickup.MCPCaller
	// clickUpResults stores the latest search results for the picker
	clickUpResults []clickup.SearchResult
	// clickUpPendingQuery stores the search query to retry after workspace selection
	clickUpPendingQuery string
	// clickUpWorkspaceMap maps picker labels ("name (id)") back to bare workspace IDs.
	clickUpWorkspaceMap map[string]string

	// Layout dimensions for mouse hit-testing
	navWidth      int
	tabsWidth     int
	contentHeight int

	// sidebarHidden tracks whether the nav is collapsed (ctrl+s toggle)
	sidebarHidden bool

	// Terminal dimensions for the global background fill.
	termWidth  int
	termHeight int

	// previewTerminal is the VT emulator for the selected instance's preview.
	// Also used for focus mode — entering focus just forwards keys to this terminal.
	previewTerminal         *session.EmbeddedTerminal
	previewTerminalInstance string // title of the instance the terminal is attached to
	previewRequested        bool   // true after the user explicitly selects the live agent pane
	previewClipboardPending bool
	previewClipboardTarget  byte

	// taskState holds the parsed task state from the store for the active repo.
	taskState *taskstate.TaskState
	// taskStateDir is the legacy plans directory path. Retained only for JSON migration.
	// New code should not depend on this path existing on disk.
	taskStateDir string
	// signalsDir is the directory where agent sentinel files are written.
	// Defaults to <repoRoot>/.kasmos/signals/ (project-local, gitignored).
	signalsDir string
	// embeddedServer is the in-process HTTP+SQLite task store server started on boot.
	// Always non-nil after newHome() returns.
	embeddedServer *taskstore.EmbeddedServer
	// taskStore is the task store client. Always non-nil after newHome() returns —
	// points at the embedded server URL unless appConfig.DatabaseURL overrides it.
	taskStore taskstore.Store
	// taskStoreProject is the project name used with the remote store (derived from repo basename).
	taskStoreProject string
	// auditLogger records structured audit events to the planstore SQLite database.
	// Falls back to NopLogger when planstore is HTTP-backed or unconfigured.
	auditLogger auditlog.Logger

	// previewTickCount counts preview ticks for throttled banner animation
	previewTickCount int

	// metadataTickCount counts metadata ticks for throttled PR state polling.
	metadataTickCount int

	// cachedPlanFile is the filename of the last rendered plan (for cache hit).
	cachedPlanFile string
	// cachedPlanRendered is the glamour-rendered markdown of cachedPlanFile.
	cachedPlanRendered string

	// waveOrchestrators tracks active wave orchestrations by plan filename.
	waveOrchestrators map[string]*orchestration.WaveOrchestrator

	// pendingAllComplete holds plan files whose all-waves-complete prompt was
	// deferred because an overlay was active when the orchestrator finished.
	// Drained on each metadata tick once the overlay clears.
	pendingAllComplete []string

	// pendingWaveConfirmTaskFile is set while a wave-advance (or failed-wave decision)
	// confirmation overlay is showing, so cancel can reset the orchestrator latch.
	pendingWaveConfirmTaskFile string
	// waveConfirmDismissedAt is the time the wave confirm dialog was last dismissed
	// via Esc. Used to impose a cooldown before re-showing the dialog.
	waveConfirmDismissedAt time.Time

	// pendingWaveAbortAction is the abort action for a failed-wave decision dialog.
	// Triggered when the user presses 'a' while the failed-wave overlay is active.
	pendingWaveAbortAction tea.Cmd
	// pendingWaveNextAction is the advance action for a failed-wave decision dialog.
	// Triggered when the user presses 'n' (next wave) while the failed-wave overlay is active.
	pendingWaveNextAction tea.Cmd

	// plannerPrompted tracks plan files whose planner-exit dialog has been
	// answered (yes or no). Prevents re-prompting every metadata tick.
	// NOT set on esc — allows re-prompt.
	plannerPrompted map[string]bool

	// coderPushPrompted tracks plan files whose coder-exit push dialog has
	// been answered (yes or no) or dismissed (esc). Prevents the dialog from
	// re-firing on every metadata tick while the coder instance is still in
	// the list. Cleared when a new coder is spawned for the same plan
	// (e.g. via spawnFixerWithFeedback) so the next round can prompt again.
	coderPushPrompted map[string]bool

	// deferredPlannerDialogs holds plan files whose PlannerFinished dialog
	// could not be shown because an overlay was active at signal-processing time.
	// On each metadata tick, any queued plans are shown once the overlay clears.
	deferredPlannerDialogs []string

	// pendingPlannerInstanceTitle is the title of the planner instance that
	// triggered the current planner-exit confirmation dialog.
	pendingPlannerInstanceTitle string

	// pendingPlannerTaskFile is the plan file associated with the planner instance
	// that triggered the current planner-exit confirmation dialog. Set by the
	// PlannerFinished signal handler so cancel/esc handlers can mark plannerPrompted
	// without needing to look up the (possibly already removed) instance by title.
	pendingPlannerTaskFile string

	// fsm is the sole writer of task state. All task status mutations flow
	// through fsm.Transition — direct SetStatus calls are not allowed.
	fsm *taskfsm.TaskStateMachine

	// processor is the signal processing engine that converts FSM sentinel signals
	// into typed Action values. Lazily initialized via ensureProcessor() on first use.
	// Nil when taskStore is not set (e.g. in tests that don't need signal processing).
	processor *loop.Processor
	// daemonStatusChecker verifies that the daemon is reachable and this repo is registered.
	// Nil disables daemon gating, which keeps narrow unit tests lightweight.
	daemonStatusChecker func(string) daemonStatusMsg
	// daemonRepoRegistrar registers the active repo with the daemon on demand.
	// Nil disables in-app repo registration, which keeps narrow unit tests lightweight.
	daemonRepoRegistrar func(string) error

	// pendingReviewFeedback holds review feedback from sentinel files, keyed by
	// plan filename, to be injected as context for the next coder session.
	pendingReviewFeedback map[string]string

	// -- Permission prompt handling --

	// pendingPermissionInstance is the instance that triggered the permission modal.
	pendingPermissionInstance *session.Instance
	// pendingPermissionPattern is the pattern from the active permission overlay.
	// Captured at detection time so it's available after the overlay is dismissed.
	pendingPermissionPattern string
	// pendingPermissionDesc is the description from the active permission overlay.
	// Captured at detection time so it's available after the overlay is dismissed.
	pendingPermissionDesc string
	// permissionStore persists "allow always" decisions in the shared SQLite database.
	permissionStore config.PermissionStore
	// permissionHandled tracks in-flight auto-approvals: instance → pattern.
	// Prevents duplicate key sequences when the pane still shows the prompt
	// across multiple metadata ticks while opencode processes the first response.
	// Cleared when the pane no longer contains a permission prompt for that instance.
	permissionHandled map[*session.Instance]string
}

func newHome(ctx context.Context, program string, autoYes bool, version string, opts RunOptions) *home {
	// Load application config
	appConfig := config.LoadConfig()

	// Load application state
	appState := config.LoadState()

	// Initialize storage
	storage, err := session.NewStorage(appState)
	if err != nil {
		fmt.Printf("Failed to initialize storage: %v\n", err)
		os.Exit(1)
	}

	activeRepoPath, err := filepath.Abs(".")
	if err != nil {
		fmt.Printf("Failed to get current directory: %v\n", err)
		os.Exit(1)
	}

	project := filepath.Base(activeRepoPath)
	h := &home{
		ctx:                   ctx,
		spinner:               spinner.New(spinner.WithSpinner(spinner.Dot)),
		menu:                  ui.NewMenu(),
		auditPane:             ui.NewAuditPane(),
		statusBar:             ui.NewStatusBar(),
		tabbedWindow:          ui.NewTabbedWindow(ui.NewPreviewPane(), ui.NewInfoPane()),
		storage:               storage,
		appConfig:             appConfig,
		program:               program,
		version:               version,
		autoYes:               autoYes,
		navOnly:               opts.NavOnly,
		state:                 stateDefault,
		appState:              appState,
		activeRepoPath:        activeRepoPath,
		taskStateDir:          filepath.Join(activeRepoPath, "docs", "plans"), // legacy: only for JSON migration
		signalsDir:            filepath.Join(activeRepoPath, ".kasmos", "signals"),
		taskStoreProject:      project,
		daemonStatusChecker:   checkDaemonStatus,
		daemonRepoRegistrar:   registerRepoWithDaemon,
		instanceFinalizers:    make(map[*session.Instance]func()),
		waveOrchestrators:     make(map[string]*orchestration.WaveOrchestrator),
		plannerPrompted:       make(map[string]bool),
		coderPushPrompted:     make(map[string]bool),
		pendingReviewFeedback: make(map[string]string),
	}

	// Always start an embedded task store server. This gives us a local SQLite
	// DB as the single source of truth without requiring a separate process.
	dbPath := taskstore.ResolvedDBPath()
	embSrv, err := taskstore.StartEmbedded(dbPath, 0)
	if err != nil {
		fmt.Printf("Failed to start embedded task store: %v\n", err)
		os.Exit(1)
	}
	h.embeddedServer = embSrv

	// Default: use the embedded server's URL for the task store client.
	storeURL := embSrv.URL()
	remoteStoreUnreachable := false

	// If a remote task store is configured, use that URL instead (multi-machine
	// access over tailscale, etc.). The embedded server still runs for audit log
	// DB access via its SQLite store.
	if appConfig.DatabaseURL != "" {
		remoteStore := taskstore.NewHTTPStore(appConfig.DatabaseURL, project)
		if pingErr := remoteStore.Ping(); pingErr != nil {
			log.WarningLog.Printf("remote task store unreachable: %v — falling back to embedded", pingErr)
			remoteStoreUnreachable = true
			// storeURL stays as the embedded server URL
		} else {
			storeURL = appConfig.DatabaseURL
		}
	}

	h.taskStore = taskstore.NewHTTPStore(storeURL, project)
	h.fsm = taskfsm.New(h.taskStore, project, h.taskStateDir)

	// One-time migration: import plan-state.json into the DB if it exists.
	// Use the embedded store directly (bypasses HTTP round-trip).
	// Only runs when plan-state.json is present; subsequent boots skip this.
	planStateJSON := filepath.Join(h.taskStateDir, "plan-state.json")
	if _, statErr := os.Stat(planStateJSON); statErr == nil {
		migrated, migrateErr := taskstore.MigrateFromJSON(embSrv.Store(), project, h.taskStateDir)
		if migrateErr != nil {
			log.WarningLog.Printf("plan-state.json migration failed: %v", migrateErr)
		} else {
			log.InfoLog.Printf("migrated %d plans from plan-state.json to DB", migrated)
			if renameErr := os.Rename(planStateJSON, planStateJSON+".migrated"); renameErr != nil {
				log.WarningLog.Printf("failed to rename plan-state.json after migration: %v", renameErr)
			}
		}
	}

	// Initialize audit logger. Always uses local SQLite regardless of plan
	// store backend — audit events are purely local state.
	if al, err := auditlog.NewSQLiteLogger(dbPath); err != nil {
		log.WarningLog.Printf("audit logger init failed: %v", err)
		h.auditLogger = auditlog.NopLogger()
	} else {
		h.auditLogger = al
	}

	h.nav = ui.NewNavigationPanel(&h.spinner)
	h.toastManager = overlay.NewToastManager(&h.spinner)
	h.overlays = overlay.NewManager()

	// Show a warning toast if a remote task store was configured but unreachable
	// (we fell back to the embedded server).
	if remoteStoreUnreachable {
		h.toastManager.Error("remote task store unreachable — using embedded store")
	}

	permCacheDir := filepath.Join(activeRepoPath, ".kasmos")
	permStore, err := config.NewSQLitePermissionStore(dbPath)
	if err != nil {
		log.WarningLog.Printf("permission store init failed: %v", err)
	} else {
		if migrateErr := config.MigratePermissionCache(permCacheDir, project, permStore); migrateErr != nil {
			log.WarningLog.Printf("permission cache migration failed: %v", migrateErr)
		}
		h.permissionStore = permStore
	}
	h.permissionHandled = make(map[*session.Instance]string)

	h.tabbedWindow.SetAnimateBanner(appConfig.AnimateBanner)
	h.setFocusSlot(slotNav)
	h.loadTaskState()

	// Load saved instances
	instances, err := storage.LoadInstances()
	if err != nil {
		fmt.Printf("Failed to load instances: %v\n", err)
		os.Exit(1)
	}

	h.allInstances = instances

	// Add instances matching active repo to the nav
	for _, instance := range instances {
		repoPath := instance.GetRepoPath()
		if repoPath == "" || repoPath == h.activeRepoPath {
			h.nav.AddInstance(instance)()
			if autoYes {
				instance.AutoYes = true
			}
		}
	}

	h.updateSidebarTasks()

	// Reconstruct in-memory wave orchestrators for plans that were mid-wave
	// when kasmos was last restarted. Must run after loadTaskState and instance load.
	h.rebuildOrphanedOrchestrators()

	return h
}

// activeProject returns the project name derived from the active repo path.
// This matches how planstore derives the project name (filepath.Base of the repo path).
func (m *home) activeProject() string {
	return filepath.Base(m.activeRepoPath)
}

// isUserInOverlay returns true when the user is actively interacting with
// any modal overlay. Used to prevent async metadata-tick handlers from
// clobbering the active overlay by showing a confirmation dialog.
func (m *home) isUserInOverlay() bool {
	switch m.state {
	case stateDefault:
		return false
	}
	return true
}

// exitFocusModeForDialog exits focus/interactive mode if active, so that an
// incoming dialog (permission prompt, wave completion, planner-finished, etc.)
// can be displayed immediately. Focus mode is not a real overlay — it just
// forwards keys to the embedded PTY — so it is safe to interrupt.
func (m *home) exitFocusModeForDialog() {
	if m.state == stateFocusAgent {
		m.exitFocusMode()
	}
}

// updateHandleWindowSizeEvent sets the sizes of the components.
// The components will try to render inside their bounds.
func (m *home) updateHandleWindowSizeEvent(msg tea.WindowSizeMsg) {
	// Two-column layout: nav + preview
	var navWidth int
	if m.sidebarHidden {
		navWidth = 0
	} else {
		navWidth = msg.Width * 30 / 100
		if navWidth < 25 {
			navWidth = 25
		}
	}
	tabsWidth := msg.Width - navWidth

	// Keep the keybind rail compact and give the saved rows to the three columns.
	menuHeight := 1
	if msg.Height < 2 {
		menuHeight = 0
	}
	statusBarHeight := 1
	contentHeight := msg.Height - menuHeight - statusBarHeight
	if contentHeight < 1 {
		contentHeight = 1
	}
	// Detect actual terminal resize vs spurious tea.RequestWindowSize side-effects.
	termResized := msg.Width != m.termWidth || msg.Height != m.termHeight

	m.termWidth = msg.Width
	m.termHeight = msg.Height
	m.toastManager.SetSize(msg.Width, msg.Height)
	if m.statusBar != nil {
		m.statusBar.SetSize(msg.Width)
	}

	m.tabbedWindow.SetSize(tabsWidth, contentHeight)

	// Nav panel gets full content height — audit pane is rendered inside its border.
	m.nav.SetSize(navWidth, contentHeight)
	if m.auditPane != nil && m.auditPane.Visible() && navWidth > 0 {
		// Size audit pane for the nav panel's inner content area.
		// border (2) + border padding (2) + item padding (2) = 6
		auditInnerW := navWidth - 6
		// Pass full content height — the nav panel clamps to whatever space
		// remains below the list content (active/plans sections + legend).
		auditH := contentHeight
		if !m.auditBootstrapped {
			m.refreshAuditPane() // load historical events on first render
			m.auditBootstrapped = true
		}
		m.auditPane.SetSize(auditInnerW, auditH)
		m.nav.SetAuditView(m.auditPane.String(), m.auditPane.ContentLines())
	} else {
		m.nav.SetAuditView("", 0)
		if m.auditPane != nil {
			m.auditPane.SetSize(0, 0)
		}
	}

	// Store for mouse hit-testing
	m.navWidth = navWidth
	m.tabsWidth = tabsWidth
	m.contentHeight = contentHeight

	if navWidth == 0 && m.focusSlot == slotNav {
		m.setFocusSlot(slotAgent)
	}

	// Only resize overlays when the terminal dimensions actually changed.
	// Many handlers emit tea.RequestWindowSize as a batched side-effect (e.g.
	// instanceStartedMsg) — those fire with the same dimensions and should
	// not overwrite the overlay's explicit sizing.
	if termResized {
		m.overlays.SetSize(msg.Width, msg.Height)
	}

	previewWidth, previewHeight := m.tabbedWindow.GetPreviewSize()
	if m.previewTerminal != nil {
		m.previewTerminal.Resize(previewWidth, previewHeight)
	}
	if err := m.nav.SetSessionPreviewSize(previewWidth, previewHeight); err != nil {
		log.ErrorLog.Print(err)
	}
	m.menu.SetSize(msg.Width, menuHeight)
}

func (m *home) Init() tea.Cmd {
	m.audit(auditlog.EventSessionStarted, "kasmos started")

	// Upon starting, we want to start the spinner. Whenever we get a spinner.TickMsg, we
	// update the spinner, which sends a new spinner.TickMsg. I think this lasts forever lol.
	return tea.Batch(
		m.spinner.Tick,
		func() tea.Msg {
			time.Sleep(50 * time.Millisecond)
			return previewTickMsg{}
		},
		tickUpdateMetadataCmd,
		m.toastTickCmd(),
		m.daemonStartupCheckCmd(),
		detectClickUpCmd(m.activeRepoPath),
	)
}

func (m *home) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case overlay.ToastTickMsg:
		m.toastManager.Tick()
		if m.toastManager.HasActiveToasts() {
			return m, m.toastTickCmd()
		}
		return m, nil
	case prCreatedMsg:
		m.toastManager.Resolve(m.pendingPRToastID, overlay.ToastSuccess, "PR created!")
		m.pendingPRToastID = ""
		m.audit(auditlog.EventPRCreated, fmt.Sprintf("PR created: %s", msg.prTitle),
			auditlog.WithInstance(msg.instanceTitle),
		)
		return m, m.toastTickCmd()
	case daemonStatusMsg:
		if !msg.ready {
			m.showDaemonRequiredDialog(msg)
		}
		return m, nil
	case daemonRepoRegisteredMsg:
		m.toastManager.Success(fmt.Sprintf("registered repo with daemon: %s", msg.path))
		return m, m.toastTickCmd()
	case prErrorMsg:
		log.ErrorLog.Printf("%v", msg.err)
		m.toastManager.Resolve(msg.id, overlay.ToastError, msg.err.Error())
		m.pendingPRToastID = ""
		return m, m.toastTickCmd()
	case prCreatedForPlanMsg:
		if msg.url != "" && m.taskStore != nil {
			if err := m.taskStore.SetPRURL(m.taskStoreProject, msg.planFile, msg.url); err != nil {
				log.WarningLog.Printf("prCreatedForPlanMsg: could not persist PR URL for %q: %v", msg.planFile, err)
			}
		}
		m.loadTaskState()
		m.updateInfoPane()
		planName := taskstate.DisplayName(msg.planFile)
		m.toastManager.Success(fmt.Sprintf("pr created for '%s'", planName))
		return m, m.toastTickCmd()
	case planRenderedMsg:
		if msg.err != nil {
			return m, m.handleError(msg.err)
		}
		m.cachedPlanFile = msg.planFile
		m.cachedPlanRendered = msg.rendered
		m.previewRequested = false
		m.tabbedWindow.SetActiveTab(ui.PreviewTab)
		m.tabbedWindow.SetDocumentContent(msg.rendered)
		return m, nil
	case previewTickMsg:
		// If previewTerminal is active, render from it (zero-latency VT emulator).
		if m.previewTerminal != nil && !m.tabbedWindow.IsDocumentMode() {
			if content, changed := m.previewTerminal.Render(); changed {
				m.tabbedWindow.SetPreviewContent(content)
			}
			if !m.previewClipboardPending {
				if selection, ok := m.previewTerminal.PollClipboardRequest(); ok {
					m.previewClipboardPending = true
					m.previewClipboardTarget = selection
					term := m.previewTerminal
					return m, tea.Batch(nextPreviewTickCmd(term), readClipboardCmd(selection))
				}
			}
		} else if m.previewTerminal == nil && !m.tabbedWindow.IsDocumentMode() {
			// No terminal — show appropriate fallback state.
			selected := m.nav.GetSelectedInstance()
			if m.shouldAttachPreviewTerminal(selected) {
				// Instance is running but terminal hasn't attached yet — show connecting indicator.
				m.tabbedWindow.SetConnectingState()
			} else {
				// nil, Loading, or Paused — delegate to UpdatePreview which renders the
				// correct fallback (banner, progress bar, or paused message).
				if err := m.tabbedWindow.UpdatePreview(selected); err != nil {
					log.ErrorLog.Printf("preview update error: %v", err)
				}
			}
		}
		// Advance spring animation every tick (20fps)
		m.tabbedWindow.TickSpring()
		// Banner animation (only when no terminal is active / fallback showing).
		m.previewTickCount++
		if m.previewTickCount%20 == 0 {
			m.tabbedWindow.TickBanner()
		}
		// Use event-driven wakeup when terminal is live, fall back to 50ms poll otherwise.
		term := m.previewTerminal
		return m, nextPreviewTickCmd(term)
	case keyupMsg:
		m.menu.ClearKeydown()
		return m, nil
	case clickUpDetectedMsg:
		m.clickUpConfig = &msg.Config
		m.nav.SetClickUpAvailable(true)
		return m, nil
	case clickUpSearchResultMsg:
		if msg.Err != nil {
			// Check if the error is a multiple-workspaces error — show picker instead of failing.
			var mwErr *clickup.MultipleWorkspacesError
			if errors.As(msg.Err, &mwErr) && len(mwErr.WorkspaceIDs) > 0 {
				m.clickUpPendingQuery = msg.Query
				m.state = stateClickUpWorkspacePicker
				// Build picker labels: "name (id)" when names are available, bare id otherwise.
				items := make([]string, len(mwErr.WorkspaceIDs))
				m.clickUpWorkspaceMap = make(map[string]string, len(mwErr.WorkspaceIDs))
				for i, id := range mwErr.WorkspaceIDs {
					if name, ok := mwErr.WorkspaceNames[id]; ok && name != "" {
						label := name + " (" + id + ")"
						items[i] = label
						m.clickUpWorkspaceMap[label] = id
					} else {
						items[i] = id
						m.clickUpWorkspaceMap[id] = id
					}
				}
				m.overlays.Show(overlay.NewPickerOverlay("select clickup workspace", items))
				return m, nil
			}
			m.toastManager.Error("clickup search failed: " + msg.Err.Error())
			m.state = stateDefault
			return m, m.toastTickCmd()
		}
		if len(msg.Results) == 0 {
			m.toastManager.Info("no clickup tasks found")
			m.state = stateDefault
			return m, m.toastTickCmd()
		}
		m.clickUpResults = msg.Results
		items := make([]string, len(msg.Results))
		for i, r := range msg.Results {
			label := r.ID + " · " + r.Name
			if r.Status != "" {
				label += " (" + r.Status + ")"
			}
			if r.ListName != "" {
				label += " — " + r.ListName
			}
			items[i] = label
		}
		m.state = stateClickUpPicker
		m.overlays.Show(overlay.NewPickerOverlay("select clickup task", items))
		return m, nil
	case tickUpdateMetadataMessage:
		// Snapshot the instance list for the goroutine. The slice header is
		// copied but the pointers are shared — CollectMetadata only reads
		// instance fields that don't change between ticks (started, Status,
		// tmuxSession, gitWorktree, Program).
		instances := m.nav.GetInstances()
		snapshots := make([]*session.Instance, len(instances))
		copy(snapshots, instances)
		taskStateDir := m.taskStateDir // snapshot for goroutine
		signalsDir := m.signalsDir     // snapshot for goroutine
		store := m.taskStore           // snapshot for goroutine
		project := m.taskStoreProject  // snapshot for goroutine
		repoPath := m.activeRepoPath   // snapshot for goroutine
		m.metadataTickCount++
		tickCount := m.metadataTickCount // capture by value for goroutine

		return m, func() tea.Msg {
			results := make([]instanceMetadata, 0, len(snapshots))
			for _, inst := range snapshots {
				if !inst.Started() || inst.Paused() {
					continue
				}
				md := inst.CollectMetadata()
				results = append(results, instanceMetadata{
					Title:              inst.Title,
					Content:            md.Content,
					ContentCaptured:    md.ContentCaptured,
					Updated:            md.Updated,
					HasPrompt:          md.HasPrompt,
					CPUPercent:         md.CPUPercent,
					MemMB:              md.MemMB,
					ResourceUsageValid: md.ResourceUsageValid,
					TmuxAlive:          md.TmuxAlive,
					PermissionPrompt:   md.PermissionPrompt,
				})
			}

			// Load plan state — moved here from the synchronous Update handler
			// to avoid blocking the event loop every 500ms.
			// Always reads from the store (embedded or remote) — no JSON fallback.
			var ps *taskstate.TaskState
			if taskStateDir != "" {
				var loaded *taskstate.TaskState
				var err error
				loaded, err = taskstate.Load(store, project, taskStateDir)
				if err != nil {
					log.WarningLog.Printf("could not load plan state: %v", err)
				} else {
					ps = loaded
				}
			}

			daemonManagedRepo := repoManagedByDaemon(repoPath)

			// Scan signals from the project-local signals directory (.kasmos/signals/).
			var signals []taskfsm.Signal
			if signalsDir != "" && !daemonManagedRepo {
				signals = taskfsm.ScanSignals(signalsDir)
			}

			var taskSignals []taskfsm.TaskSignal
			if signalsDir != "" && !daemonManagedRepo {
				taskSignals = taskfsm.ScanTaskSignals(signalsDir)
			}

			var elaborationSignals []taskfsm.ElaborationSignal
			if signalsDir != "" && !daemonManagedRepo {
				elaborationSignals = taskfsm.ScanElaborationSignals(signalsDir)
			}

			// Also scan signals from active worktrees — agents write
			// sentinel files relative to their CWD which is the worktree,
			// not the main repo. Worktrees use .kasmos/signals/ as well.
			seen := make(map[string]bool)
			for _, sig := range signals {
				seen[sig.Key()] = true
			}
			seenTaskSignals := make(map[string]bool)
			for _, ts := range taskSignals {
				seenTaskSignals[ts.Key()] = true
			}
			seenElabSignals := make(map[string]bool)
			for _, es := range elaborationSignals {
				seenElabSignals[es.TaskFile] = true
			}
			if !daemonManagedRepo {
				for _, inst := range snapshots {
					wt := inst.GetWorktreePath()
					if wt == "" {
						continue
					}
					wtSignalsDir := filepath.Join(wt, ".kasmos", "signals")
					for _, sig := range taskfsm.ScanSignals(wtSignalsDir) {
						if !seen[sig.Key()] {
							seen[sig.Key()] = true
							signals = append(signals, sig)
						}
					}
					for _, ts := range taskfsm.ScanTaskSignals(wtSignalsDir) {
						if !seenTaskSignals[ts.Key()] {
							seenTaskSignals[ts.Key()] = true
							taskSignals = append(taskSignals, ts)
						}
					}
					for _, es := range taskfsm.ScanElaborationSignals(wtSignalsDir) {
						if !seenElabSignals[es.TaskFile] {
							seenElabSignals[es.TaskFile] = true
							elaborationSignals = append(elaborationSignals, es)
						}
					}
				}
			}

			var waveSignals []taskfsm.WaveSignal
			if signalsDir != "" && !daemonManagedRepo {
				waveSignals = taskfsm.ScanWaveSignals(signalsDir)
			}

			tmuxCount := tmux.CountKasSessions(cmd2.MakeExecutor())

			// Periodically poll PR state for plans that have a PR URL.
			// Poll every 10th tick (~2s) to avoid hammering the GitHub API.
			var prStateUpdates []prStateUpdateMsg
			if tickCount%10 == 0 && store != nil {
				if entries, err := store.List(project); err == nil {
					for _, entry := range entries {
						if entry.PRURL == "" || entry.Branch == "" {
							continue
						}
						shared := gitpkg.NewSharedTaskWorktree(repoPath, entry.Branch)
						state, err := shared.QueryPRState()
						if err != nil {
							log.WarningLog.Printf("PR poll: QueryPRState for %q: %v", entry.Filename, err)
							continue
						}
						if state.URL == "" {
							continue
						}
						prStateUpdates = append(prStateUpdates, prStateUpdateMsg{
							planFile:       entry.Filename,
							reviewDecision: mapPRReviewDecision(state.ReviewDecision),
							checkStatus:    mapPRCheckStatus(state.CheckStatus),
						})
					}
				}
			}

			time.Sleep(200 * time.Millisecond)
			return metadataResultMsg{Results: results, PlanState: ps, Signals: signals, TaskSignals: taskSignals, WaveSignals: waveSignals, ElaborationSignals: elaborationSignals, DaemonManagedRepo: daemonManagedRepo, TmuxSessionCount: tmuxCount, PRStateUpdates: prStateUpdates}
		}
	case metadataResultMsg:
		// Process agent sentinel signals — feed to FSM and consume sentinel files.
		// Done in Update (main goroutine) so FSM writes are never concurrent.
		// Side-effect cmds (reviewer/coder spawns) are collected and batched below.
		var signalCmds []tea.Cmd
		if !msg.DaemonManagedRepo {
			// Process FSM signals using the Processor when available, falling back to
			// legacy inline code for home instances that don't have a taskStore
			// (e.g. tests that build home without a store).
			proc := m.ensureProcessor()
			if proc != nil {
				for planFile := range m.waveOrchestrators {
					proc.SetWaveOrchestratorActive(planFile, true)
				}

				feedbackBeforeTick := make(map[string]bool, len(m.pendingReviewFeedback))
				for planFile := range m.pendingReviewFeedback {
					feedbackBeforeTick[planFile] = true
				}

				actions := proc.ProcessFSMSignals(msg.Signals)
				for _, sig := range msg.Signals {
					taskfsm.ConsumeSignal(sig)
				}

				for _, act := range actions {
					switch a := act.(type) {
					case loop.SpawnReviewerAction:
						for _, inst := range m.nav.GetInstances() {
							if inst.TaskFile == a.PlanFile && (inst.AgentType == session.AgentTypeCoder || inst.AgentType == session.AgentTypeFixer) {
								inst.ImplementationComplete = true
								_ = inst.Pause()
								break
							}
						}
						if feedbackBeforeTick[a.PlanFile] {
							delete(m.pendingReviewFeedback, a.PlanFile)
							if cmd := m.postClickUpProgress(a.PlanFile, "fixer_complete", ""); cmd != nil {
								signalCmds = append(signalCmds, cmd)
							}
						}
						if cmd := m.spawnReviewer(a.PlanFile); cmd != nil {
							signalCmds = append(signalCmds, cmd)
						}
					case loop.ReviewApprovedAction:
						planName := taskstate.DisplayName(a.PlanFile)
						m.audit(auditlog.EventPlanTransition, "reviewing → done (review approved)",
							auditlog.WithPlan(a.PlanFile))
						m.toastManager.Success(fmt.Sprintf("review approved: %s", planName))
						if cmd := m.postClickUpProgress(a.PlanFile, "review_approved", ""); cmd != nil {
							signalCmds = append(signalCmds, cmd)
						}
						for _, inst := range m.nav.GetInstances() {
							if inst.TaskFile == a.PlanFile && inst.IsReviewer {
								inst.SetStatus(session.Paused)
								m.nav.SelectInstance(inst)
								m.updateNavPanelStatus()
								if cmd := m.instanceChanged(); cmd != nil {
									signalCmds = append(signalCmds, cmd)
								}
								break
							}
						}
					case loop.CreatePRAction:
						signalCmds = append(signalCmds, m.createPRAfterApproval(a.PlanFile, a.ReviewBody))
					case loop.ReviewChangesAction:
						if cmd := m.handleReviewChangesRequested(a.PlanFile, a.Feedback); cmd != nil {
							signalCmds = append(signalCmds, cmd)
						}
					case loop.IncrementReviewCycleAction:
						if err := m.taskState.IncrementReviewCycle(a.PlanFile); err != nil {
							log.WarningLog.Printf("could not increment review cycle for %q: %v", a.PlanFile, err)
						}
					case loop.SpawnCoderAction:
						if cmd := m.spawnFixerWithFeedback(a.PlanFile, a.Feedback); cmd != nil {
							signalCmds = append(signalCmds, cmd)
						}
					case loop.ReviewCycleLimitAction:
						planName := taskstate.DisplayName(a.PlanFile)
						m.toastManager.Error(fmt.Sprintf(
							"review-fix loop stopped: cycle limit reached (%d/%d) for %s",
							a.Cycle, a.Limit, planName))
						m.audit(auditlog.EventPlanTransition,
							fmt.Sprintf("review-fix cycle limit reached (%d/%d)", a.Cycle, a.Limit),
							auditlog.WithPlan(a.PlanFile))
					case loop.PlannerCompleteAction:
						capturedPlanFile := a.PlanFile
						{
							summary := ""
							if m.taskStore != nil {
								if content, err := m.taskStore.GetContent(m.taskStoreProject, capturedPlanFile); err == nil {
									if plan, err := taskparser.Parse(content); err == nil {
										totalTasks := 0
										for _, w := range plan.Waves {
											totalTasks += len(w.Tasks)
										}
										summary = fmt.Sprintf("%d tasks, %d waves", totalTasks, len(plan.Waves))
									}
								}
							}
							if cmd := m.postClickUpProgress(capturedPlanFile, "plan_ready", summary); cmd != nil {
								signalCmds = append(signalCmds, cmd)
							}
						}
						if m.plannerPrompted[capturedPlanFile] {
							continue
						}
						m.exitFocusModeForDialog()
						if m.isUserInOverlay() {
							m.deferredPlannerDialogs = append(m.deferredPlannerDialogs, capturedPlanFile)
							continue
						}
						for _, inst := range m.nav.GetInstances() {
							if inst.TaskFile == capturedPlanFile && inst.AgentType == session.AgentTypePlanner {
								if cmd := m.focusInstanceForOverlay(inst); cmd != nil {
									signalCmds = append(signalCmds, cmd)
								}
								m.pendingPlannerInstanceTitle = inst.Title
								break
							}
						}
						m.pendingPlannerTaskFile = capturedPlanFile
						m.confirmAction(
							fmt.Sprintf("task '%s' is ready. start implementation?", taskstate.DisplayName(capturedPlanFile)),
							func() tea.Msg {
								return plannerCompleteMsg{planFile: capturedPlanFile}
							},
						)
					}
				}
			} else {
				for _, sig := range msg.Signals {
					if sig.Event == taskfsm.ImplementFinished {
						if _, hasOrch := m.waveOrchestrators[sig.TaskFile]; hasOrch {
							log.WarningLog.Printf("ignoring implement-finished signal for %q — wave orchestrator active", sig.TaskFile)
							taskfsm.ConsumeSignal(sig)
							continue
						}
					}

					if err := m.fsm.Transition(sig.TaskFile, sig.Event); err != nil {
						log.WarningLog.Printf("signal %s for %s rejected: %v", sig.Event, sig.TaskFile, err)
						taskfsm.ConsumeSignal(sig)
						continue
					}
					taskfsm.ConsumeSignal(sig)

					switch sig.Event {
					case taskfsm.ImplementFinished:
						for _, inst := range m.nav.GetInstances() {
							if inst.TaskFile == sig.TaskFile && (inst.AgentType == session.AgentTypeCoder || inst.AgentType == session.AgentTypeFixer) {
								inst.ImplementationComplete = true
								_ = inst.Pause()
								break
							}
						}
						if _, hasFeedback := m.pendingReviewFeedback[sig.TaskFile]; hasFeedback {
							delete(m.pendingReviewFeedback, sig.TaskFile)
							if cmd := m.postClickUpProgress(sig.TaskFile, "fixer_complete", ""); cmd != nil {
								signalCmds = append(signalCmds, cmd)
							}
						}
						if cmd := m.spawnReviewer(sig.TaskFile); cmd != nil {
							signalCmds = append(signalCmds, cmd)
						}
					case taskfsm.ReviewApproved:
						planName := taskstate.DisplayName(sig.TaskFile)
						m.audit(auditlog.EventPlanTransition, "reviewing → done (review approved)",
							auditlog.WithPlan(sig.TaskFile))
						m.toastManager.Success(fmt.Sprintf("review approved: %s", planName))
						if cmd := m.postClickUpProgress(sig.TaskFile, "review_approved", ""); cmd != nil {
							signalCmds = append(signalCmds, cmd)
						}
						for _, inst := range m.nav.GetInstances() {
							if inst.TaskFile == sig.TaskFile && inst.IsReviewer {
								inst.SetStatus(session.Paused)
								m.nav.SelectInstance(inst)
								m.updateNavPanelStatus()
								if cmd := m.instanceChanged(); cmd != nil {
									signalCmds = append(signalCmds, cmd)
								}
								break
							}
						}
						if m.taskStore != nil {
							if entry, err := m.taskStore.Get(m.taskStoreProject, sig.TaskFile); err == nil {
								if shouldCreatePR(entry) {
									signalCmds = append(signalCmds, m.createPRAfterApproval(sig.TaskFile, sig.Body))
								}
							}
						}
					case taskfsm.ReviewChangesRequested:
						feedback := sig.Body
						if cmd := m.handleReviewChangesRequested(sig.TaskFile, feedback); cmd != nil {
							signalCmds = append(signalCmds, cmd)
						}
						if m.appConfig == nil || !m.appConfig.AutoReviewFix {
							break
						}
						if m.appConfig != nil && m.appConfig.MaxReviewFixCycles > 0 {
							if cycle, err := m.taskState.ReviewCycle(sig.TaskFile); err == nil {
								if cycle+1 > m.appConfig.MaxReviewFixCycles {
									planName := taskstate.DisplayName(sig.TaskFile)
									m.toastManager.Error(fmt.Sprintf(
										"review-fix loop stopped: cycle limit reached (%d/%d) for %s",
										cycle+1, m.appConfig.MaxReviewFixCycles, planName))
									m.audit(auditlog.EventPlanTransition,
										fmt.Sprintf("review-fix cycle limit reached (%d/%d)", cycle+1, m.appConfig.MaxReviewFixCycles),
										auditlog.WithPlan(sig.TaskFile))
									continue // skip spawning fixer
								}
							}
						}
						if err := m.taskState.IncrementReviewCycle(sig.TaskFile); err != nil {
							log.WarningLog.Printf("could not increment review cycle for %q: %v", sig.TaskFile, err)
						}
						if cmd := m.spawnFixerWithFeedback(sig.TaskFile, feedback); cmd != nil {
							signalCmds = append(signalCmds, cmd)
						}
					case taskfsm.PlannerFinished:
						capturedPlanFile := sig.TaskFile
						{
							summary := ""
							if m.taskStore != nil {
								if content, err := m.taskStore.GetContent(m.taskStoreProject, capturedPlanFile); err == nil {
									if plan, err := taskparser.Parse(content); err == nil {
										totalTasks := 0
										for _, w := range plan.Waves {
											totalTasks += len(w.Tasks)
										}
										summary = fmt.Sprintf("%d tasks, %d waves", totalTasks, len(plan.Waves))
									}
								}
							}
							if cmd := m.postClickUpProgress(capturedPlanFile, "plan_ready", summary); cmd != nil {
								signalCmds = append(signalCmds, cmd)
							}
						}
						if m.plannerPrompted[capturedPlanFile] {
							break
						}
						m.exitFocusModeForDialog()
						if m.isUserInOverlay() {
							m.deferredPlannerDialogs = append(m.deferredPlannerDialogs, capturedPlanFile)
							break
						}
						for _, inst := range m.nav.GetInstances() {
							if inst.TaskFile == sig.TaskFile && inst.AgentType == session.AgentTypePlanner {
								if cmd := m.focusInstanceForOverlay(inst); cmd != nil {
									signalCmds = append(signalCmds, cmd)
								}
								m.pendingPlannerInstanceTitle = inst.Title
								break
							}
						}
						m.pendingPlannerTaskFile = capturedPlanFile
						m.confirmAction(
							fmt.Sprintf("task '%s' is ready. start implementation?", taskstate.DisplayName(capturedPlanFile)),
							func() tea.Msg {
								return plannerCompleteMsg{planFile: capturedPlanFile}
							},
						)
					}
				}
			}

			for _, ts := range msg.TaskSignals {
				orch, exists := m.waveOrchestrators[ts.TaskFile]
				if !exists {
					log.WarningLog.Printf("ignoring task-finished signal for %q — no active wave orchestrator", ts.TaskFile)
					taskfsm.ConsumeTaskSignal(ts)
					continue
				}
				if ts.WaveNumber != orch.CurrentWaveNumber() {
					log.WarningLog.Printf("ignoring task-finished signal for %q wave %d — active wave is %d", ts.TaskFile, ts.WaveNumber, orch.CurrentWaveNumber())
					taskfsm.ConsumeTaskSignal(ts)
					continue
				}
				if !orch.IsTaskRunning(ts.TaskNumber) {
					taskfsm.ConsumeTaskSignal(ts)
					continue
				}

				orch.MarkTaskComplete(ts.TaskNumber)
				for _, inst := range m.nav.GetInstances() {
					if inst.TaskFile != ts.TaskFile || inst.TaskNumber != ts.TaskNumber || inst.WaveNumber != ts.WaveNumber {
						continue
					}
					inst.ImplementationComplete = true
					inst.SetStatus(session.Ready)
					break
				}
				taskfsm.ConsumeTaskSignal(ts)
			}

			if len(msg.Signals) > 0 || len(msg.TaskSignals) > 0 {
				m.loadTaskState() // refresh after signal processing
			}

			// Retry deferred PlannerFinished dialogs — show the first queued plan
			// whose dialog was skipped because an overlay was active at signal time.
			if len(m.deferredPlannerDialogs) > 0 {
				m.exitFocusModeForDialog()
			}
			if len(m.deferredPlannerDialogs) > 0 && !m.isUserInOverlay() {
				planFile := m.deferredPlannerDialogs[0]
				m.deferredPlannerDialogs = m.deferredPlannerDialogs[1:]
				if !m.plannerPrompted[planFile] {
					for _, inst := range m.nav.GetInstances() {
						if inst.TaskFile == planFile && inst.AgentType == session.AgentTypePlanner {
							if cmd := m.focusInstanceForOverlay(inst); cmd != nil {
								signalCmds = append(signalCmds, cmd)
							}
							m.pendingPlannerInstanceTitle = inst.Title
							break
						}
					}
					m.pendingPlannerTaskFile = planFile
					m.confirmAction(
						fmt.Sprintf("task '%s' is ready. start implementation?", taskstate.DisplayName(planFile)),
						func() tea.Msg {
							return plannerCompleteMsg{planFile: planFile}
						},
					)
				}
			}

			// Process wave signals — trigger implementation for specific waves.
			for _, ws := range msg.WaveSignals {
				taskfsm.ConsumeWaveSignal(ws)

				// Check if orchestrator already exists
				if _, exists := m.waveOrchestrators[ws.TaskFile]; exists {
					m.toastManager.Error(fmt.Sprintf("wave already running for '%s'", taskstate.DisplayName(ws.TaskFile)))
					continue
				}

				// Read and parse the plan from store
				content, err := m.taskStore.GetContent(m.taskStoreProject, ws.TaskFile)
				if err != nil {
					log.WarningLog.Printf("wave signal: could not read plan %s: %v", ws.TaskFile, err)
					continue
				}
				plan, err := taskparser.Parse(content)
				if err != nil {
					m.toastManager.Error(fmt.Sprintf("plan '%s' has no wave headers", taskstate.DisplayName(ws.TaskFile)))
					continue
				}

				if ws.WaveNumber > len(plan.Waves) {
					m.toastManager.Error(fmt.Sprintf("plan has %d waves, requested wave %d", len(plan.Waves), ws.WaveNumber))
					continue
				}

				entry, ok := m.taskState.Entry(ws.TaskFile)
				if !ok {
					log.WarningLog.Printf("wave signal: plan %s not found in plan state", ws.TaskFile)
					continue
				}

				orch := orchestration.NewWaveOrchestrator(ws.TaskFile, plan)
				orch.SetStore(m.taskStore, m.taskStoreProject)
				m.waveOrchestrators[ws.TaskFile] = orch

				// Fast-forward to the requested wave
				for i := 1; i < ws.WaveNumber; i++ {
					tasks := orch.StartNextWave()
					for _, t := range tasks {
						orch.MarkTaskComplete(t.Number)
					}
				}

				mdl, cmd := m.startNextWave(orch, entry)
				m = mdl.(*home)
				if cmd != nil {
					signalCmds = append(signalCmds, cmd)
				}
			}

			// Process elaboration signals — elaborator-finished files written by the
			// elaborator agent once it has enriched all task bodies and stored the
			// updated plan in the task store. On receipt we re-read the plan,
			// replace it in the orchestrator, kill the elaborator instance, and
			// start wave 1 normally.
			for _, es := range msg.ElaborationSignals {
				taskfsm.ConsumeElaborationSignal(es)

				orch, exists := m.waveOrchestrators[es.TaskFile]
				if !exists || orch.State() != orchestration.WaveStateElaborating {
					log.WarningLog.Printf("ignoring elaborator-finished signal for %q — no active elaboration", es.TaskFile)
					continue
				}

				// Re-read the enriched plan from the store.
				content, err := m.taskStore.GetContent(m.taskStoreProject, es.TaskFile)
				if err != nil {
					log.WarningLog.Printf("elaboration signal: could not read plan %s: %v", es.TaskFile, err)
					continue
				}
				plan, err := taskparser.Parse(content)
				if err != nil {
					log.WarningLog.Printf("elaboration signal: could not parse enriched plan %s: %v", es.TaskFile, err)
					continue
				}

				// Replace the plan in the orchestrator with the enriched version.
				orch.UpdatePlan(plan)

				// Kill the elaborator instance.
				for _, inst := range m.nav.GetInstances() {
					if inst.TaskFile == es.TaskFile && inst.AgentType == session.AgentTypeElaborator {
						_ = inst.Kill()
						break
					}
				}

				entry, ok := m.taskState.Entry(es.TaskFile)
				if !ok {
					continue
				}

				m.toastManager.Info(fmt.Sprintf("plan elaborated — starting wave 1 for '%s'", taskstate.DisplayName(es.TaskFile)))
				mdl, cmd := m.startNextWave(orch, entry)
				m = mdl.(*home)
				if cmd != nil {
					signalCmds = append(signalCmds, cmd)
				}
			}
		}

		// Apply collected metadata to instances — zero I/O, just field writes.
		// All subprocess calls (TapEnter, SendPrompt) are deferred to tea.Cmds.
		instanceMap := make(map[string]*session.Instance)
		for _, inst := range m.nav.GetInstances() {
			instanceMap[inst.Title] = inst
		}

		var asyncCmds []tea.Cmd

		for _, md := range msg.Results {
			inst, ok := instanceMap[md.Title]
			if !ok {
				continue
			}

			if md.ContentCaptured {
				inst.CachedContent = md.Content
				inst.CachedContentSet = true

				if md.Updated {
					inst.SetStatus(session.Running)
					inst.PromptDetected = false
					// Mark that the agent has produced real work only after the
					// queued task prompt has been dispatched and we observe
					// non-prompt output. This prevents startup/prologue output and
					// prompt-echo ticks from prematurely completing wave tasks.
					if inst.TaskNumber > 0 && !inst.HasWorked && inst.QueuedPrompt == "" && !md.HasPrompt {
						inst.HasWorked = true
					}
					if md.Content != "" {
						inst.LastActivity = session.ParseActivity(md.Content, inst.Program)
					}
				} else {
					if md.HasPrompt {
						inst.PromptDetected = true
						// Defer tmux send-keys to async Cmd (was blocking Update).
						i := inst
						asyncCmds = append(asyncCmds, func() tea.Msg {
							i.TapEnter()
							return nil
						})
					} else {
						inst.SetStatus(session.Ready)
					}
					if inst.Status != session.Running {
						inst.LastActivity = nil
					}
				}
			}

			// Permission prompt detection for opencode.
			if md.PermissionPrompt != nil && (m.state == stateDefault || m.state == stateFocusAgent) {
				m.exitFocusModeForDialog()
				pp := md.PermissionPrompt
				cacheKey := config.CacheKey(pp.Pattern, pp.Description)
				// Guard key: use cache key if available, else sentinel.
				// Must match what app_input.go sets on confirm.
				guardKey := cacheKey
				if guardKey == "" {
					guardKey = "__handled__"
				}

				if _, handled := m.permissionHandled[inst]; handled {
					// Already handled this prompt appearance — skip until cleared.
				} else if cacheKey != "" && m.permissionStore != nil && m.permissionStore.IsAllowedAlways(m.activeProject(), cacheKey) {
					// Auto-approve cached permission.
					m.permissionHandled[inst] = guardKey
					i := inst
					asyncCmds = append(asyncCmds, func() tea.Msg {
						return permissionAutoApproveMsg{instance: i}
					})
				} else {
					// Focus the instance so the user can see the agent output behind the overlay.
					if cmd := m.focusInstanceForOverlay(inst); cmd != nil {
						asyncCmds = append(asyncCmds, cmd)
					}
					// Show modal (statePermission blocks re-entry on subsequent ticks).
					perm := overlay.NewPermissionOverlay(inst.Title, pp.Description, pp.Pattern)
					m.pendingPermissionPattern = pp.Pattern
					m.pendingPermissionDesc = pp.Description
					m.overlays.Show(perm)
					m.pendingPermissionInstance = inst
					m.state = statePermission
					m.audit(auditlog.EventPermissionDetected,
						fmt.Sprintf("permission prompt detected for %s", inst.Title),
						auditlog.WithInstance(inst.Title),
					)
				}
			} else if md.PermissionPrompt == nil {
				// Prompt cleared — remove the in-flight guard so a future permission
				// prompt for this instance can trigger auto-approve again.
				delete(m.permissionHandled, inst)
			}

			// Deliver queued prompt via async Cmd — SendPrompt contains a 100ms
			// sleep + two tmux subprocess calls that were blocking the event loop.
			if inst.QueuedPrompt != "" && (inst.Status == session.Ready || inst.PromptDetected) {
				prompt := inst.QueuedPrompt
				inst.QueuedPrompt = "" // clear immediately to prevent re-send
				inst.AwaitingWork = true
				i := inst
				asyncCmds = append(asyncCmds, func() tea.Msg {
					if err := i.SendPrompt(prompt); err != nil {
						log.WarningLog.Printf("could not send queued prompt to %q: %v", i.Title, err)
					}
					return nil
				})
			}

			if md.ResourceUsageValid {
				inst.CPUPercent = md.CPUPercent
				inst.MemMB = md.MemMB
			}
		}

		// Clear activity for non-started / paused instances
		for _, inst := range m.nav.GetInstances() {
			if !inst.Started() || inst.Paused() {
				inst.LastActivity = nil
			}
		}

		// Apply plan state loaded in the goroutine (replaces synchronous loadTaskState call).
		// Skip when signals were processed: loadTaskState() above already gave us fresh state.
		// msg.PlanState was loaded before signals were scanned, so it would be stale.
		if msg.PlanState != nil && (msg.DaemonManagedRepo || len(msg.Signals) == 0) {
			m.taskState = msg.PlanState
		}

		// Store the latest tmux session count for the bottom bar.
		m.tmuxSessionCount = msg.TmuxSessionCount
		m.menu.SetTmuxSessionCount(m.tmuxSessionCount)

		if m.taskState != nil {
			tmuxAliveMap := make(map[string]bool, len(msg.Results))
			for _, md := range msg.Results {
				tmuxAliveMap[md.Title] = md.TmuxAlive
			}

			// Implementer-exit → push-prompt: when a coder or fixer session's tmux
			// pane has exited and the plan is still in StatusImplementing, prompt the
			// user to push the implementation branch before advancing to reviewing.
			// Skip when a confirmation overlay is already showing to avoid re-prompting
			// on every tick while the user is deciding.
			for _, inst := range m.nav.GetInstances() {
				if m.isUserInOverlay() {
					break
				}
				alive, collected := tmuxAliveMap[inst.Title]
				if !collected {
					continue
				}
				entry := m.taskState.Plans[inst.TaskFile]
				if !shouldPromptPushAfterImplementerExit(entry, inst, alive) {
					continue
				}
				// Wave task instances never trigger the single-coder completion flow.
				// Wave completion is handled by the orchestrator, not the coder-exit prompt.
				if inst.TaskNumber > 0 {
					continue
				}
				// Skip if the push prompt was already shown and dismissed for this plan.
				// Cleared when a new implementer is spawned for the next round.
				if m.coderPushPrompted[inst.TaskFile] {
					continue
				}
				// Focus the implementer instance so the user can see its output behind the overlay.
				m.exitFocusModeForDialog()
				if cmd := m.focusInstanceForOverlay(inst); cmd != nil {
					asyncCmds = append(asyncCmds, cmd)
				}
				if cmd := m.promptPushBranchThenAdvance(inst); cmd != nil {
					asyncCmds = append(asyncCmds, cmd)
				}
				// Only prompt for one instance per tick to avoid stacking overlays.
				break
			}

			// Tmux death detection: mark instances as exited when their tmux
			// session dies so the UI renders them greyed-out + strikethrough
			// and allows cleanup. Covers solo agents, reviewers, and any
			// other instance whose tmux session disappears while the TUI runs.
			for _, inst := range m.nav.GetInstances() {
				if inst.Exited || inst.Paused() {
					continue
				}
				alive, collected := tmuxAliveMap[inst.Title]
				if collected && !alive {
					inst.Exited = true
					if inst.Status == session.Running {
						inst.SetStatus(session.Ready)
					}
					m.audit(auditlog.EventAgentFinished, fmt.Sprintf("agent finished: %s", inst.Title),
						auditlog.WithInstance(inst.Title),
						auditlog.WithAgent(inst.AgentType),
						auditlog.WithPlan(inst.TaskFile),
					)
				}
			}

			// Dead elaborator recovery: if an elaborator instance died without
			// writing its signal, recover by re-reading the (possibly enriched)
			// plan and starting wave 1. Prevents the elaboration loop where a
			// crashed elaborator leaves the orchestrator stuck forever.
			for planFile, orch := range m.waveOrchestrators {
				if orch.State() != orchestration.WaveStateElaborating {
					continue
				}
				// Check if the elaborator instance for this plan is dead.
				var deadElaborator *session.Instance
				for _, inst := range m.nav.GetInstances() {
					if inst.TaskFile == planFile && inst.AgentType == session.AgentTypeElaborator && inst.Exited {
						deadElaborator = inst
						break
					}
				}
				if deadElaborator == nil {
					continue
				}
				log.WarningLog.Printf("elaborator for %q died without signaling — recovering", planFile)

				// Re-read the plan from the store (elaborator may have enriched it before crashing).
				if m.taskStore != nil {
					if content, err := m.taskStore.GetContent(m.taskStoreProject, planFile); err == nil {
						if plan, parseErr := taskparser.Parse(content); parseErr == nil {
							orch.UpdatePlan(plan)
						}
					}
				}
				// If re-read failed, force out of elaborating with the original plan.
				if orch.State() == orchestration.WaveStateElaborating {
					orch.UpdatePlan(orch.Plan())
				}

				// Remove the dead elaborator instance.
				m.killExistingPlanAgent(planFile, session.AgentTypeElaborator)

				entry, ok := m.taskState.Entry(planFile)
				if !ok {
					continue
				}

				planName := taskstate.DisplayName(planFile)
				m.toastManager.Info(fmt.Sprintf("elaborator crashed — starting wave 1 for '%s'", planName))
				m.audit(auditlog.EventWaveStarted, "elaborator crash recovery: starting wave 1",
					auditlog.WithPlan(planFile))

				mdl, cmd := m.startNextWave(orch, entry)
				m = mdl.(*home)
				if cmd != nil {
					signalCmds = append(signalCmds, cmd)
				}
			}

			// Drain deferred all-complete prompts that were blocked by an overlay.
			if len(m.pendingAllComplete) > 0 {
				m.exitFocusModeForDialog()
			}
			if !m.isUserInOverlay() && len(m.pendingAllComplete) > 0 {
				planFile := m.pendingAllComplete[0]
				m.pendingAllComplete = m.pendingAllComplete[1:]
				planName := taskstate.DisplayName(planFile)
				if cmd := m.focusPlanInstanceForOverlay(planFile); cmd != nil {
					asyncCmds = append(asyncCmds, cmd)
				}
				message := fmt.Sprintf("all waves complete for '%s'. push branch and start review?", planName)
				m.confirmAction(message, func() tea.Msg {
					return waveAllCompleteMsg{planFile: planFile}
				})
			}

			// Wave completion monitoring: check task completion and trigger wave transitions.
			// We process both orchestration.WaveStateRunning (check task statuses) and orchestration.WaveStateWaveComplete
			// (re-show confirm dialog after user cancelled, resetting the latch via ResetConfirm).
			for planFile, orch := range m.waveOrchestrators {
				orchState := orch.State()
				if orchState != orchestration.WaveStateRunning && orchState != orchestration.WaveStateWaveComplete && orchState != orchestration.WaveStateAllComplete {
					continue
				}

				if orchState == orchestration.WaveStateRunning {
					// Check task status updates only while the wave is actively running.
					planName := taskstate.DisplayName(planFile)
					for _, task := range orch.CurrentWaveTasks() {
						taskTitle := fmt.Sprintf("%s-W%d-T%d", planName, orch.CurrentWaveNumber(), task.Number)
						inst, exists := instanceMap[taskTitle]
						if !exists {
							// Instance not in metadata results — check if it exists in the
							// nav list but hasn't started yet (async spawn still in flight).
							// Only mark failed if the instance is truly missing.
							stillSpawning := false
							for _, navInst := range m.nav.GetInstances() {
								if navInst.Title == taskTitle && (!navInst.Started() || navInst.Status == session.Loading) {
									stillSpawning = true
									break
								}
							}
							if stillSpawning {
								continue // wait for async start to complete
							}
							orch.MarkTaskFailed(task.Number)
							continue
						}
						if inst.Paused() {
							// Paused task instances are treated as failures.
							orch.MarkTaskFailed(task.Number)
							continue
						}
						alive, collected := tmuxAliveMap[inst.Title]
						if !collected {
							continue
						}
						if !alive {
							orch.MarkTaskFailed(task.Number)
						}
					}
					orchState = orch.State() // refresh after task updates
				}

				// All waves complete — pause the last wave's tasks, prompt for review.
				if orchState == orchestration.WaveStateAllComplete {
					capturedPlanFile := planFile
					planName := taskstate.DisplayName(planFile)
					totalWaves := orch.TotalWaves()
					waveNumFinal := orch.CurrentWaveNumber()
					completedFinal := orch.CompletedTaskCount()
					totalFinal := completedFinal + orch.FailedTaskCount()

					// Pause all task instances (they're done, free up resources).
					for _, inst := range m.nav.GetInstances() {
						if inst.TaskFile == capturedPlanFile && inst.TaskNumber > 0 {
							inst.ImplementationComplete = true
							_ = inst.Pause()
						}
					}
					delete(m.waveOrchestrators, planFile)
					m.audit(auditlog.EventWaveCompleted, "all waves complete: "+planName,
						auditlog.WithPlan(capturedPlanFile))
					// Post wave complete comment to ClickUp for multi-wave plans.
					if orch.ShouldPostWaveCompleteComment() {
						detail := fmt.Sprintf("%d/%d: %d/%d tasks", waveNumFinal, totalWaves, completedFinal, totalFinal)
						if cmd := m.postClickUpProgress(capturedPlanFile, "wave_complete", detail); cmd != nil {
							asyncCmds = append(asyncCmds, cmd)
						}
					}

					m.exitFocusModeForDialog()
					if !m.isUserInOverlay() {
						// Focus a task instance so the user can see agent output behind the overlay.
						if cmd := m.focusPlanInstanceForOverlay(capturedPlanFile); cmd != nil {
							asyncCmds = append(asyncCmds, cmd)
						}
						message := fmt.Sprintf("all waves complete for '%s'. push branch and start review?", planName)
						m.confirmAction(message, func() tea.Msg {
							return waveAllCompleteMsg{planFile: capturedPlanFile}
						})
					} else {
						// Overlay is active — defer the prompt so it fires on the next
						// tick when the overlay clears. Without this, the orchestrator
						// deletion above means we never re-enter this code path.
						m.pendingAllComplete = append(m.pendingAllComplete, capturedPlanFile)
					}
					continue
				}

				// orchState must be orchestration.WaveStateWaveComplete here.
				// Show wave decision confirm once per wave (NeedsConfirm is one-shot;
				// ResetConfirm on cancel allows the prompt to reappear next tick).
				needsConfirm := orch.NeedsConfirm()
				if needsConfirm {
					m.exitFocusModeForDialog()
				}
				if !m.isUserInOverlay() && time.Since(m.waveConfirmDismissedAt) > 30*time.Second && needsConfirm {
					waveNum := orch.CurrentWaveNumber()
					completed := orch.CompletedTaskCount()
					failed := orch.FailedTaskCount()
					total := completed + failed
					entry, _ := m.taskState.Entry(planFile)

					capturedPlanFile := planFile
					capturedEntry := entry
					planName := taskstate.DisplayName(planFile)

					// Post intermediate wave complete comment to ClickUp for
					// multi-wave plans with no failures.
					if failed == 0 && orch.ShouldPostWaveCompleteComment() {
						detail := fmt.Sprintf("%d/%d: %d/%d tasks", waveNum, orch.TotalWaves(), completed, total)
						if cmd := m.postClickUpProgress(capturedPlanFile, "wave_complete", detail); cmd != nil {
							asyncCmds = append(asyncCmds, cmd)
						}
					}

					if failed > 0 {
						// Failed wave — always show the decision dialog (retry/next/abort)
						if cmd := m.focusPlanInstanceForOverlay(capturedPlanFile); cmd != nil {
							asyncCmds = append(asyncCmds, cmd)
						}
						m.audit(auditlog.EventWaveFailed,
							fmt.Sprintf("wave %d: %d/%d tasks failed", waveNum, failed, total),
							auditlog.WithPlan(capturedPlanFile),
							auditlog.WithWave(waveNum, 0))
						message := fmt.Sprintf(
							"%s — wave %d: %d/%d tasks complete, %d failed.\n\n"+
								"[r] retry failed   [n] next wave   [a] abort",
							planName, waveNum, completed, total, failed)
						m.waveFailedConfirmAction(message, capturedPlanFile, capturedEntry)
					} else if m.appConfig.AutoAdvanceWaves {
						// Auto-advance: skip confirmation, directly advance to next wave
						m.audit(auditlog.EventWaveCompleted,
							fmt.Sprintf("wave %d complete: %d/%d tasks (auto-advancing)", waveNum, completed, total),
							auditlog.WithPlan(capturedPlanFile),
							auditlog.WithWave(waveNum, 0))
						m.toastManager.Info(fmt.Sprintf("%s — wave %d complete, auto-advancing...", planName, waveNum))
						asyncCmds = append(asyncCmds, func() tea.Msg {
							return waveAdvanceMsg{planFile: capturedPlanFile, entry: capturedEntry}
						})
						asyncCmds = append(asyncCmds, m.toastTickCmd())
					} else {
						// Manual mode: focus instance and show confirmation dialog
						if cmd := m.focusPlanInstanceForOverlay(capturedPlanFile); cmd != nil {
							asyncCmds = append(asyncCmds, cmd)
						}
						m.audit(auditlog.EventWaveCompleted,
							fmt.Sprintf("wave %d complete: %d/%d tasks", waveNum, completed, total),
							auditlog.WithPlan(capturedPlanFile),
							auditlog.WithWave(waveNum, 0))
						message := fmt.Sprintf("%s — wave %d complete (%d/%d). start wave %d?",
							planName, waveNum, completed, total, waveNum+1)
						m.waveStandardConfirmAction(message, capturedPlanFile, capturedEntry)
					}
				}
			}
		}

		// Apply PR state updates from periodic polling.
		if len(msg.PRStateUpdates) > 0 && m.taskStore != nil {
			selectedPlanFile := m.nav.GetSelectedPlanFile()
			selectedPlanChanged := false
			for _, u := range msg.PRStateUpdates {
				if err := m.taskStore.SetPRState(m.taskStoreProject, u.planFile, u.reviewDecision, u.checkStatus); err != nil {
					log.WarningLog.Printf("PR state update: could not persist for %q: %v", u.planFile, err)
				}
				if u.planFile == selectedPlanFile {
					selectedPlanChanged = true
				}
			}
			if selectedPlanChanged {
				m.loadTaskState()
				m.updateInfoPane()
			}
		}

		m.updateSidebarTasks()
		m.updateInfoPane()
		completionCmd := m.checkPlanCompletion()
		asyncCmds = append(asyncCmds, signalCmds...)
		asyncCmds = append(asyncCmds, tickUpdateMetadataCmd, completionCmd)
		// Restart toast tick loop if any toasts were created during this tick
		// (e.g. by transitionToReview or spawnFixerWithFeedback).
		if m.toastManager.HasActiveToasts() {
			asyncCmds = append(asyncCmds, m.toastTickCmd())
		}
		return m, tea.Batch(asyncCmds...)
	case tea.MouseClickMsg:
		return m.handleMouseClick(msg)
	case tea.MouseWheelMsg:
		return m.handleMouseWheel(msg)
	case tea.KeyPressMsg:
		return m.handleKeyPress(msg)
	case tea.WindowSizeMsg:
		m.updateHandleWindowSizeEvent(msg)
		return m, nil
	case error:
		// Handle errors from confirmation actions
		return m, m.handleError(msg)
	case instanceChangedMsg:
		// Handle instance changed after confirmation action
		m.updateNavPanelStatus()
		return m, m.instanceChanged()
	case previewTerminalReadyMsg:
		// Discard stale attach if selection changed while spawning.
		selected := m.nav.GetSelectedInstance()
		if msg.err != nil || !m.shouldAttachPreviewTerminal(selected) || selected.Title != msg.instanceTitle {
			return m, asyncClosePreviewTerminal(msg.term)
		}
		m.previewTerminal = msg.term
		m.previewTerminalInstance = msg.instanceTitle
		return m, nil
	case killInstanceMsg:
		// Async pre-kill checks passed — pause instead of destroying (branch preserved).
		for _, inst := range m.allInstances {
			if inst.Title == msg.title {
				if err := inst.Pause(); err != nil {
					return m, m.handleError(err)
				}
				break
			}
		}
		m.saveAllInstances()
		m.updateNavPanelStatus()
		return m, tea.Batch(tea.RequestWindowSize, m.instanceChanged())
	case taskStageConfirmedMsg:
		// User confirmed past the topic-concurrency gate — execute the stage.
		return m.executeTaskStage(msg.planFile, msg.stage)
	case taskRefreshMsg:
		// Reload plan state and refresh sidebar after async plan mutation.
		m.loadTaskState()
		m.updateSidebarTasks()
		return m, tea.RequestWindowSize
	case clickUpTaskFetchedMsg:
		if msg.Err != nil {
			m.toastManager.Error("clickup fetch failed: " + msg.Err.Error())
			m.state = stateDefault
			return m, m.toastTickCmd()
		}
		m.state = stateDefault
		return m.importClickUpTask(msg.Task)
	case waveAdvanceMsg:
		orch, ok := m.waveOrchestrators[msg.planFile]
		if !ok {
			return m, nil
		}
		// Pause completed wave's instances before starting the next.
		planName := taskstate.DisplayName(msg.planFile)
		for _, task := range orch.CurrentWaveTasks() {
			taskTitle := fmt.Sprintf("%s-W%d-T%d", planName, orch.CurrentWaveNumber(), task.Number)
			for _, inst := range m.nav.GetInstances() {
				if inst.Title == taskTitle && inst.PromptDetected {
					if err := inst.Pause(); err != nil {
						log.WarningLog.Printf("could not pause task %s: %v", taskTitle, err)
					}
				}
			}
		}
		return m.startNextWave(orch, msg.entry)
	case waveRetryMsg:
		orch, ok := m.waveOrchestrators[msg.planFile]
		if !ok {
			return m, nil
		}
		return m.retryFailedWaveTasks(orch, msg.entry)
	case waveAbortMsg:
		delete(m.waveOrchestrators, msg.planFile)
		// Kill and remove all task instances that belong to the aborted plan.
		// Their tmux sessions are already dead (tasks failed), so no worktree
		// check is needed — just clean them out of the list.
		// Collect first to avoid mutating m.allInstances while iterating it.
		var taskInsts []*session.Instance
		for _, inst := range m.allInstances {
			if inst.TaskFile == msg.planFile && inst.TaskNumber > 0 {
				taskInsts = append(taskInsts, inst)
			}
		}
		for _, inst := range taskInsts {
			if m.nav.SelectInstance(inst) {
				m.nav.Kill()
			}
			m.removeFromAllInstances(inst.Title)
		}
		m.saveAllInstances()
		m.updateNavPanelStatus()
		m.toastManager.Info(fmt.Sprintf("wave orchestration aborted for %s",
			taskstate.DisplayName(msg.planFile)))
		return m, tea.Batch(tea.RequestWindowSize, m.instanceChanged(), m.toastTickCmd())
	case waveAllCompleteMsg:
		// Capture the target instance on the main goroutine to avoid a data
		// race when the returned tea.Cmd runs concurrently with future Updates
		// that may mutate m.nav.instances.
		planFile := msg.planFile
		var pushInst *session.Instance
		for _, inst := range m.nav.GetInstances() {
			if inst.TaskFile == planFile && inst.TaskNumber > 0 {
				pushInst = inst
				break
			}
		}
		return m, func() tea.Msg {
			if pushInst != nil {
				if worktree, err := pushInst.GetGitWorktree(); err == nil && worktree != nil {
					_ = worktree.Push(false)
				}
			}
			return wavePushCompleteMsg{planFile: planFile}
		}
	case wavePushCompleteMsg:
		// After async push completes for wave flow, transition and spawn reviewer.
		planFile := msg.planFile
		planName := taskstate.DisplayName(planFile)

		if err := m.fsm.Transition(planFile, taskfsm.ImplementFinished); err != nil {
			log.WarningLog.Printf("wave push-complete: could not transition %q to reviewing: %v", planFile, err)
		}
		m.loadTaskState()
		m.updateSidebarTasks()

		var reviewerCmd tea.Cmd
		if cmd := m.spawnReviewer(planFile); cmd != nil {
			reviewerCmd = cmd
		}
		m.toastManager.Info(fmt.Sprintf("all waves complete for '%s' — starting review", planName))
		return m, tea.Batch(tea.RequestWindowSize, reviewerCmd, m.toastTickCmd())
	case coderCompleteMsg:
		// Single-plan implementation finished and user confirmed push.
		// Transition FSM and spawn reviewer — mirrors waveAllCompleteMsg flow.
		planFile := msg.planFile
		planName := taskstate.DisplayName(planFile)

		if err := m.fsm.Transition(planFile, taskfsm.ImplementFinished); err != nil {
			log.WarningLog.Printf("coder-complete: could not transition %q to reviewing: %v", planFile, err)
		}

		// Clear the push-prompt dedup flag — the plan is now in reviewing, so
		// if a review round sends it back to implementing the next implementer can
		// trigger the push prompt cleanly.
		delete(m.coderPushPrompted, planFile)

		// Mark the current implementer instance as implementation-complete and pause it.
		for _, inst := range m.nav.GetInstances() {
			if inst.TaskFile == planFile && (inst.AgentType == session.AgentTypeCoder || inst.AgentType == session.AgentTypeFixer) {
				inst.ImplementationComplete = true
				_ = inst.Pause()
				break
			}
		}

		m.loadTaskState()
		m.updateSidebarTasks()

		var reviewerCmd tea.Cmd
		if cmd := m.spawnReviewer(planFile); cmd != nil {
			reviewerCmd = cmd
		}
		m.toastManager.Info(fmt.Sprintf("implementation complete for '%s' — starting review", planName))
		return m, tea.Batch(tea.RequestWindowSize, reviewerCmd, m.toastTickCmd())
	case tmuxSessionsMsg:
		if msg.err != nil {
			return m, m.handleError(msg.err)
		}
		if len(msg.sessions) == 0 {
			if m.toastManager != nil {
				m.toastManager.Info("no kas tmux sessions found")
				return m, m.toastTickCmd()
			}
			return m, nil
		}
		// Build instance lookup for enrichment.
		instMap := make(map[string]*session.Instance, len(m.allInstances))
		for _, inst := range m.allInstances {
			if inst.Started() {
				instMap[tmux.ToKasTmuxNamePublic(inst.Title)] = inst
			}
		}
		items := make([]overlay.TmuxBrowserItem, len(msg.sessions))
		for i, s := range msg.sessions {
			items[i] = overlay.TmuxBrowserItem{
				Name:     s.Name,
				Title:    s.Title,
				Created:  s.Created,
				Windows:  s.Windows,
				Attached: s.Attached,
				Width:    s.Width,
				Height:   s.Height,
				Managed:  s.Managed,
			}
			if inst, ok := instMap[s.Name]; ok {
				items[i].TaskFile = inst.TaskFile
				items[i].AgentType = inst.AgentType
				items[i].Status = statusString(inst.Status)
			}
		}
		m.overlays.Show(overlay.NewTmuxBrowserOverlay(items))
		m.state = stateTmuxBrowser
		return m, nil
	case tmuxKillResultMsg:
		if msg.err != nil {
			m.toastManager.Error(fmt.Sprintf("failed to kill session: %v", msg.err))
		} else {
			m.toastManager.Success(fmt.Sprintf("killed session '%s'", msg.name))
		}
		return m, m.toastTickCmd()
	case tmuxAttachReturnMsg:
		m.toastManager.Info("detached from tmux session")
		return m, tea.Batch(tea.RequestWindowSize, m.toastTickCmd())
	case permissionAutoApproveMsg:
		if msg.instance != nil && msg.instance.Started() {
			i := msg.instance
			return m, func() tea.Msg {
				i.SendPermissionResponse(tmux.PermissionAllowAlways)
				return nil
			}
		}
		return m, nil
	case planTitleMsg:
		if m.state == stateNewPlanDeriving {
			if msg.err == nil && msg.title != "" {
				m.pendingPlanName = msg.title
			}
			topicNames := m.getTopicNames()
			topicNames = append([]string{"(No topic)"}, topicNames...)
			pickerTitle := fmt.Sprintf("assign to topic for '%s'", m.pendingPlanName)
			p := overlay.NewPickerOverlay(pickerTitle, topicNames)
			p.SetAllowCustom(true)
			m.overlays.Show(p)
			m.state = stateNewPlanTopic
			return m, nil
		}
		// Safety net: if title arrives while already in topic picker, update silently
		if msg.err == nil && msg.title != "" {
			if m.state == stateNewPlanTopic && m.pendingPlanDesc != "" {
				m.pendingPlanName = msg.title
				if po, ok := m.overlays.Current().(*overlay.PickerOverlay); ok {
					po.SetTitle(
						fmt.Sprintf("assign to topic for '%s'", msg.title),
					)
					return m, tea.RequestWindowSize
				}
			}
		}
		return m, nil
	case plannerCompleteMsg:
		// User confirmed: start implementation. Kill the planner instance (may still be alive
		// when triggered by the PlannerFinished sentinel, unlike the tmux-death path).
		m.plannerPrompted[msg.planFile] = true
		m.killExistingPlanAgent(msg.planFile, session.AgentTypePlanner)
		_ = m.saveAllInstances()
		m.pendingPlannerInstanceTitle = ""
		m.pendingPlannerTaskFile = ""
		m.updateNavPanelStatus()
		return m.triggerTaskStage(msg.planFile, "implement")
	case instanceStartedMsg:
		if msg.err != nil {
			// Remove the specific instance that failed — not the currently selected one.
			_ = msg.instance.Kill()
			m.nav.RemoveByTitle(msg.instance.Title)
			m.removeFromAllInstances(msg.instance.Title)
			m.updateNavPanelStatus()
			return m, m.handleError(msg.err)
		}
		// Instance started successfully — add to master list, save and finalize
		m.allInstances = append(m.allInstances, msg.instance)
		if err := m.saveAllInstances(); err != nil {
			return m, m.handleError(err)
		}
		m.updateNavPanelStatus()
		if fn, ok := m.instanceFinalizers[msg.instance]; ok {
			fn()
			delete(m.instanceFinalizers, msg.instance)
		}
		if m.autoYes {
			msg.instance.AutoYes = true
		}
		return m, tea.Batch(tea.RequestWindowSize, m.instanceChanged())
	case tea.ClipboardMsg:
		if m.previewClipboardPending {
			selection := m.previewClipboardTarget
			if selection == 0 {
				selection = ansi.SystemClipboard
			}
			m.previewClipboardPending = false
			m.previewClipboardTarget = 0
			if m.previewTerminal != nil {
				if err := m.previewTerminal.SendKey([]byte(ansi.SetClipboard(selection, msg.Content))); err != nil {
					return m, m.handleError(err)
				}
			}
			return m, nil
		}
	case tea.PasteMsg:
		// Forward pasted text to the embedded PTY in focus mode.
		if m.state == stateFocusAgent && m.previewTerminal != nil {
			if content := msg.Content; content != "" {
				// Wrap in bracketed paste so the program inside tmux sees it
				// as a paste event rather than typed input.
				data := []byte("\x1b[200~" + content + "\x1b[201~")
				_ = m.previewTerminal.SendKey(data)
			} else {
				// Empty paste content means the clipboard holds non-text data
				// (e.g. an image). Forward raw ctrl+v (0x16) so the embedded
				// program can request clipboard contents via OSC 52 or its own
				// native paste mechanism.
				_ = m.previewTerminal.SendKey([]byte{0x16})
			}
			return m, nil
		}
	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		return m, cmd
	default:
		// Forward unknown CSI sequences (kitty keyboard protocol, etc.) to the
		// embedded PTY when in focus/interactive mode. Bubbletea emits these as
		// an unexported []byte-based type; use reflect to extract raw bytes.
		if m.state == stateFocusAgent && m.previewTerminal != nil {
			v := reflect.ValueOf(msg)
			if v.Kind() == reflect.Slice && v.Type().Elem().Kind() == reflect.Uint8 {
				if data := v.Bytes(); len(data) > 0 {
					_ = m.previewTerminal.SendKey(data)
				}
				return m, nil
			}
		}
	}
	return m, nil
}

func nextPreviewTickCmd(term *session.EmbeddedTerminal) tea.Cmd {
	return func() tea.Msg {
		if term != nil {
			term.WaitForRender(16 * time.Millisecond)
		} else {
			time.Sleep(50 * time.Millisecond)
		}
		return previewTickMsg{}
	}
}

func readClipboardCmd(selection byte) tea.Cmd {
	return func() tea.Msg {
		if selection == ansi.PrimaryClipboard {
			return tea.ReadPrimaryClipboard()
		}
		return tea.ReadClipboard()
	}
}

func (m *home) handleQuit() (tea.Model, tea.Cmd) {
	// Check if any instances are actively running or loading.
	hasActive := false
	for _, inst := range m.nav.GetInstances() {
		if inst.Status == session.Running || inst.Status == session.Loading {
			hasActive = true
			break
		}
	}

	if hasActive {
		quitAction := func() tea.Msg {
			m.audit(auditlog.EventSessionStopped, "kasmos stopped")
			_ = m.saveAllInstances()
			return tea.QuitMsg{}
		}
		return m, m.confirmAction("quit kasmos? active sessions will be preserved.", quitAction)
	}

	m.audit(auditlog.EventSessionStopped, "kasmos stopped")
	if err := m.saveAllInstances(); err != nil {
		return m, m.handleError(err)
	}
	return m, tea.Quit
}

func (m *home) View() tea.View {
	// All columns use identical padding and height for uniform alignment.
	colStyle := lipgloss.NewStyle().Height(m.contentHeight)
	previewWithPadding := colStyle.Render(m.tabbedWindow.String())

	// Layout: nav | preview/tabs
	var cols []string
	if !m.sidebarHidden {
		cols = append(cols, colStyle.Render(m.nav.String()))
	}
	cols = append(cols, previewWithPadding)
	listAndPreview := lipgloss.JoinHorizontal(lipgloss.Top, cols...)
	statusBarView := ""
	if m.statusBar != nil {
		m.statusBar.SetData(m.computeStatusBarData())
		statusBarView = m.statusBar.String()
	}
	if m.menu != nil && m.nav != nil {
		m.menu.SetSidebarSpaceAction(m.nav.SelectedSpaceAction())
	}

	mainView := lipgloss.JoinVertical(
		lipgloss.Left,
		statusBarView,
		listAndPreview,
		m.menu.String(),
	)

	result := m.overlays.Render(mainView)

	if toastView := m.toastManager.View(); toastView != "" {
		x, y := m.toastManager.GetPosition()
		result = overlay.PlaceOverlay(x, y, toastView, result, false, false)
	}

	// Process bubblezone markers before rendering is complete
	// (zone markers inflate lipgloss.Width if left in place).
	result = safeZoneScan(result)

	// Height-fill — ensure enough lines for bubbletea's alt-screen renderer.
	// OSC 11 handles the actual background color; this just pads vertically.
	result = ui.FillBackground(result, m.termHeight)

	v := tea.NewView(result)
	v.AltScreen = true
	// We only use click/release/wheel interactions. All-motion floods Update with
	// hover events and makes the full-screen zone scan/path render laggy.
	v.MouseMode = tea.MouseModeCellMotion
	return v
}

func safeZoneScan(s string) (scanned string) {
	defer func() {
		if recover() != nil {
			scanned = s
		}
	}()

	return zone.Scan(s)
}

// permissionAutoApproveMsg is sent when a cached "allow always" pattern is detected.
type permissionAutoApproveMsg struct {
	instance *session.Instance
}

// permissionResponseMsg is sent when the user confirms a permission choice in the modal.
type permissionResponseMsg struct {
	instance *session.Instance
	choice   overlay.PermissionChoice
	pattern  string
}

// prCreatedMsg is sent when async PR creation succeeds.
type prCreatedMsg struct {
	instanceTitle string
	prTitle       string
}

// prCreatedForPlanMsg is sent when automatic PR creation on review approval succeeds.
type prCreatedForPlanMsg struct {
	planFile string
	url      string
}

// prStateUpdateMsg carries updated PR review/check state for a single plan.
type prStateUpdateMsg struct {
	planFile       string
	reviewDecision string
	checkStatus    string
}

// prErrorMsg is sent when async PR creation fails.
type prErrorMsg struct {
	id  string
	err error
}

// previewTickMsg implements tea.Msg and triggers a preview update
type previewTickMsg struct{}

type tickUpdateMetadataMessage struct{}

// previewTerminalReadyMsg signals that the async terminal attach completed.
type previewTerminalReadyMsg struct {
	term          *session.EmbeddedTerminal
	instanceTitle string
	err           error
}

type instanceChangedMsg struct{}

// killInstanceMsg is sent after async pre-kill checks pass (worktree not checked out).
// Model mutations (list.Kill, removeFromAllInstances) happen in Update, not in the goroutine.
type killInstanceMsg struct {
	title string
}

// taskStageConfirmedMsg is sent when the user confirms proceeding past the
// topic-concurrency gate. Re-enters plan stage execution skipping the
// concurrency check that was already acknowledged.
type taskStageConfirmedMsg struct {
	planFile string
	stage    string
}

// taskRefreshMsg triggers a plan state reload and sidebar refresh in Update.
type taskRefreshMsg struct{}

// waveAdvanceMsg is sent when the user confirms advancing to the next wave.
type waveAdvanceMsg struct {
	planFile string
	entry    taskstate.TaskEntry
}

// waveRetryMsg is sent when the user chooses "retry" on the failed-wave decision prompt.
type waveRetryMsg struct {
	planFile string
	entry    taskstate.TaskEntry
}

// waveAbortMsg is sent when the user chooses "abort" on the failed-wave decision prompt.
type waveAbortMsg struct {
	planFile string
}

// waveAllCompleteMsg is sent when the user confirms advancing to review
// after all waves in a plan have finished.
type waveAllCompleteMsg struct {
	planFile string
}

// wavePushCompleteMsg is sent when the async wave-complete push path finishes.
type wavePushCompleteMsg struct {
	planFile string
}

// coderCompleteMsg is sent when a single-coder (non-wave) implementation finishes
// and the user confirms pushing. Triggers FSM transition and reviewer spawn.
type coderCompleteMsg struct {
	planFile string
}

// plannerCompleteMsg is sent when the user confirms starting implementation
// after a planner session finishes.
type plannerCompleteMsg struct {
	planFile string
}

// tmuxSessionsMsg carries discovered kas_ tmux sessions (managed + orphaned).
type tmuxSessionsMsg struct {
	sessions []tmux.SessionInfo
	err      error
}

// tmuxKillResultMsg is sent after an orphaned tmux session is killed.
type tmuxKillResultMsg struct {
	name string
	err  error
}

// tmuxAttachReturnMsg is sent when the user detaches from a passively attached orphan session.
type tmuxAttachReturnMsg struct{}

// clickUpDetectedMsg is sent at startup when ClickUp MCP is detected.
type clickUpDetectedMsg struct {
	Config clickup.MCPServerConfig
}

// clickUpSearchResultMsg is sent when ClickUp search completes.
type clickUpSearchResultMsg struct {
	Results []clickup.SearchResult
	Query   string // original query, used to retry after workspace selection
	Err     error
}

// clickUpTaskFetchedMsg is sent when a full ClickUp task is fetched.
type clickUpTaskFetchedMsg struct {
	Task *clickup.Task
	Err  error
}

// addInstanceFinalizer registers a finalizer for the given instance.
// Lazily initializes the map so tests that don't pre-initialize it still work.
func (m *home) addInstanceFinalizer(inst *session.Instance, fn func()) {
	if m.instanceFinalizers == nil {
		m.instanceFinalizers = make(map[*session.Instance]func())
	}
	m.instanceFinalizers[inst] = fn
}

// instanceStartedMsg is sent when an async instance startup completes.
type instanceStartedMsg struct {
	instance *session.Instance
	err      error
}

type keyupMsg struct{}

// planRenderedMsg delivers the async glamour render result back to the Update loop.
type planRenderedMsg struct {
	planFile string
	rendered string
	err      error
}

// instanceMetadata holds the results of polling a single instance's subprocess data.
// Collected in a goroutine, applied to the model in Update.
type instanceMetadata struct {
	Title              string
	Content            string // tmux capture-pane output (reused for preview, activity, hash)
	ContentCaptured    bool
	Updated            bool
	HasPrompt          bool
	CPUPercent         float64
	MemMB              float64
	ResourceUsageValid bool
	TmuxAlive          bool
	PermissionPrompt   *session.PermissionPrompt // non-nil when opencode shows a permission dialog
}

// metadataResultMsg carries all per-instance metadata collected by the async tick.
type metadataResultMsg struct {
	Results            []instanceMetadata
	PlanState          *taskstate.TaskState        // pre-loaded plan state (nil if dir not set)
	Signals            []taskfsm.Signal            // agent sentinel files found this tick
	TaskSignals        []taskfsm.TaskSignal        // task completion sentinel files found this tick
	WaveSignals        []taskfsm.WaveSignal        // implement-wave-N signal files found this tick
	ElaborationSignals []taskfsm.ElaborationSignal // elaborator-finished signal files found this tick
	DaemonManagedRepo  bool                        // true when the active repo is managed by a running daemon
	TmuxSessionCount   int                         // number of kas_-prefixed tmux sessions
	PRStateUpdates     []prStateUpdateMsg          // PR review/check state refreshed this tick
}

// tickUpdateMetadataCmd is the callback to update the metadata of the instances every 200ms. We iterate
// over all instances and capture their output, but each tmux capture-pane call is <5ms so this is fine
// even at 20 instances (~100ms total). 200ms gives 5 ticks/sec for responsive signal processing.
var tickUpdateMetadataCmd = func() tea.Msg {
	time.Sleep(200 * time.Millisecond)
	return tickUpdateMetadataMessage{}
}

func (m *home) toastTickCmd() tea.Cmd {
	return func() tea.Msg {
		time.Sleep(50 * time.Millisecond)
		return overlay.ToastTickMsg{}
	}
}

func (m *home) searchClickUp(query string) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(m.ctx, clickUpOpTimeout)
		defer cancel()

		importer, err := m.getOrCreateImporter(ctx)
		if err != nil {
			return clickUpSearchResultMsg{Query: query, Err: normalizeClickUpError(err)}
		}

		searchDone := make(chan clickUpSearchResultMsg, 1)
		go func() {
			results, searchErr := importer.Search(query)
			searchDone <- clickUpSearchResultMsg{Query: query, Results: results, Err: searchErr}
		}()

		select {
		case msg := <-searchDone:
			msg.Err = normalizeClickUpError(msg.Err)
			if msg.Err != nil {
				// Don't nil the importer for MultipleWorkspacesError — we need
				// to call SetWorkspaceID on it after the user picks a workspace.
				var mwErr *clickup.MultipleWorkspacesError
				if errors.As(msg.Err, &mwErr) {
					// Resolve workspace IDs to names for a better picker UX.
					mwErr.WorkspaceNames = importer.FetchWorkspaceNames(mwErr.WorkspaceIDs)
				} else {
					m.clickUpImporter = nil // force re-init on next attempt
				}
			}
			return msg
		case <-ctx.Done():
			m.clickUpImporter = nil // force re-init on next attempt
			return clickUpSearchResultMsg{Query: query, Err: normalizeClickUpError(ctx.Err())}
		}
	}
}

func (m *home) fetchClickUpTaskWithTimeout(taskID string) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(m.ctx, clickUpOpTimeout)
		defer cancel()

		if m.clickUpImporter == nil {
			return clickUpTaskFetchedMsg{Err: fmt.Errorf("importer not initialized")}
		}

		fetchDone := make(chan clickUpTaskFetchedMsg, 1)
		go func() {
			task, fetchErr := m.clickUpImporter.FetchTask(taskID)
			fetchDone <- clickUpTaskFetchedMsg{Task: task, Err: fetchErr}
		}()

		select {
		case msg := <-fetchDone:
			msg.Err = normalizeClickUpError(msg.Err)
			if msg.Err != nil {
				m.clickUpImporter = nil // force re-init on next attempt
			}
			return msg
		case <-ctx.Done():
			m.clickUpImporter = nil // force re-init on next attempt
			return clickUpTaskFetchedMsg{Err: normalizeClickUpError(ctx.Err())}
		}
	}
}

func normalizeClickUpError(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, context.Canceled) {
		return fmt.Errorf("operation canceled")
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return fmt.Errorf("operation timed out after %s", clickUpOpTimeout)
	}
	return err
}

func (m *home) getOrCreateImporter(ctx context.Context) (*clickup.Importer, error) {
	if m.clickUpImporter != nil {
		return m.clickUpImporter, nil
	}
	if m.clickUpConfig == nil {
		return nil, fmt.Errorf("no clickup MCP server configured")
	}

	transport, err := m.createTransport(ctx, *m.clickUpConfig)
	if err != nil {
		return nil, err
	}

	client, err := mcpclient.NewClient(transport)
	if err != nil {
		_ = transport.Close()
		return nil, err
	}
	if err := client.Initialize(); err != nil {
		_ = client.Close()
		return nil, fmt.Errorf("MCP initialize: %w", err)
	}
	if _, err := client.ListTools(); err != nil {
		_ = client.Close()
		return nil, fmt.Errorf("MCP list tools: %w", err)
	}

	m.clickUpMCPClient = client
	m.clickUpImporter = clickup.NewImporter(client)

	// Restore saved workspace_id from per-project config.
	if projCfg := clickup.LoadProjectConfig(m.activeRepoPath); projCfg.WorkspaceID != "" {
		m.clickUpImporter.SetWorkspaceID(projCfg.WorkspaceID)
	}

	return m.clickUpImporter, nil
}

func (m *home) createTransport(ctx context.Context, cfg clickup.MCPServerConfig) (mcpclient.Transport, error) {
	switch cfg.Type {
	case "http":
		token, err := m.getClickUpToken(ctx)
		if err != nil {
			return nil, err
		}
		return mcpclient.NewHTTPTransport(cfg.URL, token), nil
	case "stdio":
		envSlice := make([]string, 0, len(cfg.Env))
		for k, v := range cfg.Env {
			envSlice = append(envSlice, k+"="+v)
		}
		return mcpclient.NewStdioTransport(cfg.Command, cfg.Args, envSlice)
	default:
		return nil, fmt.Errorf("unsupported transport type: %s", cfg.Type)
	}
}

func (m *home) getClickUpToken(ctx context.Context) (string, error) {
	// 1. Check opencode's mcp-auth.json first (populated by `opencode mcp auth clickup`).
	ocPath := mcpclient.OpencodeMCPAuthPath()
	if tok, err := mcpclient.LoadOpencodeToken(ocPath, "clickup"); err == nil && !tok.IsExpired() {
		return tok.AccessToken, nil
	}

	// 2. Fall back to kasmos's own cached token.
	path := mcpclient.TokenPath()
	tok, err := mcpclient.LoadToken(path)
	if err == nil && !tok.IsExpired() {
		return tok.AccessToken, nil
	}

	// 3. Last resort: run our own OAuth flow.
	oauthCfg := mcpclient.OAuthConfig{
		AuthURL:  "https://app.clickup.com/api",
		TokenURL: "https://api.clickup.com/api/v2/oauth/token",
		ClientID: "kasmos", // TODO: register ClickUp OAuth app
	}
	tok, err = mcpclient.OAuthFlow(ctx, oauthCfg, nil)
	if err != nil {
		return "", fmt.Errorf("oauth: %w", err)
	}
	if err := mcpclient.SaveToken(path, tok); err != nil {
		return "", fmt.Errorf("save token: %w", err)
	}
	return tok.AccessToken, nil
}

func detectClickUpCmd(repoPath string) tea.Cmd {
	return func() tea.Msg {
		claudeDir := filepath.Join(os.Getenv("HOME"), ".claude")
		cfg, found := clickup.DetectMCP(repoPath, claudeDir)
		if !found {
			return nil
		}
		return clickUpDetectedMsg{Config: cfg}
	}
}
