package slack

import (
	"context"
	"testing"

	"github.com/helixml/helix/api/pkg/org/infrastructure/persistence/memory"
	slackcore "github.com/helixml/helix/api/pkg/serviceconnection/slack"
)

// A delivery verified against one global Slack app's signing secret must not
// be routed into a workspace that was installed from a different app, even
// when its team_id matches: team_id is payload content, the signature is not.
func newAppBoundIngest(t *testing.T) (*Ingest, *recordingPublisher) {
	t.Helper()
	s := memory.New()
	pub := &recordingPublisher{}
	ws := fakeWorkspaces{team: "T1", ws: Workspace{ID: "sc1", OrgID: "org1", TeamID: "T1", BotToken: "xoxb-x", AppConnectionID: "app-A"}}
	ing := NewIngest(ws, s, pub, nil)
	seedSlackTrigger(t, s, "org1", "tp1", "sc1", "")
	return ing, pub
}

func TestIngest_SignedByOtherApp_Dropped(t *testing.T) {
	ing, pub := newAppBoundIngest(t)
	ctx := slackcore.WithVerifiedAppID(context.Background(), "app-B")
	if err := ing.OnEvent(ctx, "T1", slackcore.Event{Channel: "C1", User: "U1", Text: "hi", TS: "1700.1"}); err != nil {
		t.Fatalf("OnEvent: %v", err)
	}
	if len(pub.calls) != 0 {
		t.Fatalf("delivery signed by another app must not publish; got %d calls", len(pub.calls))
	}
}

func TestIngest_SignedByInstallingApp_Published(t *testing.T) {
	ing, pub := newAppBoundIngest(t)
	ctx := slackcore.WithVerifiedAppID(context.Background(), "app-A")
	if err := ing.OnEvent(ctx, "T1", slackcore.Event{Channel: "C1", User: "U1", Text: "hi", TS: "1700.1"}); err != nil {
		t.Fatalf("OnEvent: %v", err)
	}
	if len(pub.calls) != 1 {
		t.Fatalf("delivery signed by the installing app must publish once; got %d calls", len(pub.calls))
	}
}
