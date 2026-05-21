package domain

import (
	"context"
	"encoding/json"

	"github.com/google/jsonschema-go/jsonschema"

	"github.com/helixml/helix/api/pkg/org/tool"
)

// Invocation bundles the per-call data passed to Tool.Invoke. The
// pipeline populates it from the caller's grant; tools parse Args
// according to their own input schema.
type Invocation struct {
	Caller Worker
	Args   json.RawMessage
}

// Tool is the generic unit of capability. Tools are exposed to callers
// over MCP — Description and InputSchema feed the MCP `tools/list`
// response, and Invoke handles `tools/call`. Built-in structural tools,
// owner-defined tools, and any future MCP-shaped tools all implement
// this interface.
type Tool interface {
	// Name is the stable identifier used in grants and MCP tool calls.
	Name() tool.Name

	// Description is a human-readable summary the LLM sees when
	// deciding whether to call this tool.
	Description() string

	// InputSchema is the JSON Schema for Invoke's args, used by MCP
	// clients to validate calls and by LLMs to understand the call shape.
	InputSchema() *jsonschema.Schema

	// Invoke executes the tool. Holding the grant is the entire
	// authorisation — the tool does not re-check the caller's scope
	// because there is no scope.
	Invoke(ctx context.Context, inv Invocation) (json.RawMessage, error)
}
