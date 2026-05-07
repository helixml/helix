package tools

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/google/jsonschema-go/jsonschema"

	"github.com/helixml/helix-org/domain"
)

// CreateStream creates a new named Stream. The caller becomes the
// creator. Stream names are unique across the org. The transport
// defaults to "local" — events live in SQLite and reach subscribers
// through the in-process broadcaster and dispatcher. Other transports
// (when implemented) compose external I/O over the same local store.
type CreateStream struct {
	deps Deps
}

const CreateStreamName domain.ToolName = "create_stream"

var createStreamSchema = mustSchema[createStreamArgs]()

func (t *CreateStream) Name() domain.ToolName { return CreateStreamName }
func (t *CreateStream) Description() string {
	return "Create a new named Stream. The caller becomes the creator. Stream names are unique. " +
		"Optional `transport` describes how events on the Stream move to/from the outside world; " +
		"omit it to use the default `local` transport (in-process pub/sub only). " +
		"Valid transport.kind values: \"local\", \"webhook\", \"email\", \"github\". " +
		"Example for an inbound HTTP webhook: " +
		`{"transport":{"kind":"webhook"}}` +
		". Example for a bidirectional webhook with an outbound URL: " +
		`{"transport":{"kind":"webhook","config":{"outbound_url":"https://example.com/in"}}}` +
		"."
}
func (t *CreateStream) InputSchema() *jsonschema.Schema { return createStreamSchema }

type createStreamArgs struct {
	ID          string                 `json:"id,omitempty"`
	Name        string                 `json:"name"`
	Description string                 `json:"description,omitempty"`
	Transport   *createStreamTransport `json:"transport,omitempty"`
}

type createStreamTransport struct {
	Kind   domain.TransportKind `json:"kind"`
	Config json.RawMessage      `json:"config,omitempty"`
}

func (t *CreateStream) Invoke(ctx context.Context, inv domain.Invocation) (json.RawMessage, error) {
	var args createStreamArgs
	if err := json.Unmarshal(inv.Args, &args); err != nil {
		return nil, fmt.Errorf("parse args: %w", err)
	}
	id := domain.StreamID(args.ID)
	if id == "" {
		id = domain.StreamID("s-" + t.deps.NewID())
	}
	transport := domain.Transport{}
	if args.Transport != nil {
		transport = domain.Transport{
			Kind:   args.Transport.Kind,
			Config: args.Transport.Config,
		}
	}
	s, err := domain.NewStream(id, args.Name, args.Description, inv.Caller.ID(), t.deps.Now(), transport)
	if err != nil {
		return nil, err
	}
	if err := t.deps.Store.Streams.Create(ctx, s); err != nil {
		return nil, err
	}
	return json.Marshal(map[string]string{"id": string(id)})
}
