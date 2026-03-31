package tui

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/helixml/helix/api/pkg/types"
)

// NewTaskModel is a modal overlay for creating a new spec task.
type NewTaskModel struct {
	api       *APIClient
	projectID string
	input     string
	creating  bool
	err       error
	width     int
	height    int
}

type newTaskCreatedMsg struct {
	task *types.SpecTask
}

type newTaskCancelMsg struct{}

func NewNewTaskModel(api *APIClient, projectID string) *NewTaskModel {
	return &NewTaskModel{
		api:       api,
		projectID: projectID,
	}
}

func (n *NewTaskModel) Init() tea.Cmd {
	return nil
}

func (n *NewTaskModel) SetSize(w, h int) {
	n.width = w
	n.height = h
}

func (n *NewTaskModel) Update(msg tea.Msg) tea.Cmd {
	switch msg := msg.(type) {
	case newTaskCreatedMsg:
		return nil // handled by app

	case errMsg:
		n.err = msg.err
		n.creating = false
		return nil

	case tea.KeyMsg:
		switch msg.String() {
		case "esc":
			if n.input != "" {
				n.input = ""
				return nil
			}
			return func() tea.Msg { return newTaskCancelMsg{} }

		case "enter":
			if n.input != "" && !n.creating {
				return n.createTask()
			}

		case "backspace":
			if len(n.input) > 0 {
				n.input = n.input[:len(n.input)-1]
			}

		default:
			if msg.Type == tea.KeyRunes {
				n.input += msg.String()
			} else if msg.Type == tea.KeySpace {
				n.input += " "
			}
		}
	}
	return nil
}

func (n *NewTaskModel) createTask() tea.Cmd {
	n.creating = true
	prompt := n.input
	projectID := n.projectID

	return func() tea.Msg {
		req := &types.CreateTaskRequest{
			ProjectID: projectID,
			Prompt:    prompt,
			Type:      "task",
			Priority:  types.SpecTaskPriorityMedium,
		}
		task, err := n.api.CreateTaskFromPrompt(apiCtx(), req)
		if err != nil {
			return errMsg{err}
		}
		return newTaskCreatedMsg{task: task}
	}
}

func (n *NewTaskModel) View() string {
	modalWidth := n.width * 2 / 3
	if modalWidth < 40 {
		modalWidth = 40
	}
	if modalWidth > 80 {
		modalWidth = 80
	}

	var b strings.Builder
	b.WriteString(styleHeader.Render("New task"))
	b.WriteString("\n\n")
	b.WriteString(styleDim.Render("  Describe what you want done:"))
	b.WriteString("\n\n")

	prompt := lipgloss.NewStyle().Foreground(colorPrimary).Render("> ")
	inputText := n.input + "█"
	if n.creating {
		inputText = styleDim.Render("creating...")
	}
	b.WriteString("  " + prompt + inputText)

	if n.err != nil {
		b.WriteString("\n\n")
		b.WriteString("  " + styleError.Render(n.err.Error()))
	}

	b.WriteString("\n\n")
	b.WriteString("  " + styleDim.Render("enter: create  esc: cancel"))

	content := b.String()
	modal := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(colorPrimary).
		Width(modalWidth).
		Padding(1, 2).
		Render(content)

	// Center
	padTop := (n.height - 12) / 2
	padLeft := (n.width - modalWidth - 2) / 2
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
