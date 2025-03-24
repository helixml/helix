package server

import (
	"net/http"

	"github.com/gorilla/mux"
	"github.com/helixml/helix/api/pkg/system"
	"github.com/helixml/helix/api/pkg/types"
)

// setupTemplateAppRoutes configures the API routes for template apps
func (s *HelixAPIServer) setupTemplateAppRoutes(r *mux.Router) {
	r.HandleFunc("/template_apps", system.DefaultWrapper(s.listTemplateApps)).Methods("GET")
	r.HandleFunc("/template_apps/{type}", system.DefaultWrapper(s.getTemplateApp)).Methods("GET")
}

// listTemplateApps returns all available template app configurations
func (s *HelixAPIServer) listTemplateApps(_ http.ResponseWriter, _ *http.Request) ([]*types.TemplateAppConfig, error) {
	return types.GetAppTemplates(), nil
}

// getTemplateApp returns a specific template app configuration by type
func (s *HelixAPIServer) getTemplateApp(_ http.ResponseWriter, r *http.Request) (*types.TemplateAppConfig, error) {
	vars := mux.Vars(r)
	templateType := types.TemplateAppType(vars["type"])

	template := types.GetTemplateByType(templateType)
	if template == nil {
		return nil, system.NewHTTPError404("template not found")
	}

	return template, nil
}
