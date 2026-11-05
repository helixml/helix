package server

import (
	"context"
	"net/http"
	"time"

	"github.com/helixml/helix/api/pkg/netguard"
	"github.com/rs/zerolog/log"
)

const gitProviderRequestTimeout = 60 * time.Second

// gitProviderURLPolicy is the destination policy for caller-supplied git
// host URLs (GitHub Enterprise / GitLab base_url, ADO organization_url).
func (s *HelixAPIServer) gitProviderURLPolicy() netguard.Policy {
	return netguard.Policy{AllowPrivate: s.Cfg.GitProviders.AllowPrivateNetworks}
}

// gitProviderHTTPClient returns a netguard-guarded client when the caller
// supplied a self-hosted baseURL, and nil (library default, i.e. github.com /
// gitlab.com) otherwise.
func gitProviderHTTPClient(policy netguard.Policy, baseURL string) *http.Client {
	if baseURL == "" {
		return nil
	}
	return policy.NewClient(gitProviderRequestTimeout)
}

// rejectDisallowedOutboundURL validates rawURL against policy and, if it is
// rejected, writes a fixed 400 and returns true. The response never says why
// (private vs reserved vs bad scheme) so it cannot be used to probe the
// internal network; the reason is logged server-side.
func rejectDisallowedOutboundURL(ctx context.Context, w http.ResponseWriter, policy netguard.Policy, field, rawURL string) bool {
	if err := policy.ValidateURL(ctx, rawURL); err != nil {
		log.Warn().Err(err).Str("field", field).Msg("rejected outbound URL")
		http.Error(w, "The "+field+" is not an allowed destination. Private network addresses must be allowed by the Helix administrator.", http.StatusBadRequest)
		return true
	}
	return false
}
