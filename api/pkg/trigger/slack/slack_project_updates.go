package slack

import (
	"context"
	"fmt"

	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/types"
)

// postProjectUpdate - subscribes to project updates and posts them to the Slack channel
func (s *SlackBot) postProjectUpdates(ctx context.Context, app *types.App) error {
	if len(app.Config.Helix.Assistants) == 0 {
		return fmt.Errorf("no assistants found")
	}
	var projectID string

	// Check if Skill is turned on as well
	assistant := app.Config.Helix.Assistants[0]

	if assistant.ProjectManager.Enabled {
		projectID = assistant.ProjectManager.ProjectID
	}

	if projectID == "" {
		return nil
	}

	// Subscribe to project updates
	sub, err := s.store.SubscribeForTasks(ctx, &store.SpecTaskSubscriptionFilter{
		ProjectID: projectID,
	}, func(task *types.SpecTask) error {
		return s.postProjectUpdate(ctx, task)
	})
	if err != nil {
		return fmt.Errorf("failed to subscribe to project updates: %w", err)
	}
	defer sub.Unsubscribe()

	return nil
}

func (s *SlackBot) postProjectUpdate(ctx context.Context, task *types.SpecTask) error {
	return nil
}
