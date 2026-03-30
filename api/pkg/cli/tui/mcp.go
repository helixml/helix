package tui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// MCPServer represents a configured MCP server.
type MCPServer struct {
	Name      string
	Status    string // "connected", "disconnected", "error"
	ToolCount int
	URL       string
}

// MCPModel is the MCP server management UI (/mcp slash command).
type MCPModel struct {
	api       *APIClient
	projectID string
	servers   []MCPServer
	cursor    int
	loading   bool
	err       error
	width     int
	height    int
}

type mcpServersLoadedMsg struct {
	servers []MCPServer
}

type mcpCancelMsg struct{}

func NewMCPModel(api *APIClient, projectID string) *MCPModel {
	return &MCPModel{
		api:       api,
		projectID: projectID,
		loading:   true,
	}
}

func (m *MCPModel) Init() tea.Cmd {
	// TODO: Fetch MCP servers from project settings via API
	// For now, return mock data
	return func() tea.Msg {
		return mcpServersLoadedMsg{
			servers: []MCPServer{
				{Name: "drone-ci", Status: "connected", ToolCount: 4, URL: "stdio://drone-ci-mcp"},
				{Name: "github", Status: "connected", ToolCount: 12, URL: "stdio://github-mcp"},
				{Name: "slack", Status: "disconnected", ToolCount: 0, URL: "stdio://slack-mcp"},
			},
		}
	}
}

func (m *MCPModel) SetSize(w, h int) {
	m.width = w
	m.height = h
}

func (m *MCPModel) Update(msg tea.Msg) tea.Cmd {
	switch msg := msg.(type) {
	case mcpServersLoadedMsg:
		m.servers = msg.servers
		m.loading = false
		return nil

	case errMsg:
		m.err = msg.err
		m.loading = false
		return nil

	case tea.KeyMsg:
		switch msg.String() {
		case "esc", "q":
			return func() tea.Msg { return mcpCancelMsg{} }
		case "j", "down":
			if m.cursor < len(m.servers)-1 {
				m.cursor++
			}
		case "k", "up":
			if m.cursor > 0 {
				m.cursor--
			}
		case "a":
			// TODO: add MCP server
		case "r":
			// TODO: remove MCP server
		case "e":
			// TODO: edit MCP server config
		}
	}
	return nil
}

func (m *MCPModel) View() string {
	modalWidth := m.width * 2 / 3
	if modalWidth < 50 {
		modalWidth = 50
	}
	if modalWidth > 80 {
		modalWidth = 80
	}

	var b strings.Builder

	title := styleHeader.Render(fmt.Sprintf("MCP Servers for %q", m.projectID))
	b.WriteString(title + "\n\n")

	if m.loading {
		b.WriteString("  Loading...")
	} else if m.err != nil {
		b.WriteString(styleError.Render(fmt.Sprintf("  Error: %v", m.err)))
	} else if len(m.servers) == 0 {
		b.WriteString(styleDim.Render("  No MCP servers configured"))
	} else {
		for i, srv := range m.servers {
			b.WriteString(m.renderServer(srv, i == m.cursor))
			b.WriteString("\n")
		}
	}

	b.WriteString("\n" + styleDim.Render("  a: add server  r: remove  e: edit config  esc: back"))

	content := b.String()
	modal := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(colorPrimary).
		Width(modalWidth).
		Padding(1, 2).
		Render(content)

	// Center
	padTop := (m.height - 15) / 2
	padLeft := (m.width - modalWidth - 2) / 2
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

func (m *MCPModel) renderServer(srv MCPServer, selected bool) string {
	var statusIcon string
	var statusStyle lipgloss.Style

	switch srv.Status {
	case "connected":
		statusIcon = "✓"
		statusStyle = lipgloss.NewStyle().Foreground(colorSuccess)
	case "disconnected":
		statusIcon = "✗"
		statusStyle = lipgloss.NewStyle().Foreground(colorError)
	case "error":
		statusIcon = "!"
		statusStyle = lipgloss.NewStyle().Foreground(colorWarning)
	default:
		statusIcon = "?"
		statusStyle = styleDim
	}

	name := lipgloss.NewStyle().Bold(true).Foreground(colorText).Render(srv.Name)
	status := statusStyle.Render(statusIcon)
	detail := ""
	if srv.ToolCount > 0 {
		detail = styleDim.Render(fmt.Sprintf("(%d tools)", srv.ToolCount))
	} else {
		detail = styleDim.Render("(disconnected)")
	}

	line := fmt.Sprintf("  %s %-20s %s", status, name, detail)

	if selected {
		return lipgloss.NewStyle().
			Background(colorSelected).
			Foreground(colorText).
			Render(line)
	}
	return line
}
