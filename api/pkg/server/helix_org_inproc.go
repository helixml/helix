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
// stashes. Org-scoped background work uses the organization owner.
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
	"github.com/rs/zerolog/log"
	"gorm.io/gorm"

	"github.com/helixml/helix/api/pkg/hydra"
	"github.com/helixml/helix/api/pkg/org/application/configregistry"
	"github.com/helixml/helix/api/pkg/org/application/lifecycle"
	"github.com/helixml/helix/api/pkg/org/domain/orgchart"
	runtimehelix "github.com/helixml/helix/api/pkg/org/infrastructure/runtime/helix"
	helixorgserver "github.com/helixml/helix/api/pkg/org/interfaces/server"
	orgapi "github.com/helixml/helix/api/pkg/org/interfaces/server/api"
	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/types"
)

// inProcHelixClient satisfies both runtimehelix.ProjectService and
// runtimehelix.SpawnerClient by routing through the HelixAPIServer's
// handler methods in-process.
type inProcHelixClient struct {
	server  *HelixAPIServer
	configs *configregistry.Registry
}

func NewInProcHelixClient(s *HelixAPIServer, configs ...*configregistry.Registry) *inProcHelixClient {
	client := &inProcHelixClient{server: s}
	if len(configs) > 0 {
		client.configs = configs[0]
	}
	return client
}

func (c *inProcHelixClient) CreateAgent(ctx context.Context, orgID, name, instructions string, config lifecycle.AgentConfig) (string, error) {
	user, err := c.resolveUser(ctx)
	if err != nil {
		org, orgErr := c.server.lookupOrg(ctx, orgID)
		if orgErr != nil {
			return "", fmt.Errorf("resolve organization %s: %w", orgID, orgErr)
		}
		user, err = c.server.Store.GetUser(ctx, &store.GetUserQuery{ID: org.Owner})
		if err != nil {
			return "", fmt.Errorf("resolve owner %s for organization %s: %w", org.Owner, org.ID, err)
		}
		if user == nil {
			return "", fmt.Errorf("resolve owner %s for organization %s: user not found", org.Owner, org.ID)
		}
	}
	if user.ID == "" {
		return "", errors.New("create agent: user is missing")
	}
	if name == "" {
		return "", errors.New("create agent: name is missing")
	}
	assistant := types.AssistantConfig{
		Name:             name,
		SystemPrompt:     instructions,
		AgentType:        types.AgentTypeZedExternal,
		CodeAgentRuntime: types.CodeAgentRuntimeZedAgent,
	}
	if c.configs != nil && c.configs.IsDefaultAgentConfigured(ctx, orgID) {
		defaults, configErr := c.configs.GetDefaultAgentConfig(ctx, orgID)
		if configErr != nil {
			return "", fmt.Errorf("read default agent config: %w", configErr)
		}
		applyResolvedAgentDefaults(&assistant, defaults)
	}
	if config.CodeAgentRuntime != "" {
		applyResolvedAgentDefaults(&assistant, types.AssistantConfig{
			CodeAgentRuntime:        config.CodeAgentRuntime,
			CodeAgentCredentialType: config.CodeAgentCredentialType,
			Provider:                config.Provider,
			Model:                   config.Model,
			ReasoningEffort:         config.ReasoningEffort,
		})
	}
	if err := types.ValidateCodeAgentModelCompatibility(assistant); err != nil {
		return "", fmt.Errorf("create agent: %w", err)
	}
	app, err := c.server.Store.CreateApp(ctx, &types.App{
		Owner:          user.ID,
		OwnerType:      types.OwnerTypeUser,
		OrganizationID: orgID,
		AgentKind:      types.AgentKindOrg,
		Config: types.AppConfig{Helix: types.AppHelixConfig{
			Name:                 name,
			DefaultAgentType:     types.AgentTypeZedExternal,
			ExternalAgentEnabled: true,
			Assistants:           []types.AssistantConfig{assistant},
		}},
	})
	if err != nil {
		return "", err
	}
	return app.ID, nil
}

func (c *inProcHelixClient) ApplyAgentDefaults(ctx context.Context, appID string, defaults types.AssistantConfig) error {
	app, err := c.server.Store.GetApp(ctx, appID)
	if err != nil {
		return err
	}
	if len(app.Config.Helix.Assistants) != 1 {
		return errors.New("apply agent defaults: org-linked agent must contain exactly one assistant")
	}
	assistant := &app.Config.Helix.Assistants[0]
	if !isDeferredAgentScaffold(*assistant) {
		return nil
	}
	applyResolvedAgentDefaults(assistant, defaults)
	if err := types.ValidateCodeAgentModelCompatibility(*assistant); err != nil {
		return fmt.Errorf("apply agent defaults: %w", err)
	}
	_, err = c.server.Store.UpdateApp(ctx, app)
	return err
}

func isDeferredAgentScaffold(assistant types.AssistantConfig) bool {
	return assistant.AgentType == types.AgentTypeZedExternal &&
		assistant.CodeAgentRuntime == types.CodeAgentRuntimeZedAgent &&
		assistant.CodeAgentCredentialType == "" &&
		assistant.Provider == "" &&
		assistant.Model == "" &&
		(assistant.ReasoningEffort == "" || assistant.ReasoningEffort == types.ReasoningEffortNone) &&
		assistant.GenerationModelProvider == "" &&
		assistant.GenerationModel == ""
}

func applyResolvedAgentDefaults(assistant *types.AssistantConfig, defaults types.AssistantConfig) {
	runtime := defaults.CodeAgentRuntime
	if runtime == "" {
		runtime = types.CodeAgentRuntimeClaudeCode
	}
	credentials := defaults.CodeAgentCredentialType
	if credentials == "" {
		credentials = types.CodeAgentCredentialTypeSubscription
	}
	if !workerRuntimeSupportsSubscription(string(runtime)) {
		credentials = types.CodeAgentCredentialTypeAPIKey
	}
	assistant.CodeAgentRuntime = runtime
	assistant.CodeAgentCredentialType = credentials
	assistant.Provider = ""
	assistant.Model = ""
	assistant.GenerationModelProvider = ""
	assistant.GenerationModel = ""
	if credentials == types.CodeAgentCredentialTypeAPIKey {
		assistant.Provider = defaults.Provider
		assistant.Model = defaults.Model
		assistant.GenerationModelProvider = defaults.Provider
		assistant.GenerationModel = defaults.Model
	} else if runtime == types.CodeAgentRuntimeCodexCLI {
		assistant.Model = defaults.Model
	}
	assistant.ReasoningEffort = defaults.ReasoningEffort
}

func (c *inProcHelixClient) UpdateAgent(ctx context.Context, appID string, patch orgapi.AgentConfigPatch, name, instructions *string) error {
	user, err := c.resolveUser(ctx)
	if err != nil {
		return err
	}
	existing, err := c.server.Store.GetApp(ctx, appID)
	if err != nil {
		return err
	}
	if err := c.server.authorizeUserToApp(ctx, user, existing, types.ActionUpdate); err != nil {
		return err
	}
	update := existing
	if len(update.Config.Helix.Assistants) != 1 {
		return errors.New("update agent: org-linked agent must contain exactly one assistant")
	}
	assistant := &update.Config.Helix.Assistants[0]
	if name != nil {
		update.Config.Helix.Name = *name
		assistant.Name = *name
	}
	if instructions != nil {
		assistant.SystemPrompt = *instructions
	}
	if patch.CodeAgentRuntime != nil {
		assistant.CodeAgentRuntime = *patch.CodeAgentRuntime
	}
	if patch.CodeAgentCredentialType != nil {
		assistant.CodeAgentCredentialType = *patch.CodeAgentCredentialType
	}
	if patch.Provider != nil {
		assistant.Provider = *patch.Provider
	}
	if patch.Model != nil {
		assistant.Model = *patch.Model
	}
	if patch.ReasoningEffort != nil {
		assistant.ReasoningEffort = *patch.ReasoningEffort
	}
	if patch.CodeAgentCredentialType != nil || patch.Provider != nil || patch.Model != nil {
		if assistant.CodeAgentCredentialType == types.CodeAgentCredentialTypeAPIKey {
			assistant.GenerationModelProvider = assistant.Provider
			assistant.GenerationModel = assistant.Model
		} else {
			assistant.GenerationModelProvider = ""
			assistant.GenerationModel = ""
		}
	}
	r, err := c.newRequest(ctx, http.MethodPut, "/api/v1/agents/"+appID, update, map[string]string{"id": appID})
	if err != nil {
		return err
	}
	if _, herr := c.server.updateAgent(nil, r); herr != nil {
		return errors.New(herr.Error())
	}
	return nil
}

func (c *inProcHelixClient) UpdateAgentContent(ctx context.Context, appID, content string) error {
	return c.UpdateAgent(ctx, appID, orgapi.AgentConfigPatch{}, nil, &content)
}

func (c *inProcHelixClient) ReadAgent(ctx context.Context, appID string) (orgapi.AgentProfile, error) {
	app, err := c.server.Store.GetApp(ctx, appID)
	if err != nil {
		return orgapi.AgentProfile{}, err
	}
	if len(app.Config.Helix.Assistants) != 1 {
		return orgapi.AgentProfile{}, fmt.Errorf("%w: org-linked agent app must contain exactly one assistant", orgapi.ErrInvalidAgentProfile)
	}
	assistant := app.Config.Helix.Assistants[0]
	name := assistant.Name
	if name == "" {
		name = app.Config.Helix.Name
	}
	return orgapi.AgentProfile{
		Name:                    name,
		Instructions:            assistant.SystemPrompt,
		CodeAgentRuntime:        assistant.CodeAgentRuntime,
		CodeAgentCredentialType: assistant.CodeAgentCredentialType,
		Provider:                assistant.Provider,
		Model:                   assistant.Model,
		ReasoningEffort:         assistant.ReasoningEffort,
	}, nil
}

func (c *inProcHelixClient) AgentProfile(ctx context.Context, appID string) (string, string, error) {
	profile, err := c.ReadAgent(ctx, appID)
	if err != nil {
		return "", "", err
	}
	return profile.Name, profile.Instructions, nil
}

// resolveUser returns the *types.User to attach to a handler-bound
// request context. Explicit users win; org-scoped background work runs
// as the organization owner.
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
	orgID := runtimehelix.OrganizationIDFromContext(ctx)
	if orgID == "" {
		orgID = helixorgserver.OrgIDFromContext(ctx)
	}
	if orgID == "" {
		return nil, errors.New("inproc helix client: no user or organization on context")
	}
	org, err := c.server.lookupOrg(ctx, orgID)
	if err != nil {
		return nil, fmt.Errorf("resolve organization %s: %w", orgID, err)
	}
	owner, err := c.server.Store.GetUser(ctx, &store.GetUserQuery{ID: org.Owner})
	if err != nil {
		return nil, fmt.Errorf("resolve owner %s for organization %s: %w", org.Owner, org.ID, err)
	}
	if owner == nil {
		return nil, fmt.Errorf("resolve owner %s for organization %s: user not found", org.Owner, org.ID)
	}
	return owner, nil
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
	// applyProject classifies the agent app it creates/links as a coding agent
	// (the classification every non-org caller wants). A bot's agent belongs to
	// the org graph instead, so reclassify here rather than in the shared
	// handler — that keeps the public apply endpoint from silently converting a
	// caller's coding agent into an org agent. Also repairs bots whose apps
	// predate agent_kind.
	if resp.AgentAppID != "" {
		if err := c.markAgentAppAsOrgKind(ctx, resp.AgentAppID); err != nil {
			return types.ProjectApplyResponse{}, err
		}
	}
	return *resp, nil
}

// markAgentAppAsOrgKind flips an agent app to org_agent so it is excluded from
// the coding-agent surfaces (spec-task selectors, project agent configuration,
// the Apps "Coding Agents" tab) that org bots must not appear in.
func (c *inProcHelixClient) markAgentAppAsOrgKind(ctx context.Context, appID string) error {
	app, err := c.server.Store.GetApp(ctx, appID)
	if err != nil {
		return fmt.Errorf("get agent app %s: %w", appID, err)
	}
	if app.AgentKind == types.AgentKindOrg {
		return nil
	}
	app.AgentKind = types.AgentKindOrg
	if _, err := c.server.Store.UpdateApp(ctx, app); err != nil {
		return fmt.Errorf("classify agent app %s as org agent: %w", appID, err)
	}
	return nil
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

// DeleteProjectSecret removes a named project secret when present. It is used
// to migrate worker identity out of project scope; absence is already the
// desired state and is therefore idempotent.
func (c *inProcHelixClient) DeleteProjectSecret(ctx context.Context, projectID, name string) error {
	listReq, err := c.newRequest(ctx, http.MethodGet, "/api/v1/projects/"+projectID+"/secrets", nil, map[string]string{"id": projectID})
	if err != nil {
		return err
	}
	existing, herr := c.server.listProjectSecrets(nil, listReq)
	if herr != nil {
		return fmt.Errorf("delete project secret (list): %s", herr.Error())
	}
	for _, secret := range existing {
		if secret == nil || secret.Name != name {
			continue
		}
		r, err := c.newRequest(ctx, http.MethodDelete, "/api/v1/secrets/"+secret.ID, nil, map[string]string{"id": secret.ID})
		if err != nil {
			return err
		}
		if _, herr := c.server.deleteSecret(nil, r); herr != nil {
			return fmt.Errorf("delete project secret: %s", herr.Error())
		}
		return nil
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

// GetAppConfig returns the typed config for an App.
func (c *inProcHelixClient) GetAppConfig(ctx context.Context, id string) (types.AppConfig, error) {
	r, err := c.newRequest(ctx, http.MethodGet, "/api/v1/agents/"+id, nil, map[string]string{"id": id})
	if err != nil {
		return types.AppConfig{}, err
	}
	app, herr := c.server.getAgent(nil, r)
	if herr != nil {
		if herr.StatusCode == http.StatusNotFound {
			return types.AppConfig{}, fmt.Errorf("get app %s: %w", id, store.ErrNotFound)
		}
		return types.AppConfig{}, fmt.Errorf("get app %s: %s", id, herr.Error())
	}
	if app == nil {
		return types.AppConfig{}, fmt.Errorf("get app %s: nil response", id)
	}
	return app.Config, nil
}

func (c *inProcHelixClient) GetApp(ctx context.Context, id string) (*types.App, error) {
	return c.server.Store.GetApp(ctx, id)
}

// UpdateAppConfig persists a mutated app config.
func (c *inProcHelixClient) UpdateAppConfig(ctx context.Context, id string, cfg types.AppConfig) error {
	app, err := c.server.Store.GetApp(ctx, id)
	if err != nil {
		return fmt.Errorf("get app %s: %w", id, err)
	}
	app.Config = cfg
	if _, err := c.server.Store.UpdateApp(ctx, app); err != nil {
		return fmt.Errorf("update app %s: %w", id, err)
	}
	return nil
}

func (s *HelixAPIServer) publishAgentToolChange(ctx context.Context, appID string) {
	app, err := s.Store.GetApp(ctx, appID)
	if err != nil {
		log.Warn().Err(err).Str("app_id", appID).Msg("tool change: failed to get linked agent")
		return
	}
	sessions, _, err := s.Store.ListSessions(ctx, store.ListSessionsQuery{
		Owner:                 app.Owner,
		OwnerType:             app.OwnerType,
		OrganizationID:        app.OrganizationID,
		AppID:                 appID,
		IncludeExternalAgents: true,
	})
	if err != nil {
		log.Warn().Err(err).Str("app_id", appID).Msg("tool change: failed to list linked sessions")
		return
	}
	for _, session := range sessions {
		s.publishAgentConfigChange(ctx, session, "tools")
	}
}

// DeleteProject soft-deletes a Helix project and stops any sessions
// currently running against it. Used by the fire-worker cascade. A
// 404 from the underlying handler is mapped to ErrProjectNotFound so
// callers can treat already-gone projects as success.
func (c *inProcHelixClient) DeleteProject(ctx context.Context, id string) error {
	project, err := c.server.Store.GetProject(ctx, id)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) || errors.Is(err, gorm.ErrRecordNotFound) {
			return fmt.Errorf("%w: project %s", runtimehelix.ErrProjectNotFound, id)
		}
		return err
	}
	deleteCtx := ctx
	if project.OrganizationID != "" {
		org, err := c.server.Store.GetOrganization(ctx, &store.GetOrganizationQuery{ID: project.OrganizationID})
		if err != nil {
			return fmt.Errorf("resolve project organization %s: %w", project.OrganizationID, err)
		}
		owner, err := c.server.Store.GetUser(ctx, &store.GetUserQuery{ID: org.Owner})
		if err != nil {
			return fmt.Errorf("resolve project organization owner %s: %w", org.Owner, err)
		}
		deleteCtx = runtimehelix.WithUser(ctx, owner)
	}
	r, err := c.newRequest(deleteCtx, http.MethodDelete, "/api/v1/projects/"+id, nil, map[string]string{"id": id})
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
	if _, err := c.server.Store.GetApp(ctx, id); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return fmt.Errorf("%w: app %s", runtimehelix.ErrProjectNotFound, id)
		}
		return err
	}
	return c.server.deleteAppData(ctx, id, false)
}

func (c *inProcHelixClient) DeleteLinkedAgent(ctx context.Context, orgID string, botID orgchart.NodeID, appID, sessionID string) error {
	if sessionID != "" {
		// Best-effort: stopping the desktop is a courtesy teardown, not a
		// precondition for deleting the bot. stopExternalAgentSession 404s on an
		// already-deleted session, 400s when the session isn't zed_external, and
		// 500s when hydra is unreachable — none of which should leave the bot
		// permanently undeletable. The container is reaped by its own lifecycle
		// either way.
		if err := c.StopExternalAgent(ctx, sessionID); err != nil {
			log.Warn().
				Err(err).
				Str("session_id", sessionID).
				Str("bot_id", string(botID)).
				Msg("failed to stop linked agent session; continuing with delete")
		}
	}
	accessor, ok := c.server.Store.(interface{ GormDB() *gorm.DB })
	if !ok {
		return fmt.Errorf("delete linked agent: store %T has no shared database", c.server.Store)
	}
	return accessor.GormDB().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// The lifecycle archives the runtime-owned project first. Detach the app
		// from any other configured projects without touching their repositories.
		if err := tx.Model(&types.Project{}).
			Where("default_helix_app_id = ?", appID).
			Update("default_helix_app_id", "").Error; err != nil {
			return fmt.Errorf("detach agent from projects: %w", err)
		}
		knowledgeIDs := tx.Model(&types.Knowledge{}).Select("id").Where("app_id = ?", appID)
		if err := tx.Where("knowledge_id IN (?)", knowledgeIDs).Delete(&types.KnowledgeVersion{}).Error; err != nil {
			return fmt.Errorf("delete knowledge versions: %w", err)
		}
		if err := tx.Where("app_id = ?", appID).Delete(&types.Knowledge{}).Error; err != nil {
			return fmt.Errorf("delete knowledge: %w", err)
		}
		if err := tx.Exec(
			"DELETE FROM org_bot_runtime_state WHERE org_id = ? AND bot_id = ?",
			orgID, string(botID),
		).Error; err != nil {
			return fmt.Errorf("delete runtime state: %w", err)
		}
		if err := tx.Exec(
			"DELETE FROM org_subscriptions WHERE org_id = ? AND bot_id = ?",
			orgID, string(botID),
		).Error; err != nil {
			return fmt.Errorf("delete subscriptions: %w", err)
		}
		result := tx.Exec(
			"DELETE FROM org_bots WHERE org_id = ? AND id = ? AND agent_app_id = ?",
			orgID, string(botID), appID,
		)
		if result.Error != nil {
			return fmt.Errorf("delete bot: %w", result.Error)
		}
		if result.RowsAffected != 1 {
			return fmt.Errorf("delete bot: linked graph node not found")
		}
		if err := tx.Delete(&types.App{ID: appID}).Error; err != nil {
			return fmt.Errorf("delete app: %w", err)
		}
		return nil
	})
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
		AutoRestartOnCrash:  true,
		OrgWorkerID:         params.WorkerID,
		RuntimeInstructions: params.Instructions,
		Messages: []*types.Message{{
			Role:    "user",
			Content: types.MessageContent{Parts: []any{params.Prompt}},
		}},
	}
	session, err := c.server.StartExternalAgentSession(ctx, req, user.ID)
	if err != nil {
		return "", fmt.Errorf("start external agent session: %w", err)
	}
	if params.Name != "" && session.Name != params.Name {
		session.Name = params.Name
		if _, err := c.server.Store.UpdateSession(ctx, *session); err != nil {
			return "", fmt.Errorf("name external agent session: %w", err)
		}
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

// SyncAgentProfile refreshes the display name on every existing session and
// the instruction files read natively by Codex and Claude on running desktops.
// The spawner calls this before clearing the existing ACP thread.
func (c *inProcHelixClient) SyncAgentProfile(ctx context.Context, sessionID, sessionName, workerID, instructions string) error {
	if sessionID == "" {
		return errors.New("SyncAgentProfile: sessionID is required")
	}
	session, err := c.server.Store.GetSession(ctx, sessionID)
	if err != nil {
		return fmt.Errorf("get session %s: %w", sessionID, err)
	}
	changed := false
	if sessionName != "" && session.Name != sessionName {
		session.Name = sessionName
		changed = true
	}
	if session.Metadata.OrgWorkerID != workerID || session.Metadata.RuntimeInstructions != instructions {
		session.Metadata.OrgWorkerID = workerID
		session.Metadata.RuntimeInstructions = instructions
		changed = true
	}
	if session.ParentApp != "" {
		app, appErr := c.server.Store.GetApp(ctx, session.ParentApp)
		if appErr != nil {
			return fmt.Errorf("get agent app %s for session %s: %w", session.ParentApp, sessionID, appErr)
		}
		runtime, modelName, ok := currentAgentInfo(app, session.Metadata.AssistantID)
		if ok && (session.Metadata.CodeAgentRuntime != runtime || session.Metadata.ZedAgentName != runtime.ZedAgentName() || session.ModelName != modelName) {
			session.Metadata.CodeAgentRuntime = runtime
			session.Metadata.ZedAgentName = runtime.ZedAgentName()
			session.ModelName = modelName
			changed = true
		}
	}
	if changed {
		if _, err := c.server.Store.UpdateSession(ctx, *session); err != nil {
			return fmt.Errorf("update agent profile for session %s: %w", sessionID, err)
		}
	}
	if c.server.externalAgentExecutor == nil {
		return errors.New("SyncAgentProfile: external agent executor is not configured")
	}
	if !c.server.externalAgentExecutor.HasRunningContainer(ctx, sessionID) {
		return nil
	}
	runtimeSession, err := c.server.externalAgentExecutor.GetSession(sessionID)
	if err != nil {
		return fmt.Errorf("get running desktop session %s: %w", sessionID, err)
	}
	sandboxID := runtimeSession.SandboxID
	if sandboxID == "" {
		sandboxID = "local"
	}
	hydraClient := hydra.NewRevDialClient(c.server.connman, "hydra-"+sandboxID)
	for _, path := range []string{"/home/retro/work/AGENTS.md", "/home/retro/work/CLAUDE.md"} {
		if err := hydraClient.WriteSandboxFile(ctx, sessionID, path, []byte(instructions), 0o644); err != nil {
			return fmt.Errorf("write %s for session %s: %w", path, sessionID, err)
		}
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
