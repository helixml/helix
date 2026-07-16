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

func recordDMRecipient(t *testing.T, s *store.Store, routerID, channel, worker string, at time.Time) {
	t.Helper()
	ev, err := domainevent.New("dm-"+routerID+"-"+channel+"-"+worker, org, domainevent.TypeSlackDMRecipient, routerID+"/"+channel, worker, routerID, nil, at)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.DomainEvents.Append(context.Background(), ev); err != nil {
		t.Fatal(err)
	}
}

func slackMessage(channelType, channel, messageID, threadID string) streaming.Message {
	extra, _ := json.Marshal(map[string]string{"slack_channel": channel, "slack_channel_type": channelType})
	return streaming.Message{MessageID: messageID, ThreadID: threadID, Body: "reply", Extra: extra}
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

func TestRecordParticipantFeedsInboundThreadFollow(t *testing.T) {
	ctx := context.Background()
	box, f, pub := newFollower(t)
	if err := f.RecordParticipant(ctx, org, "p-slack-router", "T1", "w-alice"); err != nil {
		t.Fatal(err)
	}
	if err := f.RecordParticipant(ctx, org, "p-slack-router", "T1", "w-alice"); err != nil {
		t.Fatal(err)
	}
	if parts := participants(t, box, "T1"); len(parts) != 1 || parts[0] != "w-alice" {
		t.Fatalf("want one alice participant, got %v", parts)
	}
	f.AfterRoute(ctx, threadRouter(true), streaming.Message{ThreadID: "T1", MessageID: "T2", Body: "human reply"}, nil)
	if len(pub.calls) != 1 || pub.calls[0] != "s-alice" {
		t.Fatalf("want inbound reply delivered to alice, got %v", pub.calls)
	}
}

func TestRecordDMRecipientAppendsRouterChannelEvent(t *testing.T) {
	ctx := context.Background()
	box, f, _ := newFollower(t)
	if err := f.RecordDMRecipient(ctx, org, "p-slack-router", "D1", "w-alice"); err != nil {
		t.Fatal(err)
	}
	events, err := box.DomainEvents.ListBySubject(ctx, org, domainevent.TypeSlackDMRecipient, "p-slack-router/D1", time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 1 || events[0].Worker != "w-alice" || events[0].Source != "p-slack-router" {
		t.Fatalf("DM recipient events = %#v", events)
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

func TestDMReplyRoutesLatestRecipientAndRecordsParticipant(t *testing.T) {
	ctx := context.Background()
	box, f, pub := newFollower(t)
	now := time.Date(2026, 6, 26, 12, 0, 0, 0, time.UTC)
	recordDMRecipient(t, box, "p-slack-router", "D1", "w-alice", now.Add(-time.Minute))
	recordDMRecipient(t, box, "p-slack-router", "D1", "w-bob", now)

	f.AfterRoute(ctx, threadRouter(true), slackMessage("im", "D1", "T1", ""), nameMatch("s-default"))

	if len(pub.calls) != 1 || pub.calls[0] != "s-bob" {
		t.Fatalf("want one delivery to latest recipient bob, got %v", pub.calls)
	}
	if got := participants(t, box, "T1"); len(got) != 1 || got[0] != "w-bob" {
		t.Fatalf("new root participants = %v", got)
	}
}

func TestDMReplyExplicitNameMatchWins(t *testing.T) {
	box, f, pub := newFollower(t)
	recordDMRecipient(t, box, "p-slack-router", "D1", "w-alice", time.Date(2026, 6, 26, 12, 0, 0, 0, time.UTC))

	f.AfterRoute(context.Background(), threadRouter(true), slackMessage("im", "D1", "T1", ""), nameMatch("s-bob"))

	if len(pub.calls) != 0 {
		t.Fatalf("explicit match should suppress DM fallback, got %v", pub.calls)
	}
	if got := participants(t, box, "T1"); len(got) != 1 || got[0] != "w-bob" {
		t.Fatalf("participants = %v", got)
	}
}

func TestDMReplyFallbackOnlyAppliesToTopLevelIM(t *testing.T) {
	for _, tc := range []struct {
		name        string
		channelType string
		threadID    string
	}{
		{name: "channel", channelType: "channel"},
		{name: "group DM", channelType: "mpim"},
		{name: "threaded DM", channelType: "im", threadID: "ROOT"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			box, f, pub := newFollower(t)
			recordDMRecipient(t, box, "p-slack-router", "D1", "w-alice", time.Date(2026, 6, 26, 12, 0, 0, 0, time.UTC))

			f.AfterRoute(context.Background(), threadRouter(true), slackMessage(tc.channelType, "D1", "T1", tc.threadID), nil)

			if len(pub.calls) != 0 {
				t.Fatalf("unexpected DM fallback: %v", pub.calls)
			}
		})
	}
}

func TestDMReplyRecipientExpires(t *testing.T) {
	box, f, pub := newFollower(t)
	recordDMRecipient(t, box, "p-slack-router", "D1", "w-alice", time.Date(2026, 6, 19, 11, 59, 59, 0, time.UTC))

	f.AfterRoute(context.Background(), threadRouter(true), slackMessage("im", "D1", "T1", ""), nil)

	if len(pub.calls) != 0 {
		t.Fatalf("expired recipient received fallback: %v", pub.calls)
	}
}

func TestDMReplyRecipientIsolatedByRouterAndChannel(t *testing.T) {
	box, f, pub := newFollower(t)
	now := time.Date(2026, 6, 26, 12, 0, 0, 0, time.UTC)
	recordDMRecipient(t, box, "p-slack-router", "D1", "w-alice", now.Add(-time.Minute))
	recordDMRecipient(t, box, "p-slack-router-2", "D1", "w-bob", now)
	recordDMRecipient(t, box, "p-slack-router", "D2", "w-bob", now)

	f.AfterRoute(context.Background(), threadRouter(true), slackMessage("im", "D1", "T1", ""), nil)

	if len(pub.calls) != 1 || pub.calls[0] != "s-alice" {
		t.Fatalf("want isolated alice recipient, got %v", pub.calls)
	}
}
