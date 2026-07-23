package mcptools

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/helixml/helix/api/pkg/org/domain/streaming"
	"github.com/helixml/helix/api/pkg/org/domain/transport"
)

func TestTopicViewExposesOnlyTypedSlackConfig(t *testing.T) {
	slackView, err := json.Marshal(topicViewOf(streaming.Topic{
		Transport: transport.Transport{
			Kind:   transport.KindSlack,
			Config: json.RawMessage(`{"service_connection_id":"sc-1","channel_id":"C1","token":"must-not-leak"}`),
		},
	}))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(slackView), "must-not-leak") || strings.Contains(string(slackView), `"token"`) {
		t.Fatalf("Slack topic view leaked raw config: %s", slackView)
	}
	if !strings.Contains(string(slackView), `"service_connection_id":"sc-1"`) || !strings.Contains(string(slackView), `"channel_id":"C1"`) {
		t.Fatalf("Slack topic view missing routing config: %s", slackView)
	}

	webhookView, err := json.Marshal(topicViewOf(streaming.Topic{
		Transport: transport.Transport{
			Kind:   transport.KindWebhook,
			Config: json.RawMessage(`{"outbound_url":"https://example.com","token":"must-not-leak"}`),
		},
	}))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(webhookView), "transportConfig") || strings.Contains(string(webhookView), "must-not-leak") {
		t.Fatalf("non-Slack topic view exposed raw config: %s", webhookView)
	}
}
