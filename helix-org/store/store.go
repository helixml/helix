// Package store defines the persistence contracts used by the server and
// tools. Concrete implementations live in sub-packages (e.g. sqlite).
package store

import (
	"context"
	"errors"

	"github.com/helixml/helix-org/domain"
)

// ErrNotFound signals that the requested record does not exist.
// Repos wrap this with %w so callers can errors.Is it.
var ErrNotFound = errors.New("record not found")

// Roles persists job descriptions.
type Roles interface {
	Create(ctx context.Context, role domain.Role) error
	Get(ctx context.Context, id domain.RoleID) (domain.Role, error)
	List(ctx context.Context) ([]domain.Role, error)
	Update(ctx context.Context, role domain.Role) error
}

// Positions persists slots in the org chart.
type Positions interface {
	Create(ctx context.Context, pos domain.Position) error
	Get(ctx context.Context, id domain.PositionID) (domain.Position, error)
	List(ctx context.Context) ([]domain.Position, error)
	ListChildren(ctx context.Context, parent domain.PositionID) ([]domain.Position, error)
}

// Workers persists humans and AIs.
type Workers interface {
	Create(ctx context.Context, worker domain.Worker) error
	Get(ctx context.Context, id domain.WorkerID) (domain.Worker, error)
	List(ctx context.Context) ([]domain.Worker, error)
}

// Grants persists tool grants.
type Grants interface {
	Create(ctx context.Context, g domain.ToolGrant) error
	Get(ctx context.Context, id domain.GrantID) (domain.ToolGrant, error)
	ListByWorker(ctx context.Context, workerID domain.WorkerID) ([]domain.ToolGrant, error)
	FindForWorkerAndTool(ctx context.Context, workerID domain.WorkerID, toolName domain.ToolName) (domain.ToolGrant, error)
	Delete(ctx context.Context, id domain.GrantID) error
}

// Streams persists named event sources. Streams are created explicitly
// via the create_stream tool. Every Stream carries a Transport — the
// default (TransportLocal) keeps events in SQLite and notifies the
// in-process broadcaster; other transports compose external I/O over
// the same local store.
type Streams interface {
	Create(ctx context.Context, s domain.Stream) error
	Get(ctx context.Context, id domain.StreamID) (domain.Stream, error)
	List(ctx context.Context) ([]domain.Stream, error)
}

// Subscriptions persists (Worker, Stream) links. The pair is the key —
// there is no synthetic ID.
type Subscriptions interface {
	Create(ctx context.Context, sub domain.Subscription) error
	Delete(ctx context.Context, workerID domain.WorkerID, streamID domain.StreamID) error
	Find(ctx context.Context, workerID domain.WorkerID, streamID domain.StreamID) (domain.Subscription, error)
	ListForWorker(ctx context.Context, workerID domain.WorkerID) ([]domain.Subscription, error)
	ListForStream(ctx context.Context, streamID domain.StreamID) ([]domain.Subscription, error)
}

// Events persists entries published on a Stream.
type Events interface {
	Append(ctx context.Context, e domain.Event) error
	ListForStream(ctx context.Context, streamID domain.StreamID, limit int) ([]domain.Event, error)
	// ListForWorker returns events on the Streams a Worker subscribes to,
	// newest first. If limit <= 0, no limit is applied.
	ListForWorker(ctx context.Context, workerID domain.WorkerID, limit int) ([]domain.Event, error)
	// ListSince returns events on the named Streams strictly newer than the
	// `since` event, oldest first. If streamIDs is empty, returns nothing
	// (caller's glob matched no streams). If `since` is empty, returns the
	// most recent `limit` events on the named streams in oldest-first order.
	// If `since` does not exist, returns the same as if it were empty. If
	// limit <= 0, no limit is applied.
	ListSince(ctx context.Context, streamIDs []domain.StreamID, since domain.EventID, limit int) ([]domain.Event, error)
}

// Environments persists the per-Worker directory handle. The manager
// populates the directory before hire; this table just tracks that a
// directory exists and which Worker owns it.
type Environments interface {
	Create(ctx context.Context, env domain.Environment) error
	Get(ctx context.Context, workerID domain.WorkerID) (domain.Environment, error)
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
	Roles         Roles
	Positions     Positions
	Workers       Workers
	Grants        Grants
	Streams       Streams
	Subscriptions Subscriptions
	Events        Events
	Environments  Environments
	Configs       Configs
}
