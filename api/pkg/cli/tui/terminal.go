package tui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// TerminalModel represents an embedded terminal pane connected to a
// sandbox container's tmux session. The terminal proxies I/O between
// the user's terminal and a tmux session inside the sandbox.
//
// Architecture:
//   User terminal ←→ TerminalModel ←→ WebSocket ←→ sandbox container tmux
//
// The sandbox runs tmux, so detach/reattach is handled by tmux inside
// the container. TerminalModel manages the WebSocket connection and
// renders the terminal output.
type TerminalModel struct {
	taskID    string
	taskName  string
	sessionID string // Helix session ID (for RDP connection lookup)

	// Terminal state
	lines      []string // terminal output buffer (ring buffer)
	maxLines   int
	scrollback int
	cursorRow  int
	cursorCol  int

	// Connection
	connected  bool
	connecting bool
	err        error

	// Layout
	width  int
	height int
	focused bool
}

type termConnectedMsg struct {
	taskID string
}

type termDisconnectedMsg struct {
	taskID string
	err    error
}

type termOutputMsg struct {
	taskID string
	data   string
}

func NewTerminalModel(taskID, taskName, sessionID string) *TerminalModel {
	return &TerminalModel{
		taskID:    taskID,
		taskName:  taskName,
		sessionID: sessionID,
		maxLines:  1000,
		lines:     []string{""},
	}
}

func (t *TerminalModel) Init() tea.Cmd {
	t.connecting = true
	// TODO: establish WebSocket connection to sandbox
	// For now, simulate connection
	return func() tea.Msg {
		return termConnectedMsg{taskID: t.taskID}
	}
}

func (t *TerminalModel) SetSize(w, h int) {
	t.width = w
	t.height = h
}

func (t *TerminalModel) SetFocused(f bool) {
	t.focused = f
}

func (t *TerminalModel) Update(msg tea.Msg) tea.Cmd {
	switch msg := msg.(type) {
	case termConnectedMsg:
		if msg.taskID == t.taskID {
			t.connected = true
			t.connecting = false
			t.appendOutput(fmt.Sprintf("Connected to sandbox for %s\n", t.taskName))
			t.appendOutput("ubuntu@sandbox:~/project$ ")
		}
		return nil

	case termDisconnectedMsg:
		if msg.taskID == t.taskID {
			t.connected = false
			t.err = msg.err
		}
		return nil

	case termOutputMsg:
		if msg.taskID == t.taskID {
			t.appendOutput(msg.data)
		}
		return nil

	case tea.KeyMsg:
		if !t.focused {
			return nil
		}
		// In a real implementation, all keystrokes are forwarded to the
		// WebSocket connection to the sandbox's tmux session.
		// For now, echo locally.
		switch msg.String() {
		case "enter":
			t.appendOutput("\n")
			// TODO: send to WebSocket
			t.appendOutput("ubuntu@sandbox:~/project$ ")
		case "backspace":
			// Remove last char from current line
			if len(t.lines) > 0 {
				last := t.lines[len(t.lines)-1]
				if len(last) > 0 {
					t.lines[len(t.lines)-1] = last[:len(last)-1]
				}
			}
		default:
			if msg.Type == tea.KeyRunes {
				t.appendOutput(msg.String())
			} else if msg.String() == " " {
				t.appendOutput(" ")
			}
		}
	}
	return nil
}

func (t *TerminalModel) appendOutput(data string) {
	for _, ch := range data {
		if ch == '\n' {
			t.lines = append(t.lines, "")
			// Trim ring buffer
			if len(t.lines) > t.maxLines {
				t.lines = t.lines[len(t.lines)-t.maxLines:]
			}
		} else {
			t.lines[len(t.lines)-1] += string(ch)
		}
	}
}

func (t *TerminalModel) View() string {
	if t.width < 10 || t.height < 3 {
		return ""
	}

	// Header
	headerStyle := lipgloss.NewStyle().
		Foreground(colorSuccess).
		Bold(true)
	header := headerStyle.Render("shell: " + t.taskName)
	if !t.connected {
		if t.connecting {
			header += styleDim.Render(" (connecting...)")
		} else if t.err != nil {
			header += styleError.Render(fmt.Sprintf(" (error: %v)", t.err))
		} else {
			header += styleDim.Render(" (disconnected)")
		}
	}

	// Terminal content
	contentHeight := t.height - 2 // header + bottom padding
	if contentHeight < 1 {
		contentHeight = 1
	}

	// Show last N lines
	start := len(t.lines) - contentHeight
	if start < 0 {
		start = 0
	}
	visible := t.lines[start:]

	var b strings.Builder
	for i, line := range visible {
		if len(line) > t.width {
			line = line[:t.width]
		}
		b.WriteString(line)
		if i < len(visible)-1 {
			b.WriteString("\n")
		}
	}

	// Pad remaining lines
	for i := len(visible); i < contentHeight; i++ {
		b.WriteString("\n")
	}

	return header + "\n" + b.String()
}

// IsTerminal returns true (used by pane manager to distinguish from chat).
func (t *TerminalModel) IsTerminal() bool {
	return true
}
