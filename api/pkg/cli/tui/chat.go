package tui

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/helixml/helix/api/pkg/server/wsprotocol"
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
	input        *InputModel
	spinner      *Spinner
	toolRenderer *ToolCallRenderer
	slashReg     *SlashCommandRegistry
	outbox       *Outbox
	tmuxPrefix   string
	projectName  string

	scrollOffset int
	loading      bool
	sending      bool
	agentBusy    bool
	err          error
	width        int
	height       int
	focused      bool

	// Spinner tick
	spinnerTick int
}

type interactionsLoadedMsg struct {
	sessionID    string
	interactions []*types.Interaction
}

type chatResponseMsg struct {
	sessionID string
	response  string
}

type spinnerTickMsg struct{}
type escFromChatMsg struct{}

func NewChatModel(api *APIClient, task *types.SpecTask) *ChatModel {
	sessionID := task.PlanningSessionID
	name := taskDisplayName(task)
	input := NewInputModel()
	return &ChatModel{
		api:          api,
		task:         task,
		sessionID:    sessionID,
		sessionName:  name,
		appID:        task.HelixAppID,
		input:        input,
		slashReg:     NewSlashCommandRegistry(),
		outbox:       NewOutbox(),
		loading:      true,
		focused:      true,
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
	c.input.SetWidth(w)
	c.toolRenderer = NewToolCallRenderer(w)
}

func (c *ChatModel) SetFocused(f bool) {
	c.focused = f
	c.input.SetFocused(f)
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
			c.scrollToBottom()

			// Check if agent is still working
			if len(c.interactions) > 0 {
				last := c.interactions[len(c.interactions)-1]
				if last.State == types.InteractionStateWaiting {
					c.agentBusy = true
					c.spinner = NewSpinner(c.tmuxPrefix)
					c.input.SetAgentBusy(true)
					return c.spinnerTickCmd()
				}
			}
			c.agentBusy = false
			c.input.SetAgentBusy(false)
		}
		return nil

	case chatResponseMsg:
		c.sending = false
		c.input.SetSending(false)
		return c.fetchInteractions()

	case spinnerTickMsg:
		if c.spinner != nil && c.agentBusy {
			c.spinner.Tick()
			return c.spinnerTickCmd()
		}
		return nil

	case errMsg:
		c.err = msg.err
		c.loading = false
		c.sending = false
		c.input.SetSending(false)
		return nil

	case tea.KeyMsg:
		if !c.focused {
			return nil
		}

		switch msg.String() {
		case "up":
			c.input.HistoryUp()
			return nil

		case "down":
			c.input.HistoryDown()
			return nil

		case "pgup", "shift+up", "alt+up", "ctrl+up":
			if c.scrollOffset > 0 {
				c.scrollOffset -= c.height / 2
				if c.scrollOffset < 0 {
					c.scrollOffset = 0
				}
			}
			return nil

		case "pgdown", "shift+down", "alt+down", "ctrl+down":
			c.scrollOffset += c.height / 2
			c.clampScroll()
			return nil

		case "ctrl+d":
			// Page down (vim half-page)
			c.scrollOffset += c.height / 2
			c.clampScroll()
			return nil

		case "enter":
			if !c.input.IsEmpty() {
				value := c.input.Value()

				// Handle slash commands
				if IsSlashCommand(value) {
					cmd, args := ParseSlashCommand(value)
					c.input.Clear()
					return c.handleSlashCommand(cmd, args)
				}

				// Enter = queue (doesn't interrupt current agent work)
				return c.sendPrompt(false)
			}

		case "ctrl+enter":
			if !c.input.IsEmpty() {
				// Ctrl+Enter = interrupt current agent work
				return c.sendPrompt(true)
			}

		case "shift+enter":
			c.input.InsertNewline()

		case "backspace":
			c.input.Backspace()

		case "delete":
			c.input.Delete()

		case "left":
			c.input.MoveLeft()

		case "right":
			c.input.MoveRight()

		case "home", "ctrl+a":
			c.input.MoveHome()

		case "end", "ctrl+e":
			c.input.MoveEnd()

		case "ctrl+u":
			if c.input.IsEmpty() {
				// Page up (vim half-page) when not typing
				if c.scrollOffset > 0 {
					c.scrollOffset -= c.height / 2
					if c.scrollOffset < 0 {
						c.scrollOffset = 0
					}
				}
				return nil
			}
			c.input.DeleteToStart()

		case "ctrl+k":
			c.input.DeleteToEnd()

		case "ctrl+w":
			c.input.DeleteWord()

		case "esc":
			if !c.input.IsEmpty() {
				c.input.Clear()
				return nil
			}
			if c.agentBusy {
				// Stop the agent
				c.agentBusy = false
				c.spinner = nil
				c.input.SetAgentBusy(false)
				return c.stopAgent()
			}
			// Agent idle, input empty — signal to go back to kanban
			return func() tea.Msg { return escFromChatMsg{} }

		default:
			if msg.Type == tea.KeyRunes {
				c.input.InsertRunes([]rune(msg.String()))
			} else if msg.String() == " " {
				c.input.InsertRunes([]rune{' '})
			}
		}
	}
	return nil
}

// sendPrompt queues a prompt via the prompt history sync API.
// If interrupt is true, it interrupts the current agent work (enter key).
// If false, it queues behind any in-flight work (ctrl+enter).
// Prompts are stored locally in the outbox and synced to the server.
func (c *ChatModel) sendPrompt(interrupt bool) tea.Cmd {
	message := c.input.Value()
	c.input.Clear()
	c.sending = true
	c.input.SetSending(true)

	task := c.task
	sessionID := c.sessionID

	// Generate a client-side ID for dedup
	promptID := fmt.Sprintf("tui_%d", time.Now().UnixNano())

	// Queue in the local outbox (survives disconnects)
	if c.outbox != nil {
		c.outbox.Enqueue(&types.SessionChatRequest{
			SessionID: sessionID,
			Messages: []*types.Message{{
				Role:    "user",
				Content: types.MessageContent{ContentType: types.MessageContentTypeText, Parts: []any{message}},
			}},
		})
	}

	return func() tea.Msg {
		// If no task ID, this is a new chat — create the spec task first
		if task.ID == "" {
			newTask, err := c.api.CreateTaskFromPrompt(apiCtx(), &types.CreateTaskRequest{
				ProjectID: task.ProjectID,
				Prompt:    message,
				Type:      "task",
				Priority:  types.SpecTaskPriorityMedium,
			})
			if err != nil {
				return errMsg{fmt.Errorf("failed to create task: %w", err)}
			}

			// Update the chat model with the real task
			c.task = newTask
			c.sessionID = newTask.PlanningSessionID
			c.sessionName = taskDisplayName(newTask)
			c.appID = newTask.HelixAppID

			return chatResponseMsg{sessionID: newTask.PlanningSessionID, response: ""}
		}

		interruptPtr := &interrupt
		syncReq := &types.PromptHistorySyncRequest{
			ProjectID:  task.ProjectID,
			SpecTaskID: task.ID,
			Entries: []types.PromptHistoryEntrySync{
				{
					ID:        promptID,
					SessionID: sessionID,
					Content:   message,
					Status:    "pending",
					Timestamp: time.Now().UnixMilli(),
					Interrupt: interruptPtr,
				},
			},
		}

		_, err := c.api.SyncPromptHistory(apiCtx(), syncReq)
		if err != nil {
			return errMsg{fmt.Errorf("queued locally (sync failed: %v)", err)}
		}

		c.outbox.MarkSent(promptID)
		return chatResponseMsg{sessionID: sessionID, response: ""}
	}
}

func (c *ChatModel) handleSlashCommand(cmd, args string) tea.Cmd {
	// TODO: implement individual slash commands
	_ = args
	switch cmd {
	case "web":
		if c.task != nil {
			return func() tea.Msg {
				url := c.api.WebURL(c.task.ProjectID, c.task.ID)
				return statusMsg("Open: " + url)
			}
		}
	case "status":
		if c.task != nil {
			return func() tea.Msg {
				return statusMsg("Status: " + string(c.task.Status) + " | Branch: " + c.task.BranchName)
			}
		}
	}
	return nil
}

func (c *ChatModel) stopAgent() tea.Cmd {
	task := c.task
	if task == nil || task.ID == "" {
		return nil
	}
	return func() tea.Msg {
		_ = c.api.StopAgent(apiCtx(), task.ID)
		// Also mark the latest interaction as complete locally
		// (the server will handle the actual stop)
		return statusMsg("Agent stopped")
	}
}

func (c *ChatModel) spinnerTickCmd() tea.Cmd {
	return tea.Tick(800*time.Millisecond, func(t time.Time) tea.Msg {
		return spinnerTickMsg{}
	})
}

func (c *ChatModel) scrollToBottom() {
	c.scrollOffset = c.maxScroll()
}

// clampScroll ensures scrollOffset doesn't go past the end of content.
func (c *ChatModel) clampScroll() {
	max := c.maxScroll()
	if c.scrollOffset > max {
		c.scrollOffset = max
	}
	if c.scrollOffset < 0 {
		c.scrollOffset = 0
	}
}

func (c *ChatModel) maxScroll() int {
	totalLines := c.countContentLines()
	viewHeight := c.height - c.input.ViewHeight() - 2 // header
	if totalLines > viewHeight {
		return totalLines - viewHeight
	}
	return 0
}

func (c *ChatModel) countContentLines() int {
	count := 0
	for _, ix := range c.interactions {
		count += len(c.renderInteraction(ix, c.width))
	}
	return count
}

func (c *ChatModel) View() string {
	contentWidth := c.width
	if contentWidth < 10 {
		contentWidth = 80
	}

	// Header
	header := c.renderHeader(contentWidth)

	// Input area height
	inputHeight := c.input.ViewHeight()

	// Messages area
	messagesHeight := c.height - inputHeight - 1 // header
	if messagesHeight < 1 {
		messagesHeight = 1
	}
	messages := c.renderMessages(contentWidth, messagesHeight)

	// Slash command completions
	slashCompletions := ""
	if c.focused && IsSlashCommand(c.input.Value()) {
		_, prefix := ParseSlashCommand(c.input.Value())
		_ = prefix
		completions := c.slashReg.RenderCompletions(strings.TrimPrefix(c.input.Value(), "/"), contentWidth)
		if completions != "" {
			slashCompletions = completions
		}
	}

	// Input
	input := c.input.View()

	parts := []string{header, messages}
	if c.spinner != nil && c.agentBusy {
		parts = append(parts, c.spinner.View())
	}
	if slashCompletions != "" {
		parts = append(parts, slashCompletions)
	}
	parts = append(parts, input)

	return strings.Join(parts, "\n")
}

func (c *ChatModel) renderHeader(width int) string {
	if c.task == nil {
		return styleHeader.Render(c.sessionName)
	}

	name := c.sessionName
	status := string(c.task.Status)
	branch := c.task.BranchName

	var parts []string
	if c.projectName != "" {
		parts = append(parts, styleDim.Render(c.projectName+" /"))
	}
	parts = append(parts, styleHeader.Render(name))
	parts = append(parts, styleDim.Render(status))
	if branch != "" {
		parts = append(parts, styleDim.Render(branch))
	}
	return strings.Join(parts, styleDim.Render(" · "))
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

	// Clamp scroll — never show whitespace past the end
	maxScroll := len(allLines) - height
	if maxScroll < 0 {
		maxScroll = 0
	}
	if c.scrollOffset > maxScroll {
		c.scrollOffset = maxScroll
	}
	if c.scrollOffset < 0 {
		c.scrollOffset = 0
	}

	start := c.scrollOffset
	end := start + height
	if end > len(allLines) {
		end = len(allLines)
	}

	visible := allLines[start:end]

	// Only pad if content is shorter than viewport (no scrolling needed)
	if len(allLines) < height {
		for len(visible) < height {
			visible = append(visible, "")
		}
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

	// Render from ResponseEntries (structured entries from Zed WebSocket sync)
	hasResponse := false
	if len(ix.ResponseEntries) > 0 {
		var entries []wsprotocol.ResponseEntry
		if err := json.Unmarshal(ix.ResponseEntries, &entries); err == nil && len(entries) > 0 {
			lines = append(lines, "")
			lines = append(lines, "  "+styleRoleAssistant.Render("Assistant"))
			for _, entry := range entries {
				lines = append(lines, c.renderResponseEntry(entry, contentWidth)...)
			}
			hasResponse = true
		}
	}

	// Fallback: ResponseMessage (for interactions without structured entries)
	if !hasResponse && ix.ResponseMessage != "" {
		lines = append(lines, "")
		lines = append(lines, "  "+styleRoleAssistant.Render("Assistant"))
		for _, line := range wrapText(ix.ResponseMessage, contentWidth) {
			lines = append(lines, "  "+line)
		}
	}

	if ix.Error != "" {
		lines = append(lines, "")
		lines = append(lines, "  "+styleError.Render("Error: "+ix.Error))
	}

	return lines
}

func (c *ChatModel) renderResponseEntry(entry wsprotocol.ResponseEntry, width int) []string {
	switch entry.Type {
	case "tool_call":
		return c.renderToolCallEntry(entry, width)
	case "text":
		var lines []string
		for _, line := range wrapText(entry.Content, width) {
			lines = append(lines, "  "+line)
		}
		return lines
	default:
		return []string{"  " + styleDim.Render(entry.Content)}
	}
}

func (c *ChatModel) renderToolCallEntry(entry wsprotocol.ResponseEntry, width int) []string {
	var lines []string

	// Tool call header with status
	icon := lipgloss.NewStyle().Foreground(colorPrimary).Render("✽")
	name := entry.ToolName
	if name == "" {
		name = "Tool Call"
	}

	statusStyle := styleDim
	statusIcon := ""
	switch entry.ToolStatus {
	case "Completed":
		statusStyle = lipgloss.NewStyle().Foreground(colorSuccess)
		statusIcon = " ✓"
	case "Running", "In Progress":
		statusStyle = lipgloss.NewStyle().Foreground(colorWarning)
		statusIcon = " ⟳"
	case "Error", "Failed":
		statusStyle = lipgloss.NewStyle().Foreground(colorError)
		statusIcon = " ✗"
	}

	header := fmt.Sprintf("  %s %s%s", icon, name, statusStyle.Render(statusIcon))
	lines = append(lines, header)

	// Tool call content
	if entry.Content != "" {
		contentLines := strings.Split(entry.Content, "\n")
		for _, cl := range contentLines {
			// Detect diff lines within tool call content
			if strings.HasPrefix(cl, "+") && !strings.HasPrefix(cl, "+++") {
				lines = append(lines, "  "+diffAddStyle.Render("  "+truncate(cl, width-6)))
			} else if strings.HasPrefix(cl, "-") && !strings.HasPrefix(cl, "---") {
				lines = append(lines, "  "+diffRemoveStyle.Render("  "+truncate(cl, width-6)))
			} else if strings.HasPrefix(cl, "$") {
				// Command
				cmdStyle := lipgloss.NewStyle().Foreground(colorText)
				lines = append(lines, "  "+styleDim.Render("  ⎿  ")+cmdStyle.Render(truncate(cl, width-10)))
			} else {
				lines = append(lines, "  "+styleDim.Render("  ⎿  "+truncate(cl, width-10)))
			}
		}
	}

	lines = append(lines, "") // blank line after tool call
	return lines
}

func (c *ChatModel) getPromptText(ix *types.Interaction) string {
	if ix.PromptMessage != "" {
		return ix.PromptMessage
	}
	for _, part := range ix.PromptMessageContent.Parts {
		if s, ok := part.(string); ok {
			return s
		}
	}
	return ""
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
