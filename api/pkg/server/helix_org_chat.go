package server

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os"

	"github.com/rs/zerolog/log"

	"github.com/helixml/helix-org/config"
	"github.com/helixml/helix-org/domain"
	"github.com/helixml/helix-org/helix/helixclient"
	"github.com/helixml/helix-org/server/chat"
	"github.com/helixml/helix-org/store"

	helixstore "github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/system"
	"github.com/helixml/helix/api/pkg/types"
)

// registerHelixOrgConfigSpecs declares the operational-config keys the
// embedded helix-org honours. The embedded alpha drives chat entirely
// through an existing Helix agent picked via chat.app_id — agent_type,
// provider, and model all come from the agent's own assistant config
// (see session_handlers.go:383), so the standalone chat.backend /
// chat.provider / chat.model knobs are intentionally omitted.
func registerHelixOrgConfigSpecs(r *config.Registry) {
	r.Register(config.Spec{
		Key:         "chat.session_role",
		Type:        config.TypeString,
		Default:     `"owner-chat"`,
		Description: "session_role written on Helix chat sessions opened by the chat surface.",
	})
	r.Register(config.Spec{
		Key:         "chat.app_id",
		Type:        config.TypeString,
		Description: "ID of the Helix agent under /orgs/<org>/agents that drives this chat surface. Pick one via /ui/alpha-agents — agent_type, provider, and model all come from the agent itself.",
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
	r.Register(config.Spec{
		Key:         "helix.org_url",
		Type:        config.TypeString,
		Description: "Externally-reachable URL for helix-org's MCP endpoint, written as HELIX_ORG_URL on each per-Worker Helix project. Leave empty to skip MCP wiring.",
	})
}

// buildEmbeddedChatBackend constructs a HelixBridge that delegates
// every chat send to a Helix agent picked at runtime via chat.app_id.
// The bridge re-reads chat.app_id on each send (so the operator can
// switch agents under /ui/alpha-agents without restarting), and the
// per-request bearer middleware (withHelixUserBearer) attributes the
// resulting Helix sessions to the actual logged-in user.
//
// Returns nil + nil if helix.api_key isn't set yet — the auto-provision
// path (ensureHelixOrgServiceAPIKey) normally fills it in at startup,
// but a fresh DB with no admin user is a legitimate "not configured"
// state and the UI should render that gracefully.
func buildEmbeddedChatBackend(ctx context.Context, cfg *config.Registry, _ *store.Store, logger *slog.Logger) (chat.Backend, error) {
	apiKey, _ := cfg.GetString(ctx, "helix.api_key")
	if apiKey == "" {
		log.Warn().Msg("helix-org chat backend not configured — helix.api_key not yet provisioned")
		return nil, nil
	}
	baseURL, err := cfg.GetString(ctx, "helix.url")
	if err != nil {
		return nil, fmt.Errorf("read helix.url: %w", err)
	}
	sessionRole, err := cfg.GetString(ctx, "chat.session_role")
	if err != nil {
		return nil, fmt.Errorf("read chat.session_role: %w", err)
	}

	client, err := helixclient.New(helixclient.Config{BaseURL: baseURL, APIKey: apiKey})
	if err != nil {
		return nil, fmt.Errorf("helix client: %w", err)
	}
	cwd, err := os.Getwd()
	if err != nil {
		return nil, fmt.Errorf("getwd: %w", err)
	}

	appIDFunc := func(ctx context.Context) (string, error) {
		return cfg.GetString(ctx, "chat.app_id")
	}
	bridge, err := chat.NewHelix(chat.HelixConfig{
		Client:      client,
		AppIDFunc:   appIDFunc,
		OwnerID:     "w-owner",
		SessionRole: sessionRole,
		CWD:         cwd,
		Logger:      logger,
	})
	if err != nil {
		return nil, fmt.Errorf("build helix chat bridge: %w", err)
	}
	currentApp, _ := cfg.GetString(ctx, "chat.app_id")
	log.Info().
		Str("helix_url", baseURL).
		Str("current_app_id", currentApp).
		Msg("helix-org chat backend wired (helix bridge, app-only dynamic)")
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
