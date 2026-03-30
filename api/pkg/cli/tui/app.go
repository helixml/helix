package tui

import (
	"context"
	"fmt"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
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

	picker *PickerModel
	kanban *KanbanModel
	panes  *PaneManager

	err    error
	status string
}

// Messages
type errMsg struct{ err error }
type statusMsg string

func NewApp(api *APIClient, projectID string) *App {
	tmuxCfg := LoadTmuxConfig()

	app := &App{
		api:  api,
		tmux: tmuxCfg,
		panes: NewPaneManager(),
	}

	if projectID != "" {
		// Skip picker, go straight to kanban
		app.mode = ModeKanban
		app.kanban = NewKanbanModel(api, projectID)
	} else {
		app.mode = ModePicker
		app.picker = NewPickerModel(api)
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

	case errMsg:
		a.err = msg.err
		return a, nil

	case statusMsg:
		a.status = string(msg)
		return a, nil
	}

	// Delegate to active view
	var cmd tea.Cmd
	switch a.mode {
	case ModePicker:
		cmd = a.picker.Update(msg)
	case ModeKanban:
		cmd = a.kanban.Update(msg)
	case ModePanes:
		// Forward to focused chat pane
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
		return a, tea.Quit
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
	switch key {
	// Split vertical
	case a.tmux.SplitV:
		// TODO: open a task picker to select which task goes in new pane
		a.status = "Split vertical — pick a task"
		return a, nil

	// Split horizontal
	case a.tmux.SplitH:
		a.status = "Split horizontal — pick a task"
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

	case a.tmux.PaneLeft:
		if a.tmux.PaneLeft != "" {
			a.updatePaneFocus()
			a.panes.FocusPrev() // simplified — TODO: directional focus
			a.syncPaneFocus()
		}
		return a, nil

	case a.tmux.PaneRight:
		if a.tmux.PaneRight != "" {
			a.updatePaneFocus()
			a.panes.FocusNext()
			a.syncPaneFocus()
		}
		return a, nil

	// Close pane
	case a.tmux.ClosePane:
		if !a.panes.CloseFocused() {
			// No panes left, back to kanban
			a.mode = ModeKanban
		}
		a.syncPaneFocus()
		return a, nil

	// Close all panes, back to kanban
	case "q":
		a.panes = NewPaneManager()
		a.mode = ModeKanban
		return a, nil

	// Detach
	case a.tmux.Detach:
		a.status = "Detach not yet implemented"
		return a, nil

	// Terminal
	case "t":
		a.status = "Terminal not yet implemented"
		return a, nil

	// Web URL
	case "w":
		if chat := a.panes.FocusedChat(); chat != nil && chat.task != nil {
			a.status = fmt.Sprintf("Open in browser: %s/projects/%s/tasks/%s",
				"${HELIX_URL}", chat.task.ProjectID, chat.task.ID)
		}
		return a, nil
	}

	return a, nil
}

func (a *App) updatePaneFocus() {
	// Unfocus current
	if chat := a.panes.FocusedChat(); chat != nil {
		chat.SetFocused(false)
	}
}

func (a *App) syncPaneFocus() {
	// Focus new
	if chat := a.panes.FocusedChat(); chat != nil {
		chat.SetFocused(true)
	}
}

func (a *App) View() string {
	if a.width == 0 {
		return "Loading..."
	}

	var content string
	switch a.mode {
	case ModePicker:
		content = a.picker.View()
	case ModeKanban:
		content = a.kanban.View()
	case ModePanes:
		content = a.panes.Render()
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
			help = fmt.Sprintf("[%s] |:split-v  -:split-h  o:next  x:close  d:detach  q:kanban",
				prefix)
		} else {
			paneInfo := ""
			if a.panes.PaneCount() > 1 {
				paneInfo = fmt.Sprintf(" [%d panes]", a.panes.PaneCount())
			}
			help = fmt.Sprintf("%s: prefix  esc: stop/clear%s", prefix, paneInfo)
		}
	}

	if a.err != nil {
		help = styleError.Render(fmt.Sprintf("Error: %v", a.err)) + "  " + help
	}
	if a.status != "" {
		help = a.status + "  " + help
	}

	return style.Render(help)
}

// Helper to create context for API calls.
func apiCtx() context.Context {
	return context.Background()
}
