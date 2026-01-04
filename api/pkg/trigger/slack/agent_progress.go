package slack

import (
	"context"
	"fmt"

	"github.com/rs/zerolog/log"
	"github.com/slack-go/slack"
)

// AgentProgressUpdate represents a progress update from an external agent
type AgentProgressUpdate struct {
	SessionID     string   // Helix session ID
	SessionName   string   // Session name/title
	TurnNumber    int      // Current turn number
	TurnSummary   string   // One-line summary of this turn
	Status        string   // "working", "needs_input", "completed", "error"
	ScreenshotURL string   // URL to screenshot image (optional)
	NeedsInput    bool     // Whether agent needs human input
	InputPrompt   string   // What the agent is asking (if NeedsInput)
	AppURL        string   // Base URL for links (e.g., https://app.helix.ml)
}

// PostAgentProgress posts an agent progress update to the Slack thread
// This allows agents to show their work progress with screenshots and summaries
func (s *SlackBot) PostAgentProgress(ctx context.Context, threadKey string, channel string, update *AgentProgressUpdate) error {
	if s.trigger.BotToken == "" {
		return fmt.Errorf("bot token not configured")
	}

	api := slack.New(s.trigger.BotToken)

	// Build message blocks
	blocks := s.buildProgressBlocks(update)

	// Post message to thread
	_, _, err := api.PostMessageContext(ctx, channel,
		slack.MsgOptionBlocks(blocks...),
		slack.MsgOptionTS(threadKey), // Reply in thread
	)
	if err != nil {
		log.Error().Err(err).
			Str("app_id", s.app.ID).
			Str("channel", channel).
			Str("thread_key", threadKey).
			Msg("failed to post agent progress to Slack")
		return fmt.Errorf("failed to post agent progress: %w", err)
	}

	log.Info().
		Str("app_id", s.app.ID).
		Str("session_id", update.SessionID).
		Int("turn", update.TurnNumber).
		Str("status", update.Status).
		Msg("posted agent progress to Slack thread")

	return nil
}

// buildProgressBlocks builds Slack Block Kit blocks for progress update
func (s *SlackBot) buildProgressBlocks(update *AgentProgressUpdate) []slack.Block {
	var blocks []slack.Block

	// Status emoji and header
	statusEmoji := "🔄"
	switch update.Status {
	case "working":
		statusEmoji = "🔄"
	case "needs_input":
		statusEmoji = "⚠️"
	case "completed":
		statusEmoji = "✅"
	case "error":
		statusEmoji = "❌"
	}

	// Header with session name and status
	headerText := fmt.Sprintf("%s *%s*", statusEmoji, update.SessionName)
	blocks = append(blocks, slack.NewSectionBlock(
		slack.NewTextBlockObject(slack.MarkdownType, headerText, false, false),
		nil, nil,
	))

	// Turn summary
	if update.TurnSummary != "" {
		turnText := fmt.Sprintf("*Turn %d:* %s", update.TurnNumber, update.TurnSummary)
		blocks = append(blocks, slack.NewSectionBlock(
			slack.NewTextBlockObject(slack.MarkdownType, turnText, false, false),
			nil, nil,
		))
	}

	// Screenshot image (if provided)
	if update.ScreenshotURL != "" {
		altText := fmt.Sprintf("Agent screenshot - Turn %d", update.TurnNumber)
		blocks = append(blocks, slack.NewImageBlock(
			update.ScreenshotURL,
			altText,
			"screenshot",
			nil,
		))
	}

	// Input prompt (if agent needs input)
	if update.NeedsInput && update.InputPrompt != "" {
		inputText := fmt.Sprintf("*🤔 Need input:* %s", update.InputPrompt)
		blocks = append(blocks, slack.NewSectionBlock(
			slack.NewTextBlockObject(slack.MarkdownType, inputText, false, false),
			nil, nil,
		))
	}

	// Action buttons
	sessionURL := fmt.Sprintf("%s/session/%s", update.AppURL, update.SessionID)
	viewSessionBtn := slack.NewButtonBlockElement(
		"view_session",
		update.SessionID,
		slack.NewTextBlockObject(slack.PlainTextType, "View Session", true, false),
	)
	viewSessionBtn.URL = sessionURL

	stopAgentBtn := slack.NewButtonBlockElement(
		"stop_agent",
		update.SessionID,
		slack.NewTextBlockObject(slack.PlainTextType, "Stop Agent", true, false),
	)
	stopAgentBtn.Style = slack.StyleDanger

	blocks = append(blocks, slack.NewActionBlock(
		"agent_actions",
		viewSessionBtn,
		stopAgentBtn,
	))

	// Divider
	blocks = append(blocks, slack.NewDividerBlock())

	return blocks
}

// PostSessionSummary posts a summary of the session with the table of contents
// Useful for giving users an overview of what the agent has accomplished
func (s *SlackBot) PostSessionSummary(ctx context.Context, threadKey string, channel string, sessionID string, sessionName string, summaries []TurnSummary, appURL string) error {
	if s.trigger.BotToken == "" {
		return fmt.Errorf("bot token not configured")
	}

	api := slack.New(s.trigger.BotToken)

	// Build summary text
	var summaryText string
	summaryText += fmt.Sprintf("*📋 Session Summary: %s*\n\n", sessionName)

	for _, summary := range summaries {
		summaryText += fmt.Sprintf("*%d.* %s\n", summary.TurnNumber, summary.Summary)
	}

	sessionURL := fmt.Sprintf("%s/session/%s", appURL, sessionID)
	summaryText += fmt.Sprintf("\n<<%s|View Full Session>>", sessionURL)

	// Post to thread
	_, _, err := api.PostMessageContext(ctx, channel,
		slack.MsgOptionText(summaryText, false),
		slack.MsgOptionTS(threadKey),
	)
	if err != nil {
		return fmt.Errorf("failed to post session summary: %w", err)
	}

	return nil
}

// TurnSummary represents a summary of a single turn
type TurnSummary struct {
	TurnNumber int
	Summary    string
}
