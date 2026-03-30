package tui

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
	openai "github.com/sashabaranov/go-openai"
)

// ToolCallRenderer renders tool calls in the chat view.
type ToolCallRenderer struct {
	width int
}

func NewToolCallRenderer(width int) *ToolCallRenderer {
	return &ToolCallRenderer{width: width}
}

// RenderToolCall renders a single tool call as lines of styled text.
func (r *ToolCallRenderer) RenderToolCall(tc openai.ToolCall) []string {
	if tc.Function.Name == "" {
		return nil
	}

	var args map[string]interface{}
	_ = json.Unmarshal([]byte(tc.Function.Arguments), &args)

	switch tc.Function.Name {
	case "edit_file", "str_replace_editor":
		return r.renderEdit(args)
	case "write_file", "create_file":
		return r.renderCreate(args)
	case "read_file", "view_file":
		return r.renderRead(args)
	case "bash", "execute_command", "run_command":
		return r.renderCommand(args)
	case "search", "grep", "find":
		return r.renderSearch(args)
	case "list_files", "list_directory":
		return r.renderListFiles(args)
	default:
		return r.renderGeneric(tc.Function.Name, args)
	}
}

func (r *ToolCallRenderer) renderEdit(args map[string]interface{}) []string {
	filename, oldStr, newStr := ParseToolCallDiff("edit_file", args)
	if filename == "" {
		return r.renderGeneric("edit_file", args)
	}

	var lines []string

	// Header
	icon := lipgloss.NewStyle().Foreground(colorPrimary).Render("✽")
	lines = append(lines, fmt.Sprintf("  %s Editing %s", icon, styleDim.Render(filename)))
	lines = append(lines, "")

	if oldStr != "" && newStr != "" {
		// Render inline diff
		diff := RenderInlineDiff(filename, oldStr, newStr, r.width-4)
		for _, line := range strings.Split(diff, "\n") {
			lines = append(lines, "  "+line)
		}
	}

	return lines
}

func (r *ToolCallRenderer) renderCreate(args map[string]interface{}) []string {
	filename, _, content := ParseToolCallDiff("write_file", args)
	if filename == "" {
		return r.renderGeneric("write_file", args)
	}

	icon := lipgloss.NewStyle().Foreground(colorSuccess).Render("✽")
	lineCount := len(strings.Split(content, "\n"))
	return []string{
		fmt.Sprintf("  %s Created %s (%d lines)", icon, styleDim.Render(filename), lineCount),
	}
}

func (r *ToolCallRenderer) renderRead(args map[string]interface{}) []string {
	path, _ := args["path"].(string)
	if path == "" {
		path, _ = args["file_path"].(string)
	}

	// Line range
	detail := ""
	if startLine, ok := args["start_line"]; ok {
		if endLine, ok := args["end_line"]; ok {
			detail = fmt.Sprintf(" (lines %v-%v)", startLine, endLine)
		}
	}

	icon := lipgloss.NewStyle().Foreground(colorPrimary).Render("✽")
	return []string{
		fmt.Sprintf("  %s Read %s%s", icon, styleDim.Render(path), styleDim.Render(detail)),
	}
}

func (r *ToolCallRenderer) renderCommand(args map[string]interface{}) []string {
	cmd, _ := args["command"].(string)
	if cmd == "" {
		cmd, _ = args["cmd"].(string)
	}

	icon := lipgloss.NewStyle().Foreground(colorPrimary).Render("✽")
	cmdStyle := lipgloss.NewStyle().Foreground(colorText)

	lines := []string{
		fmt.Sprintf("  %s Running: %s", icon, cmdStyle.Render(truncate(cmd, r.width-16))),
	}

	// If there's output, show it
	if output, ok := args["output"].(string); ok && output != "" {
		outputLines := strings.Split(output, "\n")
		maxLines := 5
		if len(outputLines) > maxLines {
			for _, ol := range outputLines[:maxLines] {
				lines = append(lines, "  ⎿  "+styleDim.Render(truncate(ol, r.width-8)))
			}
			lines = append(lines, styleDim.Render(fmt.Sprintf("  ⎿  ... +%d more lines", len(outputLines)-maxLines)))
		} else {
			for _, ol := range outputLines {
				lines = append(lines, "  ⎿  "+styleDim.Render(truncate(ol, r.width-8)))
			}
		}
	}

	return lines
}

func (r *ToolCallRenderer) renderSearch(args map[string]interface{}) []string {
	query, _ := args["query"].(string)
	if query == "" {
		query, _ = args["pattern"].(string)
	}

	icon := lipgloss.NewStyle().Foreground(colorPrimary).Render("✽")
	return []string{
		fmt.Sprintf("  %s Searched for %s", icon, styleDim.Render("\""+query+"\"")),
	}
}

func (r *ToolCallRenderer) renderListFiles(args map[string]interface{}) []string {
	path, _ := args["path"].(string)
	if path == "" {
		path = "."
	}

	icon := lipgloss.NewStyle().Foreground(colorPrimary).Render("✽")
	return []string{
		fmt.Sprintf("  %s Listed %s", icon, styleDim.Render(path)),
	}
}

func (r *ToolCallRenderer) renderGeneric(name string, args map[string]interface{}) []string {
	icon := lipgloss.NewStyle().Foreground(colorPrimary).Render("✽")
	summary := name
	if len(args) > 0 {
		// Show first meaningful arg
		for k, v := range args {
			if s, ok := v.(string); ok && len(s) < 60 {
				summary = fmt.Sprintf("%s(%s=%q)", name, k, s)
				break
			}
		}
	}
	return []string{
		fmt.Sprintf("  %s %s", icon, styleDim.Render(truncate(summary, r.width-8))),
	}
}
