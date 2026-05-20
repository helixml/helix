// Package prompts holds the MCP-prompt surface — server-defined
// templates that clients (Claude Code, the helix-org chat UI) surface
// as slash commands. Prompts are scaffolding for the *human* side of
// the org: structured interviews that turn vague intent into well-shaped
// graph mutations through the existing tools (create_role, hire_worker,
// etc.). They never carry behaviour of their own; the LLM consumes the
// rendered messages and dispatches via tools.
package prompts

import (
	"context"

	"github.com/helixml/helix-org/domain"
)

// Name is the identifier MCP clients use to fetch a prompt. Must be
// unique within a registry. Convention: lowercase snake_case
// (`new_role`, `hire_worker`).
type Name string

// Argument describes a single named parameter the client may pass when
// invoking the prompt. Mirrors mcp.PromptArgument so registration is
// a one-to-one copy.
type Argument struct {
	Name        string
	Title       string
	Description string
	Required    bool
}

// Message is one seed turn the prompt contributes to the conversation.
// Role is "user" or "assistant" per the MCP spec; in practice every
// helix-org prompt seeds a single user turn.
type Message struct {
	Role string
	Text string
}

// Prompt is the contract every server-defined prompt satisfies. The
// per-worker MCP server iterates the registry, filters by RequiresTool,
// and registers each survivor as an mcp.Prompt.
type Prompt interface {
	Name() Name
	Title() string
	Description() string
	Arguments() []Argument

	// RequiresTool gates visibility: only Workers holding a grant for
	// the named tool see this prompt. The empty string means visible
	// to every Worker. Gating exists because prompts that end in a
	// tool call are useless to a Worker who can't make that call —
	// surfacing the slash command would just produce a 403 at the end.
	RequiresTool() domain.ToolName

	// Render produces the seed messages for this invocation. The args
	// map is the validated set passed by the MCP client.
	Render(ctx context.Context, args map[string]string) ([]Message, error)
}
