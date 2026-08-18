package anthropic

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// claudeProfileURL returns the account that a token authenticates as. This is
// the same endpoint Claude Code itself calls (see the cc CLI's
// "oauth_profile_fetch" path): it works with both OAuth access tokens and
// setup tokens (sk-ant-oat-...) and needs no anthropic-beta header.
// It is a var (not const) so tests can point it at an httptest server.
var claudeProfileURL = "https://api.anthropic.com/api/oauth/profile"

// ClaudeProfile is the subset of Anthropic's /api/oauth/profile response that
// Helix persists on the subscription row.
type ClaudeProfile struct {
	// AccountEmail is the email of the Claude account the token belongs to —
	// distinct from the Helix user/org that connected it.
	AccountEmail       string
	AccountDisplayName string
	// Plan is the mapped organization plan ("pro", "max", "team",
	// "enterprise"); empty when the organization type is unknown.
	Plan string
	// RateLimitTier is the org's rate-limit tier (e.g. "20x").
	RateLimitTier string
}

// organization type -> plan label, mirroring the cc CLI's mapping.
var organizationTypeToPlan = map[string]string{
	"claude_max":       "max",
	"claude_pro":       "pro",
	"claude_enterprise": "enterprise",
	"claude_team":      "team",
}

type claudeProfileResponse struct {
	Account struct {
		UUID        string `json:"uuid"`
		Email       string `json:"email"`
		DisplayName string `json:"display_name"`
	} `json:"account"`
	Organization struct {
		UUID             string `json:"uuid"`
		OrganizationType string `json:"organization_type"`
		RateLimitTier    string `json:"rate_limit_tier"`
	} `json:"organization"`
}

// FetchClaudeProfile fetches the account/organization profile for a Claude
// OAuth access token or setup token. It errors on any non-200 response,
// network failure, or unparseable body — callers treat that as "identity
// unknown", never as an invalid subscription.
func FetchClaudeProfile(ctx context.Context, token string) (*ClaudeProfile, error) {
	if token == "" {
		return nil, fmt.Errorf("no token")
	}

	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, claudeProfileURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to build profile request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("network error fetching Claude profile: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, fmt.Errorf("failed to read profile response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status %d fetching Claude profile", resp.StatusCode)
	}

	var parsed claudeProfileResponse
	if err := json.Unmarshal(body, &parsed); err != nil {
		return nil, fmt.Errorf("failed to parse Claude profile: %w", err)
	}

	profile := &ClaudeProfile{
		AccountEmail:       parsed.Account.Email,
		AccountDisplayName: parsed.Account.DisplayName,
		RateLimitTier:      parsed.Organization.RateLimitTier,
	}
	if plan, ok := organizationTypeToPlan[parsed.Organization.OrganizationType]; ok {
		profile.Plan = plan
	}
	return profile, nil
}
