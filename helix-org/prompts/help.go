package prompts

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/helixml/helix/api/pkg/org/tool"
)

// HelpName is the slash-command identifier for the help-listing prompt.
const HelpName Name = "help"

// Help is a self-introspecting slash command: when invoked it walks
// the very Registry it was registered on and produces a markdown list
// of every other prompt visible at that moment, each with a one-line
// description.
//
// Holding a pointer to its own registry is the whole trick — it means
// adding a new prompt anywhere in the codebase automatically lights up
// in `/help` without anyone touching this file. The reference is taken
// at registration time but resolved lazily on every invocation, so the
// listing always reflects the registry's current state, not a snapshot.
type Help struct {
	reg *Registry
}

// NewHelp constructs a Help bound to the given registry. The registry
// pointer is captured so Render can iterate it later — register Help
// last (after the prompts it should advertise) and you're done.
func NewHelp(reg *Registry) Help { return Help{reg: reg} }

func (Help) Name() Name    { return HelpName }
func (Help) Title() string { return "Available slash commands" }
func (Help) Description() string {
	return "Lists every slash command this chat surface knows about, with a one-line " +
		"description of each. Auto-generated from the prompt registry — adding a new " +
		"prompt automatically adds it here."
}

func (Help) Arguments() []Argument { return nil }

// RequiresTool returns the empty string so every Worker sees `/help`
// regardless of grants. There is no tool to gate against — `/help`
// reads the registry, never mutates anything.
func (Help) RequiresTool() tool.Name { return "" }

func (h Help) Render(_ context.Context, _ map[string]string) ([]Message, error) {
	all := h.reg.All()
	sort.Slice(all, func(i, j int) bool { return all[i].Name() < all[j].Name() })

	var sb strings.Builder
	// Stage directions for the LLM. Without these the model paraphrases
	// or adds chatty commentary; we want a clean, deterministic listing.
	sb.WriteString("The user typed `/help`. Reply with **this exact markdown listing** — no preamble, no paraphrasing, no extra commentary:\n\n")
	sb.WriteString("---\n\n")
	sb.WriteString("**Available slash commands**\n\n")
	for _, p := range all {
		fmt.Fprintf(&sb, "- `/%s` — %s\n", p.Name(), p.Description())
	}
	return []Message{{Role: "user", Text: sb.String()}}, nil
}
