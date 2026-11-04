package server

import (
	"context"
	"net/http"
	"time"

	"github.com/helixml/helix/api/pkg/outbound"
	"github.com/rs/zerolog/log"
)

const outboundRequestTimeout = 60 * time.Second

// outboundPolicy returns the destination policy for server-side requests to
// caller-supplied URLs. Config is validated at startup in NewServer.
func (s *HelixAPIServer) outboundPolicy() (*outbound.Policy, error) {
	return outbound.NewPolicy(s.Cfg.Outbound.AllowedPrivateCIDRs)
}

// rejectDisallowedOutboundURL validates rawURL against policy and, if it is
// rejected, writes a fixed 400 and returns true. The response never says why
// (unresolvable vs private vs bad scheme) so it cannot be used to probe the
// internal network; the reason is logged server-side.
func rejectDisallowedOutboundURL(ctx context.Context, w http.ResponseWriter, policy *outbound.Policy, field, rawURL string) bool {
	if err := policy.ValidateURL(ctx, rawURL); err != nil {
		log.Warn().Err(err).Str("field", field).Msg("rejected outbound URL")
		http.Error(w, "The "+field+" is not an allowed destination. Private network addresses must be allowed by the Helix administrator.", http.StatusBadRequest)
		return true
	}
	return false
}
