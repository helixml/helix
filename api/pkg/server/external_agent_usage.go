package server

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	external_agent "github.com/helixml/helix/api/pkg/external-agent"
	openailogger "github.com/helixml/helix/api/pkg/openai/logger"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/rs/zerolog/log"
)

type acpTurnUsage struct {
	TotalTokens       int `json:"total_tokens"`
	InputTokens       int `json:"input_tokens"`
	OutputTokens      int `json:"output_tokens"`
	CachedReadTokens  int `json:"cached_read_tokens"`
	CachedWriteTokens int `json:"cached_write_tokens"`
}

func (s *HelixAPIServer) recordACPUsage(
	ctx context.Context,
	session *types.Session,
	interaction *types.Interaction,
	syncMsg *types.SyncMessage,
) error {
	var usage acpTurnUsage
	usageKnown := false
	rawUsage := syncMsg.Data["usage"]
	agentName, _ := syncMsg.Data["agent_name"].(string)
	if rawUsage != nil {
		encoded, err := json.Marshal(rawUsage)
		if err != nil {
			return fmt.Errorf("failed to encode ACP usage: %w", err)
		}
		if err := json.Unmarshal(encoded, &usage); err != nil {
			return fmt.Errorf("failed to decode ACP usage: %w", err)
		}
		usageKnown = true
	}

	if session.ParentApp == "" {
		return nil
	}
	app, err := s.Controller.Options.Store.GetApp(ctx, session.ParentApp)
	if err != nil {
		return fmt.Errorf("failed to get app %s for ACP usage: %w", session.ParentApp, err)
	}
	assistant := external_agent.FindZedExternalAssistant(app)
	if assistant == nil || !assistant.CodeAgentCredentialType.IsSubscription() {
		return nil
	}

	provider, model := acpUsageProviderAndModel(assistant)
	if !usageKnown {
		// The turn still gets a metric row (request counts stay accurate), but
		// token totals will read zero. The usual cause is a Zed build predating
		// the `usage` field on message_completed — see sandbox-versions.txt.
		log.Warn().
			Str("session_id", session.ID).
			Str("interaction_id", interaction.ID).
			Str("agent_name", agentName).
			Str("code_agent_runtime", string(assistant.CodeAgentRuntime)).
			Msg("ACP turn completed without usage data; recording metric with usage_known=false")
	}
	durationMs := interaction.DurationMs
	if durationMs == 0 && !interaction.Created.IsZero() && !interaction.Completed.IsZero() {
		durationMs = int(interaction.Completed.Sub(interaction.Created).Milliseconds())
	}

	_, err = openailogger.NewUsageLogger(s.Controller.Options.Store).CreateUsageMetric(ctx, &types.UsageMetric{
		OrganizationID:   session.OrganizationID,
		AppID:            session.ParentApp,
		UserID:           session.Owner,
		InteractionID:    interaction.ID,
		ProjectID:        session.ProjectID,
		SpecTaskID:       session.Metadata.SpecTaskID,
		Provider:         provider,
		Model:            model,
		Source:           types.UsageMetricSourceACP,
		SourceID:         session.ID + ":" + interaction.ID,
		UsageKnown:       usageKnown,
		PromptTokens:     usage.InputTokens,
		CompletionTokens: usage.OutputTokens,
		TotalTokens:      usage.TotalTokens,
		CacheReadTokens:  usage.CachedReadTokens,
		CacheWriteTokens: usage.CachedWriteTokens,
		DurationMs:       durationMs,
	})
	if err != nil {
		return fmt.Errorf("failed to record ACP usage: %w", err)
	}
	return nil
}

func acpUsageProviderAndModel(assistant *types.AssistantConfig) (string, string) {
	provider := assistant.Provider
	model := assistant.Model

	switch assistant.CodeAgentRuntime {
	case types.CodeAgentRuntimeClaudeCode:
		provider = "anthropic"
		if assistant.ClaudeSubscriptionModel != "" {
			model = assistant.ClaudeSubscriptionModel
		}
		if model == "" {
			model = "claude-subscription"
		}
	case types.CodeAgentRuntimeCodexCLI:
		provider = "openai"
		if model == "" {
			model = "codex-subscription"
		}
	default:
		if provider == "" {
			provider = strings.TrimSuffix(string(assistant.CodeAgentRuntime), "_cli")
		}
	}

	return provider, model
}
