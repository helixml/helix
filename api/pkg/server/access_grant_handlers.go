package server

import (
	"errors"
	"net/http"

	"github.com/gorilla/mux"

	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/types"
)

// listAppAccessGrants godoc
// @Summary List app access grants
// @Description List access grants for an app (organization owners and members can list access grants)
// @Tags    apps
// @Success 200 {array} types.AccessGrant
// @Router /api/v1/apps/{id}/access-grants [get]
// @Security BearerAuth
func (apiServer *HelixAPIServer) listAppAccessGrants(rw http.ResponseWriter, r *http.Request) {
	user := getRequestUser(r)
	appID := mux.Vars(r)["id"]

	app, err := apiServer.Store.GetApp(r.Context(), appID)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeErrResponse(rw, err, http.StatusNotFound)
			return
		}
		writeErrResponse(rw, err, http.StatusInternalServerError)
		return
	}

	if app.OrganizationID == "" {
		writeErrResponse(rw, errors.New("app is not associated with an organization"), http.StatusBadRequest)
		return
	}

	// Authorize user to view this application's access grants
	err = apiServer.authorizeUserToResource(r.Context(), user, app.OrganizationID, app.ID, types.ResourceApplication, types.ActionGet)
	if err != nil {
		writeErrResponse(rw, err, http.StatusForbidden)
		return
	}

	grants, err := apiServer.Store.ListAccessGrants(r.Context(), &store.ListAccessGrantsQuery{
		OrganizationID: app.OrganizationID,
		ResourceID:     app.ID,
	})
	if err != nil {
		writeErrResponse(rw, err, http.StatusInternalServerError)
		return
	}

	writeResponse(rw, grants, http.StatusOK)
}

func (apiServer *HelixAPIServer) createAppAccessGrant(rw http.ResponseWriter, r *http.Request) {
	user := getRequestUser(r)
	appID := mux.Vars(r)["id"]

	app, err := apiServer.Store.GetApp(r.Context(), appID)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeErrResponse(rw, err, http.StatusNotFound)
			return
		}
		writeErrResponse(rw, err, http.StatusInternalServerError)
		return
	}

	if app.OrganizationID == "" {
		writeErrResponse(rw, errors.New("app is not associated with an organization"), http.StatusBadRequest)
		return
	}

	// Authorize user to view this application's access grants
	err = apiServer.authorizeUserToResource(r.Context(), user, app.OrganizationID, app.ID, types.ResourceApplication, types.ActionGet)
	if err != nil {
		writeErrResponse(rw, err, http.StatusForbidden)
		return
	}

	grants, err := apiServer.Store.ListAccessGrants(r.Context(), &store.ListAccessGrantsQuery{
		OrganizationID: app.OrganizationID,
		ResourceID:     app.ID,
	})
	if err != nil {
		writeErrResponse(rw, err, http.StatusInternalServerError)
		return
	}

	writeResponse(rw, grants, http.StatusOK)
}
