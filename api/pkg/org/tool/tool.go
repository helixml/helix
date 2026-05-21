// Package tool owns the Tool concept: a capability the MCP gateway
// exposes to a calling Worker, gated by Grants. Today this package
// only carries the Name type; the Tool interface and Invocation type
// (currently in helix-org/domain) follow in a later migration.
package tool

// Name is the stable identifier for a Tool — used both in `grants`
// rows and as the MCP tool name advertised to LLM callers. Convention
// is snake_case (e.g. `hire_worker`, `read_events`, `publish`).
type Name string
