package cutover

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/helixml/helix/api/pkg/org/domain/streaming"
	"github.com/helixml/helix/api/pkg/org/domain/transport"
)

func TestTopicToTrigger(t *testing.T) {
	createdAt := time.Date(2026, 8, 21, 9, 30, 0, 0, time.FixedZone("test", 3600))
	tests := []struct {
		name       string
		transport  transport.Transport
		wantConfig string
	}{
		{name: "local strips ignored config", transport: transport.Transport{Kind: transport.KindLocal, Config: json.RawMessage(`{"legacy":true}`)}},
		{name: "webhook strips outbound URL", transport: transport.Transport{Kind: transport.KindWebhook, Config: json.RawMessage(`{"outbound_url":"https://example.com/hook"}`)}},
		{name: "email", transport: transport.Transport{Kind: transport.KindEmail, Config: json.RawMessage(`{"alias":"alerts"}`)}, wantConfig: `{"alias":"alerts"}`},
		{name: "github", transport: transport.Transport{Kind: transport.KindGitHub, Config: json.RawMessage(`{"repo":"helixml/helix","events":["issues"],"branches":["main"],"webhook_id":42}`)}, wantConfig: `{"repo":"helixml/helix","events":["issues"],"branches":["main"],"webhook_id":42}`},
		{name: "gitlab", transport: transport.Transport{Kind: transport.KindGitLab, Config: json.RawMessage(`{"repo":"helixml/helix","repository_id":"123","events":["Push Hook"],"webhook_id":7}`)}, wantConfig: `{"repo":"helixml/helix","repository_id":"123","events":["Push Hook"],"webhook_id":7}`},
		{name: "cron", transport: transport.Transport{Kind: transport.KindCron, Config: json.RawMessage(`{"schedule":"0 9 * * 1","message":"weekly"}`)}, wantConfig: `{"schedule":"0 9 * * 1","message":"weekly"}`},
		{name: "slack retains inbound channel filter", transport: transport.Transport{Kind: transport.KindSlack, Config: json.RawMessage(`{"service_connection_id":"sc-1","channel_id":"C1"}`)}, wantConfig: `{"service_connection_id":"sc-1","channel_id":"C1"}`},
		{name: "helix events strips ignored config", transport: transport.Transport{Kind: transport.KindHelixEvents, Config: json.RawMessage(`{"legacy":true}`)}},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			topic, err := streaming.NewTopic("topic-1", "Alerts", "legacy description", "worker-1", createdAt, tc.transport, "org-1")
			if err != nil {
				t.Fatalf("NewTopic() error = %v", err)
			}
			got, err := TopicToTrigger(topic)
			if err != nil {
				t.Fatalf("TopicToTrigger() error = %v", err)
			}
			if got.ID != "topic-1" || got.OrganizationID != "org-1" || got.Name != "Alerts" || got.CreatedBy != "worker-1" {
				t.Fatalf("TopicToTrigger() identity = %#v", got)
			}
			if !got.CreatedAt.Equal(createdAt) || got.CreatedAt.Location() != time.UTC {
				t.Fatalf("TopicToTrigger() CreatedAt = %v, want %v in UTC", got.CreatedAt, createdAt)
			}
			if got.Kind != tc.transport.Kind {
				t.Fatalf("TopicToTrigger() Kind = %q, want %q", got.Kind, tc.transport.Kind)
			}
			if string(got.Config) != tc.wantConfig {
				t.Fatalf("TopicToTrigger() Config = %s, want %s", got.Config, tc.wantConfig)
			}
		})
	}
}

func TestTopicToTriggerRejectsInvalidTransport(t *testing.T) {
	tests := []transport.Transport{
		{Kind: "unknown"},
		{Kind: transport.KindEmail, Config: json.RawMessage(`{"alias":`)},
		{Kind: transport.KindSlack, Config: json.RawMessage(`{}`)},
	}
	for _, value := range tests {
		topic := streaming.Topic{
			ID:             "topic-1",
			OrganizationID: "org-1",
			Name:           "Alerts",
			CreatedAt:      time.Now(),
			Transport:      value,
		}
		if _, err := TopicToTrigger(topic); err == nil {
			t.Fatalf("TopicToTrigger(%q) error = nil", value.Kind)
		}
	}
}
