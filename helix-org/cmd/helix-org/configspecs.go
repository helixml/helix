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
		Key:         "spawner.kind",
		Type:        config.TypeString,
		Default:     `"claude"`,
		Description: "Which Spawner to use for AI Worker activations: 'claude' (local dev, runs `claude -p`) or 'helix' (production, delegates to a co-located Helix server).",
	})
	r.Register(config.Spec{
		Key:         "helix.url",
		Type:        config.TypeString,
		Description: "Base URL of the co-located Helix server (e.g. http://helix:8080). Required when spawner.kind = helix.",
	})
	r.Register(config.Spec{
		Key:         "helix.api_key",
		Type:        config.TypeString,
		Description: "Bearer token used for all Helix REST and WebSocket calls. Required when spawner.kind = helix.",
	})
	r.Register(config.Spec{
		Key:         "helix.org_url",
		Type:        config.TypeString,
		Description: "helix-org's externally-resolvable URL, written as a project secret (HELIX_ORG_URL) on every per-Worker Helix project so the in-sandbox agent can call /workers/{id}/mcp. Required when spawner.kind = helix.",
	})
	r.Register(config.Spec{
		Key:         "helix.activation_timeout",
		Type:        config.TypeString,
		Default:     `"5m"`,
		Description: "Per-activation hard timeout (Go duration string). Default 5m.",
	})
	r.Register(config.Spec{
		Key:         "helix.max_inflight",
		Type:        config.TypeInt,
		Default:     `8`,
		Description: "Cap on simultaneous open Helix activations across all Workers. Default 8.",
	})
	r.Register(config.Spec{
		Key:         "chat.backend",
		Type:        config.TypeString,
		Default:     `"claude"`,
		Description: "Backend for the owner chat surface (CLI 'helix-org chat' and /ui/chat). 'claude' runs a local subprocess (dev). 'helix' delegates to a Helix chat session against the owner Worker's per-Worker project.",
	})
	r.Register(config.Spec{
		Key:         "chat.session_role",
		Type:        config.TypeString,
		Default:     `"owner-chat"`,
		Description: "session_role written on Helix chat sessions opened by the chat surface. Used for filtering Recents.",
	})
	r.Register(config.Spec{
		Key:         "chat.provider",
		Type:        config.TypeString,
		Default:     `"bunker-minimax-m2.7"`,
		Description: "Helix provider used by the chat surface (helix backend only). The provider is the prefix before the slash in a Helix model ID — e.g. 'bunker-minimax-m2.7' for 'bunker-minimax-m2.7/minimax-m2.7'.",
	})
	r.Register(config.Spec{
		Key:         "chat.model",
		Type:        config.TypeString,
		Default:     `"minimax-m2.7"`,
		Description: "Model the chat surface uses on Helix (helix backend only). Bare model name (the suffix after the provider slash). Must exist on the configured provider.",
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
