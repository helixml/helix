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

	// New project input
	creating    bool
	createStep  int    // 0=name, 1=github url
	createName  string
	createURL   string
}

type projectsLoadedMsg struct {
	projects []*types.Project
	pinned   []string
}

type backToOrgsMsg struct{}

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

	case projectPinToggledMsg:
		if msg.pinned {
			p.pinned[msg.projectID] = true
		} else {
			delete(p.pinned, msg.projectID)
		}
		// Re-sort: pinned first
		pinned := make([]*types.Project, 0)
		unpinned := make([]*types.Project, 0)
		for _, proj := range p.projects {
			if p.pinned[proj.ID] {
				pinned = append(pinned, proj)
			} else {
				unpinned = append(unpinned, proj)
			}
		}
		p.projects = append(pinned, unpinned...)
		return nil

	case errMsg:
		p.err = msg.err
		p.loading = false
		return nil

	case tea.KeyMsg:
		// Creating a new project — handle input
		if p.creating {
			return p.handleCreateInput(msg)
		}

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
		case "esc":
			return func() tea.Msg { return backToOrgsMsg{} }
		case "p":
			if len(p.projects) > 0 && p.cursor < len(p.projects) {
				proj := p.projects[p.cursor]
				isPinned := p.pinned[proj.ID]
				return p.togglePin(proj.ID, isPinned)
			}
		case "n":
			p.creating = true
			p.createStep = 0
			p.createName = ""
			p.createURL = ""
		}
	}
	return nil
}

type projectPinToggledMsg struct {
	projectID string
	pinned    bool
}

func (p *PickerModel) togglePin(projectID string, currentlyPinned bool) tea.Cmd {
	api := p.api
	return func() tea.Msg {
		var err error
		if currentlyPinned {
			err = api.UnpinProject(apiCtx(), projectID)
		} else {
			err = api.PinProject(apiCtx(), projectID)
		}
		if err != nil {
			return errMsg{err}
		}
		return projectPinToggledMsg{projectID: projectID, pinned: !currentlyPinned}
	}
}

func (p *PickerModel) handleCreateInput(msg tea.KeyMsg) tea.Cmd {
	switch msg.String() {
	case "esc":
		p.creating = false
		return nil
	case "enter":
		if p.createStep == 0 {
			if p.createName == "" {
				return nil // name required
			}
			p.createStep = 1
			return nil
		}
		// Step 1: submit (URL is optional)
		return p.submitNewProject()
	case "backspace":
		if p.createStep == 0 && len(p.createName) > 0 {
			p.createName = p.createName[:len(p.createName)-1]
		} else if p.createStep == 1 && len(p.createURL) > 0 {
			p.createURL = p.createURL[:len(p.createURL)-1]
		}
	default:
		if msg.Type == tea.KeyRunes {
			if p.createStep == 0 {
				p.createName += msg.String()
			} else {
				p.createURL += msg.String()
			}
		} else if msg.String() == " " {
			if p.createStep == 0 {
				p.createName += " "
			} else {
				p.createURL += " "
			}
		}
	}
	return nil
}

func (p *PickerModel) submitNewProject() tea.Cmd {
	name := p.createName
	url := strings.TrimSpace(p.createURL)
	orgID := p.orgID
	api := p.api

	p.creating = false
	p.loading = true

	return func() tea.Msg {
		req := &types.ProjectCreateRequest{
			OrganizationID: orgID,
			Name:           name,
			GitHubRepoURL:  url,
		}

		project, err := api.CreateProject(apiCtx(), req)
		if err != nil {
			return errMsg{err}
		}
		return projectSelectedMsg{project: project}
	}
}

func (p *PickerModel) viewCreateForm() string {
	var b strings.Builder

	b.WriteString("\n  " + styleHeader.Render("New project") + "\n\n")

	prompt := lipgloss.NewStyle().Foreground(colorPrimary).Bold(true)

	if p.createStep == 0 {
		b.WriteString("  " + prompt.Render("Name: ") + p.createName)
		b.WriteString(lipgloss.NewStyle().Foreground(colorPrimary).Render("█"))
		b.WriteString("\n\n")
		b.WriteString("  " + styleDim.Render("GitHub URL: (next step)"))
	} else {
		b.WriteString("  " + styleDim.Render("Name: "+p.createName) + "\n\n")
		b.WriteString("  " + prompt.Render("GitHub URL: ") + p.createURL)
		b.WriteString(lipgloss.NewStyle().Foreground(colorPrimary).Render("█"))
		b.WriteString("\n")
		b.WriteString("  " + styleDim.Render("(optional — press enter to skip)"))
	}

	b.WriteString("\n\n  " + styleDim.Render("enter: next/create  esc: cancel"))

	return b.String()
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

	if p.creating {
		return p.viewCreateForm()
	}

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
