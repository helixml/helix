package main

import (
	"github.com/helixml/helix-org/config"
)

// registerAllConfigSpecs declares every config key the running
// helix-org binary knows about. Both `serve` and `config <subcommand>`
// call this so the CLI's view of valid keys stays in sync with what
// subsystems actually consume at runtime.
//
// As subsystems grow (new transports, future LLM providers, etc.)
// add their Specs here. A future refactor could push registration
// into each subsystem's package-level init, but a flat list keeps
// the surface visible and reviewable in one place.
func registerAllConfigSpecs(r *config.Registry) {
	r.Register(config.Spec{
		Key:         "claude.bin",
		Type:        config.TypeString,
		Default:     `"claude"`,
		Required:    true,
		Description: "Path to the claude CLI binary used by the spawner.",
	})
	r.Register(config.Spec{
		Key:         "claude.public_url",
		Type:        config.TypeString,
		Default:     `"http://localhost:8080"`,
		Required:    true,
		Description: "Base URL Workers reach helix-org's MCP endpoint at. Set to your ngrok / Cloudflare tunnel URL when transports need to webhook in from outside.",
	})
	r.Register(config.Spec{
		Key:         "claude.model",
		Type:        config.TypeString,
		Default:     `"sonnet"`,
		Description: "Claude model alias or full name passed via --model. Defaults to 'sonnet' to keep activation costs predictable; set to 'opus' or a full name (e.g. 'claude-opus-4-7') to override.",
	})
	r.Register(config.Spec{
		Key:         "claude.effort",
		Type:        config.TypeString,
		Default:     `"low"`,
		Description: "Claude effort/thinking level passed via --effort (low|medium|high|xhigh|max). Defaults to 'low' so multi-agent activations don't burn extended-thinking budget unless explicitly raised.",
	})
	r.Register(config.Spec{
		Key:         "transport.postmark",
		Type:        config.TypeObject,
		Secrets:     []string{"token"},
		Description: `Postmark account config: {"token","inbound","from"}. Required only if any Stream uses transport=email.`,
	})
	r.Register(config.Spec{
		Key:         "transport.github",
		Type:        config.TypeObject,
		Secrets:     []string{"token", "webhook_secret"},
		Description: `GitHub webhooks config: {"token","webhook_secret"}. Required only if any Stream uses transport=github. token is the gh PAT used by Workers; webhook_secret is the HMAC secret GitHub signs deliveries with.`,
	})
}
