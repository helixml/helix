// Package queries is the read facade for the org graph. It is the
// single application-layer home for the projection reads that the REST
// read handlers and the per-Worker MCP server used to make directly
// against the store repositories.
//
// Unlike the per-aggregate mutation services (topics/roles/workers/…),
// this is intentionally ONE service spanning several repos: reads carry
// no invariants to keep honest, so there is nothing to split on, and the
// design (§5.3/§8) explicitly sanctions "a thin query service for
// consistency." Methods return domain aggregates — DTO mapping stays in
// the adapter. Each method is one repo call; no business logic lives
// here.
package queries

import (
	"context"

	"github.com/helixml/helix/api/pkg/org/domain/activation"
	"github.com/helixml/helix/api/pkg/org/domain/orgchart"
	"github.com/helixml/helix/api/pkg/org/domain/store"
	"github.com/helixml/helix/api/pkg/org/domain/streaming"
)

// Queries reads the org graph. Constructed once at the composition root
// from the narrow read repositories.
type Queries struct {
	roles       store.Roles
	workers     store.Workers
	lines       store.ReportingLines
	topics     store.Topics
	subs        store.Subscriptions
	events      store.Events
	activations activation.Repository
}

// Deps are the constructor-injected read repositories. Any may be nil if
// a deployment doesn't wire that aggregate; the matching method then
// returns an error from the nil repo (callers already tolerate read
// failures by degrading the projection).
type Deps struct {
	Roles          store.Roles
	Workers        store.Workers
	ReportingLines store.ReportingLines
	Topics        store.Topics
	Subscriptions  store.Subscriptions
	Events         store.Events
	Activations    activation.Repository
}

// New constructs the read facade.
func New(deps Deps) *Queries {
	return &Queries{
		roles:       deps.Roles,
		workers:     deps.Workers,
		lines:       deps.ReportingLines,
		topics:     deps.Topics,
		subs:        deps.Subscriptions,
		events:      deps.Events,
		activations: deps.Activations,
	}
}

func (q *Queries) ListWorkers(ctx context.Context, orgID string) ([]orgchart.Worker, error) {
	return q.workers.List(ctx, orgID)
}

func (q *Queries) GetWorker(ctx context.Context, orgID string, id orgchart.BotID) (orgchart.Worker, error) {
	return q.workers.Get(ctx, orgID, id)
}

func (q *Queries) ListRoles(ctx context.Context, orgID string) ([]orgchart.Role, error) {
	return q.roles.List(ctx, orgID)
}

func (q *Queries) GetRole(ctx context.Context, orgID string, id orgchart.BotID) (orgchart.Role, error) {
	return q.roles.Get(ctx, orgID, id)
}

// ReportingLinesWired reports whether the reporting-lines repo is
// available — some handlers 501 / degrade when it isn't.
func (q *Queries) ReportingLinesWired() bool { return q.lines != nil }

func (q *Queries) ListReportingLines(ctx context.Context, orgID string) ([]orgchart.ReportingLine, error) {
	return q.lines.List(ctx, orgID)
}

func (q *Queries) ListManagers(ctx context.Context, orgID string, reportID orgchart.BotID) ([]orgchart.BotID, error) {
	return q.lines.ListManagers(ctx, orgID, reportID)
}

func (q *Queries) ListTopics(ctx context.Context, orgID string) ([]streaming.Topic, error) {
	return q.topics.List(ctx, orgID)
}

func (q *Queries) GetTopic(ctx context.Context, orgID string, id streaming.TopicID) (streaming.Topic, error) {
	return q.topics.Get(ctx, orgID, id)
}

func (q *Queries) TopicSubscribers(ctx context.Context, orgID string, topicID streaming.TopicID) ([]streaming.Subscription, error) {
	return q.subs.ListForTopic(ctx, orgID, topicID)
}

func (q *Queries) WorkerSubscriptions(ctx context.Context, orgID string, workerID orgchart.BotID) ([]streaming.Subscription, error) {
	return q.subs.ListForWorker(ctx, orgID, workerID)
}

func (q *Queries) TopicEvents(ctx context.Context, orgID string, topicID streaming.TopicID, limit int) ([]streaming.Event, error) {
	return q.events.ListForTopic(ctx, orgID, topicID, limit)
}

func (q *Queries) AllEvents(ctx context.Context, orgID string, limit int) ([]streaming.Event, error) {
	return q.events.ListAll(ctx, orgID, limit)
}

// PageTopicEvents returns a page of events on a Topic, newest first,
// for the paginated REST messages endpoint.
func (q *Queries) PageTopicEvents(ctx context.Context, orgID string, topicID streaming.TopicID, limit, offset int) ([]streaming.Event, error) {
	return q.events.PageForTopic(ctx, orgID, topicID, limit, offset)
}

// CountTopicEvents returns the total number of events on a Topic —
// the total-count meta the paginated messages endpoint surfaces.
func (q *Queries) CountTopicEvents(ctx context.Context, orgID string, topicID streaming.TopicID) (int, error) {
	return q.events.CountForTopic(ctx, orgID, topicID)
}

func (q *Queries) WorkerEvents(ctx context.Context, orgID string, workerID orgchart.BotID, limit int) ([]streaming.Event, error) {
	return q.events.ListForWorker(ctx, orgID, workerID, limit)
}

// ListReports returns the direct reports of the given manager.
func (q *Queries) ListReports(ctx context.Context, orgID string, managerID orgchart.BotID) ([]orgchart.BotID, error) {
	return q.lines.ListReports(ctx, orgID, managerID)
}

// FindSubscription returns the (worker, topic) subscription row, or
// store.ErrNotFound (wrapped) when the worker is not subscribed.
func (q *Queries) FindSubscription(ctx context.Context, orgID string, workerID orgchart.BotID, topicID streaming.TopicID) (streaming.Subscription, error) {
	return q.subs.Find(ctx, orgID, workerID, topicID)
}

// GetActivation returns one activation audit row by id.
func (q *Queries) GetActivation(ctx context.Context, orgID string, id activation.ID) (*activation.Activation, error) {
	return q.activations.Get(ctx, orgID, id)
}
