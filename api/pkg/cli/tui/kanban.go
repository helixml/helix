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

	columns [ColCount][]*types.SpecTask // tasks grouped by column
	colIdx  KanbanColumn               // focused column
	rowIdx  [ColCount]int               // cursor row per column

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
			}
		case "k", "up":
			col := k.colIdx
			if k.rowIdx[col] > 0 {
				k.rowIdx[col]--
			}
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
				return func() tea.Msg {
					return openTaskChatMsg{task: task}
				}
			}
		case "r":
			k.loading = true
			return k.fetchTasks()
		case "n":
			return func() tea.Msg { return openNewTaskMsg{} }
		}
	}
	return nil
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
	cardHeight := k.height - 6 // project header + column header + borders
	if cardHeight < 3 {
		cardHeight = 3
	}

	var cols []string
	for i := KanbanColumn(0); i < ColCount; i++ {
		cols = append(cols, k.renderColumn(i, colWidth, innerWidth, cardHeight))
	}

	board := lipgloss.JoinHorizontal(lipgloss.Top, cols...)
	return header + "\n" + board
}

func (k *KanbanModel) renderColumn(col KanbanColumn, totalWidth, innerWidth, cardHeight int) string {
	tasks := k.columns[col]
	isFocusedCol := col == k.colIdx

	// Column header
	headerColor := statusColor(col)
	title := fmt.Sprintf("%s (%d)", col.Title(), len(tasks))

	// Build card content
	var lines []string
	for i := 0; i < cardHeight && i < len(tasks); i++ {
		t := tasks[i]
		isSelected := isFocusedCol && i == k.rowIdx[col]
		lines = append(lines, k.renderCard(t, innerWidth, isSelected))
	}

	if len(tasks) > cardHeight {
		lines = append(lines, styleDim.Render(fmt.Sprintf("+%d more", len(tasks)-cardHeight)))
	}

	// Pad to fill height
	for len(lines) < cardHeight {
		lines = append(lines, "")
	}

	content := strings.Join(lines, "\n")

	// Render as bordered box with column header as title
	borderColor := colorBorder
	if isFocusedCol {
		borderColor = colorBorderFoc
	}

	box := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(borderColor).
		Width(innerWidth).
		Height(cardHeight).
		Render(content)

	// Column header above the box
	headerStyle := lipgloss.NewStyle().
		Bold(isFocusedCol).
		Foreground(headerColor).
		Width(totalWidth).
		Align(lipgloss.Center)

	return headerStyle.Render(title) + "\n" + box
}

func (k *KanbanModel) renderCard(t *types.SpecTask, width int, selected bool) string {
	name := taskDisplayName(t)
	if len(name) > width-4 {
		name = name[:width-7] + "..."
	}

	prefix := "  "
	if selected {
		prefix = lipgloss.NewStyle().Foreground(colorPrimary).Render("> ")
	}

	line := fmt.Sprintf("%s%s", prefix, name)

	style := styleNormal.Width(width)
	if selected {
		style = lipgloss.NewStyle().
			Background(colorSelected).
			Foreground(colorText).
			Width(width)
	}

	// Status detail on second line for active tasks
	detail := ""
	if t.AgentWorkState != "" && t.AgentWorkState != "idle" {
		detail = styleDim.Render(fmt.Sprintf("    %s", t.AgentWorkState))
	} else if t.BranchName != "" {
		branch := t.BranchName
		if len(branch) > width-6 {
			branch = branch[:width-9] + "..."
		}
		detail = styleDim.Render(fmt.Sprintf("    %s", branch))
	}

	result := style.Render(line)
	if detail != "" {
		result += "\n" + detail
	}
	return result
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
