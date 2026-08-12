package webservice

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/rs/zerolog/log"
)

// DomainVerifier polls vhost_routes rows that are unverified
// (verified_at IS NULL AND verification_token != ''), and for each one
// makes an HTTP request to
//   http://<hostname>/.well-known/helix-domain-verify/<token>
// If the response body contains the token, the row is marked verified.
// Designed to be run on a ticker from the API process.
type DomainVerifier struct {
	store    store.Store
	client   *http.Client
	interval time.Duration
}

// NewDomainVerifier wires a verifier with a sensible HTTP client.
func NewDomainVerifier(s store.Store) *DomainVerifier {
	return &DomainVerifier{
		store: s,
		client: &http.Client{
			Timeout: 10 * time.Second,
			// Don't follow redirects — operator-misconfigured CDNs
			// can redirect us into infinite loops or to a third
			// party who happens to echo our token.
			CheckRedirect: func(req *http.Request, via []*http.Request) error {
				return http.ErrUseLastResponse
			},
		},
		interval: 60 * time.Second,
	}
}

// Start runs the verifier on a ticker until ctx is canceled. Designed
// to be launched as a goroutine from server bootstrap; the loop is
// idempotent and tolerates store errors.
func (v *DomainVerifier) Start(ctx context.Context) {
	ticker := time.NewTicker(v.interval)
	defer ticker.Stop()
	for {
		// Run once at startup, then on each tick.
		v.runOnce(ctx)
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

// runOnce sweeps pending rows once. Public for tests.
func (v *DomainVerifier) runOnce(ctx context.Context) {
	pending, err := v.store.ListPendingVHostRoutes(ctx, 100)
	if err != nil {
		log.Warn().Err(err).Msg("domain verifier: list pending failed")
		return
	}
	for _, row := range pending {
		if row.VerificationToken == "" {
			continue
		}
		if v.checkRow(ctx, row) {
			if err := v.store.MarkVHostRouteVerified(ctx, row.ID); err != nil {
				log.Warn().Err(err).Str("hostname", row.Hostname).Msg("domain verifier: mark verified failed")
				continue
			}
			log.Info().
				Str("hostname", row.Hostname).
				Str("project_id", row.TargetID).
				Msg("domain verifier: hostname verified")
		}
	}
}

// checkRow makes one HTTP request and returns true if the response body
// contains the expected verification token.
func (v *DomainVerifier) checkRow(ctx context.Context, row *types.VHostRoute) bool {
	url := fmt.Sprintf("http://%s/.well-known/helix-domain-verify/%s", row.Hostname, row.VerificationToken)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return false
	}
	resp, err := v.client.Do(req)
	if err != nil {
		// Common: DNS not yet pointing at us, or operator's
		// reverse proxy not yet configured to forward this host.
		return false
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return false
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1024))
	if err != nil {
		return false
	}
	return strings.TrimSpace(string(body)) == row.VerificationToken
}
