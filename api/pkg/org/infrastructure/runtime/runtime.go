// Package runtime owns the ports describing where an AI Worker
// physically executes. Two contracts live here:
//
//   - Spawner: run a single activation and block until the agent exits.
//   - WorkspaceSync: mirror canonical Role / Identity content into
//     the runtime's per-Worker workspace.
//
// Wired separately by callers (dispatcher → Spawner; tools.Deps →
// WorkspaceSync). The sole concrete runtime lives at
// api/pkg/org/infrastructure/runtime/helix/.
package runtime

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/helixml/helix/api/pkg/org/domain/activation"
	"github.com/helixml/helix/api/pkg/org/domain/orgchart"
)

// Spawner runs an AI Worker's agent process for a single activation
// and BLOCKS until the process exits. The triggers slice tells the
// Spawner (and through it, the agent) why this activation is happening
// — first hire, or one or more events on subscribed Topics that
// arrived while a previous activation was running. The Dispatcher
// coalesces bursts so the slice is usually length 1, but the agent
// must handle longer slices when traffic queues up.
//
// Spawners are typically called from inside a Dispatcher that
// serialises calls per-Worker; callers must not invoke a Spawner for
// the same Worker concurrently.
//
// The zero value — nil — means "no process will be spawned", which
// is correct for tests and for HumanWorker activations.
type Spawner func(ctx context.Context, orgID string, workerID orgchart.BotID, triggers []activation.Trigger) error

// WorkspaceSync mirrors the canonical Role and Identity content of a
// Worker into wherever that Worker's runtime reads them at activation
// time. Tools (update_role, update_identity) call MirrorFile after
// persisting to the DB so the next activation sees fresh content
// without waiting for the spawner's projection step.
//
// `name` is a logical filename for this Worker — typically "role.md"
// or "identity.md". The backend maps the name to its own on-target
// layout; callers must NOT include backend-specific path prefixes
// (no "workers/<id>/.context/...", no "job/..."). The mapping today
// (helix runtime, the sole concrete impl):
//
//   - workers/<workerID>/.context/<name> on the helix-specs branch
//     of the Worker's per-Worker repo (matches what
//     `WorkerProject.republishWorkerFiles` writes and what the
//     activation mandate tells the agent to `git pull` and `cat`)
//
// `name` must be a clean, single-segment-or-relative filename — no
// leading slash, no "..", no escape from the Worker's namespace.
//
// Workers that aren't yet provisioned in the runtime backend (e.g.
// a Helix Worker before its first activation creates the project)
// are safe no-ops — implementations skip the mirror and return nil.
//
// Naming: see ADR-0001 §7 — MirrorFile, not PublishFile. "Publish"
// is reserved for the MCP-tool sense ("append an Event to a Topic").
type WorkspaceSync interface {
	MirrorFile(ctx context.Context, orgID string, workerID orgchart.BotID, name, content, message string) error
}

// NoopWorkspaceSync is a WorkspaceSync that does nothing. Useful for
// tests and for backends that have no out-of-band mirror surface.
type NoopWorkspaceSync struct{}

// MirrorFile is the no-op WorkspaceSync: ignore the call and return nil.
func (NoopWorkspaceSync) MirrorFile(_ context.Context, _ string, _ orgchart.BotID, _, _, _ string) error {
	return nil
}

// ValidateWorkspaceName enforces the WorkspaceSync `name` contract —
// shared by every WorkspaceSync implementation so callers see the
// same rejection rules regardless of backend. Kept exported so
// future out-of-tree backends share the same enforcement.
func ValidateWorkspaceName(name string) error {
	if name == "" {
		return errors.New("workspace name is empty")
	}
	if strings.HasPrefix(name, "/") {
		return fmt.Errorf("workspace name %q is absolute", name)
	}
	for _, seg := range strings.Split(name, "/") {
		if seg == ".." {
			return fmt.Errorf("workspace name %q traverses upward", name)
		}
	}
	return nil
}

// HireHook runs runtime-side bookkeeping immediately after a Worker
// is created. It's a single-method port — one publisher (the hire
// tool), one subscriber per runtime backend (helix-runtime records
// the hiring user; claude-runtime no-ops). Not an event bus: there is
// no fan-out, no second subscriber on the horizon, and the wiring
// point picks the right implementation at construction time.
//
// hiringUserID is the upstream caller's identifier captured from
// request context — typically a Helix user_id. Empty means
// "unauthenticated context" (standalone helix-org, MCP without a
// stashed user); implementations should treat that as a no-op rather
// than an error.
//
// An OnHire error is fatal to the hire today (matches existing
// behaviour at helix-org/tools/hire_worker.go:217-222 where the same
// SaveHiringUser call returns a wrapped error). Document the trade-off
// at the call site.
type HireHook interface {
	OnHire(ctx context.Context, orgID string, workerID orgchart.BotID, hiringUserID string) error
}

// NoopHireHook is a HireHook that does nothing. Useful for
// tests and for dev runtimes (claude) that don't need per-hire
// runtime-side state.
type NoopHireHook struct{}

// OnHire is the no-op HireHook: ignore the call and return nil.
func (NoopHireHook) OnHire(_ context.Context, _ string, _ orgchart.BotID, _ string) error {
	return nil
}

// ProjectConfig is the port tools use to read and patch a Worker's
// per-Worker Helix project configuration — the startup script today,
// and any other field on Helix's ProjectUpdateRequest tomorrow
// (skills, guidelines, default agent app, …).
//
// Implementations key by orgID + workerID (the operator-facing
// identifier on the org chart); the helix runtime impl resolves
// worker→projectID via BotRuntimeState internally so MCP tool
// callers never see project IDs. Other runtimes (claude, dev) plug
// in NoopProjectConfig — the configure_worker_project tool reports
// "not supported on this runtime" when invoked.
//
// Patch semantics: only fields set on the patch are written; nil
// fields leave the underlying value alone. Used by
// configure_worker_project for partial updates from chat
// ("change the startup script but leave skills alone").
type ProjectConfig interface {
	GetWorkerProjectConfig(ctx context.Context, orgID string, workerID orgchart.BotID) (ProjectConfigSnapshot, error)
	UpdateWorkerProjectConfig(ctx context.Context, orgID string, workerID orgchart.BotID, patch ProjectConfigPatch) (ProjectConfigSnapshot, error)
}

// ProjectConfigSnapshot is the read shape returned by
// ProjectConfig.GetWorkerProjectConfig. ProjectID is exposed so
// the UI / debug tools can show it; MCP tool callers typically
// ignore it (they referenced the worker, not the project).
//
// Each new field on Helix's Project that we want to surface via
// the MCP tools gets added here. Field additions are append-only
// from the JSON wire format (existing tool clients keep working).
type ProjectConfigSnapshot struct {
	ProjectID     string `json:"project_id"`
	StartupScript string `json:"startup_script"`
}

// ProjectConfigPatch is the write shape passed to
// ProjectConfig.UpdateWorkerProjectConfig. Pointer fields = "only
// touch what's set"; nil fields leave the underlying value alone.
// Mirrors Helix's `types.ProjectUpdateRequest` pointer convention.
type ProjectConfigPatch struct {
	StartupScript *string `json:"startup_script,omitempty"`
}

// NoopProjectConfig satisfies ProjectConfig without doing anything
// — the default for tools.Deps so production code paths that
// don't wire a real impl don't crash. The MCP tools surface a
// clear "not configured" error rather than corrupt data.
type NoopProjectConfig struct{}

func (NoopProjectConfig) GetWorkerProjectConfig(_ context.Context, _ string, _ orgchart.BotID) (ProjectConfigSnapshot, error) {
	return ProjectConfigSnapshot{}, ErrProjectConfigUnsupported
}

func (NoopProjectConfig) UpdateWorkerProjectConfig(_ context.Context, _ string, _ orgchart.BotID, _ ProjectConfigPatch) (ProjectConfigSnapshot, error) {
	return ProjectConfigSnapshot{}, ErrProjectConfigUnsupported
}

// ErrProjectConfigUnsupported is what the noop impl returns. Tools
// translate it into a friendly snackbar / MCP error.
var ErrProjectConfigUnsupported = errors.New("project config access not wired on this runtime")

// SpecTasks is the port the org MCP spec-task tools use to manage the
// spec tasks in a Worker's own Helix project. Its verbs mirror the
// actions a human project manager performs in the UI — create, review,
// approve, request changes, open PRs — rather than generic CRUD.
//
// Implementations key by orgID + workerID; the helix runtime impl
// resolves worker→projectID via WorkerRuntimeState internally. Every verb
// also takes an optional projectID: empty means "the Worker's own
// project" (the original behaviour — a Worker managing its own tasks); a
// non-empty projectID targets another project the caller manages, and the
// impl MUST assert that project belongs to the caller's org (a hard
// cross-org block) before acting. This is what lets an org-wide project
// manager Bot drive spec tasks across several projects in its org. Other
// runtimes plug in NoopSpecTasks, and the tools surface
// ErrSpecTasksUnsupported.
type SpecTasks interface {
	// Create makes a new spec task in the target project (status
	// backlog). Mirrors the REST create-from-prompt path.
	Create(ctx context.Context, orgID string, workerID orgchart.BotID, projectID string, in CreateSpecTaskInput) (SpecTaskView, error)
	// List returns the target project's spec tasks, optionally filtered.
	List(ctx context.Context, orgID string, workerID orgchart.BotID, projectID string, filter ListSpecTasksFilter) ([]SpecTaskView, error)
	// Get returns one spec task; it must belong to the target project.
	Get(ctx context.Context, orgID string, workerID orgchart.BotID, projectID, taskID string) (SpecTaskView, error)
	// Update changes a task's editable metadata without bypassing its
	// lifecycle workflow.
	Update(ctx context.Context, orgID string, workerID orgchart.BotID, projectID, taskID string, in UpdateSpecTaskInput) (SpecTaskView, error)
	// StartPlanning begins spec generation (or queues implementation
	// when the task is in skip-planning / just-do-it mode).
	StartPlanning(ctx context.Context, orgID string, workerID orgchart.BotID, projectID, taskID string) (SpecTaskView, error)
	// StopAgent stops the task's running desktop, if any. It leaves the task
	// and session records intact so work can be resumed.
	StopAgent(ctx context.Context, orgID string, workerID orgchart.BotID, projectID, taskID string) (SpecTaskView, error)
	// ReviewSpec returns the generated requirements/design/tasks for the
	// caller to review before approving or requesting changes.
	ReviewSpec(ctx context.Context, orgID string, workerID orgchart.BotID, projectID, taskID string) (SpecReviewView, error)
	// ApproveSpec approves the generated spec, advancing the task toward
	// implementation.
	ApproveSpec(ctx context.Context, orgID string, workerID orgchart.BotID, projectID, taskID string) (SpecTaskView, error)
	// RequestChanges sends the spec back for revision with a comment.
	RequestChanges(ctx context.Context, orgID string, workerID orgchart.BotID, projectID, taskID, comment string) (SpecTaskView, error)
	// CreatePullRequests tells the system the code is good and to open
	// the pull request(s) — one per repo attached to the project. It
	// does NOT merge/approve on GitHub.
	CreatePullRequests(ctx context.Context, orgID string, workerID orgchart.BotID, projectID, taskID string) (SpecTaskView, error)
}

// CreateSpecTaskInput is the create shape. Only Name and Description are
// required; the rest mirror the optional fields the Optimus skill /
// REST create accept.
type CreateSpecTaskInput struct {
	Name           string   `json:"name"`
	Description    string   `json:"description"`
	Type           string   `json:"type,omitempty"`     // feature | bug | refactor (default feature)
	Priority       string   `json:"priority,omitempty"` // low | medium | high | critical (default medium)
	OriginalPrompt string   `json:"original_prompt,omitempty"`
	SkipPlanning   bool     `json:"skip_planning,omitempty"`
	DependsOn      []string `json:"depends_on,omitempty"`
}

// UpdateSpecTaskInput is the safe metadata-edit surface. Lifecycle state
// changes use the dedicated start/review/approve/stop verbs instead.
type UpdateSpecTaskInput struct {
	Name         *string   `json:"name,omitempty"`
	Description  *string   `json:"description,omitempty"`
	Type         *string   `json:"type,omitempty"`
	Priority     *string   `json:"priority,omitempty"`
	SkipPlanning *bool     `json:"skip_planning,omitempty"`
	DependsOn    *[]string `json:"depends_on,omitempty"`
}

// ListSpecTasksFilter narrows a List call. Empty fields = no filter.
type ListSpecTasksFilter struct {
	Status   string `json:"status,omitempty"`
	Priority string `json:"priority,omitempty"`
	Type     string `json:"type,omitempty"`
}

// SpecTaskView is the tool-facing projection of a spec task. Append-only
// from the JSON wire format so existing tool clients keep working.
type SpecTaskView struct {
	ID           string            `json:"id"`
	Name         string            `json:"name"`
	Description  string            `json:"description,omitempty"`
	Status       string            `json:"status"`
	Priority     string            `json:"priority,omitempty"`
	Type         string            `json:"type,omitempty"`
	BranchName   string            `json:"branch_name,omitempty"`
	PullRequests []PullRequestView `json:"pull_requests,omitempty"`
}

// PullRequestView is one PR opened for a task's repo. CreatePullRequests
// can produce several (one per attached repo).
type PullRequestView struct {
	RepositoryName string `json:"repository_name,omitempty"`
	URL            string `json:"url,omitempty"`
	State          string `json:"state,omitempty"`
}

// SpecReviewView is the read shape for ReviewSpec — the generated spec
// documents plus the task's current status so the reviewer has context.
type SpecReviewView struct {
	TaskID       string `json:"task_id"`
	Status       string `json:"status"`
	Requirements string `json:"requirements,omitempty"`
	Design       string `json:"design,omitempty"`
	Tasks        string `json:"tasks,omitempty"`
}

// NoopSpecTasks satisfies SpecTasks without doing anything — the default
// for tools.Deps so production paths that don't wire a real impl don't
// crash. Every verb returns ErrSpecTasksUnsupported.
type NoopSpecTasks struct{}

func (NoopSpecTasks) Create(_ context.Context, _ string, _ orgchart.BotID, _ string, _ CreateSpecTaskInput) (SpecTaskView, error) {
	return SpecTaskView{}, ErrSpecTasksUnsupported
}
func (NoopSpecTasks) List(_ context.Context, _ string, _ orgchart.BotID, _ string, _ ListSpecTasksFilter) ([]SpecTaskView, error) {
	return nil, ErrSpecTasksUnsupported
}
func (NoopSpecTasks) Get(_ context.Context, _ string, _ orgchart.BotID, _, _ string) (SpecTaskView, error) {
	return SpecTaskView{}, ErrSpecTasksUnsupported
}
func (NoopSpecTasks) Update(_ context.Context, _ string, _ orgchart.BotID, _, _ string, _ UpdateSpecTaskInput) (SpecTaskView, error) {
	return SpecTaskView{}, ErrSpecTasksUnsupported
}
func (NoopSpecTasks) StartPlanning(_ context.Context, _ string, _ orgchart.BotID, _, _ string) (SpecTaskView, error) {
	return SpecTaskView{}, ErrSpecTasksUnsupported
}
func (NoopSpecTasks) StopAgent(_ context.Context, _ string, _ orgchart.BotID, _, _ string) (SpecTaskView, error) {
	return SpecTaskView{}, ErrSpecTasksUnsupported
}
func (NoopSpecTasks) ReviewSpec(_ context.Context, _ string, _ orgchart.BotID, _, _ string) (SpecReviewView, error) {
	return SpecReviewView{}, ErrSpecTasksUnsupported
}
func (NoopSpecTasks) ApproveSpec(_ context.Context, _ string, _ orgchart.BotID, _, _ string) (SpecTaskView, error) {
	return SpecTaskView{}, ErrSpecTasksUnsupported
}
func (NoopSpecTasks) RequestChanges(_ context.Context, _ string, _ orgchart.BotID, _, _, _ string) (SpecTaskView, error) {
	return SpecTaskView{}, ErrSpecTasksUnsupported
}
func (NoopSpecTasks) CreatePullRequests(_ context.Context, _ string, _ orgchart.BotID, _, _ string) (SpecTaskView, error) {
	return SpecTaskView{}, ErrSpecTasksUnsupported
}

// ErrSpecTasksUnsupported is what the noop impl returns. Tools translate
// it into a friendly MCP error.
var ErrSpecTasksUnsupported = errors.New("spec task access not wired on this runtime")

// Projects is the port the org MCP project-discovery tools use to list
// and read the Helix projects in the caller's org — so an org-wide
// project-manager Bot can discover which projects exist before deciding
// which to manage. Reads are ALWAYS scoped to the caller's org: List
// returns only that org's projects, and Get asserts the project belongs
// to the org (a project id from another tenant returns an error, never
// another org's data). Other runtimes plug in NoopProjects and the tools
// surface ErrProjectsUnsupported.
type Projects interface {
	// List returns the projects in the caller's org.
	List(ctx context.Context, orgID string) ([]ProjectView, error)
	// Get returns one project by id; it must belong to the caller's org.
	Get(ctx context.Context, orgID, projectID string) (ProjectView, error)
}

// ProjectView is the tool-facing projection of a Helix project. Append-only
// from the JSON wire format so existing tool clients keep working.
type ProjectView struct {
	ID             string `json:"id"`
	Name           string `json:"name"`
	Description    string `json:"description,omitempty"`
	Status         string `json:"status,omitempty"`
	DefaultRepoID  string `json:"default_repo_id,omitempty"`
	DefaultAgentID string `json:"default_helix_app_id,omitempty"`
}

// NoopProjects satisfies Projects without doing anything — the default for
// tools.Deps so production paths that don't wire a real impl don't crash.
type NoopProjects struct{}

func (NoopProjects) List(_ context.Context, _ string) ([]ProjectView, error) {
	return nil, ErrProjectsUnsupported
}
func (NoopProjects) Get(_ context.Context, _, _ string) (ProjectView, error) {
	return ProjectView{}, ErrProjectsUnsupported
}

// ErrProjectsUnsupported is what the noop impl returns. Tools translate it
// into a friendly MCP error.
var ErrProjectsUnsupported = errors.New("project access not wired on this runtime")

// Repositories is the port the org MCP repository tools use to list the
// Helix git repositories in the caller's org and attach/detach them on a
// Bot's project so that Bot's sandbox can clone and work in them.
//
// List is always org-scoped. Attach/Detach/ListForBot resolve the Bot's
// Helix project via runtime state (a Bot with no project yet — never
// activated — returns ErrBotProjectNotReady). Other runtimes plug in
// NoopRepositories and the tools surface ErrRepositoriesUnsupported.
type Repositories interface {
	// List returns every git repository belonging to the org.
	List(ctx context.Context, orgID string) ([]RepoView, error)
	// ListForBot returns the repositories currently attached to the Bot's
	// Helix project. Primary is marked when the project default_repo_id
	// matches.
	ListForBot(ctx context.Context, orgID string, botID orgchart.BotID) ([]RepoView, error)
	// AttachToBot attaches an org repository to the Bot's project.
	// primary=true also sets it as the project's default/primary repo.
	AttachToBot(ctx context.Context, orgID string, botID orgchart.BotID, repoID string, primary bool) ([]RepoView, error)
	// DetachFromBot removes a repository from the Bot's project.
	DetachFromBot(ctx context.Context, orgID string, botID orgchart.BotID, repoID string) ([]RepoView, error)
}

// RepoView is the tool-facing projection of a Helix git repository.
// Append-only from the JSON wire format.
type RepoView struct {
	ID            string `json:"id"`
	Name          string `json:"name"`
	Description   string `json:"description,omitempty"`
	CloneURL      string `json:"clone_url,omitempty"`
	ExternalURL   string `json:"external_url,omitempty"`
	ExternalType  string `json:"external_type,omitempty"`
	IsExternal    bool   `json:"is_external,omitempty"`
	DefaultBranch string `json:"default_branch,omitempty"`
	// Primary is only meaningful when the view is returned for a Bot
	// (ListForBot / Attach / Detach): true when this repo is the project's
	// default_repo_id.
	Primary bool `json:"primary,omitempty"`
}

// NoopRepositories is the default for tools.Deps so unwired runtimes
// don't crash.
type NoopRepositories struct{}

func (NoopRepositories) List(_ context.Context, _ string) ([]RepoView, error) {
	return nil, ErrRepositoriesUnsupported
}
func (NoopRepositories) ListForBot(_ context.Context, _ string, _ orgchart.BotID) ([]RepoView, error) {
	return nil, ErrRepositoriesUnsupported
}
func (NoopRepositories) AttachToBot(_ context.Context, _ string, _ orgchart.BotID, _ string, _ bool) ([]RepoView, error) {
	return nil, ErrRepositoriesUnsupported
}
func (NoopRepositories) DetachFromBot(_ context.Context, _ string, _ orgchart.BotID, _ string) ([]RepoView, error) {
	return nil, ErrRepositoriesUnsupported
}

// ErrRepositoriesUnsupported is what the noop impl returns.
var ErrRepositoriesUnsupported = errors.New("repository access not wired on this runtime")

// ErrBotProjectNotReady means the Bot has no Helix project yet (typically
// never activated). Tools surface a clear "activate the bot first" message.
var ErrBotProjectNotReady = errors.New("bot has no helix project yet — activate it first")
