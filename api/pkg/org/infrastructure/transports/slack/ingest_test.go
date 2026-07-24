package slack

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/helixml/helix/api/pkg/org/domain/store"
	"github.com/helixml/helix/api/pkg/org/domain/streaming"
	"github.com/helixml/helix/api/pkg/org/domain/transport"
	"github.com/helixml/helix/api/pkg/org/infrastructure/persistence/memory"
	slackcore "github.com/helixml/helix/api/pkg/serviceconnection/slack"
)

// fakeWorkspaces resolves exactly one team -> one workspace install.
type fakeWorkspaces struct {
	team string
	ws   Workspace
}

func (f fakeWorkspaces) ByTeamID(_ context.Context, team string) (Workspace, error) {
	if team == f.team {
		return f.ws, nil
	}
	return Workspace{}, ErrNoWorkspace
}
func (f fakeWorkspaces) ByID(_ context.Context, id string) (Workspace, error) {
	if id == f.ws.ID {
		return f.ws, nil
	}
	return Workspace{}, ErrNoWorkspace
}

// recordingPublisher captures Publish calls.
type recordingPublisher struct {
	calls []publishCall
}
type publishCall struct {
	orgID   string
	topicID streaming.TopicID
	from    string
	msg     streaming.Message
}

func (p *recordingPublisher) PublishInbound(_ context.Context, orgID string, topicID streaming.TopicID, from string, msg streaming.Message) (streaming.Event, error) {
	p.calls = append(p.calls, publishCall{orgID, topicID, from, msg})
	return streaming.Event{}, nil
}

func seedSlackTopic(t *testing.T, s *store.Store, orgID, id, connID, channelID string) {
	t.Helper()
	cfg, _ := json.Marshal(transport.SlackConfig{ServiceConnectionID: connID, ChannelID: channelID})
	tp, err := streaming.NewTopic(
		streaming.TopicID(id), "slack-"+id, "", "", time.Now().UTC(),
		transport.Transport{Kind: transport.KindSlack, Config: cfg}, orgID,
	)
	if err != nil {
		t.Fatalf("NewTopic: %v", err)
	}
	if err := s.Topics.Create(context.Background(), tp); err != nil {
		t.Fatalf("Topics.Create: %v", err)
	}
}

func newTestIngest(t *testing.T) (*Ingest, *recordingPublisher, *store.Store) {
	t.Helper()
	s := memory.New()
	pub := &recordingPublisher{}
	ws := fakeWorkspaces{team: "T1", ws: Workspace{ID: "sc1", OrgID: "org1", TeamID: "T1", BotToken: "xoxb-x"}}
	return NewIngest(ws, s, pub, nil), pub, s
}

func TestIngest_BotEvent_Dropped(t *testing.T) {
	ing, pub, s := newTestIngest(t)
	seedSlackTopic(t, s, "org1", "tp1", "sc1", "")

	err := ing.OnEvent(context.Background(), "T1", slackcore.Event{Channel: "C1", Text: "hi", BotID: "B1"})
	if err != nil {
		t.Fatalf("OnEvent: %v", err)
	}
	if len(pub.calls) != 0 {
		t.Fatalf("bot event must not publish; got %d calls", len(pub.calls))
	}
}

func TestIngest_UnknownTeam_Dropped(t *testing.T) {
	ing, pub, s := newTestIngest(t)
	seedSlackTopic(t, s, "org1", "tp1", "sc1", "")

	if err := ing.OnEvent(context.Background(), "T-unknown", slackcore.Event{Channel: "C1", User: "U1", Text: "hi"}); err != nil {
		t.Fatalf("OnEvent: %v", err)
	}
	if len(pub.calls) != 0 {
		t.Fatalf("unknown team must not publish; got %d calls", len(pub.calls))
	}
}

// Workspace-scoped: a message from ANY channel of the workspace publishes
// onto the workspace topic, with the channel carried in the message Extra.
func TestIngest_AnyChannel_Published(t *testing.T) {
	ing, pub, s := newTestIngest(t)
	seedSlackTopic(t, s, "org1", "tp1", "sc1", "")

	err := ing.OnEvent(context.Background(), "T1", slackcore.Event{
		Channel: "C-random", ChannelType: "channel", User: "U1", Text: "!qa-bot help", TS: "1700.1", ThreadTS: "1699.9",
	})
	if err != nil {
		t.Fatalf("OnEvent: %v", err)
	}
	if len(pub.calls) != 1 {
		t.Fatalf("want 1 publish, got %d", len(pub.calls))
	}
	c := pub.calls[0]
	if c.orgID != "org1" || c.topicID != "tp1" || c.from != "" {
		t.Fatalf("publish args mismatch: %+v", c)
	}
	if c.msg.Body != "!qa-bot help" || c.msg.MessageID != "1700.1" || c.msg.ThreadID != "1699.9" || c.msg.From != "U1" {
		t.Fatalf("message mapping mismatch: %+v", c.msg)
	}
	var ex slackExtra
	if err := json.Unmarshal(c.msg.Extra, &ex); err != nil {
		t.Fatalf("unmarshal extra: %v", err)
	}
	if ex.Channel != "C-random" || ex.ChannelType != "channel" {
		t.Fatalf("Extra Slack coordinates = %#v", ex)
	}
	// The transport stamps a ReplyHint carrying the concrete coordinates
	// the agent needs to reply through publish or use the Slack API for rich actions.
	for _, want := range []string{"workspace-wide ingress Topic tp1 is inbound-only", "list_topics, find this ingress Topic by ID", "identify its service_connection_id", "If publish is available", "service_connection_id matches that ingress Topic", "channel_id is C-random", "If publish is unavailable or no matching Topic exists", "mint_credential", "chat.postMessage", "reactions.add", "files.upload", "conversations.replies", "conversations.history"} {
		if !strings.Contains(c.msg.ReplyHint, want) {
			t.Fatalf("ReplyHint %q missing %q", c.msg.ReplyHint, want)
		}
	}
	for _, want := range []string{"Do not fetch Slack history by default", "Only when earlier thread context is necessary", "Only when channel-root context is genuinely necessary"} {
		if !strings.Contains(c.msg.ReplyHint, want) {
			t.Fatalf("ReplyHint %q missing conditional history guidance %q", c.msg.ReplyHint, want)
		}
	}
}

func TestIngest_DMDoesNotEnterChannelBoundTopic(t *testing.T) {
	ing, pub, s := newTestIngest(t)
	seedSlackTopic(t, s, "org1", "tp1", "sc1", "C1")

	if err := ing.OnEvent(context.Background(), "T1", slackcore.Event{Channel: "D1", ChannelType: "im", User: "U1", Text: "hi"}); err != nil {
		t.Fatalf("OnEvent: %v", err)
	}
	if len(pub.calls) != 0 {
		t.Fatalf("DM must not publish to a channel-bound topic; got %d calls", len(pub.calls))
	}
}

func TestIngest_ExactChannelOverridesWorkspaceFallback(t *testing.T) {
	ing, pub, s := newTestIngest(t)
	seedSlackTopic(t, s, "org1", "tp1", "sc1", "C1")
	seedSlackTopic(t, s, "org1", "tp-wide", "sc1", "")

	if err := ing.OnEvent(context.Background(), "T1", slackcore.Event{Channel: "C1", ChannelType: "channel", User: "U1", Text: "hi"}); err != nil {
		t.Fatalf("OnEvent: %v", err)
	}
	if len(pub.calls) != 1 || pub.calls[0].topicID != "tp1" {
		t.Fatalf("matching channel publishes = %+v", pub.calls)
	}
	for _, want := range []string{"if publish is available", "call publish on Topic tp1", "Otherwise call mint_credential"} {
		if !strings.Contains(pub.calls[0].msg.ReplyHint, want) {
			t.Fatalf("exact topic reply hint %q missing %q", pub.calls[0].msg.ReplyHint, want)
		}
	}
	if strings.Contains(pub.calls[0].msg.ReplyHint, "list_topics") {
		t.Fatalf("exact topic reply hint = %q", pub.calls[0].msg.ReplyHint)
	}
}

func TestIngest_WorkspaceFallbackUsedWithoutExactChannel(t *testing.T) {
	ing, pub, s := newTestIngest(t)
	seedSlackTopic(t, s, "org1", "tp-wide", "sc1", "")
	seedSlackTopic(t, s, "org1", "tp-other", "sc1", "C2")

	if err := ing.OnEvent(context.Background(), "T1", slackcore.Event{Channel: "C1", ChannelType: "channel", User: "U1", Text: "hi"}); err != nil {
		t.Fatalf("OnEvent: %v", err)
	}
	if len(pub.calls) != 1 || pub.calls[0].topicID != "tp-wide" {
		t.Fatalf("fallback publishes = %+v", pub.calls)
	}
}

func TestIngest_MultipleExactChannelTopicsAllReceive(t *testing.T) {
	ing, pub, s := newTestIngest(t)
	seedSlackTopic(t, s, "org1", "tp-exact-1", "sc1", "C1")
	seedSlackTopic(t, s, "org1", "tp-exact-2", "sc1", "C1")
	seedSlackTopic(t, s, "org1", "tp-wide", "sc1", "")

	if err := ing.OnEvent(context.Background(), "T1", slackcore.Event{Channel: "C1", ChannelType: "channel", User: "U1", Text: "hi"}); err != nil {
		t.Fatalf("OnEvent: %v", err)
	}
	got := map[streaming.TopicID]bool{}
	for _, call := range pub.calls {
		got[call.topicID] = true
	}
	if len(pub.calls) != 2 || !got["tp-exact-1"] || !got["tp-exact-2"] {
		t.Fatalf("exact publishes = %+v", pub.calls)
	}
}

func TestReplyHint_TopLevelDMOmitsThreadTS(t *testing.T) {
	hint := replyHint("tp-wide", false, "T1", "D1", "im", "1700.1", "")
	if !strings.Contains(hint, "omit threadId and reply at the DM root") {
		t.Fatalf("top-level DM hint must stay at root: %q", hint)
	}
	if strings.Contains(hint, "threadId=1700.1") {
		t.Fatalf("top-level DM hint must not invent a thread: %q", hint)
	}
}

func TestReplyHint_ThreadedDMUsesIncomingThread(t *testing.T) {
	hint := replyHint("tp-wide", false, "T1", "D1", "im", "1700.2", "1699.9")
	if !strings.Contains(hint, "threadId=1699.9") || !strings.Contains(hint, "conversations.replies with channel=D1 and ts=1699.9") {
		t.Fatalf("threaded DM hint must preserve incoming root: %q", hint)
	}
	if strings.Contains(hint, "omit threadId") {
		t.Fatalf("threaded DM hint must not instruct a root reply: %q", hint)
	}
}

func TestIngest_WrongWorkspaceBinding_NotPublished(t *testing.T) {
	ing, pub, s := newTestIngest(t)
	// Topic bound to a DIFFERENT workspace connection (sc2).
	seedSlackTopic(t, s, "org1", "tp1", "sc2", "")

	if err := ing.OnEvent(context.Background(), "T1", slackcore.Event{Channel: "C1", User: "U1", Text: "hi"}); err != nil {
		t.Fatalf("OnEvent: %v", err)
	}
	if len(pub.calls) != 0 {
		t.Fatalf("topic bound to another workspace must not publish; got %d", len(pub.calls))
	}
}
