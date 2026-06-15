package prompts

import "fmt"

// Registry is an in-memory map of Name to Prompt. The shape mirrors
// tools.Registry deliberately: same lifecycle (built once at startup,
// read concurrently per request) and same "addable without changing
// core" property — wiring a new prompt is one Register call, not a
// switch in the server.
type Registry struct {
	prompts map[Name]Prompt
}

// NewRegistry returns an empty registry.
func NewRegistry() *Registry {
	return &Registry{prompts: make(map[Name]Prompt)}
}

// Register adds a prompt. Duplicate names fail loudly: a prompt is a
// slash command, and ambiguous slash commands are user-hostile.
func (r *Registry) Register(p Prompt) error {
	name := p.Name()
	if name == "" {
		return fmt.Errorf("prompt name is empty")
	}
	if _, exists := r.prompts[name]; exists {
		return fmt.Errorf("prompt %q already registered", name)
	}
	r.prompts[name] = p
	return nil
}

// Get returns the prompt by name, or an error if unknown.
func (r *Registry) Get(name Name) (Prompt, error) {
	p, ok := r.prompts[name]
	if !ok {
		return nil, fmt.Errorf("prompt %q not registered", name)
	}
	return p, nil
}

// All returns every registered prompt. Order is not guaranteed and
// callers must not assume it.
func (r *Registry) All() []Prompt {
	out := make([]Prompt, 0, len(r.prompts))
	for _, p := range r.prompts {
		out = append(out, p)
	}
	return out
}
