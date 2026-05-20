package helix

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"github.com/helixml/helix-org/domain"
	"github.com/helixml/helix-org/helix/helixclient"
	"github.com/helixml/helix-org/store"
)

// ProjectApplier ensures a Worker has a Helix project of its own.
// Used by both Spawner (AI Worker activations) and chat.HelixBridge
// (owner chat) — every Worker, human or AI, that drives an LLM call
// needs a per-Worker project so the org-graph MCP server can be wired
// in via the project's auto-provisioned Agent App.
//
// Idempotent: re-applying for a Worker that already has a project is
// a no-op for the project itself, but always re-pushes the canonical
// role/identity files so update_role / update_identity changes land.
type ProjectApplier struct {
	Client      helixclient.Client
	Store       *store.Store
	HelixOrgURL string
	OrgID       string
	// Runtime overrides the default `zed_agent` runtime constant.
	// Empty means "use the package-level Runtime const" (zed_agent),
	// which routes inference back through Helix and honours
	// Provider/Model. Set to `"claude_code"` to bypass Helix inference
	// entirely — the sandbox-side Claude Code CLI authenticates with
	// Anthropic via its own OAuth subscription, ignoring
	// Provider/Model. Used by the embedded SaaS so workers run on the
	// operator's Claude subscription instead of a Helix-side API key.
	Runtime  string
	Provider string
	Model    string
	// Credentials selects the in-sandbox auth source for the runtime.
	// `"subscription"` tells `claude_code` to authenticate Anthropic
	// via the operator's Claude OAuth subscription (no Provider/Model
	// needed). Empty falls back to Helix-routed inference via
	// Provider/Model — only meaningful for `zed_agent` / `qwen_code` /
	// etc. runtimes that go through Helix's anthropic proxy.
	Credentials string
	// AgentMD is the org-wide agent policy pushed verbatim to
	// `.context/agent.md` on every Worker's helix-specs branch. Empty
	// string skips the push.
	AgentMD string
	// MCPAuthBearer is added as an `Authorization: Bearer <value>`
	// header on the helix-org MCP entry attached to each Worker's
	// agent app. Used when HelixOrgURL routes through an auth-gated
	// proxy (e.g. the embedded SaaS alpha's Helix MCP gateway at
	// /api/v1/mcp/helix-org/). Empty means "no auth header" — the
	// standalone helix-org tunnel path has no auth on /workers/{id}/mcp
	// so callers leave this empty there.
	MCPAuthBearer string
	Logger        *slog.Logger
}

// Ensure applies a Helix project for the given Worker if one
// doesn't exist yet. Returns the resolved project / agent-app /
// repo IDs (read from the runtime state after persistence so callers
// see the same view of state).
func (a *ProjectApplier) Ensure(ctx context.Context, workerID domain.WorkerID) (projectID, agentAppID, repoID string, err error) {
	worker, err := a.Store.Workers.Get(ctx, workerID)
	if err != nil {
		return "", "", "", fmt.Errorf("get worker: %w", err)
	}
	state, err := LoadState(ctx, a.Store, workerID)
	if err != nil {
		return "", "", "", err
	}
	// Resolve role content from the Worker's first position (if any).
	// We need this both on first apply (to seed agent.md / role.md /
	// identity.md) and on every subsequent Ensure (so role hot-edits
	// propagate to the helix-specs branch).
	var roleContent, roleName string
	if positions := worker.Positions(); len(positions) > 0 {
		if pos, err := a.Store.Positions.Get(ctx, positions[0]); err == nil {
			if role, err := a.Store.Roles.Get(ctx, pos.RoleID); err == nil {
				roleContent = role.Content
				roleName = string(role.ID)
			}
		}
	}
	// Fast path: project already exists. Skip the expensive
	// ApplyProject / CreateGitRepo / AttachRepo steps but DO re-push
	// role + identity so update_role / update_identity changes
	// propagate. CreateBranch + PutFile are idempotent and cheap.
	if state.ProjectID != "" {
		a.republishWorkerFiles(ctx, workerID, state.RepoID, roleContent, worker.IdentityContent())
		return state.ProjectID, state.AgentAppID, state.RepoID, nil
	}
	// Every project is applied with the same Runtime — see
	// helix.Runtime for why. The auto-provisioned Agent App is the
	// vehicle for our MCP wiring; we attach helix-org's MCP server
	// to it in a follow-up step (UpdateApp can't be done in apply).
	runtime := a.Runtime
	if runtime == "" {
		runtime = Runtime
	}
	resp, err := a.Client.ApplyProject(ctx, helixclient.ProjectApplyRequest{
		OrganizationID: a.OrgID,
		Name:           string(workerID),
		Spec: helixclient.ProjectSpec{
			Description: worker.IdentityContent(),
			Agent: &helixclient.ProjectAgentSpec{
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
	_ = a.Client.PutProjectSecret(ctx, resp.ProjectID, "HELIX_ORG_URL", a.HelixOrgURL)
	_ = a.Client.PutProjectSecret(ctx, resp.ProjectID, "HELIX_WORKER_ID", string(workerID))
	// Discover the project's primary repo and its org (we need the
	// org to create a same-org repo; Helix rejects cross-org attaches).
	repoID = ""
	var projOrgID string
	if proj, err := a.Client.GetProject(ctx, resp.ProjectID); err == nil {
		repoID = proj.DefaultRepoID
		projOrgID = proj.OrganizationID
	}
	// Helix's project-apply does NOT auto-create a default repo. The
	// desktop's startup script then refuses to launch Zed with
	// "No repositories were cloned successfully" — at which point the
	// session sits forever in `state=running` without an agent thread.
	// For our owner-chat / org-graph use case we don't need a real
	// code repo, just a Helix-internal one to satisfy the workspace
	// check. Create one if missing and attach it as primary.
	if repoID == "" {
		var ownerID string
		if me, err := a.Client.WhoAmI(ctx); err == nil {
			ownerID = me.User
		}
		if ownerID != "" {
			repo, err := a.Client.CreateGitRepo(ctx, helixclient.CreateGitRepoRequest{
				Name:           string(workerID),
				OwnerID:        ownerID,
				OrganizationID: projOrgID,
				// Seed the default branch with a README so subsequent
				// pushes to `helix-specs` (the branch our role/identity
				// files live on) have a base commit to fork from. A
				// brand-new bare repo has no branches at all and any
				// PUT to a non-existent branch fails with `Remote
				// branch helix-specs not found in upstream origin`.
				InitialFiles: map[string]string{
					"README.md": "# " + string(workerID) + "\n\nWorkspace for Helix Worker `" + string(workerID) + "`. Files in `job/` carry the role + identity prompt.\n",
				},
			})
			if err != nil && a.Logger != nil {
				a.Logger.Warn("create git repo for project", "worker", workerID, "err", err)
			} else if err == nil {
				if err := a.Client.AttachRepoToProject(ctx, resp.ProjectID, repo.ID, true); err != nil {
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
	// Attach helix-org's MCP server to the auto-provisioned Agent
	// App. Helix's project-apply doesn't accept MCPs in
	// ProjectAgentSpec (only the simple WebSearch/Browser/Calculator
	// flags), so we GetApp + mutate + UpdateApp in a second step.
	// The MCP URL must be reachable from Helix's runner — operator
	// runs cloudflared (or similar) and sets `helix.org_url` to the
	// public tunnel URL.
	if resp.AgentAppID != "" && a.HelixOrgURL != "" {
		mcpURL := strings.TrimRight(a.HelixOrgURL, "/") + "/workers/" + string(workerID) + "/mcp"
		// Prefer the per-request bearer from ctx so the attached MCP
		// entry authenticates as the actual user who triggered this
		// project apply — the chat-bridge path carries the logged-in
		// user's bearer via withHelixUserBearer; the spawner path
		// carries the hiring user's bearer via BearerForUser. Falling
		// back to the static MCPAuthBearer keeps the original
		// service-key behaviour for standalone deploys.
		bearer := helixclient.BearerFromContext(ctx)
		if bearer == "" {
			bearer = a.MCPAuthBearer
		}
		var headers map[string]string
		if bearer != "" {
			headers = map[string]string{"Authorization": "Bearer " + bearer}
		}
		if err := helixclient.AttachMCPToAppWithHeaders(ctx, a.Client, resp.AgentAppID, "helix", "http", mcpURL, headers); err != nil {
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
// identity.md files on the Worker's helix-specs branch. Called from
// both the first-apply path (so the branch and files exist before the
// desktop boots) and the fast path (so update_role / update_identity
// edits propagate on every activation).
//
// All operations are idempotent and cheap: CreateBranch on an existing
// branch is a 200, and PutFile overwrites any existing content. We log
// errors but never fail Ensure on them — partial state is recoverable
// on the next activation, but a hard fail would block the dispatch
// chain entirely.
//
// The agent inside the desktop must `git pull origin helix-specs`
// before reading these files, otherwise it'll see the worktree's
// pre-existing copy. The spawner's activation prompt
// (helixSpecsMandate) carries that pull instruction.
func (a *ProjectApplier) republishWorkerFiles(ctx context.Context, workerID domain.WorkerID, repoID, roleContent, identityContent string) {
	if repoID == "" {
		return
	}
	if err := a.Client.CreateBranch(ctx, repoID, "helix-specs", "main"); err != nil {
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
		if err := a.Client.PutFile(ctx, repoID, helixclient.PutFileRequest{
			Path:    path,
			Branch:  "helix-specs",
			Message: "republish " + path,
			Content: content,
		}); err != nil && a.Logger != nil {
			a.Logger.Warn("republish worker files: put", "worker", workerID, "path", path, "err", err)
		}
	}
}
