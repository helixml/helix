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

// listSessionPreviewTokens returns the currently active preview tokens
// for a session.
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

// mintSessionPreviewToken creates a new preview token row for a session
// + port. Returns the new route (so the UI gets the URL immediately).
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

// rotateSessionPreviewToken replaces the hostname on an existing token
// row (old URL stops working, new one takes effect).
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

// deleteSessionPreviewToken revokes a preview token.
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
