package tui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/helixml/helix/api/pkg/types"
)

// PickerModel is a project picker shown on startup.
type PickerModel struct {
	api      *APIClient
	projects []*types.Project
	cursor   int
	loading  bool
	err      error
	width    int
	height   int
}

type projectsLoadedMsg struct {
	projects []*types.Project
}

type projectSelectedMsg struct {
	project *types.Project
}

func NewPickerModel(api *APIClient) *PickerModel {
	return &PickerModel{
		api:     api,
		loading: true,
	}
}

func (p *PickerModel) Init() tea.Cmd {
	return func() tea.Msg {
		projects, err := p.api.ListProjects(apiCtx())
		if err != nil {
			return errMsg{err}
		}
		return projectsLoadedMsg{projects: projects}
	}
}

func (p *PickerModel) SetSize(w, h int) {
	p.width = w
	p.height = h
}

func (p *PickerModel) Update(msg tea.Msg) tea.Cmd {
	switch msg := msg.(type) {
	case projectsLoadedMsg:
		p.projects = msg.projects
		p.loading = false
		// If only one project, auto-select it
		if len(p.projects) == 1 {
			return func() tea.Msg {
				return projectSelectedMsg{project: p.projects[0]}
			}
		}
		return nil

	case errMsg:
		p.err = msg.err
		p.loading = false
		return nil

	case tea.KeyMsg:
		switch msg.String() {
		case "j", "down":
			if p.cursor < len(p.projects)-1 {
				p.cursor++
			}
		case "k", "up":
			if p.cursor > 0 {
				p.cursor--
			}
		case "enter":
			if len(p.projects) > 0 && p.cursor < len(p.projects) {
				proj := p.projects[p.cursor]
				return func() tea.Msg {
					return projectSelectedMsg{project: proj}
				}
			}
		}
	}
	return nil
}

func (p *PickerModel) View() string {
	if p.loading {
		return "\n  Loading projects..."
	}
	if p.err != nil {
		return fmt.Sprintf("\n  %s %v", styleError.Render("Error:"), p.err)
	}
	if len(p.projects) == 0 {
		return "\n  No projects found. Create one in the Helix web UI first."
	}

	var b strings.Builder

	title := styleHeader.Render("Select a project")
	b.WriteString("\n  " + title + "\n\n")

	for i, proj := range p.projects {
		name := proj.Name
		if name == "" {
			name = proj.ID
		}

		desc := truncate(proj.Description, 60)
		stats := ""
		if proj.Stats.TotalTasks > 0 {
			stats = fmt.Sprintf("%d tasks", proj.Stats.TotalTasks)
			if proj.Stats.InProgressTasks > 0 {
				stats += fmt.Sprintf(", %d active", proj.Stats.InProgressTasks)
			}
		}

		line := fmt.Sprintf("  %s", name)
		if desc != "" {
			line += styleDim.Render("  " + desc)
		}
		if stats != "" {
			line += "  " + styleDim.Render("["+stats+"]")
		}

		if i == p.cursor {
			pointer := lipgloss.NewStyle().Foreground(colorPrimary).Render("> ")
			b.WriteString(pointer)
			b.WriteString(lipgloss.NewStyle().
				Foreground(colorText).
				Bold(true).
				Render(line))
		} else {
			b.WriteString("  ")
			b.WriteString(styleNormal.Render(line))
		}
		b.WriteString("\n")
	}

	b.WriteString("\n  " + styleDim.Render("j/k: navigate  enter: select  q: quit"))

	return b.String()
}
