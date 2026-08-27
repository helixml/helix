package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/mux"
	"github.com/helixml/helix/api/pkg/config"
	"github.com/helixml/helix/api/pkg/model"
	"github.com/helixml/helix/api/pkg/openai/manager"
	"github.com/helixml/helix/api/pkg/pricing"
	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/rs/zerolog/log"
)

// listProviders godoc
// @Summary List currently configured providers
// @Description List currently configured providers
// @Tags    providers

// @Success 200 {array} types.Provider
// @Router /api/v1/providers [get]
// @Security BearerAuth
func (s *HelixAPIServer) listProviders(rw http.ResponseWriter, r *http.Request) {
	user := getRequestUser(r)

	providers, err := s.providerManager.ListProviders(r.Context(), user.ID)
	if err != nil {
		log.Err(err).Msg("error listing providers")
		http.Error(rw, "Internal server error: "+err.Error(), http.StatusInternalServerError)
		return
	}

	rw.Header().Set("Content-Type", "application/json")
	err = json.NewEncoder(rw).Encode(providers)
	if err != nil {
		log.Err(err).Msg("error writing response")
		http.Error(rw, "Internal server error", http.StatusInternalServerError)
		return
	}
}

var blankAPIKey = "********"

// providersManagementEnabled reports whether non-admin users may manage their
// own provider endpoints. Two operator controls grant this and they mean the
// same thing: the static ENABLE_CUSTOM_USER_PROVIDERS env var and the admin
// UI's "Providers Management" toggle stored in system settings.
func providersManagementEnabled(settings *types.SystemSettings, cfg *config.ServerConfig) bool {
	return settings.ProvidersManagementEnabled || cfg.Providers.EnableCustomUserProviders
}

func (s *HelixAPIServer) isProvidersManagementEnabled(ctx context.Context) bool {
	systemSettings, err := s.Store.GetSystemSettings(ctx)
	if err != nil {
		return s.Cfg.Providers.EnableCustomUserProviders
	}
	return providersManagementEnabled(systemSettings, s.Cfg)
}

// listProviderEndpoints godoc
// @Summary List currently configured provider endpoints
// @Description List currently configured providers
// @Tags    providers

// @Success 200 {array} types.ProviderEndpoint
// @Param with_models query bool false "Include models"
// @Param org_id query string false "Organization ID"
// @Param code_agent_runtime query string false "Filter by organization code-agent harness policy"
// @Param all query bool false "Include all endpoints (system admin only)"
// @Router /api/v1/provider-endpoints [get]
// @Security BearerAuth
func (s *HelixAPIServer) listProviderEndpoints(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	includeModels := r.URL.Query().Get("with_models") == "true"
	orgID := r.URL.Query().Get("org_id")
	runtimeParam := r.URL.Query().Get("code_agent_runtime")
	all := r.URL.Query().Get("all") == "true"
	var runtime types.CodeAgentRuntime
	if runtimeParam != "" {
		if orgID == "" {
			http.Error(rw, "org_id is required with code_agent_runtime", http.StatusBadRequest)
			return
		}
		runtime = types.CodeAgentRuntime(runtimeParam)
		if !types.IsSelectableCodeAgentRuntime(runtime) {
			http.Error(rw, fmt.Sprintf("unsupported code agent runtime %q", runtime), http.StatusBadRequest)
			return
		}
	}

	if orgID != "" {
		org, err := s.lookupOrg(ctx, orgID)
		if err != nil {
			if errors.Is(err, store.ErrNotFound) {
				writeErrResponse(rw, err, http.StatusNotFound)
				return
			}
			writeErrResponse(rw, fmt.Errorf("failed to lookup org: %w", err), http.StatusInternalServerError)
			return
		}
		orgID = org.ID
	}

	user := getRequestUser(r)

	if orgID != "" {
		// Check if user has access to view teams
		_, err := s.authorizeOrgMember(r.Context(), user, orgID)
		if err != nil {
			log.Err(err).Msg("error authorizing org member")
			http.Error(rw, "Could not authorize org member: "+err.Error(), http.StatusForbidden)
			return
		}
	}

	var harness *types.OrgCodeAgentHarness
	if runtimeParam != "" {
		var err error
		harness, err = s.loadOrgCodeAgentHarnessPolicy(ctx, orgID, runtime)
		if err != nil {
			writeErrResponse(rw, err, http.StatusInternalServerError)
			return
		}
	}

	var (
		providerEndpoints []*types.ProviderEndpoint
		err               error
	)

	query := &store.ListProviderEndpointsQuery{
		WithGlobal: true,
		All:        all,
	}

	if orgID != "" {
		query.OwnerType = types.OwnerTypeOrg
		query.Owner = orgID
	} else {
		query.OwnerType = types.OwnerTypeUser
		query.Owner = user.ID
	}

	// If authenticated, fetch user endpoints
	if user != nil {
		if query.All && !user.Admin {
			http.Error(rw, "Only system admins can list all endpoints", http.StatusForbidden)
			return
		}

		providerEndpoints, err = s.Store.ListProviderEndpoints(ctx, query)
		if err != nil {
			log.Err(err).Msg("error listing provider endpoints")
			http.Error(rw, "Internal server error: "+err.Error(), http.StatusInternalServerError)
			return
		}

		for idx := range providerEndpoints {
			if providerEndpoints[idx].APIKey != "" {
				providerEndpoints[idx].APIKey = blankAPIKey
			}
		}

		// Sort endpoints by name before adding global ones
		sort.Slice(providerEndpoints, func(i, j int) bool {
			return providerEndpoints[i].Name < providerEndpoints[j].Name
		})
	}

	// Get global ones from the provider manager
	globalProviderEndpoints, err := s.providerManager.ListProviders(ctx, "")
	if err != nil {
		log.Err(err).Msg("error listing providers")
		http.Error(rw, "Internal server error: "+err.Error(), http.StatusInternalServerError)
		return
	}

	for _, provider := range globalProviderEndpoints {
		// Sandbox-absorbs-runner equivalent of the old runnerController
		// gate: skip the Helix provider when no sandbox is currently
		// serving any model. Otherwise the picker offers an option that
		// returns "model X is not available" for every request, which
		// is a worse UX than not advertising it at all.
		if provider == types.ProviderHelix && s.inferenceRouter != nil {
			if len(s.inferenceRouter.AvailableModels()) == 0 {
				continue
			}
		}

		providerEndpoints = append(providerEndpoints, s.globalProviderEndpoint(provider))
	}
	if harness != nil {
		providerEndpoints = filterProviderEndpointsForHarness(providerEndpoints, harness, runtime)
	}

	// Set default
	for idx := range providerEndpoints {
		if providerEndpoints[idx].Name == s.Cfg.Inference.Provider {
			providerEndpoints[idx].Default = true
		}
	}

	// Re-sort the endpoints with default first, then by name
	sort.Slice(providerEndpoints, func(i, j int) bool {
		// User endpoints come to the top
		if providerEndpoints[i].EndpointType == types.ProviderEndpointTypeUser && providerEndpoints[j].EndpointType != types.ProviderEndpointTypeUser {
			return true
		}
		if providerEndpoints[j].EndpointType == types.ProviderEndpointTypeUser && providerEndpoints[i].EndpointType != types.ProviderEndpointTypeUser {
			return false
		}

		// If i is default and j is not, i comes first
		if providerEndpoints[i].Default && !providerEndpoints[j].Default {
			return true
		}
		// If j is default and i is not, j comes first
		if providerEndpoints[j].Default && !providerEndpoints[i].Default {
			return false
		}
		// If both are default or both are not default, sort by name
		return providerEndpoints[i].Name < providerEndpoints[j].Name
	})

	var wg sync.WaitGroup
	var mu sync.Mutex

	// Load models if required
	if includeModels {
		for idx := range providerEndpoints {
			wg.Add(1)
			go func(idx int) {
				defer wg.Done()
				result, err := s.getProviderModels(ctx, providerEndpoints[idx])
				if err != nil {
					log.Err(err).
						Str("provider", providerEndpoints[idx].Name).
						Str("endpoint_id", providerEndpoints[idx].ID).
						Str("owner", providerEndpoints[idx].Owner).
						Msg("error listing models")
					mu.Lock()
					providerEndpoints[idx].Status = types.ProviderEndpointStatusError
					providerEndpoints[idx].Error = err.Error()
					mu.Unlock()
					return
				}

				mu.Lock()
				if result.Degraded != nil {
					// Upstream is currently unreachable but we have a previous
					// successful snapshot — surface "degraded" without dropping
					// the model list so the picker keeps working.
					providerEndpoints[idx].Status = types.ProviderEndpointStatusError
					providerEndpoints[idx].Error = result.Degraded.Error()
				} else {
					providerEndpoints[idx].Status = types.ProviderEndpointStatusOK
				}
				providerEndpoints[idx].AvailableModels = result.Models
				mu.Unlock()
			}(idx)
		}
	}

	wg.Wait()

	writeResponse(rw, providerEndpoints, http.StatusOK)
}

// modelCacheTTL is how long a cached model list lives in the cache. It is
// deliberately longer than the freshness window so a transient upstream outage
// doesn't empty the model picker for every UI page — see getProviderModels.
const modelCacheTTL = 1 * time.Hour

// emptyModelCacheTTL is a much shorter TTL applied when an upstream
// returned ZERO models. The long modelCacheTTL is safe for "here is the
// list" answers, but pinning an empty list for 1h causes a cold-start
// blackout: the Helix self-provider returns an empty list during the
// brief window between API boot and the first sandbox heartbeat landing,
// and an unlucky cache miss in that window would leave the model picker
// empty for up to an hour after sandboxes came online. 30s lets the
// picker recover within a single user retry without forcing every page
// load to re-fetch.
const emptyModelCacheTTL = 30 * time.Second

// errUpstreamUnreachable marks refresh failures that came from the upstream
// /v1/models call itself (not from provider client construction). Callers use
// errors.Is to decide whether falling back to a cached payload is safe: an
// upstream timeout is, a config error is not.
var errUpstreamUnreachable = errors.New("upstream models endpoint unreachable")

// modelCacheKey is the single source of truth for the per-provider models cache
// key. Anything that mutates a provider (delete, rename, URL/key/Models change)
// must invalidate the entry under the OLD name and let the next read repopulate.
func modelCacheKey(name, owner string) string {
	return fmt.Sprintf("%s:%s", name, owner)
}

// catalogueCacheKey is the key for a provider's FULL upstream model list, kept
// separately from the effective (whitelist-applied) list under modelCacheKey.
// The models-picker UI needs the full catalogue to offer choices; everything
// else must only ever see the effective list.
func catalogueCacheKey(name, owner string) string {
	return fmt.Sprintf("catalogue:%s:%s", name, owner)
}

// globalProviderEndpoint builds the synthetic ProviderEndpoint for an
// env-baked global provider (one that has no database row). Used both when
// listing endpoints for the UI and when resolving a model's owning provider
// for chat-completion routing.
func (s *HelixAPIServer) globalProviderEndpoint(provider types.Provider) *types.ProviderEndpoint {
	var baseURL string
	switch provider {
	case types.ProviderOpenAI:
		baseURL = s.Cfg.Providers.OpenAI.BaseURL
	case types.ProviderTogetherAI:
		baseURL = s.Cfg.Providers.TogetherAI.BaseURL
	case types.ProviderVLLM:
		baseURL = s.Cfg.Providers.VLLM.BaseURL
	case types.ProviderHelix:
		baseURL = "internal"
	}

	return &types.ProviderEndpoint{
		ID:             types.GlobalProviderID(string(provider)),
		Name:           string(provider),
		Description:    "",
		BaseURL:        baseURL,
		EndpointType:   types.ProviderEndpointTypeGlobal,
		Owner:          string(types.OwnerTypeSystem),
		APIKey:         "",
		BillingEnabled: s.Cfg.Providers.BillingEnabled, // Controlled by PROVIDERS_BILLING_ENABLED env var
	}
}

// resolveModelProviderLive is the last-resort routing resolver for
// /v1/chat/completions. The fast path (findProviderWithModel) only reads the
// per-provider model cache, which is populated lazily by /v1/models and the
// model-picker fetch. When neither has run yet — a fresh API process, or a
// downstream Helix that forwards a prefix-stripped bare model id to an upstream
// Helix configured as a provider — the cache is cold, the bare id has no
// usable provider prefix to parse, and routing would otherwise fall through to
// the default ("helix") provider and 500 with "model X is not configured in the
// default provider". This resolver warms each accessible provider's model list
// via the shared stale-while-revalidate getProviderModels path (singleflighted,
// cached, 3s timeout) and returns the provider that actually serves modelName
// plus the bare upstream id. Returns ("", "") when no provider serves it.
//
// Only called after the cache-only lookup and prefix parsing both fail, so the
// common (prefixed, or cache-warm) requests never pay the live-fetch cost. The
// "helix" provider is skipped: it is the default that already failed, and
// enumerating runner models here adds nothing.
func (s *HelixAPIServer) resolveModelProviderLive(ctx context.Context, modelName, ownerID, orgID string) (string, string) {
	var endpoints []*types.ProviderEndpoint

	dbProviders, err := s.Store.ListProviderEndpoints(ctx, &store.ListProviderEndpointsQuery{
		Owner:      ownerID,
		OwnerType:  types.OwnerTypeUser,
		WithGlobal: true,
	})
	if err != nil {
		log.Warn().Err(err).Msg("live model resolution: failed to list provider endpoints")
	} else {
		endpoints = append(endpoints, dbProviders...)
	}

	if orgID != "" && orgID != ownerID {
		if orgProviders, err := s.Store.ListProviderEndpoints(ctx, &store.ListProviderEndpointsQuery{
			Owner:     orgID,
			OwnerType: types.OwnerTypeOrg,
		}); err == nil {
			endpoints = append(endpoints, orgProviders...)
		}
	}

	existing := make(map[string]bool)
	for _, ep := range endpoints {
		existing[ep.Name] = true
	}
	if globals, err := s.providerManager.ListProviders(ctx, ""); err == nil {
		for _, g := range globals {
			if existing[string(g)] {
				continue
			}
			endpoints = append(endpoints, s.globalProviderEndpoint(g))
		}
	}

	for _, ep := range endpoints {
		if ep.Name == string(types.ProviderHelix) {
			continue
		}
		residue := modelName
		if strings.HasPrefix(modelName, ep.Name+"/") {
			residue = modelName[len(ep.Name)+1:]
		}
		pm, err := s.getProviderModels(ctx, ep)
		if err != nil {
			continue
		}
		for _, m := range pm.Models {
			if m.ID == modelName || m.ID == residue {
				log.Debug().
					Str("model", modelName).
					Str("provider", ep.Name).
					Str("bare_model", residue).
					Msg("resolved provider via live model lookup (cache was cold)")
				return ep.Name, residue
			}
		}
	}

	return "", ""
}

// invalidateProviderModelCache clears the cached model list for the given
// provider so subsequent reads refetch from upstream. Call this whenever a
// provider's identity (name) or content (BaseURL, Models, APIKey, billing
// flags) changes, including deletes. Without this, a deleted/renamed/edited
// provider continues serving stale models for up to modelCacheTTL.
func (s *HelixAPIServer) invalidateProviderModelCache(name, owner string) {
	s.cache.Del(modelCacheKey(name, owner))
	s.cache.Del(catalogueCacheKey(name, owner))
}

// cachedModels is the cache payload — a model list plus the time it was
// fetched. Freshness is decided by the timestamp on read, not by the cache
// entry's TTL, so a single entry can serve as both the "fresh" copy and the
// stale-while-revalidate fallback. This keeps invalidation atomic (one key)
// and avoids the race window in a fresh-key / stale-key split.
type cachedModels struct {
	Models    []types.OpenAIModel `json:"models"`
	FetchedAt time.Time           `json:"fetched_at"`
}

// ProviderModels is the result of getProviderModels.
type ProviderModels struct {
	Models []types.OpenAIModel
	// Degraded is non-nil when Models came from the cache after an upstream
	// refresh failed. The picker stays populated and the API response carries
	// Status=error + Error=<reason> so the UI can show a degraded marker.
	Degraded error
}

// getProviderModels returns a provider's model list with stale-while-revalidate
// semantics:
//
//   - If a cached payload exists and is younger than ModelsCacheTTL, return it
//     as fresh (Degraded=nil).
//   - Otherwise refresh from upstream. On success, cache and return.
//   - On a transient upstream failure, fall back to the cached payload if one
//     exists and mark Degraded with the underlying error.
//   - Provider-client construction errors are NOT considered transient — they
//     mean the provider is misconfigured (missing key, bad URL, deleted row)
//     and serving cached models would mask the real failure. Those propagate
//     as a hard error even if we have cached data.
func (s *HelixAPIServer) getProviderModels(ctx context.Context, providerEndpoint *types.ProviderEndpoint) (ProviderModels, error) {
	return s.readProviderModels(ctx, providerEndpoint, modelCacheKey(providerEndpoint.Name, providerEndpoint.Owner))
}

// getProviderCatalogue returns the provider's FULL upstream model list,
// ignoring the endpoint's enabled-models whitelist. Only the models-picker UI
// may use this: it is what the operator chooses from. Every other consumer
// must go through getProviderModels so the whitelist is honoured.
func (s *HelixAPIServer) getProviderCatalogue(ctx context.Context, providerEndpoint *types.ProviderEndpoint) (ProviderModels, error) {
	return s.readProviderModels(ctx, providerEndpoint, catalogueCacheKey(providerEndpoint.Name, providerEndpoint.Owner))
}

// readProviderModels implements the stale-while-revalidate read for one of the
// two cache keys a refresh populates (effective list vs full catalogue).
func (s *HelixAPIServer) readProviderModels(ctx context.Context, providerEndpoint *types.ProviderEndpoint, key string) (ProviderModels, error) {
	cached, hit := s.loadCachedModels(key)
	if hit && time.Since(cached.FetchedAt) < s.Cfg.WebServer.ModelsCacheTTL {
		return ProviderModels{Models: cached.Models}, nil
	}

	fresh, refreshErr := s.refreshProviderModels(ctx, providerEndpoint, key)
	switch {
	case refreshErr == nil:
		return ProviderModels{Models: fresh}, nil
	case hit && errors.Is(refreshErr, errUpstreamUnreachable):
		return ProviderModels{Models: cached.Models, Degraded: refreshErr}, nil
	default:
		return ProviderModels{}, refreshErr
	}
}

// refreshProviderModels fetches a fresh model list from upstream, populates
// both cache entries (the full catalogue and the whitelist-filtered effective
// list) and returns the one belonging to `key`.
//
// On failure the returned error wraps errUpstreamUnreachable iff the failure
// was in the /v1/models call itself (so callers may fall back to the cache);
// other errors (provider construction, JSON marshal) are hard failures.
func (s *HelixAPIServer) refreshProviderModels(ctx context.Context, providerEndpoint *types.ProviderEndpoint, key string) ([]types.OpenAIModel, error) {
	// Double-check the cache — another caller may have populated it while we
	// were queued behind the upstream fetch.
	if cached, hit := s.loadCachedModels(key); hit && time.Since(cached.FetchedAt) < s.Cfg.WebServer.ModelsCacheTTL {
		return cached.Models, nil
	}

	catalogueKey := catalogueCacheKey(providerEndpoint.Name, providerEndpoint.Owner)
	wantCatalogue := key == catalogueKey

	catalogue, fetchErr := s.fetchUpstreamModels(ctx, providerEndpoint)
	if fetchErr != nil {
		// Custom endpoints often don't expose /v1/models. If an explicit model
		// list is configured, treat that as the source of truth so the picker
		// and chat-completions routing both work. The catalogue read can't use
		// that fallback — it exists to show what the upstream offers, so an
		// unreachable upstream must surface as an error, not an empty list.
		if wantCatalogue || len(providerEndpoint.Models) == 0 || !errors.Is(fetchErr, errUpstreamUnreachable) {
			return nil, fetchErr
		}
		log.Debug().
			Err(fetchErr).
			Str("provider", providerEndpoint.Name).
			Str("owner", providerEndpoint.Owner).
			Strs("enabled_models", providerEndpoint.Models).
			Msg("upstream /v1/models failed; using the endpoint's configured model list")
		catalogue = nil
	}

	fetchedAt := time.Now()
	if fetchErr == nil {
		s.storeCachedModels(catalogueKey, catalogue, fetchedAt)
	}

	effective := s.decorateModels(ctx, providerEndpoint, applyModelWhitelist(providerEndpoint, catalogue))
	s.storeCachedModels(modelCacheKey(providerEndpoint.Name, providerEndpoint.Owner), effective, fetchedAt)

	if wantCatalogue {
		return catalogue, nil
	}
	return effective, nil
}

// fetchUpstreamModels calls the provider's /v1/models. Concurrent calls for the
// same BaseURL are collapsed into one upstream request — different endpoints
// (names/owners/whitelists) can share a URL, and the raw catalogue is identical
// for all of them. Per-endpoint work (whitelist, pricing, billing) happens
// outside this singleflight, where it can't leak between endpoints.
func (s *HelixAPIServer) fetchUpstreamModels(ctx context.Context, providerEndpoint *types.ProviderEndpoint) ([]types.OpenAIModel, error) {
	result, err, _ := s.modelFetchGroup.Do(providerEndpoint.BaseURL, func() (interface{}, error) {
		providerRef := providerEndpoint.Name
		if providerEndpoint.ID != "" {
			providerRef = providerEndpoint.ID
		}
		clientRequest := &manager.GetClientRequest{
			Provider: providerRef,
			Owner:    providerEndpoint.Owner,
		}
		if providerEndpoint.OwnerType == types.OwnerTypeOrg {
			clientRequest.OwnerType = types.OwnerTypeOrg
		}
		provider, err := s.providerManager.GetClient(ctx, clientRequest)
		if err != nil {
			log.Err(err).
				Str("provider", providerEndpoint.Name).
				Str("owner", providerEndpoint.Owner).
				Msg("error getting provider")
			// Not wrapped as errUpstreamUnreachable: this is a config problem,
			// not a transient outage. Falling back to cached models here would
			// hide a misconfigured/deleted provider from the user.
			return nil, err
		}

		// Models should respond in 3 seconds or less, otherwise we'll kill the request
		fetchCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
		defer cancel()

		models, listErr := provider.ListModels(fetchCtx)
		if listErr != nil {
			log.Err(listErr).
				Str("provider", providerEndpoint.Name).
				Str("owner", providerEndpoint.Owner).
				Msg("error listing models")
			// Wrap as errUpstreamUnreachable so the caller may serve from
			// cache. fmt.Errorf with two %w preserves both sentinels for
			// errors.Is checks.
			return nil, fmt.Errorf("%w: %w", errUpstreamUnreachable, listErr)
		}
		return models, nil
	})
	if err != nil {
		return nil, err
	}
	return result.([]types.OpenAIModel), nil
}

// applyModelWhitelist reduces a provider's upstream catalogue to the models the
// operator enabled on the endpoint. An empty list means "everything upstream
// offers" — the default for a freshly added provider. Enabled ids the upstream
// doesn't advertise are still returned, synthesized: that is what makes a
// custom endpoint with no working /v1/models usable, and it keeps a pinned
// model working when an aggregator temporarily drops it from its listing.
func applyModelWhitelist(providerEndpoint *types.ProviderEndpoint, catalogue []types.OpenAIModel) []types.OpenAIModel {
	if len(providerEndpoint.Models) == 0 {
		return catalogue
	}

	byID := make(map[string]types.OpenAIModel, len(catalogue))
	for _, m := range catalogue {
		byID[m.ID] = m
	}

	models := make([]types.OpenAIModel, 0, len(providerEndpoint.Models))
	seen := make(map[string]bool, len(providerEndpoint.Models))
	for _, id := range providerEndpoint.Models {
		if id == "" || seen[id] {
			continue
		}
		seen[id] = true
		if m, ok := byID[id]; ok {
			models = append(models, m)
			continue
		}
		models = append(models, types.OpenAIModel{
			ID:      id,
			Object:  "model",
			OwnedBy: providerEndpoint.Name,
			Type:    "chat",
			Enabled: true,
		})
	}
	return models
}

// decorateModels attaches per-endpoint metadata: pricing (from the model info
// catalogue), reasoning-effort capability, and the billing gate. This is
// deliberately outside the upstream singleflight — BillingEnabled is a property
// of the endpoint, not of the URL, so two endpoints sharing an upstream must
// not inherit each other's decoration.
func (s *HelixAPIServer) decorateModels(ctx context.Context, providerEndpoint *types.ProviderEndpoint, models []types.OpenAIModel) []types.OpenAIModel {
	for idx, m := range models {
		modelInfo, err := s.modelInfoProvider.GetModelInfo(ctx, &model.ModelInfoRequest{
			BaseURL:  providerEndpoint.BaseURL,
			Provider: providerEndpoint.Name,
			Model:    m.ID,
		})
		if err == nil {
			models[idx].ModelInfo = modelInfo
		}

		// Effort capability is resolved independently of the pricing
		// catalogue: self-hosted models (vLLM and friends) have no catalogue
		// entry and never will, but we still know what efforts they accept.
		if profile, ok := model.LookupReasoningEfforts(m.ID); ok {
			models[idx].ReasoningEfforts = profile
		}

		// If billing is enabled and we don't have pricing, disable the model
		if providerEndpoint.BillingEnabled {
			if modelInfo == nil {
				models[idx].Enabled = false
				continue
			}
			// Got model info, checking the price
			cost, _ := pricing.CalculateTokenPrice(modelInfo, pricing.TokenUsage{
				PromptTokens:     10,
				CompletionTokens: 10,
			})
			if cost.PromptCost == 0 && cost.CompletionCost == 0 {
				models[idx].Enabled = false
			}
		}
	}
	return models
}

// storeCachedModels writes a model list to the cache. An empty list gets a much
// shorter TTL — see emptyModelCacheTTL.
func (s *HelixAPIServer) storeCachedModels(key string, models []types.OpenAIModel, fetchedAt time.Time) {
	payload, err := json.Marshal(cachedModels{Models: models, FetchedAt: fetchedAt})
	if err != nil {
		log.Warn().Err(err).Str("cache_key", key).Msg("failed to marshal provider models for cache")
		return
	}
	ttl := modelCacheTTL
	if len(models) == 0 {
		ttl = emptyModelCacheTTL
	}
	s.cache.SetWithTTL(key, string(payload), 1, ttl)
}

// loadCachedModels reads and parses the cache payload. A corrupt entry is
// deleted so the next read can repopulate it cleanly rather than tripping the
// same unmarshal error forever.
func (s *HelixAPIServer) loadCachedModels(key string) (cachedModels, bool) {
	raw, found := s.cache.Get(key)
	if !found {
		return cachedModels{}, false
	}
	var c cachedModels
	if err := json.Unmarshal([]byte(raw), &c); err != nil {
		log.Warn().Err(err).Str("cache_key", key).Msg("provider models cache entry corrupt; dropping")
		s.cache.Del(key)
		return cachedModels{}, false
	}
	return c, true
}

// createProviderEndpoint godoc
// @Summary Create a new provider endpoint
// @Description Create a new provider endpoint
// @Tags    providers

// @Success 200 {object} types.ProviderEndpoint
// @Router /api/v1/provider-endpoints [post]
// @Security BearerAuth
func (s *HelixAPIServer) createProviderEndpoint(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	user := getRequestUser(r)

	isAdmin := s.isAdmin(r)

	// Check if providers management is enabled
	if !s.isProvidersManagementEnabled(ctx) && !isAdmin {
		http.Error(rw, "Providers management is not enabled", http.StatusForbidden)
		return
	}

	var endpoint types.ProviderEndpoint
	if err := json.NewDecoder(r.Body).Decode(&endpoint); err != nil {
		log.Err(err).Msg("error decoding request body")
		http.Error(rw, "Invalid request body: "+err.Error(), http.StatusBadRequest)
		return
	}

	// If org ID is set, authorize
	if endpoint.OwnerType == types.OwnerTypeOrg && endpoint.Owner != "" {
		_, err := s.authorizeOrgOwner(r.Context(), user, endpoint.Owner)
		if err != nil {
			log.Err(err).Msg("error authorizing org member")
			http.Error(rw, "Could not authorize org member: "+err.Error(), http.StatusForbidden)
			return
		}
	} else {
		// Otherwise, default to user
		endpoint.OwnerType = types.OwnerTypeUser
		endpoint.Owner = user.ID
	}

	// Default endpoint visibility to the authenticated owner scope.
	if endpoint.EndpointType == "" {
		if endpoint.OwnerType == types.OwnerTypeOrg {
			endpoint.EndpointType = types.ProviderEndpointTypeOrg
		} else {
			endpoint.EndpointType = types.ProviderEndpointTypeUser
		}
	}

	switch endpoint.EndpointType {
	case types.ProviderEndpointTypeGlobal:
		if !isAdmin {
			http.Error(rw, "Only admins can add global endpoints", http.StatusForbidden)
			return
		}
	case types.ProviderEndpointTypeOrg:
		if endpoint.OwnerType != types.OwnerTypeOrg {
			http.Error(rw, "Organization provider endpoints require an organization owner", http.StatusBadRequest)
			return
		}
	case types.ProviderEndpointTypeUser:
		if endpoint.OwnerType != types.OwnerTypeUser {
			http.Error(rw, "Personal provider endpoints require a user owner", http.StatusBadRequest)
			return
		}
	default:
		http.Error(rw, fmt.Sprintf("Unsupported endpoint type %q", endpoint.EndpointType), http.StatusBadRequest)
		return
	}

	// Only admins can add endpoints with API key path auth
	if endpoint.APIKeyFromFile != "" && !isAdmin {
		http.Error(rw, "Only admins can add endpoints with API key path auth", http.StatusForbidden)
		return
	}

	endpoint.Name = strings.TrimSpace(endpoint.Name)
	duplicate, err := s.providerEndpointNameExists(ctx, &endpoint, "")
	if err != nil {
		log.Err(err).Msg("error listing providers for name validation")
		http.Error(rw, "Internal server error: "+err.Error(), http.StatusInternalServerError)
		return
	}
	if duplicate {
		http.Error(rw, fmt.Sprintf("Provider with name '%s' already exists", endpoint.Name), http.StatusBadRequest)
		return
	}

	createdEndpoint, err := s.Store.CreateProviderEndpoint(ctx, &endpoint)
	if err != nil {
		log.Err(err).Msg("error creating provider endpoint")
		http.Error(rw, "Error creating provider endpoint: "+err.Error(), http.StatusInternalServerError)
		return
	}

	// Mask API key before any concurrent access to createdEndpoint.
	endpointForWarm := *createdEndpoint // copy — goroutine needs the real API key
	createdEndpoint.APIKey = "*****"

	// Warm the model cache asynchronously so the first ?with_models=true request is instant.
	// Use a detached context so the HTTP request completing doesn't cancel the fetch.
	go func() {
		warmCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if _, err := s.getProviderModels(warmCtx, &endpointForWarm); err != nil {
			log.Warn().Err(err).Str("provider", endpointForWarm.Name).Msg("model cache warm failed after provider create (provider may not be reachable yet)")
		}
	}()

	rw.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(rw).Encode(createdEndpoint); err != nil {
		log.Err(err).Msg("error writing response")
		http.Error(rw, "Internal server error", http.StatusInternalServerError)
		return
	}
}

func (s *HelixAPIServer) providerEndpointNameExists(ctx context.Context, endpoint *types.ProviderEndpoint, excludeID string) (bool, error) {
	query := &store.ListProviderEndpointsQuery{Owner: endpoint.Owner}
	switch endpoint.EndpointType {
	case types.ProviderEndpointTypeGlobal:
		query.Owner = ""
		query.WithGlobal = true
	case types.ProviderEndpointTypeOrg:
		query.OwnerType = types.OwnerTypeOrg
	case types.ProviderEndpointTypeUser:
		query.OwnerType = types.OwnerTypeUser
	default:
		return false, fmt.Errorf("unsupported provider endpoint type %q", endpoint.EndpointType)
	}
	endpoints, err := s.Store.ListProviderEndpoints(ctx, query)
	if err != nil {
		return false, err
	}
	wantedName := types.CanonicalProviderName(strings.TrimSpace(endpoint.Name))
	for _, existing := range endpoints {
		if existing == nil || existing.ID == excludeID || existing.EndpointType != endpoint.EndpointType {
			continue
		}
		if endpoint.EndpointType != types.ProviderEndpointTypeGlobal && existing.Owner != endpoint.Owner {
			continue
		}
		if types.CanonicalProviderName(strings.TrimSpace(existing.Name)) == wantedName {
			return true, nil
		}
	}
	return false, nil
}

// updateProviderEndpoint godoc
// @Summary Update a provider endpoint
// @Description Update a provider endpoint. Global endpoints can only be updated by admins.
// @Tags    providers

// @Success 200 {object} types.UpdateProviderEndpoint
// @Router /api/v1/provider-endpoints/{id} [put]
// @Security BearerAuth
func (s *HelixAPIServer) updateProviderEndpoint(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	user := getRequestUser(r)
	endpointID := mux.Vars(r)["id"]

	// Check if providers management is enabled
	if !s.isProvidersManagementEnabled(r.Context()) && !user.Admin {
		http.Error(rw, "Providers management is not enabled", http.StatusForbidden)
		return
	}

	// Get existing endpoint
	existingEndpoint, err := s.Store.GetProviderEndpoint(ctx, &store.GetProviderEndpointsQuery{ID: endpointID})
	if err != nil {
		if err == store.ErrNotFound {
			http.Error(rw, "Provider endpoint not found", http.StatusNotFound)
			return
		}
		log.Err(err).Msg("error getting provider endpoint")
		http.Error(rw, "Error getting provider endpoint: "+err.Error(), http.StatusInternalServerError)
		return
	}

	// If endpoint is org endpoint, authorize org owner
	if existingEndpoint.OwnerType == types.OwnerTypeOrg {
		_, err := s.authorizeOrgOwner(r.Context(), user, existingEndpoint.Owner)
		if err != nil {
			log.Err(err).Msg("error authorizing org member")
			http.Error(rw, "Could not authorize org member: "+err.Error(), http.StatusForbidden)
			return
		}
	}

	// Check ownership - only allow updates to owned endpoints or if user is admin
	if existingEndpoint.Owner != user.ID && !user.Admin {
		http.Error(rw, "Unauthorized", http.StatusUnauthorized)
		return
	}

	var updatedEndpoint types.UpdateProviderEndpoint
	if err := json.NewDecoder(r.Body).Decode(&updatedEndpoint); err != nil {
		log.Err(err).Msg("error decoding request body")
		http.Error(rw, "Invalid request body: "+err.Error(), http.StatusBadRequest)
		return
	}

	// For global endpoints, only allow updates by admins. Gate on the stored
	// row, not the request payload: a global endpoint now retains its original
	// (possibly non-admin) owner, so the ownership check above no longer blocks
	// that owner. If we only checked updatedEndpoint.EndpointType, a request that
	// omits endpoint_type would skip this gate entirely and let the owner edit a
	// globally-shared endpoint.
	if (existingEndpoint.EndpointType == types.ProviderEndpointTypeGlobal ||
		updatedEndpoint.EndpointType == types.ProviderEndpointTypeGlobal) && !user.Admin {
		http.Error(rw, "Only admins can update global endpoints", http.StatusForbidden)
		return
	}

	if updatedEndpoint.BillingEnabled != nil && !user.Admin {
		http.Error(rw, "Only admins can change provider billing", http.StatusForbidden)
		return
	}

	// Only admins can add endpoints with API key path auth
	if existingEndpoint.APIKeyFromFile != "" && !user.Admin {
		http.Error(rw, "Only admins can add endpoints with API key path auth", http.StatusForbidden)
		return
	}

	// Capture identity BEFORE mutation so we can invalidate the cache entry
	// keyed under the old name/owner — a rename leaves the old entry stranded
	// otherwise.
	prevName, prevOwner := existingEndpoint.Name, existingEndpoint.Owner

	// Apply endpoint type change. Keep the existing owner — global visibility is
	// keyed on endpoint_type='global' in the store, not on owner, so there is no
	// reason to reassign ownership. Stamping Owner="system" here made a real DB
	// row indistinguishable from a synthetic env-var endpoint (ID="-", Owner=
	// "system"), which the UI treats as read-only, permanently stranding it. This
	// now matches createProviderEndpoint, which leaves a global endpoint owned by
	// its admin creator.
	identityChanged := false
	if updatedEndpoint.EndpointType != "" && updatedEndpoint.EndpointType != existingEndpoint.EndpointType {
		switch updatedEndpoint.EndpointType {
		case types.ProviderEndpointTypeGlobal:
			existingEndpoint.EndpointType = updatedEndpoint.EndpointType
			identityChanged = true
		default:
			http.Error(rw, fmt.Sprintf("Unsupported endpoint type switch to %q", updatedEndpoint.EndpointType), http.StatusBadRequest)
			return
		}
	}

	// Preserve ID and ownership information
	// Update name if provided and different from existing
	if updatedEndpoint.Name != "" && strings.TrimSpace(updatedEndpoint.Name) != existingEndpoint.Name {
		newName := strings.TrimSpace(updatedEndpoint.Name)
		existingEndpoint.Name = newName
		identityChanged = true
	}
	if identityChanged {
		duplicate, err := s.providerEndpointNameExists(ctx, existingEndpoint, existingEndpoint.ID)
		if err != nil {
			log.Err(err).Msg("error listing providers for name validation")
			http.Error(rw, "Internal server error: "+err.Error(), http.StatusInternalServerError)
			return
		}
		if duplicate {
			http.Error(rw, fmt.Sprintf("Provider with name '%s' already exists", existingEndpoint.Name), http.StatusBadRequest)
			return
		}
	}
	existingEndpoint.Description = updatedEndpoint.Description
	if updatedEndpoint.Icon != nil {
		existingEndpoint.Icon = strings.TrimSpace(*updatedEndpoint.Icon)
	}
	if updatedEndpoint.BillingEnabled != nil {
		existingEndpoint.BillingEnabled = *updatedEndpoint.BillingEnabled
	}
	existingEndpoint.Models = updatedEndpoint.Models
	existingEndpoint.BaseURL = strings.TrimSpace(updatedEndpoint.BaseURL)
	// A nil map means the caller left headers out of the request; an empty map
	// means "remove every header". Both are legitimate, so only the nil case
	// keeps the stored headers.
	if updatedEndpoint.Headers != nil {
		existingEndpoint.Headers = updatedEndpoint.Headers
	}
	if updatedEndpoint.APIKey != nil {
		existingEndpoint.APIKey = strings.TrimSpace(*updatedEndpoint.APIKey)
	}

	if updatedEndpoint.APIKeyFromFile != nil {
		existingEndpoint.APIKeyFromFile = strings.TrimSpace(*updatedEndpoint.APIKeyFromFile)
	}

	switch {
	case updatedEndpoint.APIKey != nil:
		// If from key, clear the API key file
		existingEndpoint.APIKey = strings.TrimSpace(*updatedEndpoint.APIKey)
		existingEndpoint.APIKeyFromFile = ""
	case updatedEndpoint.APIKeyFromFile != nil:
		// If from file, clear the API key
		existingEndpoint.APIKeyFromFile = strings.TrimSpace(*updatedEndpoint.APIKeyFromFile)
		existingEndpoint.APIKey = ""
	}

	// Update Vertex AI fields if provided
	if updatedEndpoint.VertexProjectID != nil {
		existingEndpoint.VertexProjectID = strings.TrimSpace(*updatedEndpoint.VertexProjectID)
	}
	if updatedEndpoint.VertexRegion != nil {
		existingEndpoint.VertexRegion = strings.TrimSpace(*updatedEndpoint.VertexRegion)
	}
	if updatedEndpoint.VertexCredentialsJSON != nil {
		existingEndpoint.VertexCredentialsJSON = strings.TrimSpace(*updatedEndpoint.VertexCredentialsJSON)
	}
	if updatedEndpoint.VertexCredentialsFile != nil {
		existingEndpoint.VertexCredentialsFile = strings.TrimSpace(*updatedEndpoint.VertexCredentialsFile)
	}

	// Update the endpoint
	savedEndpoint, err := s.Store.UpdateProviderEndpoint(ctx, existingEndpoint)
	if err != nil {
		log.Err(err).Msg("error updating provider endpoint")
		http.Error(rw, "Error updating provider endpoint: "+err.Error(), http.StatusInternalServerError)
		return
	}

	// Invalidate the model cache for both the old identity (covers renames /
	// owner-type changes leaving a stranded entry) and the new identity (covers
	// BaseURL / Models / APIKey edits where the key didn't change but the
	// upstream might now return a different model list).
	s.invalidateProviderModelCache(prevName, prevOwner)
	if savedEndpoint.Name != prevName || savedEndpoint.Owner != prevOwner {
		s.invalidateProviderModelCache(savedEndpoint.Name, savedEndpoint.Owner)
	}

	// Mask API key in response
	savedEndpoint.APIKey = "*****"

	rw.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(rw).Encode(savedEndpoint); err != nil {
		log.Err(err).Msg("error writing response")
		http.Error(rw, "Internal server error", http.StatusInternalServerError)
		return
	}
}

// loadEditableProviderEndpoint loads a provider endpoint and authorizes the
// caller to change it. Same rules as updateProviderEndpoint: providers
// management must be on (or the caller an admin), org endpoints need org
// ownership, user endpoints need to be the caller's own, and global endpoints
// are admin-only.
func (s *HelixAPIServer) loadEditableProviderEndpoint(rw http.ResponseWriter, r *http.Request) (*types.ProviderEndpoint, bool) {
	ctx := r.Context()
	user := getRequestUser(r)

	if !s.isProvidersManagementEnabled(ctx) && !user.Admin {
		http.Error(rw, "Providers management is not enabled", http.StatusForbidden)
		return nil, false
	}

	endpoint, err := s.Store.GetProviderEndpoint(ctx, &store.GetProviderEndpointsQuery{ID: getID(r)})
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			http.Error(rw, "Provider endpoint not found", http.StatusNotFound)
			return nil, false
		}
		log.Err(err).Msg("error getting provider endpoint")
		http.Error(rw, "Error getting provider endpoint: "+err.Error(), http.StatusInternalServerError)
		return nil, false
	}

	if endpoint.OwnerType == types.OwnerTypeOrg {
		if _, err := s.authorizeOrgOwner(ctx, user, endpoint.Owner); err != nil {
			log.Err(err).Msg("error authorizing org member")
			http.Error(rw, "Could not authorize org member: "+err.Error(), http.StatusForbidden)
			return nil, false
		}
	} else if endpoint.Owner != user.ID && !user.Admin {
		http.Error(rw, "Unauthorized", http.StatusUnauthorized)
		return nil, false
	}

	if endpoint.EndpointType == types.ProviderEndpointTypeGlobal && !user.Admin {
		http.Error(rw, "Only admins can update global endpoints", http.StatusForbidden)
		return nil, false
	}

	return endpoint, true
}

// listProviderEndpointModels godoc
// @Summary List a provider endpoint's full model catalogue
// @Description Returns every model the upstream provider advertises, plus the subset currently enabled on the endpoint. Aggregators such as OpenRouter list hundreds of models, so this is deliberately separate from the endpoint's effective (enabled-only) model list.
// @Tags    providers
// @Produce json
// @Param   id path string true "Provider endpoint ID"
// @Param   refresh query bool false "Bypass the cached catalogue and refetch from upstream"
// @Success 200 {object} types.ProviderEndpointModels
// @Failure 403 {object} system.HTTPError
// @Failure 404 {object} system.HTTPError
// @Failure 502 {object} system.HTTPError
// @Router /api/v1/provider-endpoints/{id}/available-models [get]
// @Security BearerAuth
func (s *HelixAPIServer) listProviderEndpointModels(rw http.ResponseWriter, r *http.Request) {
	endpoint, ok := s.loadEditableProviderEndpoint(rw, r)
	if !ok {
		return
	}

	if r.URL.Query().Get("refresh") == "true" {
		s.invalidateProviderModelCache(endpoint.Name, endpoint.Owner)
	}

	catalogue, err := s.getProviderCatalogue(r.Context(), endpoint)
	if err != nil {
		log.Warn().Err(err).
			Str("provider", endpoint.Name).
			Msg("error listing models for provider endpoint")
		writeErrResponse(rw, fmt.Errorf("could not list models from %s: %w", endpoint.Name, err), http.StatusBadGateway)
		return
	}

	enabled := endpoint.Models
	if enabled == nil {
		enabled = []string{}
	}

	writeResponse(rw, types.ProviderEndpointModels{
		Models:        catalogue.Models,
		EnabledModels: enabled,
	}, http.StatusOK)
}

// updateProviderEndpointModels godoc
// @Summary Set the models enabled on a provider endpoint
// @Description Replaces the endpoint's enabled-models whitelist. An empty list enables the provider's whole catalogue.
// @Tags    providers
// @Accept  json
// @Produce json
// @Param   id path string true "Provider endpoint ID"
// @Param   request body types.UpdateProviderEndpointModels true "Enabled models"
// @Success 200 {object} types.ProviderEndpoint
// @Failure 400 {object} system.HTTPError
// @Failure 403 {object} system.HTTPError
// @Failure 404 {object} system.HTTPError
// @Router /api/v1/provider-endpoints/{id}/models [put]
// @Security BearerAuth
func (s *HelixAPIServer) updateProviderEndpointModels(rw http.ResponseWriter, r *http.Request) {
	endpoint, ok := s.loadEditableProviderEndpoint(rw, r)
	if !ok {
		return
	}

	var req types.UpdateProviderEndpointModels
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(rw, "Invalid request body: "+err.Error(), http.StatusBadRequest)
		return
	}

	models := make([]string, 0, len(req.Models))
	seen := make(map[string]bool, len(req.Models))
	for _, m := range req.Models {
		m = strings.TrimSpace(m)
		if m == "" || seen[m] {
			continue
		}
		seen[m] = true
		models = append(models, m)
	}
	endpoint.Models = models

	saved, err := s.Store.UpdateProviderEndpoint(r.Context(), endpoint)
	if err != nil {
		log.Err(err).Msg("error updating provider endpoint models")
		http.Error(rw, "Error updating provider endpoint: "+err.Error(), http.StatusInternalServerError)
		return
	}

	// The effective model list is derived from the whitelist, so it has to be
	// refetched — otherwise the picker keeps serving the previous selection for
	// up to modelCacheTTL.
	s.invalidateProviderModelCache(saved.Name, saved.Owner)

	saved.APIKey = "*****"
	writeResponse(rw, saved, http.StatusOK)
}

// deleteProviderEndpoint godoc
// @Summary Delete a provider endpoint
// @Description Delete a provider endpoint. Global endpoints cannot be deleted.
// @Tags    providers

// @Success 204 "No Content"
// @Router /api/v1/provider-endpoints/{id} [delete]
// @Security BearerAuth
func (s *HelixAPIServer) deleteProviderEndpoint(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	user := getRequestUser(r)
	endpointID := mux.Vars(r)["id"]

	// Check if providers management is enabled
	if !s.isProvidersManagementEnabled(r.Context()) && !user.Admin {
		http.Error(rw, "Providers management is not enabled", http.StatusForbidden)
		return
	}

	// Get existing endpoint
	existingEndpoint, err := s.Store.GetProviderEndpoint(ctx, &store.GetProviderEndpointsQuery{ID: endpointID})
	if err != nil {
		if err == store.ErrNotFound {
			http.Error(rw, "Provider endpoint not found", http.StatusNotFound)
			return
		}
		log.Err(err).Msg("error getting provider endpoint")
		http.Error(rw, "Error getting provider endpoint: "+err.Error(), http.StatusInternalServerError)
		return
	}

	// Prevent deletion of global endpoints
	if existingEndpoint.EndpointType == types.ProviderEndpointTypeGlobal && !s.isAdmin(r) {
		http.Error(rw, "Global endpoints cannot be deleted", http.StatusForbidden)
		return
	}

	// If endpoint is org endpoint, authorize org owner
	if existingEndpoint.OwnerType == types.OwnerTypeOrg {
		_, err := s.authorizeOrgOwner(r.Context(), user, existingEndpoint.Owner)
		if err != nil {
			log.Err(err).Msg("error authorizing org member")
			http.Error(rw, "Could not authorize org member: "+err.Error(), http.StatusForbidden)
			return
		}
	} else {
		// Check ownership - only allow deletion of owned endpoints or if user is admin
		if existingEndpoint.Owner != user.ID && !user.Admin {
			http.Error(rw, "Unauthorized", http.StatusUnauthorized)
			return
		}
	}

	if err := s.Store.DeleteProviderEndpoint(ctx, endpointID); err != nil {
		log.Err(err).Msg("error deleting provider endpoint")
		http.Error(rw, "Error deleting provider endpoint: "+err.Error(), http.StatusInternalServerError)
		return
	}

	// Invalidate the model-list cache so the model picker, /v1/chat/completions
	// routing, and /api/v1/providers/models stop serving the deleted provider's
	// models. Without this, stale entries linger for up to ModelsCacheTTL and
	// requests during that window can still resolve through a deleted provider.
	s.invalidateProviderModelCache(existingEndpoint.Name, existingEndpoint.Owner)

	rw.WriteHeader(http.StatusOK)
}

// getProviderDailyUsage godoc
// @Summary Get provider daily usage
// @Description Get provider daily usage
// @Accept json
// @Produce json
// @Tags    providers
// @Param   id path string true "Provider ID"
// @Param   from query string false "Start date"
// @Param   to query string false "End date"
// @Success 200 {array} types.AggregatedUsageMetric
// @Failure 400 {object} system.HTTPError
// @Failure 404 {object} system.HTTPError
// @Failure 500 {object} system.HTTPError
// @Router /api/v1/provider-endpoints/{id}/daily-usage [get]
// @Security BearerAuth
func (s *HelixAPIServer) getProviderDailyUsage(rw http.ResponseWriter, r *http.Request) {
	user := getRequestUser(r)
	id := getID(r)

	// Check if providers management is enabled
	if !s.isProvidersManagementEnabled(r.Context()) && !user.Admin {
		writeErrResponse(rw, errors.New("providers management is not enabled"), http.StatusForbidden)
		return
	}

	from := time.Now().Add(-time.Hour * 24 * 7) // Last 7 days
	to := time.Now()

	var err error

	if r.URL.Query().Get("from") != "" {
		from, err = time.Parse(time.RFC3339, r.URL.Query().Get("from"))
		if err != nil {
			writeErrResponse(rw, fmt.Errorf("failed to parse from date: %w", err), http.StatusBadRequest)
			return
		}
	}

	if r.URL.Query().Get("to") != "" {
		to, err = time.Parse(time.RFC3339, r.URL.Query().Get("to"))
		if err != nil {
			writeErrResponse(rw, fmt.Errorf("failed to parse to date: %w", err), http.StatusBadRequest)
			return
		}
	}

	visible, err := s.providerVisible(r.Context(), user, id)
	if err != nil {
		writeErrResponse(rw, fmt.Errorf("error checking provider visibility: %w", err), http.StatusInternalServerError)
		return
	}

	if !visible {
		writeErrResponse(rw, errors.New("not authorized to access this provider"), http.StatusForbidden)
		return
	}

	metrics, err := s.Store.GetProviderDailyUsageMetrics(r.Context(), id, from, to)
	if err != nil {
		writeErrResponse(rw, fmt.Errorf("error getting provider daily usage: %w", err), http.StatusInternalServerError)
		return
	}

	writeResponse(rw, metrics, http.StatusOK)
}

// getProviderThroughputUsage godoc
// @Summary Get provider throughput usage
// @Description Get provider throughput aggregated into 30-minute or hourly buckets
// @Accept json
// @Produce json
// @Tags    providers
// @Param   id path string true "Provider ID"
// @Param   from query string false "Start date"
// @Param   to query string false "End date"
// @Param   aggregation_level query string false "Aggregation level" Enums(30min,hourly) default(30min)
// @Success 200 {array} types.AggregatedUsageMetric
// @Failure 400 {object} system.HTTPError
// @Failure 403 {object} system.HTTPError
// @Failure 500 {object} system.HTTPError
// @Router /api/v1/provider-endpoints/{id}/throughput-usage [get]
// @Security BearerAuth
func (s *HelixAPIServer) getProviderThroughputUsage(rw http.ResponseWriter, r *http.Request) {
	user := getRequestUser(r)
	id := getID(r)

	if !s.isProvidersManagementEnabled(r.Context()) && !user.Admin {
		writeErrResponse(rw, errors.New("providers management is not enabled"), http.StatusForbidden)
		return
	}

	from := time.Now().Add(-time.Hour * 24 * 7)
	to := time.Now()
	var err error

	if value := r.URL.Query().Get("from"); value != "" {
		from, err = time.Parse(time.RFC3339, value)
		if err != nil {
			writeErrResponse(rw, fmt.Errorf("failed to parse from date: %w", err), http.StatusBadRequest)
			return
		}
	}

	if value := r.URL.Query().Get("to"); value != "" {
		to, err = time.Parse(time.RFC3339, value)
		if err != nil {
			writeErrResponse(rw, fmt.Errorf("failed to parse to date: %w", err), http.StatusBadRequest)
			return
		}
	}

	aggregationLevel := store.AggregationLevel30Min
	switch value := r.URL.Query().Get("aggregation_level"); value {
	case "", string(store.AggregationLevel30Min):
	case string(store.AggregationLevelHourly):
		aggregationLevel = store.AggregationLevelHourly
	default:
		writeErrResponse(rw, fmt.Errorf("invalid aggregation level %q", value), http.StatusBadRequest)
		return
	}

	visible, err := s.providerVisible(r.Context(), user, id)
	if err != nil {
		writeErrResponse(rw, fmt.Errorf("error checking provider visibility: %w", err), http.StatusInternalServerError)
		return
	}
	if !visible {
		writeErrResponse(rw, errors.New("not authorized to access this provider"), http.StatusForbidden)
		return
	}

	metrics, err := s.Store.GetAggregatedUsageMetrics(r.Context(), &store.GetAggregatedUsageMetricsQuery{
		AggregationLevel: aggregationLevel,
		Provider:         id,
		From:             from,
		To:               to,
	})
	if err != nil {
		writeErrResponse(rw, fmt.Errorf("error getting provider throughput usage: %w", err), http.StatusInternalServerError)
		return
	}

	writeResponse(rw, metrics, http.StatusOK)
}

// getProviderUsersDailyUsage godoc
// @Summary Get provider daily usage per user
// @Description Get provider daily usage per user
// @Accept json
// @Produce json
// @Tags    providers
// @Param   id path string true "Provider ID"
// @Param   from query string false "Start date"
// @Param   to query string false "End date"
// @Success 200 {array} types.UsersAggregatedUsageMetric
// @Failure 400 {object} system.HTTPError
// @Failure 404 {object} system.HTTPError
// @Failure 500 {object} system.HTTPError
// @Router /api/v1/provider-endpoints/{id}/users-daily-usage [get]
// @Security BearerAuth
func (s *HelixAPIServer) getProviderUsersDailyUsage(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	user := getRequestUser(r)
	id := getID(r)

	// Check if providers management is enabled
	if !s.isProvidersManagementEnabled(ctx) && !user.Admin {
		writeErrResponse(rw, errors.New("providers management is not enabled"), http.StatusForbidden)
		return
	}

	from := time.Now().Add(-time.Hour * 24 * 7) // Last 7 days
	to := time.Now()

	var err error

	if r.URL.Query().Get("from") != "" {
		from, err = time.Parse(time.RFC3339, r.URL.Query().Get("from"))
		if err != nil {
			writeErrResponse(rw, fmt.Errorf("failed to parse from date: %w", err), http.StatusBadRequest)
			return
		}
	}

	if r.URL.Query().Get("to") != "" {
		to, err = time.Parse(time.RFC3339, r.URL.Query().Get("to"))
		if err != nil {
			writeErrResponse(rw, fmt.Errorf("failed to parse to date: %w", err), http.StatusBadRequest)
			return
		}
	}

	visible, err := s.providerVisible(r.Context(), user, id)
	if err != nil {
		writeErrResponse(rw, fmt.Errorf("error checking provider visibility: %w", err), http.StatusInternalServerError)
		return
	}

	if !visible {
		writeErrResponse(rw, errors.New("not authorized to access this provider"), http.StatusForbidden)
		return
	}

	metrics, err := s.Store.GetUsersAggregatedUsageMetrics(r.Context(), id, from, to)
	if err != nil {
		writeErrResponse(rw, fmt.Errorf("error getting provider daily usage: %w", err), http.StatusInternalServerError)
		return
	}

	writeResponse(rw, metrics, http.StatusOK)
}

func (s *HelixAPIServer) providerVisible(ctx context.Context, user *types.User, id string) (bool, error) {
	globalProviderEndpoints, err := s.providerManager.ListProviders(ctx, "")
	if err != nil {
		return false, fmt.Errorf("error listing providers: %w", err)
	}

	for _, provider := range globalProviderEndpoints {
		if string(provider) == id {
			return true, nil
		}
	}

	// Get provider
	providerEndpoint, err := s.Store.GetProviderEndpoint(ctx, &store.GetProviderEndpointsQuery{
		ID: id,
	})
	if err != nil {
		return false, fmt.Errorf("error getting provider endpoint: %w", err)
	}

	// If it's an org provider, authorize as org member to this org
	if providerEndpoint.OwnerType == types.OwnerTypeOrg {
		_, err := s.authorizeOrgMember(ctx, user, providerEndpoint.Owner)
		if err != nil {
			return false, fmt.Errorf("error authorizing org member: %w", err)
		}
		return true, nil
	}

	// Otherwise, check if it's the user's provider
	if providerEndpoint.Owner == user.ID {
		return true, nil
	}

	// Otherwise, it's not visible
	return false, nil
}

// StartModelCacheRefresh starts a background goroutine that periodically refreshes
// the model cache for all providers. This ensures that the cache is populated even
// for API-only clients that don't use the UI (which triggers cache population via
// the /api/v1/provider-endpoints?with_models=true endpoint).
//
// The refresh runs:
// 1. Immediately on startup
// 2. Then periodically based on ModelsCacheTTL (default 1 minute)
//
// This is important for handling:
//   - HuggingFace model IDs like "Qwen/Qwen3-Coder" that could be incorrectly parsed
//     as provider prefixes if the cache is empty
//   - Providers that were down at startup and later come back up
//   - New models added to providers
func (s *HelixAPIServer) StartModelCacheRefresh(ctx context.Context) {
	// Use ModelsCacheTTL as the refresh interval, with a minimum of 30 seconds
	refreshInterval := s.Cfg.WebServer.ModelsCacheTTL
	if refreshInterval < 30*time.Second {
		refreshInterval = 30 * time.Second
	}

	log.Info().
		Dur("refresh_interval", refreshInterval).
		Msg("starting background model cache refresh")

	// Run initial refresh immediately
	go func() {
		s.refreshAllProviderModels(ctx)

		// Then run periodically
		ticker := time.NewTicker(refreshInterval)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				log.Info().Msg("stopping background model cache refresh")
				return
			case <-ticker.C:
				s.refreshAllProviderModels(ctx)
			}
		}
	}()
}

// refreshAllProviderModels fetches and caches model lists from all accessible providers.
// This includes both global providers from env vars and database-stored providers.
// Errors are logged but don't stop the refresh process for other providers.
func (s *HelixAPIServer) refreshAllProviderModels(ctx context.Context) {
	startTime := time.Now()
	var successCount, errorCount int

	// First refresh global providers from env vars (these use "system" as owner)
	globalProviders, err := s.providerManager.ListProviders(ctx, "")
	if err != nil {
		log.Warn().Err(err).Msg("failed to list global providers for cache refresh")
	} else {
		for _, provider := range globalProviders {
			endpoint := &types.ProviderEndpoint{
				Name:  string(provider),
				Owner: string(types.OwnerTypeSystem),
			}

			// Skip helix provider - it uses the internal scheduler, not external models
			if provider == types.ProviderHelix {
				continue
			}

			_, err := s.getProviderModels(ctx, endpoint)
			if err != nil {
				log.Debug().
					Err(err).
					Str("provider", string(provider)).
					Msg("failed to refresh models for global provider (provider may be down)")
				errorCount++
			} else {
				successCount++
			}
		}
	}

	// Then refresh ALL database-stored providers (system, per-user, and
	// per-org). The previous filter (Owner=system + WithGlobal) skipped
	// org-scoped user providers entirely, so their model-list cache was only
	// populated when a UI call hit /api/v1/providers/.../models — meaning
	// /v1/chat/completions routing for those providers silently failed
	// (findProviderWithModel cache miss → default-provider fence) until
	// someone visited the dashboard. Use All=true so the cache is warm for
	// every configured provider regardless of scope.
	dbProviders, err := s.Store.ListProviderEndpoints(ctx, &store.ListProviderEndpointsQuery{
		All: true,
	})
	if err != nil {
		log.Warn().Err(err).Msg("failed to list database providers for cache refresh")
	} else {
		for _, provider := range dbProviders {
			_, err := s.getProviderModels(ctx, provider)
			if err != nil {
				log.Debug().
					Err(err).
					Str("provider", provider.Name).
					Str("owner", provider.Owner).
					Msg("failed to refresh models for database provider (provider may be down)")
				errorCount++
			} else {
				successCount++
			}
		}
	}

	log.Info().
		Int("success_count", successCount).
		Int("error_count", errorCount).
		Dur("duration", time.Since(startTime)).
		Msg("completed model cache refresh")
}
