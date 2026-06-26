package slack

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// AuthorizeBaseURL is the Slack OAuth v2 authorize endpoint the "Add to
// Slack" button sends the admin to.
const AuthorizeBaseURL = "https://slack.com/oauth/v2/authorize"

// DefaultTokenURL is Slack's OAuth v2 token-exchange endpoint. The
// exchange uses a plain HTTP call (not slack-go, whose endpoint is
// hardcoded) so tests can redirect it to a fake server.
const DefaultTokenURL = "https://slack.com/api/oauth.v2.access"

// AuthorizeURL builds the Slack OAuth v2 authorize URL the admin is sent
// to when installing the app into a workspace. state carries whatever
// the caller needs to correlate the callback (e.g. an encrypted org id).
func AuthorizeURL(clientID, redirectURI string, scopes []string, state string) string {
	q := url.Values{}
	q.Set("client_id", clientID)
	q.Set("scope", strings.Join(scopes, ","))
	q.Set("redirect_uri", redirectURI)
	q.Set("state", state)
	return AuthorizeBaseURL + "?" + q.Encode()
}

// Install is the result of a successful OAuth code exchange — the
// coordinates of one workspace install.
type Install struct {
	BotToken  string
	TeamID    string
	TeamName  string
	BotUserID string
	AppID     string
}

// oauthV2Response is the subset of Slack's oauth.v2.access response we
// persist.
type oauthV2Response struct {
	OK          bool   `json:"ok"`
	Error       string `json:"error"`
	AccessToken string `json:"access_token"`
	BotUserID   string `json:"bot_user_id"`
	AppID       string `json:"app_id"`
	Team        struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	} `json:"team"`
}

// CodeExchanger exchanges an OAuth code for a workspace install. tokenURL
// empty uses DefaultTokenURL; client nil uses a 10s default.
type CodeExchanger struct {
	TokenURL string
	Client   *http.Client
}

// ExchangeCode swaps the authorize-callback code for a bot token + team
// id against the app's client credentials.
func (e CodeExchanger) ExchangeCode(ctx context.Context, clientID, clientSecret, code, redirectURI string) (Install, error) {
	tokenURL := e.TokenURL
	if tokenURL == "" {
		tokenURL = DefaultTokenURL
	}
	client := e.Client
	if client == nil {
		client = &http.Client{Timeout: 10 * time.Second}
	}

	form := url.Values{}
	form.Set("client_id", clientID)
	form.Set("client_secret", clientSecret)
	form.Set("code", code)
	form.Set("redirect_uri", redirectURI)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, tokenURL, strings.NewReader(form.Encode()))
	if err != nil {
		return Install{}, fmt.Errorf("slack oauth: build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := client.Do(req)
	if err != nil {
		return Install{}, fmt.Errorf("slack oauth: exchange: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	body, _ := io.ReadAll(resp.Body)

	var parsed oauthV2Response
	if err := json.Unmarshal(body, &parsed); err != nil {
		return Install{}, fmt.Errorf("slack oauth: decode response: %w", err)
	}
	if !parsed.OK {
		return Install{}, fmt.Errorf("slack oauth: exchange failed: %s", parsed.Error)
	}
	if parsed.AccessToken == "" || parsed.Team.ID == "" {
		return Install{}, fmt.Errorf("slack oauth: response missing access_token or team id")
	}
	return Install{
		BotToken:  parsed.AccessToken,
		TeamID:    parsed.Team.ID,
		TeamName:  parsed.Team.Name,
		BotUserID: parsed.BotUserID,
		AppID:     parsed.AppID,
	}, nil
}
