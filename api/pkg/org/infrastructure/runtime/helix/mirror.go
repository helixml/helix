package helix

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/helixml/helix/api/pkg/org/domain/orgchart"
	"github.com/helixml/helix/api/pkg/org/domain/store"
	"github.com/helixml/helix/api/pkg/org/infrastructure/wakebus"
	"github.com/helixml/helix/api/pkg/pubsub"
	"github.com/helixml/helix/api/pkg/types"
)

const defaultMirrorPoll = 5 * time.Second

// MirrorConfig wires the session-layer transcript Mirror.
type MirrorConfig struct {
	PubSub      pubsub.PubSub
	Snapshotter SessionPreamble
	// Client resolves the session owner so we subscribe to the right
	// GetSessionQueue(owner, session) topic.
	Client SpawnerClient
	// ExploratorySession returns a project's current exploratory session
	// (the one the inline chat / live UI follow). The mirror polls it to
	// track the worker as its session churns. "" means no session yet.
	ExploratorySession func(ctx context.Context, projectID string) (string, error)
	Store              *store.Store
	Hub                *wakebus.Bus
	NewID              func() string
	Now                func() time.Time
	Logger             *slog.Logger
	PollInterval       time.Duration // <=0 uses defaultMirrorPoll; seam for tests
}

// Mirror is the single writer of worker transcript segments: it keeps
// one subscription per tracked worker pointed at that worker's current
// session and republishes every settled entry (plus the user prompt)
// onto s-transcript-<worker>. Every turn — spawner activation, inline
// chat, anything on /sessions/chat — flows through the session topic, so
// one subscriber captures them all.
//
// A worker's session is not stable (stale resume opens a fresh one;
// inline chat can land on a newer one than the spawner persisted), so we
// track the worker — whose project is stable — and poll its current
// exploratory session, re-pointing when it changes.
type Mirror struct {
	base context.Context
	cfg  MirrorConfig

	mu      sync.Mutex
	tracked map[mirrorKey]context.CancelFunc
}

// mirrorKey identifies a tracked worker. It MUST include orgID: worker
// IDs are unique only within an org (the store keys workers by the
// composite (org_id, id)), so every org's owner shares the id
// "w-owner". Keying trackers by workerID alone collapses every org's
// w-owner into one entry — the second org's Ensure sees the key already
// present and no-ops, so that org's transcript is never mirrored, and
// Stop tears down whichever org happens to hold the slot.
type mirrorKey struct {
	orgID    string
	workerID orgchart.BotID
}

// NewMirror constructs a Mirror. base bounds every tracker (typically
// the server lifetime). A nil PubSub disables the mirror.
func NewMirror(base context.Context, cfg MirrorConfig) *Mirror {
	if base == nil {
		base = context.Background()
	}
	return &Mirror{
		base:    base,
		cfg:     cfg,
		tracked: map[mirrorKey]context.CancelFunc{},
	}
}

func (m *Mirror) pollInterval() time.Duration {
	if m.cfg.PollInterval > 0 {
		return m.cfg.PollInterval
	}
	return defaultMirrorPoll
}

// Ensure starts tracking a worker (idempotent). The session is resolved
// by the tracker, not passed in — the caller's notion of "the session"
// is exactly what goes stale.
func (m *Mirror) Ensure(orgID string, workerID orgchart.BotID) {
	if m == nil || m.cfg.PubSub == nil {
		return
	}
	key := mirrorKey{orgID: orgID, workerID: workerID}
	m.mu.Lock()
	if _, ok := m.tracked[key]; ok {
		m.mu.Unlock()
		return
	}
	ctx, cancel := context.WithCancel(m.base)
	m.tracked[key] = cancel
	m.mu.Unlock()
	go m.track(ctx, orgID, workerID)
}

// EnsureAll tracks every worker in an org — called from bootstrap so
// pre-existing / inline-chat-only workers are mirrored without an
// activation first.
func (m *Mirror) EnsureAll(ctx context.Context, orgID string) {
	if m == nil || m.cfg.PubSub == nil || m.cfg.Store == nil {
		return
	}
	bots, err := m.cfg.Store.Bots.List(ctx, orgID)
	if err != nil {
		if m.cfg.Logger != nil {
			m.cfg.Logger.Warn("helix mirror: list bots for sweep", "org", orgID, "err", err)
		}
		return
	}
	for _, b := range bots {
		// Human nodes never run and never have a session — mirroring one
		// would leave a goroutine polling for a session that never appears.
		if b.IsHuman() {
			continue
		}
		m.Ensure(orgID, b.ID)
	}
}

// Stop stops tracking a worker (on fire). orgID is required — trackers
// are keyed per (org, worker), so a bare workerID would tear down the
// wrong tenant's tracker when two orgs share the id.
func (m *Mirror) Stop(orgID string, workerID orgchart.BotID) {
	if m == nil {
		return
	}
	key := mirrorKey{orgID: orgID, workerID: workerID}
	m.mu.Lock()
	defer m.mu.Unlock()
	if cancel, ok := m.tracked[key]; ok {
		cancel()
		delete(m.tracked, key)
	}
}

// track resolves the worker's current session and (re)subscribes
// whenever it changes, polling on an interval, until ctx fires. Each
// session gets a fresh bridge.
func (m *Mirror) track(ctx context.Context, orgID string, workerID orgchart.BotID) {
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
		curCancel() // old pump flushes pending entries on ctx.Done
		subCtx, cancel := context.WithCancel(ctx)
		curCancel = cancel
		curSession = desired
		m.subscribe(subCtx, orgID, workerID, desired)
		if m.cfg.Logger != nil {
			m.cfg.Logger.Info("helix mirror: pointed at session", "worker", workerID, "session", desired)
		}
	}

	repoint() // immediately, don't wait for the first tick
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

// resolveSession returns the worker's current exploratory session,
// falling back to the persisted pointer when that lookup is unavailable.
func (m *Mirror) resolveSession(ctx context.Context, orgID string, workerID orgchart.BotID) string {
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

// subscribe attaches a bridge to one session's topic and pumps its
// frames onto the transcript until ctx fires. The first subscribe
// is synchronous so a frame published right after isn't raced.
func (m *Mirror) subscribe(ctx context.Context, orgID string, workerID orgchart.BotID, sessionID string) {
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
	rec := newTranscriptRecorder(m.cfg.Store, m.cfg.Hub, m.cfg.NewID, m.cfg.Now, m.cfg.Logger)
	publish := func(body string) {
		if body == "" {
			return
		}
		_, _ = rec.Record(ctx, orgID, workerID, body)
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
			b.topic.Flush()
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
