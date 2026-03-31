package tui

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// TerminalModel provides a command-line interface to the sandbox container.
// Commands are executed via POST /external-agents/{sessionID}/exec through
// the desktop-bridge's exec endpoint. Each command runs to completion
// (not a persistent shell — that requires PTY support in desktop-bridge).
type TerminalModel struct {
	api       *APIClient
	sessionID string
	taskName  string

	// Command history and output
	history   []termEntry
	input     string
	running   bool
	err       error

	// Layout
	width     int
	height    int
	focused   bool
	scrollOff int
}

type termEntry struct {
	Command  string
	Output   string
	Error    string
	ExitCode int
	Duration time.Duration
}

type termExecResultMsg struct {
	sessionID string
	entry     termEntry
}

func NewTerminalModel(api *APIClient, sessionID, taskName string) *TerminalModel {
	return &TerminalModel{
		api:       api,
		sessionID: sessionID,
		taskName:  taskName,
		focused:   true,
	}
}

func (t *TerminalModel) Init() tea.Cmd {
	return nil
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
	case termExecResultMsg:
		if msg.sessionID == t.sessionID {
			t.history = append(t.history, msg.entry)
			t.running = false
			t.scrollToBottom()
		}
		return nil

	case errMsg:
		t.err = msg.err
		t.running = false
		return nil

	case tea.KeyMsg:
		if !t.focused {
			return nil
		}

		switch msg.String() {
		case "enter":
			if t.input != "" && !t.running {
				return t.execCommand()
			}

		case "backspace":
			if len(t.input) > 0 {
				t.input = t.input[:len(t.input)-1]
			}

		case "ctrl+u":
			t.input = ""

		case "ctrl+l":
			t.history = nil
			t.scrollOff = 0

		case "esc":
			if t.input != "" {
				t.input = ""
				return nil
			}
			return func() tea.Msg { return escFromChatMsg{} }

		default:
			if msg.Type == tea.KeyRunes {
				t.input += msg.String()
			} else if msg.Type == tea.KeySpace {
				t.input += " "
			}
		}
	}
	return nil
}

func (t *TerminalModel) execCommand() tea.Cmd {
	cmd := t.input
	t.input = ""
	t.running = true
	sessionID := t.sessionID
	api := t.api

	return func() tea.Msg {
		start := time.Now()

		// Parse command into args
		args := strings.Fields(cmd)
		if len(args) == 0 {
			return nil
		}

		reqBody, _ := json.Marshal(map[string]interface{}{
			"command": args,
			"timeout": 30,
		})

		var result struct {
			Success  bool   `json:"success"`
			Output   string `json:"output"`
			Error    string `json:"error"`
			ExitCode int    `json:"exit_code"`
		}

		err := api.client.MakeRequest(
			apiCtx(),
			http.MethodPost,
			"/external-agents/"+sessionID+"/exec",
			strings.NewReader(string(reqBody)),
			&result,
		)

		entry := termEntry{
			Command:  cmd,
			Duration: time.Since(start),
		}

		if err != nil {
			entry.Error = err.Error()
			entry.ExitCode = -1
		} else {
			entry.Output = result.Output
			entry.Error = result.Error
			entry.ExitCode = result.ExitCode
		}

		return termExecResultMsg{sessionID: sessionID, entry: entry}
	}
}

func (t *TerminalModel) scrollToBottom() {
	lines := t.countLines()
	viewH := t.height - 4 // header + input + borders
	if lines > viewH {
		t.scrollOff = lines - viewH
	}
}

func (t *TerminalModel) countLines() int {
	count := 0
	for _, e := range t.history {
		count += 1 // $ command line
		count += strings.Count(e.Output, "\n") + 1
		if e.Error != "" {
			count++
		}
	}
	return count
}

func (t *TerminalModel) View() string {
	w := t.width
	if w < 20 {
		w = 80
	}

	// Header
	headerStyle := lipgloss.NewStyle().Foreground(colorSuccess).Bold(true)
	header := headerStyle.Render("shell") + styleDim.Render(" · "+t.taskName)
	if !t.focused {
		header += styleDim.Render(" (unfocused)")
	}

	// History
	viewH := t.height - 4
	if viewH < 1 {
		viewH = 1
	}

	var allLines []string
	for _, e := range t.history {
		// Command line
		cmdStyle := lipgloss.NewStyle().Foreground(colorSuccess)
		allLines = append(allLines, cmdStyle.Render("$ "+e.Command))

		// Output
		if e.Output != "" {
			for _, line := range strings.Split(e.Output, "\n") {
				allLines = append(allLines, "  "+line)
			}
		}

		// Error
		if e.Error != "" {
			allLines = append(allLines, "  "+styleError.Render(e.Error))
		}
	}

	// Apply scroll
	start := t.scrollOff
	if start > len(allLines) {
		start = len(allLines)
	}
	end := start + viewH
	if end > len(allLines) {
		end = len(allLines)
	}

	visible := allLines[start:end]
	for len(visible) < viewH {
		visible = append(visible, "")
	}

	content := strings.Join(visible, "\n")

	// Input
	var inputLine string
	if t.running {
		inputLine = styleDim.Render("$ running...")
	} else if t.focused {
		prompt := lipgloss.NewStyle().Foreground(colorSuccess).Render("$ ")
		cursor := lipgloss.NewStyle().Foreground(colorPrimary).Render("█")
		inputLine = prompt + t.input + cursor
	}

	return header + "\n" + content + "\n" + inputLine
}
