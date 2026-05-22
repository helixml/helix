package prompts

import (
	"context"
	_ "embed"
	"strings"

	"github.com/helixml/helix/api/pkg/org/tool"
	"github.com/helixml/helix/helix-org/tools"
)

// RoleName is the slash-command identifier for the role-design prompt.
// Surfaced as `/role` in MCP clients. Singular verb-less form follows
// Claude Code's own slash-command convention (`/init`, `/review`,
// `/compact`) — never `/new_xxx`.
const RoleName Name = "role"

//go:embed templates/role.md
var roleTemplate string

// Role drafts a fresh Role markdown from a one-line title hint, saves
// it via create_role without asking permission, then offers in-place
// edits or chains into hire_worker. All the actual content lives in
// templates/role.md; this file is just the registration shell.
type Role struct{}

func (Role) Name() Name    { return RoleName }
func (Role) Title() string { return "Draft a Role from a title" }

func (Role) Description() string {
	return "Drafts and saves a new Role from a title — e.g. `/role cto`, " +
		"`/role marketing director`, `/role customer support`. After saving, " +
		"offers edits in-place or hires someone into it."
}

func (Role) Arguments() []Argument {
	return []Argument{{
		Name:        "hint",
		Title:       "Role title",
		Description: "The Role to draft, in plain words — e.g. 'cto', 'marketing director', 'customer support'. The LLM uses this as the seed for the whole markdown.",
		Required:    false,
	}}
}

// RequiresTool gates the prompt on the create_role grant: a Worker
// without it can't save the result, so surfacing the slash command
// would only produce a dead-end at the very last step.
func (Role) RequiresTool() tool.Name { return tools.CreateRoleName }

func (Role) Render(_ context.Context, args map[string]string) ([]Message, error) {
	body := roleTemplate
	if hint := strings.TrimSpace(args["hint"]); hint != "" {
		body += "\n\n---\n\n**Role title from the operator:** " + hint +
			"\n\nDraft from this directly — no interview.\n"
	}
	return []Message{{Role: "user", Text: body}}, nil
}
