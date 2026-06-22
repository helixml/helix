// Package store defines the persistence contracts for the org-graph
// subsystem (workers, positions, roles, topics, events,
// subscriptions, activations, environments, configs). The concrete
// implementation lives in the sibling gorm sub-package — dialect-
// portable GORM, wired against helix's Postgres connection.
package store

import (
	"context"
	"errors"

	"github.com/helixml/helix/api/pkg/org/domain/activation"
	"github.com/helixml/helix/api/pkg/org/domain/config"
	"github.com/helixml/helix/api/pkg/org/domain/orgchart"
	"github.com/helixml/helix/api/pkg/org/domain/processor"
	"github.com/helixml/helix/api/pkg/org/domain/streaming"
	"github.com/helixml/helix/api/pkg/org/domain/transport"
)

// ErrNotFound signals that the requested record does not exist.
// Repos wrap this with %w so callers can errors.Is it.
var ErrNotFound = errors.New("record not found")

// ErrConflict signals a uniqueness violation (e.g. two rows with the
// same org-scoped name). Repos wrap it with %w and a human-readable
// prefix; adapters errors.Is it to map to 409 Conflict instead of
// leaking the raw driver error.
var ErrConflict = errors.New("already exists")

// Every store method takes an explicit `orgID string` parameter
// (except Create/Update, where the org is carried by the domain
// aggregate). The composite (id, org_id) PK is what lets short
// readable IDs (`w-owner`, `p-root`, `r-owner`) repeat across helix
// tenants. ErrNotFound is returned when the (orgID, id) pair doesn't
// exist — even if the bare id exists under another org.

// Roles persists job descriptions.
type Roles interface {
	Create(ctx context.Context, role orgchart.Role) error
	Get(ctx context.Context, orgID string, id orgchart.RoleID) (orgchart.Role, error)
	List(ctx context.Context, orgID string) ([]orgchart.Role, error)
	Update(ctx context.Context, role orgchart.Role) error
	// Delete removes the role row. Caller is expected to have torn
	// down dependent workers; the lifecycle service in
	// application/lifecycle owns the cascade.
	Delete(ctx context.Context, orgID string, id orgchart.RoleID) error
}

// Workers persists humans and AIs. Update mutates fields the system
// allows changing in place — currently just IdentityContent (set at
// hire by the caller, replaced wholesale by update_identity). Identity
// is the per-Worker description; the system holds it in the domain
// rather than on disk so it survives any change in env layout.
//
// Delete removes the worker row and structurally cascades the rows
// that reference it: its subscriptions (worker-anchored) and every
// reporting line where it is the manager or the report. See the gorm
// and memory implementations.
type Workers interface {
	Create(ctx context.Context, worker orgchart.Worker) error
	Get(ctx context.Context, orgID string, id orgchart.WorkerID) (orgchart.Worker, error)
	List(ctx context.Context, orgID string) ([]orgchart.Worker, error)
	Update(ctx context.Context, worker orgchart.Worker) error
	Delete(ctx context.Context, orgID string, id orgchart.WorkerID) error
}

// ReportingLines persists the org's many-to-many reporting graph:
// each row says ReportID reports to ManagerID. Worker-anchored on both
// ends — deleting either endpoint Worker drops the line (the gorm
// store enforces this with ON DELETE CASCADE foreign keys; the memory
// store mirrors it). The graph is a DAG; cycle prevention lives in the
// add-parent handler, not here.
type ReportingLines interface {
	// Add inserts a (manager, report) line. Idempotent: re-adding an
	// existing line is a no-op (no error).
	Add(ctx context.Context, line orgchart.ReportingLine) error
	// Remove drops the (report → manager) line. Returns ErrNotFound
	// when no such line exists.
	Remove(ctx context.Context, orgID string, reportID, managerID orgchart.WorkerID) error
	// List returns every reporting line in the org.
	List(ctx context.Context, orgID string) ([]orgchart.ReportingLine, error)
	// ListManagers returns the managers the given report reports to.
	ListManagers(ctx context.Context, orgID string, reportID orgchart.WorkerID) ([]orgchart.WorkerID, error)
	// ListReports returns the direct reports of the given manager.
	ListReports(ctx context.Context, orgID string, managerID orgchart.WorkerID) ([]orgchart.WorkerID, error)
}

// WorkerRuntimeState is a sidecar key/value store keyed by
// (orgID, workerID, backend). Runtime backends (the Helix integration
// today, future local containers, etc.) write whatever per-Worker
// pointers they need — Helix uses keys like "session_id", "project_id",
// "agent_app_id", "repo_id" — without forcing the domain to grow a
// field every time.
//
// The "backend" component is a free-form string the runtime owns
// (e.g. "helix"); helix-org core never reads or writes it.
type WorkerRuntimeState interface {
	Get(ctx context.Context, orgID string, workerID orgchart.WorkerID, backend string) (map[string]string, error)
	Set(ctx context.Context, orgID string, workerID orgchart.WorkerID, backend, key, value string) error
	SetMany(ctx context.Context, orgID string, workerID orgchart.WorkerID, backend string, kv map[string]string) error
	Clear(ctx context.Context, orgID string, workerID orgchart.WorkerID, backend string) error
}

// Topics persists named event sources. Topics are created explicitly
// via the create_topic tool. Every Topic carries a Transport — the
// default (TransportLocal) keeps events local and notifies the
// in-process broadcaster; other transports compose external I/O over
// the same local store.
type Topics interface {
	Create(ctx context.Context, s streaming.Topic) error
	Get(ctx context.Context, orgID string, id streaming.TopicID) (streaming.Topic, error)
	List(ctx context.Context, orgID string) ([]streaming.Topic, error)
	// ListByTransportKind returns every topic whose transport kind
	// matches, across every org. Used by background components that
	// scan tenant boundaries (e.g. the cron topic scheduler) — NOT
	// for any per-tenant request path. Returns an empty slice when no
	// topics match; never returns ErrNotFound for "no rows".
	ListByTransportKind(ctx context.Context, kind transport.Kind) ([]streaming.Topic, error)
	// Update replaces the mutable fields on a Topic: name,
	// description, and the entire transport (kind + config). The
	// composite (id, orgID) identifies the row; ID, OrganizationID,
	// CreatedBy and CreatedAt are immutable and ignored. Returns
	// store.ErrNotFound when the row doesn't exist.
	Update(ctx context.Context, s streaming.Topic) error
	// Delete removes a topic row. Composite key (id, orgID). Callers
	// (REST handler, MCP delete_topic tool when added) are
	// responsible for any cascading subscription / role-manifest
	// cleanup — the Topics repo itself is intentionally narrow.
	Delete(ctx context.Context, orgID string, id streaming.TopicID) error
}

// Subscriptions persists (Worker, Topic) links. The triple
// (orgID, workerID, topicID) is the key — there is no synthetic ID.
// Subscriptions are WORKER-anchored: firing a Worker drops its
// subscriptions. The hiring playbook re-subscribes new hires
// explicitly, which lets two Workers in the same Role consume
// different topics (specialisation) or only the on-call subset of a
// role wake up on a given event (load patterns).
type Subscriptions interface {
	Create(ctx context.Context, sub streaming.Subscription) error
	Delete(ctx context.Context, orgID string, workerID orgchart.WorkerID, topicID streaming.TopicID) error
	Find(ctx context.Context, orgID string, workerID orgchart.WorkerID, topicID streaming.TopicID) (streaming.Subscription, error)
	ListForWorker(ctx context.Context, orgID string, workerID orgchart.WorkerID) ([]streaming.Subscription, error)
	ListForTopic(ctx context.Context, orgID string, topicID streaming.TopicID) ([]streaming.Subscription, error)
}

// Events persists entries published on a Topic.
type Events interface {
	Append(ctx context.Context, e streaming.Event) error
	ListForTopic(ctx context.Context, orgID string, topicID streaming.TopicID, limit int) ([]streaming.Event, error)
	// PageForTopic returns a window of events on one Topic, newest
	// first (same ordering as ListForTopic), skipping offset rows and
	// returning at most limit. Powers page-number pagination of the
	// REST messages endpoint. offset/limit <= 0 are treated as "no
	// skip" / "no cap" respectively.
	PageForTopic(ctx context.Context, orgID string, topicID streaming.TopicID, limit, offset int) ([]streaming.Event, error)
	// CountForTopic returns the total number of events on one Topic —
	// the total-count meta the paginated messages endpoint surfaces,
	// independent of any page window.
	CountForTopic(ctx context.Context, orgID string, topicID streaming.TopicID) (int, error)
	ListForWorker(ctx context.Context, orgID string, workerID orgchart.WorkerID, limit int) ([]streaming.Event, error)
	ListSince(ctx context.Context, orgID string, topicIDs []streaming.TopicID, since streaming.EventID, limit int) ([]streaming.Event, error)
	// ListAll returns events across every Topic in the given org,
	// newest first. Powers the unified "All topics" activity feed in
	// the UI. If limit <= 0, no limit is applied — callers are
	// expected to pass a sane cap.
	ListAll(ctx context.Context, orgID string, limit int) ([]streaming.Event, error)
}

// Processors persists Processor nodes — the transform/filter boxes
// interposed on the edge between a Topic and its subscribers. A
// Processor reads one input Topic (InputTopicID) and writes its
// auto-provisioned output Topics. ListByInputTopic is the dispatch
// hot path: on every publish the runner asks "which processors read
// this topic?".
type Processors interface {
	Create(ctx context.Context, p processor.Processor) error
	Get(ctx context.Context, orgID string, id processor.ProcessorID) (processor.Processor, error)
	List(ctx context.Context, orgID string) ([]processor.Processor, error)
	// ListByInputTopic returns every processor in the org whose
	// InputTopicID matches — the dispatcher's fan-out lookup. Returns
	// an empty slice when none match; never ErrNotFound for "no rows".
	ListByInputTopic(ctx context.Context, orgID string, in streaming.TopicID) ([]processor.Processor, error)
	// Update replaces the mutable fields: name, kind, config, outputs.
	// Composite (id, orgID) identifies the row; ID, OrganizationID,
	// CreatedBy, CreatedAt are immutable. Returns ErrNotFound when the
	// row doesn't exist.
	Update(ctx context.Context, p processor.Processor) error
	// Delete removes a processor row. Composite key (id, orgID).
	// Cascading the auto-created output Topics is the caller's job
	// (the processors application service), mirroring how Topics.Delete
	// leaves subscription cleanup to its caller.
	Delete(ctx context.Context, orgID string, id processor.ProcessorID) error
}

// Configs persists operational-config rows: transport credentials,
// model selection, runtime knobs, etc. Keyed by (orgID, key) so each
// helix tenant has its own settings.
type Configs interface {
	Set(ctx context.Context, cfg config.Config) error
	Get(ctx context.Context, orgID, key string) (config.Config, error)
	List(ctx context.Context, orgID, prefix string) ([]config.Config, error)
	Delete(ctx context.Context, orgID, key string) error
}

// Store bundles all repositories a single concrete implementation provides.
// Handlers and tools depend on the narrower interfaces above; Store is the
// wiring point.
//
// Activations is the typed port defined in api/pkg/org/activation —
// the interface lives next to the aggregate it persists, so the
// storage boundary is part of the domain package, not a parallel
// declaration here. Lifted in B5.5.
type Store struct {
	Roles              Roles
	Workers            Workers
	ReportingLines     ReportingLines
	WorkerRuntimeState WorkerRuntimeState
	Topics            Topics
	Subscriptions      Subscriptions
	Events             Events
	Configs            Configs
	Activations        activation.Repository
	Processors         Processors
}
