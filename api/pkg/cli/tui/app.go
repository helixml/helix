package tui

import (
	"context"
	"fmt"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/helixml/helix/api/pkg/types"
)

// AppMode represents the top-level mode of the TUI.
type AppMode int

const (
	ModePicker AppMode = iota
	ModeMain          // kanban + tabs + panes
)

// App is the top-level bubbletea model.
type App struct {
	api  *APIClient
	tmux *TmuxConfig
	conn *ConnectionManager

	mode       AppMode
	width      int
	height     int
	prefixNext bool // true when prefix key was just pressed

	picker *PickerModel
	kanban *KanbanModel
	tabs   *TabBar

	// Modal overlays (only one active at a time)
	taskPicker *TaskPickerModel
	newTask    *NewTaskModel

	pendingRestore *TUIState

	err    error
	status string
}

// Messages
type errMsg struct{ err error }
type statusMsg string
type tickMsg time.Time
type restorePanesMsg struct {
	state *TUIState
	tasks []*types.SpecTask
}

const pollInterval = 10 * time.Second

func NewApp(api *APIClient, projectID string) *App {
	tmuxCfg := LoadTmuxConfig()

	app := &App{
		api:  api,
		tmux: tmuxCfg,
		conn: NewConnectionManager(),
		tabs: NewTabBar(),
	}

	if projectID != "" {
		app.mode = ModeMain
		app.kanban = NewKanbanModel(api, projectID)
	} else {
		if state := LoadState(); state != nil && state.ProjectID != "" {
			app.mode = ModeMain
			app.kanban = NewKanbanModel(api, state.ProjectID)
			if state.Panes != nil {
				app.pendingRestore = state
			}
		} else {
			app.mode = ModePicker
			app.picker = NewPickerModel(api)
		}
	}

	return app
}

func (a *App) Init() tea.Cmd {
	cmds := []tea.Cmd{a.tickCmd()}

	switch a.mode {
	case ModePicker:
		cmds = append(cmds, a.picker.Init())
	case ModeMain:
		cmds = append(cmds, a.kanban.Init())
	}

	return tea.Batch(cmds...)
}

func (a *App) tickCmd() tea.Cmd {
	return tea.Tick(pollInterval, func(t time.Time) tea.Msg {
		return tickMsg(t)
	})
}

func (a *App) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		a.width = msg.Width
		a.height = msg.Height
		a.updateSizes()
		return a, nil

	case tea.KeyMsg:
		return a.handleKey(msg)

	case tickMsg:
		cmds := []tea.Cmd{a.tickCmd()}
		// Refresh kanban if on kanban tab
		if a.mode == ModeMain && a.kanban != nil && a.isOnKanbanTab() {
			cmds = append(cmds, a.kanban.fetchTasks())
		}
		return a, tea.Batch(cmds...)

	case projectSelectedMsg:
		a.mode = ModeMain
		a.kanban = NewKanbanModel(a.api, msg.project.ID)
		a.kanban.SetProject(msg.project)
		a.updateSizes()
		return a, a.kanban.Init()

	case tasksLoadedMsg:
		a.conn.RecordSuccess()
		if a.kanban != nil {
			a.kanban.Update(msg)
		}
		if a.pendingRestore != nil && a.pendingRestore.Panes != nil {
			cmd := a.restorePanes(a.pendingRestore)
			a.pendingRestore = nil
			return a, cmd
		}
		return a, nil

	case openTaskChatMsg:
		return a, a.openTaskInTab(msg.task)

	case taskPickerDoneMsg:
		a.taskPicker = nil
		tab := a.tabs.ActiveTab()
		if tab != nil && tab.Panes != nil {
			chat := NewChatModel(a.api, msg.task)
			tab.Panes.SplitFocused(msg.splitDir, chat)
			a.syncPaneFocus()
			return a, chat.Init()
		}
		return a, nil

	case taskPickerCancelMsg:
		a.taskPicker = nil
		return a, nil

	case newTaskCreatedMsg:
		a.newTask = nil
		a.status = "Task created: " + taskDisplayName(msg.task)
		if a.kanban != nil {
			return a, a.kanban.fetchTasks()
		}
		return a, nil

	case newTaskCancelMsg:
		a.newTask = nil
		return a, nil

	case openNewTaskMsg:
		if a.kanban != nil {
			a.newTask = NewNewTaskModel(a.api, a.kanban.projectID)
			a.newTask.SetSize(a.width, a.contentHeight())
		}
		return a, nil

	case restorePanesMsg:
		taskMap := make(map[string]*types.SpecTask)
		for _, t := range msg.tasks {
			taskMap[t.ID] = t
		}
		return a, a.applyRestoredPanes(msg.state, taskMap)

	case specApprovedMsg:
		a.status = "Specs approved! Task moving to implementation."
		if a.kanban != nil {
			return a, a.kanban.fetchTasks()
		}
		return a, nil

	case errMsg:
		a.err = msg.err
		a.conn.RecordFailure(msg.err)
		return a, nil

	case statusMsg:
		a.status = string(msg)
		return a, nil

	case spinnerTickMsg:
		// Forward to focused chat
		if chat := a.focusedChat(); chat != nil {
			cmd := chat.Update(msg)
			return a, cmd
		}
		return a, nil
	}

	// Delegate to active sub-model
	var cmd tea.Cmd

	// Modal overlays take priority
	if a.taskPicker != nil {
		cmd = a.taskPicker.Update(msg)
		return a, cmd
	}
	if a.newTask != nil {
		cmd = a.newTask.Update(msg)
		return a, cmd
	}

	switch a.mode {
	case ModePicker:
		cmd = a.picker.Update(msg)
	case ModeMain:
		if a.isOnKanbanTab() {
			cmd = a.kanban.Update(msg)
		} else if chat := a.focusedChat(); chat != nil {
			cmd = chat.Update(msg)
		}
	}
	return a, cmd
}

func (a *App) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	key := msg.String()

	if key == "ctrl+c" {
		a.saveState()
		return a, tea.Quit
	}

	// Modal overlays
	if a.taskPicker != nil {
		cmd := a.taskPicker.Update(msg)
		return a, cmd
	}
	if a.newTask != nil {
		cmd := a.newTask.Update(msg)
		return a, cmd
	}

	// Prefix key handling
	if a.mode == ModeMain && !a.isOnKanbanTab() {
		if a.prefixNext {
			a.prefixNext = false
			return a.handlePrefixedKey(key)
		}
		if key == a.tmux.Prefix {
			a.prefixNext = true
			return a, nil
		}
	}

	// Kanban-mode prefix (for tab switching from kanban)
	if a.mode == ModeMain && a.isOnKanbanTab() {
		if a.prefixNext {
			a.prefixNext = false
			return a.handlePrefixedKey(key)
		}
		if key == a.tmux.Prefix {
			a.prefixNext = true
			return a, nil
		}
	}

	// Mode-specific quit
	if key == "q" {
		switch a.mode {
		case ModePicker:
			return a, tea.Quit
		case ModeMain:
			if a.isOnKanbanTab() {
				a.saveState()
				return a, tea.Quit
			}
			// In chat tab, 'q' goes to input
		}
	}

	// Delegate
	var cmd tea.Cmd
	switch a.mode {
	case ModePicker:
		cmd = a.picker.Update(msg)
	case ModeMain:
		if a.isOnKanbanTab() {
			cmd = a.kanban.Update(msg)
		} else if chat := a.focusedChat(); chat != nil {
			cmd = chat.Update(msg)
		}
	}
	return a, cmd
}

func (a *App) handlePrefixedKey(key string) (tea.Model, tea.Cmd) {
	projectID := ""
	if a.kanban != nil {
		projectID = a.kanban.projectID
	}

	switch key {
	// Create new tab
	case "c":
		if projectID != "" {
			a.taskPicker = NewTaskPickerModel(a.api, projectID, SplitVertical)
			a.taskPicker.SetSize(a.width, a.contentHeight())
			// Reuse task picker but treat selection as "open in new tab"
			return a, a.taskPicker.Init()
		}

	// Split panes
	case a.tmux.SplitV:
		if projectID != "" && !a.isOnKanbanTab() {
			a.taskPicker = NewTaskPickerModel(a.api, projectID, SplitVertical)
			a.taskPicker.SetSize(a.width, a.contentHeight())
			return a, a.taskPicker.Init()
		}

	case a.tmux.SplitH:
		if projectID != "" && !a.isOnKanbanTab() {
			a.taskPicker = NewTaskPickerModel(a.api, projectID, SplitHorizontal)
			a.taskPicker.SetSize(a.width, a.contentHeight())
			return a, a.taskPicker.Init()
		}

	// Tab navigation
	case "n":
		a.tabs.NextTab()
		return a, nil
	case "p":
		a.tabs.PrevTab()
		return a, nil
	case "0", "1", "2", "3", "4", "5", "6", "7", "8", "9":
		idx := int(key[0] - '0')
		a.tabs.GoToTab(idx)
		return a, nil

	// Close tab
	case "&":
		a.tabs.CloseTab()
		return a, nil

	// Pane navigation
	case a.tmux.PaneNext:
		a.cyclePaneFocus(true)
		return a, nil
	case a.tmux.PanePrev:
		a.cyclePaneFocus(false)
		return a, nil

	// Close pane
	case a.tmux.ClosePane:
		tab := a.tabs.ActiveTab()
		if tab != nil && tab.Panes != nil {
			if !tab.Panes.CloseFocused() {
				// Last pane closed, close the tab
				a.tabs.CloseTab()
			}
			a.syncPaneFocus()
		}
		return a, nil

	// Detach
	case a.tmux.Detach:
		a.saveState()
		return a, tea.Quit

	// Terminal
	case "t":
		a.status = "Terminal: not yet implemented"
		return a, nil

	// Web URL
	case "w":
		if chat := a.focusedChat(); chat != nil && chat.task != nil {
			url := a.api.WebURL(chat.task.ProjectID, chat.task.ID)
			a.status = "Open: " + url
		}
		return a, nil
	}

	// Directional pane nav
	if a.tmux.PaneLeft != "" && key == a.tmux.PaneLeft {
		a.cyclePaneFocus(false)
		return a, nil
	}
	if a.tmux.PaneRight != "" && key == a.tmux.PaneRight {
		a.cyclePaneFocus(true)
		return a, nil
	}
	if a.tmux.PaneDown != "" && key == a.tmux.PaneDown {
		a.cyclePaneFocus(true)
		return a, nil
	}
	if a.tmux.PaneUp != "" && key == a.tmux.PaneUp {
		a.cyclePaneFocus(false)
		return a, nil
	}

	return a, nil
}

// --- Tab/pane helpers ---

func (a *App) isOnKanbanTab() bool {
	return a.tabs.ActiveIndex() == 0
}

func (a *App) openTaskInTab(task *types.SpecTask) tea.Cmd {
	// Check if task already has a tab
	existing := a.tabs.FindTabByTask(task.ID)
	if existing != nil {
		for i, t := range a.tabs.tabs {
			if t == existing {
				a.tabs.GoToTab(i)
				return nil
			}
		}
	}

	// Create new tab
	tab := a.tabs.AddTab(task)
	tab.Panes = NewPaneManager()
	tab.Panes.SetSize(a.width, a.contentHeight()-1) // -1 for tab bar

	chat := NewChatModel(a.api, task)
	tab.Panes.OpenPane(chat)
	a.syncPaneFocus()

	return chat.Init()
}

func (a *App) focusedChat() *ChatModel {
	tab := a.tabs.ActiveTab()
	if tab == nil || tab.Panes == nil {
		return nil
	}
	return tab.Panes.FocusedChat()
}

func (a *App) cyclePaneFocus(forward bool) {
	tab := a.tabs.ActiveTab()
	if tab == nil || tab.Panes == nil {
		return
	}
	if chat := tab.Panes.FocusedChat(); chat != nil {
		chat.SetFocused(false)
	}
	if forward {
		tab.Panes.FocusNext()
	} else {
		tab.Panes.FocusPrev()
	}
	a.syncPaneFocus()
}

func (a *App) syncPaneFocus() {
	tab := a.tabs.ActiveTab()
	if tab == nil || tab.Panes == nil {
		return
	}
	if chat := tab.Panes.FocusedChat(); chat != nil {
		chat.SetFocused(true)
	}
}

func (a *App) contentHeight() int {
	return a.height - 2 // status bar + tab bar
}

func (a *App) updateSizes() {
	ch := a.contentHeight()
	if a.picker != nil {
		a.picker.SetSize(a.width, ch)
	}
	if a.kanban != nil {
		a.kanban.SetSize(a.width, ch-1)
	}
	if a.taskPicker != nil {
		a.taskPicker.SetSize(a.width, ch)
	}
	a.tabs.SetWidth(a.width)
	// Update active tab's pane sizes
	tab := a.tabs.ActiveTab()
	if tab != nil && tab.Panes != nil {
		tab.Panes.SetSize(a.width, ch-1) // -1 for tab bar
	}
}

// --- State persistence ---

func (a *App) restorePanes(state *TUIState) tea.Cmd {
	taskIDs := collectTaskIDs(state.Panes)
	if len(taskIDs) == 0 {
		return nil
	}

	return func() tea.Msg {
		var tasks []*types.SpecTask
		for _, id := range taskIDs {
			task, err := a.api.GetSpecTask(apiCtx(), id)
			if err != nil {
				continue
			}
			tasks = append(tasks, task)
		}
		if len(tasks) == 0 {
			return statusMsg("Could not restore any panes")
		}
		return restorePanesMsg{state: state, tasks: tasks}
	}
}

func (a *App) applyRestoredPanes(state *TUIState, taskMap map[string]*types.SpecTask) tea.Cmd {
	var cmds []tea.Cmd

	// Open each task in a tab
	for _, id := range collectTaskIDs(state.Panes) {
		task, ok := taskMap[id]
		if !ok {
			continue
		}
		cmd := a.openTaskInTab(task)
		if cmd != nil {
			cmds = append(cmds, cmd)
		}
	}

	if len(cmds) > 0 {
		a.status = fmt.Sprintf("Restored %d tab(s)", len(cmds))
	}
	return tea.Batch(cmds...)
}

func (a *App) saveState() {
	state := BuildStateFromApp(a)
	if state.ProjectID != "" {
		_ = SaveState(state)
	}
}

// --- View ---

func (a *App) View() string {
	if a.width == 0 {
		return "Loading..."
	}

	// Connection bar (mosh-style)
	connBar := a.conn.RenderBar(a.width)

	var content string

	// Modal overlays
	if a.taskPicker != nil {
		content = a.taskPicker.View()
	} else if a.newTask != nil {
		content = a.newTask.View()
	} else {
		switch a.mode {
		case ModePicker:
			content = a.picker.View()
		case ModeMain:
			if a.isOnKanbanTab() {
				content = a.kanban.View()
			} else {
				tab := a.tabs.ActiveTab()
				if tab != nil && tab.Panes != nil {
					content = tab.Panes.Render()
				}
			}
		}
	}

	// Tab bar (only in main mode)
	tabBar := ""
	if a.mode == ModeMain && a.tabs.TabCount() > 1 {
		tabBar = a.tabs.View()
	}

	statusBar := a.renderStatusBar()

	parts := []string{}
	if connBar != "" {
		parts = append(parts, connBar)
	}
	parts = append(parts, content)
	if tabBar != "" {
		parts = append(parts, tabBar)
	}
	parts = append(parts, statusBar)

	result := ""
	for i, p := range parts {
		if i > 0 {
			result += "\n"
		}
		result += p
	}
	return result
}

func (a *App) renderStatusBar() string {
	style := styleStatusBar.Width(a.width)

	var help string
	switch a.mode {
	case ModePicker:
		help = "j/k: navigate  enter: select  q: quit"
	case ModeMain:
		prefix := a.tmux.Prefix
		if a.prefixNext {
			help = fmt.Sprintf("[%s] waiting for command...", prefix)
		} else if a.isOnKanbanTab() {
			help = "h/l: column  j/k: task  enter: open  n: new task  r: refresh  q: quit"
		} else {
			paneInfo := ""
			tab := a.tabs.ActiveTab()
			if tab != nil && tab.Panes != nil && tab.Panes.PaneCount() > 1 {
				paneInfo = fmt.Sprintf(" [%d panes]", tab.Panes.PaneCount())
			}
			help = fmt.Sprintf("%s: cmds  esc: stop/clear%s", prefix, paneInfo)
		}
	}

	if a.err != nil {
		help = styleError.Render(fmt.Sprintf("Error: %v", a.err)) + "  " + help
	}
	if a.status != "" {
		help = styleDim.Render(a.status) + "  " + help
	}

	return style.Render(help)
}

// Helper to create context for API calls.
func apiCtx() context.Context {
	return context.Background()
}
