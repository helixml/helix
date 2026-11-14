package server

import (
	"context"
	"strings"
	"time"

	"github.com/rs/zerolog/log"

	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/system"
	"github.com/helixml/helix/api/pkg/types"
)

func (s *HelixAPIServer) latestProviderCallForInteraction(ctx context.Context, interactionID string) (*types.LLMCall, error) {
	calls, _, err := s.Store.ListLLMCalls(ctx, &store.ListLLMCallsQuery{
		InteractionID: interactionID,
		Page:          1,
		PerPage:       1,
	})
	if err != nil {
		return nil, err
	}
	if len(calls) == 0 || calls[0] == nil {
		return nil, nil
	}
	return calls[0], nil
}

func (s *HelixAPIServer) providerFailureForInteraction(ctx context.Context, interaction *types.Interaction) (*types.LLMCall, error) {
	if interaction == nil || interaction.CodeAgentConfigSnapshot == nil {
		return nil, nil
	}
	call, err := s.latestProviderCallForInteraction(ctx, interaction.ID)
	if err != nil || call != nil {
		if call == nil || strings.TrimSpace(call.Error) == "" || !providerFailureMatchesSnapshot(call, interaction.CodeAgentConfigSnapshot) {
			return nil, err
		}
		return call, err
	}

	calls, _, err := s.Store.ListLLMCalls(ctx, &store.ListLLMCallsQuery{
		SessionID: interaction.SessionID,
		Page:      1,
		PerPage:   10,
	})
	if err != nil {
		return nil, err
	}
	ended := time.Now()
	if interaction.State != types.InteractionStateWaiting && !interaction.Updated.IsZero() {
		ended = interaction.Updated
	}
	var latest *types.LLMCall
	var latestEnded time.Time
	for _, call := range calls {
		callEnded := providerCallEndedAt(call)
		if call == nil ||
			(call.InteractionID != "" && call.InteractionID != "n/a") ||
			(!interaction.Created.IsZero() && call.Created.Before(interaction.Created)) ||
			call.Created.After(ended) || callEnded.Before(ended.Add(-providerFailureLookback)) || callEnded.After(ended) ||
			!providerFailureMatchesSnapshot(call, interaction.CodeAgentConfigSnapshot) {
			continue
		}
		if latest == nil || callEnded.After(latestEnded) {
			latest = call
			latestEnded = callEnded
		}
	}
	if latest == nil || strings.TrimSpace(latest.Error) == "" {
		return nil, nil
	}
	return latest, nil
}

func providerCallEndedAt(call *types.LLMCall) time.Time {
	if call == nil {
		return time.Time{}
	}
	if call.DurationMs > 0 && !call.Created.IsZero() {
		return call.Created.Add(time.Duration(call.DurationMs) * time.Millisecond)
	}
	if !call.Updated.IsZero() {
		return call.Updated
	}
	return call.Created
}

func isExplicitProviderFailure(errorMsg string) bool {
	if !strings.Contains(errorMsg, "agent turn aborted") {
		return false
	}
	if system.Has5xxStatusCode(errorMsg) {
		return true
	}
	return strings.Contains(errorMsg, `"errorName"`) && strings.Contains(errorMsg, `"APIError"`) &&
		(strings.Contains(errorMsg, "dial tcp") || strings.Contains(errorMsg, "i/o timeout") ||
			strings.Contains(errorMsg, "context deadline exceeded") || strings.Contains(errorMsg, "connection reset"))
}

func providerFailureMatchesSnapshot(failure *types.LLMCall, snapshot *types.InteractionCodeAgentConfigSnapshot) bool {
	if failure == nil || snapshot == nil {
		return false
	}
	sameProvider := failure.Provider == snapshot.Provider ||
		(types.IsGlobalProviderID(snapshot.Provider) && types.IsGlobalProvider(failure.Provider) &&
			types.CanonicalProviderName(snapshot.Provider) == failure.Provider) ||
		(types.IsGlobalProviderID(failure.Provider) && types.IsGlobalProvider(snapshot.Provider) &&
			types.CanonicalProviderName(failure.Provider) == snapshot.Provider)
	if !sameProvider {
		return false
	}
	return strings.TrimPrefix(failure.Model, failure.Provider+"/") == strings.TrimPrefix(snapshot.Model, snapshot.Provider+"/")
}

func (s *HelixAPIServer) clearProviderRecoveryAfterCompletedTurn(ctx context.Context, session *types.Session, interaction *types.Interaction) (bool, error) {
	if session == nil || interaction == nil || session.Metadata.Recovery == nil ||
		session.Metadata.Recovery.PromptID != interaction.PromptID ||
		interaction.State != types.InteractionStateComplete || strings.TrimSpace(interaction.Error) != "" {
		return false, nil
	}
	latest, err := s.Store.GetLatestInteractionByPromptID(ctx, interaction.PromptID)
	if err != nil {
		return false, err
	}
	if latest == nil || latest.ID != interaction.ID {
		return false, nil
	}
	recovery := session.Metadata.Recovery
	return s.Store.ClearSessionProviderRecovery(ctx, session.ID, recovery.PromptID, recovery.InteractionID)
}

func (s *HelixAPIServer) recoverProviderFailedActivationAfterConfigChange(ctx context.Context, session *types.Session) (bool, error) {
	if session == nil || session.Metadata.OrgWorkerID == "" || !session.Metadata.AutoRestartOnCrash {
		return false, nil
	}
	entry, err := s.Store.GetLatestActivationPromptHistoryEntry(ctx, session.ID)
	if err != nil {
		return false, err
	}
	recovered := false
	if entry != nil && entry.DeletedAt == nil && entry.Status == "sent" {
		interaction, err := s.Store.GetLatestInteractionByPromptID(ctx, entry.ID)
		if err != nil {
			return false, err
		}
		if interaction != nil && interaction.State == types.InteractionStateError {
			state := session.Metadata.Recovery
			providerFailure := isExplicitProviderFailure(interaction.Error) ||
				(state != nil && state.PromptID == entry.ID && state.InteractionID == interaction.ID)
			if !providerFailure {
				failure, err := s.providerFailureForInteraction(ctx, interaction)
				if err != nil {
					return false, err
				}
				providerFailure = failure != nil
			}
			if providerFailure {
				recovered, err = s.Store.RecoverFailedPrompt(ctx, session.ID, entry.ID, interaction.ID)
				if err != nil {
					return false, err
				}
			}
		}
	}
	if session.Metadata.Recovery != nil {
		state := session.Metadata.Recovery
		cleared, err := s.Store.ClearSessionProviderRecovery(ctx, session.ID, state.PromptID, state.InteractionID)
		if err != nil {
			return false, err
		}
		if cleared {
			session.Metadata.Recovery = nil
		}
	}
	return recovered, nil
}

func (s *HelixAPIServer) recoverProviderFailedWorkers(ctx context.Context, endpoint *types.ProviderEndpoint, models ProviderModels) {
	providers := []string{endpoint.Name}
	if endpoint.ID != "" && endpoint.ID != endpoint.Name {
		providers = append(providers, endpoint.ID)
	} else if endpoint.ID == "" && endpoint.Owner == string(types.OwnerTypeSystem) && types.IsGlobalProvider(endpoint.Name) {
		providers = append(providers, types.GlobalProviderID(endpoint.Name))
	}
	sessions, _, err := s.Store.ListSessions(ctx, store.ListSessionsQuery{
		AnyOwner:                  true,
		AnyOrganization:           true,
		IncludeExternalAgents:     true,
		ProviderRecoveryProviders: providers,
	})
	if err != nil {
		log.Warn().Err(err).Str("provider", endpoint.Name).Msg("Failed to list provider recovery sessions")
		return
	}
	now := time.Now()
	for _, session := range sessions {
		recovered, err := s.recoverProviderFailedSession(ctx, session, models.Models, models.FetchedAt, now)
		if err != nil {
			log.Warn().Err(err).Str("session_id", session.ID).Msg("Failed to recover provider prompt")
			continue
		}
		if recovered {
			state := session.Metadata.Recovery
			log.Info().Str("session_id", session.ID).Str("provider", state.Provider).Str("model", state.Model).
				Msg("Provider recovered; requeued autonomous bot prompt")
			s.nudgeSessionQueue(session.ID)
		}
	}
}

func (s *HelixAPIServer) recoverProviderFailedSession(ctx context.Context, session *types.Session, models []types.OpenAIModel, observedAt, now time.Time) (bool, error) {
	state := session.Metadata.Recovery
	if state == nil || session.Metadata.OrgWorkerID == "" || !session.Metadata.AutoRestartOnCrash ||
		state.Attempts >= maxProviderRecoveryAttempts || state.NextCheckAt.After(now) || !observedAt.After(state.FailedAt) ||
		!providerModelAvailable(state.Model, state.Provider, models) {
		return false, nil
	}
	return s.Store.RecoverProviderPrompt(ctx, session.ID, state.PromptID, now, now.Add(providerRecoveryDelay(state.Attempts+1)), maxProviderRecoveryAttempts)
}

func providerModelAvailable(model, provider string, models []types.OpenAIModel) bool {
	model = strings.TrimPrefix(model, provider+"/")
	for _, available := range models {
		if available.ID == model {
			return true
		}
	}
	return false
}
