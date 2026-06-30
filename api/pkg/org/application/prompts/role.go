package prompts

import (
	"context"
	_ "embed"
	"strings"

	"github.com/helixml/helix/api/pkg/org/domain/tool"
)

// RoleName is the slash-command identifier for the bot-design prompt.
// Surfaced as `/role` in MCP clients — the on-disk prompt filename
// (role.md) and slash command are kept for continuity even though the
// concept is now a Bot. Singular verb-less form follows Claude Code's
// own slash-command convention (`/init`, `/review`, `/compact`) — never
// `/new_xxx`.
const RoleName Name = "role"

//go:embed templates/role.md
var roleTemplate string

// Role drafts a fresh Bot markdown from a one-line title hint, saves it
// via create_role without asking permission, then offers in-place
// edits. The Bot *is* the role: its content is its prompt and its tools
// are its live MCP surface. All the actual content lives in
// templates/role.md; this file is just the registration shell.
type Role struct{}

func (Role) Name() Name    { return RoleName }
func (Role) Title() string { return "Draft a bot from a title" }

func (Role) Description() string {
	return "Drafts and saves a new bot from a title — e.g. `/role cto`, " +
		"`/role marketing director`, `/role customer support`. After saving, " +
		"offers edits in-place."
}

func (Role) Arguments() []Argument {
	return []Argument{{
		Name:        "hint",
		Title:       "Bot title",
		Description: "The bot to draft, in plain words — e.g. 'cto', 'marketing director', 'customer support'. The LLM uses this as the seed for the whole markdown.",
		Required:    false,
	}}
}

// RequiresTool gates the prompt on the create_bot tool: a Bot whose
// tools don't list it can't save the result, so surfacing the slash
// command would only produce a dead-end at the very last step.
// The literal (not the tools-package constant) keeps this application
// package free of a dependency on the MCP-tool adapter package;
// "create_bot" is a stable public tool name and RegisterBuiltins fails
// fast at boot if the registered tool name ever drifts from it.
func (Role) RequiresTool() tool.Name { return "create_bot" }

func (Role) Render(_ context.Context, args map[string]string) ([]Message, error) {
	body := roleTemplate
	if hint := strings.TrimSpace(args["hint"]); hint != "" {
		body += "\n\n---\n\n**Bot title from the operator:** " + hint +
			"\n\nDraft from this directly — no interview.\n"
	}
	return []Message{{Role: "user", Text: body}}, nil
}
