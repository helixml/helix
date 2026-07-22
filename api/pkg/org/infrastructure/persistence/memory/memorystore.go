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
	"errors"
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/helixml/helix/api/pkg/org/domain/activation"
	"github.com/helixml/helix/api/pkg/org/domain/config"
	"github.com/helixml/helix/api/pkg/org/domain/domainevent"
	"github.com/helixml/helix/api/pkg/org/domain/orgchart"
	"github.com/helixml/helix/api/pkg/org/domain/processor"
	"github.com/helixml/helix/api/pkg/org/domain/store"
	"github.com/helixml/helix/api/pkg/org/domain/streaming"
	"github.com/helixml/helix/api/pkg/org/domain/transport"
)

// New returns a fresh *store.Store backed by in-memory repos. Use
// for tests and dev paths that don't need Postgres.
func New() *store.Store {
	subs := &subscriptionsRepo{rows: map[subKey]streaming.Subscription{}}
	lines := &reportingLinesRepo{rows: map[lineKey]struct{}{}}
	bots := &botsRepo{rows: map[orgKey]orgchart.Bot{}, subs: subs, lines: lines}
	topics := &topicsRepo{rows: map[orgKey]streaming.Topic{}, subs: subs}
	return &store.Store{
		Bots:            bots,
		ReportingLines:  lines,
		BotRuntimeState: &runtimeStateRepo{rows: map[runtimeKey]string{}},
		Topics:          topics,
		Subscriptions:   subs,
		Events:          &eventsRepo{rows: []streaming.Event{}, subs: subs, bots: bots},
		Configs:         &configsRepo{rows: map[orgKey]config.Config{}},
		Activations:     &activationsRepo{rows: map[orgKey]*activation.Activation{}},
		Processors:      &processorsRepo{rows: map[orgKey]processor.Processor{}},
		ChartPositions:  &chartPositionsRepo{rows: map[chartPosKey]orgchart.ChartPosition{}},
		DomainEvents:    &domainEventsRepo{},
	}
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

// ---- Bots ---------------------------------------------------------------

type botsRepo struct {
	mu   sync.RWMutex
	rows map[orgKey]orgchart.Bot
	// subs and lines are held by reference so Delete can cascade: a
	// deleted bot's own subscriptions and every reporting line that
	// references it (as manager or report) are dropped, mirroring the
	// gorm store's ON DELETE CASCADE foreign keys.
	subs  *subscriptionsRepo
	lines *reportingLinesRepo
}

func (r *botsRepo) Create(_ context.Context, b orgchart.Bot) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	k := orgKey{OrgID: b.OrganizationID, ID: string(b.ID)}
	if _, ok := r.rows[k]; ok {
		return fmt.Errorf("bot %q in org %q: already exists", b.ID, b.OrganizationID)
	}
	r.rows[k] = b
	return nil
}

func (r *botsRepo) Get(_ context.Context, orgID string, id orgchart.BotID) (orgchart.Bot, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if b, ok := r.rows[orgKey{OrgID: orgID, ID: string(id)}]; ok {
		return b, nil
	}
	return orgchart.Bot{}, fmt.Errorf("bot %q in org %q: %w", id, orgID, store.ErrNotFound)
}

func (r *botsRepo) List(_ context.Context, orgID string) ([]orgchart.Bot, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]orgchart.Bot, 0)
	for k, b := range r.rows {
		if k.OrgID == orgID {
			out = append(out, b)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, nil
}

func (r *botsRepo) Update(_ context.Context, b orgchart.Bot) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	k := orgKey{OrgID: b.OrganizationID, ID: string(b.ID)}
	if _, ok := r.rows[k]; !ok {
		return fmt.Errorf("bot %q in org %q: %w", b.ID, b.OrganizationID, store.ErrNotFound)
	}
	r.rows[k] = b
	return nil
}

// Delete removes the bot and cascades the rows that reference it,
// matching the gorm store: the bot's own subscriptions and every
// reporting line where it is the manager or the report are dropped.
func (r *botsRepo) Delete(_ context.Context, orgID string, id orgchart.BotID) error {
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
	if r.subs != nil {
		r.subs.deleteAllForBot(orgID, id)
	}
	if r.lines != nil {
		r.lines.deleteAllForBot(orgID, id)
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

func (r *reportingLinesRepo) Remove(_ context.Context, orgID string, reportID, managerID orgchart.BotID) error {
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
			out = append(out, orgchart.ReportingLine{OrgID: k.OrgID, ManagerID: orgchart.BotID(k.ManagerID), ReportID: orgchart.BotID(k.ReportID)})
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

func (r *reportingLinesRepo) ListManagers(_ context.Context, orgID string, reportID orgchart.BotID) ([]orgchart.BotID, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]orgchart.BotID, 0)
	for k := range r.rows {
		if k.OrgID == orgID && k.ReportID == string(reportID) {
			out = append(out, orgchart.BotID(k.ManagerID))
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out, nil
}

func (r *reportingLinesRepo) ListReports(_ context.Context, orgID string, managerID orgchart.BotID) ([]orgchart.BotID, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]orgchart.BotID, 0)
	for k := range r.rows {
		if k.OrgID == orgID && k.ManagerID == string(managerID) {
			out = append(out, orgchart.BotID(k.ReportID))
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out, nil
}

// deleteAllForBot drops every reporting line where the bot is the
// manager or the report. Used by botsRepo.Delete to cascade — the
// memory-store analogue of the gorm ON DELETE CASCADE foreign keys.
func (r *reportingLinesRepo) deleteAllForBot(orgID string, botID orgchart.BotID) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for k := range r.rows {
		if k.OrgID == orgID && (k.ManagerID == string(botID) || k.ReportID == string(botID)) {
			delete(r.rows, k)
		}
	}
}

// ---- BotRuntimeState ---------------------------------------------------

type runtimeKey struct {
	OrgID   string
	BotID   string
	Backend string
	Key     string
}

type runtimeStateRepo struct {
	mu   sync.RWMutex
	rows map[runtimeKey]string
}

func (r *runtimeStateRepo) Get(_ context.Context, orgID string, botID orgchart.BotID, backend string) (map[string]string, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := map[string]string{}
	for k, v := range r.rows {
		if k.OrgID == orgID && k.BotID == string(botID) && k.Backend == backend {
			out[k.Key] = v
		}
	}
	return out, nil
}

func (r *runtimeStateRepo) Set(_ context.Context, orgID string, botID orgchart.BotID, backend, key, value string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.rows[runtimeKey{OrgID: orgID, BotID: string(botID), Backend: backend, Key: key}] = value
	return nil
}

func (r *runtimeStateRepo) SetMany(_ context.Context, orgID string, botID orgchart.BotID, backend string, kv map[string]string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	for key, value := range kv {
		r.rows[runtimeKey{OrgID: orgID, BotID: string(botID), Backend: backend, Key: key}] = value
	}
	return nil
}

func (r *runtimeStateRepo) Clear(_ context.Context, orgID string, botID orgchart.BotID, backend string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	for k := range r.rows {
		if k.OrgID == orgID && k.BotID == string(botID) && k.Backend == backend {
			delete(r.rows, k)
		}
	}
	return nil
}

// ---- Topics ------------------------------------------------------------

type topicsRepo struct {
	mu   sync.RWMutex
	rows map[orgKey]streaming.Topic
	// subs is held by reference so Delete can cascade: every
	// subscription to a deleted topic is dropped, mirroring the gorm
	// store.
	subs *subscriptionsRepo
}

func (s *topicsRepo) Create(_ context.Context, st streaming.Topic) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	k := orgKey{OrgID: st.OrganizationID, ID: string(st.ID)}
	if _, ok := s.rows[k]; ok {
		return fmt.Errorf("topic %q in org %q: already exists", st.ID, st.OrganizationID)
	}
	// Enforce composite (org_id, name) uniqueness to mirror the gorm
	// idx_topic_org_name constraint. Wrap store.ErrConflict so adapters map
	// it to 409 (and the topics service's pre-check reads it).
	for k2, ex := range s.rows {
		if k2.OrgID == st.OrganizationID && ex.Name == st.Name {
			return fmt.Errorf("a topic named %q in this org %w", st.Name, store.ErrConflict)
		}
	}
	s.rows[k] = st
	return nil
}

func (s *topicsRepo) Get(_ context.Context, orgID string, id streaming.TopicID) (streaming.Topic, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if st, ok := s.rows[orgKey{OrgID: orgID, ID: string(id)}]; ok {
		return st, nil
	}
	return streaming.Topic{}, fmt.Errorf("topic %q in org %q: %w", id, orgID, store.ErrNotFound)
}

func (s *topicsRepo) List(_ context.Context, orgID string) ([]streaming.Topic, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]streaming.Topic, 0)
	for k, st := range s.rows {
		if k.OrgID == orgID {
			out = append(out, st)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, nil
}

func (s *topicsRepo) ListByTransportKind(_ context.Context, kind transport.Kind) ([]streaming.Topic, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]streaming.Topic, 0)
	for _, st := range s.rows {
		if st.Transport.Kind == kind {
			out = append(out, st)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].OrganizationID != out[j].OrganizationID {
			return out[i].OrganizationID < out[j].OrganizationID
		}
		return out[i].ID < out[j].ID
	})
	return out, nil
}

func (s *topicsRepo) Update(_ context.Context, st streaming.Topic) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	k := orgKey{OrgID: st.OrganizationID, ID: string(st.ID)}
	existing, ok := s.rows[k]
	if !ok {
		return fmt.Errorf("topic %q in org %q: %w", st.ID, st.OrganizationID, store.ErrNotFound)
	}
	// Re-check the composite (org_id, name) uniqueness when name
	// changes — mirror the gorm idx_topic_org_name constraint.
	if st.Name != existing.Name {
		for k2, ex := range s.rows {
			if k2 == k {
				continue
			}
			if k2.OrgID == st.OrganizationID && ex.Name == st.Name {
				return fmt.Errorf("topic name %q already in use in org %q", st.Name, st.OrganizationID)
			}
		}
	}
	// Mutable fields only — keep CreatedBy / CreatedAt / ID / OrgID.
	existing.Name = st.Name
	existing.Description = st.Description
	existing.Transport = st.Transport
	s.rows[k] = existing
	return nil
}

// Delete removes the topic and cascades its subscriptions, matching
// the gorm store: every bot-anchored row for this topic is dropped
// so none dangle past the topic row.
func (s *topicsRepo) Delete(_ context.Context, orgID string, id streaming.TopicID) error {
	s.mu.Lock()
	key := orgKey{OrgID: orgID, ID: string(id)}
	if _, ok := s.rows[key]; !ok {
		s.mu.Unlock()
		return fmt.Errorf("topic %q in org %q: %w", id, orgID, store.ErrNotFound)
	}
	delete(s.rows, key)
	s.mu.Unlock()
	if s.subs != nil {
		s.subs.deleteAllForTopic(orgID, id)
	}
	return nil
}

// ---- Subscriptions ------------------------------------------------------

type subKey struct {
	OrgID   string
	BotID   string
	TopicID string
}

type subscriptionsRepo struct {
	mu   sync.RWMutex
	rows map[subKey]streaming.Subscription
}

func (s *subscriptionsRepo) Create(_ context.Context, sub streaming.Subscription) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	k := subKey{OrgID: sub.OrganizationID, BotID: string(sub.BotID), TopicID: string(sub.TopicID)}
	if _, ok := s.rows[k]; ok {
		return fmt.Errorf("subscription %q→%q in org %q: already exists", sub.BotID, sub.TopicID, sub.OrganizationID)
	}
	s.rows[k] = sub
	return nil
}

func (s *subscriptionsRepo) Delete(_ context.Context, orgID string, botID orgchart.BotID, topicID streaming.TopicID) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	k := subKey{OrgID: orgID, BotID: string(botID), TopicID: string(topicID)}
	if _, ok := s.rows[k]; !ok {
		return fmt.Errorf("subscription %q→%q in org %q: %w", botID, topicID, orgID, store.ErrNotFound)
	}
	delete(s.rows, k)
	return nil
}

// deleteAllForBot drops every subscription held by the given bot.
// Used by botsRepo.Delete to cascade — idempotent, no error when the
// bot has none.
func (s *subscriptionsRepo) deleteAllForBot(orgID string, botID orgchart.BotID) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for k := range s.rows {
		if k.OrgID == orgID && k.BotID == string(botID) {
			delete(s.rows, k)
		}
	}
}

// deleteAllForTopic drops every subscription to the given topic.
// Used by topicsRepo.Delete to cascade — idempotent, no error when
// the topic has no subscribers.
func (s *subscriptionsRepo) deleteAllForTopic(orgID string, topicID streaming.TopicID) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for k := range s.rows {
		if k.OrgID == orgID && k.TopicID == string(topicID) {
			delete(s.rows, k)
		}
	}
}

func (s *subscriptionsRepo) Find(_ context.Context, orgID string, botID orgchart.BotID, topicID streaming.TopicID) (streaming.Subscription, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	k := subKey{OrgID: orgID, BotID: string(botID), TopicID: string(topicID)}
	if sub, ok := s.rows[k]; ok {
		return sub, nil
	}
	return streaming.Subscription{}, fmt.Errorf("subscription %q→%q in org %q: %w", botID, topicID, orgID, store.ErrNotFound)
}

func (s *subscriptionsRepo) ListForBot(_ context.Context, orgID string, botID orgchart.BotID) ([]streaming.Subscription, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]streaming.Subscription, 0)
	for k, sub := range s.rows {
		if k.OrgID == orgID && k.BotID == string(botID) {
			out = append(out, sub)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].TopicID < out[j].TopicID })
	return out, nil
}

func (s *subscriptionsRepo) ListForTopic(_ context.Context, orgID string, topicID streaming.TopicID) ([]streaming.Subscription, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]streaming.Subscription, 0)
	for k, sub := range s.rows {
		if k.OrgID == orgID && k.TopicID == string(topicID) {
			out = append(out, sub)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].BotID < out[j].BotID })
	return out, nil
}

// ---- Events -------------------------------------------------------------

type eventsRepo struct {
	mu   sync.RWMutex
	rows []streaming.Event // append-only, newest at end
	// subs + bots are held by reference so ListForBot can join against
	// subscriptions for the bot the same way the gorm impl does.
	subs *subscriptionsRepo
	bots *botsRepo
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

func (e *eventsRepo) DeleteForTopic(_ context.Context, orgID string, topicID streaming.TopicID) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	kept := e.rows[:0]
	for _, ev := range e.rows {
		if ev.OrganizationID != orgID || ev.TopicID != topicID {
			kept = append(kept, ev)
		}
	}
	e.rows = kept
	return nil
}

func (e *eventsRepo) ListForTopic(_ context.Context, orgID string, topicID streaming.TopicID, limit int) ([]streaming.Event, error) {
	e.mu.RLock()
	defer e.mu.RUnlock()
	out := make([]streaming.Event, 0)
	// Newest first.
	for i := len(e.rows) - 1; i >= 0; i-- {
		ev := e.rows[i]
		if ev.OrganizationID != orgID || ev.TopicID != topicID {
			continue
		}
		out = append(out, ev)
		if limit > 0 && len(out) >= limit {
			break
		}
	}
	return out, nil
}

func (e *eventsRepo) PageForTopic(_ context.Context, orgID string, topicID streaming.TopicID, limit, offset int) ([]streaming.Event, error) {
	e.mu.RLock()
	defer e.mu.RUnlock()
	out := make([]streaming.Event, 0)
	skipped := 0
	// Newest first, same ordering as ListForTopic.
	for i := len(e.rows) - 1; i >= 0; i-- {
		ev := e.rows[i]
		if ev.OrganizationID != orgID || ev.TopicID != topicID {
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

func (e *eventsRepo) CountForTopic(_ context.Context, orgID string, topicID streaming.TopicID) (int, error) {
	e.mu.RLock()
	defer e.mu.RUnlock()
	count := 0
	for _, ev := range e.rows {
		if ev.OrganizationID == orgID && ev.TopicID == topicID {
			count++
		}
	}
	return count, nil
}

func (e *eventsRepo) ListForBot(ctx context.Context, orgID string, botID orgchart.BotID, limit int) ([]streaming.Event, error) {
	// Match gorm's join semantics: events on topics the bot is
	// subscribed to. Subscriptions are bot-anchored.
	if e.bots == nil {
		return nil, errors.New("eventsRepo: bots repo not wired")
	}
	if _, err := e.bots.Get(ctx, orgID, botID); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return nil, nil
		}
		return nil, fmt.Errorf("resolve bot for event listing: %w", err)
	}
	subs, err := e.subs.ListForBot(ctx, orgID, botID)
	if err != nil {
		return nil, err
	}
	subscribed := map[streaming.TopicID]bool{}
	for _, sub := range subs {
		subscribed[sub.TopicID] = true
	}
	e.mu.RLock()
	defer e.mu.RUnlock()
	out := make([]streaming.Event, 0)
	for i := len(e.rows) - 1; i >= 0; i-- {
		ev := e.rows[i]
		if ev.OrganizationID != orgID || !subscribed[ev.TopicID] {
			continue
		}
		out = append(out, ev)
		if limit > 0 && len(out) >= limit {
			break
		}
	}
	return out, nil
}

func (e *eventsRepo) ListSince(_ context.Context, orgID string, topicIDs []streaming.TopicID, since streaming.EventID, limit int) ([]streaming.Event, error) {
	// Empty topic set returns nothing — the caller passed no topics
	// to listen on, so there's nothing to return. Matches gorm's
	// IN ()-on-empty behaviour.
	if len(topicIDs) == 0 {
		return []streaming.Event{}, nil
	}
	e.mu.RLock()
	defer e.mu.RUnlock()
	wanted := map[streaming.TopicID]bool{}
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
		if !wanted[ev.TopicID] {
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

func (a *activationsRepo) ListForWorker(_ context.Context, orgID string, botID orgchart.BotID, limit int) ([]*activation.Activation, error) {
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
