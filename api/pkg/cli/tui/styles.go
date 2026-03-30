package tui

import "github.com/charmbracelet/lipgloss"

// Colors — muted palette inspired by Claude Code
var (
	colorPrimary    = lipgloss.Color("63")  // muted blue-purple
	colorSecondary  = lipgloss.Color("241") // dim gray
	colorText       = lipgloss.Color("252") // light gray
	colorDim        = lipgloss.Color("241") // dim
	colorBorder     = lipgloss.Color("238") // dark border
	colorBorderFoc  = lipgloss.Color("63")  // focused border
	colorBg         = lipgloss.Color("235") // dark bg
	colorSelected   = lipgloss.Color("237") // selected row bg
	colorHeader     = lipgloss.Color("229") // warm white for headers
	colorError      = lipgloss.Color("196") // red
	colorSuccess    = lipgloss.Color("78")  // green
	colorWarning    = lipgloss.Color("214") // orange
	colorUserMsg    = lipgloss.Color("252") // user message text
	colorAssistMsg  = lipgloss.Color("252") // assistant message text
	colorRoleUser   = lipgloss.Color("111") // "You" label
	colorRoleAssist = lipgloss.Color("183") // "Assistant" label

	// Priority colors
	colorPrioLow      = lipgloss.Color("241")
	colorPrioMedium   = lipgloss.Color("229")
	colorPrioHigh     = lipgloss.Color("214")
	colorPrioCritical = lipgloss.Color("196")

	// Status colors
	colorStatusBacklog = lipgloss.Color("241")
	colorStatusPlan    = lipgloss.Color("111")
	colorStatusActive  = lipgloss.Color("78")
	colorStatusReview  = lipgloss.Color("214")
	colorStatusDone    = lipgloss.Color("241")
)

// Reusable styles
var (
	styleBorderFocused = lipgloss.NewStyle().
				Border(lipgloss.RoundedBorder()).
				BorderForeground(colorBorderFoc)

	styleBorderUnfocused = lipgloss.NewStyle().
				Border(lipgloss.RoundedBorder()).
				BorderForeground(colorBorder)

	styleHeader = lipgloss.NewStyle().
			Bold(true).
			Foreground(colorHeader)

	styleDim = lipgloss.NewStyle().
			Foreground(colorDim)

	styleSelected = lipgloss.NewStyle().
			Background(colorSelected).
			Foreground(colorText)

	styleNormal = lipgloss.NewStyle().
			Foreground(colorText)

	styleStatusBar = lipgloss.NewStyle().
			Foreground(colorDim).
			Background(colorBg).
			Padding(0, 1)

	styleError = lipgloss.NewStyle().
			Foreground(colorError)

	styleRoleUser = lipgloss.NewStyle().
			Bold(true).
			Foreground(colorRoleUser)

	styleRoleAssistant = lipgloss.NewStyle().
				Bold(true).
				Foreground(colorRoleAssist)
)

// priorityStyle returns a styled string for a priority level.
func priorityStyle(p string) string {
	switch p {
	case "critical":
		return lipgloss.NewStyle().Foreground(colorPrioCritical).Bold(true).Render("!!!")
	case "high":
		return lipgloss.NewStyle().Foreground(colorPrioHigh).Render("!!")
	case "medium":
		return lipgloss.NewStyle().Foreground(colorPrioMedium).Render("!")
	default:
		return styleDim.Render("-")
	}
}

// statusColor returns the appropriate color for a kanban column.
func statusColor(col KanbanColumn) lipgloss.Color {
	switch col {
	case ColBacklog:
		return colorStatusBacklog
	case ColPlanning:
		return colorStatusPlan
	case ColInProgress:
		return colorStatusActive
	case ColReview:
		return colorStatusReview
	case ColDone:
		return colorStatusDone
	default:
		return colorDim
	}
}
