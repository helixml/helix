package server

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/gorilla/mux"
	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/system"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/helixml/helix/api/pkg/vhost"
	"github.com/helixml/helix/api/pkg/webservice"
	"github.com/rs/zerolog/log"
)

// DeployWebServiceRequest is the body for POST
// /api/v1/projects/:id/web-service/deploy. CommitSHA is optional; empty
// means "deploy current HEAD of the primary repo".
type DeployWebServiceRequest struct {
	CommitSHA string `json:"commit_sha,omitempty"`
}

// ProjectWebServiceResponse aggregates everything the UI needs to render
// the Web Service tab for a project.
type ProjectWebServiceResponse struct {
	State   *types.ProjectWebServiceState `json:"state"`
	Domains []*types.VHostRoute           `json:"domains"`
	Deploys []*types.WebServiceDeploy     `json:"deploys"`
}

// PutProjectWebServiceRequest is the body for PUT
// /api/v1/projects/:id/web-service. All fields are optional; omitted
// fields preserve their current value.
type PutProjectWebServiceRequest struct {
	Enabled       *bool `json:"enabled,omitempty"`
	ContainerPort *int  `json:"container_port,omitempty"`
}

// SetActiveSandboxRequest is the body for POST
// /api/v1/projects/:id/web-service/active-sandbox — the operator-driven
// "manual deploy" path. The auto-deploy-on-push orchestrator (when it
// lands in a follow-up PR) uses the same store primitive.
type SetActiveSandboxRequest struct {
	SandboxID string `json:"sandbox_id"`
}

// AddDomainRequest is the body for POST
// /api/v1/projects/:id/web-service/domains.
type AddDomainRequest struct {
	Hostname string `json:"hostname"`
}

// getProjectWebService godoc
// @Summary Get project web service state
// @Description Return enable/disable state, hostnames, and recent deploys for a project's web service.
// @Tags Projects
// @Produce json
// @Param id path string true "Project ID"
// @Success 200 {object} ProjectWebServiceResponse
// @Router /api/v1/projects/{id}/web-service [get]
// @Security BearerAuth
func (s *HelixAPIServer) getProjectWebService(_ http.ResponseWriter, r *http.Request) (*ProjectWebServiceResponse, *system.HTTPError) {
	project, herr := s.requireProjectAccess(r, types.ActionGet)
	if herr != nil {
		return nil, herr
	}

	state, err := s.Store.GetProjectWebServiceState(r.Context(), project.ID)
	if err != nil && !errors.Is(err, store.ErrNotFound) {
		return nil, system.NewHTTPError500(err.Error())
	}
	if state == nil {
		// Not yet configured — return a zero-valued state so the UI can
		// render the "off" toggle without a separate code path.
		state = &types.ProjectWebServiceState{ProjectID: project.ID}
	}

	domains, err := s.Store.ListVHostRoutesByTarget(r.Context(), types.VHostTargetProjectWebService, project.ID)
	if err != nil {
		return nil, system.NewHTTPError500(err.Error())
	}

	deploys, err := s.Store.ListWebServiceDeploys(r.Context(), project.ID, 20)
	if err != nil {
		return nil, system.NewHTTPError500(err.Error())
	}

	return &ProjectWebServiceResponse{
		State:   state,
		Domains: domains,
		Deploys: deploys,
	}, nil
}

// putProjectWebService godoc
// @Summary Update project web service state
// @Description Toggle web service enable/disable and update container_port. Enabling pre-seeds the default subdomain.
// @Tags Projects
// @Accept json
// @Produce json
// @Param id path string true "Project ID"
// @Param body body PutProjectWebServiceRequest true "Update request"
// @Success 200 {object} ProjectWebServiceResponse
// @Router /api/v1/projects/{id}/web-service [put]
// @Security BearerAuth
func (s *HelixAPIServer) putProjectWebService(_ http.ResponseWriter, r *http.Request) (*ProjectWebServiceResponse, *system.HTTPError) {
	project, herr := s.requireProjectAccess(r, types.ActionUpdate)
	if herr != nil {
		return nil, herr
	}

	var req PutProjectWebServiceRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		return nil, system.NewHTTPError400("invalid request body: " + err.Error())
	}

	current, err := s.Store.GetProjectWebServiceState(r.Context(), project.ID)
	if err != nil && !errors.Is(err, store.ErrNotFound) {
		return nil, system.NewHTTPError500(err.Error())
	}
	if current == nil {
		current = &types.ProjectWebServiceState{ProjectID: project.ID, ContainerPort: 8080}
	}

	wasEnabled := current.Enabled
	if req.Enabled != nil {
		current.Enabled = *req.Enabled
	}
	if req.ContainerPort != nil {
		if *req.ContainerPort < 1 || *req.ContainerPort > 65535 {
			return nil, system.NewHTTPError400("container_port must be 1..65535")
		}
		current.ContainerPort = *req.ContainerPort
	}
	if current.ContainerPort == 0 {
		current.ContainerPort = 8080
	}

	if err := s.Store.UpsertProjectWebServiceState(r.Context(), current); err != nil {
		return nil, system.NewHTTPError500(err.Error())
	}

	// Newly enabled → try to allocate a default subdomain.
	if !wasEnabled && current.Enabled {
		if base := s.vhostBaseDomain(); base != "" {
			hostname, allocErr := vhost.AllocateDefaultSubdomain(r.Context(), project.Name, base, s.vhostReserveOpts(), 50)
			if allocErr == nil {
				now := time.Now()
				route := &types.VHostRoute{
					Hostname:   hostname,
					TargetKind: types.VHostTargetProjectWebService,
					TargetID:   project.ID,
					Port:       current.ContainerPort,
					IsDefault:  true,
					VerifiedAt: &now,
				}
				if err := s.Store.CreateVHostRoute(r.Context(), route); err != nil {
					log.Warn().Err(err).Str("project_id", project.ID).Msg("failed to create default subdomain")
				}
			} else {
				log.Warn().Err(allocErr).Str("project_id", project.ID).Msg("could not allocate default subdomain")
			}
		}
	}

	// Newly disabled → tear down all routes for this project.
	if wasEnabled && !current.Enabled {
		if err := s.Store.DeleteVHostRoutesByTarget(r.Context(), types.VHostTargetProjectWebService, project.ID); err != nil {
			log.Warn().Err(err).Str("project_id", project.ID).Msg("failed to delete project vhost routes")
		}
	}

	return s.getProjectWebService(nil, r)
}

// setActiveWebServiceSandbox godoc
// @Summary Point a project web service at a sandbox
// @Description Manual deploy primitive — set the sandbox that vhost requests route to.
// @Tags Projects
// @Accept json
// @Produce json
// @Param id path string true "Project ID"
// @Param body body SetActiveSandboxRequest true "Active sandbox request"
// @Success 200 {object} types.ProjectWebServiceState
// @Router /api/v1/projects/{id}/web-service/active-sandbox [post]
// @Security BearerAuth
func (s *HelixAPIServer) setActiveWebServiceSandbox(_ http.ResponseWriter, r *http.Request) (*types.ProjectWebServiceState, *system.HTTPError) {
	project, herr := s.requireProjectAccess(r, types.ActionUpdate)
	if herr != nil {
		return nil, herr
	}
	var req SetActiveSandboxRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		return nil, system.NewHTTPError400("invalid request body: " + err.Error())
	}
	if req.SandboxID == "" {
		return nil, system.NewHTTPError400("sandbox_id is required")
	}
	// Sanity check the sandbox exists and the caller is allowed to use it.
	sb, err := s.Store.GetSandbox(r.Context(), req.SandboxID)
	if err != nil {
		return nil, system.NewHTTPError404("sandbox not found")
	}
	if sb.OrganizationID != project.OrganizationID {
		return nil, system.NewHTTPError403("sandbox is in a different organization")
	}

	if err := s.Store.SetActiveWebServiceSandbox(r.Context(), project.ID, req.SandboxID); err != nil {
		return nil, system.NewHTTPError500(err.Error())
	}

	// Record a manual-deploy row so the deploys list reflects it.
	now := time.Now()
	deploy := &types.WebServiceDeploy{
		ProjectID:  project.ID,
		SandboxID:  req.SandboxID,
		Status:     types.WebServiceDeployStatusLive,
		StartedAt:  now,
		FinishedAt: &now,
	}
	if err := s.Store.CreateWebServiceDeploy(r.Context(), deploy); err != nil {
		log.Warn().Err(err).Msg("failed to record manual deploy row")
	}

	state, err := s.Store.GetProjectWebServiceState(r.Context(), project.ID)
	if err != nil {
		return nil, system.NewHTTPError500(err.Error())
	}
	return state, nil
}

// deployProjectWebService godoc
// @Summary Trigger an auto-deploy of the project's web service
// @Description Provisions a fresh sandbox, clones the primary repo at the requested SHA, runs .helix/startup.sh, and cuts routing over once it's up.
// @Tags Projects
// @Accept json
// @Produce json
// @Param id path string true "Project ID"
// @Param body body DeployWebServiceRequest true "Deploy request"
// @Success 202 {object} types.WebServiceDeploy
// @Router /api/v1/projects/{id}/web-service/deploy [post]
// @Security BearerAuth
//
// deployProjectWebService triggers webservice.Controller.Redeploy. Returns
// the in-flight deploy row (status=pending/building); the actual
// provisioning + bootstrap + cutover runs asynchronously.
func (s *HelixAPIServer) deployProjectWebService(_ http.ResponseWriter, r *http.Request) (*types.WebServiceDeploy, *system.HTTPError) {
	user := getRequestUser(r)
	project, herr := s.requireProjectAccess(r, types.ActionUpdate)
	if herr != nil {
		return nil, herr
	}
	if s.webServiceController == nil {
		return nil, system.NewHTTPError500("web service controller not initialised")
	}

	var req DeployWebServiceRequest
	if r.ContentLength > 0 {
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			return nil, system.NewHTTPError400("invalid request body: " + err.Error())
		}
	}

	deploy, err := s.webServiceController.Redeploy(r.Context(), webservice.DeployRequest{
		ProjectID: project.ID,
		Owner:     user.ID,
		CommitSHA: req.CommitSHA,
	})
	if err != nil {
		return nil, system.NewHTTPError400(err.Error())
	}
	return deploy, nil
}

// addProjectWebServiceDomain godoc
// @Summary Add a custom domain to a project web service
// @Description Insert an unverified domain row. Verification happens out-of-band via the .well-known endpoint.
// @Tags Projects
// @Accept json
// @Produce json
// @Param id path string true "Project ID"
// @Param body body AddDomainRequest true "Domain to add"
// @Success 200 {object} types.VHostRoute
// @Router /api/v1/projects/{id}/web-service/domains [post]
// @Security BearerAuth
//
// addProjectWebServiceDomain registers a custom hostname for the project.
// Inserts the row in unverified state with a fresh verification_token.
// Verification (DNS resolves to us + token check) happens out-of-band
// via the cron poller and / or the /.well-known/helix-domain-verify
// endpoint.
func (s *HelixAPIServer) addProjectWebServiceDomain(_ http.ResponseWriter, r *http.Request) (*types.VHostRoute, *system.HTTPError) {
	project, herr := s.requireProjectAccess(r, types.ActionUpdate)
	if herr != nil {
		return nil, herr
	}
	var req AddDomainRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		return nil, system.NewHTTPError400("invalid request body: " + err.Error())
	}
	hostname := strings.ToLower(strings.TrimSpace(req.Hostname))
	if hostname == "" {
		return nil, system.NewHTTPError400("hostname is required")
	}

	opts := s.vhostReserveOpts()
	opts.Hostname = hostname
	if err := vhost.ReserveHostname(r.Context(), opts); err != nil {
		if errors.Is(err, vhost.ErrHostnameReserved) {
			return nil, system.NewHTTPError409(err.Error())
		}
		return nil, system.NewHTTPError500(err.Error())
	}

	state, err := s.Store.GetProjectWebServiceState(r.Context(), project.ID)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return nil, system.NewHTTPError400("web service not enabled for this project")
		}
		return nil, system.NewHTTPError500(err.Error())
	}

	token, err := generateVerificationToken()
	if err != nil {
		return nil, system.NewHTTPError500(err.Error())
	}

	route := &types.VHostRoute{
		Hostname:          hostname,
		TargetKind:        types.VHostTargetProjectWebService,
		TargetID:          project.ID,
		Port:              state.ContainerPort,
		IsDefault:         false,
		VerificationToken: token,
	}
	if err := s.Store.CreateVHostRoute(r.Context(), route); err != nil {
		return nil, system.NewHTTPError500(err.Error())
	}
	return route, nil
}

// deleteProjectWebServiceDomain godoc
// @Summary Remove a custom domain from a project web service
// @Tags Projects
// @Produce json
// @Param id path string true "Project ID"
// @Param domain_id path string true "Domain row ID"
// @Success 200 {object} map[string]bool
// @Router /api/v1/projects/{id}/web-service/domains/{domain_id} [delete]
// @Security BearerAuth
func (s *HelixAPIServer) deleteProjectWebServiceDomain(_ http.ResponseWriter, r *http.Request) (interface{}, *system.HTTPError) {
	project, herr := s.requireProjectAccess(r, types.ActionUpdate)
	if herr != nil {
		return nil, herr
	}
	domainID := mux.Vars(r)["domain_id"]
	if domainID == "" {
		return nil, system.NewHTTPError400("domain_id is required")
	}
	route, err := s.Store.GetVHostRouteByID(r.Context(), domainID)
	if err != nil {
		return nil, system.NewHTTPError404("domain not found")
	}
	if route.TargetKind != types.VHostTargetProjectWebService || route.TargetID != project.ID {
		return nil, system.NewHTTPError404("domain not found")
	}
	if err := s.Store.DeleteVHostRoute(r.Context(), domainID); err != nil {
		return nil, system.NewHTTPError500(err.Error())
	}
	return map[string]bool{"deleted": true}, nil
}

// domainVerificationResponse serves the well-known verification endpoint.
// Lives outside the auth router because the verifier must reach us
// from the public internet.
func (s *HelixAPIServer) domainVerificationResponse(w http.ResponseWriter, r *http.Request) {
	token := mux.Vars(r)["token"]
	if token == "" {
		http.Error(w, "missing token", http.StatusBadRequest)
		return
	}
	// We don't look the token up here — the caller (DNS resolver hitting
	// us) just needs the token echoed back. The verifier compares the
	// token in the response with the one stored on the route row.
	w.Header().Set("Content-Type", "text/plain")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(token))
}

// --- helpers ---

// requireProjectAccess loads the project from the URL and authorizes
// the caller.
func (s *HelixAPIServer) requireProjectAccess(r *http.Request, action types.Action) (*types.Project, *system.HTTPError) {
	user := getRequestUser(r)
	projectID := getID(r)
	project, err := s.Store.GetProject(r.Context(), projectID)
	if err != nil {
		return nil, system.NewHTTPError404("project not found")
	}
	if err := s.authorizeUserToProject(r.Context(), user, project, action); err != nil {
		return nil, system.NewHTTPError403(err.Error())
	}
	return project, nil
}

// vhostBaseDomain returns the configured DEV_SUBDOMAIN-derived base,
// or "" when vhost hosting is not configured.
func (s *HelixAPIServer) vhostBaseDomain() string {
	cfg := parseVHostConfig(s.Cfg.WebServer.DevSubdomain, s.Cfg.WebServer.URL)
	if !cfg.Enabled {
		return ""
	}
	return cfg.BaseDomain
}

// vhostReserveOpts assembles the standard ReserveHostname Options used
// by every code path that adds a row to vhost_routes.
func (s *HelixAPIServer) vhostReserveOpts() vhost.Options {
	return vhost.Options{
		CanonicalServerURL: s.Cfg.WebServer.URL,
		BaseDomain:         s.vhostBaseDomain(),
		Store:              s.Store,
	}
}

// generateVerificationToken returns a random URL-safe token used for
// domain ownership verification.
func generateVerificationToken() (string, error) {
	return system.GenerateID(), nil
}
