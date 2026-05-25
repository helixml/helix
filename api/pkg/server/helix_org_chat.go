package server

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/rs/zerolog/log"

	"github.com/helixml/helix/api/pkg/org/config"

	"github.com/helixml/helix/api/pkg/org/worker"
	helixstore "github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/system"
	"github.com/helixml/helix/api/pkg/types"
)

// registerHelixOrgConfigSpecs declares the operational-config keys the
// embedded helix-org honours. The embedded alpha has exactly one
// user-facing knob: `worker.runtime` — the code-agent runtime every
// Worker (owner included) gets provisioned with. Everything else is
// derived. The `helix.*` keys are auto-managed plumbing the user
// shouldn't normally touch.
func registerHelixOrgConfigSpecs(r *config.Registry) {
	r.Register(config.Spec{
		Key:         "worker.runtime",
		Type:        config.TypeString,
		Default:     `"claude_code"`,
		Description: "Code-agent runtime applied to every Worker's Helix project. `claude_code` (default) is the Anthropic Claude CLI; `zed_agent` is the Helix-routed conversational agent. Other runtimes (e.g. `qwen_code`) work if Helix supports them.",
	})
	r.Register(config.Spec{
		Key:         "worker.credentials",
		Type:        config.TypeString,
		Default:     `"subscription"`,
		Description: "Auth source for the runtime. `subscription` (default) uses the operator's connected Claude OAuth (only valid for `claude_code`). `api_key` routes inference through Helix's anthropic/openai/etc. provider (configured separately in Helix Providers); requires `worker.provider` and `worker.model` to be set. For `zed_agent` and other non-subscription runtimes this is effectively always `api_key`.",
	})
	r.Register(config.Spec{
		Key:         "worker.provider",
		Type:        config.TypeString,
		Description: "Helix provider name (e.g. `anthropic`, `openai`) routed-through inference uses. Required when `worker.credentials=api_key` or when `worker.runtime` is anything other than `claude_code`. Must match a provider configured in Helix's Providers panel (or auto-provisioned from `ANTHROPIC_API_KEY` / `OPENAI_API_KEY` env vars at startup).",
	})
	r.Register(config.Spec{
		Key:         "worker.model",
		Type:        config.TypeString,
		Description: "Model ID for the chosen provider (e.g. `claude-sonnet-4-5`, `gpt-4o-mini`). Required alongside `worker.provider` whenever inference routes through Helix. Ignored for `claude_code`+`subscription` (the CLI picks its own model).",
	})
	r.Register(config.Spec{
		Key:         "worker.specs_mandate",
		Type:        config.TypeString,
		Description: "Activation-prompt directive that tells every Worker how to find role.md / identity.md / agent.md on the helix-specs branch and how to checkpoint state back. The default (runtimehelix.DefaultHelixSpecsMandate) handles the standard layout; override when the file paths, the git-pull recipe, or the checkpoint convention change without redeploying. Use an empty string to fall back to the default.",
	})
	r.Register(config.Spec{
		Key:         "helix.url",
		Type:        config.TypeString,
		Default:     `"http://localhost:8080"`,
		Description: "Base URL of the Helix server this org talks to. Defaults to localhost because we're embedded in the api container.",
	})
	r.Register(config.Spec{
		Key:         "helix.api_key",
		Type:        config.TypeString,
		Description: "Fallback bearer token for the embedded helix-org client when no logged-in user is on the request (rare — most calls forward the user's own api key). Auto-provisioned at startup against the first admin user.",
	})
	// Transport-level secrets: every Stream whose transport is `postmark`
	// or `github` reads these. Secrets are redacted on `config get` —
	// see TestRegisterHelixOrgConfigSpecs_RedactsTransportSecrets. Any
	// future refactor that drops one of the entries from the Secrets
	// list would silently start leaking the value to anyone with shell
	// access who reads the configs table; the test pins them.
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

// ensureHelixOrgServiceAPIKey returns a valid Helix api_key for the
// embedded helix-org client, minting one on first run. The result is
// also persisted to the helix.api_key config so subsequent reads pick
// it up without re-checking the store. Stale keys (config row points
// at a deleted api_keys row) are silently replaced.
//
// The owner of the minted key is the first admin user found in the
// Helix DB. All gated alpha users currently drive the same owner
// Worker (design note: shared Worker risk), so co-tenanting on one
// service identity is consistent — multi-tenant attribution is a
// future change.
func ensureHelixOrgServiceAPIKey(ctx context.Context, st helixstore.Store, reg *config.Registry) (string, error) {
	if existing, _ := reg.GetString(ctx, "helix.api_key"); existing != "" {
		if _, err := st.GetAPIKey(ctx, &types.ApiKey{Key: existing}); err == nil {
			return existing, nil
		}
		log.Warn().Msg("helix-org helix.api_key in config no longer exists in helix DB — re-provisioning")
	}

	admins, _, err := st.ListUsers(ctx, &helixstore.ListUsersQuery{Admin: true})
	if err != nil {
		return "", fmt.Errorf("list admins: %w", err)
	}
	if len(admins) == 0 {
		return "", fmt.Errorf("no admin user found — register one before opening the helix-org alpha")
	}
	owner := admins[0]

	// Grant the alpha-feature flag to the service owner so the MCP
	// gateway accepts requests authenticated by this key. Without it,
	// per-Worker MCP calls from Zed sandboxes 403 — the backend's
	// requireFeature check applies to every authenticated caller,
	// including service identities. Idempotent.
	hasFlag := false
	for _, f := range owner.AlphaFeatures {
		if f == alphaFeatureHelixOrg {
			hasFlag = true
			break
		}
	}
	if !hasFlag {
		owner.AlphaFeatures = append(owner.AlphaFeatures, alphaFeatureHelixOrg)
		if _, err := st.UpdateUser(ctx, owner); err != nil {
			return "", fmt.Errorf("grant alpha flag to service owner: %w", err)
		}
		log.Info().Str("owner_email", owner.Email).Msg("helix-org granted alpha flag to service owner")
	}

	keyStr, err := system.GenerateAPIKey()
	if err != nil {
		return "", fmt.Errorf("generate api key: %w", err)
	}
	if _, err := st.CreateAPIKey(ctx, &types.ApiKey{
		Owner:     owner.ID,
		OwnerType: types.OwnerTypeUser,
		Key:       keyStr,
		Name:      "helix-org alpha (auto-provisioned)",
		Type:      types.APIkeytypeAPI,
	}); err != nil {
		return "", fmt.Errorf("create api key: %w", err)
	}

	payload, err := json.Marshal(keyStr)
	if err != nil {
		return "", fmt.Errorf("encode api key: %w", err)
	}
	if err := reg.Set(ctx, "helix.api_key", string(payload), worker.ID("w-owner")); err != nil {
		return "", fmt.Errorf("save api key to config: %w", err)
	}
	log.Info().
		Str("owner_id", owner.ID).
		Str("owner_email", owner.Email).
		Msg("helix-org auto-provisioned service api key")
	return keyStr, nil
}

// resolveUserHelixAPIKey returns an api_key owned by userID, minting
// one if the user has none yet. Cached lookups are unnecessary —
// ListAPIKeys is a single indexed query and the cost is dominated by
// the LLM round-trip immediately following.
func resolveUserHelixAPIKey(ctx context.Context, st helixstore.Store, userID string) (string, error) {
	keys, err := st.ListAPIKeys(ctx, &helixstore.ListAPIKeysQuery{Owner: userID, Type: types.APIkeytypeAPI})
	if err != nil {
		return "", fmt.Errorf("list api keys: %w", err)
	}
	if len(keys) > 0 {
		return keys[0].Key, nil
	}
	keyStr, err := system.GenerateAPIKey()
	if err != nil {
		return "", fmt.Errorf("generate api key: %w", err)
	}
	if _, err := st.CreateAPIKey(ctx, &types.ApiKey{
		Owner:     userID,
		OwnerType: types.OwnerTypeUser,
		Key:       keyStr,
		Name:      "helix-org alpha (per-user)",
		Type:      types.APIkeytypeAPI,
	}); err != nil {
		return "", fmt.Errorf("create api key: %w", err)
	}
	log.Info().Str("user_id", userID).Msg("helix-org auto-provisioned per-user api key")
	return keyStr, nil
}
