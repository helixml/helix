package tui

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/gorilla/websocket"
)

// TerminalModel provides a persistent shell inside the sandbox container
// via WebSocket PTY. Connects to GET /sessions/{id}/terminal which proxies
// through RevDial to the desktop-bridge's /pty endpoint.
type TerminalModel struct {
	api       *APIClient
	sessionID string
	taskName  string

	// WebSocket connection
	conn      *websocket.Conn
	connected bool
	connecting bool

	// Screen buffer (last N lines of output)
	mu        sync.Mutex
	lines     []string
	maxLines  int
	scrollOff int

	// Layout
	width   int
	height  int
	focused bool
	err     error
}

// Messages for async WebSocket I/O
type termConnectedMsg struct{ sessionID string }
type termOutputMsg struct {
	sessionID string
	data      string
}
type termDisconnectedMsg struct {
	sessionID string
	err       error
}

func NewTerminalModel(api *APIClient, sessionID, taskName string) *TerminalModel {
	return &TerminalModel{
		api:       api,
		sessionID: sessionID,
		taskName:  taskName,
		maxLines:  5000,
		lines:     []string{""},
		focused:   true,
	}
}

func (t *TerminalModel) Init() tea.Cmd {
	return t.connect()
}

func (t *TerminalModel) SetSize(w, h int) {
	t.width = w
	t.height = h
	// Send resize to PTY
	if t.conn != nil && t.connected {
		msg, _ := json.Marshal(map[string]interface{}{
			"type": "resize",
			"cols": w,
			"rows": h - 2, // header
		})
		t.conn.WriteMessage(websocket.TextMessage, msg)
	}
}

func (t *TerminalModel) SetFocused(f bool) {
	t.focused = f
}

func (t *TerminalModel) connect() tea.Cmd {
	t.connecting = true
	sessionID := t.sessionID
	api := t.api

	// Store reference so we can set t.conn from the cmd result
	term := t

	return func() tea.Msg {
		// Build WebSocket URL from API URL
		baseURL := api.baseURL
		wsURL := strings.Replace(baseURL, "http://", "ws://", 1)
		wsURL = strings.Replace(wsURL, "https://", "wss://", 1)
		wsURL = fmt.Sprintf("%s/api/v1/sessions/%s/terminal?cols=80&rows=24", wsURL, sessionID)

		header := http.Header{}
		header.Set("Authorization", "Bearer "+api.apiKey())

		conn, _, err := websocket.DefaultDialer.Dial(wsURL, header)
		if err != nil {
			return termDisconnectedMsg{sessionID: sessionID, err: err}
		}

		term.conn = conn
		return termConnectedMsg{sessionID: sessionID}
	}
}

func (t *TerminalModel) startReading() tea.Cmd {
	conn := t.conn
	sessionID := t.sessionID

	return func() tea.Msg {
		_, data, err := conn.ReadMessage()
		if err != nil {
			return termDisconnectedMsg{sessionID: sessionID, err: err}
		}
		return termOutputMsg{sessionID: sessionID, data: string(data)}
	}
}

func (t *TerminalModel) Update(msg tea.Msg) tea.Cmd {
	switch msg := msg.(type) {
	case termConnectedMsg:
		if msg.sessionID == t.sessionID {
			t.connected = true
			t.connecting = false
			t.appendOutput("Connected to sandbox shell\r\n")
			return t.startReading()
		}
		return nil

	case termOutputMsg:
		if msg.sessionID == t.sessionID {
			t.appendOutput(msg.data)
			// Continue reading
			return t.startReading()
		}
		return nil

	case termDisconnectedMsg:
		if msg.sessionID == t.sessionID {
			t.connected = false
			t.connecting = false
			if msg.err != nil {
				t.err = msg.err
				t.appendOutput(fmt.Sprintf("\r\n[disconnected: %v]\r\n", msg.err))
			}
		}
		return nil

	case tea.KeyMsg:
		if !t.focused || !t.connected {
			if !t.focused {
				return nil
			}
			// Not connected — esc goes back
			if msg.String() == "esc" {
				return func() tea.Msg { return escFromChatMsg{} }
			}
			// Try reconnect on enter
			if msg.String() == "enter" && !t.connecting {
				return t.connect()
			}
			return nil
		}

		// Send keystrokes to the PTY via WebSocket
		var data []byte
		switch msg.Type {
		case tea.KeyEnter:
			data = []byte("\r")
		case tea.KeyBackspace:
			data = []byte{0x7f}
		case tea.KeyTab:
			data = []byte("\t")
		case tea.KeyEsc:
			data = []byte{0x1b}
		case tea.KeyUp:
			data = []byte("\x1b[A")
		case tea.KeyDown:
			data = []byte("\x1b[B")
		case tea.KeyRight:
			data = []byte("\x1b[C")
		case tea.KeyLeft:
			data = []byte("\x1b[D")
		case tea.KeyCtrlC:
			data = []byte{0x03}
		case tea.KeyCtrlD:
			data = []byte{0x04}
		case tea.KeyCtrlZ:
			data = []byte{0x1a}
		case tea.KeyCtrlL:
			data = []byte{0x0c}
		case tea.KeyCtrlA:
			data = []byte{0x01}
		case tea.KeyCtrlE:
			data = []byte{0x05}
		case tea.KeyCtrlU:
			data = []byte{0x15}
		case tea.KeyCtrlK:
			data = []byte{0x0b}
		case tea.KeyCtrlW:
			data = []byte{0x17}
		case tea.KeySpace:
			data = []byte(" ")
		case tea.KeyRunes:
			data = []byte(msg.String())
		default:
			return nil
		}

		if len(data) > 0 && t.conn != nil {
			t.conn.WriteMessage(websocket.BinaryMessage, data)
		}
		return nil
	}
	return nil
}

func (t *TerminalModel) appendOutput(data string) {
	t.mu.Lock()
	defer t.mu.Unlock()

	for _, ch := range data {
		switch ch {
		case '\n':
			t.lines = append(t.lines, "")
		case '\r':
			// Carriage return — go back to start of current line
			if len(t.lines) > 0 {
				// Don't add a new line, just allow overwriting
			}
		case '\x1b':
			// Start of escape sequence — pass through for now
			if len(t.lines) > 0 {
				t.lines[len(t.lines)-1] += string(ch)
			}
		default:
			if len(t.lines) > 0 {
				t.lines[len(t.lines)-1] += string(ch)
			}
		}
	}

	// Trim to max lines
	if len(t.lines) > t.maxLines {
		t.lines = t.lines[len(t.lines)-t.maxLines:]
	}
}

func (t *TerminalModel) View() string {
	w := t.width
	if w < 20 {
		w = 80
	}
	h := t.height
	if h < 3 {
		h = 24
	}

	// Header
	headerStyle := lipgloss.NewStyle().Foreground(colorSuccess).Bold(true)
	header := headerStyle.Render("shell")
	if t.taskName != "" {
		header += styleDim.Render(" · " + t.taskName)
	}

	if t.connecting {
		header += styleDim.Render(" (connecting...)")
	} else if !t.connected {
		header += styleError.Render(" (disconnected)")
		if t.err != nil {
			header += styleDim.Render(" enter: reconnect  esc: back")
		}
	}

	// Terminal content — show last N lines
	viewH := h - 2 // header + status
	if viewH < 1 {
		viewH = 1
	}

	t.mu.Lock()
	start := len(t.lines) - viewH
	if start < 0 {
		start = 0
	}
	visible := make([]string, 0, viewH)
	for i := start; i < len(t.lines); i++ {
		line := t.lines[i]
		if len(line) > w {
			line = line[:w]
		}
		visible = append(visible, line)
	}
	t.mu.Unlock()

	// Pad
	for len(visible) < viewH {
		visible = append(visible, "")
	}

	content := strings.Join(visible, "\n")
	return header + "\n" + content
}

// Close cleans up the WebSocket connection.
func (t *TerminalModel) Close() {
	if t.conn != nil {
		t.conn.Close()
	}
}
