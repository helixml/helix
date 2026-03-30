package tui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// RenderMarkdown renders a subset of markdown for terminal display.
// Handles: headers, bold, italic, code blocks, inline code, lists, checkboxes.
func RenderMarkdown(text string, width int) string {
	if width < 10 {
		width = 80
	}

	lines := strings.Split(text, "\n")
	var result []string
	inCodeBlock := false
	codeLang := ""

	for _, line := range lines {
		// Code blocks
		if strings.HasPrefix(line, "```") {
			if inCodeBlock {
				inCodeBlock = false
				result = append(result, styleDim.Render("  └"+strings.Repeat("─", width-4)+"┘"))
				continue
			}
			inCodeBlock = true
			codeLang = strings.TrimPrefix(line, "```")
			label := "code"
			if codeLang != "" {
				label = codeLang
			}
			result = append(result, styleDim.Render("  ┌─ "+label+" "+strings.Repeat("─", max(0, width-len(label)-8))+"┐"))
			continue
		}

		if inCodeBlock {
			codeStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("229"))
			result = append(result, styleDim.Render("  │ ")+codeStyle.Render(truncate(line, width-6))+styleDim.Render(" │"))
			continue
		}

		// H1
		if strings.HasPrefix(line, "# ") {
			result = append(result, lipgloss.NewStyle().
				Bold(true).
				Foreground(colorHeader).
				Render(line[2:]))
			continue
		}

		// H2
		if strings.HasPrefix(line, "## ") {
			result = append(result, lipgloss.NewStyle().
				Bold(true).
				Foreground(colorHeader).
				Render(line[3:]))
			continue
		}

		// H3
		if strings.HasPrefix(line, "### ") {
			result = append(result, lipgloss.NewStyle().
				Bold(true).
				Foreground(colorText).
				Render(line[4:]))
			continue
		}

		// Checkbox lists
		if strings.HasPrefix(line, "- [ ] ") {
			result = append(result, "  "+styleDim.Render("☐")+" "+renderInlineMarkdown(line[6:]))
			continue
		}
		if strings.HasPrefix(line, "- [x] ") || strings.HasPrefix(line, "- [X] ") {
			result = append(result, "  "+lipgloss.NewStyle().Foreground(colorSuccess).Render("☑")+" "+renderInlineMarkdown(line[6:]))
			continue
		}

		// Bullet lists
		if strings.HasPrefix(line, "- ") {
			result = append(result, "  "+styleDim.Render("•")+" "+renderInlineMarkdown(line[2:]))
			continue
		}
		if strings.HasPrefix(line, "* ") {
			result = append(result, "  "+styleDim.Render("•")+" "+renderInlineMarkdown(line[2:]))
			continue
		}

		// Numbered lists
		if len(line) > 2 && line[0] >= '1' && line[0] <= '9' && line[1] == '.' {
			result = append(result, "  "+renderInlineMarkdown(line))
			continue
		}

		// Blockquotes
		if strings.HasPrefix(line, "> ") {
			quoteStyle := lipgloss.NewStyle().
				Foreground(colorDim).
				Italic(true)
			result = append(result, "  "+lipgloss.NewStyle().Foreground(colorPrimary).Render("│")+" "+quoteStyle.Render(line[2:]))
			continue
		}

		// Horizontal rule
		if line == "---" || line == "***" || line == "___" {
			result = append(result, styleDim.Render(strings.Repeat("─", width)))
			continue
		}

		// Empty line
		if line == "" {
			result = append(result, "")
			continue
		}

		// Regular text with inline formatting
		result = append(result, renderInlineMarkdown(line))
	}

	return strings.Join(result, "\n")
}

// renderInlineMarkdown handles inline formatting: **bold**, *italic*, `code`.
func renderInlineMarkdown(text string) string {
	var result strings.Builder
	i := 0
	runes := []rune(text)

	for i < len(runes) {
		// Bold: **text**
		if i+1 < len(runes) && runes[i] == '*' && runes[i+1] == '*' {
			end := findClosing(runes, i+2, "**")
			if end >= 0 {
				inner := string(runes[i+2 : end])
				result.WriteString(lipgloss.NewStyle().Bold(true).Render(inner))
				i = end + 2
				continue
			}
		}

		// Italic: *text*
		if runes[i] == '*' && (i == 0 || runes[i-1] == ' ') {
			end := findClosingRune(runes, i+1, '*')
			if end >= 0 {
				inner := string(runes[i+1 : end])
				result.WriteString(lipgloss.NewStyle().Italic(true).Render(inner))
				i = end + 1
				continue
			}
		}

		// Inline code: `code`
		if runes[i] == '`' {
			end := findClosingRune(runes, i+1, '`')
			if end >= 0 {
				inner := string(runes[i+1 : end])
				codeStyle := lipgloss.NewStyle().
					Foreground(lipgloss.Color("229")).
					Background(lipgloss.Color("236"))
				result.WriteString(codeStyle.Render(inner))
				i = end + 1
				continue
			}
		}

		result.WriteRune(runes[i])
		i++
	}

	return result.String()
}

func findClosing(runes []rune, start int, delim string) int {
	delimRunes := []rune(delim)
	for i := start; i <= len(runes)-len(delimRunes); i++ {
		match := true
		for j := 0; j < len(delimRunes); j++ {
			if runes[i+j] != delimRunes[j] {
				match = false
				break
			}
		}
		if match {
			return i
		}
	}
	return -1
}

func findClosingRune(runes []rune, start int, ch rune) int {
	for i := start; i < len(runes); i++ {
		if runes[i] == ch {
			return i
		}
	}
	return -1
}
