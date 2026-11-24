package server

import (
	"net/http"

	"github.com/gorilla/mux"
	"github.com/rs/zerolog/log"

	"github.com/helixml/helix/api/pkg/server/linkpreview"
)

// linkPreviewsEnabled reports whether spec task link previews are served.
// They need SERVER_URL because Open Graph image URLs must be absolute.
func (apiServer *HelixAPIServer) linkPreviewsEnabled() bool {
	return apiServer.Cfg.WebServer.LinkPreviewsEnabled && apiServer.Cfg.WebServer.URL != ""
}

// linkPreviewHead returns the Open Graph tags for a spec task page, or "" for
// any other page or a task without a preview (the shell's generic tags stay).
func (apiServer *HelixAPIServer) linkPreviewHead(r *http.Request) string {
	if !apiServer.linkPreviewsEnabled() {
		return ""
	}
	projectID, taskID, ok := linkpreview.MatchTaskRoute(r.URL.Path)
	if !ok {
		return ""
	}
	p, err := linkpreview.Build(r.Context(), apiServer.Store, taskID, projectID)
	if err != nil {
		return ""
	}
	return linkpreview.MetaTags(p, apiServer.Cfg.WebServer.URL, r.URL.Path)
}

// serveSpecTaskPreviewImage renders the public Open Graph card for a task.
// Public by design: chat-app crawlers fetch it without cookies. It exposes only
// linkpreview.Preview's allow-listed fields, and every failure is the same 404.
func (apiServer *HelixAPIServer) serveSpecTaskPreviewImage(w http.ResponseWriter, r *http.Request) {
	if !apiServer.linkPreviewsEnabled() {
		http.NotFound(w, r)
		return
	}
	p, err := linkpreview.Build(r.Context(), apiServer.Store, mux.Vars(r)["taskId"], "")
	if err != nil {
		http.NotFound(w, r)
		return
	}
	img, err := linkpreview.RenderPNG(p)
	if err != nil {
		log.Error().Err(err).Str("task_id", p.TaskID).Msg("render link preview image")
		http.Error(w, "render failed", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "image/png")
	w.Header().Set("Cache-Control", "public, max-age=300")
	_, _ = w.Write(img)
}
