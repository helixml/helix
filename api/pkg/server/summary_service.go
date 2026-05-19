package server

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/helixml/helix/api/pkg/openai/manager"
	"github.com/helixml/helix/api/pkg/pubsub"
	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/rs/zerolog/log"
	gopenai "github.com/sashabaranov/go-openai"
)

// SummaryService generates interaction summaries and session titles using LLM
// It uses the kodit enrichment model configured in system settings
type SummaryService struct {
	store           store.Store
	providerManager manager.ProviderManager
	pubsub          pubsub.PubSub

	// Rate limiting to avoid overwhelming the LLM provider
	mu            sync.Mutex
	pendingCount  int
	maxConcurrent int
}

// NewSummaryService creates a new SummaryService
func NewSummaryService(store store.Store, providerManager manager.ProviderManager, ps pubsub.PubSub) *SummaryService {
	return &SummaryService{
		store:           store,
		providerManager: providerManager,
		pubsub:          ps,
		maxConcurrent:   5, // Max concurrent summary requests
	}
}

// GenerateInteractionSummaryAsync generates a one-line summary for an interaction asynchronously
// This is called when an interaction completes (ResponseMessage is set)
func (s *SummaryService) GenerateInteractionSummaryAsync(ctx context.Context, interaction *types.Interaction, ownerID string) {
	// Skip if interaction already has a summary
	if interaction.Summary != "" {
		return
	}

	// Skip if no content to summarize
	if interaction.PromptMessage == "" && types.TextFromInteraction(interaction) == "" {
		return
	}

	// Rate limiting check
	s.mu.Lock()
	if s.pendingCount >= s.maxConcurrent {
		s.mu.Unlock()
		log.Debug().
			Str("interaction_id", interaction.ID).
			Msg("Skipping summary generation - too many pending requests")
		return
	}
	s.pendingCount++
	s.mu.Unlock()

	go func() {
		defer func() {
			s.mu.Lock()
			s.pendingCount--
			s.mu.Unlock()
		}()

		// Use a fresh context with timeout (not tied to the request context)
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		summary, err := s.generateInteractionSummary(ctx, interaction, ownerID)
		if err != nil {
			log.Warn().
				Err(err).
				Str("interaction_id", interaction.ID).
				Msg("Failed to generate interaction summary")
			return
		}

		// Save the summary
		if err := s.store.UpdateInteractionSummary(ctx, interaction.ID, summary); err != nil {
			log.Error().
				Err(err).
				Str("interaction_id", interaction.ID).
				Msg("Failed to save interaction summary")
		} else {
			log.Debug().
				Str("interaction_id", interaction.ID).
				Str("summary", summary).
				Msg("Generated and saved interaction summary")
		}
	}()
}

// generateInteractionSummary generates a one-line summary using the kodit model
func (s *SummaryService) generateInteractionSummary(ctx context.Context, interaction *types.Interaction, ownerID string) (string, error) {
	// Get the kodit model configuration from system settings
	settings, err := s.store.GetEffectiveSystemSettings(ctx)
	if err != nil {
		return "", fmt.Errorf("failed to get system settings: %w", err)
	}

	// Fall back to extractive summary if kodit model not configured
	if settings.KoditEnrichmentProvider == "" || settings.KoditEnrichmentModel == "" {
		return generateInteractionSummary(interaction), nil
	}

	// Get the provider client
	client, err := s.providerManager.GetClient(ctx, &manager.GetClientRequest{
		Provider: settings.KoditEnrichmentProvider,
		Owner:    ownerID,
	})
	if err != nil {
		log.Warn().
			Err(err).
			Str("provider", settings.KoditEnrichmentProvider).
			Msg("Failed to get provider client for summary, using extractive fallback")
		return generateInteractionSummary(interaction), nil
	}

	// Build the prompt for summarization
	content := buildSummaryPromptContent(interaction)
	if content == "" {
		return generateInteractionSummary(interaction), nil
	}

	// Make the LLM call
	// The summary will be used in:
	// 1. Session TOC (numbered list for navigation)
	// 2. Title history (tracking topic evolution)
	// 3. Search results (finding relevant past interactions)
	// 4. Session tab hover preview
	resp, err := client.CreateChatCompletion(ctx, gopenai.ChatCompletionRequest{
		Model: settings.KoditEnrichmentModel,
		Messages: []gopenai.ChatCompletionMessage{
			{
				Role: "system",
				Content: `Generate a one-line summary (max 100 chars) for a conversation turn.
This summary will be used for:
- Table of contents navigation (finding past discussions)
- Tracking how the session topic evolved
- Search results when looking for past work
- Hover previews in session tabs

Guidelines:
- Start with an action verb when possible (e.g., "Implemented", "Fixed", "Discussed", "Asked about")
- Include specific technical terms, function names, or file names if mentioned
- Focus on WHAT was done/discussed, not how
- Be searchable - include keywords someone might search for later
- No quotes, no ending punctuation`,
			},
			{
				Role:    "user",
				Content: content,
			},
		},
		MaxTokens:   50, // Keep response very short
		Temperature: 0.3,
	})
	if err != nil {
		return "", fmt.Errorf("LLM call failed: %w", err)
	}

	if len(resp.Choices) == 0 || resp.Choices[0].Message.Content == "" {
		return generateInteractionSummary(interaction), nil
	}

	// Clean up the summary
	summary := strings.TrimSpace(resp.Choices[0].Message.Content)
	// Remove any quotes the model might add
	summary = strings.Trim(summary, `"'`)
	// Truncate if too long
	if len(summary) > 100 {
		if idx := strings.LastIndex(summary[:100], " "); idx > 50 {
			summary = summary[:idx] + "..."
		} else {
			summary = summary[:97] + "..."
		}
	}

	return summary, nil
}

// UpdateSessionTitleAsync updates the session title based on TOC and previous title
// Only updates if the content has substantially changed
func (s *SummaryService) UpdateSessionTitleAsync(ctx context.Context, sessionID string, ownerID string) {
	// Rate limiting check
	s.mu.Lock()
	if s.pendingCount >= s.maxConcurrent {
		s.mu.Unlock()
		log.Debug().
			Str("session_id", sessionID).
			Msg("Skipping session title update - too many pending requests")
		return
	}
	s.pendingCount++
	s.mu.Unlock()

	go func() {
		defer func() {
			s.mu.Lock()
			s.pendingCount--
			s.mu.Unlock()
		}()

		// Use a fresh context with timeout
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		if err := s.updateSessionTitle(ctx, sessionID, ownerID); err != nil {
			log.Warn().
				Err(err).
				Str("session_id", sessionID).
				Msg("Failed to update session title")
		}
	}()
}

// updateSessionTitle generates a new session title based on TOC
func (s *SummaryService) updateSessionTitle(ctx context.Context, sessionID string, ownerID string) error {
	// Get session
	session, err := s.store.GetSession(ctx, sessionID)
	if err != nil {
		return fmt.Errorf("failed to get session: %w", err)
	}

	// Get interactions for TOC
	interactions, _, err := s.store.ListInteractions(ctx, &types.ListInteractionsQuery{
		SessionID:    sessionID,
		GenerationID: session.GenerationID,
		PerPage:      20, // Only use first 20 interactions for title
	})
	if err != nil {
		return fmt.Errorf("failed to list interactions: %w", err)
	}

	if len(interactions) == 0 {
		return nil // Nothing to summarize
	}

	// Build TOC for title generation - show in reverse chronological order
	// with emphasis on recent topics (newest first)
	var tocLines []string
	for i := len(interactions) - 1; i >= 0; i-- {
		interaction := interactions[i]
		summary := interaction.Summary
		if summary == "" {
			summary = generateInteractionSummary(interaction)
		}
		// Show turn number (1-indexed) and mark recent turns
		turnNum := i + 1
		if i >= len(interactions)-3 {
			tocLines = append(tocLines, fmt.Sprintf("[RECENT] Turn %d: %s", turnNum, summary))
		} else {
			tocLines = append(tocLines, fmt.Sprintf("Turn %d: %s", turnNum, summary))
		}
	}
	toc := strings.Join(tocLines, "\n")

	// Also build forward chronological for fallback title
	forwardSummary := ""
	if len(interactions) > 0 {
		forwardSummary = generateInteractionSummary(interactions[len(interactions)-1])
	}

	// Get the kodit model configuration
	settings, err := s.store.GetEffectiveSystemSettings(ctx)
	if err != nil {
		return fmt.Errorf("failed to get system settings: %w", err)
	}

	// Fall back to most recent interaction summary if kodit not configured
	if settings.KoditEnrichmentProvider == "" || settings.KoditEnrichmentModel == "" {
		if session.Name == "" && forwardSummary != "" {
			// Use most recent interaction as title
			newTitle := forwardSummary
			if len(newTitle) > 60 {
				if idx := strings.LastIndex(newTitle[:60], " "); idx > 30 {
					newTitle = newTitle[:idx]
				} else {
					newTitle = newTitle[:57] + "..."
				}
			}
			return s.store.UpdateSessionName(ctx, sessionID, newTitle)
		}
		return nil
	}

	// Get provider client
	client, err := s.providerManager.GetClient(ctx, &manager.GetClientRequest{
		Provider: settings.KoditEnrichmentProvider,
		Owner:    ownerID,
	})
	if err != nil {
		return fmt.Errorf("failed to get provider client: %w", err)
	}

	// Build prompt with previous title context - bias toward NEW topics at end
	var promptContent string
	if session.Name != "" {
		promptContent = fmt.Sprintf(`Current session title: "%s"

Conversation turns (newest first, [RECENT] marks last 3 turns):
%s

Generate a session title (max 60 characters).
- If the [RECENT] turns discuss a NEW topic different from the current title, update the title to reflect the new topic.
- If the conversation is still on the same topic, keep the current title.
- Focus on what the user is CURRENTLY working on (the [RECENT] turns).
Do not include quotes.`, session.Name, toc)
	} else {
		promptContent = fmt.Sprintf(`Conversation turns (newest first, [RECENT] marks last 3 turns):
%s

Generate a session title (max 60 characters) that describes what the user is currently working on. Focus on the [RECENT] turns. Do not include quotes.`, toc)
	}

	// Make the LLM call
	resp, err := client.CreateChatCompletion(ctx, gopenai.ChatCompletionRequest{
		Model: settings.KoditEnrichmentModel,
		Messages: []gopenai.ChatCompletionMessage{
			{
				Role:    "system",
				Content: `You are a title generator. Generate concise, descriptive titles for conversations. Respond with only the title, no quotes or extra text.`,
			},
			{
				Role:    "user",
				Content: promptContent,
			},
		},
		MaxTokens:   30,
		Temperature: 0.3,
	})
	if err != nil {
		return fmt.Errorf("LLM call failed: %w", err)
	}

	if len(resp.Choices) == 0 || resp.Choices[0].Message.Content == "" {
		return nil
	}

	// Clean up the title
	newTitle := strings.TrimSpace(resp.Choices[0].Message.Content)
	newTitle = strings.Trim(newTitle, `"'`)
	if len(newTitle) > 60 {
		if idx := strings.LastIndex(newTitle[:60], " "); idx > 30 {
			newTitle = newTitle[:idx]
		} else {
			newTitle = newTitle[:57] + "..."
		}
	}

	// Only update if title changed
	if newTitle != session.Name && newTitle != "" {
		log.Debug().
			Str("session_id", sessionID).
			Str("old_title", session.Name).
			Str("new_title", newTitle).
			Msg("Updating session title")

		// Update title
		if err := s.store.UpdateSessionName(ctx, sessionID, newTitle); err != nil {
			return err
		}

		// Track title history for hover preview in SpecTask tab view
		// Find the most recent interaction to link to
		var lastInteractionID string
		var turn int
		if len(interactions) > 0 {
			lastInteraction := interactions[len(interactions)-1]
			lastInteractionID = lastInteraction.ID
			turn = len(interactions)
		}

		// Prepend to title history (newest first)
		entry := &types.TitleHistoryEntry{
			Title:         newTitle,
			ChangedAt:     time.Now(),
			Turn:          turn,
			InteractionID: lastInteractionID,
		}

		// Get fresh session to update metadata
		session, err = s.store.GetSession(ctx, sessionID)
		if err != nil {
			log.Warn().Err(err).Str("session_id", sessionID).Msg("Failed to get session for title history update")
			return nil // Title already updated, just skip history
		}

		// Prepend new entry (newest first), keep max 20 entries
		session.Metadata.TitleHistory = append([]*types.TitleHistoryEntry{entry}, session.Metadata.TitleHistory...)
		if len(session.Metadata.TitleHistory) > 20 {
			session.Metadata.TitleHistory = session.Metadata.TitleHistory[:20]
		}

		// Save updated metadata
		if err := s.store.UpdateSessionMetadata(ctx, sessionID, session.Metadata); err != nil {
			log.Warn().Err(err).Str("session_id", sessionID).Msg("Failed to update title history")
		}

		// Publish WebSocket event so clients see the title and metadata update
		s.publishSessionUpdate(ctx, session)
	}

	return nil
}

// publishSessionUpdate sends a WebSocket event for session updates
func (s *SummaryService) publishSessionUpdate(ctx context.Context, session *types.Session) {
	if s.pubsub == nil {
		log.Debug().Msg("PubSub not initialized, skipping session update publishing")
		return
	}

	event := &types.WebsocketEvent{
		Type:      types.WebsocketEventSessionUpdate,
		SessionID: session.ID,
		Owner:     session.Owner,
		Session:   session,
	}

	message, err := json.Marshal(event)
	if err != nil {
		log.Error().Err(err).Str("session_id", session.ID).Msg("Failed to marshal session update event")
		return
	}

	if err := s.pubsub.Publish(ctx, pubsub.GetSessionQueue(session.Owner, session.ID), message); err != nil {
		log.Error().Err(err).Str("session_id", session.ID).Msg("Failed to publish session update event")
	}
}

// GenerateSpecTaskTitleAsync fires off a background LLM call to populate the
// spec task's short_title from its original prompt. Best-effort: errors and
// missing enrichment-model configuration are logged at debug/warn and the
// task is left with an empty short_title (the frontend falls back to name).
//
// Implements services.TitleGenerator so SpecDrivenTaskService can call it
// without importing the server package.
func (s *SummaryService) GenerateSpecTaskTitleAsync(ctx context.Context, taskID, ownerID, prompt string) {
	prompt = strings.TrimSpace(prompt)
	if taskID == "" || prompt == "" {
		return
	}

	s.mu.Lock()
	if s.pendingCount >= s.maxConcurrent {
		s.mu.Unlock()
		log.Debug().Str("task_id", taskID).Msg("Skipping spec task title generation - too many pending requests")
		return
	}
	s.pendingCount++
	s.mu.Unlock()

	go func() {
		defer func() {
			s.mu.Lock()
			s.pendingCount--
			s.mu.Unlock()
		}()

		bgCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		title, err := s.generateSpecTaskTitle(bgCtx, ownerID, prompt)
		if err != nil {
			log.Warn().Err(err).Str("task_id", taskID).Msg("Failed to generate spec task short title")
			return
		}
		if title == "" {
			return
		}

		updated, err := s.store.UpdateSpecTaskShortTitle(bgCtx, taskID, title)
		if err != nil {
			log.Warn().Err(err).Str("task_id", taskID).Msg("Failed to store spec task short title")
			return
		}
		if !updated {
			log.Debug().Str("task_id", taskID).Msg("Spec task short title already set, skipped overwrite")
			return
		}
		log.Debug().Str("task_id", taskID).Str("short_title", title).Msg("Generated spec task short title")
	}()
}

// generateSpecTaskTitle calls the kodit enrichment model and returns a snappy
// title for the given prompt. Returns an empty string with no error when the
// enrichment model is not configured (graceful degradation).
func (s *SummaryService) generateSpecTaskTitle(ctx context.Context, ownerID, prompt string) (string, error) {
	settings, err := s.store.GetEffectiveSystemSettings(ctx)
	if err != nil {
		return "", fmt.Errorf("failed to get system settings: %w", err)
	}
	if settings.KoditEnrichmentProvider == "" || settings.KoditEnrichmentModel == "" {
		return "", nil
	}

	client, err := s.providerManager.GetClient(ctx, &manager.GetClientRequest{
		Provider: settings.KoditEnrichmentProvider,
		Owner:    ownerID,
	})
	if err != nil {
		return "", fmt.Errorf("failed to get provider client: %w", err)
	}

	// Keep the prompt LLM call cheap: a long prompt does not help an LLM
	// pick a snappy title, and risks token-budget surprises.
	if len(prompt) > 2000 {
		prompt = prompt[:2000]
	}

	resp, err := client.CreateChatCompletion(ctx, gopenai.ChatCompletionRequest{
		Model: settings.KoditEnrichmentModel,
		Messages: []gopenai.ChatCompletionMessage{
			{
				Role: "system",
				Content: `Generate a snappy task title (max 60 characters) for a software engineering task. The title appears on a Kanban card and tab strip, so it must be scannable at a glance.

Guidelines:
- Start with an imperative verb (Add, Fix, Refactor, Generate, Wire up, ...).
- Mention the concrete subject (component, file, behaviour).
- No quotes, no trailing punctuation, no "Task:" prefix.
- Do NOT echo the whole prompt — distil it.`,
			},
			{
				Role:    "user",
				Content: prompt,
			},
		},
		MaxTokens:   50,
		Temperature: 0.3,
	})
	if err != nil {
		return "", fmt.Errorf("LLM call failed: %w", err)
	}
	if len(resp.Choices) == 0 {
		return "", nil
	}

	return cleanGeneratedTitle(resp.Choices[0].Message.Content), nil
}

// cleanGeneratedTitle normalises a model's title response: trims whitespace
// and quote characters, drops leading "Task:"-style prefixes, and truncates
// to 60 chars at a word boundary when possible. Returns empty if nothing
// usable survives.
func cleanGeneratedTitle(raw string) string {
	title := strings.TrimSpace(raw)
	title = strings.Trim(title, `"'`)
	// Models sometimes prepend a label like "Title:" or "Task:".
	for _, prefix := range []string{"Title:", "title:", "Task:", "task:"} {
		title = strings.TrimSpace(strings.TrimPrefix(title, prefix))
	}
	title = strings.TrimRight(title, ".!?")
	title = strings.TrimSpace(title)

	if len(title) > 60 {
		if idx := strings.LastIndex(title[:60], " "); idx > 30 {
			title = title[:idx]
		} else {
			title = title[:57] + "..."
		}
	}
	return title
}

// buildSummaryPromptContent builds the content for the summarization prompt
func buildSummaryPromptContent(interaction *types.Interaction) string {
	var parts []string

	if interaction.PromptMessage != "" {
		// Truncate long prompts
		prompt := interaction.PromptMessage
		if len(prompt) > 500 {
			prompt = prompt[:500] + "..."
		}
		parts = append(parts, "User asked: "+prompt)
	}

	if responseText := types.TextFromInteraction(interaction); responseText != "" {
		// Truncate long responses
		if len(responseText) > 500 {
			responseText = responseText[:500] + "..."
		}
		parts = append(parts, "Assistant responded: "+responseText)
	}

	return strings.Join(parts, "\n\n")
}
