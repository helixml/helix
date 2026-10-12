package server

import (
	"io"
	"net/http"
	"regexp"

	"github.com/gorilla/mux"
	"github.com/helixml/helix/api/pkg/types"
)

// visualizationIDPattern matches the ids minted by the html_render tool
// ("viz_" + lowercase ULID). It gates the filestore path so a request cannot
// traverse out of the session's visualizations folder.
var visualizationIDPattern = regexp.MustCompile(`^viz_[a-z0-9]{1,40}$`)

// getSessionVisualization serves an agent-published HTML visualization for a
// session, to be loaded inline in a sandboxed iframe by the frontend. Auth
// mirrors getSession (bearer / access_token cookie / access_token query), so an
// iframe carries the SPA's cookie. The page is served as inert HTML; isolation
// is enforced client-side by the iframe sandbox (no allow-same-origin).
//
// @Summary Get a session visualization
// @Description Serve an agent-published HTML visualization page for inline rendering
// @Tags    sessions
// @Produce text/html
// @Param id path string true "Session ID"
// @Param viz_id query string true "Visualization ID"
// @Success 200 {string} string "HTML document"
// @Router /api/v1/sessions/{id}/visualization [get]
// @Security BearerAuth
func (apiServer *HelixAPIServer) getSessionVisualization(rw http.ResponseWriter, req *http.Request) {
	sessionID := mux.Vars(req)["id"]
	vizID := req.URL.Query().Get("viz_id")
	if sessionID == "" || vizID == "" {
		http.Error(rw, "session id and visualization id are required", http.StatusBadRequest)
		return
	}
	if !visualizationIDPattern.MatchString(vizID) {
		http.Error(rw, "invalid visualization id", http.StatusBadRequest)
		return
	}

	ctx := req.Context()
	user := getRequestUser(req)

	session, err := apiServer.Store.GetSession(ctx, sessionID)
	if err != nil {
		http.Error(rw, err.Error(), http.StatusNotFound)
		return
	}
	if err := apiServer.authorizeUserToSession(ctx, user, session, types.ActionGet); err != nil {
		http.Error(rw, err.Error(), http.StatusForbidden)
		return
	}

	reader, err := apiServer.Controller.FilestoreVisualizationRead(ctx, session.Owner, sessionID, vizID)
	if err != nil {
		http.Error(rw, "visualization not found", http.StatusNotFound)
		return
	}
	defer reader.Close()

	// Defense in depth on top of the client-side iframe sandbox: refuse content
	// sniffing, forbid being framed by other origins' top-level pages only via
	// the app, and keep the page from reaching back into app storage. The page
	// itself is already bootstrap-injected and self-contained.
	rw.Header().Set("Content-Type", "text/html; charset=utf-8")
	rw.Header().Set("X-Content-Type-Options", "nosniff")
	rw.Header().Set("Content-Security-Policy", "sandbox allow-scripts allow-forms allow-popups; base-uri 'none'; form-action 'none'")
	rw.Header().Set("Cache-Control", "private, max-age=300")
	rw.WriteHeader(http.StatusOK)
	if _, err := io.Copy(rw, reader); err != nil {
		// Response already started; nothing to do but log via the request logger.
		return
	}
}
