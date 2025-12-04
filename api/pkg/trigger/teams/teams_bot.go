package teams

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/helixml/helix/api/pkg/config"
	"github.com/helixml/helix/api/pkg/controller"
	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/trigger/shared"
	"github.com/helixml/helix/api/pkg/types"

	"github.com/infracloudio/msbotbuilder-go/core"
	"github.com/infracloudio/msbotbuilder-go/schema"
	"github.com/rs/zerolog/log"
)

// tokenCache holds cached OAuth tokens
type tokenCache struct {
	token     string
	expiresAt time.Time
	mu        sync.RWMutex
}

// TeamsBot - agent instance that connects to Microsoft Teams
type TeamsBot struct {
	cfg        *config.ServerConfig
	store      store.Store
	controller *controller.Controller

	ctx       context.Context
	ctxCancel context.CancelFunc

	app     *types.App // App/agent configuration
	trigger *types.TeamsTrigger

	adapter    core.Adapter
	tokenCache *tokenCache
}

func NewTeamsBot(cfg *config.ServerConfig, store store.Store, controller *controller.Controller, app *types.App, trigger *types.TeamsTrigger) *TeamsBot {
	return &TeamsBot{
		cfg:        cfg,
		store:      store,
		controller: controller,
		app:        app,
		trigger:    trigger,
		tokenCache: &tokenCache{},
	}
}

func (t *TeamsBot) Stop() {
	if t.ctxCancel != nil {
		log.Info().Str("app_id", t.app.ID).Msg("stopping Teams bot")
		t.ctxCancel()
	}
}

// Update controller status with the current status of the bot
func (t *TeamsBot) setStatus(ok bool, message string) {
	t.controller.SetTriggerStatus(t.app.ID, types.TriggerTypeTeams, types.TriggerStatus{
		Type:    types.TriggerTypeTeams,
		OK:      ok,
		Message: message,
	})
}

// Start initializes the Teams bot adapter
func (t *TeamsBot) Start(ctx context.Context) error {
	defer func() {
		if r := recover(); r != nil {
			log.Error().Msgf("panic in Teams bot: %v", r)
		}
	}()

	log.Info().Str("app_id", t.app.ID).Msg("starting Teams bot")
	defer log.Info().Str("app_id", t.app.ID).Msg("stopping Teams bot")

	t.ctx, t.ctxCancel = context.WithCancel(ctx)

	// Create Bot Framework adapter settings
	setting := core.AdapterSetting{
		AppID:       t.trigger.AppID,
		AppPassword: t.trigger.AppPassword,
	}

	// Log the credentials being used (mask password)
	maskedPassword := "***"
	if len(t.trigger.AppPassword) > 4 {
		maskedPassword = t.trigger.AppPassword[:2] + "***" + t.trigger.AppPassword[len(t.trigger.AppPassword)-2:]
	}
	log.Info().
		Str("app_id", t.app.ID).
		Str("ms_app_id", t.trigger.AppID).
		Str("ms_app_password", maskedPassword).
		Int("password_length", len(t.trigger.AppPassword)).
		Msg("Teams bot credentials")

	// For single-tenant bots, set the tenant ID for authentication
	// Multi-tenant bots should leave this empty (defaults to botframework.com)
	if t.trigger.TenantID != "" {
		setting.ChannelAuthTenant = t.trigger.TenantID
		// Also set the OAuth endpoint for single-tenant token acquisition
		setting.OauthEndpoint = fmt.Sprintf("https://login.microsoftonline.com/%s/oauth2/v2.0/token", t.trigger.TenantID)
		log.Info().
			Str("app_id", t.app.ID).
			Str("tenant_id", t.trigger.TenantID).
			Str("oauth_endpoint", setting.OauthEndpoint).
			Msg("using single-tenant authentication")
	}

	// Create adapter
	adapter, err := core.NewBotAdapter(setting)
	if err != nil {
		t.setStatus(false, fmt.Sprintf("Failed to create adapter: %v", err))
		return fmt.Errorf("failed to create bot adapter: %w", err)
	}
	t.adapter = adapter

	t.setStatus(true, "Teams bot ready (waiting for webhook)")

	// Wait for context cancellation
	<-t.ctx.Done()

	return nil
}

// HandleActivity processes an incoming activity from Teams webhook
func (t *TeamsBot) HandleActivity(w http.ResponseWriter, req *http.Request) {
	log.Debug().Str("app_id", t.app.ID).Msg("HandleActivity called")

	if t.adapter == nil {
		log.Error().Str("app_id", t.app.ID).Msg("adapter not initialized")
		http.Error(w, "Bot not ready", http.StatusServiceUnavailable)
		return
	}

	ctx := req.Context()

	// Parse the incoming request to get the activity (validates auth)
	incomingActivity, err := t.adapter.ParseRequest(ctx, req)
	if err != nil {
		log.Error().Err(err).Str("app_id", t.app.ID).Msg("failed to parse request")
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	// Handle the activity based on type
	// We bypass the library's ProcessActivity because it doesn't support single-tenant auth for sending
	// Use a background context since the HTTP request context will be canceled after we return
	bgCtx := context.Background()

	switch incomingActivity.Type {
	case schema.Message:
		go t.handleMessageActivity(bgCtx, incomingActivity)
	case schema.ConversationUpdate:
		go t.handleConversationUpdateActivity(bgCtx, incomingActivity)
	case "typing":
		// Ignore typing indicators
		log.Debug().Str("app_id", t.app.ID).Msg("ignoring typing indicator")
	default:
		log.Debug().
			Str("app_id", t.app.ID).
			Str("activity_type", string(incomingActivity.Type)).
			Msg("ignoring unsupported activity type")
	}

	// Return 200 OK immediately - we process async
	w.WriteHeader(http.StatusOK)
}

// handleMessageActivity processes a message activity asynchronously
func (t *TeamsBot) handleMessageActivity(ctx context.Context, incomingActivity schema.Activity) {
	log.Info().
		Str("app_id", t.app.ID).
		Str("conversation_id", incomingActivity.Conversation.ID).
		Str("channel_id", incomingActivity.ChannelID).
		Str("from_id", incomingActivity.From.ID).
		Str("from_name", incomingActivity.From.Name).
		Str("text", incomingActivity.Text).
		Str("service_url", incomingActivity.ServiceURL).
		Str("recipient_id", incomingActivity.Recipient.ID).
		Msg("received Teams message")

	// Remove bot mention from the message text
	messageText := t.removeBotMention(incomingActivity)

	if strings.TrimSpace(messageText) == "" {
		log.Debug().Str("app_id", t.app.ID).Msg("empty message after removing mention, ignoring")
		return
	}

	// Get conversation ID and check if we have an existing thread
	conversationID := incomingActivity.Conversation.ID
	channelID := incomingActivity.ChannelID
	teamID := ""
	if incomingActivity.ChannelData != nil {
		if tid, ok := incomingActivity.ChannelData["teamsTeamId"].(string); ok {
			teamID = tid
		}
	}

	// Check for existing thread
	existingThread, err := t.store.GetTeamsThread(ctx, t.app.ID, conversationID)
	isNewConversation := err != nil || existingThread == nil

	// Handle the message
	response, err := t.handleMessage(ctx, existingThread, messageText, conversationID, channelID, teamID, isNewConversation)
	if err != nil {
		log.Error().Err(err).Str("app_id", t.app.ID).Msg("failed to handle message")
		errorResponse := fmt.Sprintf("Sorry, I encountered an error: %v", err)
		if sendErr := t.sendActivityDirect(ctx, incomingActivity, errorResponse); sendErr != nil {
			log.Error().Err(sendErr).Str("app_id", t.app.ID).Msg("failed to send error response")
		}
		return
	}

	// Convert markdown to Teams-compatible format and send
	teamsFormattedResponse := convertMarkdownToTeamsFormat(response)
	if err := t.sendActivityDirect(ctx, incomingActivity, teamsFormattedResponse); err != nil {
		log.Error().Err(err).Str("app_id", t.app.ID).Msg("failed to send response")
	}
}

// handleConversationUpdateActivity handles conversation update events
func (t *TeamsBot) handleConversationUpdateActivity(ctx context.Context, incomingActivity schema.Activity) {
	log.Debug().
		Str("app_id", t.app.ID).
		Str("conversation_id", incomingActivity.Conversation.ID).
		Msg("conversation update received")

	// Check if the bot was added to the conversation
	for _, member := range incomingActivity.MembersAdded {
		if member.ID == incomingActivity.Recipient.ID {
			log.Info().
				Str("app_id", t.app.ID).
				Str("conversation_id", incomingActivity.Conversation.ID).
				Msg("bot added to conversation")

			welcomeMsg := "Hello! I'm a Helix AI assistant. Mention me or send me a message to get started."
			if err := t.sendActivityDirect(ctx, incomingActivity, welcomeMsg); err != nil {
				log.Error().Err(err).Str("app_id", t.app.ID).Msg("failed to send welcome message")
			}
			return
		}
	}
}

func (t *TeamsBot) handleMessage(ctx context.Context, existingThread *types.TeamsThread, messageText, conversationID, channelID, teamID string, isNewConversation bool) (string, error) {
	log.Debug().
		Str("app_id", t.app.ID).
		Str("conversation_id", conversationID).
		Bool("is_new_conversation", isNewConversation).
		Msg("handleMessage called")

	var (
		session *types.Session
		err     error
	)

	if isNewConversation {
		// Create new session and thread
		log.Info().
			Str("app_id", t.app.ID).
			Str("conversation_id", conversationID).
			Msg("starting new Teams session")

		newSession := shared.NewTriggerSession(ctx, types.TriggerTypeTeams.String(), t.app)
		session = newSession.Session

		// Create the new thread
		_, err = t.createNewThread(ctx, conversationID, channelID, teamID, session.ID)
		if err != nil {
			log.Error().Err(err).Msg("failed to create new thread")
			return "", fmt.Errorf("failed to create new thread: %w", err)
		}

		log.Debug().
			Str("app_id", t.app.ID).
			Str("conversation_id", conversationID).
			Str("session_id", session.ID).
			Msg("stored new session for conversation")

		err = t.controller.WriteSession(ctx, session)
		if err != nil {
			log.Error().Err(err).Str("app_id", t.app.ID).Msg("failed to create session")
			return "", fmt.Errorf("failed to create session: %w", err)
		}
	} else {
		// Continue existing conversation
		session, err = t.store.GetSession(ctx, existingThread.SessionID)
		if err != nil {
			log.Error().Err(err).Msg("failed to get session")
			return "", fmt.Errorf("failed to get session: %w", err)
		}

		log.Info().
			Str("app_id", t.app.ID).
			Str("conversation_id", conversationID).
			Str("session_id", session.ID).
			Msg("continuing existing Teams session")
	}

	// Get user for the request
	user, err := t.store.GetUser(ctx, &store.GetUserQuery{
		ID: t.app.Owner,
	})
	if err != nil {
		log.Error().Err(err).Str("app_id", t.app.ID).Str("user_id", t.app.Owner).Msg("failed to get user")
		return "", fmt.Errorf("failed to get user: %w", err)
	}

	resp, err := t.controller.RunBlockingSession(ctx, &controller.RunSessionRequest{
		OrganizationID: t.app.OrganizationID,
		App:            t.app,
		Session:        session,
		User:           user,
		PromptMessage:  types.MessageContent{Parts: []any{messageText}},
	})
	if err != nil {
		return "", fmt.Errorf("failed to get response from inference API: %w", err)
	}

	return resp.ResponseMessage, nil
}

func (t *TeamsBot) createNewThread(ctx context.Context, conversationID, channelID, teamID, sessionID string) (*types.TeamsThread, error) {
	thread := &types.TeamsThread{
		ConversationID: conversationID,
		AppID:          t.app.ID,
		ChannelID:      channelID,
		TeamID:         teamID,
		SessionID:      sessionID,
	}

	return t.store.CreateTeamsThread(ctx, thread)
}

// removeBotMention removes the bot @mention from the message text
// In Teams, mentions are typically in the format <at>BotName</at>
func (t *TeamsBot) removeBotMention(act schema.Activity) string {
	text := act.Text

	// Remove HTML-style mentions like <at>BotName</at>
	atMentionRegex := regexp.MustCompile(`<at>[^<]*</at>`)
	text = atMentionRegex.ReplaceAllString(text, "")

	return strings.TrimSpace(text)
}

// convertMarkdownToTeamsFormat converts markdown to Teams-compatible format
// Teams supports a subset of markdown, this function handles the conversion
func convertMarkdownToTeamsFormat(markdown string) string {
	result := markdown

	// Teams supports most standard markdown, but we need to handle a few edge cases

	// Convert [DOC_ID:xxx] markers to numbered citations [1], [2], etc.
	// Teams can't link to internal documents, so we just show citation numbers
	result = shared.ConvertDocIDsToNumberedCitations(result)

	// Handle any triple newlines (reduce to double)
	multipleNewlines := regexp.MustCompile(`\n{3,}`)
	result = multipleNewlines.ReplaceAllString(result, "\n\n")

	return result
}

// getAccessToken acquires an OAuth token for sending messages to the Bot Framework
// Supports both multi-tenant (botframework.com) and single-tenant authentication
func (t *TeamsBot) getAccessToken(ctx context.Context) (string, error) {
	// Check cache first
	t.tokenCache.mu.RLock()
	if t.tokenCache.token != "" && time.Now().Before(t.tokenCache.expiresAt) {
		token := t.tokenCache.token
		t.tokenCache.mu.RUnlock()
		return token, nil
	}
	t.tokenCache.mu.RUnlock()

	// Determine the OAuth endpoint based on tenant configuration
	var tokenURL string
	if t.trigger.TenantID != "" {
		// Single-tenant: use tenant-specific endpoint
		tokenURL = fmt.Sprintf("https://login.microsoftonline.com/%s/oauth2/v2.0/token", t.trigger.TenantID)
	} else {
		// Multi-tenant: use botframework.com
		tokenURL = "https://login.microsoftonline.com/botframework.com/oauth2/v2.0/token"
	}

	// Prepare the token request
	data := url.Values{}
	data.Set("grant_type", "client_credentials")
	data.Set("client_id", t.trigger.AppID)
	data.Set("client_secret", t.trigger.AppPassword)
	data.Set("scope", "https://api.botframework.com/.default")

	log.Debug().
		Str("app_id", t.app.ID).
		Str("token_url", tokenURL).
		Str("client_id", t.trigger.AppID).
		Msg("requesting OAuth token")

	req, err := http.NewRequestWithContext(ctx, "POST", tokenURL, strings.NewReader(data.Encode()))
	if err != nil {
		return "", fmt.Errorf("failed to create token request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to execute token request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read token response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		log.Error().
			Str("app_id", t.app.ID).
			Int("status", resp.StatusCode).
			Str("body", string(body)).
			Msg("token request failed")
		return "", fmt.Errorf("token request failed with status %d: %s", resp.StatusCode, string(body))
	}

	var tokenResp struct {
		AccessToken string `json:"access_token"`
		ExpiresIn   int    `json:"expires_in"`
		TokenType   string `json:"token_type"`
	}
	if err := json.Unmarshal(body, &tokenResp); err != nil {
		return "", fmt.Errorf("failed to parse token response: %w", err)
	}

	// Cache the token with a small buffer before expiry
	t.tokenCache.mu.Lock()
	t.tokenCache.token = tokenResp.AccessToken
	t.tokenCache.expiresAt = time.Now().Add(time.Duration(tokenResp.ExpiresIn-60) * time.Second)
	t.tokenCache.mu.Unlock()

	log.Debug().
		Str("app_id", t.app.ID).
		Int("expires_in", tokenResp.ExpiresIn).
		Msg("acquired OAuth token")

	return tokenResp.AccessToken, nil
}

// sendActivityDirect sends an activity directly to the Bot Framework service
// This bypasses the msbotbuilder-go library's broken single-tenant support
func (t *TeamsBot) sendActivityDirect(ctx context.Context, incomingActivity schema.Activity, text string) error {
	// Get an access token
	token, err := t.getAccessToken(ctx)
	if err != nil {
		return fmt.Errorf("failed to get access token: %w", err)
	}

	// Create the reply activity
	replyActivity := schema.Activity{
		Type:         schema.Message,
		From:         incomingActivity.Recipient,
		Recipient:    incomingActivity.From,
		Conversation: incomingActivity.Conversation,
		ReplyToID:    incomingActivity.ID,
		Text:         text,
		TextFormat:   "markdown",
	}

	// Build the URL for sending the reply
	// Format: {serviceUrl}/v3/conversations/{conversationId}/activities/{activityId}
	sendURL := fmt.Sprintf("%sv3/conversations/%s/activities/%s",
		incomingActivity.ServiceURL,
		url.PathEscape(incomingActivity.Conversation.ID),
		url.PathEscape(incomingActivity.ID))

	log.Debug().
		Str("app_id", t.app.ID).
		Str("send_url", sendURL).
		Str("conversation_id", incomingActivity.Conversation.ID).
		Msg("sending activity to Bot Framework")

	// Serialize the activity
	activityJSON, err := json.Marshal(replyActivity)
	if err != nil {
		return fmt.Errorf("failed to marshal activity: %w", err)
	}

	// Create and send the request
	req, err := http.NewRequestWithContext(ctx, "POST", sendURL, bytes.NewReader(activityJSON))
	if err != nil {
		return fmt.Errorf("failed to create send request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send activity: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to read send response: %w", err)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		log.Error().
			Str("app_id", t.app.ID).
			Int("status", resp.StatusCode).
			Str("body", string(body)).
			Str("send_url", sendURL).
			Msg("failed to send activity")
		return fmt.Errorf("send activity failed with status %d: %s", resp.StatusCode, string(body))
	}

	log.Debug().
		Str("app_id", t.app.ID).
		Int("status", resp.StatusCode).
		Msg("activity sent successfully")

	return nil
}
