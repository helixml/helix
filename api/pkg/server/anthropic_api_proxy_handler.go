package server

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/helixml/helix/api/pkg/anthropic"
	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/system"
	"github.com/helixml/helix/api/pkg/types"

	"github.com/rs/zerolog/log"
)

func (s *HelixAPIServer) anthropicAPIProxyHandler(w http.ResponseWriter, r *http.Request) {
	endpoint, err := s.getProviderEndpoint(r)
	if err != nil {
		log.Error().Err(err).Msg("failed to get provider endpoint")

		http.Error(w, "Failed to get provider endpoint: "+err.Error(), http.StatusInternalServerError)
		return
	}
	r = anthropic.SetRequestProviderEndpoint(r, endpoint)

	s.anthropicProxy.ServeHTTP(w, r)
}

func (s *HelixAPIServer) getProviderEndpoint(r *http.Request) (*types.ProviderEndpoint, error) {
	user := getRequestUser(r)
	if user == nil {
		return nil, fmt.Errorf("user is required")
	}

	provider := r.Header.Get("X-Provider")
	orgID := r.Header.Get("X-Org-ID")

	if provider == "" {
		return nil, fmt.Errorf("provider ID is required")
	}

	if orgID != "" {
		_, err := s.authorizeOrgMember(r.Context(), user, orgID)
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

		return s.Store.GetProviderEndpoint(r.Context(), q)
	}

	q := &store.GetProviderEndpointsQuery{
		Owner: orgID,
	}

	if strings.HasPrefix(provider, system.ProviderEndpointPrefix) {
		q.ID = provider
	} else {
		q.Name = provider
	}

	return s.Store.GetProviderEndpoint(r.Context(), q)
}
