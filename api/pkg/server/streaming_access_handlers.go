package server

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/gorilla/mux"
	"github.com/helixml/helix/api/pkg/system"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/rs/zerolog/log"
)

// Streaming JWT claims
type StreamingTokenClaims struct {
	jwt.RegisteredClaims
	UserID       string `json:"user_id"`
	SessionID    string `json:"session_id"`
	WolfLobbyID  string `json:"wolf_lobby_id"`
	AccessLevel  string `json:"access_level"`
	GrantedVia   string `json:"granted_via"`
}

// @Summary Get streaming token
// @Description Generate time-limited token for streaming session access with RBAC
// @Tags Streaming
// @Accept json
// @Produce json
// @Param id path string true "Session ID"
// @Success 200 {object} types.StreamingTokenResponse
// @Failure 401 {object} system.HTTPError
// @Failure 403 {object} system.HTTPError
// @Failure 404 {object} system.HTTPError
// @Failure 500 {object} system.HTTPError
// @Security ApiKeyAuth
// @Router /api/v1/sessions/{id}/stream-token [get]
func (apiServer *HelixAPIServer) getSessionStreamingToken(_ http.ResponseWriter, req *http.Request) (*types.StreamingTokenResponse, *system.HTTPError) {
	ctx := req.Context()
	vars := mux.Vars(req)
	sessionID := vars["id"]
	user := getRequestUser(req)

	if user == nil {
		return nil, system.NewHTTPError401("authentication required")
	}

	// Get session
	session, err := apiServer.Store.GetSession(ctx, sessionID)
	if err != nil {
		log.Error().Err(err).Str("session_id", sessionID).Msg("Failed to get session")
		return nil, system.NewHTTPError404("session not found")
	}

	// Check access
	accessLevel, grantedVia, err := apiServer.checkStreamingAccess(ctx, user.ID, session.Owner, sessionID, "")
	if err != nil {
		log.Warn().
			Err(err).
			Str("user_id", user.ID).
			Str("session_id", sessionID).
			Msg("Streaming access denied")
		return nil, system.NewHTTPError403("access denied")
	}

	// Generate token
	token, err := apiServer.generateStreamingToken(user.ID, sessionID, session.Metadata.WolfLobbyPIN, accessLevel, grantedVia)
	if err != nil {
		log.Error().Err(err).Msg("Failed to generate streaming token")
		return nil, system.NewHTTPError500("failed to generate token")
	}

	// Log access
	auditLog := &types.StreamingAccessAuditLog{
		SessionID:    sessionID,
		UserID:       user.ID,
		AccessLevel:  accessLevel,
		AccessMethod: grantedVia,
		IPAddress:    req.RemoteAddr,
		UserAgent:    req.UserAgent(),
	}
	if err := apiServer.Store.LogStreamingAccess(ctx, auditLog); err != nil {
		log.Error().Err(err).Msg("Failed to log streaming access")
	}

	expiresAt := time.Now().Add(1 * time.Hour)

	return &types.StreamingTokenResponse{
		StreamToken:     token,
		WolfLobbyID:     session.Metadata.WolfLobbyID,
		WolfLobbyPIN:    session.Metadata.WolfLobbyPIN,
		MoonlightHostID: 0, // Wolf is always host 0 in moonlight-web
		MoonlightAppID:  1, // Default app ID
		AccessLevel:     accessLevel,
		ExpiresAt:       expiresAt,
	}, nil
}

// @Summary Get PDE streaming token
// @Description Generate time-limited token for PDE streaming access with RBAC
// @Tags PersonalDevEnvironments
// @Accept json
// @Produce json
// @Param id path string true "PDE ID"
// @Success 200 {object} types.StreamingTokenResponse
// @Failure 401 {object} system.HTTPError
// @Failure 403 {object} system.HTTPError
// @Failure 404 {object} system.HTTPError
// @Failure 500 {object} system.HTTPError
// @Security ApiKeyAuth
// @Router /api/v1/personal-dev-environments/{id}/stream-token [get]
func (apiServer *HelixAPIServer) getPDEStreamingToken(_ http.ResponseWriter, req *http.Request) (*types.StreamingTokenResponse, *system.HTTPError) {
	ctx := req.Context()
	vars := mux.Vars(req)
	pdeID := vars["id"]
	user := getRequestUser(req)

	if user == nil {
		return nil, system.NewHTTPError401("authentication required")
	}

	// Get PDE
	pde, err := apiServer.Store.GetPersonalDevEnvironment(ctx, pdeID)
	if err != nil {
		log.Error().Err(err).Str("pde_id", pdeID).Msg("Failed to get PDE")
		return nil, system.NewHTTPError404("PDE not found")
	}

	// Check access
	accessLevel, grantedVia, err := apiServer.checkStreamingAccess(ctx, user.ID, pde.UserID, "", pdeID)
	if err != nil {
		log.Warn().
			Err(err).
			Str("user_id", user.ID).
			Str("pde_id", pdeID).
			Msg("PDE streaming access denied")
		return nil, system.NewHTTPError403("access denied")
	}

	// Generate token
	token, err := apiServer.generateStreamingToken(user.ID, pdeID, pde.WolfLobbyPIN, accessLevel, grantedVia)
	if err != nil {
		log.Error().Err(err).Msg("Failed to generate streaming token")
		return nil, system.NewHTTPError500("failed to generate token")
	}

	// Log access
	auditLog := &types.StreamingAccessAuditLog{
		PDEID:        pdeID,
		WolfLobbyID:  pde.WolfLobbyID,
		UserID:       user.ID,
		AccessLevel:  accessLevel,
		AccessMethod: grantedVia,
		IPAddress:    req.RemoteAddr,
		UserAgent:    req.UserAgent(),
	}
	if err := apiServer.Store.LogStreamingAccess(ctx, auditLog); err != nil {
		log.Error().Err(err).Msg("Failed to log PDE streaming access")
	}

	expiresAt := time.Now().Add(1 * time.Hour)

	return &types.StreamingTokenResponse{
		StreamToken:     token,
		WolfLobbyID:     pde.WolfLobbyID,
		WolfLobbyPIN:    pde.WolfLobbyPIN,
		MoonlightHostID: 0,
		MoonlightAppID:  1,
		AccessLevel:     accessLevel,
		ExpiresAt:       expiresAt,
	}, nil
}

// checkStreamingAccess checks if user has access to stream a session/PDE
// Returns: accessLevel, grantedVia, error
func (apiServer *HelixAPIServer) checkStreamingAccess(ctx context.Context, userID, ownerID, sessionID, pdeID string) (string, string, error) {
	// Check 1: Is user the owner?
	if ownerID == userID {
		return "admin", "owner", nil
	}

	targetID := sessionID
	if pdeID != "" {
		targetID = pdeID
	}

	// Check 2: Direct user grant
	grant, err := apiServer.Store.GetStreamingAccessGrantByUser(ctx, targetID, userID)
	if err == nil && grant.RevokedAt == nil {
		if grant.ExpiresAt != nil && grant.ExpiresAt.Before(time.Now()) {
			return "", "", fmt.Errorf("grant expired")
		}
		return grant.AccessLevel, "user_grant", nil
	}

	// Check 3: Team grant
	// TODO: Implement GetUserTeams when team system is ready
	// userTeams, err := apiServer.Store.GetUserTeams(ctx, userID)
	// ...

	// Check 4: Role grant
	// TODO: Implement GetUserRoles when role system is ready
	// userRoles, err := apiServer.Store.GetUserRoles(ctx, userID)
	// ...

	return "", "", fmt.Errorf("no access to session/PDE")
}

// generateStreamingToken creates a JWT token for streaming access
func (apiServer *HelixAPIServer) generateStreamingToken(userID, sessionID, wolfLobbyPIN, accessLevel, grantedVia string) (string, error) {
	// Use runner token as JWT secret (or generate dedicated streaming secret in production)
	secret := []byte(apiServer.Cfg.WebServer.RunnerToken)

	claims := StreamingTokenClaims{
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   userID,
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(1 * time.Hour)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			Issuer:    "helix-streaming",
		},
		UserID:       userID,
		SessionID:    sessionID,
		WolfLobbyID:  wolfLobbyPIN, // Include lobby PIN in token
		AccessLevel:  accessLevel,
		GrantedVia:   grantedVia,
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString(secret)
}

// validateStreamingToken validates a streaming JWT token
func (apiServer *HelixAPIServer) validateStreamingToken(tokenString string) (*StreamingTokenClaims, error) {
	secret := []byte(apiServer.Cfg.WebServer.RunnerToken)

	token, err := jwt.ParseWithClaims(tokenString, &StreamingTokenClaims{}, func(token *jwt.Token) (interface{}, error) {
		return secret, nil
	})

	if err != nil {
		return nil, err
	}

	if claims, ok := token.Claims.(*StreamingTokenClaims); ok && token.Valid {
		return claims, nil
	}

	return nil, fmt.Errorf("invalid token")
}

// @Summary Create streaming access grant
// @Description Grant streaming access to another user/team/role
// @Tags Streaming
// @Accept json
// @Produce json
// @Param id path string true "Session ID"
// @Param request body types.StreamingAccessGrant true "Access grant details"
// @Success 200 {object} types.StreamingAccessGrant
// @Failure 400 {object} system.HTTPError
// @Failure 401 {object} system.HTTPError
// @Failure 403 {object} system.HTTPError
// @Failure 500 {object} system.HTTPError
// @Security ApiKeyAuth
// @Router /api/v1/sessions/{id}/streaming-access [post]
func (apiServer *HelixAPIServer) createSessionStreamingAccess(_ http.ResponseWriter, req *http.Request) (*types.StreamingAccessGrant, *system.HTTPError) {
	ctx := req.Context()
	vars := mux.Vars(req)
	sessionID := vars["id"]
	user := getRequestUser(req)

	if user == nil {
		return nil, system.NewHTTPError401("authentication required")
	}

	// Get session
	session, err := apiServer.Store.GetSession(ctx, sessionID)
	if err != nil {
		return nil, system.NewHTTPError404("session not found")
	}

	// Only owner or admin can grant access
	accessLevel, _, err := apiServer.checkStreamingAccess(ctx, user.ID, session.Owner, sessionID, "")
	if err != nil || accessLevel != "admin" {
		return nil, system.NewHTTPError403("only owner can grant access")
	}

	// Parse request
	var grantReq types.StreamingAccessGrant
	if err := json.NewDecoder(req.Body).Decode(&grantReq); err != nil {
		return nil, system.NewHTTPError400("invalid request")
	}

	// Set required fields
	grantReq.SessionID = sessionID
	grantReq.OwnerUserID = session.Owner
	grantReq.GrantedBy = user.ID

	// Create grant
	grant, err := apiServer.Store.CreateStreamingAccessGrant(ctx, &grantReq)
	if err != nil {
		log.Error().Err(err).Msg("Failed to create streaming access grant")
		return nil, system.NewHTTPError500("failed to create grant")
	}

	log.Info().
		Str("session_id", sessionID).
		Str("granted_to_user", grant.GrantedUserID).
		Str("access_level", grant.AccessLevel).
		Msg("Streaming access granted")

	return grant, nil
}

// @Summary List streaming access grants
// @Description List all active streaming access grants for a session
// @Tags Streaming
// @Accept json
// @Produce json
// @Param id path string true "Session ID"
// @Success 200 {array} types.StreamingAccessGrant
// @Failure 401 {object} system.HTTPError
// @Failure 403 {object} system.HTTPError
// @Failure 404 {object} system.HTTPError
// @Security ApiKeyAuth
// @Router /api/v1/sessions/{id}/streaming-access [get]
func (apiServer *HelixAPIServer) listSessionStreamingAccess(_ http.ResponseWriter, req *http.Request) ([]*types.StreamingAccessGrant, *system.HTTPError) {
	ctx := req.Context()
	vars := mux.Vars(req)
	sessionID := vars["id"]
	user := getRequestUser(req)

	if user == nil {
		return nil, system.NewHTTPError401("authentication required")
	}

	// Get session
	session, err := apiServer.Store.GetSession(ctx, sessionID)
	if err != nil {
		return nil, system.NewHTTPError404("session not found")
	}

	// Only owner or admin can list grants
	accessLevel, _, err := apiServer.checkStreamingAccess(ctx, user.ID, session.Owner, sessionID, "")
	if err != nil || accessLevel != "admin" {
		return nil, system.NewHTTPError403("only owner can view grants")
	}

	grants, err := apiServer.Store.ListStreamingAccessGrants(ctx, sessionID)
	if err != nil {
		log.Error().Err(err).Msg("Failed to list streaming access grants")
		return nil, system.NewHTTPError500("failed to list grants")
	}

	return grants, nil
}

// @Summary Revoke streaming access
// @Description Revoke a streaming access grant
// @Tags Streaming
// @Accept json
// @Produce json
// @Param id path string true "Session ID"
// @Param grant_id path string true "Grant ID"
// @Success 200 {object} map[string]string
// @Failure 401 {object} system.HTTPError
// @Failure 403 {object} system.HTTPError
// @Failure 404 {object} system.HTTPError
// @Security ApiKeyAuth
// @Router /api/v1/sessions/{id}/streaming-access/{grant_id} [delete]
func (apiServer *HelixAPIServer) revokeSessionStreamingAccess(_ http.ResponseWriter, req *http.Request) (map[string]string, *system.HTTPError) {
	ctx := req.Context()
	vars := mux.Vars(req)
	sessionID := vars["id"]
	grantID := vars["grant_id"]
	user := getRequestUser(req)

	if user == nil {
		return nil, system.NewHTTPError401("authentication required")
	}

	// Get grant
	grant, err := apiServer.Store.GetStreamingAccessGrant(ctx, grantID)
	if err != nil {
		return nil, system.NewHTTPError404("grant not found")
	}

	// Verify grant belongs to this session
	if grant.SessionID != sessionID && grant.PDEID != sessionID {
		return nil, system.NewHTTPError403("grant does not belong to this session")
	}

	// Only owner or grantor can revoke
	if grant.OwnerUserID != user.ID && grant.GrantedBy != user.ID {
		return nil, system.NewHTTPError403("only owner or grantor can revoke")
	}

	// Revoke grant
	if err := apiServer.Store.RevokeStreamingAccessGrant(ctx, grantID, user.ID); err != nil {
		log.Error().Err(err).Msg("Failed to revoke streaming access grant")
		return nil, system.NewHTTPError500("failed to revoke grant")
	}

	log.Info().
		Str("grant_id", grantID).
		Str("session_id", sessionID).
		Str("revoked_by", user.ID).
		Msg("Streaming access revoked")

	return map[string]string{"status": "revoked"}, nil
}
