package dispatch_test

// Multi-tenant isolation gate for the org-graph service layer.
//
// WHY THIS FILE EXISTS
// --------------------
// The org store is keyed by composite (id, org_id) PKs, so the store
// layer is tenant-safe by construction (proved in
// infrastructure/persistence/gorm/multitenant_test.go). Every
// multi-tenancy bug we've actually shipped lived one layer UP, in the
// long-lived in-memory structures that sit above the store and were
// keyed by an id that is unique only WITHIN an org:
//
//   - the per-Worker spawner config (frozen to the first org)
//   - the activation Queue's serialisation lanes (keyed by workerID)
//   - the transcript Mirror's tracker map (keyed by workerID)
//   - the wakebus wake topics (keyed by topicID)
//
// Every org's owner is "w-owner"; user-named topics like "s-general"
// collide trivially. So the canonical trigger for this whole bug class
// is: TWO orgs holding the SAME ids. This file drives that trigger
// through the real Dispatcher → Queue → Spawner wiring and asserts an
// event for one org never activates the other org's identically-named
// Worker.
//
// WHEN YOU ADD A NEW PROCESS-WIDE SINGLETON / CACHE to the org runtime,
// add its colliding-id isolation assertion here (or, for a leaf
// component, alongside it — see the siblings below) so the gate keeps
// pace:
//   - activation:  TestQueueIsolatesSameWorkerIDAcrossOrgs
//   - helix:       TestMirrorIsolatesSameWorkerIDAcrossOrgs,
//                  TestSpawnerHonorsSharedSemaphore,
//                  TestEnsureScopesProjectToParamOrg_NotStructOrgID
//   - wakebus:   TestNotify_IsolatedAcrossOrgs,
//                  TestSubscribeAll_IsolatedAcrossOrgs

import (
	"context"
	"io"
	"log/slog"
	"sort"
	"testing"
	"time"

	"github.com/helixml/helix/api/pkg/org/application/dispatch"
	"github.com/helixml/helix/api/pkg/org/domain/activation"
	"github.com/helixml/helix/api/pkg/org/domain/orgchart"
	"github.com/helixml/helix/api/pkg/org/domain/store"
	"github.com/helixml/helix/api/pkg/org/domain/streaming"
	"github.com/helixml/helix/api/pkg/org/domain/transport"
	orggorm "github.com/helixml/helix/api/pkg/org/infrastructure/persistence/gorm"
	"github.com/helixml/helix/api/pkg/org/infrastructure/runtime"
)

// orgActivation captures one Spawner invocation WITH its org, which is
// the field the bug class corrupts (the leaf recordedActivation in
// dispatcher_test.go deliberately drops orgID).
type orgActivation struct {
	OrgID    string
	WorkerID orgchart.BotID
}

// seedTenant provisions one org with a worker + topic + subscription,
// all using caller-supplied ids. Call it twice with the SAME ids and
// different orgs to set up a collision.
func seedTenant(t *testing.T, s *store.Store, orgID string, workerID orgchart.BotID, topicID streaming.TopicID) {
	t.Helper()
	ctx := context.Background()
	now := time.Now().UTC()

	w, err := orgchart.NewBot(workerID, "# "+string(workerID), nil, nil, now, orgID)
	if err != nil {
		t.Fatalf("[%s] new bot: %v", orgID, err)
	}
	if err := s.Bots.Create(ctx, w); err != nil {
		t.Fatalf("[%s] create bot: %v", orgID, err)
	}
	// A local topic (no outbound) so Dispatch's only effect is the
	// subscriber activation we're asserting on.
	topic, err := streaming.NewTopic(topicID, string(topicID), "", workerID, now, transport.LocalTransport(), orgID)
	if err != nil {
		t.Fatalf("[%s] new topic: %v", orgID, err)
	}
	if err := s.Topics.Create(ctx, topic); err != nil {
		t.Fatalf("[%s] create topic: %v", orgID, err)
	}
	sub, err := streaming.NewSubscription(string(workerID), topicID, now, orgID)
	if err != nil {
		t.Fatalf("[%s] new subscription: %v", orgID, err)
	}
	if err := s.Subscriptions.Create(ctx, sub); err != nil {
		t.Fatalf("[%s] create subscription: %v", orgID, err)
	}
}

// TestDispatch_IsolatesCollidingIDsAcrossOrgs is the integration leg of
// the gate: two orgs with the SAME worker id ("w-owner") subscribed to
// the SAME topic id ("s-general"). An event for org-a must activate
// ONLY org-a's worker, under org-a — never org-b's identically-named
// worker. This exercises the real Dispatcher.Dispatch →
// Subscriptions.ListForTopic(org) → Queue.Enqueue(org) → Spawner(org)
// path, catching any call site that drops or crosses the org.
func TestDispatch_IsolatesCollidingIDsAcrossOrgs(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	s := orggorm.GetOrgTestDB(t)

	const (
		workerID = orgchart.BotID("w-owner")      // identical across orgs
		topicID  = streaming.TopicID("s-general") // identical across orgs
	)
	seedTenant(t, s, "org-a", workerID, topicID)
	seedTenant(t, s, "org-b", workerID, topicID)

	rec := make(chan orgActivation, 16)
	spawner := runtime.Spawner(func(_ context.Context, orgID string, wid orgchart.BotID, _ []activation.Trigger) error {
		rec <- orgActivation{OrgID: orgID, WorkerID: wid}
		return nil
	})
	d := dispatch.New(s, spawner, slog.New(slog.NewTextHandler(io.Discard, nil)))

	drain := func(window time.Duration) []orgActivation {
		deadline := time.After(window)
		var got []orgActivation
		for {
			select {
			case r := <-rec:
				got = append(got, r)
			case <-deadline:
				sort.Slice(got, func(i, j int) bool { return got[i].OrgID < got[j].OrgID })
				return got
			}
		}
	}

	// Event for org-a only. NewMessageEvent encodes the canonical
	// Message envelope the dispatcher parses before fan-out.
	eA, err := streaming.NewMessageEvent("e-a-1", topicID, "external",
		streaming.Message{From: "external", Body: "for org-a"}, time.Now().UTC(), "org-a")
	if err != nil {
		t.Fatalf("new event a: %v", err)
	}
	d.Dispatch(ctx, eA)

	got := drain(500 * time.Millisecond)
	if len(got) != 1 {
		t.Fatalf("org-a event produced %d activations, want exactly 1: %+v — a colliding-id worker in another org must not be activated", len(got), got)
	}
	if got[0].OrgID != "org-a" || got[0].WorkerID != workerID {
		t.Fatalf("org-a event activated %+v, want {org-a w-owner} — the activation crossed tenants", got[0])
	}

	// Now org-b: must activate org-b's worker, under org-b.
	eB, err := streaming.NewMessageEvent("e-b-1", topicID, "external",
		streaming.Message{From: "external", Body: "for org-b"}, time.Now().UTC(), "org-b")
	if err != nil {
		t.Fatalf("new event b: %v", err)
	}
	d.Dispatch(ctx, eB)

	got = drain(500 * time.Millisecond)
	if len(got) != 1 {
		t.Fatalf("org-b event produced %d activations, want exactly 1: %+v", len(got), got)
	}
	if got[0].OrgID != "org-b" || got[0].WorkerID != workerID {
		t.Fatalf("org-b event activated %+v, want {org-b w-owner}", got[0])
	}
}

// TestDispatch_CollidingIDsActivateConcurrently deterministically pins
// the activation-queue half of the bug class at the integration level.
// Two orgs' identically-named "w-owner" must serialise INDEPENDENTLY:
// an in-flight activation for org-a's w-owner must not block org-b's.
//
// The spawner blocks until released. We dispatch to both orgs and
// require BOTH activations to enter the spawner concurrently. If the
// queue keyed its lanes by workerID alone (the shipped bug), both orgs'
// w-owner would share one lane and the second would queue behind the
// first — only one entry would arrive while blocked and this test would
// time out.
func TestDispatch_CollidingIDsActivateConcurrently(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	s := orggorm.GetOrgTestDB(t)

	const (
		workerID = orgchart.BotID("w-owner")
		topicID  = streaming.TopicID("s-general")
	)
	seedTenant(t, s, "org-a", workerID, topicID)
	seedTenant(t, s, "org-b", workerID, topicID)

	entered := make(chan string, 2) // orgID of each activation that started
	release := make(chan struct{})
	spawner := runtime.Spawner(func(_ context.Context, orgID string, _ orgchart.BotID, _ []activation.Trigger) error {
		entered <- orgID
		<-release
		return nil
	})
	d := dispatch.New(s, spawner, slog.New(slog.NewTextHandler(io.Discard, nil)))

	mkEvent := func(id streaming.EventID, org string) streaming.Event {
		e, err := streaming.NewMessageEvent(id, topicID, "external",
			streaming.Message{From: "external", Body: "x"}, time.Now().UTC(), org)
		if err != nil {
			t.Fatalf("new event: %v", err)
		}
		return e
	}
	d.Dispatch(ctx, mkEvent("e-a", "org-a"))
	d.Dispatch(ctx, mkEvent("e-b", "org-b"))

	// Both must enter concurrently — collect two distinct orgs before
	// releasing either. A shared lane would deliver only one here.
	got := map[string]struct{}{}
	deadline := time.After(2 * time.Second)
	for len(got) < 2 {
		select {
		case org := <-entered:
			got[org] = struct{}{}
		case <-deadline:
			close(release)
			t.Fatalf("only %d/2 colliding-id activations ran concurrently: %v — org-a's w-owner is blocking org-b's (shared lane, cross-tenant)", len(got), got)
		}
	}
	close(release)

	if _, ok := got["org-a"]; !ok {
		t.Fatal("org-a's w-owner never activated")
	}
	if _, ok := got["org-b"]; !ok {
		t.Fatal("org-b's w-owner never activated")
	}
}
