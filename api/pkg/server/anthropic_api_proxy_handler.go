package server

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"github.com/helixml/helix/api/pkg/anthropic"
	oai "github.com/helixml/helix/api/pkg/openai"
	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/system"
	"github.com/helixml/helix/api/pkg/types"

	"github.com/rs/zerolog/log"
)

func (s *HelixAPIServer) anthropicAPIProxyHandler(w http.ResponseWriter, r *http.Request) {
	user := getRequestUser(r)
	if user == nil {
		http.Error(w, "user is required", http.StatusUnauthorized)
		return
	}

	// Claude Code allows setting headers: https://docs.anthropic.com/en/docs/claude-code/settings
	provider := r.Header.Get("X-Provider")
	orgID := r.Header.Get("X-Org-ID")

	endpoint, err := s.getProviderEndpoint(r.Context(), user, orgID, provider)
	if err != nil {
		log.Error().Err(err).Msg("failed to get provider endpoint")

		http.Error(w, "Failed to get provider endpoint: "+err.Error(), http.StatusInternalServerError)
		return
	}

	if endpoint.OwnerType == types.OwnerTypeOrg {
		orgID = endpoint.Owner
	} else {
		orgID = ""
	}

	ctx := oai.SetContextValues(r.Context(), &oai.ContextValues{
		OwnerID:       user.ID,
		SessionID:     "n/a",
		InteractionID: "n/a",
	})

	ctx = oai.SetContextOrganizationID(ctx, orgID)

	r = r.WithContext(ctx)

	hasEnoughBalance, err := s.Controller.HasEnoughBalance(ctx, getRequestUser(r), orgID, endpoint.BillingEnabled)
	if err != nil {
		log.Error().Err(err).Msg("failed to check balance")
		http.Error(w, "Failed to check balance: "+err.Error(), http.StatusInternalServerError)
		return
	}

	if !hasEnoughBalance {
		http.Error(w, "Insufficient balance", http.StatusPaymentRequired)
		return
	}

	r = anthropic.SetRequestProviderEndpoint(r, endpoint)

	s.anthropicProxy.ServeHTTP(w, r)
}

func (s *HelixAPIServer) getProviderEndpoint(ctx context.Context, user *types.User, orgID, provider string) (*types.ProviderEndpoint, error) {
	if provider == "" {
		return nil, fmt.Errorf("provider ID is required")
	}

	if orgID != "" {
		_, err := s.authorizeOrgMember(ctx, user, orgID)
		if err != nil {
			return nil, fmt.Errorf("error authorizing org member: %w", err)
		}

		q := &store.GetProviderEndpointsQuery{
			Owner: orgID,
		}

		if strings.HasPrefix(provider, system.ProviderEndpointPrefix) {
			q.ID = provider
		} else {
			q.Name = provider
		}

		return s.Store.GetProviderEndpoint(ctx, q)
	}

	q := &store.GetProviderEndpointsQuery{
		Owner: orgID,
	}

	if strings.HasPrefix(provider, system.ProviderEndpointPrefix) {
		q.ID = provider
	} else {
		q.Name = provider
	}

	return s.Store.GetProviderEndpoint(ctx, q)
}
