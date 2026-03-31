package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/helixml/helix/api/pkg/types"
)

// Tab represents a single tab (a spec task or special view).
type Tab struct {
	ID       int
	Name     string
	TaskID   string          // empty for kanban tab
	Task     *types.SpecTask // nil for kanban tab
	Panes    *PaneManager    // each tab has its own pane tree
	IsKanban bool
	Unread   int // unread notification count
}

// TabBar manages the bottom tab bar.
type TabBar struct {
	tabs    []*Tab
	active  int // index of the active tab
	nextID  int
	width   int
}

func NewTabBar() *TabBar {
	tb := &TabBar{nextID: 1}
	// Tab 0 is always kanban
	tb.tabs = append(tb.tabs, &Tab{
		ID:       0,
		Name:     "kanban",
		IsKanban: true,
	})
	return tb
}

func (tb *TabBar) SetWidth(w int) {
	tb.width = w
}

// ActiveTab returns the currently active tab.
func (tb *TabBar) ActiveTab() *Tab {
	if tb.active < len(tb.tabs) {
		return tb.tabs[tb.active]
	}
	return tb.tabs[0]
}

// ActiveIndex returns the index of the active tab.
func (tb *TabBar) ActiveIndex() int {
	return tb.active
}

// AddTab creates a new tab for a spec task and makes it active.
func (tb *TabBar) AddTab(task *types.SpecTask) *Tab {
	name := taskDisplayName(task)
	if len(name) > 20 {
		name = name[:17] + "..."
	}

	tab := &Tab{
		ID:     tb.nextID,
		Name:   name,
		TaskID: task.ID,
		Task:   task,
		Panes:  NewPaneManager(),
	}
	tb.nextID++
	tb.tabs = append(tb.tabs, tab)
	tb.active = len(tb.tabs) - 1
	return tab
}

// CloseTab closes the active tab (unless it's kanban).
func (tb *TabBar) CloseTab() {
	if tb.active == 0 {
		return // can't close kanban
	}
	tb.tabs = append(tb.tabs[:tb.active], tb.tabs[tb.active+1:]...)
	if tb.active >= len(tb.tabs) {
		tb.active = len(tb.tabs) - 1
	}
}

// NextTab moves to the next tab.
func (tb *TabBar) NextTab() {
	tb.active = (tb.active + 1) % len(tb.tabs)
}

// PrevTab moves to the previous tab.
func (tb *TabBar) PrevTab() {
	tb.active = (tb.active - 1 + len(tb.tabs)) % len(tb.tabs)
}

// GoToTab jumps to a specific tab by index.
func (tb *TabBar) GoToTab(idx int) {
	if idx >= 0 && idx < len(tb.tabs) {
		tb.active = idx
	}
}

// FindTabByTask returns the tab for a given task ID, or nil.
func (tb *TabBar) FindTabByTask(taskID string) *Tab {
	for _, t := range tb.tabs {
		if t.TaskID == taskID {
			return t
		}
	}
	return nil
}

// TabCount returns the number of tabs.
func (tb *TabBar) TabCount() int {
	return len(tb.tabs)
}

// View renders the tab bar at the bottom of the screen.
func (tb *TabBar) View() string {
	if tb.width < 10 {
		return ""
	}

	activeStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("231")).
		Background(lipgloss.Color("240"))

	inactiveStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("245")).
		Background(lipgloss.Color("235"))

	var parts []string
	for i, tab := range tb.tabs {
		label := fmt.Sprintf(" %d:%s ", i, tab.Name)

		// Active indicator
		if i == tb.active {
			label += "* "
		}

		// Unread badge
		if tab.Unread > 0 {
			label += fmt.Sprintf("(%d) ", tab.Unread)
		}

		if i == tb.active {
			parts = append(parts, activeStyle.Render(label))
		} else {
			parts = append(parts, inactiveStyle.Render(label))
		}
	}

	bar := strings.Join(parts, "")

	// Pad to full width
	barLen := 0
	for _, tab := range tb.tabs {
		barLen += len(fmt.Sprintf(" %d:%s ", 0, tab.Name)) + 2
	}
	if barLen < tb.width {
		bar += lipgloss.NewStyle().
			Background(lipgloss.Color("236")).
			Render(strings.Repeat(" ", tb.width-barLen))
	}

	return bar
}
