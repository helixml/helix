package webservice

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/helixml/helix/api/pkg/netguard"
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

// NewDomainVerifier wires a verifier whose HTTP client applies policy to
// every address a custom hostname resolves to, so a hostname pointed at
// loopback, an internal service or cloud metadata is never fetched.
// Redirects are not followed: operator-misconfigured CDNs can redirect us
// into infinite loops or to a third party who happens to echo our token.
func NewDomainVerifier(s store.Store, policy netguard.Policy) *DomainVerifier {
	return &DomainVerifier{
		store:    s,
		client:   policy.NewClient(10 * time.Second),
		interval: 60 * time.Second,
	}
}

// ErrInvalidHostname is returned by ValidateHostname for a value that is not
// a plain DNS name.
var ErrInvalidHostname = errors.New("invalid hostname")

// ValidateHostname checks that host is a plain, fully-qualified DNS name: no
// scheme, port, path, credentials or IP literal, and not a local-only name.
// The verifier builds a URL from it, so anything else could change which
// server is contacted.
func ValidateHostname(host string) error {
	if len(host) == 0 || len(host) > 253 {
		return fmt.Errorf("%w: length must be 1-253 characters", ErrInvalidHostname)
	}
	if net.ParseIP(host) != nil {
		return fmt.Errorf("%w: IP addresses are not allowed", ErrInvalidHostname)
	}
	labels := strings.Split(host, ".")
	if len(labels) < 2 {
		return fmt.Errorf("%w: must be a fully-qualified domain name", ErrInvalidHostname)
	}
	for _, label := range labels {
		if len(label) == 0 || len(label) > 63 {
			return fmt.Errorf("%w: each label must be 1-63 characters", ErrInvalidHostname)
		}
		if label[0] == '-' || label[len(label)-1] == '-' {
			return fmt.Errorf("%w: labels may not start or end with a hyphen", ErrInvalidHostname)
		}
		for _, c := range label {
			if (c < 'a' || c > 'z') && (c < '0' || c > '9') && c != '-' {
				return fmt.Errorf("%w: only lowercase letters, digits, hyphens and dots are allowed", ErrInvalidHostname)
			}
		}
	}
	switch tld := labels[len(labels)-1]; tld {
	case "localhost", "local", "internal", "localdomain":
		return fmt.Errorf("%w: %q names are not publicly resolvable", ErrInvalidHostname, tld)
	}
	if strings.Trim(labels[len(labels)-1], "0123456789") == "" {
		return fmt.Errorf("%w: top-level label may not be numeric", ErrInvalidHostname)
	}
	return nil
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
	if err := ValidateHostname(row.Hostname); err != nil {
		return false
	}
	url := fmt.Sprintf("http://%s/.well-known/helix-domain-verify/%s", row.Hostname, row.VerificationToken)
	return v.fetchToken(ctx, url, row.VerificationToken)
}

// fetchToken GETs url through the policy-guarded client and reports whether
// the body is exactly token.
func (v *DomainVerifier) fetchToken(ctx context.Context, url, token string) bool {
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
	return strings.TrimSpace(string(body)) == token
}
