// Package memorystore provides an in-memory implementation of store.Store for testing.
//
// It implements the methods used by the WebSocket sync handlers
// (websocket_external_agent_sync.go). Unimplemented methods will panic via
// the embedded nil store.Store interface, providing a clear stack trace when
// an unexpected method is called.
package memorystore

import (
	"context"
	"sort"
	"sync"
	"time"

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
	mu           sync.RWMutex

	// OnInteractionUpdated is called after every UpdateInteraction call.
	// Test binaries use this to detect completion events.
	OnInteractionUpdated func(*types.Interaction)
}

// New creates a new in-memory store.
func New() *MemoryStore {
	return &MemoryStore{
		sessions:     make(map[string]*types.Session),
		interactions: make(map[string]*types.Interaction),
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
	cp := session
	m.sessions[session.ID] = &cp
	return &cp, nil
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

// --- Interaction methods ---

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
func (m *MemoryStore) MarkPromptAsFailed(_ context.Context, _ string) error  { return nil }

// SpecTask methods — always return "not found" (no spectasks in test)
func (m *MemoryStore) GetSpecTask(_ context.Context, _ string) (*types.SpecTask, error) {
	return nil, store.ErrNotFound
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

// App methods — always return "not found" (no apps in test)
func (m *MemoryStore) GetApp(_ context.Context, _ string) (*types.App, error) {
	return nil, store.ErrNotFound
}

// Zed settings override — always return "not found"
func (m *MemoryStore) GetZedSettingsOverride(_ context.Context, _ string) (*types.ZedSettingsOverride, error) {
	return nil, store.ErrNotFound
}
