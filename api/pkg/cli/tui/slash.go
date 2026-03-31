package tui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// SlashCommand represents an available slash command.
type SlashCommand struct {
	Name        string
	Description string
}

// SlashCommandRegistry holds all available slash commands.
type SlashCommandRegistry struct {
	commands []SlashCommand
}

func NewSlashCommandRegistry() *SlashCommandRegistry {
	return &SlashCommandRegistry{
		commands: []SlashCommand{
			{Name: "mcp", Description: "Configure MCP servers for this project/agent"},
			{Name: "model", Description: "Switch the model for this task"},
			{Name: "approve", Description: "Approve the current spec/implementation"},
			{Name: "reject", Description: "Request changes to spec/implementation"},
			{Name: "branch", Description: "Show/change the working branch"},
			{Name: "pr", Description: "Show pull request status"},
			{Name: "logs", Description: "Show agent logs"},
			{Name: "web", Description: "Open in web browser"},
			{Name: "status", Description: "Show task status details"},
			{Name: "help", Description: "Show all available commands"},
		},
	}
}

// Match returns commands matching the given prefix.
func (r *SlashCommandRegistry) Match(prefix string) []SlashCommand {
	prefix = strings.TrimPrefix(prefix, "/")
	prefix = strings.ToLower(prefix)

	if prefix == "" {
		return r.commands
	}

	var matches []SlashCommand
	for _, cmd := range r.commands {
		if strings.HasPrefix(cmd.Name, prefix) {
			matches = append(matches, cmd)
		}
	}
	return matches
}

// RenderCompletions renders the slash command completion menu.
func (r *SlashCommandRegistry) RenderCompletions(prefix string, width int) string {
	matches := r.Match(prefix)
	if len(matches) == 0 {
		return ""
	}

	var b strings.Builder

	nameStyle := lipgloss.NewStyle().Foreground(colorPrimary).Bold(true)
	descStyle := styleDim

	for _, cmd := range matches {
		name := "/" + cmd.Name
		b.WriteString("  " + nameStyle.Render(name))

		// Pad to align descriptions
		padLen := 16 - len(name)
		if padLen < 2 {
			padLen = 2
		}
		b.WriteString(strings.Repeat(" ", padLen))
		b.WriteString(descStyle.Render(cmd.Description))
		b.WriteString("\n")
	}

	return b.String()
}

// IsSlashCommand returns true if the input starts with '/'.
func IsSlashCommand(input string) bool {
	return strings.HasPrefix(strings.TrimSpace(input), "/")
}

// ParseSlashCommand splits "/command args" into command name and args.
func ParseSlashCommand(input string) (string, string) {
	input = strings.TrimSpace(input)
	if !strings.HasPrefix(input, "/") {
		return "", input
	}
	input = input[1:] // strip /
	parts := strings.SplitN(input, " ", 2)
	cmd := parts[0]
	args := ""
	if len(parts) > 1 {
		args = parts[1]
	}
	return cmd, args
}
