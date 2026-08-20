package anthropic

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// Claude Code's OAuth endpoints and public client id, read from the CLI bundle
// (v2.1.229) and confirmed against a live `claude auth login --claudeai`.
// They are vars (not consts) so tests can point them at an httptest server.
var (
	claudeAuthorizeURL = "https://claude.com/cai/oauth/authorize"
	claudeTokenURL     = "https://platform.claude.com/v1/oauth/token"
)

const (
	// claudeOAuthClientID is the public client every Claude Code install uses.
	claudeOAuthClientID = "9d1c250a-e61b-44d9-88ed-5944d1962f5e"

	// claudeManualRedirectURI is an Anthropic-hosted page that displays the
	// authorization code for the user to copy. Using it is what makes this flow
	// work for a server that cannot receive a browser redirect — there is no
	// localhost listener and no callback into Helix.
	claudeManualRedirectURI = "https://platform.claude.com/oauth/code/callback"

	// claudeOAuthScopes must include user:profile — that scope is the whole
	// reason this route can name the account, and the reason setup tokens
	// cannot (see design/2026-08-18-claude-profile-identity.md).
	claudeOAuthScopes = "org:create_api_key user:profile user:inference user:sessions:claude_code user:mcp_servers user:file_upload"
)

// ClaudeLoginChallenge is the PKCE material for one login attempt. The verifier
// is handed to the browser that started the login and posted back with the code:
// Helix is not the OAuth client here, the user's session is, so holding it
// server-side would buy no security while breaking multi-replica deployments.
type ClaudeLoginChallenge struct {
	AuthorizeURL string `json:"authorize_url"`
	CodeVerifier string `json:"code_verifier"`
	State        string `json:"state"`
}

func randomURLSafe(byteLen int) (string, error) {
	buf := make([]byte, byteLen)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("failed to generate random bytes: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}

// StartClaudeLogin builds the authorize URL the user visits, plus the PKCE
// verifier and state that must come back with the code.
func StartClaudeLogin() (*ClaudeLoginChallenge, error) {
	verifier, err := randomURLSafe(32)
	if err != nil {
		return nil, err
	}
	state, err := randomURLSafe(32)
	if err != nil {
		return nil, err
	}
	sum := sha256.Sum256([]byte(verifier))
	challenge := base64.RawURLEncoding.EncodeToString(sum[:])

	query := url.Values{}
	query.Set("code", "true")
	query.Set("client_id", claudeOAuthClientID)
	query.Set("response_type", "code")
	query.Set("redirect_uri", claudeManualRedirectURI)
	query.Set("scope", claudeOAuthScopes)
	query.Set("code_challenge", challenge)
	query.Set("code_challenge_method", "S256")
	query.Set("state", state)

	return &ClaudeLoginChallenge{
		AuthorizeURL: claudeAuthorizeURL + "?" + query.Encode(),
		CodeVerifier: verifier,
		State:        state,
	}, nil
}

// SplitPastedCode separates the value the user copies from Anthropic's callback
// page. That page hands back "<code>#<state>", so accept either form rather than
// making the user understand the difference.
func SplitPastedCode(pasted string) (code string, state string) {
	trimmed := strings.TrimSpace(pasted)
	if idx := strings.Index(trimmed, "#"); idx >= 0 {
		return trimmed[:idx], trimmed[idx+1:]
	}
	return trimmed, ""
}

type claudeTokenResponse struct {
	AccessToken           string `json:"access_token"`
	RefreshToken          string `json:"refresh_token"`
	ExpiresIn             int64  `json:"expires_in"`
	RefreshTokenExpiresIn int64  `json:"refresh_token_expires_in"`
	Scope                 string `json:"scope"`
	TokenType             string `json:"token_type"`
	Error                 string `json:"error"`
	ErrorDescription      string `json:"error_description"`
}

// ClaudeTokens is the credential set this flow produces — the same shape the
// Claude Code CLI writes to ~/.claude/.credentials.json, so it stores and
// refreshes exactly like a credential the user pasted from their own machine.
type ClaudeTokens struct {
	AccessToken  string
	RefreshToken string
	// ExpiresAt is Unix milliseconds, matching ClaudeOAuthCredentials.
	ExpiresAt int64
	Scopes    []string
}

// ExchangeClaudeCode trades the pasted authorization code for tokens. The
// verifier proves this is the same browser session that started the login.
func ExchangeClaudeCode(ctx context.Context, code, verifier, state string) (*ClaudeTokens, error) {
	if code == "" {
		return nil, fmt.Errorf("no authorization code")
	}
	if verifier == "" {
		return nil, fmt.Errorf("no code verifier — start the login again")
	}

	body, err := json.Marshal(map[string]string{
		"grant_type":    "authorization_code",
		"code":          code,
		"redirect_uri":  claudeManualRedirectURI,
		"client_id":     claudeOAuthClientID,
		"code_verifier": verifier,
		"state":         state,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to build token request: %w", err)
	}

	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, claudeTokenURL, strings.NewReader(string(body)))
	if err != nil {
		return nil, fmt.Errorf("failed to build token request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("network error exchanging Claude authorization code: %w", err)
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, fmt.Errorf("failed to read token response: %w", err)
	}

	var parsed claudeTokenResponse
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return nil, fmt.Errorf("failed to parse token response (status %d)", resp.StatusCode)
	}
	if resp.StatusCode != http.StatusOK {
		detail := parsed.ErrorDescription
		if detail == "" {
			detail = parsed.Error
		}
		if detail == "" {
			detail = fmt.Sprintf("status %d", resp.StatusCode)
		}
		// The overwhelmingly common cause is a stale or half-copied code, so say
		// that rather than surfacing a bare OAuth error string.
		return nil, fmt.Errorf("Anthropic rejected the authorization code (%s) — codes expire quickly, try connecting again", detail)
	}
	if parsed.AccessToken == "" || parsed.RefreshToken == "" {
		return nil, fmt.Errorf("Anthropic returned an incomplete token response")
	}

	tokens := &ClaudeTokens{
		AccessToken:  parsed.AccessToken,
		RefreshToken: parsed.RefreshToken,
	}
	if parsed.ExpiresIn > 0 {
		tokens.ExpiresAt = time.Now().Add(time.Duration(parsed.ExpiresIn) * time.Second).UnixMilli()
	}
	if parsed.Scope != "" {
		tokens.Scopes = strings.Fields(parsed.Scope)
	}
	return tokens, nil
}
