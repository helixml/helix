package slack

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/helixml/helix/api/pkg/org/application/configregistry"
)

// authorizeURL is the Slack OAuth v2 authorize endpoint the "Add to
// Slack" button sends the admin to.
const authorizeURL = "https://slack.com/oauth/v2/authorize"

// defaultTokenURL is Slack's OAuth v2 token-exchange endpoint. The
// exchange is done with a plain HTTP call (not slack-go, whose endpoint
// is hardcoded) so tests can redirect it to a fake server.
const defaultTokenURL = "https://slack.com/api/oauth.v2.access"

// StateEncoder / StateDecoder carry the org id through the OAuth round
// trip in the (encrypted) state param. Production wires
// crypto.EncryptAES256GCM / Decrypt; the org id is the only payload.
type (
	StateEncoder func(orgID string) (string, error)
	StateDecoder func(state string) (orgID string, err error)
)

// OAuth drives the per-org install of the global app into an org's
// workspace (FR-4, REST mode). StartURL builds the "Add to Slack"
// authorize link; HandleCallback exchanges the returned code for a bot
// token + team id and persists them as the org's transport.slack
// install.
type OAuth struct {
	app         GlobalApp
	registry    *configregistry.Registry
	encode      StateEncoder
	decode      StateDecoder
	redirectURI string
	tokenURL    string
	client      *http.Client
	logger      *slog.Logger
}

// NewOAuth builds the install flow.
func NewOAuth(app GlobalApp, reg *configregistry.Registry, encode StateEncoder, decode StateDecoder, redirectURI string, logger *slog.Logger) *OAuth {
	if logger == nil {
		logger = slog.Default()
	}
	return &OAuth{
		app:         app,
		registry:    reg,
		encode:      encode,
		decode:      decode,
		redirectURI: redirectURI,
		tokenURL:    defaultTokenURL,
		client:      &http.Client{Timeout: 10 * time.Second},
		logger:      logger,
	}
}

// SetTokenURL overrides the token-exchange endpoint (tests).
func (o *OAuth) SetTokenURL(u string) { o.tokenURL = u }

// SetHTTPClient overrides the HTTP client (tests).
func (o *OAuth) SetHTTPClient(c *http.Client) { o.client = c }

// StartURL builds the Slack OAuth v2 authorize URL the admin is sent to
// when installing the app into their org's workspace. The org id is
// carried in the state param so the callback knows which org installed.
func (o *OAuth) StartURL(ctx context.Context, orgID string, scopes []string) (string, error) {
	app, err := o.app(ctx)
	if err != nil {
		return "", err
	}
	state, err := o.encode(orgID)
	if err != nil {
		return "", fmt.Errorf("encode state: %w", err)
	}
	q := url.Values{}
	q.Set("client_id", app.ClientID)
	q.Set("scope", strings.Join(scopes, ","))
	q.Set("redirect_uri", o.redirectURI)
	q.Set("state", state)
	return authorizeURL + "?" + q.Encode(), nil
}

// oauthV2Response is the subset of Slack's oauth.v2.access response we
// persist: the bot access token and the installed workspace's team id.
type oauthV2Response struct {
	OK          bool   `json:"ok"`
	Error       string `json:"error"`
	AccessToken string `json:"access_token"`
	Team        struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	} `json:"team"`
}

// HandleCallback completes the install: decode the org from state,
// exchange the code for a bot token + team id against the global app's
// client credentials, and persist them as transport.slack for that org.
// A state that won't decode is rejected before any exchange or write.
func (o *OAuth) HandleCallback(ctx context.Context, code, state string) error {
	orgID, err := o.decode(state)
	if err != nil {
		return fmt.Errorf("slack oauth: bad state: %w", err)
	}
	if orgID == "" {
		return fmt.Errorf("slack oauth: empty org in state")
	}
	app, err := o.app(ctx)
	if err != nil {
		return err
	}

	form := url.Values{}
	form.Set("client_id", app.ClientID)
	form.Set("client_secret", app.ClientSecret)
	form.Set("code", code)
	form.Set("redirect_uri", o.redirectURI)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, o.tokenURL, strings.NewReader(form.Encode()))
	if err != nil {
		return fmt.Errorf("slack oauth: build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := o.client.Do(req)
	if err != nil {
		return fmt.Errorf("slack oauth: exchange: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	body, _ := io.ReadAll(resp.Body)

	var parsed oauthV2Response
	if err := json.Unmarshal(body, &parsed); err != nil {
		return fmt.Errorf("slack oauth: decode response: %w", err)
	}
	if !parsed.OK {
		return fmt.Errorf("slack oauth: exchange failed: %s", parsed.Error)
	}
	if parsed.AccessToken == "" || parsed.Team.ID == "" {
		return fmt.Errorf("slack oauth: response missing access_token or team id")
	}

	cfg := Config{BotToken: parsed.AccessToken, TeamID: parsed.Team.ID}
	val, err := json.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("slack oauth: marshal config: %w", err)
	}
	if err := o.registry.Set(ctx, orgID, configKey, string(val)); err != nil {
		return fmt.Errorf("slack oauth: persist install: %w", err)
	}
	o.logger.Info("slack.oauth: installed", "org", orgID, "team", parsed.Team.ID)
	return nil
}
