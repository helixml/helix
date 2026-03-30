package tui

import (
	"strings"
	"unicode/utf8"

	"github.com/charmbracelet/lipgloss"
)

// InputModel is a text input that renders independently of the main view
// to avoid flickering and dropped keystrokes.
type InputModel struct {
	value      []rune
	cursor     int
	width      int
	focused    bool
	sending    bool
	agentBusy  bool
	multiline  bool
	historyIdx int
	history    []string
}

func NewInputModel() *InputModel {
	return &InputModel{
		focused:    true,
		historyIdx: -1,
	}
}

func (m *InputModel) SetWidth(w int) {
	m.width = w
}

func (m *InputModel) SetFocused(f bool) {
	m.focused = f
}

func (m *InputModel) SetSending(s bool) {
	m.sending = s
}

func (m *InputModel) SetAgentBusy(b bool) {
	m.agentBusy = b
}

// Value returns the current input text.
func (m *InputModel) Value() string {
	return string(m.value)
}

// SetValue sets the input text.
func (m *InputModel) SetValue(s string) {
	m.value = []rune(s)
	m.cursor = len(m.value)
}

// Clear empties the input.
func (m *InputModel) Clear() {
	if len(m.value) > 0 {
		m.history = append(m.history, string(m.value))
	}
	m.value = nil
	m.cursor = 0
	m.historyIdx = -1
}

// IsEmpty returns true if the input has no text.
func (m *InputModel) IsEmpty() bool {
	return len(m.value) == 0
}

// InsertRunes adds characters at the cursor position.
func (m *InputModel) InsertRunes(rs []rune) {
	tail := make([]rune, len(m.value[m.cursor:]))
	copy(tail, m.value[m.cursor:])
	m.value = append(m.value[:m.cursor], append(rs, tail...)...)
	m.cursor += len(rs)
}

// InsertNewline adds a newline at the cursor position.
func (m *InputModel) InsertNewline() {
	m.InsertRunes([]rune{'\n'})
	m.multiline = true
}

// Backspace deletes the character before the cursor.
func (m *InputModel) Backspace() {
	if m.cursor > 0 {
		m.value = append(m.value[:m.cursor-1], m.value[m.cursor:]...)
		m.cursor--
	}
}

// Delete deletes the character at the cursor.
func (m *InputModel) Delete() {
	if m.cursor < len(m.value) {
		m.value = append(m.value[:m.cursor], m.value[m.cursor+1:]...)
	}
}

// MoveLeft moves the cursor left.
func (m *InputModel) MoveLeft() {
	if m.cursor > 0 {
		m.cursor--
	}
}

// MoveRight moves the cursor right.
func (m *InputModel) MoveRight() {
	if m.cursor < len(m.value) {
		m.cursor++
	}
}

// MoveHome moves the cursor to the start.
func (m *InputModel) MoveHome() {
	m.cursor = 0
}

// MoveEnd moves the cursor to the end.
func (m *InputModel) MoveEnd() {
	m.cursor = len(m.value)
}

// HistoryUp navigates to the previous history entry.
func (m *InputModel) HistoryUp() {
	if len(m.history) == 0 {
		return
	}
	if m.historyIdx == -1 {
		m.historyIdx = len(m.history) - 1
	} else if m.historyIdx > 0 {
		m.historyIdx--
	}
	m.value = []rune(m.history[m.historyIdx])
	m.cursor = len(m.value)
}

// HistoryDown navigates to the next history entry.
func (m *InputModel) HistoryDown() {
	if m.historyIdx == -1 {
		return
	}
	if m.historyIdx < len(m.history)-1 {
		m.historyIdx++
		m.value = []rune(m.history[m.historyIdx])
		m.cursor = len(m.value)
	} else {
		m.historyIdx = -1
		m.value = nil
		m.cursor = 0
	}
}

// View renders the input area (Claude Code style).
func (m *InputModel) View() string {
	if !m.focused {
		return ""
	}

	width := m.width
	if width < 10 {
		width = 80
	}

	separator := styleDim.Render(strings.Repeat("─", width))

	var inputLine string
	if m.sending {
		inputLine = styleDim.Render("❯ sending...")
	} else {
		prompt := lipgloss.NewStyle().Foreground(colorPrimary).Bold(true).Render("❯")

		// Render text with cursor
		text := string(m.value)
		if m.cursor <= len(m.value) {
			before := string(m.value[:m.cursor])
			cursorChar := "█"
			after := ""
			if m.cursor < len(m.value) {
				cursorChar = string(m.value[m.cursor])
				after = string(m.value[m.cursor+1:])
			}
			cursorStyle := lipgloss.NewStyle().Reverse(true)
			text = before + cursorStyle.Render(cursorChar) + after
		}

		inputLine = prompt + " " + text
	}

	// Status line below
	var statusParts []string
	statusParts = append(statusParts, "⏵⏵ bypass permissions is always on (you're in a sandbox)")
	if m.agentBusy {
		statusParts = append(statusParts, "esc to interrupt")
	}
	statusLine := styleDim.Render("  " + strings.Join(statusParts, " · "))

	return separator + "\n" + inputLine + "\n" + separator + "\n" + statusLine
}

// ViewHeight returns the number of lines the input area will render.
func (m *InputModel) ViewHeight() int {
	lines := 4 // separator + input + separator + status
	// Add extra lines for multiline input
	text := string(m.value)
	lines += strings.Count(text, "\n")
	return lines
}

// runeWidth returns the display width of a string in runes.
func runeWidth(s string) int {
	return utf8.RuneCountInString(s)
}
