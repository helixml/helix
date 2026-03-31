package tui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/helixml/helix/api/pkg/types"
)

// KanbanColumn represents a column in the kanban board.
type KanbanColumn int

const (
	ColBacklog KanbanColumn = iota
	ColPlanning
	ColInProgress
	ColReview
	ColDone
	ColCount // sentinel
)

func (c KanbanColumn) Title() string {
	switch c {
	case ColBacklog:
		return "Backlog"
	case ColPlanning:
		return "Planning"
	case ColInProgress:
		return "In Progress"
	case ColReview:
		return "Review"
	case ColDone:
		return "Done"
	default:
		return "?"
	}
}

// statusToColumn maps a SpecTaskStatus to a kanban column.
func statusToColumn(s types.SpecTaskStatus) KanbanColumn {
	switch s {
	case types.TaskStatusBacklog:
		return ColBacklog

	case types.TaskStatusQueuedSpecGeneration,
		types.TaskStatusSpecGeneration,
		types.TaskStatusSpecReview,
		types.TaskStatusSpecRevision,
		types.TaskStatusSpecApproved,
		types.TaskStatusSpecFailed:
		return ColPlanning

	case types.TaskStatusQueuedImplementation,
		types.TaskStatusImplementationQueued,
		types.TaskStatusImplementation,
		types.TaskStatusImplementationFailed:
		return ColInProgress

	case types.TaskStatusImplementationReview,
		types.TaskStatusPullRequest:
		return ColReview

	case types.TaskStatusDone:
		return ColDone

	default:
		return ColBacklog
	}
}

// KanbanModel is the kanban board view.
type KanbanModel struct {
	api       *APIClient
	projectID string
	project   *types.Project

	columns   [ColCount][]*types.SpecTask // tasks grouped by column
	colIdx    KanbanColumn               // focused column
	rowIdx    [ColCount]int               // cursor row per column
	scrollOff [ColCount]int              // scroll offset per column

	confirmTask   *types.SpecTask // non-nil when showing "start planning?" prompt
	archiveTask   *types.SpecTask // non-nil when showing "archive?" prompt

	loading bool
	err     error
	width   int
	height  int
}

type tasksLoadedMsg struct {
	tasks []*types.SpecTask
}

type openTaskChatMsg struct {
	task *types.SpecTask
}

type openNewTaskMsg struct{}
type backToProjectsMsg struct{}
type startPlanningMsg struct{ task *types.SpecTask }
type openReviewMsg struct{ task *types.SpecTask }
type archiveTaskMsg struct{ task *types.SpecTask }

func NewKanbanModel(api *APIClient, projectID string) *KanbanModel {
	return &KanbanModel{
		api:       api,
		projectID: projectID,
		loading:   true,
	}
}

func (k *KanbanModel) Init() tea.Cmd {
	return k.fetchTasks()
}

func (k *KanbanModel) SetSize(w, h int) {
	k.width = w
	k.height = h
}

func (k *KanbanModel) SetProject(p *types.Project) {
	k.project = p
	k.projectID = p.ID
}

func (k *KanbanModel) fetchTasks() tea.Cmd {
	return func() tea.Msg {
		// Fetch project details if we don't have them yet
		if k.project == nil && k.projectID != "" {
			p, err := k.api.GetProject(apiCtx(), k.projectID)
			if err == nil {
				k.project = p
			}
		}

		tasks, err := k.api.ListSpecTasks(apiCtx(), k.projectID)
		if err != nil {
			return errMsg{err}
		}
		return tasksLoadedMsg{tasks: tasks}
	}
}

func (k *KanbanModel) Update(msg tea.Msg) tea.Cmd {
	switch msg := msg.(type) {
	case tasksLoadedMsg:
		k.loading = false
		k.err = nil
		// Clear columns
		for i := range k.columns {
			k.columns[i] = nil
		}
		// Sort tasks into columns
		for _, t := range msg.tasks {
			col := statusToColumn(t.Status)
			k.columns[col] = append(k.columns[col], t)
		}
		return nil

	case errMsg:
		k.err = msg.err
		k.loading = false
		return nil

	case tea.KeyMsg:
		// Archive confirmation
		if k.archiveTask != nil {
			switch msg.String() {
			case "y", "enter":
				task := k.archiveTask
				k.archiveTask = nil
				return func() tea.Msg { return archiveTaskMsg{task: task} }
			default:
				k.archiveTask = nil
			}
			return nil
		}

		// Confirmation prompt for backlog tasks
		if k.confirmTask != nil {
			switch msg.String() {
			case "y", "enter":
				task := k.confirmTask
				k.confirmTask = nil
				return func() tea.Msg { return startPlanningMsg{task: task} }
			case "v":
				// View the task without starting
				task := k.confirmTask
				k.confirmTask = nil
				return func() tea.Msg { return openTaskChatMsg{task: task} }
			default:
				// Any other key cancels
				k.confirmTask = nil
			}
			return nil
		}

		key := msg.String()
		// Also check key type for arrow keys (some terminals report differently)
		switch msg.Type {
		case tea.KeyLeft:
			key = "left"
		case tea.KeyRight:
			key = "right"
		case tea.KeyUp:
			key = "up"
		case tea.KeyDown:
			key = "down"
		}

		switch key {
		case "h", "left":
			if k.colIdx > 0 {
				k.colIdx--
			}
		case "l", "right":
			if k.colIdx < ColCount-1 {
				k.colIdx++
			}
		case "j", "down":
			col := k.colIdx
			if k.rowIdx[col] < len(k.columns[col])-1 {
				k.rowIdx[col]++
				k.ensureVisible(col)
			}
		case "k", "up":
			col := k.colIdx
			if k.rowIdx[col] > 0 {
				k.rowIdx[col]--
				k.ensureVisible(col)
			}
		case "ctrl+d":
			col := k.colIdx
			half := k.visibleTaskSlots(col) / 2
			k.rowIdx[col] += half
			if k.rowIdx[col] >= len(k.columns[col]) {
				k.rowIdx[col] = len(k.columns[col]) - 1
			}
			if k.rowIdx[col] < 0 {
				k.rowIdx[col] = 0
			}
			k.ensureVisible(col)
		case "ctrl+u":
			col := k.colIdx
			half := k.visibleTaskSlots(col) / 2
			k.rowIdx[col] -= half
			if k.rowIdx[col] < 0 {
				k.rowIdx[col] = 0
			}
			k.ensureVisible(col)
		case "1":
			k.colIdx = ColBacklog
		case "2":
			k.colIdx = ColPlanning
		case "3":
			k.colIdx = ColInProgress
		case "4":
			k.colIdx = ColReview
		case "5":
			k.colIdx = ColDone
		case "enter":
			col := k.colIdx
			tasks := k.columns[col]
			if len(tasks) > 0 && k.rowIdx[col] < len(tasks) {
				task := tasks[k.rowIdx[col]]
				switch task.Status {
				case types.TaskStatusBacklog:
					// Show confirmation — don't auto-start
					k.confirmTask = task
					return nil
				case types.TaskStatusSpecReview:
					return func() tea.Msg { return openReviewMsg{task: task} }
				default:
					return func() tea.Msg { return openTaskChatMsg{task: task} }
				}
			}
		case "r":
			k.loading = true
			return k.fetchTasks()
		case "n":
			return func() tea.Msg { return openNewTaskMsg{} }
		case "x":
			col := k.colIdx
			tasks := k.columns[col]
			if len(tasks) > 0 && k.rowIdx[col] < len(tasks) {
				k.archiveTask = tasks[k.rowIdx[col]]
			}
		case "esc":
			return func() tea.Msg { return backToProjectsMsg{} }
		}
	}
	return nil
}

func (k *KanbanModel) cardHeight() int {
	h := k.height
	if h < 10 {
		h = 24
	}
	ch := h - 6
	if ch < 3 {
		ch = 3
	}
	return ch
}

// visibleTaskSlots returns how many task cards actually fit in the column,
// accounting for the header, separator, and scroll indicator lines.
func (k *KanbanModel) visibleTaskSlots(col KanbanColumn) int {
	slots := k.cardHeight() - 2 // header + separator inside box
	if k.scrollOff[col] > 0 {
		slots-- // "↑ N above" indicator
	}
	remaining := len(k.columns[col]) - (k.scrollOff[col] + slots)
	if remaining > 0 {
		slots-- // "↓ N below" indicator
	}
	if slots < 1 {
		slots = 1
	}
	return slots
}

func (k *KanbanModel) ensureVisible(col KanbanColumn) {
	slots := k.visibleTaskSlots(col)
	if k.rowIdx[col] < k.scrollOff[col] {
		k.scrollOff[col] = k.rowIdx[col]
	}
	if k.rowIdx[col] >= k.scrollOff[col]+slots {
		k.scrollOff[col] = k.rowIdx[col] - slots + 1
	}
}

func (k *KanbanModel) View() string {
	if k.loading {
		return "\n  Loading kanban board..."
	}
	if k.err != nil {
		return fmt.Sprintf("\n  %s %v\n  Press r to retry.", styleError.Render("Error:"), k.err)
	}

	// Project header
	projectName := k.projectID
	if k.project != nil && k.project.Name != "" {
		projectName = k.project.Name
	}
	header := styleHeader.Render(projectName)

	totalTasks := 0
	for _, col := range k.columns {
		totalTasks += len(col)
	}
	header += styleDim.Render(fmt.Sprintf("  %d tasks", totalTasks))

	// Render each column as a bordered box
	colWidth := k.width / int(ColCount)
	if colWidth < 16 {
		colWidth = 16
	}
	innerWidth := colWidth - 4 // border + padding
	ch := k.cardHeight()

	var cols []string
	for i := KanbanColumn(0); i < ColCount; i++ {
		cols = append(cols, k.renderColumn(i, colWidth, innerWidth, ch))
	}

	board := lipgloss.JoinHorizontal(lipgloss.Top, cols...)

	// Archive prompt
	if k.archiveTask != nil {
		name := taskDisplayName(k.archiveTask)
		prompt := fmt.Sprintf("\n  %s  %s\n  %s",
			styleHeader.Render(truncate(name, 50)),
			styleDim.Render("archive?"),
			lipgloss.NewStyle().Foreground(colorWarning).Render("y: archive  any other key: cancel"))
		return header + "\n" + board + prompt
	}

	// Confirmation prompt
	if k.confirmTask != nil {
		name := taskDisplayName(k.confirmTask)
		prompt := fmt.Sprintf("\n  %s  %s\n  %s",
			styleHeader.Render(truncate(name, 50)),
			styleDim.Render("(backlog)"),
			lipgloss.NewStyle().Foreground(colorWarning).Render("y: start planning  v: view  esc: cancel"))
		return header + "\n" + board + prompt
	}

	return header + "\n" + board
}

func (k *KanbanModel) renderColumn(col KanbanColumn, totalWidth, innerWidth, cardHeight int) string {
	tasks := k.columns[col]
	isFocusedCol := col == k.colIdx

	// Column header (rendered inside the box as first line)
	headerColor := statusColor(col)
	title := fmt.Sprintf("%s (%d)", col.Title(), len(tasks))
	headerStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(headerColor).
		Width(innerWidth).
		Align(lipgloss.Center)

	var lines []string
	lines = append(lines, headerStyle.Render(title))
	lines = append(lines, styleDim.Render(strings.Repeat("─", innerWidth)))

	// Build card content with scroll offset
	offset := k.scrollOff[col]
	visibleSlots := cardHeight - 2 // header + separator already take 2 lines
	if visibleSlots < 1 {
		visibleSlots = 1
	}

	end := offset + visibleSlots
	if end > len(tasks) {
		end = len(tasks)
	}

	// Scroll indicator (top)
	if offset > 0 {
		lines = append(lines, styleDim.Render(fmt.Sprintf("  ↑ %d above", offset)))
		visibleSlots-- // takes a slot
	}

	for i := offset; i < end && len(lines) < cardHeight; i++ {
		t := tasks[i]
		isSelected := isFocusedCol && i == k.rowIdx[col]
		lines = append(lines, k.renderCard(t, innerWidth, isSelected))
	}

	// Scroll indicator (bottom)
	remaining := len(tasks) - end
	if remaining > 0 {
		lines = append(lines, styleDim.Render(fmt.Sprintf("  ↓ %d below", remaining)))
	}

	// Pad to fill height
	for len(lines) < cardHeight {
		lines = append(lines, "")
	}
	// Trim to exact height
	if len(lines) > cardHeight {
		lines = lines[:cardHeight]
	}

	content := strings.Join(lines, "\n")

	// Render as bordered box
	borderColor := colorBorder
	if isFocusedCol {
		borderColor = colorBorderFoc
	}

	return lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(borderColor).
		Width(innerWidth).
		Height(cardHeight).
		Render(content)
}

func (k *KanbanModel) renderCard(t *types.SpecTask, width int, selected bool) string {
	name := taskDisplayName(t)
	if len(name) > width-3 {
		name = name[:width-6] + "..."
	}

	if selected {
		pointer := lipgloss.NewStyle().Foreground(colorPrimary).Render("> ")
		return pointer + lipgloss.NewStyle().Bold(true).Foreground(colorText).Render(name)
	}
	return "  " + name
}

func taskDisplayName(t *types.SpecTask) string {
	if t.UserShortTitle != "" {
		return t.UserShortTitle
	}
	if t.ShortTitle != "" {
		return t.ShortTitle
	}
	if t.Name != "" {
		return t.Name
	}
	if len(t.OriginalPrompt) > 40 {
		return t.OriginalPrompt[:37] + "..."
	}
	return t.OriginalPrompt
}
