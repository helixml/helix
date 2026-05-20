package server

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"time"

	"github.com/rs/zerolog/log"

	"github.com/helixml/helix-org/agent"
	agenthelix "github.com/helixml/helix-org/agent/helix"
	"github.com/helixml/helix-org/broadcast"
	"github.com/helixml/helix-org/config"
	"github.com/helixml/helix-org/domain"
	"github.com/helixml/helix-org/helix/helixclient"
	"github.com/helixml/helix-org/server/chat"
	orgstore "github.com/helixml/helix-org/store"

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
		Description: "Code-agent runtime applied to every Worker's Helix project. Default `claude_code` uses the operator's Claude OAuth subscription with no provider/model required. Set to `zed_agent` (or another supported value) only if you have a different inference path configured.",
	})
	r.Register(config.Spec{
		Key:         "worker.anthropic_api_key",
		Type:        config.TypeString,
		Description: "Anthropic API key for the `claude_code` runtime. Empty (default) = OAuth subscription mode (the operator's `claude login` credentials are used in the sandbox). Setting this flips Workers to API-key mode: credentials=api_key, and the key is injected into each Worker's sandbox as ANTHROPIC_API_KEY. Use this on deployments without a Claude subscription. SENSITIVE — value not redacted by `config get` yet (string specs don't support field-level secrets).",
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
}

// buildEmbeddedChatBackend constructs a HelixBridge that opens the
// owner Worker's persistent zed_external chat session. The bridge
// runs ProjectApplier.Ensure(w-owner) per send to materialise the
// owner's per-Worker Helix project (idempotent — first call provisions,
// subsequent calls return the same IDs), then opens / continues the
// chat session against the project's auto-provisioned agent app.
//
// w-owner is itself a Worker: same defaults (worker.runtime) apply,
// same MCP wiring, same desktop runtime as any AI Worker the org hires
// later. There is no separate "chat backend agent" picker — the chat
// surface is a window onto the owner's own sandbox.
//
// Returns nil + nil if helix.api_key isn't set yet (auto-provision
// happens at startup; a fresh DB with no admin user is a legitimate
// "not configured" state).
func buildEmbeddedChatBackend(ctx context.Context, cfg *config.Registry, applier *agenthelix.ProjectApplier, client helixclient.Client, logger *slog.Logger, orgSt *orgstore.Store, bc *broadcast.Broadcaster, newID func() string, now func() time.Time) (chat.Backend, error) {
	if applier == nil {
		log.Warn().Msg("helix-org chat backend not configured — project applier unavailable")
		return nil, nil
	}
	cwd, err := os.Getwd()
	if err != nil {
		return nil, fmt.Errorf("getwd: %w", err)
	}

	bridge, err := chat.NewHelix(chat.HelixConfig{
		Client: client,
		Ensure: applier,
		// SessionRole=`exploratory` so Helix's per-project "Open Human
		// Desktop" button finds and reuses the owner-chat session
		// instead of spawning a parallel sandbox. The owner chat IS
		// the project's human-driven session in helix-org's model;
		// labelling it `exploratory` makes that explicit to the rest
		// of Helix (project_handlers.go::startExploratorySession
		// matches on this role).
		SessionRole: "exploratory",
		OwnerID:     "w-owner",
		CWD:         cwd,
		Logger:      logger,
		// Persist the live owner-chat session pointer on the same
		// WorkerRuntimeState row the Spawner uses, so a process
		// restart (or a parallel UI like Helix's own project page)
		// can pick up the warm Zed sandbox instead of booting a
		// fresh one.
		LoadSessionID: func(ctx context.Context, workerID domain.WorkerID) (string, error) {
			state, err := agenthelix.LoadState(ctx, applier.Store, workerID)
			if err != nil {
				return "", err
			}
			return state.SessionID, nil
		},
		SaveSessionID: func(ctx context.Context, workerID domain.WorkerID, sessionID string) error {
			return agenthelix.SaveSession(ctx, applier.Store, workerID, sessionID)
		},
		// Publish owner-chat turns to s-activations-w-owner using the
		// same helper every AI Worker activation uses. /ui/streams
		// surfaces the owner's stream alongside every other Worker's
		// — the owner is just-another-Worker from the data model
		// perspective; the only difference is *who triggers* the
		// activation (human typing into the chat surface vs.
		// dispatcher reacting to a stream event).
		PublishActivation: func(ctx context.Context, workerID domain.WorkerID, body string) {
			_, _ = agent.PublishActivationEvent(ctx, orgSt, bc, newID, now, logger, workerID, body)
		},
	})
	if err != nil {
		return nil, fmt.Errorf("build helix chat bridge: %w", err)
	}
	log.Info().Msg("helix-org chat backend wired (project-applier mode — owner is a Worker)")
	return bridge, nil
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
	if err := reg.Set(ctx, "helix.api_key", string(payload), domain.WorkerID("w-owner")); err != nil {
		return "", fmt.Errorf("save api key to config: %w", err)
	}
	log.Info().
		Str("owner_id", owner.ID).
		Str("owner_email", owner.Email).
		Msg("helix-org auto-provisioned service api key")
	return keyStr, nil
}

// withHelixUserBearer wraps an embedded helix-org handler so the
// helixclient calls it makes inherit the logged-in user's identity.
// The middleware:
//
//   - reads the user from the request context (set by Helix's
//     extractMiddleware further out in the chain)
//   - finds an api_key owned by that user (mints one labelled
//     "helix-org alpha (per-user)" on first hit so we don't depend on
//     the user having created one manually)
//   - injects the key as a per-request bearer via
//     helixclient.WithBearerToken
//
// Anything helix-org's bridge or picker does downstream then runs as
// the actual logged-in user. The auto-provisioned helix.api_key
// remains as a fallback for callers that arrive without a session
// (e.g. integration tests).
func withHelixUserBearer(next http.Handler, st helixstore.Store) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user := getRequestUser(r)
		if hasUser(user) {
			if key, err := resolveUserHelixAPIKey(r.Context(), st, user.ID); err == nil && key != "" {
				r = r.WithContext(helixclient.WithBearerToken(r.Context(), key))
			} else if err != nil {
				log.Warn().Err(err).Str("user_id", user.ID).Msg("helix-org: failed to resolve user api key; falling back to service key")
			}
		}
		next.ServeHTTP(w, r)
	})
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
