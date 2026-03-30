package tui

import (
	"context"
	"fmt"

	tea "github.com/charmbracelet/bubbletea"
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

	err    error
	status string
}

// Messages
type errMsg struct{ err error }
type statusMsg string

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
			// Pane restoration happens after tasks are loaded
			// (need task data to populate chat panes)
		} else {
			app.mode = ModePicker
			app.picker = NewPickerModel(api)
		}
	}

	return app
}

func (a *App) Init() tea.Cmd {
	switch a.mode {
	case ModePicker:
		return a.picker.Init()
	case ModeKanban:
		return a.kanban.Init()
	default:
		return nil
	}
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

	case openNewTaskMsg:
		if a.kanban != nil {
			a.newTask = NewNewTaskModel(a.api, a.kanban.projectID)
			a.newTask.SetSize(a.width, a.height-2)
		}
		return a, nil

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
