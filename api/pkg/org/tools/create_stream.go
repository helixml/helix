package tools

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/google/jsonschema-go/jsonschema"

	"github.com/helixml/helix/api/pkg/org/stream"
	"github.com/helixml/helix/api/pkg/org/tool"
	"github.com/helixml/helix/api/pkg/org/transport"
	"github.com/helixml/helix/helix-org/domain"
)

// CreateStream creates a new named Stream. The caller becomes the
// creator. Stream names are unique across the org. The transport
// defaults to "local" — events live in SQLite and reach subscribers
// through the in-process broadcaster and dispatcher. Other transports
// (when implemented) compose external I/O over the same local store.
type CreateStream struct {
	deps Deps
}

const CreateStreamName tool.Name = "create_stream"

var createStreamSchema = func() *jsonschema.Schema {
	s := mustSchema[createStreamArgs]()
	// transport accepts either the object form or a bare TransportKind
	// string shorthand. Replace the auto-derived object schema with a
	// oneOf so strict-schema MCP clients accept both shapes.
	if t, ok := s.Properties["transport"]; ok {
		object := *t // copy: object shape minus the union wrapper
		object.Type = "object"
		object.Types = nil // pointer field arrived as Types:["object","null"]; Type+Types together is a marshal error
		s.Properties["transport"] = &jsonschema.Schema{
			Description: "Transport for the new Stream. Either a bare string naming the kind (\"local\" / \"webhook\" / \"email\" / \"github\") or an object with kind and optional config.",
			OneOf: []*jsonschema.Schema{
				enumSchema(transport.KindValues(), "Transport kind shorthand."),
				&object,
			},
		}
	}
	return s
}()

func (t *CreateStream) Name() tool.Name { return CreateStreamName }
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
	Kind   transport.Kind  `json:"kind"`
	Config json.RawMessage `json:"config,omitempty"`
}

// UnmarshalJSON accepts either the canonical object form
// (`{"kind":"webhook","config":{...}}`) or a bare string shorthand
// (`"webhook"`) that means `{"kind":"webhook"}`. Smaller chat
// models reliably collapse the object to its discriminator string
// once they've seen the `kind` enum on the schema; refusing the
// shorthand just makes them retry. Both shapes are unambiguous and
// mean the same thing.
func (t *createStreamTransport) UnmarshalJSON(data []byte) error {
	if len(data) > 0 && data[0] == '"' {
		var kind transport.Kind
		if err := json.Unmarshal(data, &kind); err != nil {
			return err
		}
		t.Kind = kind
		t.Config = nil
		return nil
	}
	type raw createStreamTransport
	var r raw
	if err := json.Unmarshal(data, &r); err != nil {
		return err
	}
	*t = createStreamTransport(r)
	return nil
}

func (t *CreateStream) Invoke(ctx context.Context, inv domain.Invocation) (json.RawMessage, error) {
	var args createStreamArgs
	if err := json.Unmarshal(inv.Args, &args); err != nil {
		return nil, fmt.Errorf("parse args: %w", err)
	}
	id := stream.ID(args.ID)
	if id == "" {
		id = stream.ID("s-" + t.deps.NewID())
	}
	tr := transport.Transport{}
	if args.Transport != nil {
		tr = transport.Transport{
			Kind:   args.Transport.Kind,
			Config: args.Transport.Config,
		}
	}
	s, err := domain.NewStream(id, args.Name, args.Description, inv.Caller.ID(), t.deps.Now(), tr)
	if err != nil {
		return nil, err
	}
	if err := t.deps.Store.Streams.Create(ctx, s); err != nil {
		return nil, err
	}
	return json.Marshal(map[string]string{"id": string(id)})
}
