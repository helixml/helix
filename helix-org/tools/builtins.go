package tools

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/helixml/helix-org/broadcast"
	"github.com/helixml/helix-org/domain"
	"github.com/helixml/helix-org/store"
)

// Clock returns the current time. Tests override it.
type Clock func() time.Time

// IDGen generates new unique string IDs. Tests override it.
type IDGen func() string

// Spawner runs an AI Worker's agent process for a single activation
// and BLOCKS until the process exits. The Trigger tells the Spawner
// (and through it, the agent) why this activation is happening — first
// hire, or a new event on a subscribed Stream.
//
// Spawners are typically called from inside a Dispatcher that
// serialises calls per-Worker; callers must not invoke a Spawner for
// the same Worker concurrently.
//
// The zero value — nil — means "no process will be spawned", which is
// correct for tests and for HumanWorker activations.
type Spawner func(ctx context.Context, workerID domain.WorkerID, envPath string, trigger Trigger) error

// EventDispatcher fans a freshly-published Event out to every
// subscribed AI Worker as a separate Spawner activation. Tools call it
// after persisting an Event. The interface keeps tools.Deps free of a
// dependency on the dispatch package (avoiding an import cycle: the
// dispatcher itself imports tools).
type EventDispatcher interface {
	Dispatch(ctx context.Context, event domain.Event)
	DispatchHire(ctx context.Context, workerID domain.WorkerID, envPath string)
}

// Deps bundles the stores, clocks, and configuration tools need.
//
// EnvsDir is the directory under which each Worker's Environment lives:
// HireWorker creates <EnvsDir>/<workerId>/ at hire time and writes the
// role.md / identity.md / agent.md trio into it.
//
// Broadcaster is optional: if set, event-emitting tools (publish) will
// call its Notify method so any long-poll readers blocked on those
// streams wake up immediately.
//
// Dispatcher is optional: if set, event-emitting tools also call its
// Dispatch method so subscribed AI Workers get re-activated. Tests
// that don't exercise the runtime can leave it nil. The dispatcher
// itself owns the Spawner.
type Deps struct {
	Store       *store.Store
	Now         Clock
	NewID       IDGen
	EnvsDir     string
	Broadcaster *broadcast.Broadcaster
	Dispatcher  EventDispatcher
}

// DefaultDeps wires production defaults: real UUIDs and wall-clock time.
// EnvsDir, Broadcaster, and Dispatcher are left zero — production
// callers wire them in cmd/helix-org/serve.go.
func DefaultDeps(s *store.Store) Deps {
	return Deps{
		Store: s,
		Now:   func() time.Time { return time.Now().UTC() },
		NewID: uuid.NewString,
	}
}

// RegisterBuiltins registers every built-in tool on the registry —
// mutations on the org graph plus the matching read tools. Test tools
// (like Ping) are not included.
func RegisterBuiltins(reg *Registry, deps Deps) error {
	builtins := []domain.Tool{
		// Mutations.
		&CreateRole{deps: deps},
		&UpdateRole{deps: deps},
		&CreatePosition{deps: deps},
		&HireWorker{deps: deps},
		&GrantTool{deps: deps},
		&RevokeTool{deps: deps},
		&CreateStream{deps: deps},
		&StreamMembers{deps: deps},
		&Subscribe{deps: deps},
		&Unsubscribe{deps: deps},
		&InviteWorkers{deps: deps},
		&Publish{deps: deps},
		&DM{deps: deps},
		// Reads. Each is a thin wrapper around a store call; together
		// they replace the jsonapi GET handlers the server used to expose.
		&ListRoles{deps: deps},
		&GetRole{deps: deps},
		&ListPositions{deps: deps},
		&GetPosition{deps: deps},
		&ListPositionChildren{deps: deps},
		&ListWorkers{deps: deps},
		&GetWorker{deps: deps},
		&ListWorkerGrants{deps: deps},
		&GetWorkerEnvironment{deps: deps},
		&ListStreams{deps: deps},
		&GetStream{deps: deps},
		&ListStreamEvents{deps: deps},
		&GetGrant{deps: deps},
		&ReadEvents{deps: deps},
	}
	for _, tool := range builtins {
		if err := reg.Register(tool); err != nil {
			return fmt.Errorf("register %q: %w", tool.Name(), err)
		}
	}
	return nil
}
