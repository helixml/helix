package slackrouting_test

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/helixml/helix/api/pkg/org/application/slackrouting"
	"github.com/helixml/helix/api/pkg/org/domain/domainevent"
	"github.com/helixml/helix/api/pkg/org/domain/processor"
	"github.com/helixml/helix/api/pkg/org/domain/store"
	"github.com/helixml/helix/api/pkg/org/domain/streaming"
	"github.com/helixml/helix/api/pkg/org/infrastructure/persistence/memory"
)

// recordingPublisher captures fan-out publishes.
type recordingPublisher struct {
	calls []streaming.TopicID
}

func (p *recordingPublisher) Publish(_ context.Context, _ string, topicID streaming.TopicID, _ string, _ streaming.Message) (streaming.Event, error) {
	p.calls = append(p.calls, topicID)
	return streaming.Event{}, nil
}

// router builds a processor with two managed routes (alice→s-alice,
// bob→s-bob) and the given thread-follow setting.
func threadRouter(threadFollow bool) processor.Processor {
	cfg, _ := json.Marshal(slackrouting.RouterConfig{ThreadFollow: threadFollow})
	return processor.Processor{
		ID: "p-slack-router", OrganizationID: org, Kind: processor.KindFilter, CreatedBy: processor.SystemActor, Config: cfg,
		Outputs: []processor.Output{
			{TopicID: "s-default", Label: "default"},
			{TopicID: "s-alice", ManagedFor: "w-alice", Owned: true},
			{TopicID: "s-bob", ManagedFor: "w-bob", Owned: true},
		},
	}
}

func nameMatch(topic streaming.TopicID) []processor.Result {
	return []processor.Result{{TopicID: topic, Message: streaming.Message{}}}
}

func newFollower(t *testing.T) (*store.Store, *slackrouting.ThreadFollower, *recordingPublisher) {
	t.Helper()
	s := memory.New()
	pub := &recordingPublisher{}
	var n int
	clock := time.Date(2026, 6, 26, 12, 0, 0, 0, time.UTC)
	f := slackrouting.NewThreadFollower(slackrouting.ThreadFollowerDeps{
		Events:    s.DomainEvents,
		Publisher: pub,
		NewID:     func() string { n++; return "de-" + string(rune('a'+n)) },
		Now:       func() time.Time { return clock },
	})
	return s, f, pub
}

// participants reads the recorded members of a thread from the domain-event
// log.
func participants(t *testing.T, s *store.Store, threadRoot string) []string {
	t.Helper()
	// Membership is keyed by (router, thread) — mirror the production scoping
	// (router id "p-slack-router" from threadRouter).
	return participantsFor(t, s, "p-slack-router", threadRoot)
}

// participantsFor reads membership for a specific router's thread (the
// production subject scoping is "<routerID>/<threadRoot>").
func participantsFor(t *testing.T, s *store.Store, routerID, threadRoot string) []string {
	t.Helper()
	evs, err := s.DomainEvents.ListBySubject(context.Background(), org, domainevent.TypeSlackThreadParticipant, routerID+"/"+threadRoot, time.Time{})
	if err != nil {
		t.Fatalf("list members: %v", err)
	}
	return domainevent.Participants(evs)
}

func TestDefaultConfigEnablesThreadFollow(t *testing.T) {
	if !slackrouting.ThreadFollowEnabled(slackrouting.DefaultConfig()) {
		t.Error("DefaultConfig should enable thread-follow (on by default)")
	}
}

func TestThreadFollowRecordsNamedParticipant(t *testing.T) {
	ctx := context.Background()
	box, f, pub := newFollower(t)

	// Root message (ts=T1) names alice. threadRoot = its own message id.
	msg := streaming.Message{MessageID: "T1", Body: "hey alice"}
	f.AfterRoute(ctx, threadRouter(false), msg, nameMatch("s-alice"))

	// Alice recorded as participant of thread T1; no fan-out (follow off).
	parts := participants(t, box, "T1")
	if len(parts) != 1 || parts[0] != "w-alice" {
		t.Fatalf("want [w-alice], got %v", parts)
	}
	if len(pub.calls) != 0 {
		t.Errorf("thread-follow off: want no fan-out, got %v", pub.calls)
	}
}

func TestThreadFollowDeliversToPriorMembers(t *testing.T) {
	ctx := context.Background()
	box, f, pub := newFollower(t)
	router := threadRouter(true)

	// Turn 1: root T1 names alice → alice joins.
	f.AfterRoute(ctx, router, streaming.Message{MessageID: "T1", Body: "hey alice"}, nameMatch("s-alice"))
	// Turn 2: a reply in thread T1 names nobody. Alice (prior member) should
	// still receive it via thread-follow.
	pub.calls = nil
	f.AfterRoute(ctx, router, streaming.Message{ThreadID: "T1", MessageID: "T2", Body: "what do you think?"}, nil)

	if len(pub.calls) != 1 || pub.calls[0] != "s-alice" {
		t.Fatalf("want fan-out to s-alice, got %v", pub.calls)
	}
	_ = box
}

func TestThreadFollowPullsInNewlyNamedWorkerMidThread(t *testing.T) {
	ctx := context.Background()
	box, f, pub := newFollower(t)
	router := threadRouter(true)

	f.AfterRoute(ctx, router, streaming.Message{MessageID: "T1", Body: "hey alice"}, nameMatch("s-alice"))
	// Turn 2 names bob mid-thread. Bob gets the normal route (not a fan-out),
	// alice gets the fan-out, and bob is now a member.
	pub.calls = nil
	f.AfterRoute(ctx, router, streaming.Message{ThreadID: "T1", MessageID: "T2", Body: "ask bob too"}, nameMatch("s-bob"))

	// Fan-out goes to alice only (bob was name-matched, delivered normally).
	if len(pub.calls) != 1 || pub.calls[0] != "s-alice" {
		t.Fatalf("want fan-out to s-alice only, got %v", pub.calls)
	}
	parts := participants(t, box, "T1")
	if len(parts) != 2 {
		t.Fatalf("want alice+bob as members, got %v", parts)
	}
	// Turn 3 names nobody: both alice and bob get the fan-out.
	pub.calls = nil
	f.AfterRoute(ctx, router, streaming.Message{ThreadID: "T1", MessageID: "T3", Body: "thanks"}, nil)
	if len(pub.calls) != 2 {
		t.Fatalf("want fan-out to both members, got %v", pub.calls)
	}
}

// Two workspaces (two routers) sharing a colliding Slack thread_ts must not
// share thread membership — subjects are router-scoped.
func TestThreadFollowIsolatedPerRouter(t *testing.T) {
	ctx := context.Background()
	box, f, _ := newFollower(t)

	r1 := threadRouter(true) // p-slack-router, routes alice/bob
	r2 := r1
	r2.ID = "p-slack-router-2"

	// Same thread_ts "T1" arrives in both workspaces, each naming a different
	// worker. Membership must stay separate.
	f.AfterRoute(ctx, r1, streaming.Message{MessageID: "T1", Body: "hi alice"}, nameMatch("s-alice"))
	f.AfterRoute(ctx, r2, streaming.Message{MessageID: "T1", Body: "hi bob"}, nameMatch("s-bob"))

	if p := participantsFor(t, box, "p-slack-router", "T1"); len(p) != 1 || p[0] != "w-alice" {
		t.Fatalf("router 1 thread T1 should have only alice, got %v", p)
	}
	if p := participantsFor(t, box, "p-slack-router-2", "T1"); len(p) != 1 || p[0] != "w-bob" {
		t.Fatalf("router 2 thread T1 should have only bob, got %v", p)
	}
}

func TestThreadFollowOffStillRecordsButNoFanout(t *testing.T) {
	ctx := context.Background()
	box, f, pub := newFollower(t)
	router := threadRouter(false)

	f.AfterRoute(ctx, router, streaming.Message{MessageID: "T1", Body: "hey alice"}, nameMatch("s-alice"))
	f.AfterRoute(ctx, router, streaming.Message{ThreadID: "T1", MessageID: "T2", Body: "still there?"}, nil)
	if len(pub.calls) != 0 {
		t.Errorf("follow off: want no fan-out, got %v", pub.calls)
	}
	if len(participants(t, box, "T1")) != 1 {
		t.Errorf("alice should still be recorded")
	}
}
