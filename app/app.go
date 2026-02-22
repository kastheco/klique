package app

import (
	"context"
	"fmt"
	"github.com/kastheco/klique/config"
	"github.com/kastheco/klique/config/planstate"
	"github.com/kastheco/klique/log"
	"github.com/kastheco/klique/session"
	"github.com/kastheco/klique/session/git"
	"github.com/kastheco/klique/ui"
	"github.com/kastheco/klique/ui/overlay"
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
	// newInstanceFinalizer is called when the state is stateNew and then you press enter.
	// It registers the new instance in the list after the instance has been started.
	newInstanceFinalizer func()
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
	// focusedPanel tracks which panel has keyboard focus: 0=sidebar (left), 1=preview/center, 2=instance list (right)
	focusedPanel int
	// pendingPlanName stores the plan name during the three-step plan creation flow
	pendingPlanName string
	// pendingPlanDesc stores the plan description during the three-step plan creation flow
	pendingPlanDesc string
	// pendingPRTitle stores the PR title during the two-step PR creation flow
	pendingPRTitle string
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
		ctx:            ctx,
		spinner:        spinner.New(spinner.WithSpinner(spinner.Dot)),
		menu:           ui.NewMenu(),
		tabbedWindow:   ui.NewTabbedWindow(ui.NewPreviewPane(), ui.NewDiffPane(), ui.NewGitPane()),
		storage:        storage,
		appConfig:      appConfig,
		program:        program,
		autoYes:        autoYes,
		state:          stateDefault,
		appState:       appState,
		activeRepoPath: activeRepoPath,
		planStateDir:   filepath.Join(activeRepoPath, "docs", "plans"),
	}
	h.list = ui.NewList(&h.spinner, autoYes)
	h.toastManager = overlay.NewToastManager(&h.spinner)
	h.sidebar = ui.NewSidebar()
	h.sidebar.SetRepoName(filepath.Base(activeRepoPath))
	h.tabbedWindow.SetAnimateBanner(appConfig.AnimateBanner)
	h.setFocus(0) // Start with sidebar focused
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

	h.updateSidebarItems()

	// Persist the active repo so it appears in the picker even if it has no instances
	if state, ok := h.appState.(*config.State); ok {
		state.AddRecentRepo(activeRepoPath)
	}

	return h
}

// updateHandleWindowSizeEvent sets the sizes of the components.
// The components will try to render inside their bounds.
func (m *home) updateHandleWindowSizeEvent(msg tea.WindowSizeMsg) {
	// Three-column layout: sidebar (15%), instance list (20%), preview (remaining)
	var sidebarWidth int
	if m.sidebarHidden {
		sidebarWidth = 0
	} else {
		sidebarWidth = int(float32(msg.Width) * 0.18)
		if sidebarWidth < 20 {
			sidebarWidth = 20
		}
	}
	listWidth := int(float32(msg.Width) * 0.20)
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
				})
			}
			time.Sleep(500 * time.Millisecond)
			return metadataResultMsg{Results: results}
		}
	case metadataResultMsg:
		// Apply collected metadata to instances — zero I/O, just field writes.
		instanceMap := make(map[string]*session.Instance)
		for _, inst := range m.list.GetInstances() {
			instanceMap[inst.Title] = inst
		}

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
					if md.Content != "" {
						inst.LastActivity = session.ParseActivity(md.Content, inst.Program)
					}
				} else {
					if md.HasPrompt {
						inst.PromptDetected = true
						inst.TapEnter()
					} else {
						inst.SetStatus(session.Ready)
					}
					if inst.Status != session.Running {
						inst.LastActivity = nil
					}
				}
			}

			// Deliver queued prompt
			if inst.QueuedPrompt != "" && (inst.Status == session.Ready || inst.PromptDetected) {
				if err := inst.SendPrompt(inst.QueuedPrompt); err != nil {
					log.WarningLog.Printf("could not send queued prompt to %q: %v", inst.Title, err)
				}
				inst.QueuedPrompt = ""
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

		// Refresh plan state and sidebar (these are cheap — JSON parse + in-memory rebuild)
		m.loadPlanState()
		m.checkReviewerCompletion()
		m.updateSidebarPlans()
		m.updateSidebarItems()
		completionCmd := m.checkPlanCompletion()
		return m, tea.Batch(tickUpdateMetadataCmd, completionCmd)
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
	case planRefreshMsg:
		m.loadPlanState()
		m.updateSidebarPlans()
		m.updateSidebarItems()
		return m, nil
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
		if m.newInstanceFinalizer != nil {
			m.newInstanceFinalizer()
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
	sidebarView := colStyle.Render(m.sidebar.String())
	listWithPadding := colStyle.Render(m.list.String())
	previewWithPadding := colStyle.Render(m.tabbedWindow.String())
	// Layout: sidebar | preview (center/main) | instance list (right)
	var listAndPreview string
	if m.sidebarHidden {
		listAndPreview = lipgloss.JoinHorizontal(lipgloss.Top, previewWithPadding, listWithPadding)
	} else {
		listAndPreview = lipgloss.JoinHorizontal(lipgloss.Top, sidebarView, previewWithPadding, listWithPadding)
	}

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

// planRefreshMsg triggers a plan state reload and sidebar refresh.
// Returned from confirmation callbacks that write plan state changes.
type planRefreshMsg struct{}

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
}

// metadataResultMsg carries all per-instance metadata collected by the async tick.
type metadataResultMsg struct {
	Results []instanceMetadata
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
