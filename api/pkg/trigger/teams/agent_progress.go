package teams

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"

	"github.com/rs/zerolog/log"
)

// AgentProgressUpdate represents a progress update from an external agent
type AgentProgressUpdate struct {
	SessionID     string // Helix session ID
	SessionName   string // Session name/title
	TurnNumber    int    // Current turn number
	TurnSummary   string // One-line summary of this turn
	Status        string // "working", "needs_input", "completed", "error"
	ScreenshotURL string // URL to screenshot image (optional)
	NeedsInput    bool   // Whether agent needs human input
	InputPrompt   string // What the agent is asking (if NeedsInput)
	AppURL        string // Base URL for links (e.g., https://app.helix.ml)
}

// TurnSummary represents a summary of a single turn
type TurnSummary struct {
	TurnNumber int
	Summary    string
}

// PostAgentProgress posts an agent progress update to a Teams conversation
// This allows agents to show their work progress with screenshots and summaries
func (t *TeamsBot) PostAgentProgress(ctx context.Context, conversationID string, serviceURL string, update *AgentProgressUpdate) error {
	// Get access token
	token, err := t.getAccessToken(ctx)
	if err != nil {
		return fmt.Errorf("failed to get access token: %w", err)
	}

	// Build the Adaptive Card for the progress update
	card := t.buildProgressCard(update)

	// Create the activity with the card
	activity := map[string]interface{}{
		"type": "message",
		"attachments": []map[string]interface{}{
			{
				"contentType": "application/vnd.microsoft.card.adaptive",
				"content":     card,
			},
		},
	}

	// Send to the conversation
	sendURL := fmt.Sprintf("%sv3/conversations/%s/activities",
		serviceURL,
		url.PathEscape(conversationID))

	activityJSON, err := json.Marshal(activity)
	if err != nil {
		return fmt.Errorf("failed to marshal activity: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", sendURL, bytes.NewReader(activityJSON))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send activity: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		log.Error().
			Str("app_id", t.app.ID).
			Int("status", resp.StatusCode).
			Str("body", string(body)).
			Msg("failed to post agent progress to Teams")
		return fmt.Errorf("send failed with status %d: %s", resp.StatusCode, string(body))
	}

	log.Info().
		Str("app_id", t.app.ID).
		Str("session_id", update.SessionID).
		Int("turn", update.TurnNumber).
		Str("status", update.Status).
		Msg("posted agent progress to Teams conversation")

	return nil
}

// buildProgressCard builds an Adaptive Card for the progress update
func (t *TeamsBot) buildProgressCard(update *AgentProgressUpdate) map[string]interface{} {
	// Status emoji
	statusEmoji := "🔄"
	statusColor := "default"
	switch update.Status {
	case "working":
		statusEmoji = "🔄"
		statusColor = "good"
	case "needs_input":
		statusEmoji = "⚠️"
		statusColor = "warning"
	case "completed":
		statusEmoji = "✅"
		statusColor = "good"
	case "error":
		statusEmoji = "❌"
		statusColor = "attention"
	}

	// Build card body
	body := []map[string]interface{}{
		// Header
		{
			"type":   "TextBlock",
			"size":   "Medium",
			"weight": "Bolder",
			"text":   fmt.Sprintf("%s %s", statusEmoji, update.SessionName),
		},
	}

	// Turn summary
	if update.TurnSummary != "" {
		body = append(body, map[string]interface{}{
			"type": "TextBlock",
			"text": fmt.Sprintf("**Turn %d:** %s", update.TurnNumber, update.TurnSummary),
			"wrap": true,
		})
	}

	// Screenshot image (if provided)
	if update.ScreenshotURL != "" {
		body = append(body, map[string]interface{}{
			"type":    "Image",
			"url":     update.ScreenshotURL,
			"altText": fmt.Sprintf("Agent screenshot - Turn %d", update.TurnNumber),
			"size":    "Large",
		})
	}

	// Input prompt (if needs input)
	if update.NeedsInput && update.InputPrompt != "" {
		body = append(body, map[string]interface{}{
			"type":   "TextBlock",
			"text":   fmt.Sprintf("🤔 **Need input:** %s", update.InputPrompt),
			"wrap":   true,
			"color":  "warning",
			"weight": "Bolder",
		})
	}

	// Action buttons
	sessionURL := fmt.Sprintf("%s/session/%s", update.AppURL, update.SessionID)
	actions := []map[string]interface{}{
		{
			"type":  "Action.OpenUrl",
			"title": "View Session",
			"url":   sessionURL,
		},
	}

	card := map[string]interface{}{
		"$schema": "http://adaptivecards.io/schemas/adaptive-card.json",
		"type":    "AdaptiveCard",
		"version": "1.4",
		"body":    body,
		"actions": actions,
		"msteams": map[string]interface{}{
			"width": "Full",
		},
	}

	// Add color accent based on status
	if statusColor != "default" {
		card["msTeams"] = map[string]interface{}{
			"entities": []map[string]interface{}{
				{
					"type":   "mention",
					"status": statusColor,
				},
			},
		}
	}

	return card
}

// PostSessionSummary posts a summary of the session to the Teams conversation
func (t *TeamsBot) PostSessionSummary(ctx context.Context, conversationID string, serviceURL string, sessionID string, sessionName string, summaries []TurnSummary, appURL string) error {
	// Get access token
	token, err := t.getAccessToken(ctx)
	if err != nil {
		return fmt.Errorf("failed to get access token: %w", err)
	}

	// Build summary text
	var summaryItems []map[string]interface{}
	for _, summary := range summaries {
		summaryItems = append(summaryItems, map[string]interface{}{
			"type": "TextBlock",
			"text": fmt.Sprintf("**%d.** %s", summary.TurnNumber, summary.Summary),
			"wrap": true,
		})
	}

	sessionURL := fmt.Sprintf("%s/session/%s", appURL, sessionID)

	card := map[string]interface{}{
		"$schema": "http://adaptivecards.io/schemas/adaptive-card.json",
		"type":    "AdaptiveCard",
		"version": "1.4",
		"body": append([]map[string]interface{}{
			{
				"type":   "TextBlock",
				"size":   "Medium",
				"weight": "Bolder",
				"text":   fmt.Sprintf("📋 Session Summary: %s", sessionName),
			},
		}, summaryItems...),
		"actions": []map[string]interface{}{
			{
				"type":  "Action.OpenUrl",
				"title": "View Full Session",
				"url":   sessionURL,
			},
		},
	}

	activity := map[string]interface{}{
		"type": "message",
		"attachments": []map[string]interface{}{
			{
				"contentType": "application/vnd.microsoft.card.adaptive",
				"content":     card,
			},
		},
	}

	sendURL := fmt.Sprintf("%sv3/conversations/%s/activities",
		serviceURL,
		url.PathEscape(conversationID))

	activityJSON, err := json.Marshal(activity)
	if err != nil {
		return fmt.Errorf("failed to marshal activity: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", sendURL, bytes.NewReader(activityJSON))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send activity: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("send failed with status %d: %s", resp.StatusCode, string(body))
	}

	return nil
}
