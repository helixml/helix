// Package store defines the persistence contracts used by the server and
// tools. Concrete implementations live in sub-packages (e.g. sqlite).
package store

import (
	"context"
	"errors"

	"github.com/helixml/helix/api/pkg/org/event"
	"github.com/helixml/helix/api/pkg/org/grant"
	"github.com/helixml/helix/api/pkg/org/position"
	"github.com/helixml/helix/api/pkg/org/role"
	"github.com/helixml/helix/api/pkg/org/stream"
	"github.com/helixml/helix/api/pkg/org/tool"
	"github.com/helixml/helix/api/pkg/org/worker"
	"github.com/helixml/helix/helix-org/domain"
)

// ErrNotFound signals that the requested record does not exist.
// Repos wrap this with %w so callers can errors.Is it.
var ErrNotFound = errors.New("record not found")

// Roles persists job descriptions.
type Roles interface {
	Create(ctx context.Context, role role.Role) error
	Get(ctx context.Context, id role.ID) (role.Role, error)
	List(ctx context.Context) ([]role.Role, error)
	Update(ctx context.Context, role role.Role) error
}

// Positions persists slots in the org chart.
type Positions interface {
	Create(ctx context.Context, pos domain.Position) error
	Get(ctx context.Context, id position.ID) (domain.Position, error)
	List(ctx context.Context) ([]domain.Position, error)
	ListChildren(ctx context.Context, parent position.ID) ([]domain.Position, error)
}

// Workers persists humans and AIs. Update mutates fields the system
// allows changing in place — currently just IdentityContent (set at
// hire by the caller, replaced wholesale by update_identity). Identity
// is the per-Worker description; the system holds it in the domain
// rather than on disk so it survives any change in env layout.
type Workers interface {
	Create(ctx context.Context, worker domain.Worker) error
	Get(ctx context.Context, id worker.ID) (domain.Worker, error)
	List(ctx context.Context) ([]domain.Worker, error)
	Update(ctx context.Context, worker domain.Worker) error
}

// WorkerRuntimeState is a sidecar key/value store keyed by
// (workerID, backend). Runtime backends (the Helix integration today,
// future local containers, etc.) write whatever per-Worker pointers
// they need — Helix uses keys like "session_id", "project_id",
// "agent_app_id", "repo_id" — without forcing the domain to grow a
// field every time.
//
// The "backend" component is a free-form string the runtime owns
// (e.g. "helix"); helix-org core never reads or writes it.
//
// Get returns an empty map if the (workerID, backend) pair has no
// entries. Set upserts a single key, leaving other keys for that
// (workerID, backend) untouched. SetMany upserts a batch in the
// same way. Clear removes every entry for the pair (used when a
// Worker is fired and the runtime tears down its per-Worker state).
type WorkerRuntimeState interface {
	Get(ctx context.Context, workerID worker.ID, backend string) (map[string]string, error)
	Set(ctx context.Context, workerID worker.ID, backend, key, value string) error
	SetMany(ctx context.Context, workerID worker.ID, backend string, kv map[string]string) error
	Clear(ctx context.Context, workerID worker.ID, backend string) error
}

// Grants persists tool grants.
type Grants interface {
	Create(ctx context.Context, g domain.ToolGrant) error
	Get(ctx context.Context, id grant.ID) (domain.ToolGrant, error)
	ListByWorker(ctx context.Context, workerID worker.ID) ([]domain.ToolGrant, error)
	FindForWorkerAndTool(ctx context.Context, workerID worker.ID, toolName tool.Name) (domain.ToolGrant, error)
	Delete(ctx context.Context, id grant.ID) error
}

// Streams persists named event sources. Streams are created explicitly
// via the create_stream tool. Every Stream carries a Transport — the
// default (TransportLocal) keeps events in SQLite and notifies the
// in-process broadcaster; other transports compose external I/O over
// the same local store.
type Streams interface {
	Create(ctx context.Context, s domain.Stream) error
	Get(ctx context.Context, id stream.ID) (domain.Stream, error)
	List(ctx context.Context) ([]domain.Stream, error)
}

// Subscriptions persists (Worker, Stream) links. The pair is the key —
// there is no synthetic ID.
type Subscriptions interface {
	Create(ctx context.Context, sub domain.Subscription) error
	Delete(ctx context.Context, workerID worker.ID, streamID stream.ID) error
	Find(ctx context.Context, workerID worker.ID, streamID stream.ID) (domain.Subscription, error)
	ListForWorker(ctx context.Context, workerID worker.ID) ([]domain.Subscription, error)
	ListForStream(ctx context.Context, streamID stream.ID) ([]domain.Subscription, error)
}

// Events persists entries published on a Stream.
type Events interface {
	Append(ctx context.Context, e domain.Event) error
	ListForStream(ctx context.Context, streamID stream.ID, limit int) ([]domain.Event, error)
	// ListForWorker returns events on the Streams a Worker subscribes to,
	// newest first. If limit <= 0, no limit is applied.
	ListForWorker(ctx context.Context, workerID worker.ID, limit int) ([]domain.Event, error)
	// ListSince returns events on the named Streams strictly newer than the
	// `since` event, oldest first. If streamIDs is empty, returns nothing
	// (caller's glob matched no streams). If `since` is empty, returns the
	// most recent `limit` events on the named streams in oldest-first order.
	// If `since` does not exist, returns the same as if it were empty. If
	// limit <= 0, no limit is applied.
	ListSince(ctx context.Context, streamIDs []stream.ID, since event.ID, limit int) ([]domain.Event, error)
	// ListAll returns events across every Stream, newest first. Powers
	// the unified "All streams" activity feed in the UI. If limit <= 0,
	// no limit is applied — callers are expected to pass a sane cap.
	ListAll(ctx context.Context, limit int) ([]domain.Event, error)
}

// Environments persists the per-Worker directory handle. The manager
// populates the directory before hire; this table just tracks that a
// directory exists and which Worker owns it.
type Environments interface {
	Create(ctx context.Context, env domain.Environment) error
	Get(ctx context.Context, workerID worker.ID) (domain.Environment, error)
}

// Configs persists operational-config rows: transport credentials,
// claude binary path, model selection, etc. Keys are flat dot-
// namespaced strings; values are JSON-encoded. See design/config.md
// for the org-graph-vs-ops split. Configs are written exclusively
// through the helix-org config CLI — never via MCP.
type Configs interface {
	Set(ctx context.Context, cfg domain.Config) error
	Get(ctx context.Context, key string) (domain.Config, error)
	List(ctx context.Context, prefix string) ([]domain.Config, error)
	Delete(ctx context.Context, key string) error
}

// Store bundles all repositories a single concrete implementation provides.
// Handlers and tools depend on the narrower interfaces above; Store is the
// wiring point.
type Store struct {
	Roles              Roles
	Positions          Positions
	Workers            Workers
	WorkerRuntimeState WorkerRuntimeState
	Grants             Grants
	Streams            Streams
	Subscriptions      Subscriptions
	Events             Events
	Environments       Environments
	Configs            Configs
}
