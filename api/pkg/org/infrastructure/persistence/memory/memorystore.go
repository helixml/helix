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
package memory

import (
	"context"
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/helixml/helix/api/pkg/org/domain/activation"
	"github.com/helixml/helix/api/pkg/org/domain/config"
	"github.com/helixml/helix/api/pkg/org/domain/environment"
	"github.com/helixml/helix/api/pkg/org/domain/orgchart"
	"github.com/helixml/helix/api/pkg/org/domain/store"
	"github.com/helixml/helix/api/pkg/org/domain/streaming"
	"github.com/helixml/helix/api/pkg/org/domain/tool"
)

// New returns a fresh *store.Store backed by in-memory repos. Use
// for tests and dev paths that don't need Postgres.
func New() *store.Store {
	subs := &subscriptionsRepo{rows: map[subKey]streaming.Subscription{}}
	return &store.Store{
		Roles:              &rolesRepo{rows: map[orgKey]orgchart.Role{}},
		Positions:          &positionsRepo{rows: map[orgKey]orgchart.Position{}},
		Workers:            &workersRepo{rows: map[orgKey]orgchart.Worker{}},
		WorkerRuntimeState: &runtimeStateRepo{rows: map[runtimeKey]string{}},
		Grants:             &grantsRepo{rows: map[orgKey]orgchart.ToolGrant{}},
		Streams:            &streamsRepo{rows: map[orgKey]streaming.Stream{}},
		Subscriptions:      subs,
		Events:             &eventsRepo{rows: []streaming.Event{}, subs: subs},
		Environments:       &environmentsRepo{rows: map[orgKey]environment.Environment{}},
		Configs:            &configsRepo{rows: map[orgKey]config.Config{}},
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
	rows map[orgKey]orgchart.Role
}

func (r *rolesRepo) Create(_ context.Context, rl orgchart.Role) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	k := orgKey{OrgID: rl.OrganizationID, ID: string(rl.ID)}
	if _, ok := r.rows[k]; ok {
		return fmt.Errorf("role %q in org %q: already exists", rl.ID, rl.OrganizationID)
	}
	r.rows[k] = rl
	return nil
}

func (r *rolesRepo) Get(_ context.Context, orgID string, id orgchart.RoleID) (orgchart.Role, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if rl, ok := r.rows[orgKey{OrgID: orgID, ID: string(id)}]; ok {
		return rl, nil
	}
	return orgchart.Role{}, fmt.Errorf("role %q in org %q: %w", id, orgID, store.ErrNotFound)
}

func (r *rolesRepo) List(_ context.Context, orgID string) ([]orgchart.Role, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]orgchart.Role, 0)
	for k, rl := range r.rows {
		if k.OrgID == orgID {
			out = append(out, rl)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, nil
}

func (r *rolesRepo) Update(_ context.Context, rl orgchart.Role) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	k := orgKey{OrgID: rl.OrganizationID, ID: string(rl.ID)}
	if _, ok := r.rows[k]; !ok {
		return fmt.Errorf("role %q in org %q: %w", rl.ID, rl.OrganizationID, store.ErrNotFound)
	}
	r.rows[k] = rl
	return nil
}

func (r *rolesRepo) Delete(_ context.Context, orgID string, id orgchart.RoleID) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	k := orgKey{OrgID: orgID, ID: string(id)}
	if _, ok := r.rows[k]; !ok {
		return fmt.Errorf("role %q in org %q: %w", id, orgID, store.ErrNotFound)
	}
	delete(r.rows, k)
	return nil
}

// ---- Positions ----------------------------------------------------------

type positionsRepo struct {
	mu   sync.RWMutex
	rows map[orgKey]orgchart.Position
}

func (p *positionsRepo) Create(_ context.Context, pos orgchart.Position) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	k := orgKey{OrgID: pos.OrganizationID, ID: string(pos.ID)}
	if _, ok := p.rows[k]; ok {
		return fmt.Errorf("position %q in org %q: already exists", pos.ID, pos.OrganizationID)
	}
	p.rows[k] = pos
	return nil
}

func (p *positionsRepo) Get(_ context.Context, orgID string, id orgchart.PositionID) (orgchart.Position, error) {
	p.mu.RLock()
	defer p.mu.RUnlock()
	if pos, ok := p.rows[orgKey{OrgID: orgID, ID: string(id)}]; ok {
		return pos, nil
	}
	return orgchart.Position{}, fmt.Errorf("position %q in org %q: %w", id, orgID, store.ErrNotFound)
}

func (p *positionsRepo) List(_ context.Context, orgID string) ([]orgchart.Position, error) {
	p.mu.RLock()
	defer p.mu.RUnlock()
	out := make([]orgchart.Position, 0)
	for k, pos := range p.rows {
		if k.OrgID == orgID {
			out = append(out, pos)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, nil
}

func (p *positionsRepo) ListChildren(_ context.Context, orgID string, parent orgchart.PositionID) ([]orgchart.Position, error) {
	p.mu.RLock()
	defer p.mu.RUnlock()
	out := make([]orgchart.Position, 0)
	for k, pos := range p.rows {
		if k.OrgID != orgID || pos.ParentID == nil || *pos.ParentID != parent {
			continue
		}
		out = append(out, pos)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, nil
}

func (p *positionsRepo) Update(_ context.Context, pos orgchart.Position) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	k := orgKey{OrgID: pos.OrganizationID, ID: string(pos.ID)}
	if _, ok := p.rows[k]; !ok {
		return fmt.Errorf("position %q in org %q: %w", pos.ID, pos.OrganizationID, store.ErrNotFound)
	}
	p.rows[k] = pos
	return nil
}

func (p *positionsRepo) Delete(_ context.Context, orgID string, id orgchart.PositionID) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	k := orgKey{OrgID: orgID, ID: string(id)}
	if _, ok := p.rows[k]; !ok {
		return fmt.Errorf("position %q in org %q: %w", id, orgID, store.ErrNotFound)
	}
	delete(p.rows, k)
	return nil
}

// ---- Workers ------------------------------------------------------------

type workersRepo struct {
	mu   sync.RWMutex
	rows map[orgKey]orgchart.Worker
}

func (w *workersRepo) Create(_ context.Context, wk orgchart.Worker) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	k := orgKey{OrgID: wk.OrganizationID(), ID: string(wk.ID())}
	if _, ok := w.rows[k]; ok {
		return fmt.Errorf("worker %q in org %q: already exists", wk.ID(), wk.OrganizationID())
	}
	w.rows[k] = wk
	return nil
}

func (w *workersRepo) Get(_ context.Context, orgID string, id orgchart.WorkerID) (orgchart.Worker, error) {
	w.mu.RLock()
	defer w.mu.RUnlock()
	if wk, ok := w.rows[orgKey{OrgID: orgID, ID: string(id)}]; ok {
		return wk, nil
	}
	return nil, fmt.Errorf("worker %q in org %q: %w", id, orgID, store.ErrNotFound)
}

func (w *workersRepo) List(_ context.Context, orgID string) ([]orgchart.Worker, error) {
	w.mu.RLock()
	defer w.mu.RUnlock()
	out := make([]orgchart.Worker, 0)
	for k, wk := range w.rows {
		if k.OrgID == orgID {
			out = append(out, wk)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID() < out[j].ID() })
	return out, nil
}

func (w *workersRepo) Update(_ context.Context, wk orgchart.Worker) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	k := orgKey{OrgID: wk.OrganizationID(), ID: string(wk.ID())}
	if _, ok := w.rows[k]; !ok {
		return fmt.Errorf("worker %q in org %q: %w", wk.ID(), wk.OrganizationID(), store.ErrNotFound)
	}
	w.rows[k] = wk
	return nil
}

func (w *workersRepo) Delete(_ context.Context, orgID string, id orgchart.WorkerID) error {
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

func (r *runtimeStateRepo) Get(_ context.Context, orgID string, workerID orgchart.WorkerID, backend string) (map[string]string, error) {
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

func (r *runtimeStateRepo) Set(_ context.Context, orgID string, workerID orgchart.WorkerID, backend, key, value string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.rows[runtimeKey{OrgID: orgID, WorkerID: string(workerID), Backend: backend, Key: key}] = value
	return nil
}

func (r *runtimeStateRepo) SetMany(_ context.Context, orgID string, workerID orgchart.WorkerID, backend string, kv map[string]string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	for key, value := range kv {
		r.rows[runtimeKey{OrgID: orgID, WorkerID: string(workerID), Backend: backend, Key: key}] = value
	}
	return nil
}

func (r *runtimeStateRepo) Clear(_ context.Context, orgID string, workerID orgchart.WorkerID, backend string) error {
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
	rows map[orgKey]orgchart.ToolGrant
}

func (g *grantsRepo) Create(_ context.Context, gr orgchart.ToolGrant) error {
	g.mu.Lock()
	defer g.mu.Unlock()
	k := orgKey{OrgID: gr.OrganizationID, ID: string(gr.ID)}
	if _, ok := g.rows[k]; ok {
		return fmt.Errorf("grant %q in org %q: already exists", gr.ID, gr.OrganizationID)
	}
	g.rows[k] = gr
	return nil
}

func (g *grantsRepo) Get(_ context.Context, orgID string, id orgchart.GrantID) (orgchart.ToolGrant, error) {
	g.mu.RLock()
	defer g.mu.RUnlock()
	if gr, ok := g.rows[orgKey{OrgID: orgID, ID: string(id)}]; ok {
		return gr, nil
	}
	return orgchart.ToolGrant{}, fmt.Errorf("grant %q in org %q: %w", id, orgID, store.ErrNotFound)
}

func (g *grantsRepo) ListByWorker(_ context.Context, orgID string, workerID orgchart.WorkerID) ([]orgchart.ToolGrant, error) {
	g.mu.RLock()
	defer g.mu.RUnlock()
	out := make([]orgchart.ToolGrant, 0)
	for k, gr := range g.rows {
		if k.OrgID == orgID && gr.WorkerID == workerID {
			out = append(out, gr)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, nil
}

func (g *grantsRepo) FindForWorkerAndTool(_ context.Context, orgID string, workerID orgchart.WorkerID, toolName tool.Name) (orgchart.ToolGrant, error) {
	g.mu.RLock()
	defer g.mu.RUnlock()
	for k, gr := range g.rows {
		if k.OrgID == orgID && gr.WorkerID == workerID && gr.ToolName == toolName {
			return gr, nil
		}
	}
	return orgchart.ToolGrant{}, fmt.Errorf("grant for worker %q tool %q in org %q: %w", workerID, toolName, orgID, store.ErrNotFound)
}

func (g *grantsRepo) Delete(_ context.Context, orgID string, id orgchart.GrantID) error {
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
	rows map[orgKey]streaming.Stream
}

func (s *streamsRepo) Create(_ context.Context, st streaming.Stream) error {
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

func (s *streamsRepo) Get(_ context.Context, orgID string, id streaming.StreamID) (streaming.Stream, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if st, ok := s.rows[orgKey{OrgID: orgID, ID: string(id)}]; ok {
		return st, nil
	}
	return streaming.Stream{}, fmt.Errorf("stream %q in org %q: %w", id, orgID, store.ErrNotFound)
}

func (s *streamsRepo) List(_ context.Context, orgID string) ([]streaming.Stream, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]streaming.Stream, 0)
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
	rows map[subKey]streaming.Subscription
}

func (s *subscriptionsRepo) Create(_ context.Context, sub streaming.Subscription) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	k := subKey{OrgID: sub.OrganizationID, WorkerID: string(sub.WorkerID), StreamID: string(sub.StreamID)}
	if _, ok := s.rows[k]; ok {
		return fmt.Errorf("subscription %q→%q in org %q: already exists", sub.WorkerID, sub.StreamID, sub.OrganizationID)
	}
	s.rows[k] = sub
	return nil
}

func (s *subscriptionsRepo) Delete(_ context.Context, orgID string, workerID orgchart.WorkerID, streamID streaming.StreamID) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	k := subKey{OrgID: orgID, WorkerID: string(workerID), StreamID: string(streamID)}
	if _, ok := s.rows[k]; !ok {
		return fmt.Errorf("subscription %q→%q in org %q: %w", workerID, streamID, orgID, store.ErrNotFound)
	}
	delete(s.rows, k)
	return nil
}

func (s *subscriptionsRepo) Find(_ context.Context, orgID string, workerID orgchart.WorkerID, streamID streaming.StreamID) (streaming.Subscription, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	k := subKey{OrgID: orgID, WorkerID: string(workerID), StreamID: string(streamID)}
	if sub, ok := s.rows[k]; ok {
		return sub, nil
	}
	return streaming.Subscription{}, fmt.Errorf("subscription %q→%q in org %q: %w", workerID, streamID, orgID, store.ErrNotFound)
}

func (s *subscriptionsRepo) ListForWorker(_ context.Context, orgID string, workerID orgchart.WorkerID) ([]streaming.Subscription, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]streaming.Subscription, 0)
	for k, sub := range s.rows {
		if k.OrgID == orgID && k.WorkerID == string(workerID) {
			out = append(out, sub)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].StreamID < out[j].StreamID })
	return out, nil
}

func (s *subscriptionsRepo) ListForStream(_ context.Context, orgID string, streamID streaming.StreamID) ([]streaming.Subscription, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]streaming.Subscription, 0)
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
	rows []streaming.Event // append-only, newest at end
	// subs is held by reference so ListForWorker can join with the
	// subscription set the same way the gorm impl does — events on
	// streams the worker is subscribed to are visible, not just
	// events sourced by the worker.
	subs *subscriptionsRepo
}

func (e *eventsRepo) Append(_ context.Context, ev streaming.Event) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.rows = append(e.rows, ev)
	return nil
}

func (e *eventsRepo) ListForStream(_ context.Context, orgID string, streamID streaming.StreamID, limit int) ([]streaming.Event, error) {
	e.mu.RLock()
	defer e.mu.RUnlock()
	out := make([]streaming.Event, 0)
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

func (e *eventsRepo) ListForWorker(ctx context.Context, orgID string, workerID orgchart.WorkerID, limit int) ([]streaming.Event, error) {
	// Match gorm's join semantics: events on streams the worker is
	// subscribed to. Sourced-by-worker events are intentionally NOT
	// included unless the worker is also subscribed to the stream
	// (read_events filters its own posts out via the trigger
	// pipeline, not at the storage layer).
	subs, err := e.subs.ListForWorker(ctx, orgID, workerID)
	if err != nil {
		return nil, err
	}
	subscribed := map[streaming.StreamID]bool{}
	for _, sub := range subs {
		subscribed[sub.StreamID] = true
	}
	e.mu.RLock()
	defer e.mu.RUnlock()
	out := make([]streaming.Event, 0)
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

func (e *eventsRepo) ListSince(_ context.Context, orgID string, streamIDs []streaming.StreamID, since streaming.EventID, limit int) ([]streaming.Event, error) {
	// Empty stream set returns nothing — the caller passed no streams
	// to listen on, so there's nothing to return. Matches gorm's
	// IN ()-on-empty behaviour.
	if len(streamIDs) == 0 {
		return []streaming.Event{}, nil
	}
	e.mu.RLock()
	defer e.mu.RUnlock()
	wanted := map[streaming.StreamID]bool{}
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
	out := make([]streaming.Event, 0)
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

func (e *eventsRepo) ListAll(_ context.Context, orgID string, limit int) ([]streaming.Event, error) {
	e.mu.RLock()
	defer e.mu.RUnlock()
	out := make([]streaming.Event, 0)
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
	rows map[orgKey]environment.Environment
}

func (e *environmentsRepo) Create(_ context.Context, env environment.Environment) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	k := orgKey{OrgID: env.OrganizationID, ID: string(env.WorkerID)}
	if _, ok := e.rows[k]; ok {
		return fmt.Errorf("environment for worker %q in org %q: already exists", env.WorkerID, env.OrganizationID)
	}
	e.rows[k] = env
	return nil
}

func (e *environmentsRepo) Get(_ context.Context, orgID string, workerID orgchart.WorkerID) (environment.Environment, error) {
	e.mu.RLock()
	defer e.mu.RUnlock()
	if env, ok := e.rows[orgKey{OrgID: orgID, ID: string(workerID)}]; ok {
		return env, nil
	}
	return environment.Environment{}, fmt.Errorf("environment for worker %q in org %q: %w", workerID, orgID, store.ErrNotFound)
}

func (e *environmentsRepo) Delete(_ context.Context, orgID string, workerID orgchart.WorkerID) error {
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
	rows map[orgKey]config.Config
}

func (c *configsRepo) Set(_ context.Context, cfg config.Config) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	k := orgKey{OrgID: cfg.OrganizationID, ID: cfg.Key}
	c.rows[k] = cfg
	return nil
}

func (c *configsRepo) Get(_ context.Context, orgID, key string) (config.Config, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	if cfg, ok := c.rows[orgKey{OrgID: orgID, ID: key}]; ok {
		return cfg, nil
	}
	return config.Config{}, fmt.Errorf("config %q in org %q: %w", key, orgID, store.ErrNotFound)
}

func (c *configsRepo) List(_ context.Context, orgID, prefix string) ([]config.Config, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	out := make([]config.Config, 0)
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

func (a *activationsRepo) ListForWorker(_ context.Context, orgID string, workerID orgchart.WorkerID, limit int) ([]*activation.Activation, error) {
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
