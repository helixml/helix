package subscriptions

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/helixml/helix/api/pkg/org/domain/orgchart"
	"github.com/helixml/helix/api/pkg/org/domain/store"
	"github.com/helixml/helix/api/pkg/org/domain/streaming"
	"github.com/helixml/helix/api/pkg/org/domain/transport"
	"github.com/helixml/helix/api/pkg/org/infrastructure/persistence/memory"
)

func fixedClock() time.Time { return time.Date(2026, 6, 10, 12, 0, 0, 0, time.UTC) }

func newService(st *store.Store) *Subscriptions {
	return New(Deps{
		Subscriptions: st.Subscriptions,
		Topics:        st.Topics,
		Bots:          st.Bots,
		Now:           fixedClock,
	})
}

// seed creates a topic + worker in the org so subscribe has valid
// endpoints. Returns nothing — ids are fixed.
func seed(t *testing.T, st *store.Store, orgID string) {
	t.Helper()
	ctx := context.Background()
	topic, err := streaming.NewTopic("s-1", "s-1", "", "w-owner", fixedClock(), transport.LocalTransport(), orgID)
	if err != nil {
		t.Fatalf("new topic: %v", err)
	}
	if err := st.Topics.Create(ctx, topic); err != nil {
		t.Fatalf("create topic: %v", err)
	}
	b, _ := orgchart.NewBot("w-mark", "content", nil, fixedClock(), orgID)
	if err := st.Bots.Create(ctx, b); err != nil {
		t.Fatalf("create bot: %v", err)
	}
}

func TestSubscribe_Idempotent(t *testing.T) {
	t.Parallel()
	st := memory.New()
	svc := newService(st)
	ctx := context.Background()
	seed(t, st, "org-test")

	sub, created, err := svc.Subscribe(ctx, "org-test", "w-mark", "s-1")
	if err != nil {
		t.Fatalf("Subscribe: %v", err)
	}
	if !created {
		t.Fatal("first subscribe should report created=true")
	}
	if sub.BotID != "w-mark" || sub.TopicID != "s-1" {
		t.Fatalf("unexpected sub: %+v", sub)
	}

	// Second subscribe is a no-op, created=false, no error.
	_, created2, err := svc.Subscribe(ctx, "org-test", "w-mark", "s-1")
	if err != nil {
		t.Fatalf("second Subscribe: %v", err)
	}
	if created2 {
		t.Fatal("second subscribe should report created=false")
	}
	// Exactly one row.
	subs, _ := st.Subscriptions.ListForTopic(ctx, "org-test", "s-1")
	if len(subs) != 1 {
		t.Fatalf("subs = %d, want 1", len(subs))
	}
}

func TestSubscribe_TopicNotFound(t *testing.T) {
	t.Parallel()
	st := memory.New()
	svc := newService(st)
	seed(t, st, "org-test")
	_, _, err := svc.Subscribe(context.Background(), "org-test", "w-mark", "s-missing")
	if !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
}

func TestSubscribe_WorkerNotFound(t *testing.T) {
	t.Parallel()
	st := memory.New()
	svc := newService(st)
	seed(t, st, "org-test")
	_, _, err := svc.Subscribe(context.Background(), "org-test", "w-ghost", "s-1")
	if !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
}

func TestUnsubscribe(t *testing.T) {
	t.Parallel()
	st := memory.New()
	svc := newService(st)
	ctx := context.Background()
	seed(t, st, "org-test")
	if _, _, err := svc.Subscribe(ctx, "org-test", "w-mark", "s-1"); err != nil {
		t.Fatalf("Subscribe: %v", err)
	}
	if err := svc.Unsubscribe(ctx, "org-test", "w-mark", "s-1"); err != nil {
		t.Fatalf("Unsubscribe: %v", err)
	}
	subs, _ := st.Subscriptions.ListForTopic(ctx, "org-test", "s-1")
	if len(subs) != 0 {
		t.Fatalf("subs = %d, want 0", len(subs))
	}
}

func TestSubscribeTopics_MultipleIdempotent(t *testing.T) {
	t.Parallel()
	st := memory.New()
	svc := newService(st)
	ctx := context.Background()
	seed(t, st, "org-test")
	// add a second topic
	s2, err := streaming.NewTopic("s-2", "s-2", "", "w-owner", fixedClock(), transport.LocalTransport(), "org-test")
	if err != nil {
		t.Fatalf("new topic: %v", err)
	}
	if err := st.Topics.Create(ctx, s2); err != nil {
		t.Fatalf("create s-2: %v", err)
	}

	if err := svc.SubscribeTopics(ctx, "org-test", "w-mark", []streaming.TopicID{"s-1", "s-2"}); err != nil {
		t.Fatalf("SubscribeTopics: %v", err)
	}
	// Re-subscribe (both already present) is idempotent.
	if err := svc.SubscribeTopics(ctx, "org-test", "w-mark", []streaming.TopicID{"s-1", "s-2"}); err != nil {
		t.Fatalf("SubscribeTopics (repeat): %v", err)
	}
	for _, tid := range []streaming.TopicID{"s-1", "s-2"} {
		if _, err := st.Subscriptions.Find(ctx, "org-test", "w-mark", tid); err != nil {
			t.Fatalf("missing subscription (w-mark, %s): %v", tid, err)
		}
	}
}

func TestSubscribeTopics_UnknownTopicRejected(t *testing.T) {
	t.Parallel()
	st := memory.New()
	svc := newService(st)
	ctx := context.Background()
	seed(t, st, "org-test")
	err := svc.SubscribeTopics(ctx, "org-test", "w-mark", []streaming.TopicID{"s-1", "s-ghost"})
	if !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
	// Validated up front → the good topic must NOT have been subscribed.
	if _, err := st.Subscriptions.Find(ctx, "org-test", "w-mark", "s-1"); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("s-1 subscribed despite a bad id in the batch: %v", err)
	}
}

func TestUnsubscribeTopics_Idempotent(t *testing.T) {
	t.Parallel()
	st := memory.New()
	svc := newService(st)
	ctx := context.Background()
	seed(t, st, "org-test")
	if err := svc.SubscribeTopics(ctx, "org-test", "w-mark", []streaming.TopicID{"s-1"}); err != nil {
		t.Fatalf("SubscribeTopics: %v", err)
	}
	// Unsubscribing a subscribed + an unsubscribed topic is fine (idempotent).
	if err := svc.UnsubscribeTopics(ctx, "org-test", "w-mark", []streaming.TopicID{"s-1"}); err != nil {
		t.Fatalf("UnsubscribeTopics: %v", err)
	}
	if _, err := st.Subscriptions.Find(ctx, "org-test", "w-mark", "s-1"); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("s-1 still subscribed after unsubscribe: %v", err)
	}
	// Repeat is a no-op, not an error.
	if err := svc.UnsubscribeTopics(ctx, "org-test", "w-mark", []streaming.TopicID{"s-1"}); err != nil {
		t.Fatalf("UnsubscribeTopics (repeat): %v", err)
	}
}
