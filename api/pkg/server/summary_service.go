package server

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/helixml/helix/api/pkg/openai/manager"
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

	// Rate limiting to avoid overwhelming the LLM provider
	mu            sync.Mutex
	pendingCount  int
	maxConcurrent int
}

// NewSummaryService creates a new SummaryService
func NewSummaryService(store store.Store, providerManager manager.ProviderManager) *SummaryService {
	return &SummaryService{
		store:           store,
		providerManager: providerManager,
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
	if interaction.PromptMessage == "" && interaction.ResponseMessage == "" {
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
	}

	return nil
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

	if interaction.ResponseMessage != "" {
		// Truncate long responses
		response := interaction.ResponseMessage
		if len(response) > 500 {
			response = response[:500] + "..."
		}
		parts = append(parts, "Assistant responded: "+response)
	}

	return strings.Join(parts, "\n\n")
}
