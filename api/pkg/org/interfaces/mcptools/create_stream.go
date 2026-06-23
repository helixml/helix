package mcptools

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/google/jsonschema-go/jsonschema"

	"github.com/helixml/helix/api/pkg/org/application/topics"
	"github.com/helixml/helix/api/pkg/org/domain/tool"
	"github.com/helixml/helix/api/pkg/org/domain/transport"
)

// CreateTopic creates a new named Topic. The caller becomes the
// creator. Topic names are unique across the org. The transport
// defaults to "local" — events live in SQLite and reach subscribers
// through the in-process broadcaster and dispatcher. Other transports
// (when implemented) compose external I/O over the same local store.
type CreateTopic struct {
	deps Deps
}

const CreateTopicName tool.Name = "create_topic"

var createTopicSchema = func() *jsonschema.Schema {
	s := mustSchema[createTopicArgs]()
	// transport accepts either the object form or a bare TransportKind
	// string shorthand. Replace the auto-derived object schema with a
	// oneOf so strict-schema MCP clients accept both shapes.
	if t, ok := s.Properties["transport"]; ok {
		object := *t // copy: object shape minus the union wrapper
		object.Type = "object"
		object.Types = nil // pointer field arrived as Types:["object","null"]; Type+Types together is a marshal error
		s.Properties["transport"] = &jsonschema.Schema{
			Description: "Transport for the new Topic. Either a bare string naming the kind (\"local\" / \"webhook\" / \"email\" / \"github\") or an object with kind and optional config.",
			OneOf: []*jsonschema.Schema{
				enumSchema(transport.KindValues(), "Transport kind shorthand."),
				&object,
			},
		}
	}
	return s
}()

func (t *CreateTopic) Name() tool.Name { return CreateTopicName }
func (t *CreateTopic) Description() string {
	return "Create a new named Topic. The caller becomes the creator. Topic names are unique. " +
		"Optional `transport` describes how events on the Topic move to/from the outside world; " +
		"omit it to use the default `local` transport (in-process pub/sub only). " +
		"Valid transport.kind values: \"local\", \"webhook\", \"email\", \"github\". " +
		"Example for an inbound HTTP webhook: " +
		`{"transport":{"kind":"webhook"}}` +
		". Example for a bidirectional webhook with an outbound URL: " +
		`{"transport":{"kind":"webhook","config":{"outbound_url":"https://example.com/in"}}}` +
		"."
}
func (t *CreateTopic) InputSchema() *jsonschema.Schema { return createTopicSchema }

type createTopicArgs struct {
	ID          string                 `json:"id,omitempty"`
	Name        string                 `json:"name"`
	Description string                 `json:"description,omitempty"`
	Transport   *createTopicTransport `json:"transport,omitempty"`
}

type createTopicTransport struct {
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
func (t *createTopicTransport) UnmarshalJSON(data []byte) error {
	if len(data) > 0 && data[0] == '"' {
		var kind transport.Kind
		if err := json.Unmarshal(data, &kind); err != nil {
			return err
		}
		t.Kind = kind
		t.Config = nil
		return nil
	}
	type raw createTopicTransport
	var r raw
	if err := json.Unmarshal(data, &r); err != nil {
		return err
	}
	*t = createTopicTransport(r)
	return nil
}

func (t *CreateTopic) Invoke(ctx context.Context, inv tool.Invocation) (json.RawMessage, error) {
	var args createTopicArgs
	if err := json.Unmarshal(inv.Args, &args); err != nil {
		return nil, fmt.Errorf("parse args: %w", err)
	}
	orgID := inv.Caller.OrganizationID()
	if orgID == "" {
		return nil, fmt.Errorf("create_topic: caller has no OrgID")
	}
	tr := transport.Transport{}
	if args.Transport != nil {
		tr = transport.Transport{
			Kind:   args.Transport.Kind,
			Config: args.Transport.Config,
		}
	}
	s, notice, err := t.deps.Topics.Create(ctx, orgID, topics.CreateParams{
		ID:          args.ID,
		Name:        args.Name,
		Description: args.Description,
		CreatedBy:   inv.Caller.ID(),
		Transport:   tr,
	})
	if err != nil {
		return nil, err
	}
	out := map[string]string{"id": string(s.ID)}
	if notice != "" {
		out["notice"] = notice
	}
	return json.Marshal(out)
}
