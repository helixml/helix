package prompts

import (
	"context"
	_ "embed"
	"strings"

	"github.com/helixml/helix/api/pkg/org/domain/tool"
)

// PMBotName is the slash-command identifier for the project-manager-bot
// design prompt — surfaced as `/pm-bot`. It drafts an org-wide bot that
// manages the spec tasks of other projects in its org, following the
// discover → draft → connect flow in templates/pm_bot.md.
const PMBotName Name = "pm-bot"

//go:embed templates/pm_bot.md
var pmBotTemplate string

// PMBot drafts and saves a project-manager bot. Like Role it is a thin
// registration shell — all the guidance lives in templates/pm_bot.md. It
// exists as its own prompt (rather than folding into /role) because a PM
// bot has a specific, repeatable shape: discover projects, grant the
// spec-task + discovery tools, subscribe to the projects' spec-task topics.
type PMBot struct{}

func (PMBot) Name() Name    { return PMBotName }
func (PMBot) Title() string { return "Draft a project-manager bot" }

func (PMBot) Description() string {
	return "Drafts and saves an org-wide project-manager bot that watches one or " +
		"more projects and drives their spec tasks. Lists the org's projects, asks " +
		"which to manage, grants the spec-task tools, and subscribes to their events."
}

func (PMBot) Arguments() []Argument {
	return []Argument{{
		Name:        "hint",
		Title:       "Projects or focus",
		Description: "Optional: the projects to manage or the bot's focus in plain words. The LLM uses this to skip the discovery question.",
		Required:    false,
	}}
}

// RequiresTool gates the prompt on create_bot: without it the drafted bot
// can't be saved, so surfacing the slash command would dead-end. Mirrors
// Role; the literal keeps this package free of the MCP-tool adapter dep.
func (PMBot) RequiresTool() tool.Name { return "create_bot" }

func (PMBot) Render(_ context.Context, args map[string]string) ([]Message, error) {
	body := pmBotTemplate
	if hint := strings.TrimSpace(args["hint"]); hint != "" {
		body += "\n\n---\n\n**Operator hint:** " + hint +
			"\n\nUse this to pick the projects directly — keep the questions minimal.\n"
	}
	return []Message{{Role: "user", Text: body}}, nil
}
