package slack

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/helixml/helix/api/pkg/org/domain/store"
	"github.com/helixml/helix/api/pkg/org/domain/streaming"
	"github.com/helixml/helix/api/pkg/org/infrastructure/persistence/memory"
)

func appendInbound(t *testing.T, s *store.Store, orgID, topicID, channel, ts, threadTS string) {
	t.Helper()
	extra, _ := json.Marshal(slackExtra{Channel: channel})
	msg := streaming.Message{From: "U1", Body: "hi", MessageID: ts, ThreadID: threadTS, Extra: extra}
	ev, err := streaming.NewMessageEvent(streaming.EventID("e-"+ts), streaming.TopicID(topicID), "", msg, time.Now().UTC(), orgID)
	if err != nil {
		t.Fatalf("NewMessageEvent: %v", err)
	}
	if err := s.Events.Append(context.Background(), ev); err != nil {
		t.Fatalf("Append: %v", err)
	}
}

func newTestOutbound(events store.Events) *Outbound {
	return NewOutbound(nil, events, nil, nil)
}

func TestResolveTarget_ExplicitChannelOnReply(t *testing.T) {
	o := newTestOutbound(memory.New().Events)
	extra, _ := json.Marshal(slackExtra{Channel: "C-explicit"})
	ch, thr := o.resolveTarget(context.Background(), streaming.Topic{ID: "tp1", OrganizationID: "org1"},
		streaming.Message{Extra: extra, ThreadID: "1700.1"})
	if ch != "C-explicit" || thr != "1700.1" {
		t.Fatalf("explicit: got (%q,%q)", ch, thr)
	}
}

func TestResolveTarget_ThreadMatch(t *testing.T) {
	s := memory.New()
	appendInbound(t, s, "org1", "tp1", "C-other", "1700.0", "")  // unrelated
	appendInbound(t, s, "org1", "tp1", "C-thread", "1700.5", "") // the message being replied to (ts 1700.5)
	o := newTestOutbound(s.Events)

	// Reply threads under message 1700.5 -> should target C-thread.
	ch, thr := o.resolveTarget(context.Background(), streaming.Topic{ID: "tp1", OrganizationID: "org1"},
		streaming.Message{ThreadID: "1700.5"})
	if ch != "C-thread" {
		t.Fatalf("thread-match channel = %q, want C-thread", ch)
	}
	if thr != "1700.5" {
		t.Fatalf("thread = %q, want 1700.5", thr)
	}
}

func TestResolveTarget_FallsBackToLatestInbound(t *testing.T) {
	s := memory.New()
	appendInbound(t, s, "org1", "tp1", "C-old", "1700.0", "")
	appendInbound(t, s, "org1", "tp1", "C-new", "1700.9", "") // newest
	o := newTestOutbound(s.Events)

	// Reply carries no channel/thread -> newest inbound channel.
	ch, _ := o.resolveTarget(context.Background(), streaming.Topic{ID: "tp1", OrganizationID: "org1"},
		streaming.Message{})
	if ch != "C-new" {
		t.Fatalf("fallback channel = %q, want C-new (newest inbound)", ch)
	}
}

func TestResolveTarget_SkipsWorkerEvents(t *testing.T) {
	s := memory.New()
	// A worker-published event (source != "") must be ignored as a channel source.
	wmsg := streaming.Message{Body: "a reply"}
	wev, _ := streaming.NewMessageEvent("e-w", "tp1", "w-bot", wmsg, time.Now().UTC(), "org1")
	_ = s.Events.Append(context.Background(), wev)
	appendInbound(t, s, "org1", "tp1", "C-inbound", "1700.0", "")
	o := newTestOutbound(s.Events)

	ch, _ := o.resolveTarget(context.Background(), streaming.Topic{ID: "tp1", OrganizationID: "org1"}, streaming.Message{})
	if ch != "C-inbound" {
		t.Fatalf("channel = %q, want C-inbound (worker events skipped)", ch)
	}
}
