package tools

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/helixml/helix/api/pkg/org/application/streamhub"
	"github.com/helixml/helix/api/pkg/org/application/streams"
	"github.com/helixml/helix/api/pkg/org/application/topology"
	"github.com/helixml/helix/api/pkg/org/domain/activation"
	"github.com/helixml/helix/api/pkg/org/domain/credential"
	"github.com/helixml/helix/api/pkg/org/domain/orgchart"
	"github.com/helixml/helix/api/pkg/org/domain/store"
	"github.com/helixml/helix/api/pkg/org/domain/streaming"
	"github.com/helixml/helix/api/pkg/org/domain/tool"
	"github.com/helixml/helix/api/pkg/org/infrastructure/runtime"
)

// Clock returns the current time. Tests override it.
type Clock func() time.Time

// IDGen generates new unique string IDs. Tests override it.
type IDGen func() string

// EventDispatcher fans a freshly-published Event out to every
// subscribed AI Worker as a separate Spawner activation. Tools call it
// after persisting an Event. The interface keeps tools.Deps free of a
// dependency on the dispatch package (avoiding an import cycle: the
// dispatcher itself imports tools).
type EventDispatcher interface {
	Dispatch(ctx context.Context, event streaming.Event)
	// DispatchHire fires a TriggerHire activation. activationID is the
	// pre-allocated audit-row ID hire_worker created before calling
	// DispatchHire — it travels through the trigger so the Spawner
	// reuses the existing row instead of writing a sibling. Empty
	// activationID is allowed for callers that don't pre-allocate
	// (legacy code paths, tests that don't wire activation.Repository).
	DispatchHire(ctx context.Context, orgID string, workerID orgchart.WorkerID, envPath string, activationID activation.ID)
}

// Deps bundles the stores, clocks, and configuration tools need.
//
// EnvsDir is the directory under which each Worker's Environment lives:
// HireWorker creates <EnvsDir>/<workerId>/ at hire time and writes the
// role.md / identity.md / agent.md trio into it.
//
// Hub is optional: if set, event-emitting tools (publish) will
// call its Notify method so any long-poll readers blocked on those
// streams wake up immediately.
//
// Dispatcher is optional: if set, event-emitting tools also call its
// Dispatch method so subscribed AI Workers get re-activated. Tests
// that don't exercise the runtime can leave it nil. The dispatcher
// itself owns the Spawner.
//
// Workspace is required (use runtime.NoopWorkspaceSync{} for tests).
// update_role and update_identity call MirrorFile on it after
// persisting to the DB so the per-runtime view of role/identity stays
// in sync with the canonical domain copy.
type Deps struct {
	Store      *store.Store
	Now        Clock
	NewID      IDGen
	EnvsDir    string
	Hub        *streamhub.Hub
	Dispatcher EventDispatcher
	Workspace  runtime.WorkspaceSync
	// HireHook runs runtime-side bookkeeping after a new Worker is
	// created (hire_worker invokes it once the Worker row is in the
	// store). Pick the right impl at wiring time — the helix runtime
	// uses helix.Hire to persist the hiring user; claude / dev
	// runtimes use runtime.NoopHireHook.
	HireHook runtime.HireHook

	// ProjectConfig is the read/write port for a Worker's helix
	// project configuration (startup script, skills, etc). Backs
	// the get_worker_project + configure_worker_project MCP tools.
	// Wire the helix runtime impl in production; tests + claude/dev
	// runtimes can leave it nil — the default is NoopProjectConfig
	// which returns ErrProjectConfigUnsupported. The MCP tool wraps
	// that into a friendly error message.
	ProjectConfig runtime.ProjectConfig

	// Topology reconciles the activation/team Streams implied by the
	// reporting graph. hire_worker calls it after writing the new
	// Worker + reporting line so the hire's activation Stream and the
	// manager's team Stream materialise from one declarative pass. A
	// nil Reconciler is a no-op (tests / runtimes without topology),
	// but DefaultDeps wires one so the standard path is always covered.
	Topology *topology.Reconciler

	// CredentialProviders is the registry the mint_credential MCP tool
	// dispatches on. Keys are provider names (e.g. "github", "slack")
	// matching the agent-supplied `provider` arg; values are minted
	// on-demand by the per-transport Provider implementation. Wire
	// providers in helix-org's bootstrap (api/pkg/server/helix_org.go)
	// — adding a new provider is a new file in its transport package
	// plus one map entry there, with no edits to mint_credential.
	//
	// nil/empty map is allowed: mint_credential will register and report
	// the empty list to callers; the same shape DefaultDeps installs
	// for tests that do not exercise the credential path.
	CredentialProviders map[string]credential.Provider
}

// streamsService builds the stream-mutation application service from
// the tool deps. The MCP stream tools are thin adapters over it, so the
// create/update/delete logic lives in exactly one place (shared with
// the REST handlers).
func (d Deps) streamsService() *streams.Streams {
	return streams.New(streams.Deps{
		Streams: d.Store.Streams,
		Now:     d.Now,
		NewID:   d.NewID,
	})
}

// DefaultDeps wires production defaults: real UUIDs and wall-clock time,
// and a no-op WorkspaceSync that callers replace with the runtime-
// specific implementation. EnvsDir, Hub, and Dispatcher are
// left zero — production callers wire them in cmd/helix-org/serve.go.
func DefaultDeps(s *store.Store) Deps {
	d := Deps{
		Store:               s,
		Now:                 func() time.Time { return time.Now().UTC() },
		NewID:               uuid.NewString,
		Workspace:           runtime.NoopWorkspaceSync{},
		HireHook:            runtime.NoopHireHook{},
		ProjectConfig:       runtime.NoopProjectConfig{},
		CredentialProviders: map[string]credential.Provider{},
	}
	d.Topology = &topology.Reconciler{Store: s, Now: d.Now}
	return d
}

// RegisterBuiltins registers every built-in tool on the registry —
// mutations on the org graph plus the matching read tools. Test tools
// (like Ping) are not included.
func RegisterBuiltins(reg *Registry, deps Deps) error {
	if deps.Workspace == nil {
		return fmt.Errorf("tools.RegisterBuiltins: deps.Workspace is required (use runtime.NoopWorkspaceSync{} for tests)")
	}
	builtins := []tool.Tool{
		// Mutations.
		&CreateRole{deps: deps},
		&UpdateRole{deps: deps},
		&UpdateIdentity{deps: deps},
		&HireWorker{deps: deps},
		&CreateStream{deps: deps},
		&MintCredential{deps: deps, providers: deps.CredentialProviders},
		&StreamMembers{deps: deps},
		&Subscribe{deps: deps},
		&Unsubscribe{deps: deps},
		&InviteWorkers{deps: deps},
		&Publish{deps: deps},
		&DM{deps: deps},
		&ConfigureWorkerProject{deps: deps},
		// Reads. Each is a thin wrapper around a store call; together
		// they replace the jsonapi GET handlers the server used to expose.
		&ListRoles{deps: deps},
		&GetRole{deps: deps},
		&ListWorkers{deps: deps},
		&GetWorker{deps: deps},
		&Managers{deps: deps},
		&Reports{deps: deps},
		&GetWorkerEnvironment{deps: deps},
		&GetWorkerProject{deps: deps},
		&ListStreams{deps: deps},
		&GetStream{deps: deps},
		&ListStreamEvents{deps: deps},
		&ReadEvents{deps: deps},
		&WorkerLog{deps: deps},
	}
	for _, tool := range builtins {
		if err := reg.Register(tool); err != nil {
			return fmt.Errorf("register %q: %w", tool.Name(), err)
		}
	}
	// Fail fast if BaseReadTools references a name that isn't registered
	// — a typo in defaults.go would otherwise produce silently-broken
	// Roles whose reconciled tool list is missing one of the baseline
	// entries.
	for _, name := range BaseReadTools {
		if _, err := reg.Get(name); err != nil {
			return fmt.Errorf("BaseReadTools references unregistered tool %q: %w", name, err)
		}
	}
	return nil
}
