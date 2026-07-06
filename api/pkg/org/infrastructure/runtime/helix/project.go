package helix

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"github.com/helixml/helix/api/pkg/org/domain/orgchart"
	"github.com/helixml/helix/api/pkg/org/domain/store"
	"github.com/helixml/helix/api/pkg/types"
)

// ErrProjectNotFound is the sentinel a ProjectService impl must return
// when GetProject is called against a project that no longer exists on
// the Helix side. WorkerProject compares with errors.Is — a 404-shaped
// error from the underlying transport must wrap this sentinel so the
// fast-path verification can clear stale state and re-apply.
var ErrProjectNotFound = errors.New("helix project: not found")

// ProjectService is the slice of Helix's project/git/app API that the
// per-Worker WorkerProject depends on. The wiring in api/pkg/server
// provides an in-process impl that calls the HelixAPIServer's handler
// methods directly (no HTTP loopback).
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
	// exist; WorkerProject uses errors.Is to detect this.
	GetProject(ctx context.Context, id string) (types.Project, error)

	// UpdateProject applies a partial patch to a Helix project. Used
	// by the helix runtime's ProjectConfig impl to back the
	// configure_worker_project MCP tool. Patch semantics follow
	// types.ProjectUpdateRequest — only non-nil fields are written.
	// Returns the post-update project so callers can confirm what
	// landed without an extra GetProject round-trip.
	UpdateProject(ctx context.Context, id string, patch types.ProjectUpdateRequest) (types.Project, error)

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

	// GetAppConfig returns the typed config for an App. Used to round-
	// trip the helix.assistants[0] MCP list for MCP attachment.
	GetAppConfig(ctx context.Context, id string) (types.AppConfig, error)

	// UpdateAppConfig persists a mutated app config. WorkerProject
	// uses this to attach helix-org's MCP server to the auto-provisioned
	// Agent App.
	UpdateAppConfig(ctx context.Context, id string, config types.AppConfig) error

	// DeleteProject soft-deletes a Helix project and stops any active
	// sessions running against it. Used by the fire-worker cascade to
	// tear down the per-Worker project on worker delete. Missing
	// projects (already deleted, or never created) are reported via
	// ErrProjectNotFound so callers can ignore them.
	DeleteProject(ctx context.Context, id string) error

	// DeleteApp removes a Helix App. Used by the fire-worker cascade
	// to clean up the per-Worker agent app that ApplyProject auto-
	// provisioned. Missing apps are reported via ErrProjectNotFound
	// (re-used sentinel — semantically "the resource isn't there
	// anymore"); callers should treat that as success.
	DeleteApp(ctx context.Context, id string) error
}

// WorkerProject ensures a Worker has a Helix project of its own.
// Used by both Spawner (AI Worker activations) and chat.HelixBridge
// (owner chat) — every Worker, human or AI, that drives an LLM call
// needs a per-Worker project so the org-graph MCP server can be wired
// in via the project's auto-provisioned Agent App.
//
// Idempotent: re-applying for a Bot that already has a project is
// a no-op for the project itself, but always re-pushes the canonical
// role.md file so update_role changes land.
//
// WorkerProject routes the project / git / app calls through the
// ProjectService interface and the file pushes through ProjectGit
// (a thin slice of WorkspaceGit). Production wiring in
// api/pkg/server/helix_org.go satisfies ProjectService with the
// in-process adapter that calls HelixAPIServer handlers directly.
type WorkerProject struct {
	Service ProjectService
	// Workspace owns the on-branch file layout — WorkerProject
	// delegates all file pushes (agent.md / role.md at first apply)
	// through it so there is exactly one place in the helix runtime
	// that knows the `workers/<id>/.context/` / `.context/` path
	// convention.
	Workspace   *Workspace
	Store       *store.Store
	HelixOrgURL string
	OrgID       string
	// OrgDisplayName is the org's human label, used to build the
	// project's display name (`<Bot> @ <Org>`). The host resolves it
	// from the main store and stamps it here; empty falls back to a
	// bare bot label. Both the owner-chat applier and the spawner MUST
	// set it identically — the project is upserted by name, so a
	// divergent value would create a duplicate project.
	OrgDisplayName string
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
	Logger      *slog.Logger
}

// Ensure applies a Helix project for the given Worker if one
// doesn't exist yet. Returns the resolved project / agent-app /
// repo IDs.
//
// The receiver's Service / Store / Workspace fields are all
// required. Misconfiguring any of them used to nil-deref deep
// inside the activation path (project.go:156 in the original
// crash); now they're checked up front so the failure is a
// clear error message instead of a panic.
func (a *WorkerProject) Ensure(ctx context.Context, orgID string, workerID orgchart.BotID) (projectID, agentAppID, repoID string, err error) {
	if a == nil {
		return "", "", "", errors.New("worker project applier is nil")
	}
	if a.Service == nil {
		return "", "", "", errors.New("worker project applier: Service (ProjectService) is nil — host wiring forgot to pass it through")
	}
	if a.Store == nil {
		return "", "", "", errors.New("worker project applier: Store is nil")
	}
	bot, err := a.Store.Bots.Get(ctx, orgID, workerID)
	if err != nil {
		return "", "", "", fmt.Errorf("get bot: %w", err)
	}
	state, err := LoadState(ctx, a.Store, orgID, workerID)
	if err != nil {
		return "", "", "", err
	}
	// The Bot IS the role: its Content is the prompt that lands in
	// role.md, and its ID names the agent app.
	roleContent := bot.Content
	roleName := string(bot.ID)
	runtime := a.Runtime
	if runtime == "" {
		runtime = Runtime
	}
	// Project display name: `<Bot> @ <Org>` (e.g. "Chief of Staff @ Acme")
	// rather than the bare slug. bot.Name may be empty (fall back to the
	// ID); OrgDisplayName may be empty in bare/test wirings (fall back to
	// just the bot label). Deterministic so upsert-by-name stays idempotent.
	botLabel := bot.Name
	if botLabel == "" {
		botLabel = string(bot.ID)
	}
	projectName := botLabel
	if a.OrgDisplayName != "" {
		projectName = fmt.Sprintf("%s @ %s", botLabel, a.OrgDisplayName)
	}
	applyReq := types.ProjectApplyRequest{
		// Scope the project to the org this Ensure call was invoked for,
		// not the struct's OrgID field. They are normally equal, but a
		// caller that reuses a WorkerProject across orgs (or stamps OrgID
		// from frozen config) must not be able to apply one org's project
		// into another — the org parameter is the authority here.
		OrganizationID: orgID,
		Name:           projectName,
		Spec: types.ProjectSpec{
			Description: bot.Content,
			Agent: &types.ProjectAgentSpec{
				Name:        roleName,
				Runtime:     runtime,
				Provider:    a.Provider,
				Model:       a.Model,
				Credentials: a.Credentials,
			},
		},
	}
	if state.ProjectID != "" {
		if _, err := a.Service.GetProject(ctx, state.ProjectID); err != nil {
			if errors.Is(err, ErrProjectNotFound) {
				if clearErr := ClearProject(ctx, a.Store, orgID, workerID); clearErr != nil {
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
			// Project already exists — fast path.
			//
			// We DO re-call ApplyProject so worker.* changes (runtime,
			// credentials, provider, model) made on the Settings page
			// after this worker was first provisioned propagate to the
			// helix-side agent app on the next activation. ApplyProject
			// is upsert-by-name and idempotent — without this re-apply,
			// the agent app's Runtime/Credentials/Provider/Model stay
			// frozen at first-apply time and operators have to fire +
			// re-hire every worker to pick up settings drift. That gap
			// surfaced when the chart UI's owner-chat hit
			// "Authentication required" because the org had been
			// flipped to api_key mode but w-owner's agent app still
			// thought it was in subscription mode.
			//
			if _, err := a.Service.ApplyProject(ctx, applyReq); err != nil {
				return "", "", "", fmt.Errorf("refresh project spec for %s: %w", workerID, err)
			}
			// Re-publish canonical files so DB edits to the bot's content
			// (role.md) and agent.md propagate to the helix-specs branch on
			// every activation — that's the contract DefaultHelixSpecsMandate
			// promises every Bot. Idempotent and cheap: CreateBranch
			// and CreateOrUpdateFileContents both no-op on unchanged input.
			a.republishWorkerFiles(ctx, workerID, state.RepoID, roleContent)
			return state.ProjectID, state.AgentAppID, state.RepoID, nil
		}
	}
	resp, err := a.Service.ApplyProject(ctx, applyReq)
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
	// Helix's project-apply does NOT auto-create a default repo. We
	// MUST create one and attach it as primary, because:
	//
	//   - HELIX_REPOSITORIES is set from the project's attached repos
	//     when hydra launches the desktop; an empty list means the
	//     bringup script has nothing for Zed to open.
	//   - update_role writes role.md into the repo on the helix-specs
	//     branch — without a repo it has nowhere to go.
	//
	// Earlier versions of this code logged a warning on failure and
	// returned a project with empty RepoID, which surfaced as a 5-min
	// desktop timeout on activation. Now we fail loudly: the caller
	// gets a clear error at apply time, the activation queue records
	// it, the operator sees it in the snackbar instead of "Still
	// waiting for external agent to connect".
	if repoID == "" {
		ownerID, _ := a.Service.WhoAmI(ctx)
		if ownerID == "" {
			return "", "", "", fmt.Errorf("apply project for %s: cannot create per-Worker repo — WhoAmI returned empty owner id (host wiring forgot to supply a service user?)", workerID)
		}
		repo, err := a.Service.CreateGitRepo(ctx, types.GitRepositoryCreateRequest{
			Name:           string(workerID),
			OwnerID:        ownerID,
			OrganizationID: projOrgID,
			InitialFiles: map[string]string{
				"README.md": "# " + string(workerID) + "\n\nWorkspace for Helix bot `" + string(workerID) + "`. Files in `job/` carry the bot's role prompt.\n",
			},
		})
		if err != nil {
			return "", "", "", fmt.Errorf("apply project for %s: create per-Worker repo: %w", workerID, err)
		}
		if err := a.Service.AttachRepoToProject(ctx, resp.ProjectID, repo.ID, true); err != nil {
			return "", "", "", fmt.Errorf("apply project for %s: attach repo %s to project %s: %w", workerID, repo.ID, resp.ProjectID, err)
		}
		repoID = repo.ID
		if a.Logger != nil {
			a.Logger.Info("helix repo created and attached", "worker", workerID, "repo", repo.ID)
		}
	}
	a.republishWorkerFiles(ctx, workerID, repoID, roleContent)
	// NB: helix-org MCP attachment is NOT done here. applyProject
	// (helix project handler) wholesale-replaces agentApp.Config.Helix
	// on update, so anything we attach now is clobbered on the next
	// re-apply. The Spawner and dynamicProjectApplier call
	// AttachHelixOrgMCP themselves *after* this Ensure returns — that's
	// the single place MCP mutation lives.
	if err := SaveProject(ctx, a.Store, orgID, workerID, resp.ProjectID, resp.AgentAppID, repoID); err != nil {
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

// republishWorkerFiles writes the bot's role.md file on the bot's
// helix-specs branch through the Workspace, so the on-branch path
// layout is owned in exactly one place (workspace.go). A bot has no
// separate identity — its Content IS its prompt, written to role.md.
// Best-effort: errors are logged, not returned — a single failed file
// shouldn't block the rest of the apply.
func (a *WorkerProject) republishWorkerFiles(ctx context.Context, workerID orgchart.BotID, repoID, roleContent string) {
	if repoID == "" || a.Workspace == nil {
		return
	}
	if err := a.Workspace.EnsureBranch(ctx, repoID, "main"); err != nil {
		if a.Logger != nil {
			a.Logger.Warn("republish bot files: create helix-specs branch", "bot", workerID, "err", err)
		}
	}
	if roleContent != "" {
		if err := a.Workspace.WriteWorkerFile(ctx, workerID, repoID, "role.md", roleContent, "republish role.md"); err != nil && a.Logger != nil {
			a.Logger.Warn("republish bot files: role.md", "bot", workerID, "err", err)
		}
	}
}
