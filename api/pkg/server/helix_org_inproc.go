// Package server: helix_org_inproc.go provides the in-process adapter
// satisfying runtimehelix.ProjectService and runtimehelix.SpawnerClient
// for the embedded helix-org module.
//
// Both ports share one struct (`inProcHelixClient`) so a single
// instance can be wired into the WorkerProject.Service slot (project /
// git / app surface) and the Spawner.Client slot (chat session
// surface). The struct routes each call to the matching HelixAPIServer
// handler method by crafting an *http.Request, attaching the caller's
// *types.User to the context, and invoking the handler in-process —
// no HTTP loopback.
//
// Caller identity is resolved per request from runtimehelix's context
// stashes (HelixIdentity → UserIDFromContext / BearerFromContext); when
// none is present we fall back to the constructor-supplied service
// user (the bearer-forwarding equivalent the middleware path uses).
package server

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"

	"github.com/gorilla/mux"

	runtimehelix "github.com/helixml/helix/api/pkg/org/infrastructure/runtime/helix"
	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/types"
)

// inProcHelixClient satisfies both runtimehelix.ProjectService and
// runtimehelix.SpawnerClient by routing through the HelixAPIServer's
// handler methods in-process.
type inProcHelixClient struct {
	server      *HelixAPIServer
	serviceUser types.User
}

// NewInProcHelixClient constructs an inProcHelixClient. The serviceUser
// is the fallback identity used when the per-request context carries no
// HelixIdentity (e.g. background dispatcher activations before a bearer
// is minted via BearerForUser). Must be a real persisted user with the
// access rights needed for the operations the embedded helix-org issues
// (currently: project apply, git repo create, app update, session
// create / output / stop) — typically the auto-provisioned
// `helix-org-service` user.
func NewInProcHelixClient(s *HelixAPIServer, serviceUser *types.User) *inProcHelixClient {
	c := &inProcHelixClient{server: s}
	if serviceUser != nil {
		c.serviceUser = *serviceUser
	}
	return c
}

// resolveUser returns the *types.User to attach to a handler-bound
// request context. Priority: explicit *types.User stash > resolved
// UserID from HelixIdentity / WithUserID > serviceUser fallback. When
// the legacy stashes carry only an ID, look the user row up against
// the store so the resulting *types.User has the email/full-name
// fields handlers may read.
func (c *inProcHelixClient) resolveUser(ctx context.Context) (*types.User, error) {
	if u := runtimehelix.UserFromContext(ctx); u != nil {
		return u, nil
	}
	if uid := runtimehelix.UserIDFromContext(ctx); uid != "" {
		// Try the store for the full user row; fall back to a thin
		// User{} carrying just the ID so handlers that only need user.ID
		// still work.
		if user, err := c.server.Store.GetUser(ctx, &store.GetUserQuery{ID: uid}); err == nil && user != nil {
			return user, nil
		}
		return &types.User{ID: uid}, nil
	}
	if c.serviceUser.ID != "" {
		u := c.serviceUser
		return &u, nil
	}
	return nil, errors.New("inproc helix client: no user on context and no service user configured")
}

// newRequest builds an *http.Request whose body is the JSON encoding
// of body (or nil), context carries the resolved user via
// setRequestUser, and mux URL vars carry the supplied map (so handlers
// using mux.Vars(r) see the right ID).
func (c *inProcHelixClient) newRequest(ctx context.Context, method, path string, body any, vars map[string]string) (*http.Request, error) {
	user, err := c.resolveUser(ctx)
	if err != nil {
		return nil, err
	}
	var rdr *bytes.Reader
	if body != nil {
		buf, err := json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("marshal request body: %w", err)
		}
		rdr = bytes.NewReader(buf)
	} else {
		rdr = bytes.NewReader(nil)
	}
	req, err := http.NewRequestWithContext(ctx, method, path, rdr)
	if err != nil {
		return nil, err
	}
	req = req.WithContext(setRequestUser(req.Context(), *user))
	if len(vars) > 0 {
		req = mux.SetURLVars(req, vars)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	return req, nil
}

// ---- runtimehelix.ProjectService ----

// WhoAmI returns the authenticated user's ID. The HelixAPIServer's
// /api/v1/status endpoint surfaces this via Controller.GetStatus, but
// in-process we already have the resolved *types.User on the context
// (or the service-user fallback), so we just return its ID.
func (c *inProcHelixClient) WhoAmI(ctx context.Context) (string, error) {
	user, err := c.resolveUser(ctx)
	if err != nil {
		return "", err
	}
	return user.ID, nil
}

// ApplyProject upserts a project by name within the resolved user's
// organisation. Routes through HelixAPIServer.applyProject so the same
// idempotency / authorization rules apply as the public HTTP path.
func (c *inProcHelixClient) ApplyProject(ctx context.Context, req types.ProjectApplyRequest) (types.ProjectApplyResponse, error) {
	r, err := c.newRequest(ctx, http.MethodPut, "/api/v1/projects/apply", req, nil)
	if err != nil {
		return types.ProjectApplyResponse{}, err
	}
	resp, herr := c.server.applyProject(nil, r)
	if herr != nil {
		return types.ProjectApplyResponse{}, fmt.Errorf("apply project: %s", herr.Error())
	}
	if resp == nil {
		return types.ProjectApplyResponse{}, errors.New("apply project: nil response")
	}
	return *resp, nil
}

// GetProject returns a project by ID. Maps 404 → runtimehelix.ErrProjectNotFound
// so WorkerProject.Ensure's stale-pointer recovery path triggers correctly.
func (c *inProcHelixClient) GetProject(ctx context.Context, id string) (types.Project, error) {
	r, err := c.newRequest(ctx, http.MethodGet, "/api/v1/projects/"+id, nil, map[string]string{"id": id})
	if err != nil {
		return types.Project{}, err
	}
	resp, herr := c.server.getProject(nil, r)
	if herr != nil {
		if herr.StatusCode == http.StatusNotFound {
			return types.Project{}, fmt.Errorf("%w: %s", runtimehelix.ErrProjectNotFound, herr.Message)
		}
		return types.Project{}, fmt.Errorf("get project %s: %s", id, herr.Error())
	}
	if resp == nil {
		return types.Project{}, fmt.Errorf("%w: nil project", runtimehelix.ErrProjectNotFound)
	}
	return *resp, nil
}

// UpdateProject applies a partial patch to a project. Routes
// through HelixAPIServer.updateProject which only writes fields
// whose pointers are non-nil — same semantics as the public REST
// endpoint. Used by the helix-org runtime's ProjectConfig impl to
// back the configure_worker_project MCP tool.
func (c *inProcHelixClient) UpdateProject(ctx context.Context, id string, patch types.ProjectUpdateRequest) (types.Project, error) {
	r, err := c.newRequest(ctx, http.MethodPut, "/api/v1/projects/"+id, patch, map[string]string{"id": id})
	if err != nil {
		return types.Project{}, err
	}
	resp, herr := c.server.updateProject(nil, r)
	if herr != nil {
		if herr.StatusCode == http.StatusNotFound {
			return types.Project{}, fmt.Errorf("%w: %s", runtimehelix.ErrProjectNotFound, herr.Message)
		}
		return types.Project{}, fmt.Errorf("update project %s: %s", id, herr.Error())
	}
	if resp == nil {
		return types.Project{}, errors.New("update project: nil response")
	}
	return *resp, nil
}

// PutProjectSecret upserts a project-scoped secret.
//
// The underlying store does NOT upsert: store.CreateSecret rejects
// duplicates on (owner, name, project_id, app_id) with "already
// exists", and the public POST handler exposes that error verbatim
// (intentional — UI users editing secrets get duplicate-name form
// validation). Plain "POST create" therefore breaks the spawner,
// which runs on every activation and needs idempotent write +
// in-place value refresh so a rotated OAuth token actually
// propagates to the next session without re-hiring.
//
// So here we list-then-create-or-update:
//   - GET /api/v1/projects/<id>/secrets and find any secret named
//     `name`. listProjectSecrets strips Value (we only need the ID).
//   - If none, POST /api/v1/projects/<id>/secrets to create.
//   - If one exists, PUT /api/v1/secrets/<existing-id> to overwrite
//     the value in place. updateSecret preserves owner / project_id
//     / app_id so the row stays project-scoped.
//
// Race note: two concurrent activations for the same project +
// name could both see "no existing" and both POST — the second
// would hit the duplicate-name error. The spawner already logs
// (and tolerates) PutProjectSecret errors as best-effort, and
// activations for the same worker serialize at the spawner level,
// so the race window is theoretical for current callers.
func (c *inProcHelixClient) PutProjectSecret(ctx context.Context, projectID, name, value string) error {
	listReq, err := c.newRequest(ctx, http.MethodGet, "/api/v1/projects/"+projectID+"/secrets", nil, map[string]string{"id": projectID})
	if err != nil {
		return err
	}
	existing, herr := c.server.listProjectSecrets(nil, listReq)
	if herr != nil {
		return fmt.Errorf("put project secret (list): %s", herr.Error())
	}
	var existingID string
	for _, s := range existing {
		if s != nil && s.Name == name {
			existingID = s.ID
			break
		}
	}

	if existingID == "" {
		body := types.CreateSecretRequest{Name: name, Value: value}
		r, err := c.newRequest(ctx, http.MethodPost, "/api/v1/projects/"+projectID+"/secrets", body, map[string]string{"id": projectID})
		if err != nil {
			return err
		}
		if _, herr := c.server.createProjectSecret(nil, r); herr != nil {
			return fmt.Errorf("put project secret: %s", herr.Error())
		}
		return nil
	}

	// updateSecret decodes the body into a fresh types.Secret and
	// preserves Owner/ProjectID/AppID from the existing row — but NOT
	// Name. If we omit Name here it gets blanked to "", and
	// GetProjectSecretsAsEnvVars later emits an env var "=<value>",
	// which Docker rejects as `invalid environment variable: =<value>`
	// at container-create time. Always pass Name so the row keeps its
	// identity.
	updateBody := types.Secret{Name: name, Value: []byte(value)}
	r, err := c.newRequest(ctx, http.MethodPut, "/api/v1/secrets/"+existingID, updateBody, map[string]string{"id": existingID})
	if err != nil {
		return err
	}
	if _, herr := c.server.updateSecret(nil, r); herr != nil {
		return fmt.Errorf("put project secret (update): %s", herr.Error())
	}
	return nil
}

// ListProjectSecrets returns the project's dev-scoped secrets as a
// decrypted name→value map. Reuses GetProjectSecretsAsEnvVars (the same
// resolver the desktop-boot injection uses) so scope filtering and
// decryption stay in one place, then splits each `KEY=value` back into a
// map. Dev scope matches the desktop container's environment — the bot
// reads exactly what it would have had injected at boot.
func (c *inProcHelixClient) ListProjectSecrets(ctx context.Context, projectID string) (map[string]string, error) {
	envVars, err := c.server.GetProjectSecretsAsEnvVars(ctx, projectID, types.SecretScopeDev)
	if err != nil {
		return nil, err
	}
	return parseEnvVarsToMap(envVars), nil
}

// parseEnvVarsToMap splits `KEY=value` env-var strings back into a map.
// Cut on the FIRST `=` so a value that itself contains `=` (base64, a
// URL query, …) round-trips intact. Entries with no `=` or an empty name
// are skipped — GetProjectSecretsAsEnvVars never emits those, but the
// guard keeps a malformed entry from producing a `""` key.
func parseEnvVarsToMap(envVars []string) map[string]string {
	out := make(map[string]string, len(envVars))
	for _, kv := range envVars {
		name, value, found := strings.Cut(kv, "=")
		if !found || name == "" {
			continue
		}
		out[name] = value
	}
	return out
}

// CreateGitRepo creates an internal Helix git repository. The
// createGitRepository handler writes its response directly to the
// ResponseWriter (not the typed-handler shape), so we capture the
// response with httptest.NewRecorder and parse the JSON body.
func (c *inProcHelixClient) CreateGitRepo(ctx context.Context, req types.GitRepositoryCreateRequest) (types.GitRepository, error) {
	r, err := c.newRequest(ctx, http.MethodPost, "/api/v1/git/repositories", req, nil)
	if err != nil {
		return types.GitRepository{}, err
	}
	rec := httptest.NewRecorder()
	c.server.createGitRepository(rec, r)
	if rec.Code >= 400 {
		return types.GitRepository{}, fmt.Errorf("create git repo: %s: %s", rec.Result().Status, strings.TrimSpace(rec.Body.String()))
	}
	var repo types.GitRepository
	if err := json.Unmarshal(rec.Body.Bytes(), &repo); err != nil {
		return types.GitRepository{}, fmt.Errorf("decode git repo response: %w", err)
	}
	if repo.ID == "" {
		return types.GitRepository{}, errors.New("create git repo: empty id in response")
	}
	return repo, nil
}

// GetGitRepo returns a repo by ID, mapping a 404 to ErrRepoNotFound so the
// worker-project fast path can detect a deleted repo and re-provision.
func (c *inProcHelixClient) GetGitRepo(ctx context.Context, repoID string) (types.GitRepository, error) {
	r, err := c.newRequest(ctx, http.MethodGet, "/api/v1/git/repositories/"+repoID, nil, map[string]string{"id": repoID})
	if err != nil {
		return types.GitRepository{}, err
	}
	rec := httptest.NewRecorder()
	c.server.getGitRepository(rec, r)
	if rec.Code == http.StatusNotFound {
		return types.GitRepository{}, fmt.Errorf("%w: %s", runtimehelix.ErrRepoNotFound, strings.TrimSpace(rec.Body.String()))
	}
	if rec.Code >= 400 {
		return types.GitRepository{}, fmt.Errorf("get git repo %s: %s: %s", repoID, rec.Result().Status, strings.TrimSpace(rec.Body.String()))
	}
	var repo types.GitRepository
	if err := json.Unmarshal(rec.Body.Bytes(), &repo); err != nil {
		return types.GitRepository{}, fmt.Errorf("decode git repo response: %w", err)
	}
	return repo, nil
}

// DeleteGitRepo removes a repo by ID. A missing repo is treated as success
// (the goal is that it's gone).
func (c *inProcHelixClient) DeleteGitRepo(ctx context.Context, repoID string) error {
	r, err := c.newRequest(ctx, http.MethodDelete, "/api/v1/git/repositories/"+repoID, nil, map[string]string{"id": repoID})
	if err != nil {
		return err
	}
	rec := httptest.NewRecorder()
	c.server.deleteGitRepository(rec, r)
	if rec.Code == http.StatusNotFound {
		return nil
	}
	if rec.Code >= 400 {
		return fmt.Errorf("delete git repo %s: %s: %s", repoID, rec.Result().Status, strings.TrimSpace(rec.Body.String()))
	}
	return nil
}

// AttachRepoToProject attaches a repo to a project, optionally marking
// it primary. Two underlying handlers (attachRepositoryToProject +
// setProjectPrimaryRepository) are called in sequence to mirror the
// HTTP path's two-step interaction.
func (c *inProcHelixClient) AttachRepoToProject(ctx context.Context, projectID, repoID string, primary bool) error {
	vars := map[string]string{"id": projectID, "repo_id": repoID}
	attachReq, err := c.newRequest(ctx, http.MethodPut, "/api/v1/projects/"+projectID+"/repositories/"+repoID+"/attach", nil, vars)
	if err != nil {
		return err
	}
	if _, herr := c.server.attachRepositoryToProject(nil, attachReq); herr != nil {
		return fmt.Errorf("attach repo: %s", herr.Error())
	}
	if primary {
		primaryReq, err := c.newRequest(ctx, http.MethodPut, "/api/v1/projects/"+projectID+"/repositories/"+repoID+"/primary", nil, vars)
		if err != nil {
			return err
		}
		if _, herr := c.server.setProjectPrimaryRepository(nil, primaryReq); herr != nil {
			return fmt.Errorf("set primary repo: %s", herr.Error())
		}
	}
	return nil
}

// CreateBranch makes a new branch on a repo from baseBranch. The
// underlying handler uses the non-typed http.ResponseWriter shape, so
// we capture via httptest.NewRecorder.
func (c *inProcHelixClient) CreateBranch(ctx context.Context, repoID, branch, baseBranch string) error {
	body := types.CreateBranchRequest{BranchName: branch, BaseBranch: baseBranch}
	r, err := c.newRequest(ctx, http.MethodPost, "/api/v1/git/repositories/"+repoID+"/branches", body, map[string]string{"id": repoID})
	if err != nil {
		return err
	}
	rec := httptest.NewRecorder()
	c.server.createGitRepositoryBranch(rec, r)
	if rec.Code >= 400 {
		return fmt.Errorf("create branch %s on %s: %s: %s", branch, repoID, rec.Result().Status, strings.TrimSpace(rec.Body.String()))
	}
	return nil
}

// GetAppConfig returns the typed config for an App. Used by
// runtimehelix.AttachHelixOrgMCP to round-trip MCP entries.
func (c *inProcHelixClient) GetAppConfig(ctx context.Context, id string) (types.AppConfig, error) {
	r, err := c.newRequest(ctx, http.MethodGet, "/api/v1/apps/"+id, nil, map[string]string{"id": id})
	if err != nil {
		return types.AppConfig{}, err
	}
	app, herr := c.server.getApp(nil, r)
	if herr != nil {
		return types.AppConfig{}, fmt.Errorf("get app %s: %s", id, herr.Error())
	}
	if app == nil {
		return types.AppConfig{}, fmt.Errorf("get app %s: nil response", id)
	}
	return app.Config, nil
}

// UpdateAppConfig persists a mutated app config.
func (c *inProcHelixClient) UpdateAppConfig(ctx context.Context, id string, cfg types.AppConfig) error {
	// updateApp reads the existing app to preserve immutable fields, so
	// we only need to send {id, config}.
	body := types.App{
		ID:     id,
		Config: cfg,
	}
	r, err := c.newRequest(ctx, http.MethodPut, "/api/v1/apps/"+id, body, map[string]string{"id": id})
	if err != nil {
		return err
	}
	if _, herr := c.server.updateApp(nil, r); herr != nil {
		return fmt.Errorf("update app %s: %s", id, herr.Error())
	}
	return nil
}

// DeleteProject soft-deletes a Helix project and stops any sessions
// currently running against it. Used by the fire-worker cascade. A
// 404 from the underlying handler is mapped to ErrProjectNotFound so
// callers can treat already-gone projects as success.
func (c *inProcHelixClient) DeleteProject(ctx context.Context, id string) error {
	r, err := c.newRequest(ctx, http.MethodDelete, "/api/v1/projects/"+id, nil, map[string]string{"id": id})
	if err != nil {
		return err
	}
	if _, herr := c.server.deleteProject(nil, r); herr != nil {
		if herr.StatusCode == http.StatusNotFound {
			return fmt.Errorf("%w: %s", runtimehelix.ErrProjectNotFound, herr.Message)
		}
		return fmt.Errorf("delete project %s: %s", id, herr.Error())
	}
	return nil
}

// DeleteApp removes a Helix App. Used by the fire-worker cascade to
// clean up the per-Worker agent app that ApplyProject auto-
// provisioned. 404 maps to ErrProjectNotFound (the same "already
// gone" semantics; we re-use the sentinel rather than minting a
// second one for the cascade caller's sake).
func (c *inProcHelixClient) DeleteApp(ctx context.Context, id string) error {
	r, err := c.newRequest(ctx, http.MethodDelete, "/api/v1/apps/"+id, nil, map[string]string{"id": id})
	if err != nil {
		return err
	}
	if _, herr := c.server.deleteApp(nil, r); herr != nil {
		if herr.StatusCode == http.StatusNotFound {
			return fmt.Errorf("%w: %s", runtimehelix.ErrProjectNotFound, herr.Message)
		}
		return fmt.Errorf("delete app %s: %s", id, herr.Error())
	}
	return nil
}

// ---- runtimehelix.SpawnerClient ----

// ServerStatus returns the desktop-quota slice of /api/v1/config. We
// read directly from the same sources as
// HelixAPIServer.getConfig — the Free-tier quota env value and the
// in-memory active-desktop count.
func (c *inProcHelixClient) ServerStatus(ctx context.Context) (runtimehelix.ServerStatus, error) {
	st := runtimehelix.ServerStatus{
		MaxConcurrentDesktops: c.server.Cfg.SubscriptionQuotas.Projects.Free.MaxConcurrentDesktops,
	}
	if c.server.externalAgentExecutor != nil {
		st.ActiveConcurrentDesktops = len(c.server.externalAgentExecutor.ListSessions())
	}
	return st, nil
}

// GetOutput returns the latest output snapshot for a session.
func (c *inProcHelixClient) GetOutput(ctx context.Context, sessionID string) (types.SessionOutputResponse, error) {
	r, err := c.newRequest(ctx, http.MethodGet, "/api/v1/sessions/"+sessionID+"/output", nil, map[string]string{"id": sessionID})
	if err != nil {
		return types.SessionOutputResponse{}, err
	}
	resp, herr := c.server.getSessionOutput(nil, r)
	if herr != nil {
		if herr.StatusCode == http.StatusNotFound {
			return types.SessionOutputResponse{}, fmt.Errorf("%w: %s", runtimehelix.ErrSessionNotFound, sessionID)
		}
		return types.SessionOutputResponse{}, fmt.Errorf("get session output %s: %s", sessionID, herr.Error())
	}
	if resp == nil {
		return types.SessionOutputResponse{}, fmt.Errorf("get session output %s: nil response", sessionID)
	}
	return *resp, nil
}

// SessionOwner returns the user ID that owns the session. The
// transcript bridge needs it to subscribe to the correct per-session
// pubsub topic: helix publishes session updates to
// GetSessionQueue(session.Owner, …), so subscribing with an empty owner
// (or the wrong user) silently yields zero frames — only the spawner's
// own lifecycle markers reach the activation stream. Mirrors the owner
// lookup in websocket_server_user.go.
func (c *inProcHelixClient) SessionOwner(ctx context.Context, sessionID string) (string, error) {
	session, err := c.server.Store.GetSession(ctx, sessionID)
	if err != nil {
		return "", fmt.Errorf("get session %s: %w", sessionID, err)
	}
	if session == nil {
		return "", fmt.Errorf("get session %s: not found", sessionID)
	}
	return session.Owner, nil
}

// StopExternalAgent stops a session's external Zed agent.
func (c *inProcHelixClient) StopExternalAgent(ctx context.Context, sessionID string) error {
	r, err := c.newRequest(ctx, http.MethodDelete, "/api/v1/sessions/"+sessionID+"/stop-external-agent", nil, map[string]string{"id": sessionID})
	if err != nil {
		return err
	}
	if _, herr := c.server.stopExternalAgentSession(nil, r); herr != nil {
		return fmt.Errorf("stop external agent %s: %s", sessionID, herr.Error())
	}
	return nil
}

// StartSession creates the worker's session (+ desktop + queued first
// message) via the shared StartExternalAgentSession primitive the cron
// trigger uses. Non-blocking. session_role "exploratory" so the mirror's
// GetProjectExploratorySession lookup resolves it.
func (c *inProcHelixClient) StartSession(ctx context.Context, params runtimehelix.StartSessionParams) (string, error) {
	if params.Prompt == "" {
		return "", errors.New("StartSession: Prompt is required")
	}
	user, err := c.resolveUser(ctx)
	if err != nil {
		return "", err
	}
	req := &types.SessionChatRequest{
		ProjectID:      params.ProjectID,
		OrganizationID: params.OrganizationID,
		AppID:          params.AppID,
		AgentType:      params.AgentType,
		Provider:       types.Provider(params.Provider),
		Model:          params.Model,
		SessionRole:    "exploratory",
		// Org workers are fully autonomous — nobody is watching to click the
		// in-chat Restart button — so recover the agent automatically on crash.
		AutoRestartOnCrash: true,
		Messages: []*types.Message{{
			Role:    "user",
			Content: types.MessageContent{Parts: []any{params.Prompt}},
		}},
	}
	session, err := c.server.StartExternalAgentSession(ctx, req, user.ID)
	if err != nil {
		return "", fmt.Errorf("start external agent session: %w", err)
	}
	return session.ID, nil
}

// SendMessage dispatches a follow-up turn via the same REST handler the
// frontend / spec tasks use (POST /sessions/{id}/messages). Fire-and-
// forget; Helix auto-starts a downed desktop and delivers on reconnect.
func (c *inProcHelixClient) SendMessage(ctx context.Context, sessionID, prompt string) error {
	if sessionID == "" {
		return errors.New("SendMessage: sessionID is required")
	}
	body := SessionMessageRequest{Content: prompt}
	r, err := c.newRequest(ctx, http.MethodPost, "/api/v1/sessions/"+sessionID+"/messages", body, map[string]string{"id": sessionID})
	if err != nil {
		return err
	}
	if _, herr := c.server.sendSessionMessage(nil, r); herr != nil {
		return fmt.Errorf("send session message to %s: %s", sessionID, herr.Error())
	}
	return nil
}

// ClearSession wipes a session's conversation history — and, for a
// Zed/ACP external-agent session, resets the Zed thread — via the same
// handler the public POST /sessions/{id}/clear endpoint uses. The
// spawner calls this before every worker re-activation so each turn
// starts on a fresh context window instead of growing one long-lived
// session until it hits the model limit and compacts. Authorization is
// identical to SendMessage's path (authorizeUserToSession ActionUpdate),
// so the service/hiring user the activation already runs as is allowed.
func (c *inProcHelixClient) ClearSession(ctx context.Context, sessionID string) error {
	if sessionID == "" {
		return errors.New("ClearSession: sessionID is required")
	}
	r, err := c.newRequest(ctx, http.MethodPost, "/api/v1/sessions/"+sessionID+"/clear", nil, map[string]string{"id": sessionID})
	if err != nil {
		return err
	}
	if _, herr := c.server.clearSessionHandler(nil, r); herr != nil {
		return fmt.Errorf("clear session %s: %s", sessionID, herr.Error())
	}
	return nil
}

// DeleteSession removes a session row via the same DELETE /sessions/{id}
// handler the public API uses. Mirrors StopExternalAgent. Used by the
// bot-page "Restart agent session" reset: deleting the (exploratory)
// session is what makes the follow-up activation mint a brand-new one
// rather than reuse the singleton.
func (c *inProcHelixClient) DeleteSession(ctx context.Context, sessionID string) error {
	if sessionID == "" {
		return errors.New("DeleteSession: sessionID is required")
	}
	r, err := c.newRequest(ctx, http.MethodDelete, "/api/v1/sessions/"+sessionID, nil, map[string]string{"id": sessionID})
	if err != nil {
		return err
	}
	if _, herr := c.server.deleteSession(nil, r); herr != nil {
		return fmt.Errorf("delete session %s: %s", sessionID, herr.Error())
	}
	return nil
}

// Compile-time interface assertions — both ports must be satisfied by
// the same struct so a single instance can drive WorkerProject.Service
// AND SpawnerConfig.Client.
var (
	_ runtimehelix.ProjectService = (*inProcHelixClient)(nil)
	_ runtimehelix.SpawnerClient  = (*inProcHelixClient)(nil)
)
