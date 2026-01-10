package server

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gorilla/mux"
	"github.com/helixml/helix/api/pkg/system"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/rs/zerolog/log"
)

// SessionTOCEntry represents a single entry in the session table of contents
type SessionTOCEntry struct {
	Turn      int       `json:"turn"`       // 1-indexed turn number
	ID        string    `json:"id"`         // Interaction ID
	Summary   string    `json:"summary"`    // One-line summary
	Created   time.Time `json:"created"`    // When this turn happened
	HasPrompt bool      `json:"has_prompt"` // Whether there's a user prompt
	HasResponse bool    `json:"has_response"` // Whether there's an assistant response
}

// SessionTOCResponse is the response for the session TOC endpoint
type SessionTOCResponse struct {
	SessionID   string            `json:"session_id"`
	SessionName string            `json:"session_name"`
	TotalTurns  int               `json:"total_turns"`
	Entries     []SessionTOCEntry `json:"entries"`
	// Formatted is a pre-formatted numbered list for easy consumption
	Formatted string `json:"formatted"`
}

// InteractionWithContext includes surrounding context
type InteractionWithContext struct {
	Turn        int                `json:"turn"`
	Interaction *types.Interaction `json:"interaction"`
	Previous    *InteractionBrief  `json:"previous,omitempty"`
	Next        *InteractionBrief  `json:"next,omitempty"`
}

// InteractionBrief is a brief summary of an interaction for context
type InteractionBrief struct {
	Turn    int    `json:"turn"`
	ID      string `json:"id"`
	Summary string `json:"summary"`
}

// @Summary Get session table of contents
// @Description Returns a numbered list of interaction summaries for a session
// @Tags Sessions
// @Accept json
// @Produce json
// @Param id path string true "Session ID"
// @Success 200 {object} SessionTOCResponse
// @Failure 401 {object} system.HTTPError
// @Failure 404 {object} system.HTTPError
// @Failure 500 {object} system.HTTPError
// @Security ApiKeyAuth
// @Router /api/v1/sessions/{id}/toc [get]
func (apiServer *HelixAPIServer) getSessionTOC(_ http.ResponseWriter, req *http.Request) (*SessionTOCResponse, *system.HTTPError) {
	ctx := req.Context()
	user := getRequestUser(req)
	if user == nil {
		return nil, system.NewHTTPError401("user not found")
	}

	sessionID := mux.Vars(req)["id"]
	if sessionID == "" {
		return nil, system.NewHTTPError400("session ID is required")
	}

	// Get session
	session, err := apiServer.Store.GetSession(ctx, sessionID)
	if err != nil {
		return nil, system.NewHTTPError500(fmt.Sprintf("failed to get session: %v", err))
	}
	if session == nil {
		return nil, system.NewHTTPError404("session not found")
	}

	// Check authorization
	if session.Owner != user.ID && user.ID != "runner-system" {
		return nil, system.NewHTTPError403("you don't have access to this session")
	}

	// Get all interactions for this session
	interactions, _, err := apiServer.Store.ListInteractions(ctx, &types.ListInteractionsQuery{
		SessionID:    sessionID,
		GenerationID: session.GenerationID,
		PerPage:      500, // Reasonable limit
	})
	if err != nil {
		return nil, system.NewHTTPError500(fmt.Sprintf("failed to list interactions: %v", err))
	}

	// Build TOC entries
	entries := make([]SessionTOCEntry, 0, len(interactions))
	var formattedLines []string

	for i, interaction := range interactions {
		turn := i + 1 // 1-indexed

		// Generate summary if not present
		summary := interaction.Summary
		if summary == "" {
			summary = generateInteractionSummary(interaction)
			// TODO: Save generated summary back to DB asynchronously
		}

		entry := SessionTOCEntry{
			Turn:        turn,
			ID:          interaction.ID,
			Summary:     summary,
			Created:     interaction.Created,
			HasPrompt:   interaction.PromptMessage != "",
			HasResponse: interaction.ResponseMessage != "",
		}
		entries = append(entries, entry)

		// Build formatted line
		formattedLines = append(formattedLines, fmt.Sprintf("%d. %s", turn, summary))
	}

	return &SessionTOCResponse{
		SessionID:   sessionID,
		SessionName: session.Name,
		TotalTurns:  len(entries),
		Entries:     entries,
		Formatted:   strings.Join(formattedLines, "\n"),
	}, nil
}

// @Summary Get interaction by turn number
// @Description Returns a specific interaction with surrounding context
// @Tags Sessions
// @Accept json
// @Produce json
// @Param id path string true "Session ID"
// @Param turn path int true "Turn number (1-indexed)"
// @Success 200 {object} InteractionWithContext
// @Failure 400 {object} system.HTTPError
// @Failure 401 {object} system.HTTPError
// @Failure 404 {object} system.HTTPError
// @Failure 500 {object} system.HTTPError
// @Security ApiKeyAuth
// @Router /api/v1/sessions/{id}/turns/{turn} [get]
func (apiServer *HelixAPIServer) getInteractionByTurn(_ http.ResponseWriter, req *http.Request) (*InteractionWithContext, *system.HTTPError) {
	ctx := req.Context()
	user := getRequestUser(req)
	if user == nil {
		return nil, system.NewHTTPError401("user not found")
	}

	sessionID := mux.Vars(req)["id"]
	turnStr := mux.Vars(req)["turn"]

	if sessionID == "" {
		return nil, system.NewHTTPError400("session ID is required")
	}

	turn, err := strconv.Atoi(turnStr)
	if err != nil || turn < 1 {
		return nil, system.NewHTTPError400("turn must be a positive integer")
	}

	// Get session
	session, err := apiServer.Store.GetSession(ctx, sessionID)
	if err != nil {
		return nil, system.NewHTTPError500(fmt.Sprintf("failed to get session: %v", err))
	}
	if session == nil {
		return nil, system.NewHTTPError404("session not found")
	}

	// Check authorization
	if session.Owner != user.ID && user.ID != "runner-system" {
		return nil, system.NewHTTPError403("you don't have access to this session")
	}

	// Get all interactions for this session
	interactions, _, err := apiServer.Store.ListInteractions(ctx, &types.ListInteractionsQuery{
		SessionID:    sessionID,
		GenerationID: session.GenerationID,
		PerPage:      500,
	})
	if err != nil {
		return nil, system.NewHTTPError500(fmt.Sprintf("failed to list interactions: %v", err))
	}

	// Find the requested turn (1-indexed)
	if turn > len(interactions) {
		return nil, system.NewHTTPError404(fmt.Sprintf("turn %d not found (session has %d turns)", turn, len(interactions)))
	}

	idx := turn - 1
	interaction := interactions[idx]

	// Build response with context
	result := &InteractionWithContext{
		Turn:        turn,
		Interaction: interaction,
	}

	// Add previous turn summary if exists
	if idx > 0 {
		prev := interactions[idx-1]
		summary := prev.Summary
		if summary == "" {
			summary = generateInteractionSummary(prev)
		}
		result.Previous = &InteractionBrief{
			Turn:    turn - 1,
			ID:      prev.ID,
			Summary: summary,
		}
	}

	// Add next turn summary if exists
	if idx < len(interactions)-1 {
		next := interactions[idx+1]
		summary := next.Summary
		if summary == "" {
			summary = generateInteractionSummary(next)
		}
		result.Next = &InteractionBrief{
			Turn:    turn + 1,
			ID:      next.ID,
			Summary: summary,
		}
	}

	return result, nil
}

// @Summary Search session interactions
// @Description Search for interactions within a session by content
// @Tags Sessions
// @Accept json
// @Produce json
// @Param id path string true "Session ID"
// @Param q query string true "Search query"
// @Param limit query int false "Max results (default 10)"
// @Success 200 {object} SessionTOCResponse
// @Failure 400 {object} system.HTTPError
// @Failure 401 {object} system.HTTPError
// @Failure 404 {object} system.HTTPError
// @Failure 500 {object} system.HTTPError
// @Security ApiKeyAuth
// @Router /api/v1/sessions/{id}/search [get]
func (apiServer *HelixAPIServer) searchSessionInteractions(_ http.ResponseWriter, req *http.Request) (*SessionTOCResponse, *system.HTTPError) {
	ctx := req.Context()
	user := getRequestUser(req)
	if user == nil {
		return nil, system.NewHTTPError401("user not found")
	}

	sessionID := mux.Vars(req)["id"]
	query := req.URL.Query().Get("q")

	if sessionID == "" {
		return nil, system.NewHTTPError400("session ID is required")
	}
	if query == "" {
		return nil, system.NewHTTPError400("search query 'q' is required")
	}

	limit := 10
	if limitStr := req.URL.Query().Get("limit"); limitStr != "" {
		if l, err := strconv.Atoi(limitStr); err == nil && l > 0 {
			limit = l
		}
	}

	// Get session
	session, err := apiServer.Store.GetSession(ctx, sessionID)
	if err != nil {
		return nil, system.NewHTTPError500(fmt.Sprintf("failed to get session: %v", err))
	}
	if session == nil {
		return nil, system.NewHTTPError404("session not found")
	}

	// Check authorization
	if session.Owner != user.ID && user.ID != "runner-system" {
		return nil, system.NewHTTPError403("you don't have access to this session")
	}

	// Get all interactions and filter by search query
	// TODO: Use postgres full-text search for better performance
	interactions, _, err := apiServer.Store.ListInteractions(ctx, &types.ListInteractionsQuery{
		SessionID:    sessionID,
		GenerationID: session.GenerationID,
		PerPage:      500,
	})
	if err != nil {
		return nil, system.NewHTTPError500(fmt.Sprintf("failed to list interactions: %v", err))
	}

	// Simple case-insensitive search
	queryLower := strings.ToLower(query)
	entries := make([]SessionTOCEntry, 0)
	var formattedLines []string

	for i, interaction := range interactions {
		turn := i + 1

		// Search in prompt, response, and summary
		matches := strings.Contains(strings.ToLower(interaction.PromptMessage), queryLower) ||
			strings.Contains(strings.ToLower(interaction.ResponseMessage), queryLower) ||
			strings.Contains(strings.ToLower(interaction.Summary), queryLower)

		if !matches {
			continue
		}

		summary := interaction.Summary
		if summary == "" {
			summary = generateInteractionSummary(interaction)
		}

		entry := SessionTOCEntry{
			Turn:        turn,
			ID:          interaction.ID,
			Summary:     summary,
			Created:     interaction.Created,
			HasPrompt:   interaction.PromptMessage != "",
			HasResponse: interaction.ResponseMessage != "",
		}
		entries = append(entries, entry)
		formattedLines = append(formattedLines, fmt.Sprintf("%d. %s", turn, summary))

		if len(entries) >= limit {
			break
		}
	}

	return &SessionTOCResponse{
		SessionID:   sessionID,
		SessionName: session.Name,
		TotalTurns:  len(interactions),
		Entries:     entries,
		Formatted:   strings.Join(formattedLines, "\n"),
	}, nil
}

// generateInteractionSummary creates a one-line summary from an interaction
// This is a simple extractive summary - can be upgraded to LLM-generated later
func generateInteractionSummary(interaction *types.Interaction) string {
	// If there's already a summary, use it
	if interaction.Summary != "" {
		return interaction.Summary
	}

	// Try to extract a meaningful summary
	var summary string

	// First, try to use the prompt as the basis (what was asked)
	if interaction.PromptMessage != "" {
		summary = extractFirstLine(interaction.PromptMessage, 100)
	}

	// If prompt is too short or empty, use response
	if len(summary) < 20 && interaction.ResponseMessage != "" {
		respSummary := extractFirstLine(interaction.ResponseMessage, 100)
		if len(respSummary) > len(summary) {
			summary = respSummary
		}
	}

	if summary == "" {
		summary = "(empty interaction)"
	}

	return summary
}

// extractFirstLine gets the first meaningful line, truncated to maxLen
func extractFirstLine(text string, maxLen int) string {
	// Remove leading whitespace and find first non-empty line
	lines := strings.Split(strings.TrimSpace(text), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		// Skip markdown headers for cleaner summary
		if strings.HasPrefix(line, "#") {
			line = strings.TrimLeft(line, "# ")
		}
		// Truncate if needed
		if len(line) > maxLen {
			// Try to break at word boundary
			if idx := strings.LastIndex(line[:maxLen], " "); idx > maxLen/2 {
				return line[:idx] + "..."
			}
			return line[:maxLen-3] + "..."
		}
		return line
	}
	return ""
}

// saveSummaryAsync saves a generated summary back to the database
func (apiServer *HelixAPIServer) saveSummaryAsync(ctx context.Context, interactionID string, summary string) {
	go func() {
		if err := apiServer.Store.UpdateInteractionSummary(ctx, interactionID, summary); err != nil {
			log.Error().Err(err).Str("interaction_id", interactionID).Msg("Failed to save interaction summary")
		}
	}()
}
