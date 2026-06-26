package slackrouting

import (
	"context"
	"log/slog"
	"time"

	"github.com/helixml/helix/api/pkg/org/domain/domainevent"
	"github.com/helixml/helix/api/pkg/org/domain/processor"
	"github.com/helixml/helix/api/pkg/org/domain/streaming"
)

// defaultWindow bounds the membership read: a Worker is a thread
// participant only if they were routed into it within this window. It is a
// read bound, not a retention policy — the log keeps everything; old
// participation simply ages out of "currently following". A week covers
// "reply the next morning" and "reply after the weekend".
const defaultWindow = 7 * 24 * time.Hour

// Publisher is the narrow publish port the follower uses to deliver a
// message to a thread member's route Topic. publishing.Publishing satisfies
// it (same shape as processing.Publisher).
type Publisher interface {
	Publish(ctx context.Context, orgID string, topicID streaming.TopicID, from string, msg streaming.Message) (streaming.Event, error)
}

// ThreadFollower implements processing.PostRouter for Slack auto-routers. It
// turns name-match routing into thread-aware routing using the append-only
// domain-event log as the only state:
//
//  1. Every Worker the message was name-matched to is recorded as a
//     participant of the message's thread (idempotently). Naming a new
//     Worker mid-thread pulls them in here.
//  2. When the router opts into thread-follow, every Worker already
//     participating in the thread — but not named in this message — is
//     ALSO delivered the message, so an ongoing conversation reaches
//     everyone in it, not just whoever was named last.
//
// All of this lives outside the pure processor.Process, which only ever
// does the stateless name match.
type ThreadFollower struct {
	events    domainevent.Repository
	publisher Publisher
	window    time.Duration
	now       func() time.Time
	newID     func() string
	logger    *slog.Logger
}

// ThreadFollowerDeps are the constructor-injected collaborators.
type ThreadFollowerDeps struct {
	Events    domainevent.Repository
	Publisher Publisher
	NewID     func() string
	Now       func() time.Time
	// Window overrides the participation read window (default 7 days).
	Window time.Duration
	Logger *slog.Logger
}

// NewThreadFollower constructs a ThreadFollower.
func NewThreadFollower(deps ThreadFollowerDeps) *ThreadFollower {
	now := deps.Now
	if now == nil {
		now = func() time.Time { return time.Now().UTC() }
	}
	logger := deps.Logger
	if logger == nil {
		logger = slog.Default()
	}
	window := deps.Window
	if window == 0 {
		window = defaultWindow
	}
	return &ThreadFollower{
		events:    deps.Events,
		publisher: deps.Publisher,
		window:    window,
		now:       now,
		newID:     deps.NewID,
		logger:    logger,
	}
}

// threadRoot derives the thread key a message belongs to: its explicit
// thread id, falling back to its own message id (a top-level Slack message
// is a potential thread root). Empty when neither is set (a non-Slack or
// id-less message — nothing to track).
func threadRoot(msg streaming.Message) string {
	if msg.ThreadID != "" {
		return msg.ThreadID
	}
	return msg.MessageID
}

// AfterRoute records participation for the named recipients and, when
// thread-follow is on, fans the message out to the thread's existing
// members. Best-effort: failures are logged, never propagated — a routing
// extension must not break the publish that triggered it.
func (f *ThreadFollower) AfterRoute(ctx context.Context, p processor.Processor, msg streaming.Message, results []processor.Result) {
	if f == nil || f.events == nil {
		return
	}
	root := threadRoot(msg)
	if root == "" {
		return
	}
	orgID := p.OrganizationID

	// route output Topic ⇄ ManagedFor Worker.
	workerByTopic := map[streaming.TopicID]string{}
	topicByWorker := map[string]streaming.TopicID{}
	for _, o := range p.Outputs {
		if o.ManagedFor != "" {
			workerByTopic[o.TopicID] = o.ManagedFor
			topicByWorker[o.ManagedFor] = o.TopicID
		}
	}

	// Workers this message was name-matched to.
	matched := map[string]struct{}{}
	for _, res := range results {
		if w, ok := workerByTopic[res.TopicID]; ok {
			matched[w] = struct{}{}
		}
	}

	// Prior members (before this message), within the window.
	var since time.Time
	if f.window > 0 {
		since = f.now().Add(-f.window)
	}
	prior, err := f.events.ListBySubject(ctx, orgID, domainevent.TypeSlackThreadParticipant, root, since)
	if err != nil {
		f.logger.Warn("slackrouting.threadfollow: list members", "thread", root, "err", err)
	}
	priorMembers := domainevent.Participants(prior)
	priorSet := map[string]struct{}{}
	for _, w := range priorMembers {
		priorSet[w] = struct{}{}
	}

	// Record newly-named participants (skip those already members).
	for w := range matched {
		if _, ok := priorSet[w]; ok {
			continue
		}
		ev, err := domainevent.New(f.mintID(), orgID, domainevent.TypeSlackThreadParticipant, root, w, string(p.ID), nil, f.now())
		if err != nil {
			f.logger.Warn("slackrouting.threadfollow: build event", "worker", w, "err", err)
			continue
		}
		if err := f.events.Append(ctx, ev); err != nil {
			f.logger.Warn("slackrouting.threadfollow: append member", "worker", w, "thread", root, "err", err)
		}
	}

	// Thread-follow fan-out: deliver to prior members not named in this
	// message (the named ones already got it via the normal route publish).
	if !ThreadFollowEnabled(p.Config) || f.publisher == nil {
		return
	}
	for _, w := range priorMembers {
		if _, ok := matched[w]; ok {
			continue
		}
		topic, ok := topicByWorker[w]
		if !ok {
			continue // member's managed route is gone (Worker departed) — skip
		}
		if _, err := f.publisher.Publish(ctx, orgID, topic, "", msg); err != nil {
			f.logger.Warn("slackrouting.threadfollow: deliver to member", "worker", w, "topic", topic, "err", err)
		}
	}
}

func (f *ThreadFollower) mintID() string {
	if f.newID != nil {
		return f.newID()
	}
	// Fall back to a timestamp-derived id (Now is always seamed).
	return "de-" + f.now().UTC().Format("20060102150405.000000000")
}

// compile-time assertion that AfterRoute matches processing.PostRouter.
var _ interface {
	AfterRoute(ctx context.Context, p processor.Processor, msg streaming.Message, results []processor.Result)
} = (*ThreadFollower)(nil)
