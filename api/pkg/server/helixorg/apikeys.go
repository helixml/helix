package helixorg

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/rs/zerolog/log"

	"github.com/helixml/helix/api/pkg/org/application/configregistry"
	helixstore "github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/system"
	"github.com/helixml/helix/api/pkg/types"
)

// APIKeys provisions Helix api_keys for the embedded helix-org module:
// the shared per-org service identity (Service, minted on first run and
// cached in config) and per-user keys (User, minted on demand for the
// Spawner to run an activation as the picking user).
//
// It is a small noun port (per design §5.3 — not an "IdentityProvisioner")
// that keeps the credential-provisioning business logic out of the
// composition file and behind a fakeable seam. The org runtime consumes
// the User half through its existing BearerForUser func; the bootstrap
// middleware consumes the Service half.
type APIKeys interface {
	Service(ctx context.Context, orgID string) (string, error)
	User(ctx context.Context, userID string) (string, error)
}

// HelixAPIKeys implements APIKeys against the host Helix store and the
// org config registry. This is the one place that reads/writes the
// host's users / api_keys for helix-org provisioning.
type HelixAPIKeys struct {
	store   helixstore.Store
	configs *configregistry.Registry
}

var _ APIKeys = (*HelixAPIKeys)(nil)

func NewHelixAPIKeys(st helixstore.Store, configs *configregistry.Registry) *HelixAPIKeys {
	return &HelixAPIKeys{store: st, configs: configs}
}

// Service returns a valid Helix api_key for the embedded helix-org
// client, minting one on first run. The result is persisted to the
// helix.api_key config so subsequent reads pick it up without
// re-checking the store. Stale keys (config row points at a deleted
// api_keys row) are silently replaced.
//
// The minted key belongs to the organization owner.
func (k *HelixAPIKeys) Service(ctx context.Context, orgID string) (string, error) {
	org, err := k.store.GetOrganization(ctx, &helixstore.GetOrganizationQuery{ID: orgID})
	if err != nil {
		return "", fmt.Errorf("get organization: %w", err)
	}
	if org == nil || org.Owner == "" {
		return "", fmt.Errorf("organization %s has no owner", orgID)
	}
	if existing, _ := k.configs.GetString(ctx, orgID, "helix.api_key"); existing != "" {
		if key, err := k.store.GetAPIKey(ctx, &types.ApiKey{Key: existing}); err == nil && key != nil && key.Owner == org.Owner {
			return existing, nil
		}
		log.Warn().Msg("helix-org helix.api_key is missing or not owned by the organization owner — re-provisioning")
	}

	owner, err := k.store.GetUser(ctx, &helixstore.GetUserQuery{ID: org.Owner})
	if err != nil {
		return "", fmt.Errorf("get organization owner: %w", err)
	}
	if owner == nil {
		return "", fmt.Errorf("organization owner %s not found", org.Owner)
	}

	keyStr, err := system.GenerateAPIKey()
	if err != nil {
		return "", fmt.Errorf("generate api key: %w", err)
	}
	if _, err := k.store.CreateAPIKey(ctx, &types.ApiKey{
		Owner:     owner.ID,
		OwnerType: types.OwnerTypeUser,
		Key:       keyStr,
		Name:      "helix-org service (auto-provisioned)",
		Type:      types.APIkeytypeAPI,
	}); err != nil {
		return "", fmt.Errorf("create api key: %w", err)
	}

	payload, err := json.Marshal(keyStr)
	if err != nil {
		return "", fmt.Errorf("encode api key: %w", err)
	}
	if err := k.configs.Set(ctx, orgID, "helix.api_key", string(payload)); err != nil {
		return "", fmt.Errorf("save api key to config: %w", err)
	}
	log.Info().
		Str("owner_id", owner.ID).
		Str("owner_email", owner.Email).
		Msg("helix-org auto-provisioned service api key")
	return keyStr, nil
}

// User returns a PERSONAL api_key owned by userID — no organization, project,
// spec task, session or app binding — minting one if the user has none.
// Cached lookups are unnecessary — ListAPIKeys is a single indexed query and
// the cost is dominated by the LLM round-trip immediately following.
//
// PERSONAL IS LOAD-BEARING. This is the credential an org bot's activation
// runs under (the Spawner's BearerForUser), and the activation starts the
// bot's OWN session — which a scoped key is refused: org-bot sessions may not
// run on a key bound to an organization (let alone a project, spec task,
// session or app), because the session's owner, quota and audit trail must be
// a person. ListAPIKeys orders by nothing, so keys[0] is whatever Postgres
// returns first — and when a user's key list gains a scoped key (an
// automation integration's key, for example), an unfiltered first-pick hands
// the spawner that key and EVERY new org-bot session starts failing before
// the session row exists, while the durable activation queue retries with a
// growing backoff. The bot looks alive — its run events land on the trigger —
// and no session ever appears. Seen live 2026-11-19.
func (k *HelixAPIKeys) User(ctx context.Context, userID string) (string, error) {
	keys, err := k.store.ListAPIKeys(ctx, &helixstore.ListAPIKeysQuery{Owner: userID, Type: types.APIkeytypeAPI})
	if err != nil {
		return "", fmt.Errorf("list api keys: %w", err)
	}
	for _, key := range keys {
		if !isPersonalAPIKey(key) {
			continue
		}
		return key.Key, nil
	}
	log.Warn().Str("user_id", userID).Int("keys", len(keys)).
		Msg("helix-org: user has no personal api key, minting one for activations")
	keyStr, err := system.GenerateAPIKey()
	if err != nil {
		return "", fmt.Errorf("generate api key: %w", err)
	}
	if _, err := k.store.CreateAPIKey(ctx, &types.ApiKey{
		Owner:     userID,
		OwnerType: types.OwnerTypeUser,
		Key:       keyStr,
		Name:      "helix-org (per-user)",
		Type:      types.APIkeytypeAPI,
	}); err != nil {
		return "", fmt.Errorf("create api key: %w", err)
	}
	log.Info().Str("user_id", userID).Msg("helix-org auto-provisioned per-user api key")
	return keyStr, nil
}

// isPersonalAPIKey reports whether the key acts for the person themself, with
// no binding that would narrow what an activation may touch. An org-scoped
// key is excluded on purpose even though org work is where activations run:
// starting a session is attributed to a person, and the scoped-credential
// checks treat org keys the same way.
func isPersonalAPIKey(k *types.ApiKey) bool {
	if k == nil {
		return false
	}
	if k.OrganizationID != "" || k.ProjectID != "" || k.SpecTaskID != "" || k.SessionID != "" {
		return false
	}
	return k.AppID == nil || !k.AppID.Valid || k.AppID.String == ""
}
