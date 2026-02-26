package app

import (
	"context"
	"errors"
	"fmt"
	"reflect"

	"github.com/kastheco/kasmos/config"
	"github.com/kastheco/kasmos/config/planfsm"
	"github.com/kastheco/kasmos/config/planstate"
	"github.com/kastheco/kasmos/internal/clickup"
	"github.com/kastheco/kasmos/internal/mcpclient"
	sentrypkg "github.com/kastheco/kasmos/internal/sentry"
	"github.com/kastheco/kasmos/log"
	"github.com/kastheco/kasmos/session"
	"github.com/kastheco/kasmos/session/git"
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

const GlobalInstanceLimit = 10

const clickUpOpTimeout = 30 * time.Second

// Run is the main entrypoint into the application.
func Run(ctx context.Context, program string, autoYes bool) error {
	// Set the terminal's default background to the theme base color so every
	// ANSI reset and unstyled cell falls back to #232136 instead of black.
	restore := ui.SetTerminalBackground(string(ui.ColorBase))
	defer restore()
	defer sentrypkg.RecoverPanic()

	zone.NewGlobal()
	p := tea.NewProgram(
		newHome(ctx, program, autoYes),
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
	// stateClickUpSearch is the state when the user is typing a ClickUp search query.
	stateClickUpSearch
	// stateClickUpPicker is the state when the user is picking from ClickUp search results.
	stateClickUpPicker
	// stateClickUpFetching is when kasmos is fetching a full task from ClickUp.
	stateClickUpFetching
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
	// 0=nav, 1=agent tab, 2=diff tab, 3=info tab
	focusSlot int
	// pendingPlanName stores the plan name during the two-step plan creation flow
	pendingPlanName string
	// pendingPlanDesc stores the plan description during the two-step plan creation flow
	pendingPlanDesc string
	// pendingPRTitle stores the PR title during the two-step PR creation flow
	pendingPRTitle string
	// pendingChangeTopicPlan stores the plan filename during the change-topic flow
	pendingChangeTopicPlan string
	// pendingPRToastID stores the toast ID for the in-progress PR creation
	pendingPRToastID string

	// contextMenu is the right-click context menu overlay
	contextMenu *overlay.ContextMenu
	// pickerOverlay is the topic picker overlay for move-to-topic
	pickerOverlay *overlay.PickerOverlay
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

	// previewTickCount counts preview ticks for throttled banner animation
	previewTickCount int

	// cachedPlanFile is the filename of the last rendered plan (for cache hit).
	cachedPlanFile string
	// cachedPlanRendered is the glamour-rendered markdown of cachedPlanFile.
	cachedPlanRendered string

	// waveOrchestrators tracks active wave orchestrations by plan filename.
	waveOrchestrators map[string]*WaveOrchestrator

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

	// pendingPlannerInstanceTitle is the title of the planner instance that
	// triggered the current planner-exit confirmation dialog.
	pendingPlannerInstanceTitle string

	// fsm is the sole writer of plan-state.json. All plan status mutations flow
	// through fsm.Transition — direct SetStatus calls are not allowed.
	fsm *planfsm.PlanStateMachine

	// pendingReviewFeedback holds review feedback from sentinel files, keyed by
	// plan filename, to be injected as context for the next coder session.
	pendingReviewFeedback map[string]string
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

	h := &home{
		ctx:                   ctx,
		spinner:               spinner.New(spinner.WithSpinner(spinner.Dot)),
		menu:                  ui.NewMenu(),
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
		instanceFinalizers:    make(map[*session.Instance]func()),
		waveOrchestrators:     make(map[string]*WaveOrchestrator),
		plannerPrompted:       make(map[string]bool),
		pendingReviewFeedback: make(map[string]string),
	}
	h.fsm = planfsm.New(h.planStateDir)
	h.nav = ui.NewNavigationPanel(&h.spinner)
	h.toastManager = overlay.NewToastManager(&h.spinner)

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
	h.updateNavPanelStatus()

	// Reconstruct in-memory wave orchestrators for plans that were mid-wave
	// when kasmos was last restarted. Must run after loadPlanState and instance load.
	h.rebuildOrphanedOrchestrators()

	// Persist the active repo so it appears in the picker even if it has no instances
	if state, ok := h.appState.(*config.State); ok {
		state.AddRecentRepo(activeRepoPath)
	}

	return h
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
	m.termWidth = msg.Width
	m.termHeight = msg.Height
	m.toastManager.SetSize(msg.Width, msg.Height)
	if m.statusBar != nil {
		m.statusBar.SetSize(msg.Width)
	}

	m.tabbedWindow.SetSize(tabsWidth, contentHeight)
	m.nav.SetSize(navWidth, contentHeight)

	// Store for mouse hit-testing
	m.navWidth = navWidth
	m.tabsWidth = tabsWidth
	m.contentHeight = contentHeight

	if navWidth == 0 && m.focusSlot == slotNav {
		m.setFocusSlot(slotAgent)
	}

	if m.textInputOverlay != nil {
		m.textInputOverlay.SetSize(int(float32(msg.Width)*0.6), int(float32(msg.Height)*0.4))
	}
	if m.textOverlay != nil {
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
				})
			}

			// Load plan state from disk — moved here from the synchronous
			// Update handler to avoid blocking the event loop every 500ms.
			var ps *planstate.PlanState
			if planStateDir != "" {
				loaded, err := planstate.Load(planStateDir)
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

			time.Sleep(500 * time.Millisecond)
			return metadataResultMsg{Results: results, PlanState: ps, Signals: signals}
		}
	case metadataResultMsg:
		// Process agent sentinel signals — feed to FSM and consume sentinel files.
		// Done in Update (main goroutine) so FSM writes are never concurrent.
		// Side-effect cmds (reviewer/coder spawns) are collected and batched below.
		var signalCmds []tea.Cmd
		for _, sig := range msg.Signals {
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
			}
		}
		if len(msg.Signals) > 0 {
			m.loadPlanState() // refresh after signal processing
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

		// Inline reviewer completion check using cached TmuxAlive from metadata
		// (replaces checkReviewerCompletion which called tmux has-session per reviewer).
		if m.planState != nil {
			tmuxAliveMap := make(map[string]bool, len(msg.Results))
			for _, md := range msg.Results {
				tmuxAliveMap[md.Title] = md.TmuxAlive
			}
			for _, inst := range m.nav.GetInstances() {
				if inst.PlanFile == "" || !inst.IsReviewer || !inst.Started() || inst.Paused() {
					continue
				}
				alive, collected := tmuxAliveMap[inst.Title]
				if !collected || alive {
					continue
				}
				entry := m.planState.Plans[inst.PlanFile]
				if entry.Status != planstate.StatusReviewing {
					continue
				}
				// Reviewer death → ReviewApproved: one-shot FSM transition, rare event.
				if err := m.fsm.Transition(inst.PlanFile, planfsm.ReviewApproved); err != nil {
					log.WarningLog.Printf("could not mark plan %q completed: %v", inst.PlanFile, err)
				}
			}

			// Planner-exit → implement-prompt: fires when a planner pane dies and the
			// plan is StatusPlanning (no sentinel written, tmux-death fallback) or
			// StatusReady (sentinel processed this tick, FSM transitioned). Skip if
			// already prompted (yes/no answered) or if a confirm overlay is active.
			for _, inst := range m.nav.GetInstances() {
				if m.state == stateConfirm {
					break
				}
				if inst.AgentType != session.AgentTypePlanner || inst.PlanFile == "" {
					continue
				}
				if m.plannerPrompted[inst.PlanFile] {
					continue
				}
				alive, collected := tmuxAliveMap[inst.Title]
				if !collected || alive {
					continue
				}
				entry, ok := m.planState.Entry(inst.PlanFile)
				// Fire for StatusPlanning (crash fallback) and StatusReady (sentinel path).
				if !ok || (entry.Status != planstate.StatusPlanning && entry.Status != planstate.StatusReady) {
					continue
				}
				capturedPlanFile := inst.PlanFile
				capturedTitle := inst.Title
				m.confirmAction(
					fmt.Sprintf("plan '%s' is ready. start implementation?", planstate.DisplayName(capturedPlanFile)),
					func() tea.Msg {
						return plannerCompleteMsg{planFile: capturedPlanFile}
					},
				)
				m.pendingPlannerInstanceTitle = capturedTitle
				break // one prompt per tick
			}

			// Coder-exit → push-prompt: when a coder session's tmux pane has exited
			// and the plan is still in StatusImplementing, prompt the user to push the
			// implementation branch before advancing to reviewing.
			// Skip when a confirmation overlay is already showing to avoid re-prompting
			// on every tick while the user is deciding.
			for _, inst := range m.nav.GetInstances() {
				if m.state == stateConfirm {
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
				if cmd := m.promptPushBranchThenAdvance(inst); cmd != nil {
					asyncCmds = append(asyncCmds, cmd)
				}
				// Only prompt for one instance per tick to avoid stacking overlays.
				break
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

					if m.state != stateConfirm {
						message := fmt.Sprintf("all waves complete for '%s'. push branch and start review?", planName)
						m.confirmAction(message, func() tea.Msg {
							return waveAllCompleteMsg{planFile: capturedPlanFile}
						})
					}
					continue
				}

				// orchState must be WaveStateWaveComplete here.
				// Show wave decision confirm once per wave (NeedsConfirm is one-shot;
				// ResetConfirm on cancel allows the prompt to reappear next tick).
				if m.state != stateConfirm && time.Since(m.waveConfirmDismissedAt) > 30*time.Second && orch.NeedsConfirm() {
					waveNum := orch.CurrentWaveNumber()
					completed := orch.CompletedTaskCount()
					failed := orch.FailedTaskCount()
					total := completed + failed
					entry, _ := m.planState.Entry(planFile)

					capturedPlanFile := planFile
					capturedEntry := entry
					planName := planstate.DisplayName(planFile)
					if failed > 0 {
						message := fmt.Sprintf(
							"%s — wave %d: %d/%d tasks complete, %d failed.\n\n"+
								"[r] retry failed   [n] next wave   [a] abort",
							planName, waveNum, completed, total, failed)
						m.waveFailedConfirmAction(message, capturedPlanFile, capturedEntry)
					} else {
						message := fmt.Sprintf("%s — wave %d complete (%d/%d). start wave %d?",
							planName, waveNum, completed, total, waveNum+1)
						m.waveStandardConfirmAction(message, capturedPlanFile, capturedEntry)
					}
				}
			}
		}

		m.updateSidebarPlans()
		m.updateNavPanelStatus()
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
		m.updateNavPanelStatus()
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
		m.updateNavPanelStatus()

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
		m.updateNavPanelStatus()

		var reviewerCmd tea.Cmd
		if cmd := m.spawnReviewer(planFile); cmd != nil {
			reviewerCmd = cmd
		}
		m.toastManager.Info(fmt.Sprintf("implementation complete for '%s' — starting review", planName))
		return m, tea.Batch(tea.WindowSize(), reviewerCmd, m.toastTickCmd())
	case plannerCompleteMsg:
		// User confirmed: start implementation. Kill the dead planner instance first.
		m.plannerPrompted[msg.planFile] = true
		if m.pendingPlannerInstanceTitle != "" {
			m.removeFromAllInstances(m.pendingPlannerInstanceTitle)
			m.saveAllInstances()
		}
		m.pendingPlannerInstanceTitle = ""
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
	case m.state == stateNewPlan && m.formOverlay != nil:
		result = overlay.PlaceOverlay(0, 0, m.formOverlay.Render(), mainView, true, true)
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
	case m.state == stateContextMenu && m.contextMenu != nil:
		cx, cy := m.contextMenu.GetPosition()
		result = overlay.PlaceOverlay(cx, cy, m.contextMenu.Render(), mainView, true, false)
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

// prCreatedMsg is sent when async PR creation succeeds.
type prCreatedMsg struct{}

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
}

// metadataResultMsg carries all per-instance metadata collected by the async tick.
type metadataResultMsg struct {
	Results   []instanceMetadata
	PlanState *planstate.PlanState // pre-loaded plan state (nil if dir not set)
	Signals   []planfsm.Signal     // agent sentinel files found this tick
}

// tickUpdateMetadataCmd is the callback to update the metadata of the instances every 500ms. Note that we iterate
// overall the instances and capture their output. It's a pretty expensive operation. Let's do it 2x a second only.
var tickUpdateMetadataCmd = func() tea.Msg {
	time.Sleep(500 * time.Millisecond)
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
