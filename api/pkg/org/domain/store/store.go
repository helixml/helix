// Package store defines the persistence contracts for the org-graph
// subsystem (workers, positions, roles, streams, events,
// subscriptions, activations, environments, configs). The concrete
// implementation lives in the sibling gorm sub-package — dialect-
// portable GORM, wired against helix's Postgres connection.
package store

import (
	"context"
	"errors"

	"github.com/helixml/helix/api/pkg/org/domain/activation"
	"github.com/helixml/helix/api/pkg/org/domain/config"
	"github.com/helixml/helix/api/pkg/org/domain/environment"
	"github.com/helixml/helix/api/pkg/org/domain/orgchart"
	"github.com/helixml/helix/api/pkg/org/domain/streaming"
	"github.com/helixml/helix/api/pkg/org/domain/transport"
)

// ErrNotFound signals that the requested record does not exist.
// Repos wrap this with %w so callers can errors.Is it.
var ErrNotFound = errors.New("record not found")

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

// Streams persists named event sources. Streams are created explicitly
// via the create_stream tool. Every Stream carries a Transport — the
// default (TransportLocal) keeps events local and notifies the
// in-process broadcaster; other transports compose external I/O over
// the same local store.
type Streams interface {
	Create(ctx context.Context, s streaming.Stream) error
	Get(ctx context.Context, orgID string, id streaming.StreamID) (streaming.Stream, error)
	List(ctx context.Context, orgID string) ([]streaming.Stream, error)
	// ListByTransportKind returns every stream whose transport kind
	// matches, across every org. Used by background components that
	// scan tenant boundaries (e.g. the cron stream scheduler) — NOT
	// for any per-tenant request path. Returns an empty slice when no
	// streams match; never returns ErrNotFound for "no rows".
	ListByTransportKind(ctx context.Context, kind transport.Kind) ([]streaming.Stream, error)
	// Update replaces the mutable fields on a Stream: name,
	// description, and the entire transport (kind + config). The
	// composite (id, orgID) identifies the row; ID, OrganizationID,
	// CreatedBy and CreatedAt are immutable and ignored. Returns
	// store.ErrNotFound when the row doesn't exist.
	Update(ctx context.Context, s streaming.Stream) error
	// Delete removes a stream row. Composite key (id, orgID). Callers
	// (REST handler, MCP delete_stream tool when added) are
	// responsible for any cascading subscription / role-manifest
	// cleanup — the Streams repo itself is intentionally narrow.
	Delete(ctx context.Context, orgID string, id streaming.StreamID) error
}

// Subscriptions persists (Worker, Stream) links. The triple
// (orgID, workerID, streamID) is the key — there is no synthetic ID.
// Subscriptions are WORKER-anchored: firing a Worker drops its
// subscriptions. The hiring playbook re-subscribes new hires
// explicitly, which lets two Workers in the same Role consume
// different streams (specialisation) or only the on-call subset of a
// role wake up on a given event (load patterns).
type Subscriptions interface {
	Create(ctx context.Context, sub streaming.Subscription) error
	Delete(ctx context.Context, orgID string, workerID orgchart.WorkerID, streamID streaming.StreamID) error
	Find(ctx context.Context, orgID string, workerID orgchart.WorkerID, streamID streaming.StreamID) (streaming.Subscription, error)
	ListForWorker(ctx context.Context, orgID string, workerID orgchart.WorkerID) ([]streaming.Subscription, error)
	ListForStream(ctx context.Context, orgID string, streamID streaming.StreamID) ([]streaming.Subscription, error)
}

// Events persists entries published on a Stream.
type Events interface {
	Append(ctx context.Context, e streaming.Event) error
	ListForStream(ctx context.Context, orgID string, streamID streaming.StreamID, limit int) ([]streaming.Event, error)
	ListForWorker(ctx context.Context, orgID string, workerID orgchart.WorkerID, limit int) ([]streaming.Event, error)
	ListSince(ctx context.Context, orgID string, streamIDs []streaming.StreamID, since streaming.EventID, limit int) ([]streaming.Event, error)
	// ListAll returns events across every Stream in the given org,
	// newest first. Powers the unified "All streams" activity feed in
	// the UI. If limit <= 0, no limit is applied — callers are
	// expected to pass a sane cap.
	ListAll(ctx context.Context, orgID string, limit int) ([]streaming.Event, error)
}

// Environments persists the per-Worker directory handle. The manager
// populates the directory before hire; this table just tracks that a
// directory exists and which Worker owns it.
type Environments interface {
	Create(ctx context.Context, env environment.Environment) error
	Get(ctx context.Context, orgID string, workerID orgchart.WorkerID) (environment.Environment, error)
	Delete(ctx context.Context, orgID string, workerID orgchart.WorkerID) error
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
	Streams            Streams
	Subscriptions      Subscriptions
	Events             Events
	Environments       Environments
	Configs            Configs
	Activations        activation.Repository
}
