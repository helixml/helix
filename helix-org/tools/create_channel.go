package tools

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/google/jsonschema-go/jsonschema"

	"github.com/helixml/helix-org/domain"
)

// CreateChannel creates a new named Channel. The caller becomes the creator.
// Channel names are unique across the org.
type CreateChannel struct {
	deps Deps
}

const CreateChannelName domain.ToolName = "create_channel"

var createChannelSchema = mustSchema[createChannelArgs]()

func (t *CreateChannel) Name() domain.ToolName { return CreateChannelName }
func (t *CreateChannel) Description() string {
	return "Create a new named Channel. The caller becomes the creator. Channel names are unique."
}
func (t *CreateChannel) InputSchema() *jsonschema.Schema { return createChannelSchema }

type createChannelArgs struct {
	ID          string `json:"id,omitempty"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
}

func (t *CreateChannel) Invoke(ctx context.Context, inv domain.Invocation) (json.RawMessage, error) {
	var args createChannelArgs
	if err := json.Unmarshal(inv.Args, &args); err != nil {
		return nil, fmt.Errorf("parse args: %w", err)
	}
	id := domain.ChannelID(args.ID)
	if id == "" {
		id = domain.ChannelID("c-" + t.deps.NewID())
	}
	ch, err := domain.NewChannel(id, args.Name, args.Description, inv.Caller.ID(), t.deps.Now())
	if err != nil {
		return nil, err
	}
	if err := t.deps.Store.Channels.Create(ctx, ch); err != nil {
		return nil, err
	}
	return json.Marshal(map[string]string{"id": string(id)})
}
