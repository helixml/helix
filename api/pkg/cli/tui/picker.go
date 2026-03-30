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
	orgID    string // selected organization
	projects []*types.Project
	pinned   map[string]bool // set of pinned project IDs
	cursor   int
	offset   int // scroll offset
	loading  bool
	err      error
	width    int
	height   int
}

type projectsLoadedMsg struct {
	projects []*types.Project
	pinned   []string
}

type projectSelectedMsg struct {
	project *types.Project
}

func NewPickerModel(api *APIClient, orgID string) *PickerModel {
	return &PickerModel{
		api:     api,
		orgID:   orgID,
		pinned:  make(map[string]bool),
		loading: true,
	}
}

func (p *PickerModel) Init() tea.Cmd {
	return func() tea.Msg {
		projects, err := p.api.ListProjects(apiCtx(), p.orgID)
		if err != nil {
			return errMsg{err}
		}

		// Fetch pinned project IDs from user status
		var pinnedIDs []string
		status, err := p.api.GetUserStatus(apiCtx())
		if err == nil && status != nil {
			pinnedIDs = status.Config.PinnedProjectIDs
		}

		return projectsLoadedMsg{projects: projects, pinned: pinnedIDs}
	}
}

func (p *PickerModel) SetSize(w, h int) {
	p.width = w
	p.height = h
}

func (p *PickerModel) Update(msg tea.Msg) tea.Cmd {
	switch msg := msg.(type) {
	case projectsLoadedMsg:
		p.pinned = make(map[string]bool)
		for _, id := range msg.pinned {
			p.pinned[id] = true
		}

		// Sort: pinned first, then by name
		pinned := make([]*types.Project, 0)
		unpinned := make([]*types.Project, 0)
		for _, proj := range msg.projects {
			if p.pinned[proj.ID] {
				pinned = append(pinned, proj)
			} else {
				unpinned = append(unpinned, proj)
			}
		}
		p.projects = append(pinned, unpinned...)
		p.loading = false

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
				p.ensureVisible()
			}
		case "k", "up":
			if p.cursor > 0 {
				p.cursor--
				p.ensureVisible()
			}
		case "G":
			if len(p.projects) > 0 {
				p.cursor = len(p.projects) - 1
				p.ensureVisible()
			}
		case "g":
			p.cursor = 0
			p.offset = 0
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

func (p *PickerModel) ensureVisible() {
	visibleRows := p.visibleRows()
	if p.cursor < p.offset {
		p.offset = p.cursor
	}
	if p.cursor >= p.offset+visibleRows {
		p.offset = p.cursor - visibleRows + 1
	}
}

func (p *PickerModel) visibleRows() int {
	rows := p.height - 5 // title + margins
	if rows < 3 {
		rows = 3
	}
	return rows
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

	visible := p.visibleRows()
	end := p.offset + visible
	if end > len(p.projects) {
		end = len(p.projects)
	}

	lastWasPinned := false
	for i := p.offset; i < end; i++ {
		proj := p.projects[i]
		isPinned := p.pinned[proj.ID]

		// Show separator between pinned and unpinned
		if lastWasPinned && !isPinned {
			b.WriteString(styleDim.Render("  ────────────") + "\n")
		}
		lastWasPinned = isPinned

		name := proj.Name
		if name == "" {
			name = proj.ID
		}

		desc := truncate(proj.Description, 50)
		stats := ""
		if proj.Stats.TotalTasks > 0 {
			stats = fmt.Sprintf("%d tasks", proj.Stats.TotalTasks)
			if proj.Stats.InProgressTasks > 0 {
				stats += fmt.Sprintf(", %d active", proj.Stats.InProgressTasks)
			}
		}

		pin := ""
		if isPinned {
			pin = lipgloss.NewStyle().Foreground(colorWarning).Render("* ")
		}

		line := fmt.Sprintf("  %s%s", pin, name)
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

	// Scroll indicators
	if p.offset > 0 {
		b.WriteString(styleDim.Render(fmt.Sprintf("  ↑ %d more above", p.offset)) + "\n")
	}
	remaining := len(p.projects) - end
	if remaining > 0 {
		b.WriteString(styleDim.Render(fmt.Sprintf("  ↓ %d more below", remaining)) + "\n")
	}

	return b.String()
}
