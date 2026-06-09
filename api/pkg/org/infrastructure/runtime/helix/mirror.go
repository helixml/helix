package helix

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/helixml/helix/api/pkg/org/application/agent"
	"github.com/helixml/helix/api/pkg/org/application/streamhub"
	"github.com/helixml/helix/api/pkg/org/domain/orgchart"
	"github.com/helixml/helix/api/pkg/org/domain/store"
	"github.com/helixml/helix/api/pkg/pubsub"
	"github.com/helixml/helix/api/pkg/types"
)

// defaultMirrorPoll is how often the mirror re-resolves a worker's
// current session and re-points if it changed.
const defaultMirrorPoll = 5 * time.Second

// MirrorConfig wires the session-layer transcript Mirror. It is the
// subset of SpawnerConfig the Mirror needs, plus a resolver for the
// worker's current session.
type MirrorConfig struct {
	PubSub      pubsub.PubSub
	Snapshotter SessionPreamble
	// Client resolves the session owner (SessionOwner) so the mirror
	// subscribes to the correct GetSessionQueue(owner, session) topic.
	Client SpawnerClient
	// ExploratorySession returns a worker project's current
	// (most-recent) exploratory session ID — the session the inline
	// chat and the live UI follow. The mirror polls it to track the
	// worker as its session churns (a stale resume opens a fresh
	// session; the persisted pointer can lag). Wired to
	// store.GetProjectExploratorySession. "" means "no session yet".
	ExploratorySession func(ctx context.Context, projectID string) (string, error)
	Store              *store.Store
	Hub                *streamhub.Hub
	NewID              func() string
	Now                func() time.Time
	Logger             *slog.Logger
	// PollInterval seams the re-point cadence for tests. <=0 uses
	// defaultMirrorPoll.
	PollInterval time.Duration
}

// Mirror keeps one transcript subscription per *tracked worker*,
// pointed at that worker's current session, and republishes every
// settled entry (plus the user's prompt) onto s-activations-<worker>.
// It is the single writer of worker transcript segments: every turn on
// a worker's session — spawner activation, human inline chat, anything
// that posts to /sessions/chat — flows through the session topic, so
// one subscriber captures them all.
//
// A worker's *session* is not stable: a stale resume opens a fresh one,
// and the inline chat can land on a newer session than the spawner last
// persisted. So the mirror doesn't pin a session ID — it tracks the
// worker and polls its current exploratory session, re-pointing the
// subscription whenever it changes. The worker (and its project) is the
// stable identity; the session is not.
type Mirror struct {
	base context.Context
	cfg  MirrorConfig

	mu      sync.Mutex
	tracked map[orgchart.WorkerID]context.CancelFunc
}

// NewMirror constructs a Mirror. base is the long-lived context that
// bounds every tracker — typically the server's lifetime context. A nil
// PubSub disables the mirror (Ensure is a no-op).
func NewMirror(base context.Context, cfg MirrorConfig) *Mirror {
	if base == nil {
		base = context.Background()
	}
	return &Mirror{
		base:    base,
		cfg:     cfg,
		tracked: map[orgchart.WorkerID]context.CancelFunc{},
	}
}

func (m *Mirror) pollInterval() time.Duration {
	if m.cfg.PollInterval > 0 {
		return m.cfg.PollInterval
	}
	return defaultMirrorPoll
}

// Ensure starts tracking a worker, idempotently. The tracker resolves
// the worker's current session, subscribes, and thereafter re-points on
// every poll if the session changed. Already-tracked workers are a
// no-op. The session is resolved by the tracker, not passed in, because
// the caller's notion of "the session" is exactly the thing that goes
// stale.
func (m *Mirror) Ensure(orgID string, workerID orgchart.WorkerID) {
	if m == nil || m.cfg.PubSub == nil {
		return
	}
	m.mu.Lock()
	if _, ok := m.tracked[workerID]; ok {
		m.mu.Unlock()
		return
	}
	ctx, cancel := context.WithCancel(m.base)
	m.tracked[workerID] = cancel
	m.mu.Unlock()
	go m.track(ctx, orgID, workerID)
}

// EnsureAll tracks every worker in an org. Called from the per-org
// bootstrap so pre-existing workers (and inline-chat-only ones) are
// mirrored without needing an activation first.
func (m *Mirror) EnsureAll(ctx context.Context, orgID string) {
	if m == nil || m.cfg.PubSub == nil || m.cfg.Store == nil {
		return
	}
	workers, err := m.cfg.Store.Workers.List(ctx, orgID)
	if err != nil {
		if m.cfg.Logger != nil {
			m.cfg.Logger.Warn("helix mirror: list workers for sweep", "org", orgID, "err", err)
		}
		return
	}
	for _, w := range workers {
		m.Ensure(orgID, w.ID())
	}
}

// Stop stops tracking a worker (on fire) — cancels its tracker and the
// current session subscription.
func (m *Mirror) Stop(workerID orgchart.WorkerID) {
	if m == nil {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if cancel, ok := m.tracked[workerID]; ok {
		cancel()
		delete(m.tracked, workerID)
	}
}

// track owns one worker's mirror for the life of ctx: it resolves the
// worker's current session and (re)subscribes whenever it changes,
// polling on an interval. Each session gets a fresh bridge (fresh
// EntryStream dedup + prompt-dedup state).
func (m *Mirror) track(ctx context.Context, orgID string, workerID orgchart.WorkerID) {
	var (
		curSession string
		curCancel  = func() {}
	)
	defer func() { curCancel() }()

	repoint := func() {
		desired := m.resolveSession(ctx, orgID, workerID)
		if desired == "" || desired == curSession {
			return
		}
		// Session changed — tear down the old subscription (its pump
		// flushes any pending entries on ctx.Done) and attach to the
		// new one.
		curCancel()
		subCtx, cancel := context.WithCancel(ctx)
		curCancel = cancel
		curSession = desired
		m.subscribe(subCtx, orgID, workerID, desired)
		if m.cfg.Logger != nil {
			m.cfg.Logger.Info("helix mirror: pointed at session", "worker", workerID, "session", desired)
		}
	}

	repoint() // resolve + subscribe immediately, don't wait for the first tick
	ticker := time.NewTicker(m.pollInterval())
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			repoint()
		}
	}
}

// resolveSession returns the worker's current session to mirror: its
// project's most-recent exploratory session (what the live UI follows),
// falling back to the persisted session pointer when the exploratory
// lookup is unavailable.
func (m *Mirror) resolveSession(ctx context.Context, orgID string, workerID orgchart.WorkerID) string {
	state, err := LoadState(ctx, m.cfg.Store, orgID, workerID)
	if err != nil {
		return ""
	}
	if state.ProjectID != "" && m.cfg.ExploratorySession != nil {
		if sid, err := m.cfg.ExploratorySession(ctx, state.ProjectID); err == nil && sid != "" {
			return sid
		} else if err != nil && m.cfg.Logger != nil {
			m.cfg.Logger.Warn("helix mirror: resolve exploratory session", "worker", workerID, "project", state.ProjectID, "err", err)
		}
	}
	return state.SessionID
}

// subscribe attaches a bridge to one session's pubsub topic and pumps
// its frames onto the activation stream until ctx fires. The first
// subscribe is synchronous so a frame published right after isn't raced.
func (m *Mirror) subscribe(ctx context.Context, orgID string, workerID orgchart.WorkerID, sessionID string) {
	ownerID := ""
	if m.cfg.Client != nil {
		if owner, err := m.cfg.Client.SessionOwner(ctx, sessionID); err != nil {
			if m.cfg.Logger != nil {
				m.cfg.Logger.Warn("helix mirror: resolve session owner", "worker", workerID, "session", sessionID, "err", err)
			}
		} else {
			ownerID = owner
		}
	}
	publish := func(body string) {
		if body == "" {
			return
		}
		_, _ = agent.PublishActivationEvent(ctx, m.cfg.Store, m.cfg.Hub, m.cfg.NewID, m.cfg.Now, m.cfg.Logger, orgID, workerID, body)
	}
	b := newBridge(publish)
	ch, err := SubscribeSessionUpdates(ctx, m.cfg.PubSub, m.cfg.Snapshotter, ownerID, sessionID)
	if err != nil {
		if m.cfg.Logger != nil {
			m.cfg.Logger.Warn("helix mirror: subscribe", "worker", workerID, "session", sessionID, "err", err)
		}
		ch = nil
	}
	go m.pump(ctx, b, ownerID, sessionID, ch)
}

// pump drains the subscription channel into the bridge and reconnects
// with capped backoff until ctx fires.
func (m *Mirror) pump(ctx context.Context, b *bridge, ownerID, sessionID string, ch <-chan types.WebsocketEvent) {
	delay := time.Second
	for {
		if ch != nil {
			for u := range ch {
				b.apply(u)
			}
		}
		select {
		case <-ctx.Done():
			b.stream.Flush()
			return
		case <-time.After(delay):
		}
		if delay < 30*time.Second {
			delay *= 2
		}
		var err error
		ch, err = SubscribeSessionUpdates(ctx, m.cfg.PubSub, m.cfg.Snapshotter, ownerID, sessionID)
		if err != nil {
			if m.cfg.Logger != nil {
				m.cfg.Logger.Warn("helix mirror: re-subscribe", "session", sessionID, "err", err)
			}
			ch = nil
		}
	}
}
