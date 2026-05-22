package chat

import (
	"context"
	"fmt"
	"time"

	"log/slog"

	"github.com/helixml/helix/api/pkg/org/activation"
	"github.com/helixml/helix/api/pkg/org/prompts"
	"github.com/helixml/helix/api/pkg/org/worker"
)

// ActivationPublisher writes activation events to
// s-activations-<workerID> for every chat turn — same shape the
// spawner uses for AI Worker activations. Optional: when nil the
// bridge runs without publishing (useful for tests).
type ActivationPublisher func(ctx context.Context, workerID worker.ID, body string)

// HelixBridge is a chat surface bound to a single target Worker.
// The Worker may be the owner ("w-owner" in today's alpha) or any
// other AI Worker the operator wants a direct chat with — the
// bridge does not distinguish.
//
// The HTTP handlers and SSE broadcast plumbing that used to live on
// this type were deleted in Phase C of the UI migration (the htmx SSR
// that consumed them is gone). The struct is preserved so the
// embedding host can keep wiring the bridge — its constructor still
// performs the same configuration validation, exposes the same
// project-applier integration, and accepts the same activation-
// publishing / session-persistence callbacks — ready for the next
// chat surface to attach. Reviving chat means wiring new
// transport-layer methods that read this state; the wiring that
// constructs the bridge in api/pkg/server/helix_org_chat.go
// continues to work unchanged.
type HelixBridge struct {
	client      ChatBridgeClient
	ensure      ProjectEnsurer                        // resolves the target Worker's per-Worker project; nil in app-only mode
	appID       string                                // app-only mode: when set, skip project lifecycle and chat against this existing Helix app
	appIDFunc   func(context.Context) (string, error) // app-only mode: dynamic lookup
	workerID    worker.ID                             // the Worker this bridge is chatting with
	sessionRole string
	provider    string
	model       string
	cwd         string
	logger      *slog.Logger
	prompts     *prompts.Registry

	// loadSessionID / saveSessionID persist the chat session pointer
	// so it survives process restarts. Optional — see
	// HelixConfig.LoadSessionID / SaveSessionID.
	loadSessionID func(ctx context.Context, workerID worker.ID) (string, error)
	saveSessionID func(ctx context.Context, workerID worker.ID, sessionID string) error

	// publishActivation writes events to s-activations-<workerID> so
	// the target Worker's chat turns are observable alongside every AI
	// Worker's. nil disables publishing (tests, app-only mode).
	publishActivation ActivationPublisher

	// activations + newID + now persist one Activation row per send so
	// the chat audit surface matches the Spawner's. All three nil = no
	// row written; rows fire only when every field is wired.
	activations activation.Repository
	newID       func() string
	now         func() time.Time
}

// ProjectEnsurer resolves a Worker's Helix project IDs. The chat
// bridge calls Ensure(ctx, workerID) per send so the target Worker's
// project (and its auto-provisioned Agent App with MCP wiring) is
// the chat target. The interface keeps the chat package free of a
// hard import on tools/.
type ProjectEnsurer interface {
	Ensure(ctx context.Context, workerID worker.ID) (projectID, agentAppID, repoID string, err error)
}

// HelixConfig wires a HelixBridge. The bridge holds no global
// project ID — each chat session opens against the target Worker's
// per-Worker project, looked up via Ensure on every send.
type HelixConfig struct {
	Client ChatBridgeClient
	Ensure ProjectEnsurer
	// WorkerID is the chat target — the Worker this bridge talks to.
	WorkerID    worker.ID
	SessionRole string // chat.session_role, e.g. "exploratory"
	Provider    string // chat.provider (ignored in app-only mode)
	Model       string // chat.model (ignored in app-only mode)
	CWD         string // server cwd, only used as a stable label
	Logger      *slog.Logger

	// AppID enables "app-only" mode: instead of helix-org provisioning
	// its own per-Worker project, the bridge opens chat sessions
	// against this existing Helix app. Mutually exclusive with Ensure.
	AppID string

	// AppIDFunc is the dynamic variant of AppID: re-read per send.
	// Mutually exclusive with AppID and Ensure.
	AppIDFunc func(context.Context) (string, error)

	// LoadSessionID / SaveSessionID are optional persistence hooks for
	// the chat session pointer.
	LoadSessionID func(ctx context.Context, workerID worker.ID) (string, error)
	SaveSessionID func(ctx context.Context, workerID worker.ID, sessionID string) error

	// PublishActivation writes events to s-activations-<workerID>.
	// Optional — leave nil to suppress.
	PublishActivation ActivationPublisher

	// Activations + NewID + Now persist an Activation row per chat
	// turn. All three are optional and travel as a group.
	Activations activation.Repository
	NewID       func() string
	Now         func() time.Time
}

// NewHelix returns a HelixBridge bound to the supplied Helix client.
// Either Ensure (project-applier mode) or AppID (app-only mode) must
// be set; they're mutually exclusive.
func NewHelix(cfg HelixConfig) (*HelixBridge, error) {
	if cfg.Client == nil {
		return nil, fmt.Errorf("chat helix bridge: Client is required")
	}
	configured := 0
	if cfg.Ensure != nil {
		configured++
	}
	if cfg.AppID != "" {
		configured++
	}
	if cfg.AppIDFunc != nil {
		configured++
	}
	if configured == 0 {
		return nil, fmt.Errorf("chat helix bridge: one of Ensure, AppID, AppIDFunc must be set")
	}
	if configured > 1 {
		return nil, fmt.Errorf("chat helix bridge: Ensure, AppID, AppIDFunc are mutually exclusive")
	}
	if cfg.WorkerID == "" {
		return nil, fmt.Errorf("chat helix bridge: WorkerID is required")
	}
	if cfg.SessionRole == "" {
		return nil, fmt.Errorf("chat helix bridge: SessionRole is required (set chat.session_role)")
	}
	logger := cfg.Logger
	if logger == nil {
		logger = slog.Default()
	}
	return &HelixBridge{
		client:            cfg.Client,
		ensure:            cfg.Ensure,
		appID:             cfg.AppID,
		appIDFunc:         cfg.AppIDFunc,
		workerID:          cfg.WorkerID,
		sessionRole:       cfg.SessionRole,
		provider:          cfg.Provider,
		model:             cfg.Model,
		cwd:               cfg.CWD,
		logger:            logger,
		loadSessionID:     cfg.LoadSessionID,
		saveSessionID:     cfg.SaveSessionID,
		publishActivation: cfg.PublishActivation,
		activations:       cfg.Activations,
		newID:             cfg.NewID,
		now:               cfg.Now,
	}, nil
}

// WithPrompts attaches the slash-command registry the future chat
// surface will use to expand `/<name>` inputs server-side before
// posting to Helix. Returns the bridge so the call composes.
func (b *HelixBridge) WithPrompts(reg *prompts.Registry) *HelixBridge {
	b.prompts = reg
	return b
}
