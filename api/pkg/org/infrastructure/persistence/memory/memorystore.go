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
	"github.com/helixml/helix/api/pkg/org/domain/asset"
	"github.com/helixml/helix/api/pkg/org/domain/attachment"
	"github.com/helixml/helix/api/pkg/org/domain/config"
	"github.com/helixml/helix/api/pkg/org/domain/domainevent"
	"github.com/helixml/helix/api/pkg/org/domain/orgchart"
	"github.com/helixml/helix/api/pkg/org/domain/processor"
	"github.com/helixml/helix/api/pkg/org/domain/store"
	"github.com/helixml/helix/api/pkg/org/domain/streaming"
	"github.com/helixml/helix/api/pkg/org/domain/trigger"
	"github.com/helixml/helix/api/pkg/org/domain/workersecret"
)

// New returns a fresh *store.Store backed by in-memory repos. Use
// for tests and dev paths that don't need Postgres.
func New() *store.Store {
	lines := &reportingLinesRepo{rows: map[lineKey]struct{}{}}
	assetLinks := &assetLinksRepo{rows: map[assetLinkKey]asset.Link{}}
	bots := &nodesRepo{rows: map[orgKey]orgchart.Node{}, lines: lines}
	attachments := &attachmentsRepo{rows: map[orgKey]attachment.Attachment{}}
	bots.attachments = attachments
	processors := &processorsRepo{rows: map[orgKey]processor.Processor{}, attachments: attachments}
	triggers := &triggersRepo{rows: map[orgKey]trigger.Trigger{}, attachments: attachments}
	retired := &retiredRepo{processorInput: map[store.OrgScopedID]streaming.StreamID{}}
	return &store.Store{
		Nodes:                bots,
		ReportingLines:       lines,
		NodeRuntimeState:     &runtimeStateRepo{rows: map[runtimeKey]string{}},
		Events:               &eventsRepo{rows: []streaming.Event{}},
		Configs:              &configsRepo{rows: map[orgKey]config.Config{}},
		Activations:          &activationsRepo{rows: map[orgKey]*activation.Activation{}},
		Processors:           processors,
		Triggers:             triggers,
		WorkerAttachments:    attachments,
		WorkerSecretBindings: &workerSecretBindingsRepo{rows: map[workerSecretKey]workersecret.Binding{}},
		Assets:               &assetsRepo{rows: map[orgKey]asset.Asset{}, links: assetLinks},
		AssetLinks:           assetLinks,
		ChartPositions:       &chartPositionsRepo{rows: map[chartPosKey]orgchart.ChartPosition{}},
		DomainEvents:         &domainEventsRepo{},

		RetiredTopics:          retired,
		RetiredSubscriptions:   &retiredSubsRepo{inner: retired},
		RetiredProcessorInputs: &retiredInputsRepo{inner: retired},
	}
}

type workerSecretKey struct {
	orgID    string
	workerID orgchart.NodeID
	name     string
}
type workerSecretBindingsRepo struct {
	mu   sync.RWMutex
	rows map[workerSecretKey]workersecret.Binding
}

func (r *workerSecretBindingsRepo) Create(_ context.Context, b workersecret.Binding) error {
	if err := b.Validate(); err != nil {
		return err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	k := workerSecretKey{b.OrganizationID, b.WorkerID, b.Name}
	if _, ok := r.rows[k]; ok {
		return fmt.Errorf("worker secret binding: %w", store.ErrConflict)
	}
	r.rows[k] = b
	return nil
}
func (r *workerSecretBindingsRepo) Update(_ context.Context, b workersecret.Binding) error {
	if err := b.Validate(); err != nil {
		return err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	k := workerSecretKey{b.OrganizationID, b.WorkerID, b.Name}
	if _, ok := r.rows[k]; !ok {
		return fmt.Errorf("worker secret binding: %w", store.ErrNotFound)
	}
	r.rows[k] = b
	return nil
}
func (r *workerSecretBindingsRepo) Get(_ context.Context, orgID string, workerID orgchart.NodeID, name string) (workersecret.Binding, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	b, ok := r.rows[workerSecretKey{orgID, workerID, name}]
	if !ok {
		return workersecret.Binding{}, fmt.Errorf("worker secret binding: %w", store.ErrNotFound)
	}
	return b, nil
}
func (r *workerSecretBindingsRepo) List(_ context.Context, orgID string, workerID orgchart.NodeID) ([]workersecret.Binding, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]workersecret.Binding, 0)
	for k, b := range r.rows {
		if k.orgID == orgID && k.workerID == workerID {
			out = append(out, b)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}
func (r *workerSecretBindingsRepo) ListBySecretID(_ context.Context, secretID string) ([]workersecret.Binding, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]workersecret.Binding, 0)
	for _, b := range r.rows {
		if b.SourceKind == workersecret.SourceHelixSecret && b.SecretID == secretID {
			out = append(out, b)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}
func (r *workerSecretBindingsRepo) Delete(_ context.Context, orgID string, workerID orgchart.NodeID, name string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	k := workerSecretKey{orgID, workerID, name}
	if _, ok := r.rows[k]; !ok {
		return fmt.Errorf("worker secret binding: %w", store.ErrNotFound)
	}
	delete(r.rows, k)
	return nil
}

// ---- ChartPositions -----------------------------------------------------

type chartPosKey struct {
	orgID string
	kind  string
	id    string
}

type chartPositionsRepo struct {
	mu   sync.RWMutex
	rows map[chartPosKey]orgchart.ChartPosition
}

func (r *chartPositionsRepo) List(_ context.Context, orgID string) ([]orgchart.ChartPosition, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]orgchart.ChartPosition, 0)
	for k, p := range r.rows {
		if k.orgID == orgID {
			out = append(out, p)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Kind != out[j].Kind {
			return out[i].Kind < out[j].Kind
		}
		return out[i].ID < out[j].ID
	})
	return out, nil
}

func (r *chartPositionsRepo) Upsert(_ context.Context, pos orgchart.ChartPosition) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.rows[chartPosKey{orgID: pos.OrganizationID, kind: pos.Kind, id: pos.ID}] = pos
	return nil
}

func (r *chartPositionsRepo) UpsertMany(ctx context.Context, positions []orgchart.ChartPosition) error {
	for _, p := range positions {
		if err := r.Upsert(ctx, p); err != nil {
			return err
		}
	}
	return nil
}

func (r *chartPositionsRepo) Delete(_ context.Context, orgID, kind, id string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	k := chartPosKey{orgID: orgID, kind: kind, id: id}
	if _, ok := r.rows[k]; !ok {
		return fmt.Errorf("chart_position: %w", store.ErrNotFound)
	}
	delete(r.rows, k)
	return nil
}

func (r *chartPositionsRepo) Clear(_ context.Context, orgID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	for k := range r.rows {
		if k.orgID == orgID {
			delete(r.rows, k)
		}
	}
	return nil
}

// ---- DomainEvents -------------------------------------------------------

// domainEventsRepo is the in-memory append-only log. A flat slice is fine:
// the log is small and only ever appended to and range-scanned.
type domainEventsRepo struct {
	mu   sync.RWMutex
	rows []domainevent.DomainEvent
}

func (r *domainEventsRepo) Append(_ context.Context, e domainevent.DomainEvent) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.rows = append(r.rows, e)
	return nil
}

func (r *domainEventsRepo) ListBySubject(_ context.Context, orgID string, typ domainevent.Type, subject string, since time.Time) ([]domainevent.DomainEvent, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]domainevent.DomainEvent, 0)
	for _, e := range r.rows {
		if e.OrganizationID != orgID || e.Type != typ || e.Subject != subject {
			continue
		}
		if !since.IsZero() && e.CreatedAt.Before(since) {
			continue
		}
		out = append(out, e)
	}
	// Newest first.
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.After(out[j].CreatedAt) })
	return out, nil
}

// orgKey is the composite (orgID, id) the memory repos use as a
// map key. Tests assert that two different orgs with the same id
// are distinct rows, and the orgKey shape encodes that directly.
type orgKey struct {
	OrgID string
	ID    string
}

// ---- Nodes ---------------------------------------------------------------

type nodesRepo struct {
	mu   sync.RWMutex
	rows map[orgKey]orgchart.Node
	// lines and attachments are held by reference so Delete can cascade:
	// a deleted bot's own attachments and every reporting line that
	// references it (as manager or report) are dropped, mirroring the
	// gorm store's ON DELETE CASCADE foreign keys.
	lines       *reportingLinesRepo
	attachments *attachmentsRepo
}

func (r *nodesRepo) Create(_ context.Context, b orgchart.Node) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	k := orgKey{OrgID: b.OrganizationID, ID: string(b.ID)}
	if _, ok := r.rows[k]; ok {
		return fmt.Errorf("bot %q in org %q: already exists", b.ID, b.OrganizationID)
	}
	r.rows[k] = b
	return nil
}

func (r *nodesRepo) Get(_ context.Context, orgID string, id orgchart.NodeID) (orgchart.Node, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if b, ok := r.rows[orgKey{OrgID: orgID, ID: string(id)}]; ok {
		return b, nil
	}
	return orgchart.Node{}, fmt.Errorf("bot %q in org %q: %w", id, orgID, store.ErrNotFound)
}

func (r *nodesRepo) List(_ context.Context, orgID string) ([]orgchart.Node, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]orgchart.Node, 0)
	for k, b := range r.rows {
		if k.OrgID == orgID {
			out = append(out, b)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, nil
}

func (r *nodesRepo) Update(_ context.Context, b orgchart.Node) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	k := orgKey{OrgID: b.OrganizationID, ID: string(b.ID)}
	if _, ok := r.rows[k]; !ok {
		return fmt.Errorf("bot %q in org %q: %w", b.ID, b.OrganizationID, store.ErrNotFound)
	}
	r.rows[k] = b
	return nil
}

func (r *nodesRepo) ClaimAgentApp(_ context.Context, orgID string, id orgchart.NodeID, appID string) (bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	k := orgKey{OrgID: orgID, ID: string(id)}
	b, ok := r.rows[k]
	if !ok {
		return false, fmt.Errorf("bot %q in org %q: %w", id, orgID, store.ErrNotFound)
	}
	if b.AgentID != "" {
		return false, nil
	}
	r.rows[k] = b.WithAgentID(appID)
	return true, nil
}

// Delete removes the bot and cascades the rows that reference it,
// matching the gorm store: the bot's own subscriptions and every
// reporting line where it is the manager or the report are dropped.
func (r *nodesRepo) Delete(_ context.Context, orgID string, id orgchart.NodeID) error {
	r.mu.Lock()
	k := orgKey{OrgID: orgID, ID: string(id)}
	if _, ok := r.rows[k]; !ok {
		r.mu.Unlock()
		return fmt.Errorf("bot %q in org %q: %w", id, orgID, store.ErrNotFound)
	}
	delete(r.rows, k)
	r.mu.Unlock()
	// Cascade under the dependent repos' own mutexes — release ours
	// first to avoid lock-ordering hazards.
	if r.lines != nil {
		r.lines.deleteAllForBot(orgID, id)
	}
	if r.attachments != nil {
		r.attachments.deleteForWorker(orgID, string(id))
	}
	return nil
}

// ---- ReportingLines ----------------------------------------------------

// lineKey is the composite (org, manager, report) PK the memory repo
// keys on — mirrors the gorm reportingLineRow composite PK.
type lineKey struct {
	OrgID     string
	ManagerID string
	ReportID  string
}

type reportingLinesRepo struct {
	mu   sync.RWMutex
	rows map[lineKey]struct{}
}

func (r *reportingLinesRepo) Add(_ context.Context, line orgchart.ReportingLine) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	// Idempotent: re-adding an existing edge is a no-op.
	r.rows[lineKey{OrgID: line.OrgID, ManagerID: string(line.ManagerID), ReportID: string(line.ReportID)}] = struct{}{}
	return nil
}

func (r *reportingLinesRepo) Remove(_ context.Context, orgID string, reportID, managerID orgchart.NodeID) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	k := lineKey{OrgID: orgID, ManagerID: string(managerID), ReportID: string(reportID)}
	if _, ok := r.rows[k]; !ok {
		return fmt.Errorf("reporting line %q→%q in org %q: %w", reportID, managerID, orgID, store.ErrNotFound)
	}
	delete(r.rows, k)
	return nil
}

func (r *reportingLinesRepo) List(_ context.Context, orgID string) ([]orgchart.ReportingLine, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]orgchart.ReportingLine, 0)
	for k := range r.rows {
		if k.OrgID == orgID {
			out = append(out, orgchart.ReportingLine{OrgID: k.OrgID, ManagerID: orgchart.NodeID(k.ManagerID), ReportID: orgchart.NodeID(k.ReportID)})
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].ManagerID != out[j].ManagerID {
			return out[i].ManagerID < out[j].ManagerID
		}
		return out[i].ReportID < out[j].ReportID
	})
	return out, nil
}

func (r *reportingLinesRepo) ListManagers(_ context.Context, orgID string, reportID orgchart.NodeID) ([]orgchart.NodeID, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]orgchart.NodeID, 0)
	for k := range r.rows {
		if k.OrgID == orgID && k.ReportID == string(reportID) {
			out = append(out, orgchart.NodeID(k.ManagerID))
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out, nil
}

func (r *reportingLinesRepo) ListReports(_ context.Context, orgID string, managerID orgchart.NodeID) ([]orgchart.NodeID, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]orgchart.NodeID, 0)
	for k := range r.rows {
		if k.OrgID == orgID && k.ManagerID == string(managerID) {
			out = append(out, orgchart.NodeID(k.ReportID))
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out, nil
}

// deleteAllForBot drops every reporting line where the bot is the
// manager or the report. Used by  nodesRepo.Delete to cascade — the
// memory-store analogue of the gorm ON DELETE CASCADE foreign keys.
func (r *reportingLinesRepo) deleteAllForBot(orgID string, botID orgchart.NodeID) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for k := range r.rows {
		if k.OrgID == orgID && (k.ManagerID == string(botID) || k.ReportID == string(botID)) {
			delete(r.rows, k)
		}
	}
}

// ---- NodeRuntimeState ---------------------------------------------------

type runtimeKey struct {
	OrgID   string
	NodeID  string
	Backend string
	Key     string
}

type runtimeStateRepo struct {
	mu   sync.RWMutex
	rows map[runtimeKey]string
}

func (r *runtimeStateRepo) Get(_ context.Context, orgID string, botID orgchart.NodeID, backend string) (map[string]string, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := map[string]string{}
	for k, v := range r.rows {
		if k.OrgID == orgID && k.NodeID == string(botID) && k.Backend == backend {
			out[k.Key] = v
		}
	}
	return out, nil
}

func (r *runtimeStateRepo) Set(_ context.Context, orgID string, botID orgchart.NodeID, backend, key, value string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.rows[runtimeKey{OrgID: orgID, NodeID: string(botID), Backend: backend, Key: key}] = value
	return nil
}

func (r *runtimeStateRepo) SetMany(_ context.Context, orgID string, botID orgchart.NodeID, backend string, kv map[string]string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	for key, value := range kv {
		r.rows[runtimeKey{OrgID: orgID, NodeID: string(botID), Backend: backend, Key: key}] = value
	}
	return nil
}

func (r *runtimeStateRepo) Clear(_ context.Context, orgID string, botID orgchart.NodeID, backend string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	for k := range r.rows {
		if k.OrgID == orgID && k.NodeID == string(botID) && k.Backend == backend {
			delete(r.rows, k)
		}
	}
	return nil
}

// ---- Events ------------------------------------------------------------

type eventsRepo struct {
	mu   sync.RWMutex
	rows []streaming.Event // append-only, newest at end
}

func (e *eventsRepo) Append(_ context.Context, ev streaming.Event) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	for _, existing := range e.rows {
		if existing.OrganizationID == ev.OrganizationID && existing.ID == ev.ID {
			return fmt.Errorf("event %q: %w", ev.ID, store.ErrConflict)
		}
	}
	e.rows = append(e.rows, ev)
	return nil
}

func (e *eventsRepo) DeleteForStream(_ context.Context, orgID string, streamID streaming.StreamID) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	kept := e.rows[:0]
	for _, ev := range e.rows {
		if ev.OrganizationID != orgID || ev.StreamID != streamID {
			kept = append(kept, ev)
		}
	}
	e.rows = kept
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

func (e *eventsRepo) PageForStream(_ context.Context, orgID string, streamID streaming.StreamID, limit, offset int) ([]streaming.Event, error) {
	e.mu.RLock()
	defer e.mu.RUnlock()
	out := make([]streaming.Event, 0)
	skipped := 0
	// Newest first, same ordering as ListForStream.
	for i := len(e.rows) - 1; i >= 0; i-- {
		ev := e.rows[i]
		if ev.OrganizationID != orgID || ev.StreamID != streamID {
			continue
		}
		if offset > 0 && skipped < offset {
			skipped++
			continue
		}
		out = append(out, ev)
		if limit > 0 && len(out) >= limit {
			break
		}
	}
	return out, nil
}

func (e *eventsRepo) CountForStream(_ context.Context, orgID string, streamID streaming.StreamID) (int, error) {
	e.mu.RLock()
	defer e.mu.RUnlock()
	count := 0
	for _, ev := range e.rows {
		if ev.OrganizationID == orgID && ev.StreamID == streamID {
			count++
		}
	}
	return count, nil
}

func (e *eventsRepo) ListForStreams(_ context.Context, orgID string, streamIDs []streaming.StreamID, limit int) ([]streaming.Event, error) {
	if len(streamIDs) == 0 {
		return nil, nil
	}
	wanted := make(map[streaming.StreamID]bool, len(streamIDs))
	for _, id := range streamIDs {
		wanted[id] = true
	}
	e.mu.RLock()
	defer e.mu.RUnlock()
	out := make([]streaming.Event, 0)
	for i := len(e.rows) - 1; i >= 0; i-- {
		ev := e.rows[i]
		if ev.OrganizationID != orgID || !wanted[ev.StreamID] {
			continue
		}
		out = append(out, ev)
		if limit > 0 && len(out) >= limit {
			break
		}
	}
	return out, nil
}

func (e *eventsRepo) ListSince(_ context.Context, orgID string, topicIDs []streaming.StreamID, since streaming.EventID, limit int) ([]streaming.Event, error) {
	// Empty topic set returns nothing — the caller passed no topics
	// to listen on, so there's nothing to return. Matches gorm's
	// IN ()-on-empty behaviour.
	if len(topicIDs) == 0 {
		return []streaming.Event{}, nil
	}
	e.mu.RLock()
	defer e.mu.RUnlock()
	wanted := map[streaming.StreamID]bool{}
	for _, s := range topicIDs {
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

func (c *configsRepo) DeleteIfValue(_ context.Context, orgID, key, value string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	k := orgKey{OrgID: orgID, ID: key}
	if cfg, ok := c.rows[k]; ok && cfg.Value == value {
		delete(c.rows, k)
	}
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

func (a *activationsRepo) ListForWorker(_ context.Context, orgID string, botID orgchart.NodeID, limit int) ([]*activation.Activation, error) {
	a.mu.RLock()
	defer a.mu.RUnlock()
	out := make([]*activation.Activation, 0)
	for k, act := range a.rows {
		if k.OrgID != orgID || act.WorkerID != botID {
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

// ---- Retired pre-cutover model -----------------------------------------
//
// The Topic model is gone from the runtime, but application/cutover still
// reads it to convert deployed data. The memory store keeps a seedable
// stand-in so that conversion can be tested without Postgres: SeedRetired
// writes what the last pre-cutover release would have left behind.

type retiredRepo struct {
	mu             sync.RWMutex
	topics         []streaming.Topic
	subscriptions  []streaming.Subscription
	processorInput map[store.OrgScopedID]streaming.StreamID
}

// SeedRetired writes pre-cutover rows into the store's retired read
// model. Call it before cutover.Convert to exercise an upgrade.
func SeedRetired(s *store.Store, topics []streaming.Topic, subs []streaming.Subscription, processorInputs map[store.OrgScopedID]streaming.StreamID) {
	r, ok := s.RetiredTopics.(*retiredRepo)
	if !ok {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.topics = append(r.topics, topics...)
	r.subscriptions = append(r.subscriptions, subs...)
	for k, v := range processorInputs {
		r.processorInput[k] = v
	}
}

func (r *retiredRepo) ListAll(_ context.Context) ([]streaming.Topic, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return append([]streaming.Topic(nil), r.topics...), nil
}

func (r *retiredRepo) Delete(_ context.Context, orgID, topicID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	kept := make([]streaming.Topic, 0, len(r.topics))
	for _, t := range r.topics {
		if t.OrganizationID == orgID && t.ID == topicID {
			continue
		}
		kept = append(kept, t)
	}
	r.topics = kept
	return nil
}

type retiredSubsRepo struct{ inner *retiredRepo }

func (r *retiredSubsRepo) ListAll(_ context.Context) ([]streaming.Subscription, error) {
	r.inner.mu.RLock()
	defer r.inner.mu.RUnlock()
	return append([]streaming.Subscription(nil), r.inner.subscriptions...), nil
}

func (r *retiredSubsRepo) Delete(_ context.Context, orgID, workerID, topicID string) error {
	r.inner.mu.Lock()
	defer r.inner.mu.Unlock()
	kept := make([]streaming.Subscription, 0, len(r.inner.subscriptions))
	for _, sub := range r.inner.subscriptions {
		if sub.OrganizationID == orgID && sub.NodeID == workerID && sub.TopicID == topicID {
			continue
		}
		kept = append(kept, sub)
	}
	r.inner.subscriptions = kept
	return nil
}

type retiredInputsRepo struct{ inner *retiredRepo }

func (r *retiredInputsRepo) ListAll(_ context.Context) (map[store.OrgScopedID]streaming.StreamID, error) {
	r.inner.mu.RLock()
	defer r.inner.mu.RUnlock()
	out := make(map[store.OrgScopedID]streaming.StreamID, len(r.inner.processorInput))
	for k, v := range r.inner.processorInput {
		out[k] = v
	}
	return out, nil
}

func (r *retiredInputsRepo) Clear(_ context.Context, orgID, processorID string) error {
	r.inner.mu.Lock()
	defer r.inner.mu.Unlock()
	delete(r.inner.processorInput, store.OrgScopedID{OrgID: orgID, ID: processorID})
	return nil
}
