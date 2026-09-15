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

// Role gathers a Bot's name and purpose, drafts its markdown, saves it,
// and reports the result. The Bot *is* the role: its content is its prompt
// and its tools are its live MCP surface. All the actual content lives in
// templates/role.md; this file is just the registration shell.
type Role struct{}

func (Role) Name() Name    { return RoleName }
func (Role) Title() string { return "Draft a bot from a brief" }

func (Role) Description() string {
	return "Collects a bot's name and purpose, drafts it, and saves it — e.g. " +
		"`/role Release Manager who owns failed-build triage`."
}

func (Role) Arguments() []Argument {
	return []Argument{{
		Name:        "hint",
		Title:       "Bot brief",
		Description: "The bot's name or role title and its concrete purpose. The assistant asks for whichever part is missing before creating it.",
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
		body += "\n\n---\n\n**Bot brief from the operator:** " + hint +
			"\n\nUse this brief directly if it contains both a name and a concrete purpose. Otherwise ask for the missing part and wait.\n"
	}
	return []Message{{Role: "user", Text: body}}, nil
}
