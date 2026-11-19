// Package activations owns the host-driven activation use case behind
// the REST "activate worker" endpoint: pre-allocating the audit row so
// the handler can return an activation id synchronously while the
// Spawner picks the row up (matched by Trigger.ActivationID) and
// completes it. This keeps the api adapter free of a direct
// activation.Repository reference.
package activations

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/helixml/helix/api/pkg/org/domain/activation"
	"github.com/helixml/helix/api/pkg/org/domain/audit"
	"github.com/helixml/helix/api/pkg/org/domain/orgchart"
	"github.com/helixml/helix/api/pkg/org/domain/seedprompts"
)

// ProjectEnsurer provisions (or fast-paths) a Worker's per-Worker Helix
// project + agent app. The Activate use case runs it synchronously so
// the response carries the project / agent-app ids and the helix-org MCP
// is (re)attached before the session starts.
type ProjectEnsurer interface {
	Ensure(ctx context.Context, orgID string, workerID orgchart.NodeID) (projectID, agentAppID, repoID string, err error)
}

// Dispatcher enqueues activations on the per-Worker queue. activationID
// is the pre-allocated audit-row id; empty means the Spawner mints its own.
type Dispatcher interface {
	DispatchHire(ctx context.Context, orgID string, workerID orgchart.NodeID, activationID activation.ID)
	DispatchManual(ctx context.Context, orgID string, workerID orgchart.NodeID, activationID activation.ID)
}

// SessionResolver returns a Worker's current desktop session id (empty
// before the first activation). Used to populate the Activate response
// and to resolve the target of Stop / Restart.
type SessionResolver interface {
	SessionID(ctx context.Context, orgID string, workerID orgchart.NodeID) (string, error)
}

type SessionProgress interface {
	LatestSuccessfulTurnStartedAt(ctx context.Context, orgID string, workerID orgchart.NodeID) (time.Time, error)
}

// DesktopStopper stops a bot's desktop container without deleting the
// session row (transcript survives). Used by Stop.
type DesktopStopper interface {
	StopDesktop(ctx context.Context, sessionID string) error
}

// SessionResetter fully tears down a bot's current session (stop desktop
// → delete session row → clear the persisted pointer) so Reset's
// follow-up Activate provisions a brand-new session. The bot home is not
// part of the session and survives it.
type SessionResetter interface {
	ResetSession(ctx context.Context, orgID string, workerID orgchart.NodeID, sessionID string) error
}

type OutstandingCanceller interface {
	CancelOutstanding(ctx context.Context, orgID string, workerID orgchart.NodeID) error
}

// RuntimeRestarter recreates a bot's desktop container on its existing
// session: the old container is removed first, turns it was running are
// settled as interrupted so their activations replay under the same delivery
// id, and the new container mounts the same bot home and resumes the thread.
// Queued activations are untouched.
type RuntimeRestarter interface {
	RestartRuntime(ctx context.Context, orgID string, workerID orgchart.NodeID) error
}

// BotHomeEraser manages the bot-owned home (/home/retro/work) for Reset.
type BotHomeEraser interface {
	// HomeMayBeInSession reports whether the bot home may still live in
	// the session workspace (not yet migrated, or migration not confirmed).
	// While it does, deleting the session would delete the home, so no
	// reset scope can honour its declaration and every reset is refused.
	HomeMayBeInSession(ctx context.Context, orgID string, workerID orgchart.NodeID) (bool, error)
	// RetireHome moves the home to rollback storage, purged after retention.
	RetireHome(ctx context.Context, orgID string, workerID orgchart.NodeID) (HomeErasure, error)
}

// HomeErasure reports what happened to the bot home.
type HomeErasure struct {
	Existed     bool   `json:"existed"`
	Generation  string `json:"generation,omitempty"`
	RetiredPath string `json:"retired_path,omitempty"`
}

// ErrActivateUnavailable is returned by Activate when the project
// ensurer or dispatcher isn't wired. Adapters map it to 501.
var ErrActivateUnavailable = errors.New("activate is not wired in this deployment")

// ErrStopUnavailable is returned by Stop when the desktop stopper isn't
// wired. Adapters map it to 501.
var ErrStopUnavailable = errors.New("stop is not wired in this deployment")

// ErrRestartUnavailable is returned by Restart when no runtime restarter is
// wired. Adapters map it to 501.
var ErrRestartUnavailable = errors.New("restart is not wired in this deployment")

// ErrResetUnavailable is returned by Reset when its collaborators are not
// wired. Adapters map it to 501.
var ErrResetUnavailable = errors.New("reset is not wired in this deployment")

// ErrInvalidReset is returned for an unknown scope or a missing
// confirmation. Adapters map it to 400.
var ErrInvalidReset = errors.New("invalid reset request")

// ErrResetRefused is returned when a session-only reset would also delete a
// bot home that still lives in the session workspace. Adapters map it to 409.
var ErrResetRefused = errors.New("reset refused")

// Activations owns the host-driven agent lifecycle commands: Activate
// (start), Stop, Restart (non-destructive) and Reset (explicit,
// destructive) — shared by the REST endpoints and the MCP start_bot /
// stop_bot / restart_bot / reset_bot tools.
type Activations struct {
	repo       activation.Repository
	now        func() time.Time
	newID      func() string
	ensurer    ProjectEnsurer
	dispatcher Dispatcher
	sessions   SessionResolver
	progress   SessionProgress
	stopper    DesktopStopper
	resetter   SessionResetter
	canceller  OutstandingCanceller
	restarter  RuntimeRestarter
	homes      BotHomeEraser
	audit      audit.Recorder

	// botLocks serialises Stop, Restart and Reset of one bot in this
	// process, so a reset cannot interleave with a restart of the same bot.
	botLocks sync.Map
}

// Deps are the constructor-injected collaborators for New. Repo may be
// nil (Activate then skips the audit pre-create, as the old inline code
// did when Activations was unwired). Ensurer + Dispatcher are required
// for Activate (nil → ErrActivateUnavailable); Sessions is optional
// (nil → empty session id in the result). Stopper / Resetter are
// required for Stop / Restart respectively.
type Deps struct {
	Repo       activation.Repository
	Now        func() time.Time
	NewID      func() string
	Ensurer    ProjectEnsurer
	Dispatcher Dispatcher
	Sessions   SessionResolver
	Progress   SessionProgress
	Stopper    DesktopStopper
	Resetter   SessionResetter
	Canceller  OutstandingCanceller
	Restarter  RuntimeRestarter
	Homes      BotHomeEraser
	Audit      audit.Recorder
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
		progress:   deps.Progress,
		stopper:    deps.Stopper,
		resetter:   deps.Resetter,
		canceller:  deps.Canceller,
		restarter:  deps.Restarter,
		homes:      deps.Homes,
		audit:      deps.Audit,
	}
}

func (a *Activations) lockBot(orgID string, workerID orgchart.NodeID) func() {
	v, _ := a.botLocks.LoadOrStore(orgID+"\x00"+string(workerID), &sync.Mutex{})
	mu := v.(*sync.Mutex)
	mu.Lock()
	return mu.Unlock
}

// ActivateResult is what an activation returns to the caller: the
// pre-allocated activation id plus the project / agent-app / session ids
// the UI needs to navigate to the desktop.
type ActivateResult struct {
	ActivationID activation.ID
	ProjectID    string
	AgentID      string
	SessionID    string
}

// Activate runs the "activate worker" command end-to-end:
//
//  1. Synchronously ensure the project + agent app (re-attaches the
//     helix-org MCP — the immediate user-visible fix the operator clicked
//     "Start Desktop" for).
//  2. Read the persisted session id (empty on first activation).
//  3. Return any in-flight activation instead of dispatching a duplicate.
//  4. Otherwise pre-allocate the audit row and enqueue it on the dispatcher's
//     per-Worker queue.
//
// The worker-id is validated up front (it propagates into the
// helix-specs git layout and topic ids — a defensive format check).
// Callers should still confirm the Worker exists (404) before calling —
// Activate's Ensure will also error on a missing Worker, but a pre-check
// gives the cleaner status.
func (a *Activations) Activate(ctx context.Context, orgID string, workerID orgchart.NodeID) (ActivateResult, error) {
	return a.activate(ctx, orgID, workerID, false)
}

// LatestError returns the newest completed activation failure. A newer pending
// or successful activation, or a successful turn begun after the failure,
// clears the operator error without changing the audit row.
func (a *Activations) LatestError(ctx context.Context, orgID string, workerID orgchart.NodeID) (string, error) {
	if a.repo == nil {
		return "", nil
	}
	rows, err := a.repo.ListForWorker(ctx, orgID, workerID, 1)
	if err != nil {
		return "", err
	}
	if len(rows) == 0 || !rows[0].IsCompleted() || rows[0].Outcome.Status != activation.StatusError {
		return "", nil
	}
	if a.progress != nil {
		startedAt, err := a.progress.LatestSuccessfulTurnStartedAt(ctx, orgID, workerID)
		if err != nil {
			return "", fmt.Errorf("latest successful turn: %w", err)
		}
		if startedAt.After(*rows[0].EndedAt) {
			return "", nil
		}
	}
	return rows[0].Outcome.Error, nil
}

func (a *Activations) activate(ctx context.Context, orgID string, workerID orgchart.NodeID, forceManual bool) (ActivateResult, error) {
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
	triggerKind := activation.TriggerManual
	if a.sessions != nil {
		var sessionErr error
		sessionID, sessionErr = a.sessions.SessionID(ctx, orgID, workerID)
		if !forceManual && a.repo != nil {
			rows, historyErr := a.repo.ListForWorker(ctx, orgID, workerID, 1)
			if historyErr == nil && len(rows) > 0 && !rows[0].IsCompleted() {
				return ActivateResult{
					ActivationID: rows[0].ID,
					ProjectID:    projectID,
					AgentID:      agentAppID,
					SessionID:    sessionID,
				}, nil
			}
			if workerID == seedprompts.ChiefOfStaffBotID && sessionErr == nil && sessionID == "" && historyErr == nil && len(rows) == 0 {
				triggerKind = activation.TriggerHire
			}
		}
	}
	activationID, err := a.prepare(ctx, orgID, workerID, triggerKind)
	if err != nil {
		return ActivateResult{}, err
	}
	if triggerKind == activation.TriggerHire {
		a.dispatcher.DispatchHire(ctx, orgID, workerID, activationID)
	} else {
		a.dispatcher.DispatchManual(ctx, orgID, workerID, activationID)
	}
	return ActivateResult{
		ActivationID: activationID,
		ProjectID:    projectID,
		AgentID:      agentAppID,
		SessionID:    sessionID,
	}, nil
}

// StopResult is what Stop returns so callers can report whether a
// desktop was actually stopped or there was nothing running.
type StopResult struct {
	// Stopped is true when a live session was found and StopDesktop ran.
	// False means no session (already stopped / never started) — a no-op.
	Stopped   bool   `json:"stopped"`
	SessionID string `json:"session_id,omitempty"`
}

// Stop stops the bot's desktop sandbox without deleting the session
// (transcript stays). No-op when there is no session.
func (a *Activations) Stop(ctx context.Context, orgID string, workerID orgchart.NodeID) (StopResult, error) {
	if a.stopper == nil {
		return StopResult{}, ErrStopUnavailable
	}
	if err := orgchart.ValidID(string(workerID)); err != nil {
		return StopResult{}, fmt.Errorf("worker id: %w", err)
	}
	defer a.lockBot(orgID, workerID)()
	if a.canceller != nil {
		if err := a.canceller.CancelOutstanding(ctx, orgID, workerID); err != nil {
			return StopResult{}, fmt.Errorf("cancel outstanding activations: %w", err)
		}
	}
	var sessionID string
	if a.sessions != nil {
		sessionID, _ = a.sessions.SessionID(ctx, orgID, workerID)
	}
	if sessionID == "" {
		return StopResult{Stopped: false}, nil
	}
	if err := a.stopper.StopDesktop(ctx, sessionID); err != nil {
		return StopResult{}, fmt.Errorf("stop desktop %s: %w", sessionID, err)
	}
	return StopResult{Stopped: true, SessionID: sessionID}, nil
}

// RestartMode says what Restart did.
type RestartMode string

const (
	// RestartRuntime recreated the container on the existing session.
	RestartRuntime RestartMode = "runtime"
	// RestartFirstStart activated a bot that had no session yet.
	RestartFirstStart RestartMode = "first_start"
)

// RestartPreserves lists what an ordinary restart keeps. It is part of the
// API response so callers never have to guess.
var RestartPreserves = []string{
	"session and conversation thread",
	"bot home (/home/retro/work, including Chrome profiles)",
	"queued activations",
	"in-flight activation (replayed under the same delivery id)",
}

// RestartResult reports an ordinary restart.
type RestartResult struct {
	Mode      RestartMode
	SessionID string
	Preserves []string
	// Activation is set for RestartFirstStart.
	Activation ActivateResult
}

// Restart is the ordinary, non-destructive restart: it recreates the bot's
// desktop container on its current session with its current tools and
// sandbox config. It never cancels queued work, deletes the session or
// touches the bot home. A bot that has never started is activated instead.
// The destructive operation is Reset.
func (a *Activations) Restart(ctx context.Context, orgID string, workerID orgchart.NodeID) (RestartResult, error) {
	if err := orgchart.ValidID(string(workerID)); err != nil {
		return RestartResult{}, fmt.Errorf("worker id: %w", err)
	}
	if a.restarter == nil {
		return RestartResult{}, ErrRestartUnavailable
	}
	unlock := a.lockBot(orgID, workerID)
	defer unlock()
	var sessionID string
	if a.sessions != nil {
		var err error
		if sessionID, err = a.sessions.SessionID(ctx, orgID, workerID); err != nil {
			return RestartResult{}, fmt.Errorf("resolve session: %w", err)
		}
	}
	if sessionID == "" {
		res, err := a.Activate(ctx, orgID, workerID)
		if err != nil {
			return RestartResult{}, err
		}
		return RestartResult{Mode: RestartFirstStart, SessionID: res.SessionID, Preserves: RestartPreserves, Activation: res}, nil
	}
	if err := a.restarter.RestartRuntime(ctx, orgID, workerID); err != nil {
		return RestartResult{}, fmt.Errorf("restart runtime of %s: %w", workerID, err)
	}
	return RestartResult{Mode: RestartRuntime, SessionID: sessionID, Preserves: RestartPreserves}, nil
}

// ResetScope names exactly what an explicit reset erases.
type ResetScope string

const (
	// ResetSession erases the bot's work queue and session; the bot home
	// is kept.
	ResetSession ResetScope = "session"
	// ResetSessionAndHome also retires the bot home.
	ResetSessionAndHome ResetScope = "session_and_home"
)

// ResetDeclaration is the fixed contract of a reset scope.
type ResetDeclaration struct {
	Erases []string `json:"erases"`
	Keeps  []string `json:"keeps"`
}

var resetSessionErases = []string{
	"queued and in-flight activations (purged, not replayed)",
	"current session, its conversation thread and desktop container",
	"session runtime data (inner Docker data, /data/sessions/<session>)",
}

var resetKeepsAlways = []string{
	"bot configuration, tools, secrets and triggers",
	"transcript events and activation audit history",
	"other bots and other organizations",
}

// DeclareReset returns what a scope erases and keeps.
func DeclareReset(scope ResetScope) (ResetDeclaration, error) {
	switch scope {
	case ResetSession:
		return ResetDeclaration{
			Erases: resetSessionErases,
			Keeps:  append([]string{"bot home (/home/retro/work, including Chrome profiles)"}, resetKeepsAlways...),
		}, nil
	case ResetSessionAndHome:
		return ResetDeclaration{
			Erases: append(append([]string{}, resetSessionErases...),
				"bot home (/home/retro/work, including Chrome profiles and logins), retired to rollback storage and purged after the retention period"),
			Keeps: resetKeepsAlways,
		}, nil
	}
	return ResetDeclaration{}, fmt.Errorf("%w: unknown scope %q (want %q or %q)", ErrInvalidReset, scope, ResetSession, ResetSessionAndHome)
}

// ResetRequest is an explicit, confirmed reset.
type ResetRequest struct {
	Scope ResetScope
	// Confirm must equal the bot id.
	Confirm string
	Actor   audit.ActorType
	ActorID string
}

// ResetResult reports a completed reset.
type ResetResult struct {
	Scope             ResetScope
	Declaration       ResetDeclaration
	PreviousSessionID string
	Home              *HomeErasure
	Activation        ActivateResult
}

// Reset is the explicit, destructive reset. It cancels queued and in-flight
// work, deletes the current session and, for ResetSessionAndHome, retires
// the bot home, then activates a fresh session. Every attempt is audited
// with its scope and outcome.
func (a *Activations) Reset(ctx context.Context, orgID string, workerID orgchart.NodeID, req ResetRequest) (res ResetResult, err error) {
	if err := orgchart.ValidID(string(workerID)); err != nil {
		return ResetResult{}, fmt.Errorf("worker id: %w", err)
	}
	decl, err := DeclareReset(req.Scope)
	if err != nil {
		return ResetResult{}, err
	}
	if req.Confirm != string(workerID) {
		return ResetResult{}, fmt.Errorf("%w: confirm must equal the bot id %q", ErrInvalidReset, workerID)
	}
	if a.resetter == nil || a.canceller == nil || a.sessions == nil || a.homes == nil || a.audit == nil {
		return ResetResult{}, ErrResetUnavailable
	}
	unlock := a.lockBot(orgID, workerID)
	defer unlock()

	res = ResetResult{Scope: req.Scope, Declaration: decl}
	if err := a.recordReset(ctx, orgID, workerID, req, audit.StatusAttempted, res, nil); err != nil {
		return ResetResult{}, fmt.Errorf("audit reset: %w", err)
	}
	defer func() {
		status := audit.StatusSucceeded
		if err != nil {
			status = audit.StatusFailed
		}
		if auditErr := a.recordReset(context.WithoutCancel(ctx), orgID, workerID, req, status, res, err); auditErr != nil && err == nil {
			err = fmt.Errorf("audit reset outcome: %w", auditErr)
		}
	}()

	// Checked before anything is touched, for every scope: deleting the
	// session first would destroy an unmigrated home, and session_and_home
	// would then report it as never existing instead of retiring it.
	reason, err := a.unmigratedHomeRefusal(ctx, orgID, workerID)
	if err != nil {
		return res, err
	}
	if reason != "" {
		return res, fmt.Errorf("%w: %s", ErrResetRefused, reason)
	}

	if err := a.canceller.CancelOutstanding(ctx, orgID, workerID); err != nil {
		return res, fmt.Errorf("cancel outstanding activations: %w", err)
	}
	if err := a.closePurgedActivations(ctx, orgID, workerID); err != nil {
		return res, err
	}
	sessionID, err := a.sessions.SessionID(ctx, orgID, workerID)
	if err != nil {
		return res, fmt.Errorf("resolve session: %w", err)
	}
	res.PreviousSessionID = sessionID
	if sessionID != "" {
		if err := a.resetter.ResetSession(ctx, orgID, workerID, sessionID); err != nil {
			return res, fmt.Errorf("reset session: %w", err)
		}
	}
	if req.Scope == ResetSessionAndHome {
		erased, err := a.homes.RetireHome(ctx, orgID, workerID)
		if err != nil {
			return res, fmt.Errorf("retire bot home: %w", err)
		}
		res.Home = &erased
	}
	activated, err := a.activate(ctx, orgID, workerID, true)
	if err != nil {
		return res, fmt.Errorf("activate after reset: %w", err)
	}
	res.Activation = activated
	return res, nil
}

// ScopePlan is one reset scope as it applies to a bot right now.
type ScopePlan struct {
	Scope             ResetScope
	Declaration       ResetDeclaration
	Available         bool
	UnavailableReason string
}

// ResetPlan describes every reset scope for the bot, including whether a
// session-only reset would be refused because the home still lives inside
// the session.
func (a *Activations) ResetPlan(ctx context.Context, orgID string, workerID orgchart.NodeID) ([]ScopePlan, error) {
	if err := orgchart.ValidID(string(workerID)); err != nil {
		return nil, fmt.Errorf("worker id: %w", err)
	}
	if a.homes == nil || a.sessions == nil {
		return nil, ErrResetUnavailable
	}
	reason, err := a.unmigratedHomeRefusal(ctx, orgID, workerID)
	if err != nil {
		return nil, err
	}
	var plans []ScopePlan
	for _, scope := range []ResetScope{ResetSession, ResetSessionAndHome} {
		decl, _ := DeclareReset(scope)
		p := ScopePlan{Scope: scope, Declaration: decl, Available: true}
		if reason != "" {
			p.Available = false
			p.UnavailableReason = reason
		}
		plans = append(plans, p)
	}
	return plans, nil
}

// unmigratedHomeRefusal explains why no reset can run yet, or returns ""
// when the home is safely outside the session.
func (a *Activations) unmigratedHomeRefusal(ctx context.Context, orgID string, workerID orgchart.NodeID) (string, error) {
	inSession, err := a.homes.HomeMayBeInSession(ctx, orgID, workerID)
	if err != nil {
		return "", fmt.Errorf("check bot home: %w", err)
	}
	if !inSession {
		return "", nil
	}
	sessionID, err := a.sessions.SessionID(ctx, orgID, workerID)
	if err != nil {
		return "", fmt.Errorf("resolve session: %w", err)
	}
	if sessionID == "" {
		return "", nil
	}
	return fmt.Sprintf("bot %s still keeps its home in session %s, so deleting the session would destroy it without the declared rollback copy; restart the bot once to move the home out of the session, then reset",
		workerID, sessionID), nil
}

// errPurgedByReset closes activation rows whose delivery Reset purged.
var errPurgedByReset = errors.New("activation purged by an explicit reset")

// closePurgedActivations closes audit rows left open for deliveries that
// CancelOutstanding just purged. A row stays open when the process running
// it died (for example an API restart); the queue would have redelivered
// it, but Reset purges the queue, so nothing else would ever close it and
// Activate would keep returning it as in flight.
func (a *Activations) closePurgedActivations(ctx context.Context, orgID string, workerID orgchart.NodeID) error {
	if a.repo == nil {
		return nil
	}
	rows, err := a.repo.ListForWorker(ctx, orgID, workerID, 50)
	if err != nil {
		return fmt.Errorf("list activations to close: %w", err)
	}
	for _, row := range rows {
		if row.IsCompleted() {
			continue
		}
		if err := a.repo.Complete(ctx, orgID, row.ID, activation.OutcomeFromError(errPurgedByReset), a.now()); err != nil {
			return fmt.Errorf("close purged activation %s: %w", row.ID, err)
		}
	}
	return nil
}

const resetAuditAction = "reset_bot"

func (a *Activations) recordReset(ctx context.Context, orgID string, workerID orgchart.NodeID, req ResetRequest, status audit.Status, res ResetResult, resetErr error) error {
	args, err := json.Marshal(struct {
		BotID             orgchart.NodeID  `json:"bot_id"`
		Scope             ResetScope       `json:"scope"`
		Declaration       ResetDeclaration `json:"declaration"`
		PreviousSessionID string           `json:"previous_session_id,omitempty"`
		Home              *HomeErasure     `json:"home,omitempty"`
		NewSessionID      string           `json:"new_session_id,omitempty"`
		ActivationID      activation.ID    `json:"activation_id,omitempty"`
	}{workerID, req.Scope, res.Declaration, res.PreviousSessionID, res.Home, res.Activation.SessionID, res.Activation.ActivationID})
	if err != nil {
		return err
	}
	entry := audit.Entry{
		OrganizationID: orgID,
		ActorID:        req.ActorID,
		ActorType:      req.Actor,
		AssetID:        string(workerID),
		EventType:      audit.EventBotLifecycle,
		Action:         resetAuditAction,
		Status:         status,
		Metadata:       audit.Metadata{Arguments: args},
	}
	if req.Actor == audit.ActorUser {
		entry.UserID = req.ActorID
	}
	if resetErr != nil {
		entry.Metadata.Error = resetErr.Error()
	}
	return a.audit.Record(ctx, entry)
}

// prepare pre-allocates an activation audit row for the Worker and
// returns its id. Returns an empty id (and nil error)
// when the repository or id-generator is unwired — Activate then treats
// that as "no pre-allocation; the Spawner mints its own", matching the
// previous inline behaviour.
func (a *Activations) prepare(ctx context.Context, orgID string, workerID orgchart.NodeID, triggerKind activation.TriggerKind) (activation.ID, error) {
	if a.repo == nil || a.newID == nil {
		return "", nil
	}
	id := activation.ID("a-" + a.newID())
	act, err := activation.New(
		id,
		workerID,
		[]activation.Trigger{{Kind: triggerKind}},
		a.now(),
		orgID,
	)
	if err != nil {
		return "", fmt.Errorf("build activation: %w", err)
	}
	if err := a.repo.Create(ctx, act); err != nil {
		return "", fmt.Errorf("persist activation: %w", err)
	}
	return id, nil
}
