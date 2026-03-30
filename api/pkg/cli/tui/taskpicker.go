package tui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/helixml/helix/api/pkg/types"
)

// TaskPickerModel is a modal overlay for picking a task to open in a new pane.
type TaskPickerModel struct {
	api       *APIClient
	projectID string
	tasks     []*types.SpecTask
	filtered  []*types.SpecTask
	cursor    int
	filter    string
	loading   bool
	err       error
	width     int
	height    int
	splitDir  SplitDir // which direction to split when a task is picked
}

type taskPickerDoneMsg struct {
	task     *types.SpecTask
	splitDir SplitDir
}

type taskPickerCancelMsg struct{}

func NewTaskPickerModel(api *APIClient, projectID string, dir SplitDir) *TaskPickerModel {
	return &TaskPickerModel{
		api:       api,
		projectID: projectID,
		splitDir:  dir,
		loading:   true,
	}
}

func (p *TaskPickerModel) Init() tea.Cmd {
	return func() tea.Msg {
		tasks, err := p.api.ListSpecTasks(apiCtx(), p.projectID)
		if err != nil {
			return errMsg{err}
		}
		return tasksLoadedMsg{tasks: tasks}
	}
}

func (p *TaskPickerModel) SetSize(w, h int) {
	p.width = w
	p.height = h
}

func (p *TaskPickerModel) Update(msg tea.Msg) tea.Cmd {
	switch msg := msg.(type) {
	case tasksLoadedMsg:
		p.tasks = msg.tasks
		p.filtered = msg.tasks
		p.loading = false
		return nil

	case errMsg:
		p.err = msg.err
		p.loading = false
		return nil

	case tea.KeyMsg:
		switch msg.String() {
		case "esc":
			return func() tea.Msg { return taskPickerCancelMsg{} }

		case "enter":
			if len(p.filtered) > 0 && p.cursor < len(p.filtered) {
				task := p.filtered[p.cursor]
				dir := p.splitDir
				return func() tea.Msg {
					return taskPickerDoneMsg{task: task, splitDir: dir}
				}
			}

		case "j", "down":
			if p.cursor < len(p.filtered)-1 {
				p.cursor++
			}
		case "k", "up":
			if p.cursor > 0 {
				p.cursor--
			}

		case "backspace":
			if len(p.filter) > 0 {
				p.filter = p.filter[:len(p.filter)-1]
				p.applyFilter()
			}

		default:
			if msg.Type == tea.KeyRunes {
				p.filter += msg.String()
				p.applyFilter()
				p.cursor = 0
			}
		}
	}
	return nil
}

func (p *TaskPickerModel) applyFilter() {
	if p.filter == "" {
		p.filtered = p.tasks
		return
	}

	lower := strings.ToLower(p.filter)
	var result []*types.SpecTask
	for _, t := range p.tasks {
		name := strings.ToLower(taskDisplayName(t))
		if strings.Contains(name, lower) {
			result = append(result, t)
		}
	}
	p.filtered = result
}

func (p *TaskPickerModel) View() string {
	// Render as a centered modal
	modalWidth := p.width * 2 / 3
	if modalWidth < 40 {
		modalWidth = 40
	}
	if modalWidth > 80 {
		modalWidth = 80
	}
	modalHeight := p.height * 2 / 3
	if modalHeight < 10 {
		modalHeight = 10
	}

	var b strings.Builder

	dirLabel := "vertical"
	if p.splitDir == SplitHorizontal {
		dirLabel = "horizontal"
	}
	b.WriteString(styleHeader.Render(fmt.Sprintf("Split %s — pick a task", dirLabel)))
	b.WriteString("\n")

	// Filter input
	filterLine := styleDim.Render("/ ") + p.filter + "█"
	b.WriteString(filterLine + "\n\n")

	if p.loading {
		b.WriteString("  Loading tasks...")
	} else if p.err != nil {
		b.WriteString(styleError.Render(fmt.Sprintf("  Error: %v", p.err)))
	} else if len(p.filtered) == 0 {
		b.WriteString(styleDim.Render("  No matching tasks"))
	} else {
		maxVisible := modalHeight - 5
		for i := 0; i < maxVisible && i < len(p.filtered); i++ {
			t := p.filtered[i]
			name := taskDisplayName(t)
			status := string(t.Status)
			prio := priorityStyle(string(t.Priority))

			line := fmt.Sprintf("%s %-40s %s", prio, truncate(name, 40), styleDim.Render(status))

			if i == p.cursor {
				pointer := lipgloss.NewStyle().Foreground(colorPrimary).Render("> ")
				b.WriteString(pointer + lipgloss.NewStyle().Bold(true).Foreground(colorText).Render(line))
			} else {
				b.WriteString("  " + styleNormal.Render(line))
			}
			b.WriteString("\n")
		}
		if len(p.filtered) > maxVisible {
			b.WriteString(styleDim.Render(fmt.Sprintf("  +%d more", len(p.filtered)-maxVisible)))
		}
	}

	b.WriteString("\n" + styleDim.Render("  type to filter  enter: select  esc: cancel"))

	// Wrap in a border
	content := b.String()
	modal := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(colorPrimary).
		Width(modalWidth).
		Padding(1, 2).
		Render(content)

	// Center the modal
	padTop := (p.height - modalHeight) / 2
	padLeft := (p.width - modalWidth - 2) / 2
	if padTop < 0 {
		padTop = 0
	}
	if padLeft < 0 {
		padLeft = 0
	}

	lines := strings.Split(modal, "\n")
	var result []string
	for i := 0; i < padTop; i++ {
		result = append(result, "")
	}
	for _, line := range lines {
		result = append(result, strings.Repeat(" ", padLeft)+line)
	}

	return strings.Join(result, "\n")
}
