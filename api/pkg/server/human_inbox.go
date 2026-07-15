package server

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/slack-go/slack"

	"github.com/helixml/helix/api/pkg/org/application/slackrouting"
	"github.com/helixml/helix/api/pkg/org/domain/orgchart"
	"github.com/helixml/helix/api/pkg/org/domain/processor"
	slacktransport "github.com/helixml/helix/api/pkg/org/infrastructure/transports/slack"
	slackcore "github.com/helixml/helix/api/pkg/serviceconnection/slack"
	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/system"
	"github.com/helixml/helix/api/pkg/types"
)

// humanInbox delivers ask_human messages through Helix or Slack according to
// the person's preferred contact identity.
type humanInbox struct {
	store             store.Store
	slackWorkspaces   slackWorkspaceResolver
	slackClient       func(string) slackAPI
	threadFollower    slackThreadRecorder
	ensureSlackRouter func(context.Context, string, processor.ProcessorID, string) error
}

type slackWorkspaceResolver interface {
	resolveForOrg(context.Context, string, string) (slacktransport.Workspace, error)
}

type slackAPI interface {
	OpenConversationContext(context.Context, *slack.OpenConversationParameters) (*slack.Channel, bool, bool, error)
	PostMessageContext(context.Context, string, ...slack.MsgOption) (string, string, error)
}

type slackThreadRecorder interface {
	RecordParticipant(context.Context, string, processor.ProcessorID, string, string) error
	RecordDMRecipient(context.Context, string, processor.ProcessorID, string, string) error
}

func validateSlackReplyRouter(router processor.Processor, workspaceID, workerID string) error {
	if !router.Automated() || router.Kind != processor.KindFilter || router.InputTopicID != slackWorkspaceTopicID(workspaceID) {
		return fmt.Errorf("processor is not the workspace's automated Slack router")
	}
	if !slackrouting.ThreadFollowEnabled(router.Config) {
		return fmt.Errorf("thread follow is disabled")
	}
	for _, output := range router.Outputs {
		if output.ManagedFor == workerID {
			return nil
		}
	}
	return fmt.Errorf("router has no route for worker %q", workerID)
}

func (h humanInbox) Deliver(ctx context.Context, orgID string, person orgchart.Bot, fromBotID, fromBotName, message string, expectsReply bool) (string, error) {
	switch person.Identity["preferred_contact"] {
	case "", "helix":
		if err := h.notifyHelix(ctx, orgID, person.HelixUserID, fromBotID, fromBotName, person.Name, message, expectsReply); err != nil {
			return "", err
		}
		return "helix", nil
	case "slack":
		return h.deliverSlack(ctx, orgID, person, fromBotID, fromBotName, message, expectsReply)
	default:
		return "", fmt.Errorf("unsupported preferred contact %q", person.Identity["preferred_contact"])
	}
}

func (h humanInbox) notifyHelix(ctx context.Context, orgID, userID, fromBotID, fromBotName, personName, message string, expectsReply bool) error {
	if userID == "" {
		return fmt.Errorf("person has no linked Helix user")
	}
	id := system.GenerateAttentionEventID()
	metaMap := map[string]string{"bot_id": fromBotID, "person": personName}
	if !expectsReply {
		// Delivered as a read-only FYI — the UI shows no Respond affordance.
		metaMap["no_reply"] = "true"
	}
	meta, _ := json.Marshal(metaMap)
	ev := &types.AttentionEvent{
		ID:             id,
		UserID:         userID,
		OrganizationID: orgID,
		EventType:      types.AttentionEventOrgMessage,
		Title:          fmt.Sprintf("Message from %s", fromBotName),
		Description:    message,
		CreatedAt:      time.Now(),
		// Unique key per message so every ask_human is a distinct inbox item
		// (CreateAttentionEvent dedupes on idempotency_key).
		IdempotencyKey: id,
		Metadata:       meta,
	}
	if _, err := h.store.CreateAttentionEvent(ctx, ev); err != nil {
		return fmt.Errorf("create attention event: %w", err)
	}
	return nil
}

func (h humanInbox) deliverSlack(ctx context.Context, orgID string, person orgchart.Bot, fromBotID, fromBotName, message string, expectsReply bool) (string, error) {
	userID := person.Identity["slack_user_id"]
	if userID == "" {
		return "", fmt.Errorf("person has no slack_user_id")
	}
	if h.slackWorkspaces == nil {
		return "", fmt.Errorf("Slack delivery is not configured")
	}
	workspace, err := h.slackWorkspaces.resolveForOrg(ctx, orgID, person.Identity["slack_team_id"])
	if err != nil {
		return "", fmt.Errorf("resolve Slack workspace: %w", err)
	}
	routerID := slackAutoRouterID(workspace.ID)
	if expectsReply {
		if h.ensureSlackRouter == nil {
			return "", fmt.Errorf("Slack reply routing is not configured")
		}
		if err := h.ensureSlackRouter(ctx, orgID, routerID, fromBotID); err != nil {
			return "", fmt.Errorf("Slack reply router %q is unavailable: %w", routerID, err)
		}
		if h.threadFollower == nil {
			return "", fmt.Errorf("Slack reply routing is not configured")
		}
	}
	clientFactory := h.slackClient
	if clientFactory == nil {
		clientFactory = func(token string) slackAPI { return slackcore.New(token, "") }
	}
	client := clientFactory(workspace.BotToken)
	channelID := person.Identity["slack_channel_id"]
	delivered := "slack_channel"
	if channelID == "" {
		channel, _, _, err := client.OpenConversationContext(ctx, &slack.OpenConversationParameters{Users: []string{userID}})
		if err != nil {
			return "", fmt.Errorf("open Slack DM: %w", err)
		}
		if channel == nil || channel.ID == "" {
			return "", fmt.Errorf("open Slack DM: Slack returned no channel")
		}
		channelID = channel.ID
		delivered = "slack_dm"
	} else {
		message = fmt.Sprintf("<@%s> %s", userID, message)
	}
	text := fmt.Sprintf("*Message from %s*\n%s", fromBotName, message)
	if expectsReply && delivered == "slack_channel" {
		text += "\n\nReply in this thread to respond."
	} else if expectsReply {
		text += "\n\nReply here to respond."
	}
	_, timestamp, err := client.PostMessageContext(ctx, channelID, slack.MsgOptionText(text, false))
	if err != nil {
		return "", fmt.Errorf("post Slack message: %w", err)
	}
	if expectsReply {
		if timestamp == "" {
			return "", fmt.Errorf("post Slack message: Slack returned no message timestamp for reply routing")
		}
		if err := h.threadFollower.RecordParticipant(ctx, orgID, routerID, timestamp, fromBotID); err != nil {
			return "", fmt.Errorf("register Slack reply routing: %w", err)
		}
		if delivered == "slack_dm" {
			if err := h.threadFollower.RecordDMRecipient(ctx, orgID, routerID, channelID, fromBotID); err != nil {
				return "", fmt.Errorf("register Slack DM reply routing: %w", err)
			}
		}
	}
	return delivered, nil
}

// NotifyInfo writes an informational, no-reply message to a person's inbox (the
// notification bell), e.g. "the Chief of Staff is starting up". The metadata carries no_reply so the UI
// renders it as a read-only status, not a message to answer. Best-effort at the
// call site: a failure must not block whatever it hangs off (e.g. org create).
func (h humanInbox) NotifyInfo(ctx context.Context, orgID, userID, fromBot, title, message string) error {
	if userID == "" {
		return fmt.Errorf("person has no linked Helix user")
	}
	id := system.GenerateAttentionEventID()
	meta, _ := json.Marshal(map[string]string{"bot_id": fromBot, "no_reply": "true"})
	ev := &types.AttentionEvent{
		ID:             id,
		UserID:         userID,
		OrganizationID: orgID,
		EventType:      types.AttentionEventOrgMessage,
		Title:          title,
		Description:    message,
		CreatedAt:      time.Now(),
		IdempotencyKey: id,
		Metadata:       meta,
	}
	if _, err := h.store.CreateAttentionEvent(ctx, ev); err != nil {
		return fmt.Errorf("create attention event: %w", err)
	}
	return nil
}
