package slack

import (
	"context"
	"fmt"

	goslack "github.com/slack-go/slack"
)

type MessageAPI interface {
	PostMessageContext(context.Context, string, ...goslack.MsgOption) (string, string, error)
}

type DeliveryReceipt struct {
	Destination string
	MessageID   string
}

func DeliverText(ctx context.Context, client MessageAPI, channelID, text, threadID string) (DeliveryReceipt, error) {
	if channelID == "" {
		return DeliveryReceipt{}, fmt.Errorf("Slack outbound destination is not configured")
	}
	options := []goslack.MsgOption{goslack.MsgOptionText(text, false)}
	if threadID != "" {
		options = append(options, goslack.MsgOptionTS(threadID))
	}
	channel, timestamp, err := client.PostMessageContext(ctx, channelID, options...)
	if err != nil {
		return DeliveryReceipt{}, fmt.Errorf("post Slack message: %w", err)
	}
	if timestamp == "" {
		return DeliveryReceipt{}, fmt.Errorf("post Slack message: Slack returned no message timestamp")
	}
	if channel == "" {
		channel = channelID
	}
	return DeliveryReceipt{Destination: channel, MessageID: timestamp}, nil
}
