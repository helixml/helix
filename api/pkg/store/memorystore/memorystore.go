// Package memorystore provides an in-memory implementation of store.Store for testing.
//
// It implements the methods used by the WebSocket sync handlers
// (websocket_external_agent_sync.go). Unimplemented methods will panic via
// the embedded nil store.Store interface, providing a clear stack trace when
// an unexpected method is called.
package memorystore

import (
	"context"
	"fmt"
	"sort"
	"sync"
	"time"

	"gorm.io/datatypes"

	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/system"
	"github.com/helixml/helix/api/pkg/types"
)

// MemoryStore implements store.Store with in-memory maps.
// Only methods used by the WebSocket sync handlers are implemented.
// All other methods panic via the embedded nil store.Store interface.
type MemoryStore struct {
	store.Store // embedded nil — panics on unimplemented methods (gives clear stack trace)

	sessions     map[string]*types.Session
	interactions map[string]*types.Interaction
	apps         map[string]*types.App
	projects     map[string]*types.Project
	specTasks    map[string]*types.SpecTask
	// planningSessionClaims tracks the atomic claim set by
	// SetPlanningSessionIDIfEmpty; keyed by taskID, value is the winning
	// sessionID. Allocated lazily.
	planningSessionClaims map[string]string
	mu                    sync.RWMutex

	// OnInteractionUpdated is called after every UpdateInteraction call.
	// Test binaries use this to detect completion events.
	OnInteractionUpdated func(*types.Interaction)
}

// New creates a new in-memory store.
func New() *MemoryStore {
	return &MemoryStore{
		sessions:     make(map[string]*types.Session),
		interactions: make(map[string]*types.Interaction),
		apps:         make(map[string]*types.App),
		projects:     make(map[string]*types.Project),
		specTasks:    make(map[string]*types.SpecTask),
	}
}

// --- Test validation helpers (not part of store.Store interface) ---

// GetAllSessions returns all sessions for test validation.
func (m *MemoryStore) GetAllSessions() []*types.Session {
	m.mu.RLock()
	defer m.mu.RUnlock()
	result := make([]*types.Session, 0, len(m.sessions))
	for _, s := range m.sessions {
		cp := *s
		result = append(result, &cp)
	}
	return result
}

// GetAllInteractions returns all interactions for test validation.
func (m *MemoryStore) GetAllInteractions() []*types.Interaction {
	m.mu.RLock()
	defer m.mu.RUnlock()
	result := make([]*types.Interaction, 0, len(m.interactions))
	for _, i := range m.interactions {
		cp := *i
		result = append(result, &cp)
	}
	return result
}

// --- Session methods ---

func (m *MemoryStore) GetSession(_ context.Context, id string) (*types.Session, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	s, ok := m.sessions[id]
	if !ok {
		return nil, store.ErrNotFound
	}
	cp := *s
	return &cp, nil
}

func (m *MemoryStore) GetSessionIncludingDeleted(ctx context.Context, id string) (*types.Session, error) {
	return m.GetSession(ctx, id)
}

func (m *MemoryStore) CreateSession(_ context.Context, session types.Session) (*types.Session, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if session.ID == "" {
		session.ID = system.GenerateSessionID()
	}
	if session.Created.IsZero() {
		session.Created = time.Now()
	}
	if session.Updated.IsZero() {
		session.Updated = time.Now()
	}
	cp := session
	m.sessions[session.ID] = &cp
	return &cp, nil
}

func (m *MemoryStore) UpdateSession(_ context.Context, session types.Session) (*types.Session, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	session.Updated = time.Now()
	cp := session
	m.sessions[session.ID] = &cp
	return &cp, nil
}

func (m *MemoryStore) TouchSession(_ context.Context, sessionID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if s, ok := m.sessions[sessionID]; ok {
		s.Updated = time.Now()
	}
	return nil
}

func (m *MemoryStore) ListSessions(_ context.Context, query store.ListSessionsQuery) ([]*types.Session, int64, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	result := make([]*types.Session, 0, len(m.sessions))
	for _, s := range m.sessions {
		cp := *s
		result = append(result, &cp)
	}
	// Sort by created time descending (most recent first), like production
	sort.Slice(result, func(a, b int) bool {
		return result[a].Created.After(result[b].Created)
	})
	// Apply PerPage limit
	if query.PerPage > 0 && len(result) > query.PerPage {
		result = result[:query.PerPage]
	}
	return result, int64(len(result)), nil
}

func (m *MemoryStore) ListSessionsByOwner(_ context.Context, ownerID string) ([]*types.Session, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	result := make([]*types.Session, 0)
	for _, s := range m.sessions {
		if s.Owner == ownerID {
			cp := *s
			result = append(result, &cp)
		}
	}
	return result, nil
}

// --- Interaction methods ---

func (m *MemoryStore) ListStuckWaitingInteractions(_ context.Context, _ time.Time, _ int) ([]*types.Interaction, error) {
	return nil, nil
}

func (m *MemoryStore) CountAutoWakeAttemptsSince(_ context.Context, _ string, _ time.Time) (int64, error) {
	return 0, nil
}

func (m *MemoryStore) IncrementInteractionAutoWakeCount(_ context.Context, id string) (int, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if i, ok := m.interactions[id]; ok {
		i.AutoWakeCount++
		return i.AutoWakeCount, nil
	}
	return 0, store.ErrNotFound
}

func (m *MemoryStore) GetInteraction(_ context.Context, id string) (*types.Interaction, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	i, ok := m.interactions[id]
	if !ok {
		return nil, store.ErrNotFound
	}
	cp := *i
	return &cp, nil
}

func (m *MemoryStore) GetInteractionsSummary(_ context.Context, sessionID string, generationID int) (int64, time.Time, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	var count int64
	var maxUpdated time.Time
	for _, i := range m.interactions {
		if i.SessionID != sessionID {
			continue
		}
		if generationID > 0 && i.GenerationID != generationID {
			continue
		}
		count++
		if i.Updated.After(maxUpdated) {
			maxUpdated = i.Updated
		}
	}
	return count, maxUpdated, nil
}

func (m *MemoryStore) CreateInteraction(_ context.Context, interaction *types.Interaction) (*types.Interaction, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if interaction.ID == "" {
		interaction.ID = system.GenerateInteractionID()
	}
	if interaction.Created.IsZero() {
		interaction.Created = time.Now()
	}
	if interaction.Updated.IsZero() {
		interaction.Updated = time.Now()
	}
	cp := *interaction
	m.interactions[interaction.ID] = &cp
	return &cp, nil
}

func (m *MemoryStore) UpdateInteraction(_ context.Context, interaction *types.Interaction) (*types.Interaction, error) {
	m.mu.Lock()
	cp := *interaction
	m.interactions[interaction.ID] = &cp
	cb := m.OnInteractionUpdated
	m.mu.Unlock()

	if cb != nil {
		cb(&cp)
	}
	return &cp, nil
}

// UpdateInteractionStreamingFields mirrors the Postgres column-scoped write
// from the streaming flush path. It only touches response content / Zed
// offset+id, so a concurrent state transition (cancel / complete) is never
// clobbered. Matches the lost-update fix in websocket_external_agent_sync.go.
func (m *MemoryStore) UpdateInteractionStreamingFields(_ context.Context, interactionID string, generationID int, responseMessage string, responseEntries datatypes.JSON, lastZedMessageOffset int, lastZedMessageID string) error {
	m.mu.Lock()
	existing, ok := m.interactions[interactionID]
	if !ok || existing.GenerationID != generationID {
		m.mu.Unlock()
		return nil
	}
	existing.ResponseMessage = responseMessage
	existing.ResponseEntries = responseEntries
	existing.LastZedMessageOffset = lastZedMessageOffset
	existing.LastZedMessageID = lastZedMessageID
	existing.Updated = time.Now()
	cp := *existing
	cb := m.OnInteractionUpdated
	m.mu.Unlock()

	if cb != nil {
		cb(&cp)
	}
	return nil
}

// MarkInteractionCompleteIfWaiting transitions Waiting → Complete atomically.
// Returns true only if the row was actually transitioned, so a streaming flush
// cannot resurrect a cancelled or errored turn as "complete".
func (m *MemoryStore) MarkInteractionCompleteIfWaiting(_ context.Context, interactionID string, generationID int) (bool, error) {
	m.mu.Lock()
	existing, ok := m.interactions[interactionID]
	if !ok || existing.GenerationID != generationID {
		m.mu.Unlock()
		return false, nil
	}
	if existing.State != types.InteractionStateWaiting {
		m.mu.Unlock()
		return false, nil
	}
	existing.State = types.InteractionStateComplete
	existing.Completed = time.Now()
	existing.Updated = time.Now()
	cp := *existing
	cb := m.OnInteractionUpdated
	m.mu.Unlock()

	if cb != nil {
		cb(&cp)
	}
	return true, nil
}

func (m *MemoryStore) ListInteractions(_ context.Context, query *types.ListInteractionsQuery) ([]*types.Interaction, int64, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	var result []*types.Interaction
	for _, i := range m.interactions {
		if query.SessionID != "" && i.SessionID != query.SessionID {
			continue
		}
		// GenerationID 0 is the default; -1 means "all generations"
		if query.GenerationID >= 0 && i.GenerationID != query.GenerationID {
			continue
		}
		cp := *i
		result = append(result, &cp)
	}
	// Sort by ID ascending (creation order)
	sort.Slice(result, func(a, b int) bool { return result[a].ID < result[b].ID })
	// Apply PerPage limit
	if query.PerPage > 0 && len(result) > query.PerPage {
		result = result[:query.PerPage]
	}
	return result, int64(len(result)), nil
}

// ClearSessionInteractions deletes every interaction for a session,
// mirroring the gorm store's atomic delete-by-session_id. Idempotent: a
// session with no interactions is a no-op. Backs the clear-session
// coordinator (and the org spawner's pre-reactivation clear).
func (m *MemoryStore) ClearSessionInteractions(_ context.Context, sessionID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for id, i := range m.interactions {
		if i.SessionID == sessionID {
			delete(m.interactions, id)
		}
	}
	return nil
}

// --- Stubs for methods called in goroutines (prevent panics) ---

// Design review comments — always return "not found" (no comments in test)
func (m *MemoryStore) GetCommentByInteractionID(_ context.Context, _ string) (*types.SpecTaskDesignReviewComment, error) {
	return nil, store.ErrNotFound
}

func (m *MemoryStore) GetCommentByRequestID(_ context.Context, _ string) (*types.SpecTaskDesignReviewComment, error) {
	return nil, store.ErrNotFound
}

func (m *MemoryStore) UpdateSpecTaskDesignReviewComment(_ context.Context, _ *types.SpecTaskDesignReviewComment) error {
	return nil
}

// Pending comments — always return nil (no comments queued)
func (m *MemoryStore) GetPendingCommentByPlanningSessionID(_ context.Context, _ string) (*types.SpecTaskDesignReviewComment, error) {
	return nil, nil
}

func (m *MemoryStore) IsCommentBeingProcessedForSession(_ context.Context, _ string) (bool, error) {
	return false, nil
}

func (m *MemoryStore) GetNextQueuedCommentForSession(_ context.Context, _ string) (*types.SpecTaskDesignReviewComment, error) {
	return nil, nil
}

func (m *MemoryStore) GetSessionsWithPendingComments(_ context.Context) ([]string, error) {
	return nil, nil
}

func (m *MemoryStore) ResetStuckComments(_ context.Context) (int64, error) {
	return 0, nil
}

// Prompt queue — always return nil (no prompts queued)
func (m *MemoryStore) GetNextPendingPrompt(_ context.Context, _ string) (*types.PromptHistoryEntry, error) {
	return nil, nil
}

func (m *MemoryStore) GetAnyPendingPrompt(_ context.Context, _ string) (*types.PromptHistoryEntry, error) {
	return nil, nil
}

func (m *MemoryStore) GetNextInterruptPrompt(_ context.Context, _ string) (*types.PromptHistoryEntry, error) {
	return nil, nil
}

func (m *MemoryStore) MarkPromptAsPending(_ context.Context, _ string) error { return nil }
func (m *MemoryStore) MarkPromptAsSent(_ context.Context, _ string) error    { return nil }
func (m *MemoryStore) MarkPromptAsFailed(_ context.Context, _ string, _ string) error {
	return nil
}
func (m *MemoryStore) MarkPromptAsCrashed(_ context.Context, _ string, _ string) error {
	return nil
}
func (m *MemoryStore) ResetCrashedPromptsForSession(_ context.Context, _ string) (int, error) {
	return 0, nil
}
func (m *MemoryStore) ReconcileStuckSendingPrompts(_ context.Context) (int, error) {
	return 0, nil
}
func (m *MemoryStore) RequeueBouncedPrompt(_ context.Context, _ string) error     { return nil }
func (m *MemoryStore) DeletePromptHistoryEntry(_ context.Context, _ string) error { return nil }
func (m *MemoryStore) ClaimPromptForSending(_ context.Context, _ string) (bool, error) {
	return true, nil // In-memory: always succeed (no concurrency in tests)
}

// SpecTask methods — minimal in-memory implementation. Earlier versions
// returned ErrNotFound for everything; the fork-and-pause path needs
// real CRUD because it re-points a SpecTask's PlanningSessionID at the
// child after a fork (see HelixAPIServer.repointSpecTasksToChild).
func (m *MemoryStore) GetSpecTask(_ context.Context, id string) (*types.SpecTask, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	t, ok := m.specTasks[id]
	if !ok {
		return nil, store.ErrNotFound
	}
	cp := *t
	return &cp, nil
}

// ListSpecTasks supports the filter shapes the fork path uses today —
// notably PlanningSessionID for the reverse lookup from session → task.
// Other filters are wired in as needed; everything not handled here is
// effectively "no filter".
func (m *MemoryStore) ListSpecTasks(_ context.Context, filters *types.SpecTaskFilters) ([]*types.SpecTask, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make([]*types.SpecTask, 0, len(m.specTasks))
	for _, t := range m.specTasks {
		if filters != nil && filters.PlanningSessionID != "" && t.PlanningSessionID != filters.PlanningSessionID {
			continue
		}
		cp := *t
		out = append(out, &cp)
	}
	return out, nil
}

func (m *MemoryStore) UpdateSpecTask(_ context.Context, task *types.SpecTask) error {
	if task == nil || task.ID == "" {
		return fmt.Errorf("task ID is required")
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.specTasks[task.ID]; !ok {
		return store.ErrNotFound
	}
	cp := *task
	m.specTasks[task.ID] = &cp
	return nil
}

// SeedSpecTask is a test helper: install a SpecTask into the store
// without going through validation. Mirrors SeedApp / SeedSession.
func (m *MemoryStore) SeedSpecTask(task *types.SpecTask) {
	m.mu.Lock()
	defer m.mu.Unlock()
	cp := *task
	m.specTasks[task.ID] = &cp
}

// ListGitRepositories returns an empty list — the fork tests never seed
// repos and the pre-fork commit+push helper relies on this to no-op
// safely when there's no project (or no repos). If a future test needs
// real repos here, add a SeedGitRepository helper.
func (m *MemoryStore) ListGitRepositories(_ context.Context, _ *types.ListGitRepositoriesRequest) ([]*types.GitRepository, error) {
	return nil, nil
}

func (m *MemoryStore) TransitionSpecTaskStatus(_ context.Context, _ string, _ []types.SpecTaskStatus, _ types.SpecTaskStatus, _ map[string]any) (bool, error) {
	return false, nil
}

// SetPlanningSessionIDIfEmpty mirrors the postgres CAS — first caller wins, rest
// lose. No persistence; just a per-task atomic gate keyed by taskID so tests can
// exercise the race without a real DB.
func (m *MemoryStore) SetPlanningSessionIDIfEmpty(_ context.Context, taskID string, sessionID string) (bool, error) {
	if taskID == "" || sessionID == "" {
		return false, nil
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.planningSessionClaims == nil {
		m.planningSessionClaims = make(map[string]string)
	}
	if existing := m.planningSessionClaims[taskID]; existing != "" {
		return false, nil
	}
	m.planningSessionClaims[taskID] = sessionID
	return true, nil
}

func (m *MemoryStore) GetSpecTaskZedThreadByZedThreadID(_ context.Context, _ string) (*types.SpecTaskZedThread, error) {
	return nil, store.ErrNotFound
}

func (m *MemoryStore) GetSpecTaskExternalAgent(_ context.Context, _ string) (*types.SpecTaskExternalAgent, error) {
	return nil, store.ErrNotFound
}

func (m *MemoryStore) GetSpecTaskExternalAgentByID(_ context.Context, _ string) (*types.SpecTaskExternalAgent, error) {
	return nil, store.ErrNotFound
}

// GetUser: handlers call this to hydrate a project's owner; returning
// ErrNotFound lets them log a warn and continue rather than panic on
// the embedded nil store.Store.
func (m *MemoryStore) GetUser(_ context.Context, _ *store.GetUserQuery) (*types.User, error) {
	return nil, store.ErrNotFound
}

func (m *MemoryStore) GetApp(_ context.Context, id string) (*types.App, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	a, ok := m.apps[id]
	if !ok {
		return nil, store.ErrNotFound
	}
	cp := *a
	return &cp, nil
}

// SeedApp inserts an app for tests (not part of store.Store).
func (m *MemoryStore) SeedApp(app *types.App) {
	m.mu.Lock()
	defer m.mu.Unlock()
	cp := *app
	m.apps[app.ID] = &cp
}

func (m *MemoryStore) UpdateApp(_ context.Context, app *types.App) (*types.App, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.apps[app.ID]; !ok {
		return nil, store.ErrNotFound
	}
	cp := *app
	cp.Updated = time.Now()
	m.apps[app.ID] = &cp
	return &cp, nil
}

func (m *MemoryStore) GetProject(_ context.Context, id string) (*types.Project, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	p, ok := m.projects[id]
	if !ok {
		return nil, store.ErrNotFound
	}
	cp := *p
	return &cp, nil
}

// SeedProject inserts a project for tests (not part of store.Store).
func (m *MemoryStore) SeedProject(p *types.Project) {
	m.mu.Lock()
	defer m.mu.Unlock()
	cp := *p
	m.projects[p.ID] = &cp
}

// Zed settings override — always return "not found"
func (m *MemoryStore) GetZedSettingsOverride(_ context.Context, _ string) (*types.ZedSettingsOverride, error) {
	return nil, store.ErrNotFound
}
