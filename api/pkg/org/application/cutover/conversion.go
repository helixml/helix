package cutover

import (
	"encoding/json"
	"fmt"

	"github.com/helixml/helix/api/pkg/org/domain/streaming"
	"github.com/helixml/helix/api/pkg/org/domain/transport"
	"github.com/helixml/helix/api/pkg/org/domain/trigger"
)

// TopicToTrigger converts the durable identity and inbound transport state of
// a legacy Topic. Outbound-only configuration is deliberately discarded.
func TopicToTrigger(topic streaming.Topic) (trigger.Trigger, error) {
	config, err := inboundConfig(topic.Transport)
	if err != nil {
		return trigger.Trigger{}, fmt.Errorf("convert topic %q: %w", topic.ID, err)
	}

	converted, err := trigger.New(
		string(topic.ID),
		topic.OrganizationID,
		topic.Name,
		topic.Transport.Kind,
		config,
		topic.CreatedBy,
		topic.CreatedAt,
	)
	if err != nil {
		return trigger.Trigger{}, fmt.Errorf("convert topic %q: %w", topic.ID, err)
	}
	return converted, nil
}

func inboundConfig(value transport.Transport) (json.RawMessage, error) {
	switch value.Kind {
	case transport.KindLocal, transport.KindHelixEvents:
		return nil, nil
	case transport.KindWebhook:
		if _, err := value.WebhookConfig(); err != nil {
			return nil, err
		}
		return nil, nil
	case transport.KindEmail:
		config, err := value.EmailConfig()
		return marshalConfig(config, err)
	case transport.KindGitHub:
		config, err := value.GitHubConfig()
		return marshalConfig(config, err)
	case transport.KindGitLab:
		config, err := value.GitLabConfig()
		return marshalConfig(config, err)
	case transport.KindCron:
		config, err := value.CronConfig()
		return marshalConfig(config, err)
	case transport.KindSlack:
		config, err := value.SlackConfig()
		return marshalConfig(config, err)
	default:
		return nil, fmt.Errorf("unsupported transport kind %q", value.Kind)
	}
}

func marshalConfig(value any, parseErr error) (json.RawMessage, error) {
	if parseErr != nil {
		return nil, parseErr
	}
	encoded, err := json.Marshal(value)
	if err != nil {
		return nil, fmt.Errorf("marshal inbound transport config: %w", err)
	}
	return encoded, nil
}
