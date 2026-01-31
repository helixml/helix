package shared

import (
	"context"
	"time"

	"github.com/helixml/helix/api/pkg/data"
	"github.com/helixml/helix/api/pkg/system"
	"github.com/helixml/helix/api/pkg/types"
)

type TriggerSession struct {
	Session *types.Session
}

func NewTriggerSession(_ context.Context, triggerName string, app *types.App) *TriggerSession {

	// Prepare new session
	session := &types.Session{
		ID:             system.GenerateSessionID(),
		Trigger:        triggerName,
		Name:           triggerName,
		Created:        time.Now(),
		Updated:        time.Now(),
		Mode:           types.SessionModeInference,
		Type:           types.SessionTypeText,
		ParentApp:      app.ID,
		OrganizationID: app.OrganizationID,
		Owner:          app.Owner,
		OwnerType:      app.OwnerType,
		Metadata: types.SessionMetadata{
			Stream:       false,
			SystemPrompt: "",
			AssistantID:  "",
			HelixVersion: data.GetHelixVersion(),
		},
	}

	return &TriggerSession{
		Session: session,
	}
}
