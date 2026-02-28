package app

import (
	"context"
	"errors"
	"fmt"
	"reflect"

	cmd2 "github.com/kastheco/kasmos/cmd"
	"github.com/kastheco/kasmos/config"
	"github.com/kastheco/kasmos/config/auditlog"
	"github.com/kastheco/kasmos/config/planfsm"
	"github.com/kastheco/kasmos/config/planparser"
	"github.com/kastheco/kasmos/config/planstate"
	"github.com/kastheco/kasmos/config/planstore"
	"github.com/kastheco/kasmos/internal/clickup"
	"github.com/kastheco/kasmos/internal/mcpclient"
	sentrypkg "github.com/kastheco/kasmos/internal/sentry"
	"github.com/kastheco/kasmos/log"
	"github.com/kastheco/kasmos/session"
	"github.com/kastheco/kasmos/session/git"
	"github.com/kastheco/kasmos/session/tmux"
	"github.com/kastheco/kasmos/ui"
	"github.com/kastheco/kasmos/ui/overlay"
	"os"
	"path/filepath"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	zone "github.com/lrstanley/bubblezone"
)

const GlobalInstanceLimit = 20

const clickUpOpTimeout = 30 * time.Second

// Run is the main entrypoint into the application.
func Run(ctx context.Context, program string, autoYes bool) error {
	// Set the terminal's default background to the theme base color so every
	// ANSI reset and unstyled cell falls back to #232136 instead of black.
	restore := ui.SetTerminalBackground(string(ui.ColorBase))
	defer restore()
	defer sentrypkg.RecoverPanic()

	zone.NewGlobal()
	h := newHome(ctx, program, autoYes)
	defer h.auditLogger.Close()
	p := tea.NewProgram(
		h,
		tea.WithAltScreen(),
		tea.WithMouseAllMotion(), // Full mouse tracking for hover + scroll + click
	)
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
	// stateRenamePlan is the state when the user is renaming a plan.
	stateRenamePlan
	// stateRenameTopic is the state when the user is renaming a topic.
	stateRenameTopic
	// stateSendPrompt is the state when the user is sending a prompt via text overlay.
	stateSendPrompt
	// stateFocusAgent is the state when the user is typing directly into the agent pane.
	stateFocusAgent
	// stateContextMenu is the state when a right-click context menu is shown.
	stateContextMenu
	// stateRepoSwitch is the state when the user is switching repos via picker.
	stateRepoSwitch
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
	// statePermission is when an opencode permission prompt is detected and the modal is shown.
	statePermission
	// stateTmuxBrowser is the state when the tmux session browser overlay is shown.
	stateTmuxBrowser
	// stateChatAboutPlan is the state when the user is typing a question about a plan.
	stateChatAboutPlan
)

type home struct {
	ctx context.Context

	// -- Storage and Configuration --

	program string
	autoYes bool

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
	auditPane *ui.AuditPane
	// menu displays the bottom menu
	menu *ui.Menu
	// statusBar displays the top contextual status bar
	statusBar *ui.StatusBar
	// tabbedWindow displays the tabbed window with preview and diff panes
	tabbedWindow *ui.TabbedWindow
	// toastManager manages toast notifications
	toastManager *overlay.ToastManager
	// global spinner instance. we plumb this down to where it's needed
	spinner spinner.Model
	// textInputOverlay handles text input with state
	textInputOverlay *overlay.TextInputOverlay
	// formOverlay handles multi-field form input (plan creation)
	formOverlay *overlay.FormOverlay
	// textOverlay displays text information
	textOverlay *overlay.TextOverlay
	// confirmationOverlay displays confirmation modals
	confirmationOverlay *overlay.ConfirmationOverlay
	// pendingConfirmAction stores the tea.Cmd to run asynchronously when confirmed
	pendingConfirmAction tea.Cmd

	// nav handles unified navigation state
	// focusSlot tracks which pane has keyboard focus in the Tab ring:
	// 0=nav, 1=info tab, 2=agent tab, 3=diff tab
	focusSlot int
	// pendingPlanName stores the plan name during the two-step plan creation flow
	pendingPlanName string
	// pendingPlanDesc stores the plan description during the two-step plan creation flow
	pendingPlanDesc string
	// pendingPRTitle stores the PR title during the two-step PR creation flow
	pendingPRTitle string
	// pendingChangeTopicPlan stores the plan filename during the change-topic flow
	pendingChangeTopicPlan string
	// pendingSetStatusPlan stores the plan filename during the set-status flow
	pendingSetStatusPlan string
	// pendingChatAboutPlan stores the plan filename during the chat-about-plan flow
	pendingChatAboutPlan string
	// pendingPRToastID stores the toast ID for the in-progress PR creation
	pendingPRToastID string

	// contextMenu is the right-click context menu overlay
	contextMenu *overlay.ContextMenu
	// pickerOverlay is the topic picker overlay for move-to-topic
	pickerOverlay *overlay.PickerOverlay
	// tmuxBrowser is the tmux session browser overlay.
	tmuxBrowser *overlay.TmuxBrowserOverlay
	// tmuxSessionCount is the latest count of kas_-prefixed tmux sessions.
	tmuxSessionCount int
	// clickUpConfig stores the detected ClickUp MCP server config (nil if not detected)
	clickUpConfig *clickup.MCPServerConfig
	// clickUpImporter handles search/fetch via MCP (nil until first use)
	clickUpImporter *clickup.Importer
	// clickUpResults stores the latest search results for the picker
	clickUpResults []clickup.SearchResult

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

	// repoPickerMap maps picker display text to full repo path
	repoPickerMap map[string]string

	// planState holds the parsed plan-state.json for the active repo. Nil when missing.
	planState *planstate.PlanState
	// planStateDir is the directory containing plan-state.json (docs/plans/ of active repo).
	planStateDir string
	// planStore is the remote plan store client. Nil when unconfigured or unreachable.
	planStore planstore.Store
	// planStoreProject is the project name used with the remote store (derived from repo basename).
	planStoreProject string
	// auditLogger records structured audit events to the planstore SQLite database.
	// Falls back to NopLogger when planstore is HTTP-backed or unconfigured.
	auditLogger auditlog.Logger

	// previewTickCount counts preview ticks for throttled banner animation
	previewTickCount int

	// cachedPlanFile is the filename of the last rendered plan (for cache hit).
	cachedPlanFile string
	// cachedPlanRendered is the glamour-rendered markdown of cachedPlanFile.
	cachedPlanRendered string

	// waveOrchestrators tracks active wave orchestrations by plan filename.
	waveOrchestrators map[string]*WaveOrchestrator

	// pendingAllComplete holds plan files whose all-waves-complete prompt was
	// deferred because an overlay was active when the orchestrator finished.
	// Drained on each metadata tick once the overlay clears.
	pendingAllComplete []string

	// pendingWaveConfirmPlanFile is set while a wave-advance (or failed-wave decision)
	// confirmation overlay is showing, so cancel can reset the orchestrator latch.
	pendingWaveConfirmPlanFile string
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

	// deferredPlannerDialogs holds plan files whose PlannerFinished dialog
	// could not be shown because an overlay was active at signal-processing time.
	// On each metadata tick, any queued plans are shown once the overlay clears.
	deferredPlannerDialogs []string

	// pendingPlannerInstanceTitle is the title of the planner instance that
	// triggered the current planner-exit confirmation dialog.
	pendingPlannerInstanceTitle string

	// pendingPlannerPlanFile is the plan file associated with the planner instance
	// that triggered the current planner-exit confirmation dialog. Set by the
	// PlannerFinished signal handler so cancel/esc handlers can mark plannerPrompted
	// without needing to look up the (possibly already removed) instance by title.
	pendingPlannerPlanFile string

	// fsm is the sole writer of plan-state.json. All plan status mutations flow
	// through fsm.Transition — direct SetStatus calls are not allowed.
	fsm *planfsm.PlanStateMachine

	// pendingReviewFeedback holds review feedback from sentinel files, keyed by
	// plan filename, to be injected as context for the next coder session.
	pendingReviewFeedback map[string]string

	// -- Permission prompt handling --

	// permissionOverlay is the modal shown when an opencode permission prompt is detected.
	permissionOverlay *overlay.PermissionOverlay
	// pendingPermissionInstance is the instance that triggered the permission modal.
	pendingPermissionInstance *session.Instance
	// permissionCache caches "allow always" decisions keyed by permission pattern.
	permissionCache *config.PermissionCache
	// permissionHandled tracks in-flight auto-approvals: instance → pattern.
	// Prevents duplicate key sequences when the pane still shows the prompt
	// across multiple metadata ticks while opencode processes the first response.
	// Cleared when the pane no longer contains a permission prompt for that instance.
	permissionHandled map[*session.Instance]string
}

func newHome(ctx context.Context, program string, autoYes bool) *home {
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
		tabbedWindow:          ui.NewTabbedWindow(ui.NewPreviewPane(), ui.NewDiffPane(), ui.NewInfoPane()),
		storage:               storage,
		appConfig:             appConfig,
		program:               program,
		autoYes:               autoYes,
		state:                 stateDefault,
		appState:              appState,
		activeRepoPath:        activeRepoPath,
		planStateDir:          filepath.Join(activeRepoPath, "docs", "plans"),
		planStoreProject:      project,
		instanceFinalizers:    make(map[*session.Instance]func()),
		waveOrchestrators:     make(map[string]*WaveOrchestrator),
		plannerPrompted:       make(map[string]bool),
		pendingReviewFeedback: make(map[string]string),
	}

	// Initialize remote plan store if configured.
	if appConfig.PlanStore != "" {
		store, err := planstore.NewStoreFromConfig(appConfig.PlanStore, project)
		if err != nil {
			log.WarningLog.Printf("plan store config error: %v", err)
		} else if store != nil {
			if pingErr := store.Ping(); pingErr != nil {
				log.WarningLog.Printf("plan store unreachable: %v", pingErr)
				// store remains nil — will show toast after toastManager is initialized
			} else {
				h.planStore = store
			}
		}
	}

	if h.planStore != nil {
		h.fsm = planfsm.NewWithStore(h.planStore, project, h.planStateDir)
	} else {
		h.fsm = planfsm.New(h.planStateDir)
	}

	// Initialize audit logger. When planstore is SQLite-backed (no PlanStore URL),
	// share the same DB file so both tables coexist without conflicts.
	// When planstore is HTTP-backed or unconfigured, use a no-op logger.
	if appConfig.PlanStore == "" {
		// SQLite-backed: open (or create) the shared planstore DB for audit events.
		dbPath := planstore.ResolvedDBPath()
		if al, err := auditlog.NewSQLiteLogger(dbPath); err != nil {
			log.WarningLog.Printf("audit logger init failed: %v", err)
			h.auditLogger = auditlog.NopLogger()
		} else {
			h.auditLogger = al
		}
	} else {
		// HTTP-backed or unconfigured: discard audit events for now.
		h.auditLogger = auditlog.NopLogger()
	}

	h.nav = ui.NewNavigationPanel(&h.spinner)
	h.toastManager = overlay.NewToastManager(&h.spinner)

	// Show a warning toast if the plan store was configured but unreachable.
	if appConfig.PlanStore != "" && h.planStore == nil {
		h.toastManager.Error("plan store unreachable — changes won't persist")
	}

	permCacheDir := filepath.Join(activeRepoPath, ".kasmos")
	permCache := config.NewPermissionCache(permCacheDir)
	_ = permCache.Load()
	h.permissionCache = permCache
	h.permissionHandled = make(map[*session.Instance]string)

	h.nav.SetRepoName(filepath.Base(activeRepoPath))
	h.tabbedWindow.SetAnimateBanner(appConfig.AnimateBanner)
	h.setFocusSlot(slotNav)
	h.loadPlanState()

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

	h.updateSidebarPlans()

	// Reconstruct in-memory wave orchestrators for plans that were mid-wave
	// when kasmos was last restarted. Must run after loadPlanState and instance load.
	h.rebuildOrphanedOrchestrators()

	// Persist the active repo so it appears in the picker even if it has no instances
	if state, ok := h.appState.(*config.State); ok {
		state.AddRecentRepo(activeRepoPath)
	}

	return h
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
	// Detect actual terminal resize vs spurious tea.WindowSize() side-effects.
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
		auditInnerW := navWidth - 4 // border (2) + padding (2)
		auditH := 8                 // 1 header + 7 event lines
		if contentHeight < 20 {
			auditH = 5 // compact for small terminals
		}
		m.auditPane.SetSize(auditInnerW, auditH)
		m.nav.SetAuditView(m.auditPane.String(), auditH)
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
	// Many handlers emit tea.WindowSize() as a batched side-effect (e.g.
	// instanceStartedMsg) — those fire with the same dimensions and should
	// not overwrite the overlay's explicit sizing.
	if m.textInputOverlay != nil && termResized {
		m.textInputOverlay.SetSize(int(float32(msg.Width)*0.6), int(float32(msg.Height)*0.4))
	}
	if m.textOverlay != nil && termResized {
		m.textOverlay.SetWidth(int(float32(msg.Width) * 0.6))
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
	case prErrorMsg:
		log.ErrorLog.Printf("%v", msg.err)
		m.toastManager.Resolve(msg.id, overlay.ToastError, msg.err.Error())
		m.pendingPRToastID = ""
		return m, m.toastTickCmd()
	case planRenderedMsg:
		if msg.err != nil {
			return m, m.handleError(msg.err)
		}
		m.cachedPlanFile = msg.planFile
		m.cachedPlanRendered = msg.rendered
		m.tabbedWindow.SetActiveTab(ui.PreviewTab)
		m.tabbedWindow.SetDocumentContent(msg.rendered)
		return m, nil
	case previewTickMsg:
		// If previewTerminal is active, render from it (zero-latency VT emulator).
		if m.previewTerminal != nil && !m.tabbedWindow.IsDocumentMode() {
			if content, changed := m.previewTerminal.Render(); changed {
				m.tabbedWindow.SetPreviewContent(content)
			}
		} else if m.previewTerminal == nil && !m.tabbedWindow.IsDocumentMode() {
			// No terminal — show appropriate fallback state.
			selected := m.nav.GetSelectedInstance()
			if selected != nil && selected.Started() && selected.Status != session.Paused && selected.Status != session.Loading {
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
		return m, func() tea.Msg {
			if term != nil {
				term.WaitForRender(16 * time.Millisecond)
			} else {
				time.Sleep(50 * time.Millisecond)
			}
			return previewTickMsg{}
		}
	case keyupMsg:
		m.menu.ClearKeydown()
		return m, nil
	case clickUpDetectedMsg:
		m.clickUpConfig = &msg.Config
		m.nav.SetClickUpAvailable(true)
		return m, nil
	case clickUpSearchResultMsg:
		if msg.Err != nil {
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
		m.pickerOverlay = overlay.NewPickerOverlay("select clickup task", items)
		return m, nil
	case tickUpdateMetadataMessage:
		// Snapshot the instance list for the goroutine. The slice header is
		// copied but the pointers are shared — CollectMetadata only reads
		// instance fields that don't change between ticks (started, Status,
		// tmuxSession, gitWorktree, Program).
		instances := m.nav.GetInstances()
		snapshots := make([]*session.Instance, len(instances))
		copy(snapshots, instances)
		planStateDir := m.planStateDir // snapshot for goroutine
		store := m.planStore           // snapshot for goroutine
		project := m.planStoreProject  // snapshot for goroutine

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
					DiffStats:          md.DiffStats,
					CPUPercent:         md.CPUPercent,
					MemMB:              md.MemMB,
					ResourceUsageValid: md.ResourceUsageValid,
					TmuxAlive:          md.TmuxAlive,
					PermissionPrompt:   md.PermissionPrompt,
				})
			}

			// Load plan state — moved here from the synchronous Update handler
			// to avoid blocking the event loop every 500ms.
			// When a remote store is configured, use LoadWithStore to keep
			// in-memory state fresh from the server.
			var ps *planstate.PlanState
			if planStateDir != "" {
				var loaded *planstate.PlanState
				var err error
				if store != nil {
					loaded, err = planstate.LoadWithStore(store, project, planStateDir)
				} else {
					loaded, err = planstate.Load(planStateDir)
				}
				if err != nil {
					log.WarningLog.Printf("could not load plan state: %v", err)
				} else {
					ps = loaded
				}
			}

			var signals []planfsm.Signal
			if planStateDir != "" {
				signals = planfsm.ScanSignals(planStateDir)
			}

			// Also scan signals from active worktrees — agents write
			// sentinel files relative to their CWD which is the worktree,
			// not the main repo.
			seen := make(map[string]bool)
			for _, sig := range signals {
				seen[sig.Key()] = true
			}
			for _, inst := range snapshots {
				wt := inst.GetWorktreePath()
				if wt == "" {
					continue
				}
				wtPlansDir := filepath.Join(wt, "docs", "plans")
				for _, sig := range planfsm.ScanSignals(wtPlansDir) {
					if !seen[sig.Key()] {
						seen[sig.Key()] = true
						signals = append(signals, sig)
					}
				}
			}

			var waveSignals []planfsm.WaveSignal
			if planStateDir != "" {
				waveSignals = planfsm.ScanWaveSignals(planStateDir)
			}

			tmuxCount := tmux.CountKasSessions(cmd2.MakeExecutor())
			time.Sleep(200 * time.Millisecond)
			return metadataResultMsg{Results: results, PlanState: ps, Signals: signals, WaveSignals: waveSignals, TmuxSessionCount: tmuxCount}
		}
	case metadataResultMsg:
		// Process agent sentinel signals — feed to FSM and consume sentinel files.
		// Done in Update (main goroutine) so FSM writes are never concurrent.
		// Side-effect cmds (reviewer/coder spawns) are collected and batched below.
		var signalCmds []tea.Cmd
		for _, sig := range msg.Signals {
			// Guard: if a wave orchestrator is active for this plan, ignore
			// implement-finished signals. Wave task agents may write this sentinel
			// after completing their individual task, but the wave orchestrator
			// owns the implementing→reviewing transition. Without this guard,
			// the first task to finish prematurely triggers review and pauses
			// sibling tasks, spawning a "applying fixes" coder alongside the
			// reviewer.
			if sig.Event == planfsm.ImplementFinished {
				if _, hasOrch := m.waveOrchestrators[sig.PlanFile]; hasOrch {
					log.WarningLog.Printf("ignoring implement-finished signal for %q — wave orchestrator active", sig.PlanFile)
					planfsm.ConsumeSignal(sig)
					continue
				}
			}

			if err := m.fsm.Transition(sig.PlanFile, sig.Event); err != nil {
				log.WarningLog.Printf("signal %s for %s rejected: %v", sig.Event, sig.PlanFile, err)
				planfsm.ConsumeSignal(sig)
				continue
			}
			planfsm.ConsumeSignal(sig)

			// Side effects: spawn agents in response to successful transitions.
			switch sig.Event {
			case planfsm.ImplementFinished:
				// Pause the coder that wrote this signal.
				for _, inst := range m.nav.GetInstances() {
					if inst.PlanFile == sig.PlanFile && inst.AgentType == session.AgentTypeCoder {
						inst.ImplementationComplete = true
						_ = inst.Pause()
						break
					}
				}
				if cmd := m.spawnReviewer(sig.PlanFile); cmd != nil {
					signalCmds = append(signalCmds, cmd)
				}
			case planfsm.ReviewChangesRequested:
				feedback := sig.Body
				m.pendingReviewFeedback[sig.PlanFile] = feedback
				// Pause the reviewer that wrote this signal.
				for _, inst := range m.nav.GetInstances() {
					if inst.PlanFile == sig.PlanFile && inst.IsReviewer {
						_ = inst.Pause()
						break
					}
				}
				if cmd := m.spawnCoderWithFeedback(sig.PlanFile, feedback); cmd != nil {
					signalCmds = append(signalCmds, cmd)
				}
			case planfsm.PlannerFinished:
				capturedPlanFile := sig.PlanFile
				if m.plannerPrompted[capturedPlanFile] {
					break
				}
				if m.isUserInOverlay() {
					// Overlay is active — defer the dialog to the next tick
					// instead of silently dropping it. The sentinel has already
					// been consumed and the FSM transitioned; we must not lose
					// the "show dialog" side effect.
					m.deferredPlannerDialogs = append(m.deferredPlannerDialogs, capturedPlanFile)
					break
				}
				// Focus the planner instance so the user sees its output behind the overlay.
				for _, inst := range m.nav.GetInstances() {
					if inst.PlanFile == sig.PlanFile && inst.AgentType == session.AgentTypePlanner {
						if cmd := m.focusInstanceForOverlay(inst); cmd != nil {
							signalCmds = append(signalCmds, cmd)
						}
						m.pendingPlannerInstanceTitle = inst.Title
						break
					}
				}
				m.pendingPlannerPlanFile = capturedPlanFile
				m.confirmAction(
					fmt.Sprintf("plan '%s' is ready. start implementation?", planstate.DisplayName(capturedPlanFile)),
					func() tea.Msg {
						return plannerCompleteMsg{planFile: capturedPlanFile}
					},
				)
			}
		}
		if len(msg.Signals) > 0 {
			m.loadPlanState() // refresh after signal processing
		}

		// Retry deferred PlannerFinished dialogs — show the first queued plan
		// whose dialog was skipped because an overlay was active at signal time.
		if len(m.deferredPlannerDialogs) > 0 && !m.isUserInOverlay() {
			planFile := m.deferredPlannerDialogs[0]
			m.deferredPlannerDialogs = m.deferredPlannerDialogs[1:]
			if !m.plannerPrompted[planFile] {
				for _, inst := range m.nav.GetInstances() {
					if inst.PlanFile == planFile && inst.AgentType == session.AgentTypePlanner {
						if cmd := m.focusInstanceForOverlay(inst); cmd != nil {
							signalCmds = append(signalCmds, cmd)
						}
						m.pendingPlannerInstanceTitle = inst.Title
						break
					}
				}
				m.pendingPlannerPlanFile = planFile
				m.confirmAction(
					fmt.Sprintf("plan '%s' is ready. start implementation?", planstate.DisplayName(planFile)),
					func() tea.Msg {
						return plannerCompleteMsg{planFile: planFile}
					},
				)
			}
		}

		// Process wave signals — trigger implementation for specific waves.
		for _, ws := range msg.WaveSignals {
			planfsm.ConsumeWaveSignal(ws)

			// Check if orchestrator already exists
			if _, exists := m.waveOrchestrators[ws.PlanFile]; exists {
				m.toastManager.Error(fmt.Sprintf("wave already running for '%s'", planstate.DisplayName(ws.PlanFile)))
				continue
			}

			// Read and parse the plan
			plansDir := filepath.Join(m.activeRepoPath, "docs", "plans")
			content, err := os.ReadFile(filepath.Join(plansDir, ws.PlanFile))
			if err != nil {
				log.WarningLog.Printf("wave signal: could not read plan %s: %v", ws.PlanFile, err)
				continue
			}
			plan, err := planparser.Parse(string(content))
			if err != nil {
				m.toastManager.Error(fmt.Sprintf("plan '%s' has no wave headers", planstate.DisplayName(ws.PlanFile)))
				continue
			}

			if ws.WaveNumber > len(plan.Waves) {
				m.toastManager.Error(fmt.Sprintf("plan has %d waves, requested wave %d", len(plan.Waves), ws.WaveNumber))
				continue
			}

			entry, ok := m.planState.Entry(ws.PlanFile)
			if !ok {
				log.WarningLog.Printf("wave signal: plan %s not found in plan state", ws.PlanFile)
				continue
			}

			orch := NewWaveOrchestrator(ws.PlanFile, plan)
			m.waveOrchestrators[ws.PlanFile] = orch

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
			if md.PermissionPrompt != nil && m.state == stateDefault {
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
				} else if cacheKey != "" && m.permissionCache != nil && m.permissionCache.IsAllowedAlways(cacheKey) {
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
					m.permissionOverlay = overlay.NewPermissionOverlay(inst.Title, pp.Description, pp.Pattern)
					m.permissionOverlay.SetWidth(55)
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

			if md.DiffStats != nil {
				inst.SetDiffStats(md.DiffStats)
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

		// Apply plan state loaded in the goroutine (replaces synchronous loadPlanState call).
		// Skip when signals were processed: loadPlanState() above already gave us fresh state.
		// msg.PlanState was loaded before signals were scanned, so it would be stale.
		if msg.PlanState != nil && len(msg.Signals) == 0 {
			m.planState = msg.PlanState
		}

		// Store the latest tmux session count for the bottom bar.
		m.tmuxSessionCount = msg.TmuxSessionCount
		m.menu.SetTmuxSessionCount(m.tmuxSessionCount)

		if m.planState != nil {
			tmuxAliveMap := make(map[string]bool, len(msg.Results))
			for _, md := range msg.Results {
				tmuxAliveMap[md.Title] = md.TmuxAlive
			}

			// Coder-exit → push-prompt: when a coder session's tmux pane has exited
			// and the plan is still in StatusImplementing, prompt the user to push the
			// implementation branch before advancing to reviewing.
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
				entry := m.planState.Plans[inst.PlanFile]
				if !shouldPromptPushAfterCoderExit(entry, inst, alive) {
					continue
				}
				// Wave task instances never trigger the single-coder completion flow.
				// Wave completion is handled by the orchestrator, not the coder-exit prompt.
				if inst.TaskNumber > 0 {
					continue
				}
				// Focus the coder instance so the user can see its output behind the overlay.
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
					m.audit(auditlog.EventAgentFinished, fmt.Sprintf("agent finished: %s", inst.Title),
						auditlog.WithInstance(inst.Title),
						auditlog.WithAgent(inst.AgentType),
						auditlog.WithPlan(inst.PlanFile),
					)
				}
			}

			// Drain deferred all-complete prompts that were blocked by an overlay.
			if !m.isUserInOverlay() && len(m.pendingAllComplete) > 0 {
				planFile := m.pendingAllComplete[0]
				m.pendingAllComplete = m.pendingAllComplete[1:]
				planName := planstate.DisplayName(planFile)
				if cmd := m.focusPlanInstanceForOverlay(planFile); cmd != nil {
					asyncCmds = append(asyncCmds, cmd)
				}
				message := fmt.Sprintf("all waves complete for '%s'. push branch and start review?", planName)
				m.confirmAction(message, func() tea.Msg {
					return waveAllCompleteMsg{planFile: planFile}
				})
			}

			// Wave completion monitoring: check task completion and trigger wave transitions.
			// We process both WaveStateRunning (check task statuses) and WaveStateWaveComplete
			// (re-show confirm dialog after user cancelled, resetting the latch via ResetConfirm).
			for planFile, orch := range m.waveOrchestrators {
				orchState := orch.State()
				if orchState != WaveStateRunning && orchState != WaveStateWaveComplete {
					continue
				}

				if orchState == WaveStateRunning {
					// Check task status updates only while the wave is actively running.
					planName := planstate.DisplayName(planFile)
					for _, task := range orch.CurrentWaveTasks() {
						taskTitle := fmt.Sprintf("%s-W%d-T%d", planName, orch.CurrentWaveNumber(), task.Number)
						inst, exists := instanceMap[taskTitle]
						if !exists {
							// No matching instance — treat as failed (e.g. spawn crashed).
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
						if inst.PromptDetected && !inst.AwaitingWork {
							orch.MarkTaskComplete(task.Number)
							inst.SetStatus(session.Ready)
						} else if !alive {
							orch.MarkTaskFailed(task.Number)
						}
					}
					orchState = orch.State() // refresh after task updates
				}

				// All waves complete — pause the last wave's tasks, prompt for review.
				if orchState == WaveStateAllComplete {
					capturedPlanFile := planFile
					planName := planstate.DisplayName(planFile)

					// Pause all task instances (they're done, free up resources).
					for _, inst := range m.nav.GetInstances() {
						if inst.PlanFile == capturedPlanFile && inst.TaskNumber > 0 {
							inst.ImplementationComplete = true
							_ = inst.Pause()
						}
					}
					delete(m.waveOrchestrators, planFile)
					m.audit(auditlog.EventWaveCompleted, "all waves complete: "+planName,
						auditlog.WithPlan(capturedPlanFile))

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

				// orchState must be WaveStateWaveComplete here.
				// Show wave decision confirm once per wave (NeedsConfirm is one-shot;
				// ResetConfirm on cancel allows the prompt to reappear next tick).
				if !m.isUserInOverlay() && time.Since(m.waveConfirmDismissedAt) > 30*time.Second && orch.NeedsConfirm() {
					waveNum := orch.CurrentWaveNumber()
					completed := orch.CompletedTaskCount()
					failed := orch.FailedTaskCount()
					total := completed + failed
					entry, _ := m.planState.Entry(planFile)

					capturedPlanFile := planFile
					capturedEntry := entry
					planName := planstate.DisplayName(planFile)
					// Focus a task instance so the user can see agent output behind the overlay.
					if cmd := m.focusPlanInstanceForOverlay(capturedPlanFile); cmd != nil {
						asyncCmds = append(asyncCmds, cmd)
					}
					if failed > 0 {
						m.audit(auditlog.EventWaveFailed,
							fmt.Sprintf("wave %d: %d/%d tasks failed", waveNum, failed, total),
							auditlog.WithPlan(capturedPlanFile),
							auditlog.WithWave(waveNum, 0))
						message := fmt.Sprintf(
							"%s — wave %d: %d/%d tasks complete, %d failed.\n\n"+
								"[r] retry failed   [n] next wave   [a] abort",
							planName, waveNum, completed, total, failed)
						m.waveFailedConfirmAction(message, capturedPlanFile, capturedEntry)
					} else {
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

		m.updateSidebarPlans()
		m.updateInfoPane()
		completionCmd := m.checkPlanCompletion()
		asyncCmds = append(asyncCmds, signalCmds...)
		asyncCmds = append(asyncCmds, tickUpdateMetadataCmd, completionCmd)
		// Restart toast tick loop if any toasts were created during this tick
		// (e.g. by transitionToReview or spawnCoderWithFeedback).
		if m.toastManager.HasActiveToasts() {
			asyncCmds = append(asyncCmds, m.toastTickCmd())
		}
		return m, tea.Batch(asyncCmds...)
	case tea.MouseMsg:
		return m.handleMouse(msg)
	case tea.KeyMsg:
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
		if msg.err != nil || selected == nil || selected.Title != msg.instanceTitle {
			if msg.term != nil {
				msg.term.Close()
			}
			return m, nil
		}
		m.previewTerminal = msg.term
		m.previewTerminalInstance = msg.instanceTitle
		return m, nil
	case killInstanceMsg:
		// Async pre-kill checks passed — safe to mutate model in Update.
		m.nav.Kill()
		m.removeFromAllInstances(msg.title)
		m.saveAllInstances()
		m.updateNavPanelStatus()
		return m, tea.Batch(tea.WindowSize(), m.instanceChanged())
	case planStageConfirmedMsg:
		// User confirmed past the topic-concurrency gate — execute the stage.
		return m.executePlanStage(msg.planFile, msg.stage)
	case planRefreshMsg:
		// Reload plan state and refresh sidebar after async plan mutation.
		m.loadPlanState()
		m.updateSidebarPlans()
		return m, tea.WindowSize()
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
		planName := planstate.DisplayName(msg.planFile)
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
			if inst.PlanFile == msg.planFile && inst.TaskNumber > 0 {
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
			planstate.DisplayName(msg.planFile)))
		return m, tea.Batch(tea.WindowSize(), m.instanceChanged(), m.toastTickCmd())
	case waveAllCompleteMsg:
		// All waves finished and user confirmed — push branch and advance to review.
		planFile := msg.planFile
		planName := planstate.DisplayName(planFile)

		// Push the implementation branch (best-effort, non-blocking).
		for _, inst := range m.nav.GetInstances() {
			if inst.PlanFile == planFile && inst.TaskNumber > 0 {
				worktree, err := inst.GetGitWorktree()
				if err == nil {
					_ = worktree.PushChanges(
						fmt.Sprintf("[kas] push completed implementation for '%s'", planName),
						false,
					)
					break // one push is enough — all tasks share the worktree
				}
			}
		}

		// Transition FSM implementing → reviewing.
		if err := m.fsm.Transition(planFile, planfsm.ImplementFinished); err != nil {
			log.WarningLog.Printf("wave all-complete: could not transition %q to reviewing: %v", planFile, err)
		}
		m.loadPlanState()
		m.updateSidebarPlans()

		// Spawn reviewer agent for the completed plan.
		var reviewerCmd tea.Cmd
		if cmd := m.spawnReviewer(planFile); cmd != nil {
			reviewerCmd = cmd
		}
		m.toastManager.Info(fmt.Sprintf("all waves complete for '%s' — starting review", planName))
		return m, tea.Batch(tea.WindowSize(), reviewerCmd, m.toastTickCmd())
	case coderCompleteMsg:
		// Single-coder (non-wave) implementation finished and user confirmed push.
		// Transition FSM and spawn reviewer — mirrors waveAllCompleteMsg flow.
		planFile := msg.planFile
		planName := planstate.DisplayName(planFile)

		if err := m.fsm.Transition(planFile, planfsm.ImplementFinished); err != nil {
			log.WarningLog.Printf("coder-complete: could not transition %q to reviewing: %v", planFile, err)
		}

		// Mark the coder instance as implementation-complete and pause it.
		for _, inst := range m.nav.GetInstances() {
			if inst.PlanFile == planFile && inst.AgentType == session.AgentTypeCoder {
				inst.ImplementationComplete = true
				_ = inst.Pause()
				break
			}
		}

		m.loadPlanState()
		m.updateSidebarPlans()

		var reviewerCmd tea.Cmd
		if cmd := m.spawnReviewer(planFile); cmd != nil {
			reviewerCmd = cmd
		}
		m.toastManager.Info(fmt.Sprintf("implementation complete for '%s' — starting review", planName))
		return m, tea.Batch(tea.WindowSize(), reviewerCmd, m.toastTickCmd())
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
				items[i].PlanFile = inst.PlanFile
				items[i].AgentType = inst.AgentType
				items[i].Status = statusString(inst.Status)
			}
		}
		m.tmuxBrowser = overlay.NewTmuxBrowserOverlay(items)
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
		return m, tea.Batch(tea.WindowSize(), m.toastTickCmd())
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
			m.pickerOverlay = overlay.NewPickerOverlay(pickerTitle, topicNames)
			m.pickerOverlay.SetAllowCustom(true)
			m.state = stateNewPlanTopic
			return m, nil
		}
		// Safety net: if title arrives while already in topic picker, update silently
		if msg.err == nil && msg.title != "" {
			if m.state == stateNewPlanTopic && m.pendingPlanDesc != "" {
				m.pendingPlanName = msg.title
				if m.pickerOverlay != nil {
					m.pickerOverlay.SetTitle(
						fmt.Sprintf("assign to topic for '%s'", msg.title),
					)
					return m, tea.WindowSize()
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
		m.pendingPlannerPlanFile = ""
		m.updateNavPanelStatus()
		return m.triggerPlanStage(msg.planFile, "implement")
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
		return m, tea.Batch(tea.WindowSize(), m.instanceChanged())
	case folderPickedMsg:
		m.state = stateDefault
		m.pickerOverlay = nil
		if msg.err != nil {
			return m, m.handleError(msg.err)
		}
		if msg.path != "" {
			m.activeRepoPath = msg.path
			m.nav.SetRepoName(filepath.Base(msg.path))
			if state, ok := m.appState.(*config.State); ok {
				state.AddRecentRepo(msg.path)
			}
			m.rebuildInstanceList()
		}
		return m, nil
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
			_ = m.saveAllInstances()
			return tea.QuitMsg{}
		}
		return m, m.confirmAction("quit kasmos? active sessions will be preserved.", quitAction)
	}

	if err := m.saveAllInstances(); err != nil {
		return m, m.handleError(err)
	}
	return m, tea.Quit
}

func (m *home) View() string {
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

	var result string
	switch {
	case m.state == stateSendPrompt && m.textInputOverlay != nil:
		result = overlay.PlaceOverlay(0, 0, m.textInputOverlay.Render(), mainView, true, true)
	case m.state == statePRTitle && m.textInputOverlay != nil:
		result = overlay.PlaceOverlay(0, 0, m.textInputOverlay.Render(), mainView, true, true)
	case m.state == statePRBody && m.textInputOverlay != nil:
		result = overlay.PlaceOverlay(0, 0, m.textInputOverlay.Render(), mainView, true, true)
	case m.state == stateRenameInstance && m.textInputOverlay != nil:
		result = overlay.PlaceOverlay(0, 0, m.textInputOverlay.Render(), mainView, true, true)
	case m.state == stateRenamePlan && m.textInputOverlay != nil:
		result = overlay.PlaceOverlay(0, 0, m.textInputOverlay.Render(), mainView, true, true)
	case m.state == stateNewPlan && m.textInputOverlay != nil:
		result = overlay.PlaceOverlay(0, 0, m.textInputOverlay.Render(), mainView, true, true)
	case m.state == stateSpawnAgent && m.formOverlay != nil:
		result = overlay.PlaceOverlay(0, 0, m.formOverlay.Render(), mainView, true, true)
	case m.state == stateNewPlanTopic && m.pickerOverlay != nil:
		result = overlay.PlaceOverlay(0, 0, m.pickerOverlay.Render(), mainView, true, true)
	case m.state == stateClickUpSearch && m.textInputOverlay != nil:
		result = overlay.PlaceOverlay(0, 0, m.textInputOverlay.Render(), mainView, true, true)
	case m.state == stateClickUpPicker && m.pickerOverlay != nil:
		result = overlay.PlaceOverlay(0, 0, m.pickerOverlay.Render(), mainView, true, true)
	case m.state == stateChangeTopic && m.pickerOverlay != nil:
		result = overlay.PlaceOverlay(0, 0, m.pickerOverlay.Render(), mainView, true, true)
	case m.state == stateSetStatus && m.pickerOverlay != nil:
		result = overlay.PlaceOverlay(0, 0, m.pickerOverlay.Render(), mainView, true, true)
	case m.state == stateRepoSwitch && m.pickerOverlay != nil:
		// Position near the repo button at the bottom of the sidebar
		pickerX := 1
		pickerY := m.contentHeight - 10 // above the bottom menu, near the repo indicator
		if pickerY < 2 {
			pickerY = 2
		}
		result = overlay.PlaceOverlay(pickerX, pickerY, m.pickerOverlay.Render(), mainView, true, false)
	case m.state == statePrompt:
		if m.textInputOverlay == nil {
			log.ErrorLog.Printf("text input overlay is nil")
		}
		result = overlay.PlaceOverlay(0, 0, m.textInputOverlay.Render(), mainView, true, true)
	case m.state == stateHelp:
		if m.textOverlay == nil {
			log.ErrorLog.Printf("text overlay is nil")
		}
		result = overlay.PlaceOverlay(0, 0, m.textOverlay.Render(), mainView, true, true)
	case m.state == stateConfirm:
		if m.confirmationOverlay == nil {
			log.ErrorLog.Printf("confirmation overlay is nil")
		}
		result = overlay.PlaceOverlay(0, 0, m.confirmationOverlay.Render(), mainView, true, true)
	case m.state == statePermission && m.permissionOverlay != nil:
		result = overlay.PlaceOverlay(0, 0, m.permissionOverlay.Render(), mainView, true, true)
	case m.state == stateContextMenu && m.contextMenu != nil:
		cx, cy := m.contextMenu.GetPosition()
		result = overlay.PlaceOverlay(cx, cy, m.contextMenu.Render(), mainView, true, false)
	case m.state == stateChatAboutPlan && m.textInputOverlay != nil:
		result = overlay.PlaceOverlay(0, 0, m.textInputOverlay.Render(), mainView, true, true)
	case m.state == stateTmuxBrowser && m.tmuxBrowser != nil:
		result = overlay.PlaceOverlay(0, 0, m.tmuxBrowser.Render(), mainView, true, true)
	default:
		result = mainView
	}

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

	return result
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

// planStageConfirmedMsg is sent when the user confirms proceeding past the
// topic-concurrency gate. Re-enters plan stage execution skipping the
// concurrency check that was already acknowledged.
type planStageConfirmedMsg struct {
	planFile string
	stage    string
}

// planRefreshMsg triggers a plan state reload and sidebar refresh in Update.
type planRefreshMsg struct{}

// waveAdvanceMsg is sent when the user confirms advancing to the next wave.
type waveAdvanceMsg struct {
	planFile string
	entry    planstate.PlanEntry
}

// waveRetryMsg is sent when the user chooses "retry" on the failed-wave decision prompt.
type waveRetryMsg struct {
	planFile string
	entry    planstate.PlanEntry
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
	DiffStats          *git.DiffStats
	CPUPercent         float64
	MemMB              float64
	ResourceUsageValid bool
	TmuxAlive          bool
	PermissionPrompt   *session.PermissionPrompt // non-nil when opencode shows a permission dialog
}

// metadataResultMsg carries all per-instance metadata collected by the async tick.
type metadataResultMsg struct {
	Results          []instanceMetadata
	PlanState        *planstate.PlanState // pre-loaded plan state (nil if dir not set)
	Signals          []planfsm.Signal     // agent sentinel files found this tick
	WaveSignals      []planfsm.WaveSignal // implement-wave-N signal files found this tick
	TmuxSessionCount int                  // number of kas_-prefixed tmux sessions
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
			return clickUpSearchResultMsg{Err: normalizeClickUpError(err)}
		}

		searchDone := make(chan clickUpSearchResultMsg, 1)
		go func() {
			results, searchErr := importer.Search(query)
			searchDone <- clickUpSearchResultMsg{Results: results, Err: searchErr}
		}()

		select {
		case msg := <-searchDone:
			msg.Err = normalizeClickUpError(msg.Err)
			return msg
		case <-ctx.Done():
			return clickUpSearchResultMsg{Err: normalizeClickUpError(ctx.Err())}
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
			return msg
		case <-ctx.Done():
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

	m.clickUpImporter = clickup.NewImporter(client)
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
	path := mcpclient.TokenPath()
	tok, err := mcpclient.LoadToken(path)
	if err == nil && !tok.IsExpired() {
		return tok.AccessToken, nil
	}

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
