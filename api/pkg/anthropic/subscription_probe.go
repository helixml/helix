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

// ProbeClaudeSubscription performs a cheap liveness probe of a Claude
// subscription OAuth/setup token against Anthropic's messages API.
//
//	401         -> ProbeInvalid  (token is bad/expired/revoked)
//	200 or 429  -> ProbeValid    (429 is just a throttle, the token still works)
//	otherwise   -> ProbeInconclusive (network/5xx — don't punish the user)
//
// The returned detail string is a short human-readable reason (empty when valid).
func ProbeClaudeSubscription(ctx context.Context, token string) (ProbeResult, string) {
	if token == "" {
		return ProbeInvalid, "no token"
	}

	body := []byte(`{"model":"claude-3-5-haiku-latest","max_tokens":1,"messages":[{"role":"user","content":"ping"}]}`)

	ctx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, subscriptionProbeURL, bytes.NewReader(body))
	if err != nil {
		return ProbeInconclusive, "failed to build probe request"
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("anthropic-beta", oauthBetaHeader)
	req.Header.Set("anthropic-version", "2023-06-01")
	req.Header.Set("content-type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return ProbeInconclusive, "network error probing Anthropic: " + err.Error()
	}
	defer resp.Body.Close()
	// Drain a little so the connection can be reused; ignore errors.
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))

	switch {
	case resp.StatusCode == http.StatusUnauthorized:
		return ProbeInvalid, "invalid or expired token (401 from Anthropic)"
	case resp.StatusCode == http.StatusOK || resp.StatusCode == http.StatusTooManyRequests:
		return ProbeValid, ""
	default:
		return ProbeInconclusive, fmt.Sprintf("unexpected status %d from Anthropic", resp.StatusCode)
	}
}

// ValidateSubscription decrypts the stored credentials for a Claude subscription,
// selects the bearer token, probes it, and returns the outcome. It does NOT
// persist anything — callers decide whether to write Status/LastError/
// LastValidatedAt back via the store.
//
// For setup_token credentials a 401 is definitive. For oauth credentials whose
// access token has already expired we return ProbeInconclusive rather than
// probing: Claude Code refreshes those in-container, so a raw probe of the stale
// access token would 401 and falsely read as invalid.
func ValidateSubscription(ctx context.Context, sub *types.ClaudeSubscription) (ProbeResult, string) {
	if sub == nil {
		return ProbeInconclusive, "no subscription"
	}

	encKey, err := crypto.GetEncryptionKey()
	if err != nil {
		return ProbeInconclusive, "encryption key unavailable"
	}
	plaintext, err := crypto.DecryptAES256GCM(sub.EncryptedCredentials, encKey)
	if err != nil {
		return ProbeInconclusive, "failed to decrypt credentials"
	}

	credType := sub.CredentialType
	if credType == "" {
		credType = "oauth"
	}

	switch credType {
	case "setup_token":
		var tok types.ClaudeSetupTokenCredentials
		if err := json.Unmarshal(plaintext, &tok); err != nil || tok.SetupToken == "" {
			return ProbeInvalid, "malformed setup token credentials"
		}
		return ProbeClaudeSubscription(ctx, tok.SetupToken)
	case "oauth":
		var creds types.ClaudeOAuthCredentials
		if err := json.Unmarshal(plaintext, &creds); err != nil || creds.AccessToken == "" {
			return ProbeInvalid, "malformed oauth credentials"
		}
		// If the access token is expired but a refresh token exists, the
		// in-container Claude Code will refresh it — don't mark it invalid.
		if creds.ExpiresAt > 0 && time.UnixMilli(creds.ExpiresAt).Before(time.Now()) && creds.RefreshToken != "" {
			return ProbeInconclusive, "access token expired (refreshable in-container)"
		}
		return ProbeClaudeSubscription(ctx, creds.AccessToken)
	default:
		return ProbeInconclusive, "unknown credential type"
	}
}
