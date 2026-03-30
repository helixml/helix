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
	ModeKanban
	ModePanes
)

// App is the top-level bubbletea model.
type App struct {
	api  *APIClient
	tmux *TmuxConfig

	mode       AppMode
	width      int
	height     int
	prefixNext bool // true when prefix key was just pressed

	picker     *PickerModel
	kanban     *KanbanModel
	panes      *PaneManager
	taskPicker *TaskPickerModel // non-nil when picking a task for split
	newTask    *NewTaskModel    // non-nil when creating a new task

	pendingRestore *TUIState // non-nil if we need to restore panes after tasks load

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
		api:   api,
		tmux:  tmuxCfg,
		panes: NewPaneManager(),
	}

	if projectID != "" {
		// Skip picker, go straight to kanban
		app.mode = ModeKanban
		app.kanban = NewKanbanModel(api, projectID)
	} else {
		// Check for saved state to reattach
		if state := LoadState(); state != nil && state.ProjectID != "" {
			app.mode = ModeKanban
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
	case ModeKanban:
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
		contentH := a.height - 2 // status bar
		if a.picker != nil {
			a.picker.SetSize(a.width, contentH)
		}
		if a.kanban != nil {
			a.kanban.SetSize(a.width, contentH)
		}
		if a.panes != nil {
			a.panes.SetSize(a.width, contentH)
		}
		if a.taskPicker != nil {
			a.taskPicker.SetSize(a.width, contentH)
		}
		return a, nil

	case tea.KeyMsg:
		return a.handleKey(msg)

	case projectSelectedMsg:
		a.mode = ModeKanban
		a.kanban = NewKanbanModel(a.api, msg.project.ID)
		a.kanban.SetProject(msg.project)
		a.kanban.SetSize(a.width, a.height-2)
		return a, a.kanban.Init()

	case tasksLoadedMsg:
		// Let kanban handle the message first
		if a.kanban != nil {
			a.kanban.Update(msg)
		}
		// Check if we need to restore panes from saved state
		if a.pendingRestore != nil && a.pendingRestore.Panes != nil {
			cmd := a.restorePanes(a.pendingRestore)
			a.pendingRestore = nil
			return a, cmd
		}
		return a, nil

	case openTaskChatMsg:
		chat := NewChatModel(a.api, msg.task)
		a.mode = ModePanes
		a.panes.SetSize(a.width, a.height-2)
		a.panes.OpenPane(chat)
		return a, chat.Init()

	case taskPickerDoneMsg:
		a.taskPicker = nil
		chat := NewChatModel(a.api, msg.task)
		a.panes.SplitFocused(msg.splitDir, chat)
		a.syncPaneFocus()
		return a, chat.Init()

	case taskPickerCancelMsg:
		a.taskPicker = nil
		return a, nil

	case newTaskCreatedMsg:
		a.newTask = nil
		a.status = "Task created: " + taskDisplayName(msg.task)
		// Refresh kanban
		if a.kanban != nil {
			return a, a.kanban.fetchTasks()
		}
		return a, nil

	case newTaskCancelMsg:
		a.newTask = nil
		return a, nil

	case restorePanesMsg:
		taskMap := make(map[string]*types.SpecTask)
		for _, t := range msg.tasks {
			taskMap[t.ID] = t
		}
		return a, a.applyRestoredPanes(msg.state, taskMap)

	case openNewTaskMsg:
		if a.kanban != nil {
			a.newTask = NewNewTaskModel(a.api, a.kanban.projectID)
			a.newTask.SetSize(a.width, a.height-2)
		}
		return a, nil

	case tickMsg:
		// Background polling: refresh kanban when visible
		cmds := []tea.Cmd{a.tickCmd()}
		if a.mode == ModeKanban && a.kanban != nil {
			cmds = append(cmds, a.kanban.fetchTasks())
		}
		return a, tea.Batch(cmds...)

	case errMsg:
		a.err = msg.err
		return a, nil

	case statusMsg:
		a.status = string(msg)
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
	case ModeKanban:
		cmd = a.kanban.Update(msg)
	case ModePanes:
		if chat := a.panes.FocusedChat(); chat != nil {
			cmd = chat.Update(msg)
		}
	}
	return a, cmd
}

func (a *App) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	key := msg.String()

	// Global: quit
	if key == "ctrl+c" {
		a.saveState()
		return a, tea.Quit
	}

	// Modal overlays take all input
	if a.taskPicker != nil {
		cmd := a.taskPicker.Update(msg)
		return a, cmd
	}
	if a.newTask != nil {
		cmd := a.newTask.Update(msg)
		return a, cmd
	}

	// Prefix key handling for pane operations
	if a.mode == ModePanes {
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
		case ModeKanban:
			a.saveState()
			return a, tea.Quit
		case ModePanes:
			// 'q' goes to input in chat, don't quit
		}
	}

	// Delegate to active view
	var cmd tea.Cmd
	switch a.mode {
	case ModePicker:
		cmd = a.picker.Update(msg)
	case ModeKanban:
		cmd = a.kanban.Update(msg)
	case ModePanes:
		if chat := a.panes.FocusedChat(); chat != nil {
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
	// Split vertical
	case a.tmux.SplitV:
		if projectID != "" {
			a.taskPicker = NewTaskPickerModel(a.api, projectID, SplitVertical)
			a.taskPicker.SetSize(a.width, a.height-2)
			return a, a.taskPicker.Init()
		}
		return a, nil

	// Split horizontal
	case a.tmux.SplitH:
		if projectID != "" {
			a.taskPicker = NewTaskPickerModel(a.api, projectID, SplitHorizontal)
			a.taskPicker.SetSize(a.width, a.height-2)
			return a, a.taskPicker.Init()
		}
		return a, nil

	// Pane navigation
	case a.tmux.PaneNext:
		a.updatePaneFocus()
		a.panes.FocusNext()
		a.syncPaneFocus()
		return a, nil

	case a.tmux.PanePrev:
		a.updatePaneFocus()
		a.panes.FocusPrev()
		a.syncPaneFocus()
		return a, nil
	}

	// Directional pane navigation (vim-style if configured)
	if a.tmux.PaneLeft != "" && key == a.tmux.PaneLeft {
		a.updatePaneFocus()
		a.panes.FocusPrev()
		a.syncPaneFocus()
		return a, nil
	}
	if a.tmux.PaneRight != "" && key == a.tmux.PaneRight {
		a.updatePaneFocus()
		a.panes.FocusNext()
		a.syncPaneFocus()
		return a, nil
	}
	if a.tmux.PaneDown != "" && key == a.tmux.PaneDown {
		a.updatePaneFocus()
		a.panes.FocusNext()
		a.syncPaneFocus()
		return a, nil
	}
	if a.tmux.PaneUp != "" && key == a.tmux.PaneUp {
		a.updatePaneFocus()
		a.panes.FocusPrev()
		a.syncPaneFocus()
		return a, nil
	}

	switch key {
	// Close pane
	case a.tmux.ClosePane:
		if !a.panes.CloseFocused() {
			a.mode = ModeKanban
		}
		a.syncPaneFocus()
		return a, nil

	// Close all panes, back to kanban
	case "q":
		a.panes = NewPaneManager()
		a.mode = ModeKanban
		return a, nil

	// Detach (save state and quit)
	case a.tmux.Detach:
		a.saveState()
		return a, tea.Quit

	// Terminal
	case "t":
		a.status = "Terminal: not yet implemented"
		return a, nil

	// Web URL
	case "w":
		if chat := a.panes.FocusedChat(); chat != nil && chat.task != nil {
			url := a.api.WebURL(chat.task.ProjectID, chat.task.ID)
			a.status = "Open: " + url
		}
		return a, nil
	}

	return a, nil
}

func (a *App) updatePaneFocus() {
	if chat := a.panes.FocusedChat(); chat != nil {
		chat.SetFocused(false)
	}
}

func (a *App) syncPaneFocus() {
	if chat := a.panes.FocusedChat(); chat != nil {
		chat.SetFocused(true)
	}
}

func (a *App) restorePanes(state *TUIState) tea.Cmd {
	// Collect all task IDs from the saved pane state
	taskIDs := collectTaskIDs(state.Panes)
	if len(taskIDs) == 0 {
		return nil
	}

	// Fetch tasks to populate panes
	return func() tea.Msg {
		var tasks []*types.SpecTask
		for _, id := range taskIDs {
			task, err := a.api.GetSpecTask(apiCtx(), id)
			if err != nil {
				continue // skip tasks we can't load
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
	a.mode = ModePanes
	a.panes = NewPaneManager()
	a.panes.SetSize(a.width, a.height-2)

	var cmds []tea.Cmd
	a.buildPaneTree(state.Panes, taskMap, &cmds)

	if a.panes.IsEmpty() {
		a.mode = ModeKanban
		return nil
	}

	// Focus the task from saved state
	if state.FocusedTaskID != "" {
		leaves := a.panes.allLeaves(a.panes.Root)
		for _, leaf := range leaves {
			if leaf.Chat != nil && leaf.Chat.task != nil && leaf.Chat.task.ID == state.FocusedTaskID {
				a.panes.focused = leaf.ID
				leaf.Chat.SetFocused(true)
				break
			}
		}
	}

	a.status = fmt.Sprintf("Restored %d pane(s)", a.panes.PaneCount())
	return tea.Batch(cmds...)
}

func (a *App) buildPaneTree(ps *PaneState, taskMap map[string]*types.SpecTask, cmds *[]tea.Cmd) {
	if ps == nil {
		return
	}

	if ps.TaskID != "" {
		task, ok := taskMap[ps.TaskID]
		if !ok {
			return
		}
		chat := NewChatModel(a.api, task)
		a.panes.OpenPane(chat)
		*cmds = append(*cmds, chat.Init())
		return
	}

	// Split node — build children first, then split
	if ps.Left != nil {
		a.buildPaneTree(ps.Left, taskMap, cmds)
	}
	if ps.Right != nil {
		task := firstTaskInState(ps.Right, taskMap)
		if task != nil {
			dir := SplitVertical
			if ps.Dir == "horizontal" {
				dir = SplitHorizontal
			}
			chat := NewChatModel(a.api, task)
			a.panes.SplitFocused(dir, chat)
			*cmds = append(*cmds, chat.Init())
		}
	}
}

func firstTaskInState(ps *PaneState, taskMap map[string]*types.SpecTask) *types.SpecTask {
	if ps == nil {
		return nil
	}
	if ps.TaskID != "" {
		return taskMap[ps.TaskID]
	}
	if t := firstTaskInState(ps.Left, taskMap); t != nil {
		return t
	}
	return firstTaskInState(ps.Right, taskMap)
}

func collectTaskIDs(ps *PaneState) []string {
	if ps == nil {
		return nil
	}
	if ps.TaskID != "" {
		return []string{ps.TaskID}
	}
	var ids []string
	ids = append(ids, collectTaskIDs(ps.Left)...)
	ids = append(ids, collectTaskIDs(ps.Right)...)
	return ids
}

func (a *App) saveState() {
	state := BuildStateFromApp(a)
	if state.ProjectID != "" {
		_ = SaveState(state)
	}
}

func (a *App) View() string {
	if a.width == 0 {
		return "Loading..."
	}

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
		case ModeKanban:
			content = a.kanban.View()
		case ModePanes:
			content = a.panes.Render()
		}
	}

	statusBar := a.renderStatusBar()
	return content + "\n" + statusBar
}

func (a *App) renderStatusBar() string {
	style := styleStatusBar.Width(a.width)

	var help string
	switch a.mode {
	case ModePicker:
		help = "j/k: navigate  enter: select  q: quit"
	case ModeKanban:
		help = "h/l: column  j/k: task  enter: open  n: new task  r: refresh  q: quit"
	case ModePanes:
		prefix := a.tmux.Prefix
		if a.prefixNext {
			help = fmt.Sprintf("[%s] waiting for command...", prefix)
		} else {
			paneInfo := ""
			if a.panes.PaneCount() > 1 {
				paneInfo = fmt.Sprintf(" [%d panes]", a.panes.PaneCount())
			}
			help = fmt.Sprintf("%s: pane cmds  esc: stop/clear%s", prefix, paneInfo)
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
