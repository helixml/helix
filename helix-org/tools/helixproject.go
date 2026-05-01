package tools

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"github.com/helixml/helix-org/domain"
	"github.com/helixml/helix-org/store"
	"github.com/helixml/helix-org/tools/helixclient"
)

// HelixProjectApplier ensures a Worker has a Helix project of its
// own. Used by both `HelixSpawner` (AI Worker activations) and
// `chat.HelixBridge` (owner chat) — every Worker, human or AI, that
// drives an LLM call needs a per-Worker project so the org-graph
// MCP server can be wired in via the project's auto-provisioned
// Agent App.
//
// Idempotent: re-applying for a Worker that already has a project
// is a no-op.
type HelixProjectApplier struct {
	Client      helixclient.Client
	Store       *store.Store
	HelixOrgURL string
	OrgID       string
	Provider    string
	Model       string
	Runtime     string // "claude_code" by default
	Logger      *slog.Logger
}

// Ensure applies a Helix project for the given Worker if one
// doesn't exist yet. Returns the resolved project / agent-app /
// repo IDs (read from the Worker after persistence so callers see
// the same view of state).
func (a *HelixProjectApplier) Ensure(ctx context.Context, workerID domain.WorkerID) (projectID, agentAppID, repoID string, err error) {
	worker, err := a.Store.Workers.Get(ctx, workerID)
	if err != nil {
		return "", "", "", fmt.Errorf("get worker: %w", err)
	}
	if worker.HelixProjectID() != "" {
		return worker.HelixProjectID(), worker.HelixAgentAppID(), worker.HelixRepoID(), nil
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
	// Project-apply always produces a zed_external Agent App
	// (Helix's `projectAgentRuntimeToTypes` hard-codes that — even
	// "empty" runtime maps to zed_external+claude_code). Both human
	// and AI Workers use this so the LLM's tool calls flow through
	// MCP via the Agent App we attach to the project.
	runtime := a.Runtime
	if runtime == "" {
		runtime = "claude_code"
	}
	resp, err := a.Client.ApplyProject(ctx, helixclient.ProjectApplyRequest{
		OrganizationID: a.OrgID,
		Name:           string(workerID),
		Spec: helixclient.ProjectSpec{
			Description: worker.IdentityContent(),
			Agent: &helixclient.ProjectAgentSpec{
				Name:     roleName,
				Runtime:  runtime,
				Provider: a.Provider,
				Model:    a.Model,
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
	if repoID != "" {
		// Push role + identity to the `helix-specs` branch, at the
		// paths the spawner's activation prompt tells the agent to
		// read from (`workers/<workerID>/.context/role.md` etc.).
		//
		// Why helix-specs and not main:
		//   - The desktop's helix-workspace-setup.sh creates a separate
		//     worktree for the helix-specs branch at `~/work/helix-specs/`,
		//     ONLY if the branch already exists on the remote. So the
		//     branch needs to be pushed before the desktop boots — we
		//     do it at applier time.
		//   - Keeping role/identity on a dedicated orphan-style branch
		//     keeps `main` free for whatever the Worker's actual code
		//     workspace is, mirroring Helix's spec-task convention.
		//
		// PutFile to a non-existent branch fails with `Remote branch
		// helix-specs not found in upstream origin`, so we create the
		// branch from main first. CreateBranch is idempotent (returns
		// 200 on existing branches).
		if err := a.Client.CreateBranch(ctx, repoID, "helix-specs", "main"); err != nil {
			if a.Logger != nil {
				a.Logger.Warn("helix project bootstrap: create helix-specs branch", "worker", workerID, "err", err)
			}
		}
		for path, content := range map[string]string{
			"workers/" + string(workerID) + "/.context/role.md":     roleContent,
			"workers/" + string(workerID) + "/.context/identity.md": worker.IdentityContent(),
		} {
			if content == "" {
				continue
			}
			if err := a.Client.PutFile(ctx, repoID, helixclient.PutFileRequest{
				Path:    path,
				Branch:  "helix-specs",
				Message: "bootstrap " + path,
				Content: content,
			}); err != nil && a.Logger != nil {
				a.Logger.Warn("helix project bootstrap: put file", "worker", workerID, "path", path, "err", err)
			}
		}
	}
	// Attach helix-org's MCP server to the auto-provisioned Agent
	// App. Helix's project-apply doesn't accept MCPs in
	// ProjectAgentSpec (only the simple WebSearch/Browser/Calculator
	// flags), so we GetApp + mutate + UpdateApp in a second step.
	// The MCP URL must be reachable from Helix's runner — operator
	// runs cloudflared (or similar) and sets `helix.org_url` to the
	// public tunnel URL.
	if resp.AgentAppID != "" && a.HelixOrgURL != "" {
		mcpURL := strings.TrimRight(a.HelixOrgURL, "/") + "/workers/" + string(workerID) + "/mcp"
		if err := helixclient.AttachMCPToApp(ctx, a.Client, resp.AgentAppID, "helix", "http", mcpURL); err != nil {
			if a.Logger != nil {
				a.Logger.Warn("attach MCP to agent app", "worker", workerID, "app", resp.AgentAppID, "err", err)
			}
		} else if a.Logger != nil {
			a.Logger.Info("helix mcp attached", "worker", workerID, "app", resp.AgentAppID, "mcp", mcpURL)
		}
	}
	if err := a.Store.Workers.SetHelixProject(ctx, workerID, resp.ProjectID, resp.AgentAppID, repoID); err != nil {
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
