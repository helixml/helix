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

	runtimehelix "github.com/helixml/helix/api/pkg/org/runtime/helix"
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

// PutProjectSecret upserts a project-scoped secret. Routes through
// HelixAPIServer.createProjectSecret. Helix's underlying store does an
// upsert on (project_id, name) so re-calling with the same name updates.
func (c *inProcHelixClient) PutProjectSecret(ctx context.Context, projectID, name, value string) error {
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
// WorkerProject.attachMCPToApp to round-trip MCP entries.
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
		return types.SessionOutputResponse{}, fmt.Errorf("get session output %s: %s", sessionID, herr.Error())
	}
	if resp == nil {
		return types.SessionOutputResponse{}, fmt.Errorf("get session output %s: nil response", sessionID)
	}
	return *resp, nil
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

// StartChatWithStatus opens or continues a chat session and reports
// whether the SSE stream surfaced a transient error after the session
// ID came through. The underlying startChatSessionHandler is streaming
// (writes SSE chunks to the ResponseWriter and Flushes), so we capture
// via a custom sseCapture writer that scans the chunks — `data: `
// prefix, JSON chunks with `id` and `error.message`, sets hadWSError
// when an error chunk arrives.
func (c *inProcHelixClient) StartChatWithStatus(ctx context.Context, req runtimehelix.StartChatRequest) (types.Session, bool, error) {
	if req.Type == "" {
		req.Type = "text"
	}
	if len(req.Messages) == 0 {
		return types.Session{}, false, errors.New("StartChatWithStatus: req.Messages must contain at least one message")
	}
	if req.AgentType == "zed_external" && req.ExternalAgentConfig == nil {
		req.ExternalAgentConfig = &types.ExternalAgentConfig{}
	}

	r, err := c.newRequest(ctx, http.MethodPost, "/api/v1/sessions/chat", req, nil)
	if err != nil {
		return types.Session{}, false, err
	}
	cap := newSSECapture(req.OnSessionID)
	c.server.startChatSessionHandler(cap, r)
	if cap.statusCode >= 400 {
		return types.Session{}, false, fmt.Errorf("start chat: HTTP %d: %s", cap.statusCode, strings.TrimSpace(cap.errBody.String()))
	}
	// Try SSE parsing first — `data: ` prefix means we got a stream.
	if id, hadErr := cap.parseSSE(); id != "" {
		return types.Session{ID: id}, hadErr, nil
	}
	// Fall back: handler may have returned a JSON body (helix_basic /
	// openai shape).
	if cap.body.Len() > 0 {
		s, perr := parseStartChatResponseInProc(cap.body.Bytes())
		if perr != nil {
			return types.Session{}, false, perr
		}
		if req.OnSessionID != nil && s.ID != "" {
			req.OnSessionID(s.ID)
		}
		return s, false, nil
	}
	return types.Session{}, false, errors.New("start chat: no session id and no body")
}

// parseStartChatResponseInProc handles both the zed_external
// types.Session shape and the OpenAI chat-completion shape
// helix_basic returns.
func parseStartChatResponseInProc(raw []byte) (types.Session, error) {
	var s types.Session
	_ = json.Unmarshal(raw, &s)
	if len(s.Interactions) > 0 {
		if s.ID == "" {
			return types.Session{}, errors.New("start chat: session has no id")
		}
		return s, nil
	}
	var oai struct {
		ID      string `json:"id"`
		Choices []struct {
			Index   int `json:"index"`
			Message struct {
				Role    string `json:"role"`
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(raw, &oai); err != nil {
		return types.Session{}, fmt.Errorf("decode start-chat response: %w", err)
	}
	if oai.ID == "" {
		return types.Session{}, errors.New("start chat: empty session id")
	}
	out := types.Session{ID: oai.ID}
	if len(oai.Choices) > 0 && oai.Choices[0].Message.Content != "" {
		out.Interactions = []*types.Interaction{{
			ID:              oai.ID + ":synth",
			State:           "complete",
			ResponseMessage: oai.Choices[0].Message.Content,
		}}
	}
	return out, nil
}

// sseCapture is an http.ResponseWriter + http.Flusher that buffers
// everything the streaming startChatSessionHandler writes, so the
// adapter can scan it for SSE chunks (session ID + error.message).
//
// Flush() is a no-op — there's no client to push to, but the handler's
// `if f, ok := rw.(http.Flusher); ok` check still needs to succeed for
// chunks to be emitted on the buffer mid-handler.
type sseCapture struct {
	header      http.Header
	body        bytes.Buffer
	errBody     bytes.Buffer
	statusCode  int
	onSessionID func(string)
}

func newSSECapture(onSessionID func(string)) *sseCapture {
	return &sseCapture{
		header:      http.Header{},
		statusCode:  http.StatusOK,
		onSessionID: onSessionID,
	}
}

// Header satisfies http.ResponseWriter.
func (s *sseCapture) Header() http.Header { return s.header }

// Write satisfies http.ResponseWriter. We buffer error bodies separately
// from success bodies so the caller can surface a meaningful HTTP-style
// error when the handler returned >=400.
func (s *sseCapture) Write(b []byte) (int, error) {
	if s.statusCode >= 400 {
		return s.errBody.Write(b)
	}
	return s.body.Write(b)
}

// WriteHeader satisfies http.ResponseWriter.
func (s *sseCapture) WriteHeader(code int) { s.statusCode = code }

// Flush satisfies http.Flusher — no-op since we're an in-process buffer.
func (s *sseCapture) Flush() {}

// parseSSE scans the buffered body looking for `data: …` chunks of the
// shape `{"id":"…","error":{"message":"…"}}` and returns the first
// session ID it sees along with a flag indicating whether any chunk
// carried an error.message.
func (s *sseCapture) parseSSE() (sessionID string, hadWSError bool) {
	// Quick check: does the body look like SSE? Handler emits "data: "
	// prefixed lines for streaming sessions. helix_basic returns plain JSON.
	bodyStr := s.body.String()
	if !strings.Contains(bodyStr, "data:") {
		return "", false
	}
	for _, line := range strings.Split(bodyStr, "\n") {
		payload := strings.TrimSpace(line)
		if payload == "" {
			continue
		}
		payload = strings.TrimPrefix(payload, "data:")
		payload = strings.TrimSpace(payload)
		if payload == "" || payload == "[DONE]" {
			continue
		}
		var chunk struct {
			ID    string `json:"id"`
			Error *struct {
				Message string `json:"message"`
			} `json:"error,omitempty"`
		}
		if err := json.Unmarshal([]byte(payload), &chunk); err != nil {
			continue
		}
		if chunk.ID != "" && sessionID == "" {
			sessionID = chunk.ID
			if s.onSessionID != nil {
				s.onSessionID(sessionID)
			}
		}
		if chunk.Error != nil {
			hadWSError = true
			break
		}
	}
	return sessionID, hadWSError
}

// Compile-time interface assertions — both ports must be satisfied by
// the same struct so a single instance can drive WorkerProject.Service
// AND SpawnerConfig.Client.
var (
	_ runtimehelix.ProjectService = (*inProcHelixClient)(nil)
	_ runtimehelix.SpawnerClient  = (*inProcHelixClient)(nil)
)
