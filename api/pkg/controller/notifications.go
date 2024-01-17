package controller

import (
	"context"

	"github.com/helixml/helix/api/pkg/notification"
	"github.com/helixml/helix/api/pkg/types"
)

func (c *Controller) notifiyFinetuningComplete(ctx context.Context, session *types.Session) error {
	return c.Options.Notifier.Notify(ctx, &notification.Notification{
		Event:   notification.EventFinetuningComplete,
		Session: session,
	})
}
