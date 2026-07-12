package helixorg

import (
	"github.com/helixml/helix/api/pkg/org/application/configregistry"
)

// AlphaFeature is the alpha-feature flag that gates the embedded
// helix-org surface. Granted per-user via:
//
//	UPDATE users SET alpha_features = array_append(alpha_features, 'helix-org')
const AlphaFeature = "helix-org"

// RegisterConfigSpecs declares the operational-config keys the
// embedded helix-org honours. The embedded alpha has exactly one
// user-facing knob: `worker.runtime` — the code-agent runtime every
// Worker (owner included) gets provisioned with. Everything else is
// derived. The `helix.*` keys are auto-managed plumbing the user
// shouldn't normally touch.
func RegisterConfigSpecs(r *configregistry.Registry) {
	r.Register(configregistry.Spec{
		Key:         "worker.runtime",
		Type:        configregistry.TypeString,
		Default:     `"claude_code"`,
		Description: "Code-agent runtime applied to every Worker's Helix project. `claude_code` (default) is the Anthropic Claude CLI; `codex_cli` is OpenAI Codex; `zed_agent` is the Helix-routed conversational agent. Other supported runtimes include `qwen_code`.",
	})
	r.Register(configregistry.Spec{
		Key:         "worker.credentials",
		Type:        configregistry.TypeString,
		Default:     `"subscription"`,
		Description: "Auth source for the runtime. `subscription` (default) uses the operator's connected Claude or ChatGPT credentials for `claude_code` or `codex_cli`. `api_key` routes inference through a provider configured in Helix Providers and requires `worker.provider` and `worker.model`. Other runtimes always use `api_key`.",
	})
	r.Register(configregistry.Spec{
		Key:         "worker.provider",
		Type:        configregistry.TypeString,
		Description: "Helix provider name (e.g. `anthropic`, `openai`) routed-through inference uses. Required when `worker.credentials=api_key` or when the runtime does not support subscriptions. Must match a provider configured in Helix's Providers panel.",
	})
	r.Register(configregistry.Spec{
		Key:         "worker.model",
		Type:        configregistry.TypeString,
		Description: "Model ID for the chosen provider or Codex subscription (e.g. `claude-sonnet-4-5`, `gpt-5.6-sol`). Required alongside `worker.provider` whenever inference routes through Helix. For subscription-backed `codex_cli`, selects the Codex default model; ignored for subscription-backed `claude_code`.",
	})
	r.Register(configregistry.Spec{
		Key:         "worker.specs_mandate",
		Type:        configregistry.TypeString,
		Description: "Activation-prompt directive that tells every Worker how to find role.md / identity.md / agent.md on the helix-specs branch and how to checkpoint state back. The default (runtimehelix.DefaultHelixSpecsMandate) handles the standard layout; override when the file paths, the git-pull recipe, or the checkpoint convention change without redeploying. Use an empty string to fall back to the default.",
	})
	r.Register(configregistry.Spec{
		Key:         "helix.url",
		Type:        configregistry.TypeString,
		Default:     `"http://localhost:8080"`,
		Description: "Base URL of the Helix server this org talks to. Defaults to localhost because we're embedded in the api container.",
	})
	r.Register(configregistry.Spec{
		Key:         "helix.api_key",
		Type:        configregistry.TypeString,
		Description: "Fallback bearer token for the embedded helix-org client when no logged-in user is on the request (rare — most calls forward the user's own api key). Auto-provisioned at startup against the first admin user.",
	})
	// Transport-level secrets: every Stream whose transport is `postmark`
	// or `github` reads these. Secrets are redacted on `config get` —
	// see TestRegisterHelixOrgConfigSpecs_RedactsTransportSecrets. Any
	// future refactor that drops one of the entries from the Secrets
	// list would silently start leaking the value to anyone with shell
	// access who reads the configs table; the test pins them.
	r.Register(configregistry.Spec{
		Key:         "transport.postmark",
		Type:        configregistry.TypeObject,
		Secrets:     []string{"token"},
		Description: `Postmark account config: {"token","inbound","from"}. Required only if any Stream uses transport=email.`,
	})
	r.Register(configregistry.Spec{
		Key:         "transport.github",
		Type:        configregistry.TypeObject,
		Secrets:     []string{"token", "webhook_secret"},
		Description: `GitHub webhooks config: {"token","webhook_secret"}. Required only if any Stream uses transport=github. token is the gh PAT used by Workers; webhook_secret is the HMAC secret GitHub signs deliveries with.`,
	})
}
