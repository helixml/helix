package tools

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/google/jsonschema-go/jsonschema"

	"github.com/helixml/helix/api/pkg/org/domain/orgchart"
	"github.com/helixml/helix/api/pkg/org/domain/streaming"
	"github.com/helixml/helix/api/pkg/org/domain/tool"
)

// InviteWorkers subscribes one or more other Workers to a Stream. The
// counterpart to subscribe (which is self-only) — used to add others to
// a stream you've created, e.g. opening a DM by creating a stream and
// inviting both parties to it.
type InviteWorkers struct {
	deps Deps
}

const InviteWorkersName tool.Name = "invite_workers"

var inviteWorkersSchema = mustSchema[inviteWorkersArgs]()

func (t *InviteWorkers) Name() tool.Name { return InviteWorkersName }
func (t *InviteWorkers) Description() string {
	return "Subscribe one or more Workers to a Stream. Use this to add others " +
		"to a stream you control — e.g. opening a DM by creating a stream and " +
		"inviting both parties, or pulling a colleague into an existing thread. " +
		"Idempotent per worker: anyone already subscribed is a no-op."
}
func (t *InviteWorkers) InputSchema() *jsonschema.Schema { return inviteWorkersSchema }

type inviteWorkersArgs struct {
	StreamID  string   `json:"streamId"`
	WorkerIDs []string `json:"workerIds"`
}

func (t *InviteWorkers) Invoke(ctx context.Context, inv tool.Invocation) (json.RawMessage, error) {
	var args inviteWorkersArgs
	if err := json.Unmarshal(inv.Args, &args); err != nil {
		return nil, fmt.Errorf("parse args: %w", err)
	}
	if args.StreamID == "" {
		return nil, fmt.Errorf("streamId is required")
	}
	if len(args.WorkerIDs) == 0 {
		return nil, fmt.Errorf("workerIds must contain at least one worker")
	}
	orgID := inv.Caller.OrganizationID()
	if orgID == "" {
		return nil, fmt.Errorf("invite_workers: caller has no OrgID")
	}
	streamID := streaming.StreamID(args.StreamID)
	workerIDs := make([]orgchart.WorkerID, 0, len(args.WorkerIDs))
	for _, raw := range args.WorkerIDs {
		workerIDs = append(workerIDs, orgchart.WorkerID(raw))
	}
	// The service validates the stream + every worker up front and
	// subscribes each (idempotent per worker).
	if err := t.deps.subscriptionsService().Invite(ctx, orgID, streamID, workerIDs); err != nil {
		return nil, err
	}

	workerIDStrings := make([]string, len(workerIDs))
	for i, wid := range workerIDs {
		workerIDStrings[i] = string(wid)
	}
	return json.Marshal(map[string]any{
		"streamId":  string(streamID),
		"workerIds": workerIDStrings,
	})
}
