package server

import (
	"net/http"

	"github.com/gorilla/mux"
	"github.com/helixml/helix/api/pkg/types"
)

func (apiServer *HelixAPIServer) getSessionOperatorAccess(w http.ResponseWriter, r *http.Request) {
	user := getRequestUser(r)
	if user == nil {
		http.Error(w, "unauthenticated", http.StatusUnauthorized)
		return
	}

	ctx := r.Context()
	session, err := apiServer.Store.GetSession(ctx, mux.Vars(r)["id"])
	if err != nil {
		http.Error(w, "session not found", http.StatusNotFound)
		return
	}
	if err := apiServer.authorizeUserToSession(ctx, user, session, types.ActionUpdate); err != nil {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	if session.ProjectID != "" {
		if err := apiServer.authorizeUserToProjectByID(ctx, user, session.ProjectID, types.ActionUpdate); err != nil {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
	}

	w.WriteHeader(http.StatusNoContent)
}
