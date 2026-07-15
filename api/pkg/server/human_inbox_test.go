package server

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/slack-go/slack"
	"go.uber.org/mock/gomock"

	"github.com/helixml/helix/api/pkg/org/application/slackrouting"
	"github.com/helixml/helix/api/pkg/org/domain/orgchart"
	"github.com/helixml/helix/api/pkg/org/domain/processor"
	slacktransport "github.com/helixml/helix/api/pkg/org/infrastructure/transports/slack"
	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/types"
)

// TestHumanInboxNotifyExpectsReply verifies expectsReply controls the no_reply
// metadata flag the frontend keys off to show/hide the Respond affordance.
func TestHumanInboxNotifyExpectsReply(t *testing.T) {
	cases := []struct {
		name         string
		expectsReply bool
		wantNoReply  bool
	}{
		{"reply expected -> no no_reply flag", true, false},
		{"fyi -> no_reply flag set", false, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()
			mockStore := store.NewMockStore(ctrl)

			var captured *types.AttentionEvent
			mockStore.EXPECT().
				CreateAttentionEvent(gomock.Any(), gomock.Any()).
				DoAndReturn(func(_ context.Context, ev *types.AttentionEvent) (*types.AttentionEvent, error) {
					captured = ev
					return ev, nil
				})

			h := humanInbox{store: mockStore}
			person := orgchart.Bot{HelixUserID: "usr_1", Name: "Priya"}
			if _, err := h.Deliver(context.Background(), "org_1", person, "chief-of-staff", "Chief of Staff", "hi", tc.expectsReply); err != nil {
				t.Fatalf("Deliver: %v", err)
			}

			var meta map[string]string
			if err := json.Unmarshal(captured.Metadata, &meta); err != nil {
				t.Fatalf("unmarshal metadata: %v", err)
			}
			_, hasNoReply := meta["no_reply"]
			if hasNoReply != tc.wantNoReply {
				t.Fatalf("no_reply present = %v, want %v (metadata=%s)", hasNoReply, tc.wantNoReply, captured.Metadata)
			}
			if meta["bot_id"] != "chief-of-staff" {
				t.Fatalf("bot_id = %q, want chief-of-staff", meta["bot_id"])
			}
		})
	}
}

type fakeSlackWorkspaces struct {
	workspace slacktransport.Workspace
	err       error
}

func (f fakeSlackWorkspaces) resolveForOrg(context.Context, string, string) (slacktransport.Workspace, error) {
	return f.workspace, f.err
}

type fakeSlackAPI struct {
	openedUser string
	postedTo   string
	postTS     string
	err        error
}

func (f *fakeSlackAPI) OpenConversationContext(_ context.Context, params *slack.OpenConversationParameters) (*slack.Channel, bool, bool, error) {
	f.openedUser = params.Users[0]
	if f.err != nil {
		return nil, false, false, f.err
	}
	return &slack.Channel{GroupConversation: slack.GroupConversation{Conversation: slack.Conversation{ID: "D123"}}}, false, false, nil
}

func (f *fakeSlackAPI) PostMessageContext(_ context.Context, channelID string, _ ...slack.MsgOption) (string, string, error) {
	f.postedTo = channelID
	return channelID, f.postTS, f.err
}

type fakeThreadRecorder struct {
	router processor.ProcessorID
	root   string
	worker string
	err    error
}

func (f *fakeThreadRecorder) RecordParticipant(_ context.Context, _ string, router processor.ProcessorID, root, worker string) error {
	f.router, f.root, f.worker = router, root, worker
	return f.err
}

func TestHumanDeliverySlackDMAndChannel(t *testing.T) {
	for _, tc := range []struct {
		name        string
		channelID   string
		wantRoute   string
		wantChannel string
	}{
		{"dm", "", "slack_dm", "D123"},
		{"channel", "C123", "slack_channel", "C123"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			api := &fakeSlackAPI{postTS: "1700000000.000100"}
			recorder := &fakeThreadRecorder{}
			h := humanInbox{
				slackWorkspaces: fakeSlackWorkspaces{workspace: slacktransport.Workspace{ID: "conn-1", BotToken: "xoxb-test"}},
				slackClient:     func(string) slackAPI { return api },
				threadFollower:  recorder,
				ensureSlackRouter: func(context.Context, string, processor.ProcessorID, string) error {
					return nil
				},
			}
			person := orgchart.Bot{Identity: map[string]string{
				"preferred_contact": "slack", "slack_user_id": "U123", "slack_channel_id": tc.channelID,
			}}
			route, err := h.Deliver(context.Background(), "org-1", person, "b-sender", "Sender", "hello", true)
			if err != nil {
				t.Fatal(err)
			}
			if route != tc.wantRoute || api.postedTo != tc.wantChannel {
				t.Fatalf("route/channel = %q/%q, want %q/%q", route, api.postedTo, tc.wantRoute, tc.wantChannel)
			}
			if tc.channelID == "" && api.openedUser != "U123" {
				t.Fatalf("opened user = %q", api.openedUser)
			}
			if recorder.router != "p-slack-router-conn-1" || recorder.root != api.postTS || recorder.worker != "b-sender" {
				t.Fatalf("thread registration = %#v", recorder)
			}
		})
	}
}

func TestHumanDeliverySlackDoesNotFallback(t *testing.T) {
	h := humanInbox{slackWorkspaces: fakeSlackWorkspaces{err: errors.New("not installed")}}
	person := orgchart.Bot{HelixUserID: "usr-1", Identity: map[string]string{
		"preferred_contact": "slack", "slack_user_id": "U123",
	}}
	if _, err := h.Deliver(context.Background(), "org-1", person, "b-sender", "Sender", "hello", false); err == nil {
		t.Fatal("Slack failure must not fall back to Helix")
	}
}

func TestHumanDeliverySlackRequiresReplyRouterBeforePosting(t *testing.T) {
	api := &fakeSlackAPI{postTS: "1700000000.000100"}
	h := humanInbox{
		slackWorkspaces: fakeSlackWorkspaces{workspace: slacktransport.Workspace{ID: "conn-1", BotToken: "xoxb-test"}},
		slackClient:     func(string) slackAPI { return api },
		ensureSlackRouter: func(context.Context, string, processor.ProcessorID, string) error {
			return errors.New("missing")
		},
	}
	person := orgchart.Bot{Identity: map[string]string{"preferred_contact": "slack", "slack_user_id": "U123"}}
	if _, err := h.Deliver(context.Background(), "org-1", person, "b-sender", "Sender", "hello", true); err == nil {
		t.Fatal("expected missing reply router error")
	}
	if api.postedTo != "" {
		t.Fatal("message posted before reply routing was validated")
	}
}

func TestValidateSlackReplyRouter(t *testing.T) {
	router := processor.Processor{
		ID: "p-slack-router-conn-1", InputTopicID: "s-slack-ws-conn-1", Kind: processor.KindFilter,
		CreatedBy: processor.SystemActor, Config: slackrouting.DefaultConfig(), Outputs: []processor.Output{{ManagedFor: "b-sender"}},
	}
	if err := validateSlackReplyRouter(router, "conn-1", "b-sender"); err != nil {
		t.Fatal(err)
	}
	router.Config = nil
	if err := validateSlackReplyRouter(router, "conn-1", "b-sender"); err == nil {
		t.Fatal("disabled thread follow should be rejected")
	}
	router.Config = slackrouting.DefaultConfig()
	router.CreatedBy = "human"
	if err := validateSlackReplyRouter(router, "conn-1", "b-sender"); err == nil {
		t.Fatal("non-automated processor should be rejected")
	}
	router.CreatedBy = processor.SystemActor
	if err := validateSlackReplyRouter(router, "conn-1", "b-other"); err == nil {
		t.Fatal("missing worker route should be rejected")
	}
}
