package server

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/gorilla/mux"
	"github.com/helixml/helix/api/pkg/system"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/helixml/helix/api/pkg/vhost"
)

// MintPreviewTokenRequest is the body for POST
// /api/v1/sessions/:id/preview-tokens.
type MintPreviewTokenRequest struct {
	Port int `json:"port"`
}

// listSessionPreviewTokens godoc
// @Summary List session preview tokens
// @Tags Sessions
// @Produce json
// @Param id path string true "Session ID"
// @Success 200 {array} types.VHostRoute
// @Router /api/v1/sessions/{id}/preview-tokens [get]
// @Security BearerAuth
func (s *HelixAPIServer) listSessionPreviewTokens(_ http.ResponseWriter, r *http.Request) ([]*types.VHostRoute, *system.HTTPError) {
	user := getRequestUser(r)
	sessionID := mux.Vars(r)["id"]
	session, err := s.Store.GetSession(r.Context(), sessionID)
	if err != nil {
		return nil, system.NewHTTPError404("session not found")
	}
	if err := s.authorizeUserToSession(r.Context(), user, session, types.ActionGet); err != nil {
		return nil, system.NewHTTPError403(err.Error())
	}
	routes, err := s.Store.ListVHostRoutesByTarget(r.Context(), types.VHostTargetSandboxPreview, sessionID)
	if err != nil {
		return nil, system.NewHTTPError500(err.Error())
	}
	return routes, nil
}

// mintSessionPreviewToken godoc
// @Summary Mint a preview token for a session port
// @Description Mints a share-<adj>-<noun>-<8hex> hostname pointing at the session's container on the given port.
// @Tags Sessions
// @Accept json
// @Produce json
// @Param id path string true "Session ID"
// @Param body body MintPreviewTokenRequest true "Port to expose"
// @Success 200 {object} types.VHostRoute
// @Router /api/v1/sessions/{id}/preview-tokens [post]
// @Security BearerAuth
func (s *HelixAPIServer) mintSessionPreviewToken(_ http.ResponseWriter, r *http.Request) (*types.VHostRoute, *system.HTTPError) {
	user := getRequestUser(r)
	sessionID := mux.Vars(r)["id"]
	session, err := s.Store.GetSession(r.Context(), sessionID)
	if err != nil {
		return nil, system.NewHTTPError404("session not found")
	}
	if err := s.authorizeUserToSession(r.Context(), user, session, types.ActionUpdate); err != nil {
		return nil, system.NewHTTPError403(err.Error())
	}

	var req MintPreviewTokenRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		return nil, system.NewHTTPError400("invalid request body: " + err.Error())
	}
	if req.Port < 1 || req.Port > 65535 {
		return nil, system.NewHTTPError400("port must be 1..65535")
	}

	base := s.vhostBaseDomain()
	if base == "" {
		return nil, system.NewHTTPError400("preview tokens require DEV_SUBDOMAIN to be configured")
	}

	hostname, err := vhost.MintShareHostname(r.Context(), base, s.vhostReserveOpts(), 8)
	if err != nil {
		return nil, system.NewHTTPError500(err.Error())
	}

	now := time.Now()
	route := &types.VHostRoute{
		Hostname:   hostname,
		TargetKind: types.VHostTargetSandboxPreview,
		TargetID:   sessionID,
		Port:       req.Port,
		VerifiedAt: &now,
	}
	if err := s.Store.CreateVHostRoute(r.Context(), route); err != nil {
		return nil, system.NewHTTPError500(err.Error())
	}
	return route, nil
}

// rotateSessionPreviewToken godoc
// @Summary Rotate a session preview token hostname
// @Tags Sessions
// @Produce json
// @Param id path string true "Session ID"
// @Param token_id path string true "Token row ID"
// @Success 200 {object} types.VHostRoute
// @Router /api/v1/sessions/{id}/preview-tokens/{token_id}/rotate [post]
// @Security BearerAuth
func (s *HelixAPIServer) rotateSessionPreviewToken(_ http.ResponseWriter, r *http.Request) (*types.VHostRoute, *system.HTTPError) {
	user := getRequestUser(r)
	sessionID := mux.Vars(r)["id"]
	tokenID := mux.Vars(r)["token_id"]

	session, err := s.Store.GetSession(r.Context(), sessionID)
	if err != nil {
		return nil, system.NewHTTPError404("session not found")
	}
	if err := s.authorizeUserToSession(r.Context(), user, session, types.ActionUpdate); err != nil {
		return nil, system.NewHTTPError403(err.Error())
	}

	route, err := s.Store.GetVHostRouteByID(r.Context(), tokenID)
	if err != nil {
		return nil, system.NewHTTPError404("preview token not found")
	}
	if route.TargetKind != types.VHostTargetSandboxPreview || route.TargetID != sessionID {
		return nil, system.NewHTTPError404("preview token not found")
	}

	base := s.vhostBaseDomain()
	if base == "" {
		return nil, system.NewHTTPError400("preview tokens require DEV_SUBDOMAIN to be configured")
	}

	hostname, err := vhost.MintShareHostname(r.Context(), base, s.vhostReserveOpts(), 8)
	if err != nil {
		return nil, system.NewHTTPError500(err.Error())
	}
	if err := s.Store.RotateVHostRouteHostname(r.Context(), tokenID, hostname); err != nil {
		return nil, system.NewHTTPError500(err.Error())
	}
	updated, err := s.Store.GetVHostRouteByID(r.Context(), tokenID)
	if err != nil {
		return nil, system.NewHTTPError500(err.Error())
	}
	return updated, nil
}

// deleteSessionPreviewToken godoc
// @Summary Revoke a session preview token
// @Tags Sessions
// @Produce json
// @Param id path string true "Session ID"
// @Param token_id path string true "Token row ID"
// @Success 200 {object} map[string]bool
// @Router /api/v1/sessions/{id}/preview-tokens/{token_id} [delete]
// @Security BearerAuth
func (s *HelixAPIServer) deleteSessionPreviewToken(_ http.ResponseWriter, r *http.Request) (interface{}, *system.HTTPError) {
	user := getRequestUser(r)
	sessionID := mux.Vars(r)["id"]
	tokenID := mux.Vars(r)["token_id"]

	session, err := s.Store.GetSession(r.Context(), sessionID)
	if err != nil {
		return nil, system.NewHTTPError404("session not found")
	}
	if err := s.authorizeUserToSession(r.Context(), user, session, types.ActionUpdate); err != nil {
		return nil, system.NewHTTPError403(err.Error())
	}

	route, err := s.Store.GetVHostRouteByID(r.Context(), tokenID)
	if err != nil {
		return nil, system.NewHTTPError404("preview token not found")
	}
	if route.TargetKind != types.VHostTargetSandboxPreview || route.TargetID != sessionID {
		return nil, system.NewHTTPError404("preview token not found")
	}
	if err := s.Store.DeleteVHostRoute(r.Context(), tokenID); err != nil {
		return nil, system.NewHTTPError500(err.Error())
	}
	return map[string]bool{"deleted": true}, nil
}
