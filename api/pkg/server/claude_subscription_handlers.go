package server

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/gorilla/mux"
	"github.com/helixml/helix/api/pkg/crypto"
	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/system"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/rs/zerolog/log"
)

// @Summary Create a Claude subscription
// @Description Connect a Claude subscription by providing OAuth credentials
// @Tags Claude
// @Accept json
// @Produce json
// @Param body body types.CreateClaudeSubscriptionRequest true "Claude subscription credentials"
// @Success 200 {object} types.ClaudeSubscription
// @Failure 400 {object} system.HTTPError
// @Failure 401 {object} system.HTTPError
// @Failure 500 {object} system.HTTPError
// @Security BearerAuth
// @Router /api/v1/claude-subscriptions [post]
func (apiServer *HelixAPIServer) createClaudeSubscription(_ http.ResponseWriter, req *http.Request) (*types.ClaudeSubscription, *system.HTTPError) {
	user := getRequestUser(req)
	if user == nil {
		return nil, system.NewHTTPError401("authentication required")
	}

	var createReq types.CreateClaudeSubscriptionRequest
	if err := json.NewDecoder(req.Body).Decode(&createReq); err != nil {
		return nil, system.NewHTTPError400("invalid request body: " + err.Error())
	}

	// Validate credentials
	creds := createReq.Credentials.ClaudeAiOauth
	if creds.AccessToken == "" || creds.RefreshToken == "" {
		return nil, system.NewHTTPError400("accessToken and refreshToken are required")
	}

	// Determine owner
	ownerID := user.ID
	ownerType := types.OwnerTypeUser
	if createReq.OwnerType == types.OwnerTypeOrg {
		if createReq.OwnerID == "" {
			return nil, system.NewHTTPError400("owner_id required for org-level subscriptions")
		}
		// Verify user is org owner/admin
		_, err := apiServer.authorizeOrgOwner(req.Context(), user, createReq.OwnerID)
		if err != nil {
			return nil, system.NewHTTPError403("not authorized to manage org subscriptions: " + err.Error())
		}
		ownerID = createReq.OwnerID
		ownerType = types.OwnerTypeOrg
	}

	// Encrypt credentials
	credJSON, err := json.Marshal(creds)
	if err != nil {
		return nil, system.NewHTTPError500("failed to marshal credentials")
	}

	encKey, err := crypto.GetEncryptionKey()
	if err != nil {
		return nil, system.NewHTTPError500("failed to get encryption key")
	}

	encrypted, err := crypto.EncryptAES256GCM(credJSON, encKey)
	if err != nil {
		return nil, system.NewHTTPError500("failed to encrypt credentials")
	}

	// Calculate token expiry
	var expiresAt time.Time
	if creds.ExpiresAt > 0 {
		expiresAt = time.UnixMilli(creds.ExpiresAt)
	}

	sub := &types.ClaudeSubscription{
		OwnerID:              ownerID,
		OwnerType:            ownerType,
		Name:                 createReq.Name,
		EncryptedCredentials: encrypted,
		SubscriptionType:     creds.SubscriptionType,
		RateLimitTier:        creds.RateLimitTier,
		Scopes:               creds.Scopes,
		AccessTokenExpiresAt: expiresAt,
		Status:               "active",
		CreatedBy:            user.ID,
	}

	created, err := apiServer.Store.CreateClaudeSubscription(req.Context(), sub)
	if err != nil {
		return nil, system.NewHTTPError500("failed to create subscription: " + err.Error())
	}

	log.Info().
		Str("subscription_id", created.ID).
		Str("owner_id", ownerID).
		Str("owner_type", string(ownerType)).
		Str("subscription_type", creds.SubscriptionType).
		Msg("Created Claude subscription")

	return created, nil
}

// @Summary List Claude subscriptions
// @Description List Claude subscriptions for the current user and their org
// @Tags Claude
// @Produce json
// @Success 200 {array} types.ClaudeSubscription
// @Failure 401 {object} system.HTTPError
// @Failure 500 {object} system.HTTPError
// @Security BearerAuth
// @Router /api/v1/claude-subscriptions [get]
func (apiServer *HelixAPIServer) listClaudeSubscriptions(_ http.ResponseWriter, req *http.Request) ([]*types.ClaudeSubscription, *system.HTTPError) {
	user := getRequestUser(req)
	if user == nil {
		return nil, system.NewHTTPError401("authentication required")
	}

	// Get user's own subscriptions
	subs, err := apiServer.Store.ListClaudeSubscriptions(req.Context(), user.ID)
	if err != nil {
		return nil, system.NewHTTPError500("failed to list subscriptions: " + err.Error())
	}

	// Also get org subscriptions for any orgs the user belongs to
	memberships, err := apiServer.Store.ListOrganizationMemberships(req.Context(), &store.ListOrganizationMembershipsQuery{
		UserID: user.ID,
	})
	if err == nil {
		for _, m := range memberships {
			orgSubs, err := apiServer.Store.ListClaudeSubscriptions(req.Context(), m.OrganizationID)
			if err != nil {
				log.Warn().Err(err).Str("org_id", m.OrganizationID).Msg("Failed to list org Claude subscriptions")
				continue
			}
			subs = append(subs, orgSubs...)
		}
	}

	return subs, nil
}

// @Summary Get a Claude subscription
// @Description Get details of a specific Claude subscription (no secrets)
// @Tags Claude
// @Produce json
// @Param id path string true "Subscription ID"
// @Success 200 {object} types.ClaudeSubscription
// @Failure 401 {object} system.HTTPError
// @Failure 403 {object} system.HTTPError
// @Failure 404 {object} system.HTTPError
// @Security BearerAuth
// @Router /api/v1/claude-subscriptions/{id} [get]
func (apiServer *HelixAPIServer) getClaudeSubscription(_ http.ResponseWriter, req *http.Request) (*types.ClaudeSubscription, *system.HTTPError) {
	user := getRequestUser(req)
	if user == nil {
		return nil, system.NewHTTPError401("authentication required")
	}

	vars := mux.Vars(req)
	id := vars["id"]

	sub, err := apiServer.Store.GetClaudeSubscription(req.Context(), id)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return nil, system.NewHTTPError404("subscription not found")
		}
		return nil, system.NewHTTPError500("failed to get subscription: " + err.Error())
	}

	// Verify ownership
	if sub.OwnerType == types.OwnerTypeUser && sub.OwnerID != user.ID {
		return nil, system.NewHTTPError403("access denied")
	}
	if sub.OwnerType == types.OwnerTypeOrg {
		if _, err := apiServer.authorizeOrgOwner(req.Context(), user, sub.OwnerID); err != nil {
			return nil, system.NewHTTPError403("access denied")
		}
	}

	return sub, nil
}

// @Summary Delete a Claude subscription
// @Description Disconnect a Claude subscription
// @Tags Claude
// @Param id path string true "Subscription ID"
// @Success 200 {object} map[string]string
// @Failure 401 {object} system.HTTPError
// @Failure 403 {object} system.HTTPError
// @Failure 404 {object} system.HTTPError
// @Security BearerAuth
// @Router /api/v1/claude-subscriptions/{id} [delete]
func (apiServer *HelixAPIServer) deleteClaudeSubscription(_ http.ResponseWriter, req *http.Request) (map[string]string, *system.HTTPError) {
	user := getRequestUser(req)
	if user == nil {
		return nil, system.NewHTTPError401("authentication required")
	}

	vars := mux.Vars(req)
	id := vars["id"]

	sub, err := apiServer.Store.GetClaudeSubscription(req.Context(), id)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return nil, system.NewHTTPError404("subscription not found")
		}
		return nil, system.NewHTTPError500("failed to get subscription: " + err.Error())
	}

	// Verify ownership
	if sub.OwnerType == types.OwnerTypeUser && sub.OwnerID != user.ID {
		return nil, system.NewHTTPError403("access denied")
	}
	if sub.OwnerType == types.OwnerTypeOrg {
		if _, err := apiServer.authorizeOrgOwner(req.Context(), user, sub.OwnerID); err != nil {
			return nil, system.NewHTTPError403("access denied")
		}
	}

	if err := apiServer.Store.DeleteClaudeSubscription(req.Context(), id); err != nil {
		return nil, system.NewHTTPError500("failed to delete subscription: " + err.Error())
	}

	log.Info().
		Str("subscription_id", id).
		Str("user_id", user.ID).
		Msg("Deleted Claude subscription")

	return map[string]string{"status": "ok"}, nil
}

// ClaudeModel represents a Claude model available via Claude Code
type ClaudeModel struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
}

// @Summary List available Claude models
// @Description List Claude models available through Claude Code subscriptions
// @Tags Claude
// @Produce json
// @Success 200 {array} ClaudeModel
// @Security BearerAuth
// @Router /api/v1/claude-subscriptions/models [get]
func (apiServer *HelixAPIServer) listClaudeModels(_ http.ResponseWriter, req *http.Request) ([]*ClaudeModel, *system.HTTPError) {
	models := []*ClaudeModel{
		{ID: "claude-opus-4-6", Name: "Claude Opus 4.6", Description: "Most capable Claude model"},
		{ID: "claude-sonnet-4-5-latest", Name: "Claude Sonnet 4.5", Description: "Best balance of speed and capability"},
		{ID: "claude-haiku-4-5-latest", Name: "Claude Haiku 4.5", Description: "Fastest Claude model"},
	}
	return models, nil
}

// @Summary Get Claude credentials for a session
// @Description Get decrypted Claude credentials for use inside a desktop container.
// @Description Only accepts runner/session-scoped tokens.
// @Tags Claude
// @Produce json
// @Param id path string true "Session ID"
// @Success 200 {object} types.ClaudeOAuthCredentials
// @Failure 401 {object} system.HTTPError
// @Failure 403 {object} system.HTTPError
// @Failure 404 {object} system.HTTPError
// @Failure 500 {object} system.HTTPError
// @Security ApiKeyAuth
// @Router /api/v1/sessions/{id}/claude-credentials [get]
func (apiServer *HelixAPIServer) getSessionClaudeCredentials(_ http.ResponseWriter, req *http.Request) (*types.ClaudeOAuthCredentials, *system.HTTPError) {
	ctx := req.Context()
	vars := mux.Vars(req)
	sessionID := vars["id"]

	// Get session
	session, err := apiServer.Store.GetSession(ctx, sessionID)
	if err != nil {
		return nil, system.NewHTTPError404("session not found")
	}

	// Only allow runner token or session owner (same pattern as getZedConfig)
	user := getRequestUser(req)
	if user == nil {
		return nil, system.NewHTTPError403("access denied")
	}
	if user.TokenType != types.TokenTypeRunner && session.Owner != user.ID {
		return nil, system.NewHTTPError403("access denied")
	}

	// Use the session's organization (if any)
	orgID := session.OrganizationID

	// Get effective Claude subscription (user-level first, then org)
	sub, err := apiServer.Store.GetEffectiveClaudeSubscription(ctx, session.Owner, orgID)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return nil, system.NewHTTPError404("no Claude subscription found for session owner")
		}
		return nil, system.NewHTTPError500(fmt.Sprintf("failed to get Claude subscription: %v", err))
	}

	// Decrypt credentials
	encKey, err := crypto.GetEncryptionKey()
	if err != nil {
		return nil, system.NewHTTPError500("failed to get encryption key")
	}

	plaintext, err := crypto.DecryptAES256GCM(sub.EncryptedCredentials, encKey)
	if err != nil {
		return nil, system.NewHTTPError500("failed to decrypt credentials")
	}

	var creds types.ClaudeOAuthCredentials
	if err := json.Unmarshal(plaintext, &creds); err != nil {
		return nil, system.NewHTTPError500("failed to parse credentials")
	}

	return &creds, nil
}

// ClaudeLoginSessionResponse is returned when starting a Claude login session
type ClaudeLoginSessionResponse struct {
	SessionID string `json:"session_id"`
}

// @Summary Start a Claude login session
// @Description Launch a temporary desktop session for interactive Claude OAuth login
// @Tags Claude
// @Produce json
// @Success 200 {object} ClaudeLoginSessionResponse
// @Failure 401 {object} system.HTTPError
// @Failure 500 {object} system.HTTPError
// @Security BearerAuth
// @Router /api/v1/claude-subscriptions/start-login [post]
func (apiServer *HelixAPIServer) startClaudeLogin(_ http.ResponseWriter, req *http.Request) (*ClaudeLoginSessionResponse, *system.HTTPError) {
	user := getRequestUser(req)
	if user == nil {
		return nil, system.NewHTTPError401("authentication required")
	}

	// Get user's org ID (use first org membership)
	orgID := ""
	memberships, err := apiServer.Store.ListOrganizationMemberships(req.Context(), &store.ListOrganizationMembershipsQuery{
		UserID: user.ID,
	})
	if err == nil && len(memberships) > 0 {
		orgID = memberships[0].OrganizationID
	}

	// Create a minimal session for the login flow
	session := &types.Session{
		ID:             system.GenerateSessionID(),
		Name:           "Claude Login",
		Created:        time.Now(),
		Updated:        time.Now(),
		Mode:           types.SessionModeInference,
		Type:           types.SessionTypeText,
		Provider:       "anthropic",
		ModelName:      "external_agent",
		Owner:          user.ID,
		OwnerType:      types.OwnerTypeUser,
		OrganizationID: orgID,
		Metadata: types.SessionMetadata{
			Stream:      true,
			AgentType:   "zed_external",
			SessionRole: "exploratory",
		},
	}

	createdSession, err := apiServer.Store.CreateSession(req.Context(), *session)
	if err != nil {
		log.Error().Err(err).Str("user_id", user.ID).Msg("Failed to create Claude login session")
		return nil, system.NewHTTPError500("failed to create session")
	}

	// Create a desktop agent with minimal configuration
	zedAgent := &types.DesktopAgent{
		OrganizationID: orgID,
		SessionID:      createdSession.ID,
		UserID:         user.ID,
		Input:          "Claude Code login",
		ProjectPath:    "workspace",
		DisplayWidth:   1920,
		DisplayHeight:  1080,
		DesktopType:    "ubuntu",
	}

	// Add user's API token
	if addErr := apiServer.addUserAPITokenToAgent(req.Context(), zedAgent, user.ID); addErr != nil {
		log.Error().Err(addErr).Str("user_id", user.ID).Msg("Failed to add user API token for Claude login session")
		return nil, system.NewHTTPError500("failed to configure session")
	}

	// Start the desktop container
	agentResp, startErr := apiServer.externalAgentExecutor.StartDesktop(req.Context(), zedAgent)
	if startErr != nil {
		log.Error().Err(startErr).Str("session_id", createdSession.ID).Msg("Failed to start Claude login desktop")
		return nil, system.NewHTTPError500("failed to start desktop session")
	}

	// Update session with container info
	if agentResp.DevContainerID != "" || agentResp.SandboxID != "" {
		createdSession.Metadata.DevContainerID = agentResp.DevContainerID
		createdSession.SandboxID = agentResp.SandboxID
		if _, updateErr := apiServer.Store.UpdateSession(req.Context(), *createdSession); updateErr != nil {
			log.Error().Err(updateErr).Str("session_id", createdSession.ID).Msg("Failed to store container data")
		}
	}

	log.Info().
		Str("session_id", createdSession.ID).
		Str("user_id", user.ID).
		Msg("Started Claude login desktop session")

	return &ClaudeLoginSessionResponse{
		SessionID: createdSession.ID,
	}, nil
}

// ClaudePollLoginResponse is returned when polling for Claude credentials
type ClaudePollLoginResponse struct {
	Found       bool   `json:"found"`
	Credentials string `json:"credentials,omitempty"` // Raw credentials JSON
}

// @Summary Poll for Claude login credentials
// @Description Check if Claude credentials file has been written inside the desktop container
// @Tags Claude
// @Produce json
// @Param sessionId path string true "Session ID"
// @Success 200 {object} ClaudePollLoginResponse
// @Failure 401 {object} system.HTTPError
// @Failure 403 {object} system.HTTPError
// @Failure 404 {object} system.HTTPError
// @Failure 500 {object} system.HTTPError
// @Security BearerAuth
// @Router /api/v1/claude-subscriptions/poll-login/{sessionId} [get]
func (apiServer *HelixAPIServer) pollClaudeLogin(_ http.ResponseWriter, req *http.Request) (*ClaudePollLoginResponse, *system.HTTPError) {
	user := getRequestUser(req)
	if user == nil {
		return nil, system.NewHTTPError401("authentication required")
	}

	vars := mux.Vars(req)
	sessionID := vars["sessionId"]

	// Verify session ownership
	session, err := apiServer.Store.GetSession(req.Context(), sessionID)
	if err != nil {
		return nil, system.NewHTTPError404("session not found")
	}
	if session.Owner != user.ID {
		return nil, system.NewHTTPError403("access denied")
	}

	// Connect to desktop container via RevDial and read credentials file
	runnerID := fmt.Sprintf("desktop-%s", sessionID)
	revDialConn, err := apiServer.connman.Dial(req.Context(), runnerID)
	if err != nil {
		// Container not ready yet
		return &ClaudePollLoginResponse{Found: false}, nil
	}
	defer revDialConn.Close()

	// Execute: cat /home/retro/.claude/.credentials.json
	execReq := map[string]interface{}{
		"command": []string{"cat", "/home/retro/.claude/.credentials.json"},
		"timeout": 5,
	}
	execBody, _ := json.Marshal(execReq)

	httpReq, err := http.NewRequest("POST", "http://localhost:9876/exec", bytes.NewReader(execBody))
	if err != nil {
		return nil, system.NewHTTPError500("failed to create exec request")
	}
	httpReq.Header.Set("Content-Type", "application/json")

	if err := httpReq.Write(revDialConn); err != nil {
		return &ClaudePollLoginResponse{Found: false}, nil
	}

	execResp, err := http.ReadResponse(bufio.NewReader(revDialConn), httpReq)
	if err != nil {
		return &ClaudePollLoginResponse{Found: false}, nil
	}
	defer execResp.Body.Close()

	bodyBytes, err := io.ReadAll(execResp.Body)
	if err != nil {
		return &ClaudePollLoginResponse{Found: false}, nil
	}

	var execResult struct {
		Success  bool   `json:"success"`
		Output   string `json:"output"`
		ExitCode int    `json:"exit_code"`
	}
	if err := json.Unmarshal(bodyBytes, &execResult); err != nil {
		return &ClaudePollLoginResponse{Found: false}, nil
	}

	if !execResult.Success || execResult.ExitCode != 0 || execResult.Output == "" {
		return &ClaudePollLoginResponse{Found: false}, nil
	}

	// Validate it's valid JSON with expected structure
	var credCheck map[string]interface{}
	if err := json.Unmarshal([]byte(execResult.Output), &credCheck); err != nil {
		return &ClaudePollLoginResponse{Found: false}, nil
	}

	// Check for either claudeAiOauth wrapper or direct accessToken
	if _, ok := credCheck["claudeAiOauth"]; !ok {
		if _, ok := credCheck["accessToken"]; !ok {
			return &ClaudePollLoginResponse{Found: false}, nil
		}
	}

	return &ClaudePollLoginResponse{
		Found:       true,
		Credentials: execResult.Output,
	}, nil
}
