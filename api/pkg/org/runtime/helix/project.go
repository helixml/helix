package helix

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"

	"github.com/helixml/helix/api/pkg/org/worker"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/helixml/helix/helix-org/store"
)

// ErrProjectNotFound is the sentinel a ProjectService impl must return
// when GetProject is called against a project that no longer exists on
// the Helix side. ProjectApplier compares with errors.Is — a 404-shaped
// error from the underlying transport must wrap this sentinel so the
// fast-path verification can clear stale state and re-apply.
//
// In the H1.x transitional state, the helixclient-backed adapter maps
// helixclient.ErrNotFound to this sentinel.
var ErrProjectNotFound = errors.New("helix project: not found")

// ProjectService is the slice of Helix's project/git/app API that the
// per-Worker ProjectApplier depends on. Lifted out of helixclient.Client
// during H1.2 so api/pkg/org/runtime/helix doesn't import the legacy
// helixclient package — the wiring in api/pkg/server provides an impl
// that either wraps helixclient (today, transitional) or calls the
// controllers directly (the eventual canonical end state).
//
// All shapes mirror api/pkg/types so the wiring adapter doesn't have to
// translate. Methods take and return canonical Helix types verbatim.
type ProjectService interface {
	// WhoAmI returns the authenticated user's ID/slug. Used to resolve
	// the owner for newly-created git repositories.
	WhoAmI(ctx context.Context) (string, error)

	// ApplyProject is upsert-by-name within the operator's org. Returns
	// the resolved project and agent-app IDs plus a flag indicating
	// whether the call created a fresh project (true) or updated an
	// existing one (false).
	ApplyProject(ctx context.Context, req types.ProjectApplyRequest) (types.ProjectApplyResponse, error)

	// GetProject returns the current state of a Helix project. Returns
	// ErrProjectNotFound (wrapped if needed) when the project doesn't
	// exist; ProjectApplier uses errors.Is to detect this.
	GetProject(ctx context.Context, id string) (types.Project, error)

	// PutProjectSecret upserts an env-var injected into the agent's
	// container at session start.
	PutProjectSecret(ctx context.Context, projectID, name, value string) error

	// CreateGitRepo creates a Helix-internal git repository. Used when
	// project-apply doesn't auto-create one.
	CreateGitRepo(ctx context.Context, req types.GitRepositoryCreateRequest) (types.GitRepository, error)

	// AttachRepoToProject attaches an existing repo as a project's
	// primary (or secondary) repository.
	AttachRepoToProject(ctx context.Context, projectID, repoID string, primary bool) error

	// CreateBranch creates a new branch on a repo from baseBranch.
	// Idempotent: re-creating an existing branch is a 200.
	CreateBranch(ctx context.Context, repoID, branch, baseBranch string) error

	// GetAppRawConfig returns the raw JSON of an App's `config` column.
	// Used to round-trip the helix.assistants[0] config for MCP
	// attachment — we mutate one nested field, so working at the JSON
	// level avoids dropping unknown fields we don't have types for.
	GetAppRawConfig(ctx context.Context, id string) (json.RawMessage, error)

	// UpdateAppRawConfig persists a mutated app config (raw JSON).
	// ProjectApplier uses this to attach helix-org's MCP server to the
	// auto-provisioned Agent App.
	UpdateAppRawConfig(ctx context.Context, id string, config json.RawMessage) error
}

// ProjectApplier ensures a Worker has a Helix project of its own.
// Used by both Spawner (AI Worker activations) and chat.HelixBridge
// (owner chat) — every Worker, human or AI, that drives an LLM call
// needs a per-Worker project so the org-graph MCP server can be wired
// in via the project's auto-provisioned Agent App.
//
// Idempotent: re-applying for a Worker that already has a project is
// a no-op for the project itself, but always re-pushes the canonical
// role/identity files so update_role / update_identity changes land.
//
// H1.2 lifted ProjectApplier to its canonical home and decoupled it
// from helix-org/helix/helixclient by routing the project / git / app
// calls through the ProjectService interface and the file pushes
// through ProjectGitWriter (a thin slice of WorkspaceGitWriter).
// During the H1.x transitional state the production wiring in
// api/pkg/server/helix_org.go satisfies ProjectService with an adapter
// over helixclient.Client; the eventual end state is a direct adapter
// over the Helix controller.
type ProjectApplier struct {
	Service ProjectService
	Git     ProjectGitWriter
	Store   *store.Store
	HelixOrgURL string
	OrgID       string
	// Runtime overrides the default `zed_agent` runtime constant.
	// Empty means "use the package-level Runtime const" (zed_agent),
	// which routes inference back through Helix and honours
	// Provider/Model. Set to `"claude_code"` to bypass Helix inference
	// entirely — the sandbox-side Claude Code CLI authenticates with
	// Anthropic via its own OAuth subscription, ignoring
	// Provider/Model.
	Runtime  string
	Provider string
	Model    string
	// Credentials selects the in-sandbox auth source for the runtime.
	Credentials string
	// AgentMD is the org-wide agent policy pushed verbatim to
	// `.context/agent.md` on every Worker's helix-specs branch. Empty
	// string skips the push.
	AgentMD string
	// MCPAuthBearer is added as an `Authorization: Bearer <value>`
	// header on the helix-org MCP entry attached to each Worker's
	// agent app.
	MCPAuthBearer string
	Logger        *slog.Logger
}

// ProjectGitWriter is the slice of the git servicer ProjectApplier uses
// for branch/file mutations. Same shape as WorkspaceGitWriter but with
// CreateBranch included; *services.GitRepositoryService satisfies both.
type ProjectGitWriter interface {
	CreateBranch(ctx context.Context, repoID, branchName, baseBranch string) error
	CreateOrUpdateFileContents(ctx context.Context, repoID, path, branch string, content []byte, commitMessage, authorName, authorEmail string) (string, error)
}

// Ensure applies a Helix project for the given Worker if one
// doesn't exist yet. Returns the resolved project / agent-app /
// repo IDs (read from the runtime state after persistence so callers
// see the same view of state).
func (a *ProjectApplier) Ensure(ctx context.Context, workerID worker.ID) (projectID, agentAppID, repoID string, err error) {
	worker, err := a.Store.Workers.Get(ctx, workerID)
	if err != nil {
		return "", "", "", fmt.Errorf("get worker: %w", err)
	}
	state, err := LoadState(ctx, a.Store, workerID)
	if err != nil {
		return "", "", "", err
	}
	// Resolve role content from the Worker's first position (if any).
	var roleContent, roleName string
	if positions := worker.Positions(); len(positions) > 0 {
		if pos, err := a.Store.Positions.Get(ctx, positions[0]); err == nil {
			if role, err := a.Store.Roles.Get(ctx, pos.RoleID); err == nil {
				roleContent = role.Content
				roleName = string(role.ID)
			}
		}
	}
	// Fast path: project already exists.
	if state.ProjectID != "" {
		if _, err := a.Service.GetProject(ctx, state.ProjectID); err != nil {
			if errors.Is(err, ErrProjectNotFound) {
				if clearErr := ClearProject(ctx, a.Store, workerID); clearErr != nil {
					return "", "", "", fmt.Errorf("clear stale project state for %s: %w", workerID, clearErr)
				}
				if a.Logger != nil {
					a.Logger.Info("project applier: persisted project missing, re-applying", "worker", workerID, "stale_project_id", state.ProjectID)
				}
				// fall through to fresh apply
			} else {
				return "", "", "", fmt.Errorf("verify project %s for %s: %w", state.ProjectID, workerID, err)
			}
		} else {
			a.republishWorkerFiles(ctx, workerID, state.RepoID, roleContent, worker.IdentityContent())
			return state.ProjectID, state.AgentAppID, state.RepoID, nil
		}
	}
	runtime := a.Runtime
	if runtime == "" {
		runtime = Runtime
	}
	resp, err := a.Service.ApplyProject(ctx, types.ProjectApplyRequest{
		OrganizationID: a.OrgID,
		Name:           string(workerID),
		Spec: types.ProjectSpec{
			Description: worker.IdentityContent(),
			Agent: &types.ProjectAgentSpec{
				Name:        roleName,
				Runtime:     runtime,
				Provider:    a.Provider,
				Model:       a.Model,
				Credentials: a.Credentials,
			},
		},
	})
	if err != nil {
		return "", "", "", fmt.Errorf("apply project for %s: %w", workerID, err)
	}
	// Project secrets — env-var injection.
	_ = a.Service.PutProjectSecret(ctx, resp.ProjectID, "HELIX_ORG_URL", a.HelixOrgURL)
	_ = a.Service.PutProjectSecret(ctx, resp.ProjectID, "HELIX_WORKER_ID", string(workerID))
	// Discover the project's primary repo and its org.
	repoID = ""
	var projOrgID string
	if proj, err := a.Service.GetProject(ctx, resp.ProjectID); err == nil {
		repoID = proj.DefaultRepoID
		projOrgID = proj.OrganizationID
	}
	// Helix's project-apply does NOT auto-create a default repo —
	// create one and attach it as primary so the desktop can launch
	// Zed.
	if repoID == "" {
		ownerID, _ := a.Service.WhoAmI(ctx)
		if ownerID != "" {
			repo, err := a.Service.CreateGitRepo(ctx, types.GitRepositoryCreateRequest{
				Name:           string(workerID),
				OwnerID:        ownerID,
				OrganizationID: projOrgID,
				InitialFiles: map[string]string{
					"README.md": "# " + string(workerID) + "\n\nWorkspace for Helix Worker `" + string(workerID) + "`. Files in `job/` carry the role + identity prompt.\n",
				},
			})
			if err != nil && a.Logger != nil {
				a.Logger.Warn("create git repo for project", "worker", workerID, "err", err)
			} else if err == nil {
				if err := a.Service.AttachRepoToProject(ctx, resp.ProjectID, repo.ID, true); err != nil {
					if a.Logger != nil {
						a.Logger.Warn("attach repo to project", "worker", workerID, "repo", repo.ID, "err", err)
					}
				} else {
					repoID = repo.ID
					if a.Logger != nil {
						a.Logger.Info("helix repo created and attached", "worker", workerID, "repo", repo.ID)
					}
				}
			}
		}
	}
	a.republishWorkerFiles(ctx, workerID, repoID, roleContent, worker.IdentityContent())
	// Attach helix-org's MCP server to the auto-provisioned Agent App.
	if resp.AgentAppID != "" && a.HelixOrgURL != "" {
		mcpURL := strings.TrimRight(a.HelixOrgURL, "/") + "/workers/" + string(workerID) + "/mcp"
		// Prefer the per-request bearer from ctx so the attached MCP
		// entry authenticates as the actual user who triggered this
		// project apply.
		bearer := BearerFromContext(ctx)
		if bearer == "" {
			bearer = a.MCPAuthBearer
		}
		var headers map[string]string
		if bearer != "" {
			headers = map[string]string{"Authorization": "Bearer " + bearer}
		}
		if err := a.attachMCPToApp(ctx, resp.AgentAppID, "helix", "http", mcpURL, headers); err != nil {
			if a.Logger != nil {
				a.Logger.Warn("attach MCP to agent app", "worker", workerID, "app", resp.AgentAppID, "err", err)
			}
		} else if a.Logger != nil {
			a.Logger.Info("helix mcp attached", "worker", workerID, "app", resp.AgentAppID, "mcp", mcpURL)
		}
	}
	if err := SaveProject(ctx, a.Store, workerID, resp.ProjectID, resp.AgentAppID, repoID); err != nil {
		return "", "", "", fmt.Errorf("persist helix project IDs: %w", err)
	}
	if a.Logger != nil {
		a.Logger.Info("helix project applied",
			"worker", workerID,
			"project", resp.ProjectID,
			"agent_app", resp.AgentAppID,
			"repo", repoID,
			"created", resp.Created,
		)
	}
	return resp.ProjectID, resp.AgentAppID, repoID, nil
}

// republishWorkerFiles writes (or rewrites) the agent.md / role.md /
// identity.md files on the Worker's helix-specs branch.
func (a *ProjectApplier) republishWorkerFiles(ctx context.Context, workerID worker.ID, repoID, roleContent, identityContent string) {
	if repoID == "" || a.Git == nil {
		return
	}
	if err := a.Git.CreateBranch(ctx, repoID, "helix-specs", "main"); err != nil {
		if a.Logger != nil {
			a.Logger.Warn("republish worker files: create helix-specs branch", "worker", workerID, "err", err)
		}
	}
	for path, content := range map[string]string{
		".context/agent.md": a.AgentMD,
		"workers/" + string(workerID) + "/.context/role.md":     roleContent,
		"workers/" + string(workerID) + "/.context/identity.md": identityContent,
	} {
		if content == "" {
			continue
		}
		if _, err := a.Git.CreateOrUpdateFileContents(ctx, repoID, path, "helix-specs", []byte(content), "republish "+path, "helix-org", "helix-org@helix.local"); err != nil && a.Logger != nil {
			a.Logger.Warn("republish worker files: put", "worker", workerID, "path", path, "err", err)
		}
	}
}

// attachMCPToApp is the in-package equivalent of
// helixclient.AttachMCPToAppWithHeaders. Round-trips the app config
// to avoid dropping unknown fields, mutates assistants[0].mcps, and
// writes back.
func (a *ProjectApplier) attachMCPToApp(ctx context.Context, appID, name, transport, mcpURL string, headers map[string]string) error {
	if appID == "" {
		return errors.New("attachMCPToApp: appID is empty")
	}
	cfg, err := a.Service.GetAppRawConfig(ctx, appID)
	if err != nil {
		return fmt.Errorf("get app: %w", err)
	}
	var raw map[string]any
	if len(cfg) == 0 {
		raw = map[string]any{}
	} else if err := json.Unmarshal(cfg, &raw); err != nil {
		return fmt.Errorf("decode config: %w", err)
	}
	helix, _ := raw["helix"].(map[string]any)
	if helix == nil {
		helix = map[string]any{}
		raw["helix"] = helix
	}
	asstsAny, _ := helix["assistants"].([]any)
	if len(asstsAny) == 0 {
		return errors.New("attachMCPToApp: app has no assistants")
	}
	asst, _ := asstsAny[0].(map[string]any)
	if asst == nil {
		return errors.New("attachMCPToApp: assistant is not an object")
	}
	mcpsAny, _ := asst["mcps"].([]any)
	mcps := make([]any, 0, len(mcpsAny)+1)
	replaced := false
	for _, mAny := range mcpsAny {
		m, ok := mAny.(map[string]any)
		if !ok {
			mcps = append(mcps, mAny)
			continue
		}
		if m["name"] == name {
			m["transport"] = transport
			m["url"] = mcpURL
			if len(headers) > 0 {
				m["headers"] = headers
			} else {
				delete(m, "headers")
			}
			mcps = append(mcps, m)
			replaced = true
			continue
		}
		mcps = append(mcps, m)
	}
	if !replaced {
		entry := map[string]any{
			"name":      name,
			"transport": transport,
			"url":       mcpURL,
		}
		if len(headers) > 0 {
			entry["headers"] = headers
		}
		mcps = append(mcps, entry)
	}
	asst["mcps"] = mcps
	updated, err := json.Marshal(raw)
	if err != nil {
		return fmt.Errorf("marshal config: %w", err)
	}
	if err := a.Service.UpdateAppRawConfig(ctx, appID, updated); err != nil {
		return fmt.Errorf("update app: %w", err)
	}
	return nil
}
