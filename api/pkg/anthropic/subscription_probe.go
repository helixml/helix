package anthropic

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/helixml/helix/api/pkg/crypto"
	"github.com/helixml/helix/api/pkg/types"
)

// subscriptionProbeURL is the Anthropic messages endpoint used for liveness
// probes. It is a var (not const) so tests can point it at an httptest server.
var subscriptionProbeURL = "https://api.anthropic.com/v1/messages"

// oauthBetaHeader is mandatory when authenticating /v1/messages with a Claude
// subscription OAuth / setup token. Without it Anthropic rejects the token with
// "This credential is only authorized for use with Claude Code".
// See design/2026-02-14-claude-subscription-provider.md.
const oauthBetaHeader = "oauth-2025-04-20"

// ProbeResult classifies the outcome of a liveness probe.
type ProbeResult int

const (
	// ProbeValid means the token authenticated (HTTP 200 or 429 throttle).
	ProbeValid ProbeResult = iota
	// ProbeInvalid means the token was rejected (HTTP 401 authentication_failed).
	ProbeInvalid
	// ProbeInconclusive means the probe could not determine validity (network
	// error, 5xx, or a credential that cannot be probed without a refresh).
	ProbeInconclusive
)

// organizationIDHeader is returned by Anthropic on every /v1/messages response
// and identifies the Claude organization the token belongs to. It needs no
// OAuth scope, which makes it the one *verified* identity signal available for
// setup tokens — those cannot be profiled at all (/api/oauth/profile requires
// any_of(user:profile, user:office) and setup tokens carry inference scopes
// only). See design/2026-08-18-claude-profile-identity.md.
const organizationIDHeader = "anthropic-organization-id"

// Probe is the outcome of a liveness probe plus whatever Anthropic disclosed
// about the credential while answering it.
type Probe struct {
	Result ProbeResult
	// Detail is a short human-readable reason, empty when valid.
	Detail string
	// Token is the bearer the probe authenticated with, empty when no probe
	// ran. Callers reuse it for follow-up requests such as the
	// /api/oauth/profile identity fetch.
	Token string
	// OrganizationID is Anthropic's organization uuid for the credential, read
	// from the response header. Empty when the probe never reached Anthropic.
	OrganizationID string
}

// ProbeClaudeSubscription performs a cheap liveness probe of a Claude
// subscription OAuth/setup token against Anthropic's messages API.
//
//	401         -> ProbeInvalid  (token is bad/expired/revoked)
//	200 or 429  -> ProbeValid    (429 is just a throttle, the token still works)
//	otherwise   -> ProbeInconclusive (network/5xx — don't punish the user)
//
// The returned detail string is a short human-readable reason (empty when valid).
//
// The probe model must be a CURRENT id: Anthropic 404s (not 401s) deprecated
// model ids on /v1/messages, which reads as "inconclusive" and silently stops
// subscriptions from ever validating. claude-3-5-haiku-latest (used here until
// Aug 2026) hit exactly that — every probe 404'd and statuses were only ever
// the default "active". See design/2026-08-18-claude-profile-identity.md.
func ProbeClaudeSubscription(ctx context.Context, token string) Probe {
	if token == "" {
		return Probe{Result: ProbeInvalid, Detail: "no token"}
	}

	body := []byte(`{"model":"claude-haiku-4-5","max_tokens":1,"messages":[{"role":"user","content":"ping"}]}`)

	ctx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, subscriptionProbeURL, bytes.NewReader(body))
	if err != nil {
		return Probe{Result: ProbeInconclusive, Detail: "failed to build probe request", Token: token}
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("anthropic-beta", oauthBetaHeader)
	req.Header.Set("anthropic-version", "2023-06-01")
	req.Header.Set("content-type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return Probe{Result: ProbeInconclusive, Detail: "network error probing Anthropic: " + err.Error(), Token: token}
	}
	defer resp.Body.Close()
	// Read a little so the connection can be reused; the preview keeps
	// unexpected statuses (like a 404 for a retired probe model) diagnosable.
	preview, _ := io.ReadAll(io.LimitReader(resp.Body, 512))

	probe := Probe{Token: token, OrganizationID: resp.Header.Get(organizationIDHeader)}
	switch {
	case resp.StatusCode == http.StatusUnauthorized:
		probe.Result, probe.Detail = ProbeInvalid, "invalid or expired token (401 from Anthropic)"
	case resp.StatusCode == http.StatusOK || resp.StatusCode == http.StatusTooManyRequests:
		probe.Result = ProbeValid
	default:
		probe.Result = ProbeInconclusive
		probe.Detail = fmt.Sprintf("unexpected status %d from Anthropic: %s", resp.StatusCode, string(preview))
	}
	return probe
}

// staleRefreshGrace bounds how long an expired OAuth access token can plausibly
// be "about to be refreshed in-container".
//
// A healthy Claude Code refreshes its access token and pushes the new credentials
// back (PUT /sessions/{id}/claude-credentials), which moves ExpiresAt forward. So
// an access token that expired minutes ago is unremarkable, but one that expired
// days ago proves nothing is refreshing it — the stored refresh token is dead or
// revoked. Treating that as "inconclusive" left Status pinned at its last value,
// so a long-dead subscription kept reporting "active" while every desktop failed
// with "OAuth session expired and could not be refreshed".
const staleRefreshGrace = time.Hour

// ValidateSubscription decrypts the stored credentials for a Claude subscription,
// selects the bearer token, probes it, and returns the outcome. It does NOT
// persist anything — callers decide whether to write Status/LastError/
// LastValidatedAt back via the store.
//
// For setup_token credentials a 401 is definitive. For oauth credentials whose
// access token expired only recently we return ProbeInconclusive rather than
// probing: Claude Code refreshes those in-container, so a raw probe of the stale
// access token would 401 and falsely read as invalid. Past staleRefreshGrace that
// benefit of the doubt is withdrawn — see above.
func ValidateSubscription(ctx context.Context, sub *types.ClaudeSubscription) Probe {
	if sub == nil {
		return Probe{Result: ProbeInconclusive, Detail: "no subscription"}
	}

	encKey, err := crypto.GetEncryptionKey()
	if err != nil {
		return Probe{Result: ProbeInconclusive, Detail: "encryption key unavailable"}
	}
	plaintext, err := crypto.DecryptAES256GCM(sub.EncryptedCredentials, encKey)
	if err != nil {
		return Probe{Result: ProbeInconclusive, Detail: "failed to decrypt credentials"}
	}

	credType := sub.CredentialType
	if credType == "" {
		credType = "oauth"
	}

	switch credType {
	case "setup_token":
		var tok types.ClaudeSetupTokenCredentials
		if err := json.Unmarshal(plaintext, &tok); err != nil || tok.SetupToken == "" {
			return Probe{Result: ProbeInvalid, Detail: "malformed setup token credentials"}
		}
		return ProbeClaudeSubscription(ctx, tok.SetupToken)
	case "oauth":
		var creds types.ClaudeOAuthCredentials
		if err := json.Unmarshal(plaintext, &creds); err != nil || creds.AccessToken == "" {
			return Probe{Result: ProbeInvalid, Detail: "malformed oauth credentials"}
		}
		// If the access token is expired but a refresh token exists, the
		// in-container Claude Code will refresh it — don't mark it invalid.
		if creds.ExpiresAt > 0 && time.UnixMilli(creds.ExpiresAt).Before(time.Now()) && creds.RefreshToken != "" {
			if expiredFor := time.Since(time.UnixMilli(creds.ExpiresAt)); expiredFor > staleRefreshGrace {
				return Probe{Result: ProbeInvalid, Detail: fmt.Sprintf(
					"access token expired %s ago and has not been refreshed — re-authenticate",
					expiredFor.Round(time.Hour))}
			}
			return Probe{Result: ProbeInconclusive, Detail: "access token expired (refreshable in-container)"}
		}
		return ProbeClaudeSubscription(ctx, creds.AccessToken)
	default:
		return Probe{Result: ProbeInconclusive, Detail: "unknown credential type"}
	}
}
