package tui

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/helixml/helix/api/pkg/types"
)

// ChatModel renders a conversation for a spec task.
type ChatModel struct {
	api         *APIClient
	task        *types.SpecTask
	sessionID   string
	sessionName string
	appID       string

	interactions []*types.Interaction
	input        string
	scrollOffset int
	loading      bool
	sending      bool
	err          error
	width        int
	height       int
	focused      bool
}

type interactionsLoadedMsg struct {
	sessionID    string
	interactions []*types.Interaction
}

type chatResponseMsg struct {
	sessionID string
	response  string
}

func NewChatModel(api *APIClient, task *types.SpecTask) *ChatModel {
	sessionID := task.PlanningSessionID
	name := taskDisplayName(task)
	return &ChatModel{
		api:         api,
		task:        task,
		sessionID:   sessionID,
		sessionName: name,
		appID:       task.HelixAppID,
		loading:     true,
		focused:     true,
	}
}

func (c *ChatModel) Init() tea.Cmd {
	if c.sessionID == "" {
		c.loading = false
		return nil
	}
	return c.fetchInteractions()
}

func (c *ChatModel) SetSize(w, h int) {
	c.width = w
	c.height = h
}

func (c *ChatModel) SetFocused(f bool) {
	c.focused = f
}

func (c *ChatModel) fetchInteractions() tea.Cmd {
	sid := c.sessionID
	return func() tea.Msg {
		interactions, err := c.api.ListInteractions(apiCtx(), sid)
		if err != nil {
			return errMsg{err}
		}
		return interactionsLoadedMsg{sessionID: sid, interactions: interactions}
	}
}

func (c *ChatModel) Update(msg tea.Msg) tea.Cmd {
	switch msg := msg.(type) {
	case interactionsLoadedMsg:
		if msg.sessionID == c.sessionID {
			c.interactions = msg.interactions
			c.loading = false
			// Scroll to bottom
			c.scrollToBottom()
		}
		return nil

	case chatResponseMsg:
		c.sending = false
		// Refresh interactions to show the response
		return c.fetchInteractions()

	case errMsg:
		c.err = msg.err
		c.loading = false
		c.sending = false
		return nil

	case tea.KeyMsg:
		if !c.focused {
			return nil
		}

		switch msg.String() {
		case "up":
			if c.scrollOffset > 0 {
				c.scrollOffset--
			}
		case "down":
			c.scrollOffset++

		case "enter":
			if c.input != "" {
				return c.sendMessage()
			}

		case "backspace":
			if len(c.input) > 0 {
				c.input = c.input[:len(c.input)-1]
			}

		case "esc":
			if c.input != "" {
				c.input = ""
				return nil
			}
			// TODO: stop agent if running
			return nil

		default:
			// Append printable characters to input
			if msg.Type == tea.KeyRunes {
				c.input += msg.String()
			} else if msg.String() == " " {
				c.input += " "
			}
		}
	}
	return nil
}

func (c *ChatModel) sendMessage() tea.Cmd {
	message := c.input
	c.input = ""
	c.sending = true

	appID := c.appID
	sessionID := c.sessionID

	return func() tea.Msg {
		req := &types.SessionChatRequest{
			AppID:     appID,
			SessionID: sessionID,
			Messages: []*types.Message{
				{
					Role: "user",
					Content: types.MessageContent{
						ContentType: types.MessageContentTypeText,
						Parts:       []any{message},
					},
				},
			},
			Type: types.SessionTypeText,
		}

		resp, err := c.api.ChatSession(apiCtx(), req)
		if err != nil {
			return errMsg{err}
		}
		return chatResponseMsg{sessionID: sessionID, response: resp}
	}
}

func (c *ChatModel) scrollToBottom() {
	totalLines := c.countContentLines()
	viewHeight := c.height - 3 // header + input
	if totalLines > viewHeight {
		c.scrollOffset = totalLines - viewHeight
	} else {
		c.scrollOffset = 0
	}
}

func (c *ChatModel) countContentLines() int {
	count := 0
	for _, ix := range c.interactions {
		count += c.interactionLineCount(ix)
	}
	return count
}

func (c *ChatModel) interactionLineCount(ix *types.Interaction) int {
	count := 0
	// User message
	prompt := c.getPromptText(ix)
	if prompt != "" {
		count += 2 + lineCount(prompt, c.width-4) // role label + blank + content
	}
	// Assistant response
	if ix.ResponseMessage != "" {
		count += 2 + lineCount(ix.ResponseMessage, c.width-4)
	}
	count++ // blank line between interactions
	return count
}

func (c *ChatModel) View() string {
	contentWidth := c.width
	if contentWidth < 10 {
		contentWidth = 10
	}

	// Header
	header := c.renderHeader(contentWidth)

	// Messages area
	messagesHeight := c.height - 3 // header + input line + border
	if messagesHeight < 1 {
		messagesHeight = 1
	}
	messages := c.renderMessages(contentWidth, messagesHeight)

	// Input
	input := c.renderInput(contentWidth)

	return header + "\n" + messages + "\n" + input
}

func (c *ChatModel) renderHeader(width int) string {
	name := c.sessionName
	if c.task != nil {
		status := string(c.task.Status)
		prio := string(c.task.Priority)
		branch := c.task.BranchName

		parts := []string{styleHeader.Render(name)}
		parts = append(parts, styleDim.Render(status))
		if prio != "" {
			parts = append(parts, priorityStyle(prio))
		}
		if branch != "" {
			parts = append(parts, styleDim.Render(branch))
		}
		return strings.Join(parts, styleDim.Render(" · "))
	}
	return styleHeader.Render(name)
}

func (c *ChatModel) renderMessages(width, height int) string {
	if c.loading {
		return "\n  Loading conversation..."
	}
	if c.sessionID == "" {
		return "\n  No session yet. Send a message to start."
	}
	if len(c.interactions) == 0 {
		return "\n  No messages yet."
	}

	// Build all lines
	var allLines []string
	for _, ix := range c.interactions {
		allLines = append(allLines, c.renderInteraction(ix, width)...)
	}

	if c.sending {
		allLines = append(allLines, "")
		allLines = append(allLines, styleDim.Render("  Waiting for response..."))
	}

	// Apply scroll
	start := c.scrollOffset
	if start < 0 {
		start = 0
	}
	if start > len(allLines) {
		start = len(allLines)
	}
	end := start + height
	if end > len(allLines) {
		end = len(allLines)
	}

	visible := allLines[start:end]

	// Pad to fill height
	for len(visible) < height {
		visible = append(visible, "")
	}

	return strings.Join(visible, "\n")
}

func (c *ChatModel) renderInteraction(ix *types.Interaction, width int) []string {
	var lines []string
	contentWidth := width - 4

	// User message
	prompt := c.getPromptText(ix)
	if prompt != "" {
		lines = append(lines, "")
		lines = append(lines, "  "+styleRoleUser.Render("You"))
		for _, line := range wrapText(prompt, contentWidth) {
			lines = append(lines, "  "+line)
		}
	}

	// Assistant response
	resp := ix.ResponseMessage
	if ix.DisplayMessage != "" {
		resp = ix.DisplayMessage
	}
	if resp != "" {
		lines = append(lines, "")
		lines = append(lines, "  "+styleRoleAssistant.Render("Assistant"))
		for _, line := range wrapText(resp, contentWidth) {
			lines = append(lines, "  "+line)
		}
	}

	return lines
}

func (c *ChatModel) getPromptText(ix *types.Interaction) string {
	if ix.PromptMessage != "" {
		return ix.PromptMessage
	}
	// Try multi-part content
	for _, part := range ix.PromptMessageContent.Parts {
		if s, ok := part.(string); ok {
			return s
		}
	}
	return ""
}

func (c *ChatModel) renderInput(width int) string {
	if !c.focused {
		return ""
	}

	promptStyle := lipgloss.NewStyle().Foreground(colorPrimary)
	prompt := promptStyle.Render("> ")

	inputText := c.input + "█"
	if c.sending {
		inputText = styleDim.Render("sending...")
	}

	return prompt + inputText
}

// wrapText wraps text to fit within a given width.
func wrapText(text string, width int) []string {
	if width < 10 {
		width = 10
	}

	var result []string
	for _, paragraph := range strings.Split(text, "\n") {
		if paragraph == "" {
			result = append(result, "")
			continue
		}
		for len(paragraph) > width {
			// Find last space before width
			breakAt := width
			for i := width; i > width/2; i-- {
				if paragraph[i] == ' ' {
					breakAt = i
					break
				}
			}
			result = append(result, paragraph[:breakAt])
			paragraph = paragraph[breakAt:]
			if len(paragraph) > 0 && paragraph[0] == ' ' {
				paragraph = paragraph[1:]
			}
		}
		if paragraph != "" {
			result = append(result, paragraph)
		}
	}
	return result
}

func lineCount(text string, width int) int {
	return len(wrapText(text, width))
}
