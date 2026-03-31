package tui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

var (
	diffAddStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("156")).
			Background(lipgloss.Color("22"))

	diffRemoveStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("210")).
			Background(lipgloss.Color("52"))

	diffHeaderStyle = lipgloss.NewStyle().
			Foreground(colorDim).
			Bold(true)

	diffLineNumStyle = lipgloss.NewStyle().
				Foreground(colorDim)

	diffBorderStyle = lipgloss.NewStyle().
			Foreground(colorBorder)
)

// RenderDiff renders a unified diff with red/green coloring.
// Input is a unified diff string (output of git diff or similar).
func RenderDiff(filename string, diff string, width int) string {
	if width < 20 {
		width = 20
	}
	contentWidth := width - 4 // borders + padding

	var b strings.Builder

	// Header
	topBorder := diffBorderStyle.Render("┌─ " + filename + " " + strings.Repeat("─", max(0, contentWidth-len(filename)-2)) + "┐")
	b.WriteString(topBorder + "\n")

	lines := strings.Split(diff, "\n")
	for _, line := range lines {
		if line == "" {
			continue
		}

		// Skip diff headers (---, +++, @@)
		if strings.HasPrefix(line, "---") || strings.HasPrefix(line, "+++") {
			continue
		}
		if strings.HasPrefix(line, "@@") {
			// Hunk header — show it dimmed
			hunk := truncate(line, contentWidth)
			b.WriteString(diffBorderStyle.Render("│ ") + diffHeaderStyle.Render(hunk))
			padLen := contentWidth - len(hunk)
			if padLen > 0 {
				b.WriteString(strings.Repeat(" ", padLen))
			}
			b.WriteString(diffBorderStyle.Render(" │") + "\n")
			continue
		}

		var styled string
		displayLine := line

		if strings.HasPrefix(line, "+") {
			displayLine = truncate(displayLine, contentWidth)
			styled = diffAddStyle.Render(pad(displayLine, contentWidth))
		} else if strings.HasPrefix(line, "-") {
			displayLine = truncate(displayLine, contentWidth)
			styled = diffRemoveStyle.Render(pad(displayLine, contentWidth))
		} else {
			// Context line — remove leading space
			if len(displayLine) > 0 && displayLine[0] == ' ' {
				displayLine = displayLine[1:]
			}
			displayLine = truncate(displayLine, contentWidth)
			styled = pad(displayLine, contentWidth)
		}

		b.WriteString(diffBorderStyle.Render("│ ") + styled + diffBorderStyle.Render(" │") + "\n")
	}

	// Bottom border
	bottomBorder := diffBorderStyle.Render("└" + strings.Repeat("─", contentWidth+2) + "┘")
	b.WriteString(bottomBorder)

	return b.String()
}

// RenderInlineDiff renders a simple old→new change for small edits.
func RenderInlineDiff(filename string, oldText, newText string, width int) string {
	if width < 20 {
		width = 20
	}
	contentWidth := width - 4

	var b strings.Builder

	topBorder := diffBorderStyle.Render("┌─ " + filename + " " + strings.Repeat("─", max(0, contentWidth-len(filename)-2)) + "┐")
	b.WriteString(topBorder + "\n")

	// Old lines
	for _, line := range strings.Split(oldText, "\n") {
		display := "- " + truncate(line, contentWidth-2)
		styled := diffRemoveStyle.Render(pad(display, contentWidth))
		b.WriteString(diffBorderStyle.Render("│ ") + styled + diffBorderStyle.Render(" │") + "\n")
	}

	// New lines
	for _, line := range strings.Split(newText, "\n") {
		display := "+ " + truncate(line, contentWidth-2)
		styled := diffAddStyle.Render(pad(display, contentWidth))
		b.WriteString(diffBorderStyle.Render("│ ") + styled + diffBorderStyle.Render(" │") + "\n")
	}

	bottomBorder := diffBorderStyle.Render("└" + strings.Repeat("─", contentWidth+2) + "┘")
	b.WriteString(bottomBorder)

	return b.String()
}

func pad(s string, width int) string {
	if len(s) >= width {
		return s[:width]
	}
	return s + strings.Repeat(" ", width-len(s))
}

// ParseToolCallDiff extracts diff info from a tool call response.
// Returns filename, old content, new content.
func ParseToolCallDiff(toolName string, args map[string]interface{}) (string, string, string) {
	switch toolName {
	case "edit_file", "str_replace_editor":
		filename, _ := args["path"].(string)
		if filename == "" {
			filename, _ = args["file_path"].(string)
		}
		oldStr, _ := args["old_str"].(string)
		if oldStr == "" {
			oldStr, _ = args["old_string"].(string)
		}
		newStr, _ := args["new_str"].(string)
		if newStr == "" {
			newStr, _ = args["new_string"].(string)
		}
		return filename, oldStr, newStr

	case "write_file", "create_file":
		filename, _ := args["path"].(string)
		if filename == "" {
			filename, _ = args["file_path"].(string)
		}
		content, _ := args["content"].(string)
		return filename, "", content

	default:
		return "", "", ""
	}
}

