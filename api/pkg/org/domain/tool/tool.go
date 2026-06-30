// Package tool owns the Tool concept: an MCP tool the gateway exposes
// to a calling Bot. Which tools a Bot sees is derived live from its
// Bot.Tools list; there is no separate per-Bot permission record. The
// package carries the Name typed identifier (used both in Bot.Tools and
// as the MCP tool name advertised to LLM callers), the Tool interface
// every implementation satisfies, and the Invocation envelope passed to
// Tool.Invoke.
//
// Lifted from api/pkg/org/tool and api/pkg/org/domain/tool.go in the
// DDD restructure.
//
// Cycle break: this package intentionally does NOT import
// api/pkg/org/domain/orgchart. Invocation.Caller is typed as the local
// Caller interface (a minimal subset — ID and OrganizationID) so a thin
// adapter over orgchart.Bot satisfies it structurally without orgchart
// having to import tool. This pattern keeps the dependency DAG one-way
// (orgchart -> tool, never back), which matters because orgchart.Bot.Tools
// is []tool.Name.
package tool

import (
	"context"
	"encoding/json"

	"github.com/google/jsonschema-go/jsonschema"
)

// Name is the stable identifier for a Tool — used both in Bot.Tools
// and as the MCP tool name advertised to LLM callers. Convention
// is snake_case (e.g. `create_bot`, `read_events`, `publish`).
type Name = string

// Caller is the minimal caller surface a Tool depends on. A thin
// adapter over orgchart.Bot (exposing the bot's ID and OrganizationID)
// satisfies this; defining it locally here keeps the tool package free
// of an orgchart import.
type Caller interface {
	ID() string
	OrganizationID() string
}

// Invocation bundles the per-call data passed to Tool.Invoke. The
// pipeline populates Caller from the MCP request; tools parse Args
// according to their own input schema.
type Invocation struct {
	Caller Caller
	Args   json.RawMessage
}

// Tool is the generic unit of capability. Tools are exposed to callers
// over MCP — Description and InputSchema feed the MCP `tools/list`
// response, and Invoke handles `tools/call`. Built-in structural tools,
// owner-defined tools, and any future MCP-shaped tools all implement
// this interface.
type Tool interface {
	// Name is the stable identifier used in Bot.Tools and MCP tool calls.
	Name() Name

	// Description is a human-readable summary the LLM sees when
	// deciding whether to call this tool.
	Description() string

	// InputSchema is the JSON Schema for Invoke's args, used by MCP
	// clients to validate calls and by LLMs to understand the call shape.
	InputSchema() *jsonschema.Schema

	// Invoke executes the tool. The tool appearing in the caller's
	// Bot.Tools is the entire authorisation — the tool does not
	// re-check the caller's scope because there is no scope.
	Invoke(ctx context.Context, inv Invocation) (json.RawMessage, error)
}
