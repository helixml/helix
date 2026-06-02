// Package memorystore is an in-memory implementation of every
// repository in api/pkg/org/store. Production code paths use the
// gorm-backed store; tests use this. The shape of every method
// matches the canonical interfaces so a Store assembled here is a
// drop-in for a Postgres-backed one.
//
// Concurrency: each repo holds its own sync.RWMutex. The store is
// safe for parallel use across goroutines.
//
// The data model mirrors the gorm rows — composite (id, org_id)
// PKs are enforced by keying every map on a (orgID, id) struct.
// Cross-tenant lookups return store.ErrNotFound; the bare id
// existing under another org is not visible.
package memorystore

import (
	"context"
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/helixml/helix/api/pkg/org/activation"
	"github.com/helixml/helix/api/pkg/org/domain"
	"github.com/helixml/helix/api/pkg/org/event"
	"github.com/helixml/helix/api/pkg/org/grant"
	"github.com/helixml/helix/api/pkg/org/position"
	"github.com/helixml/helix/api/pkg/org/role"
	"github.com/helixml/helix/api/pkg/org/store"
	"github.com/helixml/helix/api/pkg/org/stream"
	"github.com/helixml/helix/api/pkg/org/tool"
	"github.com/helixml/helix/api/pkg/org/worker"
)

// New returns a fresh *store.Store backed by in-memory repos. Use
// for tests and dev paths that don't need Postgres.
func New() *store.Store {
	subs := &subscriptionsRepo{rows: map[subKey]domain.Subscription{}}
	return &store.Store{
		Roles:              &rolesRepo{rows: map[orgKey]role.Role{}},
		Positions:          &positionsRepo{rows: map[orgKey]domain.Position{}},
		Workers:            &workersRepo{rows: map[orgKey]domain.Worker{}},
		WorkerRuntimeState: &runtimeStateRepo{rows: map[runtimeKey]string{}},
		Grants:             &grantsRepo{rows: map[orgKey]domain.ToolGrant{}},
		Streams:            &streamsRepo{rows: map[orgKey]domain.Stream{}},
		Subscriptions:      subs,
		Events:             &eventsRepo{rows: []domain.Event{}, subs: subs},
		Environments:       &environmentsRepo{rows: map[orgKey]domain.Environment{}},
		Configs:            &configsRepo{rows: map[orgKey]domain.Config{}},
		Activations:        &activationsRepo{rows: map[orgKey]*activation.Activation{}},
	}
}

// orgKey is the composite (orgID, id) the memory repos use as a
// map key. Tests assert that two different orgs with the same id
// are distinct rows, and the orgKey shape encodes that directly.
type orgKey struct {
	OrgID string
	ID    string
}

// ---- Roles --------------------------------------------------------------

type rolesRepo struct {
	mu   sync.RWMutex
	rows map[orgKey]role.Role
}

func (r *rolesRepo) Create(_ context.Context, rl role.Role) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	k := orgKey{OrgID: rl.OrganizationID, ID: string(rl.ID)}
	if _, ok := r.rows[k]; ok {
		return fmt.Errorf("role %q in org %q: already exists", rl.ID, rl.OrganizationID)
	}
	r.rows[k] = rl
	return nil
}

func (r *rolesRepo) Get(_ context.Context, orgID string, id role.ID) (role.Role, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if rl, ok := r.rows[orgKey{OrgID: orgID, ID: string(id)}]; ok {
		return rl, nil
	}
	return role.Role{}, fmt.Errorf("role %q in org %q: %w", id, orgID, store.ErrNotFound)
}

func (r *rolesRepo) List(_ context.Context, orgID string) ([]role.Role, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]role.Role, 0)
	for k, rl := range r.rows {
		if k.OrgID == orgID {
			out = append(out, rl)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, nil
}

func (r *rolesRepo) Update(_ context.Context, rl role.Role) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	k := orgKey{OrgID: rl.OrganizationID, ID: string(rl.ID)}
	if _, ok := r.rows[k]; !ok {
		return fmt.Errorf("role %q in org %q: %w", rl.ID, rl.OrganizationID, store.ErrNotFound)
	}
	r.rows[k] = rl
	return nil
}

// ---- Positions ----------------------------------------------------------

type positionsRepo struct {
	mu   sync.RWMutex
	rows map[orgKey]domain.Position
}

func (p *positionsRepo) Create(_ context.Context, pos domain.Position) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	k := orgKey{OrgID: pos.OrganizationID, ID: string(pos.ID)}
	if _, ok := p.rows[k]; ok {
		return fmt.Errorf("position %q in org %q: already exists", pos.ID, pos.OrganizationID)
	}
	p.rows[k] = pos
	return nil
}

func (p *positionsRepo) Get(_ context.Context, orgID string, id position.ID) (domain.Position, error) {
	p.mu.RLock()
	defer p.mu.RUnlock()
	if pos, ok := p.rows[orgKey{OrgID: orgID, ID: string(id)}]; ok {
		return pos, nil
	}
	return domain.Position{}, fmt.Errorf("position %q in org %q: %w", id, orgID, store.ErrNotFound)
}

func (p *positionsRepo) List(_ context.Context, orgID string) ([]domain.Position, error) {
	p.mu.RLock()
	defer p.mu.RUnlock()
	out := make([]domain.Position, 0)
	for k, pos := range p.rows {
		if k.OrgID == orgID {
			out = append(out, pos)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, nil
}

func (p *positionsRepo) ListChildren(_ context.Context, orgID string, parent position.ID) ([]domain.Position, error) {
	p.mu.RLock()
	defer p.mu.RUnlock()
	out := make([]domain.Position, 0)
	for k, pos := range p.rows {
		if k.OrgID != orgID || pos.ParentID == nil || *pos.ParentID != parent {
			continue
		}
		out = append(out, pos)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, nil
}

func (p *positionsRepo) Update(_ context.Context, pos domain.Position) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	k := orgKey{OrgID: pos.OrganizationID, ID: string(pos.ID)}
	if _, ok := p.rows[k]; !ok {
		return fmt.Errorf("position %q in org %q: %w", pos.ID, pos.OrganizationID, store.ErrNotFound)
	}
	p.rows[k] = pos
	return nil
}

// ---- Workers ------------------------------------------------------------

type workersRepo struct {
	mu   sync.RWMutex
	rows map[orgKey]domain.Worker
}

func (w *workersRepo) Create(_ context.Context, wk domain.Worker) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	k := orgKey{OrgID: wk.OrganizationID(), ID: string(wk.ID())}
	if _, ok := w.rows[k]; ok {
		return fmt.Errorf("worker %q in org %q: already exists", wk.ID(), wk.OrganizationID())
	}
	w.rows[k] = wk
	return nil
}

func (w *workersRepo) Get(_ context.Context, orgID string, id worker.ID) (domain.Worker, error) {
	w.mu.RLock()
	defer w.mu.RUnlock()
	if wk, ok := w.rows[orgKey{OrgID: orgID, ID: string(id)}]; ok {
		return wk, nil
	}
	return nil, fmt.Errorf("worker %q in org %q: %w", id, orgID, store.ErrNotFound)
}

func (w *workersRepo) List(_ context.Context, orgID string) ([]domain.Worker, error) {
	w.mu.RLock()
	defer w.mu.RUnlock()
	out := make([]domain.Worker, 0)
	for k, wk := range w.rows {
		if k.OrgID == orgID {
			out = append(out, wk)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID() < out[j].ID() })
	return out, nil
}

func (w *workersRepo) Update(_ context.Context, wk domain.Worker) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	k := orgKey{OrgID: wk.OrganizationID(), ID: string(wk.ID())}
	if _, ok := w.rows[k]; !ok {
		return fmt.Errorf("worker %q in org %q: %w", wk.ID(), wk.OrganizationID(), store.ErrNotFound)
	}
	w.rows[k] = wk
	return nil
}

func (w *workersRepo) Delete(_ context.Context, orgID string, id worker.ID) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	k := orgKey{OrgID: orgID, ID: string(id)}
	if _, ok := w.rows[k]; !ok {
		return fmt.Errorf("worker %q in org %q: %w", id, orgID, store.ErrNotFound)
	}
	delete(w.rows, k)
	return nil
}

// ---- WorkerRuntimeState ------------------------------------------------

type runtimeKey struct {
	OrgID    string
	WorkerID string
	Backend  string
	Key      string
}

type runtimeStateRepo struct {
	mu   sync.RWMutex
	rows map[runtimeKey]string
}

func (r *runtimeStateRepo) Get(_ context.Context, orgID string, workerID worker.ID, backend string) (map[string]string, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := map[string]string{}
	for k, v := range r.rows {
		if k.OrgID == orgID && k.WorkerID == string(workerID) && k.Backend == backend {
			out[k.Key] = v
		}
	}
	return out, nil
}

func (r *runtimeStateRepo) Set(_ context.Context, orgID string, workerID worker.ID, backend, key, value string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.rows[runtimeKey{OrgID: orgID, WorkerID: string(workerID), Backend: backend, Key: key}] = value
	return nil
}

func (r *runtimeStateRepo) SetMany(_ context.Context, orgID string, workerID worker.ID, backend string, kv map[string]string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	for key, value := range kv {
		r.rows[runtimeKey{OrgID: orgID, WorkerID: string(workerID), Backend: backend, Key: key}] = value
	}
	return nil
}

func (r *runtimeStateRepo) Clear(_ context.Context, orgID string, workerID worker.ID, backend string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	for k := range r.rows {
		if k.OrgID == orgID && k.WorkerID == string(workerID) && k.Backend == backend {
			delete(r.rows, k)
		}
	}
	return nil
}

// ---- Grants -------------------------------------------------------------

type grantsRepo struct {
	mu   sync.RWMutex
	rows map[orgKey]domain.ToolGrant
}

func (g *grantsRepo) Create(_ context.Context, gr domain.ToolGrant) error {
	g.mu.Lock()
	defer g.mu.Unlock()
	k := orgKey{OrgID: gr.OrganizationID, ID: string(gr.ID)}
	if _, ok := g.rows[k]; ok {
		return fmt.Errorf("grant %q in org %q: already exists", gr.ID, gr.OrganizationID)
	}
	g.rows[k] = gr
	return nil
}

func (g *grantsRepo) Get(_ context.Context, orgID string, id grant.ID) (domain.ToolGrant, error) {
	g.mu.RLock()
	defer g.mu.RUnlock()
	if gr, ok := g.rows[orgKey{OrgID: orgID, ID: string(id)}]; ok {
		return gr, nil
	}
	return domain.ToolGrant{}, fmt.Errorf("grant %q in org %q: %w", id, orgID, store.ErrNotFound)
}

func (g *grantsRepo) ListByWorker(_ context.Context, orgID string, workerID worker.ID) ([]domain.ToolGrant, error) {
	g.mu.RLock()
	defer g.mu.RUnlock()
	out := make([]domain.ToolGrant, 0)
	for k, gr := range g.rows {
		if k.OrgID == orgID && gr.WorkerID == workerID {
			out = append(out, gr)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, nil
}

func (g *grantsRepo) FindForWorkerAndTool(_ context.Context, orgID string, workerID worker.ID, toolName tool.Name) (domain.ToolGrant, error) {
	g.mu.RLock()
	defer g.mu.RUnlock()
	for k, gr := range g.rows {
		if k.OrgID == orgID && gr.WorkerID == workerID && gr.ToolName == toolName {
			return gr, nil
		}
	}
	return domain.ToolGrant{}, fmt.Errorf("grant for worker %q tool %q in org %q: %w", workerID, toolName, orgID, store.ErrNotFound)
}

func (g *grantsRepo) Delete(_ context.Context, orgID string, id grant.ID) error {
	g.mu.Lock()
	defer g.mu.Unlock()
	k := orgKey{OrgID: orgID, ID: string(id)}
	if _, ok := g.rows[k]; !ok {
		return fmt.Errorf("grant %q in org %q: %w", id, orgID, store.ErrNotFound)
	}
	delete(g.rows, k)
	return nil
}

// ---- Streams ------------------------------------------------------------

type streamsRepo struct {
	mu   sync.RWMutex
	rows map[orgKey]domain.Stream
}

func (s *streamsRepo) Create(_ context.Context, st domain.Stream) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	k := orgKey{OrgID: st.OrganizationID, ID: string(st.ID)}
	if _, ok := s.rows[k]; ok {
		return fmt.Errorf("stream %q in org %q: already exists", st.ID, st.OrganizationID)
	}
	// Enforce composite (org_id, name) uniqueness to mirror the gorm
	// idx_stream_org_name constraint.
	for k2, ex := range s.rows {
		if k2.OrgID == st.OrganizationID && ex.Name == st.Name {
			return fmt.Errorf("stream name %q already in use in org %q", st.Name, st.OrganizationID)
		}
	}
	s.rows[k] = st
	return nil
}

func (s *streamsRepo) Get(_ context.Context, orgID string, id stream.ID) (domain.Stream, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if st, ok := s.rows[orgKey{OrgID: orgID, ID: string(id)}]; ok {
		return st, nil
	}
	return domain.Stream{}, fmt.Errorf("stream %q in org %q: %w", id, orgID, store.ErrNotFound)
}

func (s *streamsRepo) List(_ context.Context, orgID string) ([]domain.Stream, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]domain.Stream, 0)
	for k, st := range s.rows {
		if k.OrgID == orgID {
			out = append(out, st)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, nil
}

// ---- Subscriptions ------------------------------------------------------

type subKey struct {
	OrgID    string
	WorkerID string
	StreamID string
}

type subscriptionsRepo struct {
	mu   sync.RWMutex
	rows map[subKey]domain.Subscription
}

func (s *subscriptionsRepo) Create(_ context.Context, sub domain.Subscription) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	k := subKey{OrgID: sub.OrganizationID, WorkerID: string(sub.WorkerID), StreamID: string(sub.StreamID)}
	if _, ok := s.rows[k]; ok {
		return fmt.Errorf("subscription %q→%q in org %q: already exists", sub.WorkerID, sub.StreamID, sub.OrganizationID)
	}
	s.rows[k] = sub
	return nil
}

func (s *subscriptionsRepo) Delete(_ context.Context, orgID string, workerID worker.ID, streamID stream.ID) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	k := subKey{OrgID: orgID, WorkerID: string(workerID), StreamID: string(streamID)}
	if _, ok := s.rows[k]; !ok {
		return fmt.Errorf("subscription %q→%q in org %q: %w", workerID, streamID, orgID, store.ErrNotFound)
	}
	delete(s.rows, k)
	return nil
}

func (s *subscriptionsRepo) Find(_ context.Context, orgID string, workerID worker.ID, streamID stream.ID) (domain.Subscription, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	k := subKey{OrgID: orgID, WorkerID: string(workerID), StreamID: string(streamID)}
	if sub, ok := s.rows[k]; ok {
		return sub, nil
	}
	return domain.Subscription{}, fmt.Errorf("subscription %q→%q in org %q: %w", workerID, streamID, orgID, store.ErrNotFound)
}

func (s *subscriptionsRepo) ListForWorker(_ context.Context, orgID string, workerID worker.ID) ([]domain.Subscription, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]domain.Subscription, 0)
	for k, sub := range s.rows {
		if k.OrgID == orgID && k.WorkerID == string(workerID) {
			out = append(out, sub)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].StreamID < out[j].StreamID })
	return out, nil
}

func (s *subscriptionsRepo) ListForStream(_ context.Context, orgID string, streamID stream.ID) ([]domain.Subscription, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]domain.Subscription, 0)
	for k, sub := range s.rows {
		if k.OrgID == orgID && k.StreamID == string(streamID) {
			out = append(out, sub)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].WorkerID < out[j].WorkerID })
	return out, nil
}

// ---- Events -------------------------------------------------------------

type eventsRepo struct {
	mu   sync.RWMutex
	rows []domain.Event // append-only, newest at end
	// subs is held by reference so ListForWorker can join with the
	// subscription set the same way the gorm impl does — events on
	// streams the worker is subscribed to are visible, not just
	// events sourced by the worker.
	subs *subscriptionsRepo
}

func (e *eventsRepo) Append(_ context.Context, ev domain.Event) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.rows = append(e.rows, ev)
	return nil
}

func (e *eventsRepo) ListForStream(_ context.Context, orgID string, streamID stream.ID, limit int) ([]domain.Event, error) {
	e.mu.RLock()
	defer e.mu.RUnlock()
	out := make([]domain.Event, 0)
	// Newest first.
	for i := len(e.rows) - 1; i >= 0; i-- {
		ev := e.rows[i]
		if ev.OrganizationID != orgID || ev.StreamID != streamID {
			continue
		}
		out = append(out, ev)
		if limit > 0 && len(out) >= limit {
			break
		}
	}
	return out, nil
}

func (e *eventsRepo) ListForWorker(ctx context.Context, orgID string, workerID worker.ID, limit int) ([]domain.Event, error) {
	// Match gorm's join semantics: events on streams the worker is
	// subscribed to. Sourced-by-worker events are intentionally NOT
	// included unless the worker is also subscribed to the stream
	// (read_events filters its own posts out via the trigger
	// pipeline, not at the storage layer).
	subs, err := e.subs.ListForWorker(ctx, orgID, workerID)
	if err != nil {
		return nil, err
	}
	subscribed := map[stream.ID]bool{}
	for _, sub := range subs {
		subscribed[sub.StreamID] = true
	}
	e.mu.RLock()
	defer e.mu.RUnlock()
	out := make([]domain.Event, 0)
	for i := len(e.rows) - 1; i >= 0; i-- {
		ev := e.rows[i]
		if ev.OrganizationID != orgID || !subscribed[ev.StreamID] {
			continue
		}
		out = append(out, ev)
		if limit > 0 && len(out) >= limit {
			break
		}
	}
	return out, nil
}

func (e *eventsRepo) ListSince(_ context.Context, orgID string, streamIDs []stream.ID, since event.ID, limit int) ([]domain.Event, error) {
	// Empty stream set returns nothing — the caller passed no streams
	// to listen on, so there's nothing to return. Matches gorm's
	// IN ()-on-empty behaviour.
	if len(streamIDs) == 0 {
		return []domain.Event{}, nil
	}
	e.mu.RLock()
	defer e.mu.RUnlock()
	wanted := map[stream.ID]bool{}
	for _, s := range streamIDs {
		wanted[s] = true
	}
	// Find the index of `since`; events strictly after it are
	// returned. since == "" or "not found" means "no lower bound" —
	// start from the beginning.
	startIdx := 0
	if since != "" {
		for i, ev := range e.rows {
			if ev.ID == since {
				startIdx = i + 1
				break
			}
		}
	}
	out := make([]domain.Event, 0)
	for i := startIdx; i < len(e.rows); i++ {
		ev := e.rows[i]
		if ev.OrganizationID != orgID {
			continue
		}
		if !wanted[ev.StreamID] {
			continue
		}
		out = append(out, ev)
		if limit > 0 && len(out) >= limit {
			break
		}
	}
	return out, nil
}

func (e *eventsRepo) ListAll(_ context.Context, orgID string, limit int) ([]domain.Event, error) {
	e.mu.RLock()
	defer e.mu.RUnlock()
	out := make([]domain.Event, 0)
	for i := len(e.rows) - 1; i >= 0; i-- {
		ev := e.rows[i]
		if ev.OrganizationID != orgID {
			continue
		}
		out = append(out, ev)
		if limit > 0 && len(out) >= limit {
			break
		}
	}
	return out, nil
}

// ---- Environments ------------------------------------------------------

type environmentsRepo struct {
	mu   sync.RWMutex
	rows map[orgKey]domain.Environment
}

func (e *environmentsRepo) Create(_ context.Context, env domain.Environment) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	k := orgKey{OrgID: env.OrganizationID, ID: string(env.WorkerID)}
	if _, ok := e.rows[k]; ok {
		return fmt.Errorf("environment for worker %q in org %q: already exists", env.WorkerID, env.OrganizationID)
	}
	e.rows[k] = env
	return nil
}

func (e *environmentsRepo) Get(_ context.Context, orgID string, workerID worker.ID) (domain.Environment, error) {
	e.mu.RLock()
	defer e.mu.RUnlock()
	if env, ok := e.rows[orgKey{OrgID: orgID, ID: string(workerID)}]; ok {
		return env, nil
	}
	return domain.Environment{}, fmt.Errorf("environment for worker %q in org %q: %w", workerID, orgID, store.ErrNotFound)
}

func (e *environmentsRepo) Delete(_ context.Context, orgID string, workerID worker.ID) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	k := orgKey{OrgID: orgID, ID: string(workerID)}
	if _, ok := e.rows[k]; !ok {
		return fmt.Errorf("environment for worker %q in org %q: %w", workerID, orgID, store.ErrNotFound)
	}
	delete(e.rows, k)
	return nil
}

// ---- Configs -----------------------------------------------------------

type configsRepo struct {
	mu   sync.RWMutex
	rows map[orgKey]domain.Config
}

func (c *configsRepo) Set(_ context.Context, cfg domain.Config) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	k := orgKey{OrgID: cfg.OrganizationID, ID: cfg.Key}
	c.rows[k] = cfg
	return nil
}

func (c *configsRepo) Get(_ context.Context, orgID, key string) (domain.Config, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	if cfg, ok := c.rows[orgKey{OrgID: orgID, ID: key}]; ok {
		return cfg, nil
	}
	return domain.Config{}, fmt.Errorf("config %q in org %q: %w", key, orgID, store.ErrNotFound)
}

func (c *configsRepo) List(_ context.Context, orgID, prefix string) ([]domain.Config, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	out := make([]domain.Config, 0)
	for k, cfg := range c.rows {
		if k.OrgID != orgID {
			continue
		}
		if prefix != "" && !startsWith(k.ID, prefix) {
			continue
		}
		out = append(out, cfg)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Key < out[j].Key })
	return out, nil
}

func (c *configsRepo) Delete(_ context.Context, orgID, key string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	k := orgKey{OrgID: orgID, ID: key}
	if _, ok := c.rows[k]; !ok {
		return fmt.Errorf("config %q in org %q: %w", key, orgID, store.ErrNotFound)
	}
	delete(c.rows, k)
	return nil
}

// ---- Activations -------------------------------------------------------

type activationsRepo struct {
	mu   sync.RWMutex
	rows map[orgKey]*activation.Activation
}

func (a *activationsRepo) Create(_ context.Context, act *activation.Activation) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	k := orgKey{OrgID: act.OrganizationID, ID: string(act.ID)}
	if _, ok := a.rows[k]; ok {
		return fmt.Errorf("activation %q in org %q: already exists", act.ID, act.OrganizationID)
	}
	a.rows[k] = act
	return nil
}

func (a *activationsRepo) Complete(_ context.Context, orgID string, id activation.ID, outcome activation.Outcome, endedAt time.Time) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	k := orgKey{OrgID: orgID, ID: string(id)}
	act, ok := a.rows[k]
	if !ok {
		return fmt.Errorf("activation %q in org %q: %w", id, orgID, store.ErrNotFound)
	}
	endedAtUTC := endedAt.UTC()
	act.EndedAt = &endedAtUTC
	act.Outcome = outcome
	return nil
}

func (a *activationsRepo) Get(_ context.Context, orgID string, id activation.ID) (*activation.Activation, error) {
	a.mu.RLock()
	defer a.mu.RUnlock()
	if act, ok := a.rows[orgKey{OrgID: orgID, ID: string(id)}]; ok {
		// return a defensive copy so external mutations don't poison
		// the store
		clone := *act
		return &clone, nil
	}
	return nil, fmt.Errorf("activation %q in org %q: %w", id, orgID, store.ErrNotFound)
}

func (a *activationsRepo) ListForWorker(_ context.Context, orgID string, workerID worker.ID, limit int) ([]*activation.Activation, error) {
	a.mu.RLock()
	defer a.mu.RUnlock()
	out := make([]*activation.Activation, 0)
	for k, act := range a.rows {
		if k.OrgID != orgID || act.WorkerID != workerID {
			continue
		}
		clone := *act
		out = append(out, &clone)
	}
	// Newest StartedAt first.
	sort.Slice(out, func(i, j int) bool { return out[i].StartedAt.After(out[j].StartedAt) })
	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

// ---- helpers ----------------------------------------------------------

func startsWith(s, prefix string) bool {
	if len(prefix) > len(s) {
		return false
	}
	return s[:len(prefix)] == prefix
}
