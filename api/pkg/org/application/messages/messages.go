// Package messages owns mutations of the events retained on a source's
// event stream.
package messages

import (
	"context"
	"fmt"

	"github.com/helixml/helix/api/pkg/org/domain/store"
	"github.com/helixml/helix/api/pkg/org/domain/streaming"
)

type Notifier interface {
	Notify(orgID string, streamID streaming.StreamID)
}

type Messages struct {
	triggers store.Triggers
	events   store.Events
	notifier Notifier
}

type Deps struct {
	Triggers store.Triggers
	Events   store.Events
	Notifier Notifier
}

func New(deps Deps) *Messages {
	return &Messages{triggers: deps.Triggers, events: deps.Events, notifier: deps.Notifier}
}

// Clear deletes every retained event on one Trigger's stream and wakes
// live readers so they immediately receive the empty event list. The
// Trigger itself and all its attachments remain unchanged.
func (m *Messages) Clear(ctx context.Context, orgID, triggerID string) error {
	rows, err := m.triggers.Find(ctx, store.WithOrg(orgID), store.WithID(triggerID), store.WithLimit(1))
	if err != nil {
		return fmt.Errorf("get trigger: %w", err)
	}
	if len(rows) == 0 {
		return fmt.Errorf("trigger %q: %w", triggerID, store.ErrNotFound)
	}
	if err := m.events.DeleteForStream(ctx, orgID, triggerID); err != nil {
		return fmt.Errorf("delete trigger events: %w", err)
	}
	if m.notifier != nil {
		m.notifier.Notify(orgID, triggerID)
	}
	return nil
}
