package helix

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"

	"github.com/helixml/helix/api/pkg/org/domain/orgchart"
	"github.com/helixml/helix/api/pkg/org/domain/store"
	"github.com/helixml/helix/api/pkg/types"
)

// projectEnsureMu serialises project provisioning; repoEnsureMu serialises
// per-Worker repo creation. Two
// activations for the same bot otherwise both observe DefaultRepoID=="" and each
// create+attach a same-named repo — duplicates the desktop workspace setup then
// clones into one path and fails on. A single global lock is fine: activation is
// low-throughput and the critical section is a check-create-attach.
//
// ponytail: this lock is process-local — it only closes the window within one
// API process. It does NOT protect against two API replicas racing the same
// activation (each would still create a repo, and CreateGitRepo auto-increments
// the name rather than erroring, so the loser silently makes `<worker>-2`). The
// real fix is a store-level uniqueness constraint on (owner/org, repo name) so
// the create fails cleanly and the loser re-fetches — that removes this lock
// entirely and holds across replicas. Prod is single-replica today, so the lock
// suffices for now.
var (
	// ponytail: global lock; use per-Bot or database locks if provisioning throughput matters.
	projectEnsureMu sync.Mutex
	repoEnsureMu    sync.Mutex
)

// ErrProjectNotFound is the sentinel a ProjectService impl must return
// when GetProject is called against a project that no longer exists on
// the Helix side. WorkerProject compares with errors.Is — a 404-shaped
// error from the underlying transport must wrap this sentinel so the
// fast-path verification can clear stale state and re-apply.
var ErrProjectNotFound = errors.New("helix project: not found")

// ErrRepoNotFound is the sentinel a ProjectService impl must return (wrapped)
// when GetGitRepo is called against a repo that no longer exists. WorkerProject
// compares with errors.Is to decide whether to re-provision the repo.
var ErrRepoNotFound = errors.New("helix git repo: not found")

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

	// ListProjectSecrets returns the project's dev-scoped secrets as a
	// decrypted name→value map, read live. Backs list_secrets so a bot can
	// pick up a secret added after its container booted.
	ListProjectSecrets(ctx context.Context, projectID string) (map[string]string, error)

	// CreateGitRepo creates a Helix-internal git repository. Used when
	// project-apply doesn't auto-create one.
	CreateGitRepo(ctx context.Context, req types.GitRepositoryCreateRequest) (types.GitRepository, error)

	// GetGitRepo returns a repo by ID. Returns ErrRepoNotFound (wrapped if
	// needed) when the repo no longer exists, so the fast path can detect a
	// deleted repo and re-provision instead of handing back a dead id.
	GetGitRepo(ctx context.Context, repoID string) (types.GitRepository, error)

	// DeleteGitRepo removes a repo by ID. Used to reclaim a just-created repo
	// that could not be attached (or that lost a create race), so a retry
	// doesn't accumulate duplicates. A missing repo is not an error.
	DeleteGitRepo(ctx context.Context, repoID string) error

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
// Idempotent: ensuring a Bot that already has a project is
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
	projectEnsureMu.Lock()
	defer projectEnsureMu.Unlock()

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
		existing, err := a.Service.GetProject(ctx, state.ProjectID)
		if err != nil {
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
			// The project is tracked by ID (state.ProjectID) but ApplyProject
			// upserts by NAME. When the desired display name has drifted from
			// the tracked project's current name — an existing worker whose
			// project predates the `<Bot> @ <Org>` scheme, or an org/bot
			// rename — a bare ApplyProject would match nothing and FORK an
			// orphan project. Rename in place
			// by ID first so ApplyProject matches the same project.
			if existing.Name != projectName {
				renamed := projectName
				if _, err := a.Service.UpdateProject(ctx, state.ProjectID, types.ProjectUpdateRequest{Name: &renamed}); err != nil {
					// Don't fork: skip the by-name refresh this activation and
					// keep the worker running on its existing project. Retries
					// next activation.
					if a.Logger != nil {
						a.Logger.Warn("project applier: rename to display name failed, skipping refresh to avoid orphan", "worker", workerID, "project", state.ProjectID, "want_name", projectName, "err", err)
					}
					a.republishWorkerFiles(ctx, workerID, state.RepoID, roleContent)
					return state.ProjectID, state.AgentAppID, state.RepoID, nil
				}
			}
			if !existing.Metadata.OrgMembersAccess {
				existing.Metadata.OrgMembersAccess = true
				if _, err := a.Service.UpdateProject(ctx, state.ProjectID, types.ProjectUpdateRequest{Metadata: &existing.Metadata}); err != nil {
					return "", "", "", fmt.Errorf("enable org member access to project %s for %s: %w", state.ProjectID, workerID, err)
				}
			}
			// Runtime/model configuration is owned by the generated Helix app
			// after initial provisioning. Do not re-apply the provisioning
			// spec here: doing so would overwrite edits made through the app
			// settings UI/API with worker.* defaults on every bot start.
			// Self-heal a deleted repo: the project is live but its repo may
			// have been removed out-of-band. The project self-heals above (via
			// ErrProjectNotFound); give the repo the same treatment so the
			// worker isn't left pointing at a dead repo id. Only re-provision
			// on a definitive not-found — a transient read error keeps the
			// existing id rather than risk a needless recreate.
			repoID := state.RepoID
			if repoID != "" {
				if _, gerr := a.Service.GetGitRepo(ctx, repoID); errors.Is(gerr, ErrRepoNotFound) {
					if a.Logger != nil {
						a.Logger.Info("project applier: persisted repo missing, re-provisioning", "worker", workerID, "stale_repo_id", repoID)
					}
					newRepoID, rerr := a.ensureWorkerRepo(ctx, state.ProjectID, orgID, workerID)
					if rerr != nil {
						return "", "", "", fmt.Errorf("re-provision missing repo for %s: %w", workerID, rerr)
					}
					repoID = newRepoID
					if serr := SaveProject(ctx, a.Store, orgID, workerID, state.ProjectID, state.AgentAppID, repoID); serr != nil {
						return "", "", "", fmt.Errorf("persist re-provisioned repo for %s: %w", workerID, serr)
					}
				} else if gerr != nil && a.Logger != nil {
					a.Logger.Warn("verify persisted repo failed; keeping existing id", "worker", workerID, "repo", repoID, "err", gerr)
				}
			}
			// Re-publish canonical files so DB edits to the bot's content
			// (role.md) and agent.md propagate to the helix-specs branch on
			// every activation — that's the contract DefaultHelixSpecsMandate
			// promises every Bot. Idempotent and cheap: CreateBranch
			// and CreateOrUpdateFileContents both no-op on unchanged input.
			a.republishWorkerFiles(ctx, workerID, repoID, roleContent)
			return state.ProjectID, state.AgentAppID, repoID, nil
		}
	}
	resp, err := a.Service.ApplyProject(ctx, applyReq)
	if err != nil {
		return "", "", "", fmt.Errorf("apply project for %s: %w", workerID, err)
	}
	project, err := a.Service.GetProject(ctx, resp.ProjectID)
	if err != nil {
		return "", "", "", fmt.Errorf("get applied project %s for %s: %w", resp.ProjectID, workerID, err)
	}
	if !project.Metadata.OrgMembersAccess {
		project.Metadata.OrgMembersAccess = true
		project, err = a.Service.UpdateProject(ctx, resp.ProjectID, types.ProjectUpdateRequest{Metadata: &project.Metadata})
		if err != nil {
			return "", "", "", fmt.Errorf("enable org member access to project %s for %s: %w", resp.ProjectID, workerID, err)
		}
	}
	// Project secrets — env-var injection. The worker needs these to reach
	// the org runtime, so a failure is worth surfacing (logged, not fatal:
	// a re-apply on the next activation retries the upsert).
	if err := a.Service.PutProjectSecret(ctx, resp.ProjectID, "HELIX_ORG_URL", a.HelixOrgURL); err != nil && a.Logger != nil {
		a.Logger.Warn("put project secret HELIX_ORG_URL", "worker", workerID, "project", resp.ProjectID, "err", err)
	}
	if err := a.Service.PutProjectSecret(ctx, resp.ProjectID, "HELIX_WORKER_ID", string(workerID)); err != nil && a.Logger != nil {
		a.Logger.Warn("put project secret HELIX_WORKER_ID", "worker", workerID, "project", resp.ProjectID, "err", err)
	}
	repoID = project.DefaultRepoID
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
		// Use the authoritative orgID (the org the project was applied into),
		// NOT an org read back from GetProject — that read may have failed
		// above, leaving an empty org that would create an org-less repo and
		// then be rejected by AttachRepoToProject ("must be in the same org"),
		// leaking the just-created repo.
		var rerr error
		repoID, rerr = a.ensureWorkerRepo(ctx, resp.ProjectID, orgID, workerID)
		if rerr != nil {
			return "", "", "", fmt.Errorf("apply project for %s: %w", workerID, rerr)
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

// ensureWorkerRepo returns the project's per-Worker repo, creating and attaching
// one only if the project truly has none. Serialised on repoEnsureMu and
// re-checks GetProject inside the lock, so two concurrent activations for the
// same bot cannot each create a duplicate same-named repo (which the desktop
// workspace setup would then clone into the same path and fail on).
func (a *WorkerProject) ensureWorkerRepo(ctx context.Context, projectID, orgID string, workerID orgchart.BotID) (string, error) {
	repoEnsureMu.Lock()
	defer repoEnsureMu.Unlock()
	// A concurrent activation may have created+attached the repo since the
	// caller read DefaultRepoID; re-check under the lock before creating.
	if proj, err := a.Service.GetProject(ctx, projectID); err == nil && proj.DefaultRepoID != "" {
		// Trust the attached repo only if it still exists — a DefaultRepoID
		// pointing at a deleted repo must be re-provisioned, not handed back.
		// On a transient (non-not-found) read error, do NOT fall through to
		// create: that would risk a duplicate. Surface it and let the caller
		// retry.
		if _, gerr := a.Service.GetGitRepo(ctx, proj.DefaultRepoID); gerr == nil {
			return proj.DefaultRepoID, nil
		} else if !errors.Is(gerr, ErrRepoNotFound) {
			return "", fmt.Errorf("verify attached repo %s: %w", proj.DefaultRepoID, gerr)
		}
	}
	ownerID, err := a.Service.WhoAmI(ctx)
	if err != nil {
		return "", fmt.Errorf("cannot create per-Worker repo — WhoAmI failed: %w", err)
	}
	if ownerID == "" {
		return "", fmt.Errorf("cannot create per-Worker repo — WhoAmI returned empty owner id (host wiring forgot to supply a service user?)")
	}
	repo, err := a.Service.CreateGitRepo(ctx, types.GitRepositoryCreateRequest{
		Name:           string(workerID),
		OwnerID:        ownerID,
		OrganizationID: orgID,
		InitialFiles: map[string]string{
			"README.md": "# " + string(workerID) + "\n\nWorkspace for Helix bot `" + string(workerID) + "`. Files in `job/` carry the bot's role prompt.\n",
		},
	})
	if err != nil {
		return "", fmt.Errorf("create per-Worker repo: %w", err)
	}
	// CreateGitRepo auto-increments the name on collision (`<worker>` ->
	// `<worker>-2`) rather than erroring. A name we didn't ask for means a
	// same-named repo already existed — i.e. another process (a second API
	// replica, which repoEnsureMu can't serialise) won the create race. Don't
	// keep the duplicate: delete it and error, so the caller retries and the
	// outer re-check picks up the winner's now-attached repo.
	if repo.Name != string(workerID) {
		if derr := a.Service.DeleteGitRepo(ctx, repo.ID); derr != nil && a.Logger != nil {
			a.Logger.Warn("delete raced duplicate repo", "worker", workerID, "repo", repo.ID, "err", derr)
		}
		return "", fmt.Errorf("per-Worker repo %q already exists (create race); retrying", string(workerID))
	}
	if err := a.Service.AttachRepoToProject(ctx, projectID, repo.ID, true); err != nil {
		// The repo was created but couldn't be attached — it's an orphan with
		// nothing pointing at it. Delete it so a retry starts clean instead of
		// creating `<worker>-2` alongside the leaked one.
		if derr := a.Service.DeleteGitRepo(ctx, repo.ID); derr != nil && a.Logger != nil {
			a.Logger.Warn("delete orphaned repo after failed attach", "worker", workerID, "repo", repo.ID, "err", derr)
		}
		return "", fmt.Errorf("attach repo %s to project %s: %w", repo.ID, projectID, err)
	}
	if a.Logger != nil {
		a.Logger.Info("helix repo created and attached", "worker", workerID, "repo", repo.ID)
	}
	return repo.ID, nil
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
