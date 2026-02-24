package app

import (
	"context"
	"fmt"
	"github.com/kastheco/kasmos/config"
	"github.com/kastheco/kasmos/config/planfsm"
	"github.com/kastheco/kasmos/config/planstate"
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

// Run is the main entrypoint into the application.
func Run(ctx context.Context, program string, autoYes bool) error {
	// Set the terminal's default background to the theme base color so every
	// ANSI reset and unstyled cell falls back to #232136 instead of black.
	restore := ui.SetTerminalBackground(string(ui.ColorBase))
	defer restore()

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
	// stateNewPlanName is the state when the user is entering a plan name.
	stateNewPlanName
	// stateNewPlanDescription is the state when the user is entering a plan description.
	stateNewPlanDescription
	// stateNewPlanTopic is the state when the user is picking a topic for a new plan.
	stateNewPlanTopic
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

	// list displays the list of instances
	list *ui.List
	// menu displays the bottom menu
	menu *ui.Menu
	// tabbedWindow displays the tabbed window with preview and diff panes
	tabbedWindow *ui.TabbedWindow
	// toastManager manages toast notifications
	toastManager *overlay.ToastManager
	// global spinner instance. we plumb this down to where it's needed
	spinner spinner.Model
	// textInputOverlay handles text input with state
	textInputOverlay *overlay.TextInputOverlay
	// textOverlay displays text information
	textOverlay *overlay.TextOverlay
	// confirmationOverlay displays confirmation modals
	confirmationOverlay *overlay.ConfirmationOverlay
	// pendingConfirmAction stores the tea.Cmd to run asynchronously when confirmed
	pendingConfirmAction tea.Cmd

	// sidebar displays the topic sidebar
	sidebar *ui.Sidebar
	// focusSlot tracks which pane has keyboard focus in the Tab ring:
	// 0=sidebar, 1=agent tab, 2=diff tab, 3=git tab, 4=instance list
	focusSlot int
	// pendingPlanName stores the plan name during the three-step plan creation flow
	pendingPlanName string
	// pendingPlanDesc stores the plan description during the three-step plan creation flow
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

	// Layout dimensions for mouse hit-testing
	sidebarWidth  int
	listWidth     int
	tabsWidth     int
	contentHeight int

	// sidebarHidden tracks whether the sidebar is collapsed (ctrl+s toggle)
	sidebarHidden bool

	// Terminal dimensions for the global background fill.
	termWidth  int
	termHeight int

	// embeddedTerminal is the VT emulator for focus mode (nil when not in focus mode)
	embeddedTerminal *session.EmbeddedTerminal

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
		tabbedWindow:          ui.NewTabbedWindow(ui.NewPreviewPane(), ui.NewDiffPane(), ui.NewGitPane()),
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
	h.list = ui.NewList(&h.spinner, autoYes)
	h.toastManager = overlay.NewToastManager(&h.spinner)
	h.sidebar = ui.NewSidebar()
	h.sidebar.SetRepoName(filepath.Base(activeRepoPath))
	h.tabbedWindow.SetAnimateBanner(appConfig.AnimateBanner)
	h.setFocusSlot(slotSidebar) // Start with left sidebar focused
	h.loadPlanState()

	// Load saved instances
	instances, err := storage.LoadInstances()
	if err != nil {
		fmt.Printf("Failed to load instances: %v\n", err)
		os.Exit(1)
	}

	h.allInstances = instances

	// Add instances matching active repo to the list
	for _, instance := range instances {
		repoPath := instance.GetRepoPath()
		if repoPath == "" || repoPath == h.activeRepoPath {
			h.list.AddInstance(instance)()
			if autoYes {
				instance.AutoYes = true
			}
		}
	}

	h.updateSidebarPlans()
	h.updateSidebarItems()

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
	// Three-column layout: sidebar (18%), instance list (20%), preview (remaining)
	// The instance list column is hidden when there are no instances.
	var sidebarWidth int
	if m.sidebarHidden {
		sidebarWidth = 0
	} else {
		sidebarWidth = int(float32(msg.Width) * 0.25)
		if sidebarWidth < 20 {
			sidebarWidth = 20
		}
	}
	var listWidth int
	if m.list.TotalInstances() > 0 {
		listWidth = int(float32(msg.Width) * 0.20)
	}
	tabsWidth := msg.Width - sidebarWidth - listWidth

	// Keep the keybind rail compact and give the saved rows to the three columns.
	menuHeight := 1
	if msg.Height < 2 {
		menuHeight = 0
	}
	contentHeight := msg.Height - menuHeight
	if contentHeight < 1 {
		contentHeight = 1
	}
	m.termWidth = msg.Width
	m.termHeight = msg.Height
	m.toastManager.SetSize(msg.Width, msg.Height)

	m.tabbedWindow.SetSize(tabsWidth, contentHeight)
	m.list.SetSize(listWidth, contentHeight)
	m.sidebar.SetSize(sidebarWidth, contentHeight)

	// Store for mouse hit-testing
	m.sidebarWidth = sidebarWidth
	m.listWidth = listWidth
	m.tabsWidth = tabsWidth
	m.contentHeight = contentHeight

	// If the list column disappeared while it had focus, move to sidebar.
	if listWidth == 0 && m.focusSlot == slotList {
		m.setFocusSlot(slotSidebar)
	}

	if m.textInputOverlay != nil {
		m.textInputOverlay.SetSize(int(float32(msg.Width)*0.6), int(float32(msg.Height)*0.4))
	}
	if m.textOverlay != nil {
		m.textOverlay.SetWidth(int(float32(msg.Width) * 0.6))
	}

	previewWidth, previewHeight := m.tabbedWindow.GetPreviewSize()
	if err := m.list.SetSessionPreviewSize(previewWidth, previewHeight); err != nil {
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
		cmd := m.instanceChanged()
		// Advance banner animation every 20 ticks (~1s per frame at 50ms tick)
		m.previewTickCount++
		if m.previewTickCount%20 == 0 {
			m.tabbedWindow.TickBanner()
		}
		return m, tea.Batch(
			cmd,
			func() tea.Msg {
				time.Sleep(50 * time.Millisecond)
				return previewTickMsg{}
			},
		)
	case focusPreviewTickMsg:
		if m.state != stateFocusAgent || m.embeddedTerminal == nil {
			return m, nil
		}
		if content, changed := m.embeddedTerminal.Render(); changed {
			m.tabbedWindow.SetPreviewContent(content)
		}
		// Capture reference for the command goroutine — safe even if
		// exitFocusMode() nils m.embeddedTerminal before the command fires.
		term := m.embeddedTerminal
		return m, func() tea.Msg {
			// Block until new content is rendered or 50ms elapses.
			// This replaces the fixed 16ms sleep with event-driven wakeup,
			// cutting worst-case display latency from ~24ms to ~1-3ms.
			term.WaitForRender(50 * time.Millisecond)
			return focusPreviewTickMsg{}
		}
	case gitTabTickMsg:
		if !m.tabbedWindow.IsInGitTab() {
			return m, nil
		}
		gitPane := m.tabbedWindow.GetGitPane()
		if !gitPane.IsRunning() {
			return m, nil
		}
		// Only trigger re-render when content changed to avoid flicker
		content, changed := gitPane.Render()
		if changed {
			m.tabbedWindow.SetGitContent(content)
		}
		return m, func() tea.Msg {
			time.Sleep(33 * time.Millisecond)
			return gitTabTickMsg{}
		}
	case keyupMsg:
		m.menu.ClearKeydown()
		return m, nil
	case tickUpdateMetadataMessage:
		// Snapshot the instance list for the goroutine. The slice header is
		// copied but the pointers are shared — CollectMetadata only reads
		// instance fields that don't change between ticks (started, Status,
		// tmuxSession, gitWorktree, Program).
		instances := m.list.GetInstances()
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
				for _, inst := range m.list.GetInstances() {
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
				for _, inst := range m.list.GetInstances() {
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
		for _, inst := range m.list.GetInstances() {
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
		for _, inst := range m.list.GetInstances() {
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
			for _, inst := range m.list.GetInstances() {
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
			for _, inst := range m.list.GetInstances() {
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
					fmt.Sprintf("Plan '%s' is ready. Start implementation?", planstate.DisplayName(capturedPlanFile)),
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
			for _, inst := range m.list.GetInstances() {
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
						} else if !alive {
							orch.MarkTaskFailed(task.Number)
						}
					}
					orchState = orch.State() // refresh after task updates
				}

				// If all waves complete, delete orchestrator (coder-exit flow takes over).
				if orchState == WaveStateAllComplete {
					delete(m.waveOrchestrators, planFile)
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
							"%s — Wave %d: %d/%d tasks complete, %d failed.\n\n"+
								"[r] retry failed   [n] next wave   [a] abort",
							planName, waveNum, completed, total, failed)
						m.waveFailedConfirmAction(message, capturedPlanFile, capturedEntry)
					} else {
						message := fmt.Sprintf("%s — Wave %d complete (%d/%d). Start Wave %d?",
							planName, waveNum, completed, total, waveNum+1)
						m.waveStandardConfirmAction(message, capturedPlanFile, capturedEntry)
					}
				}
			}
		}

		m.updateSidebarPlans()
		m.updateSidebarItems()
		completionCmd := m.checkPlanCompletion()
		asyncCmds = append(asyncCmds, signalCmds...)
		asyncCmds = append(asyncCmds, tickUpdateMetadataCmd, completionCmd)
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
		m.updateSidebarItems()
		return m, m.instanceChanged()
	case killInstanceMsg:
		// Async pre-kill checks passed — safe to mutate model in Update.
		m.list.Kill()
		m.removeFromAllInstances(msg.title)
		m.saveAllInstances()
		m.updateSidebarItems()
		return m, tea.Batch(tea.WindowSize(), m.instanceChanged())
	case planRefreshMsg:
		// Reload plan state and refresh sidebar after async plan mutation.
		m.loadPlanState()
		m.updateSidebarPlans()
		m.updateSidebarItems()
		return m, tea.WindowSize()
	case waveAdvanceMsg:
		orch, ok := m.waveOrchestrators[msg.planFile]
		if !ok {
			return m, nil
		}
		// Pause completed wave's instances before starting the next.
		planName := planstate.DisplayName(msg.planFile)
		for _, task := range orch.CurrentWaveTasks() {
			taskTitle := fmt.Sprintf("%s-W%d-T%d", planName, orch.CurrentWaveNumber(), task.Number)
			for _, inst := range m.list.GetInstances() {
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
			if m.list.SelectInstance(inst) {
				m.list.Kill()
			}
			m.removeFromAllInstances(inst.Title)
		}
		m.saveAllInstances()
		m.updateSidebarItems()
		m.toastManager.Info(fmt.Sprintf("Wave orchestration aborted for %s",
			planstate.DisplayName(msg.planFile)))
		return m, tea.Batch(tea.WindowSize(), m.instanceChanged(), m.toastTickCmd())
	case plannerCompleteMsg:
		// User confirmed: start implementation. Kill the dead planner instance first.
		m.plannerPrompted[msg.planFile] = true
		if m.pendingPlannerInstanceTitle != "" {
			m.removeFromAllInstances(m.pendingPlannerInstanceTitle)
			m.saveAllInstances()
		}
		m.pendingPlannerInstanceTitle = ""
		m.updateSidebarItems()
		return m.triggerPlanStage(msg.planFile, "implement")
	case instanceStartedMsg:
		if msg.err != nil {
			m.list.Kill()
			m.updateSidebarItems()
			return m, m.handleError(msg.err)
		}
		// Instance started successfully — add to master list, save and finalize
		m.allInstances = append(m.allInstances, msg.instance)
		if err := m.saveAllInstances(); err != nil {
			return m, m.handleError(err)
		}
		m.updateSidebarItems()
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
			m.sidebar.SetRepoName(filepath.Base(msg.path))
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
	}
	return m, nil
}

func (m *home) handleQuit() (tea.Model, tea.Cmd) {
	m.killGitTab()
	if err := m.saveAllInstances(); err != nil {
		return m, m.handleError(err)
	}
	return m, tea.Quit
}

func (m *home) View() string {
	// All columns use identical padding and height for uniform alignment.
	colStyle := lipgloss.NewStyle().PaddingTop(1).Height(m.contentHeight + 1)
	previewWithPadding := colStyle.Render(m.tabbedWindow.String())

	// Layout: sidebar | instance list (middle) | preview/tabs (right)
	// The instance list column is omitted when there are no instances.
	var cols []string
	if !m.sidebarHidden {
		cols = append(cols, colStyle.Render(m.sidebar.String()))
	}
	if m.listWidth > 0 {
		cols = append(cols, colStyle.Render(m.list.String()))
	}
	cols = append(cols, previewWithPadding)
	listAndPreview := lipgloss.JoinHorizontal(lipgloss.Top, cols...)

	mainView := lipgloss.JoinVertical(
		lipgloss.Left,
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
	case m.state == stateNewPlanName && m.textInputOverlay != nil:
		result = overlay.PlaceOverlay(0, 0, m.textInputOverlay.Render(), mainView, true, true)
	case m.state == stateNewPlanDescription && m.textInputOverlay != nil:
		result = overlay.PlaceOverlay(0, 0, m.textInputOverlay.Render(), mainView, true, true)
	case m.state == stateNewPlanTopic && m.pickerOverlay != nil:
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
	result = zone.Scan(result)

	// Height-fill — ensure enough lines for bubbletea's alt-screen renderer.
	// OSC 11 handles the actual background color; this just pads vertically.
	result = ui.FillBackground(result, m.termHeight)

	return result
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

// focusPreviewTickMsg is a fast ticker (30fps) for focus mode preview refresh only.
type focusPreviewTickMsg struct{}

// gitTabTickMsg is a 30fps ticker for refreshing the git tab's lazygit rendering.
type gitTabTickMsg struct{}

type instanceChangedMsg struct{}

// killInstanceMsg is sent after async pre-kill checks pass (worktree not checked out).
// Model mutations (list.Kill, removeFromAllInstances) happen in Update, not in the goroutine.
type killInstanceMsg struct {
	title string
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

// plannerCompleteMsg is sent when the user confirms starting implementation
// after a planner session finishes.
type plannerCompleteMsg struct {
	planFile string
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
