package server

import (
	"context"
	"strings"
	"time"

	"github.com/rs/zerolog/log"

	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/types"
)

func (s *HelixAPIServer) recoverProviderFailedWorkers(ctx context.Context, endpoint *types.ProviderEndpoint, models ProviderModels) {
	providers := []string{endpoint.Name}
	if endpoint.ID != "" && endpoint.ID != endpoint.Name {
		providers = append(providers, endpoint.ID)
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
