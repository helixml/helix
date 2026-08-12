// Package messages owns mutations of the events retained on Topics.
package messages

import (
	"context"
	"fmt"

	"github.com/helixml/helix/api/pkg/org/domain/store"
	"github.com/helixml/helix/api/pkg/org/domain/streaming"
)

type Notifier interface {
	Notify(orgID string, topicID streaming.TopicID)
}

type Messages struct {
	topics   store.Topics
	events   store.Events
	notifier Notifier
}

type Deps struct {
	Topics   store.Topics
	Events   store.Events
	Notifier Notifier
}

func New(deps Deps) *Messages {
	return &Messages{topics: deps.Topics, events: deps.Events, notifier: deps.Notifier}
}

// Clear deletes every retained event on one Topic and wakes live readers so
// they immediately receive the empty event list. The Topic itself and all
// subscriptions remain unchanged.
func (m *Messages) Clear(ctx context.Context, orgID string, topicID streaming.TopicID) error {
	if _, err := m.topics.Get(ctx, orgID, topicID); err != nil {
		return fmt.Errorf("get topic: %w", err)
	}
	if err := m.events.DeleteForTopic(ctx, orgID, topicID); err != nil {
		return fmt.Errorf("delete topic messages: %w", err)
	}
	if m.notifier != nil {
		m.notifier.Notify(orgID, topicID)
	}
	return nil
}
