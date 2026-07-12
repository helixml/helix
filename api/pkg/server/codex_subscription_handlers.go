package server

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/gorilla/mux"
	"github.com/helixml/helix/api/pkg/crypto"
	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/system"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/rs/zerolog/log"
)

const maxCodexCredentialsBytes = 64 << 10

var codexDeviceCodePattern = regexp.MustCompile(`\b[A-Z0-9]{4}-[A-Z0-9]{5}\b`)
var codexDeviceURLPattern = regexp.MustCompile(`https://auth\.openai\.com/codex/device`)
var ansiEscapePattern = regexp.MustCompile(`\x1b\[[0-9;]*m`)

func decodeCodexCredentials(body io.Reader) (types.CodexAuthCredentials, error) {
	var credentials types.CodexAuthCredentials
	decoder := json.NewDecoder(io.LimitReader(body, maxCodexCredentialsBytes+1))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&credentials); err != nil {
		return credentials, fmt.Errorf("decode credentials: %w", err)
	}
	if credentials.AuthMode != "chatgpt" {
		return credentials, fmt.Errorf("auth_mode must be chatgpt")
	}
	if credentials.Tokens.IDToken == "" || credentials.Tokens.AccessToken == "" || credentials.Tokens.RefreshToken == "" || credentials.Tokens.AccountID == "" {
		return credentials, fmt.Errorf("id_token, access_token, refresh_token, and account_id are required")
	}
	if credentials.LastRefresh.IsZero() {
		return credentials, fmt.Errorf("last_refresh is required")
	}
	return credentials, nil
}

// @Summary Create a Codex subscription
// @Description Connect a ChatGPT subscription using Codex CLI credentials
// @Tags Codex
// @Accept json
// @Produce json
// @Param body body types.CreateCodexSubscriptionRequest true "Codex subscription credentials"
// @Success 200 {object} types.CodexSubscription
// @Security BearerAuth
// @Router /api/v1/codex-subscriptions [post]
func (apiServer *HelixAPIServer) createCodexSubscription(_ http.ResponseWriter, req *http.Request) (*types.CodexSubscription, *system.HTTPError) {
	user := getRequestUser(req)
	if user == nil {
		return nil, system.NewHTTPError401("authentication required")
	}
	var createReq types.CreateCodexSubscriptionRequest
	decoder := json.NewDecoder(io.LimitReader(req.Body, maxCodexCredentialsBytes+1))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&createReq); err != nil {
		return nil, system.NewHTTPError400("invalid request body: " + err.Error())
	}
	credentials := createReq.Credentials
	if err := validateCodexCredentials(credentials); err != nil {
		return nil, system.NewHTTPError400(err.Error())
	}
	normalizeCodexSubscriptionCredentials(&credentials)

	ownerID, ownerType := user.ID, types.OwnerTypeUser
	if createReq.OwnerType == types.OwnerTypeOrg {
		if createReq.OwnerID == "" {
			return nil, system.NewHTTPError400("owner_id required for org-level subscriptions")
		}
		if _, err := apiServer.authorizeOrgOwner(req.Context(), user, createReq.OwnerID); err != nil {
			return nil, system.NewHTTPError403("not authorized to manage org subscriptions")
		}
		ownerID, ownerType = createReq.OwnerID, types.OwnerTypeOrg
	}

	credentialJSON, err := json.Marshal(credentials)
	if err != nil {
		return nil, system.NewHTTPError500("failed to marshal credentials")
	}
	encryptionKey, err := crypto.GetEncryptionKey()
	if err != nil {
		return nil, system.NewHTTPError500("failed to get encryption key")
	}
	encrypted, err := crypto.EncryptAES256GCM(credentialJSON, encryptionKey)
	if err != nil {
		return nil, system.NewHTTPError500("failed to encrypt credentials")
	}
	name := createReq.Name
	if name == "" {
		name = "My Codex Subscription"
	}
	sub, err := apiServer.Store.CreateCodexSubscription(req.Context(), &types.CodexSubscription{
		OwnerID: ownerID, OwnerType: ownerType, Name: name,
		EncryptedCredentials: encrypted, AccountID: credentials.Tokens.AccountID,
		AuthMode: credentials.AuthMode, Status: "active", LastRefreshedAt: &credentials.LastRefresh,
		CreatedBy: user.ID,
	})
	if err != nil {
		return nil, system.NewHTTPError500("failed to create subscription: " + err.Error())
	}
	return sub, nil
}

func validateCodexCredentials(credentials types.CodexAuthCredentials) error {
	if credentials.AuthMode != "chatgpt" {
		return fmt.Errorf("auth_mode must be chatgpt")
	}
	if credentials.Tokens.IDToken == "" || credentials.Tokens.AccessToken == "" || credentials.Tokens.RefreshToken == "" || credentials.Tokens.AccountID == "" {
		return fmt.Errorf("id_token, access_token, refresh_token, and account_id are required")
	}
	if credentials.LastRefresh.IsZero() {
		return fmt.Errorf("last_refresh is required")
	}
	return nil
}

func normalizeCodexSubscriptionCredentials(credentials *types.CodexAuthCredentials) {
	credentials.OpenAIAPIKey = nil
}

// @Summary List Codex subscriptions
// @Tags Codex
// @Produce json
// @Success 200 {array} types.CodexSubscription
// @Security BearerAuth
// @Router /api/v1/codex-subscriptions [get]
func (apiServer *HelixAPIServer) listCodexSubscriptions(_ http.ResponseWriter, req *http.Request) ([]*types.CodexSubscription, *system.HTTPError) {
	user := getRequestUser(req)
	if user == nil {
		return nil, system.NewHTTPError401("authentication required")
	}
	subs, err := apiServer.Store.ListCodexSubscriptions(req.Context(), user.ID)
	if err != nil {
		return nil, system.NewHTTPError500("failed to list subscriptions: " + err.Error())
	}
	memberships, err := apiServer.Store.ListOrganizationMemberships(req.Context(), &store.ListOrganizationMembershipsQuery{UserID: user.ID})
	if err == nil {
		for _, membership := range memberships {
			orgSubs, listErr := apiServer.Store.ListCodexSubscriptions(req.Context(), membership.OrganizationID)
			if listErr != nil {
				log.Warn().Err(listErr).Str("org_id", membership.OrganizationID).Msg("Failed to list org Codex subscriptions")
				continue
			}
			subs = append(subs, orgSubs...)
		}
	}
	return subs, nil
}

func (apiServer *HelixAPIServer) authorizeCodexSubscription(req *http.Request, id string) (*types.CodexSubscription, *system.HTTPError) {
	user := getRequestUser(req)
	if user == nil {
		return nil, system.NewHTTPError401("authentication required")
	}
	sub, err := apiServer.Store.GetCodexSubscription(req.Context(), id)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return nil, system.NewHTTPError404("subscription not found")
		}
		return nil, system.NewHTTPError500("failed to get subscription")
	}
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

// @Summary Get a Codex subscription
// @Tags Codex
// @Produce json
// @Param id path string true "Subscription ID"
// @Success 200 {object} types.CodexSubscription
// @Security BearerAuth
// @Router /api/v1/codex-subscriptions/{id} [get]
func (apiServer *HelixAPIServer) getCodexSubscription(_ http.ResponseWriter, req *http.Request) (*types.CodexSubscription, *system.HTTPError) {
	return apiServer.authorizeCodexSubscription(req, mux.Vars(req)["id"])
}

// @Summary Delete a Codex subscription
// @Tags Codex
// @Param id path string true "Subscription ID"
// @Success 200 {object} map[string]string
// @Security BearerAuth
// @Router /api/v1/codex-subscriptions/{id} [delete]
func (apiServer *HelixAPIServer) deleteCodexSubscription(_ http.ResponseWriter, req *http.Request) (map[string]string, *system.HTTPError) {
	id := mux.Vars(req)["id"]
	if _, httpErr := apiServer.authorizeCodexSubscription(req, id); httpErr != nil {
		return nil, httpErr
	}
	if err := apiServer.Store.DeleteCodexSubscription(req.Context(), id); err != nil {
		return nil, system.NewHTTPError500("failed to delete subscription")
	}
	return map[string]string{"status": "ok"}, nil
}

// @Summary Get Codex credentials for a session
// @Tags Codex
// @Produce json
// @Param id path string true "Session ID"
// @Success 200 {object} types.CodexAuthCredentials
// @Security ApiKeyAuth
// @Router /api/v1/sessions/{id}/codex-credentials [get]
func (apiServer *HelixAPIServer) getSessionCodexCredentials(_ http.ResponseWriter, req *http.Request) (*types.CodexAuthCredentials, *system.HTTPError) {
	session, err := apiServer.Store.GetSession(req.Context(), mux.Vars(req)["id"])
	if err != nil {
		return nil, system.NewHTTPError404("session not found")
	}
	user := getRequestUser(req)
	if user == nil || (user.TokenType != types.TokenTypeRunner && session.Owner != user.ID) {
		return nil, system.NewHTTPError403("access denied")
	}
	sub, err := apiServer.Store.GetEffectiveCodexSubscription(req.Context(), session.Owner, session.OrganizationID)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return nil, system.NewHTTPError404("no Codex subscription found for session owner")
		}
		return nil, system.NewHTTPError500("failed to get Codex subscription")
	}
	key, err := crypto.GetEncryptionKey()
	if err != nil {
		return nil, system.NewHTTPError500("failed to get encryption key")
	}
	plaintext, err := crypto.DecryptAES256GCM(sub.EncryptedCredentials, key)
	if err != nil {
		return nil, system.NewHTTPError500("failed to decrypt credentials")
	}
	var credentials types.CodexAuthCredentials
	if err := json.Unmarshal(plaintext, &credentials); err != nil {
		return nil, system.NewHTTPError500("failed to parse credentials")
	}
	normalizeCodexSubscriptionCredentials(&credentials)
	return &credentials, nil
}

// @Summary Update Codex credentials for a session
// @Description Persist credentials refreshed by Codex CLI. Stale refreshes are ignored.
// @Tags Codex
// @Accept json
// @Produce json
// @Param id path string true "Session ID"
// @Param body body types.CodexAuthCredentials true "Refreshed credentials"
// @Success 200 {object} map[string]string
// @Security ApiKeyAuth
// @Router /api/v1/sessions/{id}/codex-credentials [put]
func (apiServer *HelixAPIServer) updateSessionCodexCredentials(_ http.ResponseWriter, req *http.Request) (map[string]string, *system.HTTPError) {
	session, err := apiServer.Store.GetSession(req.Context(), mux.Vars(req)["id"])
	if err != nil {
		return nil, system.NewHTTPError404("session not found")
	}
	user := getRequestUser(req)
	if user == nil || (user.TokenType != types.TokenTypeRunner && session.Owner != user.ID) {
		return nil, system.NewHTTPError403("access denied")
	}
	credentials, err := decodeCodexCredentials(req.Body)
	if err != nil {
		return nil, system.NewHTTPError400(err.Error())
	}
	normalizeCodexSubscriptionCredentials(&credentials)
	sub, err := apiServer.Store.GetEffectiveCodexSubscription(req.Context(), session.Owner, session.OrganizationID)
	if err != nil {
		return nil, system.NewHTTPError404("no Codex subscription found for session owner")
	}
	if sub.LastRefreshedAt != nil && !credentials.LastRefresh.After(*sub.LastRefreshedAt) {
		return map[string]string{"status": "stale"}, nil
	}
	credentialJSON, err := json.Marshal(credentials)
	if err != nil {
		return nil, system.NewHTTPError500("failed to marshal credentials")
	}
	key, err := crypto.GetEncryptionKey()
	if err != nil {
		return nil, system.NewHTTPError500("failed to get encryption key")
	}
	encrypted, err := crypto.EncryptAES256GCM(credentialJSON, key)
	if err != nil {
		return nil, system.NewHTTPError500("failed to encrypt credentials")
	}
	updated, err := apiServer.Store.UpdateCodexSubscriptionCredentialsIfNewer(req.Context(), sub.ID, encrypted, credentials.Tokens.AccountID, credentials.LastRefresh)
	if err != nil {
		return nil, system.NewHTTPError500("failed to update subscription")
	}
	if !updated {
		return map[string]string{"status": "stale"}, nil
	}
	return map[string]string{"status": "ok"}, nil
}

type CodexLoginSessionResponse struct {
	SessionID string `json:"session_id"`
}

// @Summary Start a Codex login session
// @Description Launch a temporary container and start Codex device authentication
// @Tags Codex
// @Produce json
// @Success 200 {object} CodexLoginSessionResponse
// @Security BearerAuth
// @Router /api/v1/codex-subscriptions/start-login [post]
func (apiServer *HelixAPIServer) startCodexLogin(_ http.ResponseWriter, req *http.Request) (*CodexLoginSessionResponse, *system.HTTPError) {
	user := getRequestUser(req)
	if user == nil {
		return nil, system.NewHTTPError401("authentication required")
	}
	orgID := ""
	memberships, err := apiServer.Store.ListOrganizationMemberships(req.Context(), &store.ListOrganizationMembershipsQuery{UserID: user.ID})
	if err == nil && len(memberships) > 0 {
		orgID = memberships[0].OrganizationID
	}
	session, err := apiServer.Store.CreateSession(req.Context(), types.Session{
		ID: system.GenerateSessionID(), Name: "Codex Login", Created: time.Now(), Updated: time.Now(),
		Mode: types.SessionModeInference, Type: types.SessionTypeText, Provider: "openai", ModelName: "external_agent",
		Owner: user.ID, OwnerType: types.OwnerTypeUser, OrganizationID: orgID,
		Metadata: types.SessionMetadata{Stream: true, AgentType: "zed_external", SessionRole: "exploratory"},
	})
	if err != nil {
		return nil, system.NewHTTPError500("failed to create login session")
	}
	agent := &types.DesktopAgent{
		OrganizationID: orgID, SessionID: session.ID, UserID: user.ID, Input: "Codex login",
		ProjectPath: "workspace", DisplayWidth: 1280, DisplayHeight: 720, DesktopType: "ubuntu",
		Env: []string{"HELIX_SKIP_ZED=1"},
	}
	agent.OnBeforeCreate = func(ctx context.Context, desktopAgent *types.DesktopAgent) error {
		return apiServer.addUserAPITokenToAgent(ctx, desktopAgent, user.ID)
	}
	if _, err := apiServer.externalAgentExecutor.StartDesktop(req.Context(), agent); err != nil {
		return nil, system.NewHTTPError500("failed to start login desktop")
	}
	go apiServer.startCodexLoginCommand(session.ID)
	return &CodexLoginSessionResponse{SessionID: session.ID}, nil
}

func (apiServer *HelixAPIServer) startCodexLoginCommand(sessionID string) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	runnerID := "desktop-" + sessionID
	for ctx.Err() == nil {
		if err := apiServer.execBackgroundInContainer(ctx, runnerID, []string{"helix-codex-auth-wrapper"}); err == nil {
			return
		}
		time.Sleep(2 * time.Second)
	}
}

type CodexPollLoginResponse struct {
	Found bool   `json:"found"`
	URL   string `json:"url,omitempty"`
	Code  string `json:"code,omitempty"`
	Error string `json:"error,omitempty"`
}

// @Summary Poll Codex login
// @Description Return device authentication instructions or persist completed credentials
// @Tags Codex
// @Produce json
// @Param sessionId path string true "Session ID"
// @Success 200 {object} CodexPollLoginResponse
// @Security BearerAuth
// @Router /api/v1/codex-subscriptions/poll-login/{sessionId} [get]
func (apiServer *HelixAPIServer) pollCodexLogin(_ http.ResponseWriter, req *http.Request) (*CodexPollLoginResponse, *system.HTTPError) {
	user := getRequestUser(req)
	if user == nil {
		return nil, system.NewHTTPError401("authentication required")
	}
	sessionID := mux.Vars(req)["sessionId"]
	session, err := apiServer.Store.GetSession(req.Context(), sessionID)
	if err != nil || session.Owner != user.ID || session.Name != "Codex Login" {
		return nil, system.NewHTTPError404("login session not found")
	}
	runnerID := "desktop-" + sessionID
	authJSON, err := apiServer.execInContainer(req.Context(), runnerID, []string{"cat", "/home/retro/.codex/auth.json"})
	if err == nil && authJSON != "" {
		credentials, parseErr := decodeCodexCredentials(strings.NewReader(authJSON))
		if parseErr != nil {
			return &CodexPollLoginResponse{Error: parseErr.Error()}, nil
		}
		if _, createErr := apiServer.createCodexSubscriptionFromCredentials(req.Context(), user.ID, credentials); createErr != nil {
			return nil, system.NewHTTPError500("failed to persist Codex credentials")
		}
		return &CodexPollLoginResponse{Found: true}, nil
	}
	output, _ := apiServer.execInContainer(req.Context(), runnerID, []string{"cat", "/tmp/codex-auth-stdout.txt"})
	response := &CodexPollLoginResponse{}
	for _, line := range strings.Split(output, "\n") {
		clean := ansiEscapePattern.ReplaceAllString(strings.TrimSpace(strings.ReplaceAll(line, "\r", "")), "")
		if deviceURL := codexDeviceURLPattern.FindString(clean); deviceURL != "" {
			response.URL = deviceURL
		}
		if code := codexDeviceCodePattern.FindString(clean); code != "" {
			response.Code = code
		}
	}
	if errorOutput, errorErr := apiServer.execInContainer(req.Context(), runnerID, []string{"cat", "/tmp/codex-auth-error.txt"}); errorErr == nil {
		response.Error = strings.TrimSpace(errorOutput)
	}
	return response, nil
}

func (apiServer *HelixAPIServer) createCodexSubscriptionFromCredentials(ctx context.Context, userID string, credentials types.CodexAuthCredentials) (*types.CodexSubscription, error) {
	normalizeCodexSubscriptionCredentials(&credentials)
	data, err := json.Marshal(credentials)
	if err != nil {
		return nil, err
	}
	key, err := crypto.GetEncryptionKey()
	if err != nil {
		return nil, err
	}
	encrypted, err := crypto.EncryptAES256GCM(data, key)
	if err != nil {
		return nil, err
	}
	return apiServer.Store.CreateCodexSubscription(ctx, &types.CodexSubscription{
		OwnerID: userID, OwnerType: types.OwnerTypeUser, Name: "My Codex Subscription",
		EncryptedCredentials: encrypted, AccountID: credentials.Tokens.AccountID, AuthMode: credentials.AuthMode,
		Status: "active", LastRefreshedAt: &credentials.LastRefresh, CreatedBy: userID,
	})
}

func (apiServer *HelixAPIServer) execBackgroundInContainer(ctx context.Context, runnerID string, command []string) error {
	connection, err := apiServer.connman.Dial(ctx, runnerID)
	if err != nil {
		return err
	}
	defer connection.Close()
	body, _ := json.Marshal(map[string]interface{}{"command": command, "background": true})
	httpReq, err := http.NewRequest(http.MethodPost, "http://localhost:9876/exec", bytes.NewReader(body))
	if err != nil {
		return err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	if err := httpReq.Write(connection); err != nil {
		return err
	}
	response, err := http.ReadResponse(bufio.NewReader(connection), httpReq)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("exec returned status %d", response.StatusCode)
	}
	var result struct {
		Success bool   `json:"success"`
		Error   string `json:"error"`
	}
	if err := json.NewDecoder(response.Body).Decode(&result); err != nil {
		return fmt.Errorf("decode exec response: %w", err)
	}
	if !result.Success {
		return fmt.Errorf("start command: %s", result.Error)
	}
	return nil
}
