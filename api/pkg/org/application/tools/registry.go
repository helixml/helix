// Package tools holds the tool registry, the invocation pipeline, and
// built-in tool implementations. The pipeline is scope-agnostic; individual
// tools own their scope shape and enforcement logic.
package tools

import (
	"fmt"
	"sort"

	"github.com/helixml/helix/api/pkg/org/domain/tool"
)

// Registry is an in-memory map of tool name to implementation.
// Built-ins are registered at server startup; MCP or owner-defined tools
// can be added later without changing the registry type.
type Registry struct {
	tools map[tool.Name]tool.Tool
}

// NewRegistry returns an empty Registry.
func NewRegistry() *Registry {
	return &Registry{tools: make(map[tool.Name]tool.Tool)}
}

// Register adds a tool. It fails if another tool is already registered under
// the same name — the owner's map of possible capabilities must be unambiguous.
func (r *Registry) Register(tool tool.Tool) error {
	name := tool.Name()
	if name == "" {
		return fmt.Errorf("tool name is empty")
	}
	if _, exists := r.tools[name]; exists {
		return fmt.Errorf("tool %q already registered", name)
	}
	r.tools[name] = tool
	return nil
}

// Get returns the tool by name, or an error if unknown.
func (r *Registry) Get(name tool.Name) (tool.Tool, error) {
	tool, ok := r.tools[name]
	if !ok {
		return nil, fmt.Errorf("tool %q not registered", name)
	}
	return tool, nil
}

// List returns every registered tool, sorted by name for stable
// rendering. Used by the chart UI's role editor to populate the
// "Tools" multi-select with the catalogue of available tools.
func (r *Registry) List() []tool.Tool {
	out := make([]tool.Tool, 0, len(r.tools))
	for _, t := range r.tools {
		out = append(out, t)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name() < out[j].Name() })
	return out
}
