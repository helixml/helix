// Package activations owns the host-driven activation use case behind
// the REST "activate worker" endpoint: pre-allocating the audit row so
// the handler can return an activation id synchronously while the
// Spawner picks the row up (matched by Trigger.ActivationID) and
// completes it. This keeps the api adapter free of a direct
// activation.Repository reference.
package activations

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"time"

	"github.com/helixml/helix/api/pkg/org/domain/activation"
	"github.com/helixml/helix/api/pkg/org/domain/orgchart"
)

// ProjectEnsurer provisions (or fast-paths) a Worker's per-Worker Helix
// project + agent app. The Activate use case runs it synchronously so
// the response carries the project / agent-app ids and the helix-org MCP
// is (re)attached before the session starts.
type ProjectEnsurer interface {
	Ensure(ctx context.Context, orgID string, workerID orgchart.WorkerID) (projectID, agentAppID, repoID string, err error)
}

// ManualDispatcher enqueues an operator-driven activation on the
// per-Worker queue. activationID is the pre-allocated audit-row id;
// empty means the Spawner mints its own.
type ManualDispatcher interface {
	DispatchManual(ctx context.Context, orgID string, workerID orgchart.WorkerID, envPath string, activationID activation.ID)
}

// SessionResolver returns a Worker's current desktop session id (empty
// before the first activation). Used only to populate the Activate
// response so the UI can deep-link straight to the desktop viewer.
type SessionResolver interface {
	SessionID(ctx context.Context, orgID string, workerID orgchart.WorkerID) (string, error)
}

// ErrActivateUnavailable is returned by Activate when the project
// ensurer or dispatcher isn't wired. Adapters map it to 501.
var ErrActivateUnavailable = errors.New("activate is not wired in this deployment")

// Activations owns the host-driven activation use case: the full
// manual-activate command (Activate), which ensures the project,
// pre-allocates the audit row, and enqueues the manual trigger.
type Activations struct {
	repo       activation.Repository
	now        func() time.Time
	newID      func() string
	ensurer    ProjectEnsurer
	dispatcher ManualDispatcher
	sessions   SessionResolver
	envsDir    string
}

// Deps are the constructor-injected collaborators for New. Repo may be
// nil (Activate then skips the audit pre-create, as the old inline code
// did when Activations was unwired). Ensurer + Dispatcher are required
// for Activate (nil → ErrActivateUnavailable); Sessions is optional
// (nil → empty session id in the result).
type Deps struct {
	Repo       activation.Repository
	Now        func() time.Time
	NewID      func() string
	Ensurer    ProjectEnsurer
	Dispatcher ManualDispatcher
	Sessions   SessionResolver
	EnvsDir    string
}

// New constructs the Activations service.
func New(deps Deps) *Activations {
	now := deps.Now
	if now == nil {
		now = func() time.Time { return time.Now().UTC() }
	}
	return &Activations{
		repo:       deps.Repo,
		now:        now,
		newID:      deps.NewID,
		ensurer:    deps.Ensurer,
		dispatcher: deps.Dispatcher,
		sessions:   deps.Sessions,
		envsDir:    deps.EnvsDir,
	}
}

// ActivateResult is what a manual activation returns to the caller: the
// pre-allocated activation id plus the project / agent-app / session ids
// the UI needs to navigate to the desktop.
type ActivateResult struct {
	ActivationID activation.ID
	ProjectID    string
	AgentAppID   string
	SessionID    string
}

// Activate runs the manual "activate worker" command end-to-end:
//
//  1. Synchronously ensure the project + agent app (re-attaches the
//     helix-org MCP — the immediate user-visible fix the operator clicked
//     "Start Desktop" for).
//  2. Read the persisted session id (empty on first activation).
//  3. Pre-allocate the audit row so the response carries an activation id.
//  4. Enqueue on the dispatcher's per-Worker queue (coalesces with any
//     in-flight activation, so a double-click folds into one follow-up).
//
// The worker-id is validated up front because it becomes a path segment
// under EnvsDir (path-injection guard). Callers should still confirm the
// Worker exists (404) before calling — Activate's Ensure will also error
// on a missing Worker, but a pre-check gives the cleaner status.
func (a *Activations) Activate(ctx context.Context, orgID string, workerID orgchart.WorkerID) (ActivateResult, error) {
	if a.ensurer == nil || a.dispatcher == nil {
		return ActivateResult{}, ErrActivateUnavailable
	}
	if err := orgchart.ValidID(string(workerID)); err != nil {
		return ActivateResult{}, fmt.Errorf("worker id: %w", err)
	}
	projectID, agentAppID, _, err := a.ensurer.Ensure(ctx, orgID, workerID)
	if err != nil {
		return ActivateResult{}, fmt.Errorf("ensure project for %s: %w", workerID, err)
	}
	var sessionID string
	if a.sessions != nil {
		sessionID, _ = a.sessions.SessionID(ctx, orgID, workerID)
	}
	activationID, err := a.prepareManual(ctx, orgID, workerID)
	if err != nil {
		return ActivateResult{}, err
	}
	envPath := ""
	if a.envsDir != "" {
		envPath = filepath.Join(a.envsDir, string(workerID))
	}
	a.dispatcher.DispatchManual(ctx, orgID, workerID, envPath, activationID)
	return ActivateResult{
		ActivationID: activationID,
		ProjectID:    projectID,
		AgentAppID:   agentAppID,
		SessionID:    sessionID,
	}, nil
}

// prepareManual pre-allocates a TriggerManual activation audit row for
// the Worker and returns its id. Returns an empty id (and nil error)
// when the repository or id-generator is unwired — Activate then treats
// that as "no pre-allocation; the Spawner mints its own", matching the
// previous inline behaviour.
func (a *Activations) prepareManual(ctx context.Context, orgID string, workerID orgchart.WorkerID) (activation.ID, error) {
	if a.repo == nil || a.newID == nil {
		return "", nil
	}
	id := activation.ID("a-" + a.newID())
	act, err := activation.New(
		id,
		workerID,
		[]activation.Trigger{{Kind: activation.TriggerManual}},
		a.now(),
		orgID,
	)
	if err != nil {
		return "", fmt.Errorf("build manual activation: %w", err)
	}
	if err := a.repo.Create(ctx, act); err != nil {
		return "", fmt.Errorf("persist manual activation: %w", err)
	}
	return id, nil
}
