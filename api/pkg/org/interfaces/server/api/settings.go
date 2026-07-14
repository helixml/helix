package api

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"github.com/helixml/helix/api/pkg/org/application/configregistry"
	"github.com/rs/zerolog/log"
)

// agentProvisioningKeys activate bots whose initial provisioning was deferred.
// worker.* remains supported for clients predating the atomic agent.default key.
var agentProvisioningKeys = map[string]bool{
	configregistry.DefaultAgentConfigKey: true,
	"worker.runtime":                     true,
	"worker.credentials":                 true,
	"worker.provider":                    true,
	"worker.model":                       true,
}

// ---- Settings -----------------------------------------------------------

// listSettings returns the registry's spec list + current redacted values.
//
// @Summary Helix-org: list settings
// @Tags HelixOrg
// @Produce json
// @Success 200 {object} api.SettingsResponse
// @Security ApiKeyAuth
// @Router /api/v1/orgs/{org}/settings [get]
func (a *apiHandler) listSettings(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	orgID, err := resolveOrgID(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	resp := SettingsResponse{
		PublicURL: a.deps.PublicURL,
		DBPath:    a.deps.DBPath,
	}
	if a.deps.Configs != nil {
		specs := a.deps.Configs.Specs()
		resp.Specs = make([]SettingsSpecDTO, 0, len(specs))
		for _, sp := range specs {
			resp.Specs = append(resp.Specs, settingsSpecDTO(ctx, orgID, a.deps.Configs, sp))
		}
	}
	writeJSON(w, http.StatusOK, resp)
}

// settingsSpecDTO resolves the current redacted value + the
// "configured" bool surfaced on each settings row. Lives outside the
// handler so a future "GET /settings/{key}" can reuse it.
func settingsSpecDTO(ctx context.Context, orgID string, reg *configregistry.Registry, sp configregistry.Spec) SettingsSpecDTO {
	row := SettingsSpecDTO{
		Key:         sp.Key,
		Type:        string(sp.Type),
		Required:    sp.Required,
		Description: sp.Description,
	}
	// "Configured" means an explicit configs row exists (not "has a
	// value via default").
	row.Configured = reg.IsConfigured(ctx, orgID, sp.Key)
	// GetRedacted falls back to the default when no row is set; an
	// error means "not configured and no default" — render empty.
	if v, err := reg.GetRedacted(ctx, orgID, sp.Key); err == nil {
		row.Value = v
	}
	return row
}

// setSetting writes a config row for the given key.
//
// @Summary Helix-org: set a setting
// @Tags HelixOrg
// @Accept json
// @Param key path string true "Setting key"
// @Param payload body api.SetSettingRequest true "Setting value (raw JSON per spec type)"
// @Success 204
// @Failure 400 {object} api.ErrorResponse
// @Security ApiKeyAuth
// @Router /api/v1/orgs/{org}/settings/{key} [put]
func (a *apiHandler) setSetting(w http.ResponseWriter, r *http.Request) {
	orgID, err := resolveOrgID(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	key := strings.TrimSpace(r.PathValue("key"))
	if key == "" {
		writeError(w, http.StatusBadRequest, errors.New("key is required"))
		return
	}
	var req SetSettingRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if err := a.deps.Configs.Set(r.Context(), orgID, key, req.Value); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	// A complete initial configuration activates bots whose provisioning was
	// deferred. Existing apps remain independently configurable.
	if agentProvisioningKeys[key] {
		a.activateDeferredBotsAfterRuntimeChange(r.Context(), orgID)
	}
	w.WriteHeader(http.StatusNoContent)
}

// activateDeferredBotsAfterRuntimeChange provisions bots that were created
// before the org's initial runtime configuration was complete:
//
//   - A Bot that was deferred at create (no project yet, because no runtime
//     was configured) is activated now — it provisions for the FIRST time
//     with the correct config, so its desktop never comes up on the gpt
//     default.
//   - A Bot that already has a project is left unchanged. Runtime/model edits
//     for an existing bot belong to its generated Helix app.
//
// Config is committed before this runs, so first provisioning reads it.
// Best-effort: per-bot failures are logged, not fatal — the settings write
// already succeeded.
func (a *apiHandler) activateDeferredBotsAfterRuntimeChange(ctx context.Context, orgID string) {
	if a.deps.Queries == nil {
		return
	}
	// The four worker.* keys are written as separate (often concurrent)
	// requests by the UI. Wait until the runtime config is COMPLETE before
	// provisioning anything, so a deferred bot is never brought up on a
	// half-written config and then re-applied — it provisions exactly once,
	// correct. Partial writes are no-ops; the write that completes the config
	// does the work.
	if !a.runtimeConfigComplete(ctx, orgID) {
		return
	}
	bs, err := a.deps.Queries.ListBots(ctx, orgID)
	if err != nil {
		log.Warn().Err(err).Str("org", orgID).Msg("activate deferred bots after runtime change: list bots failed")
		return
	}
	for _, b := range bs {
		// A human node is never provisioned/activated — skip it, or the
		// runtime-change sweep would spin up a desktop for every person.
		if b.IsHuman() {
			continue
		}
		provisioned := true
		if a.deps.BotRuntime != nil {
			info, err := a.deps.BotRuntime.State(ctx, orgID, b.ID)
			provisioned = err == nil && info.ProjectID != ""
		}
		if !provisioned {
			// Deferred bot: activate it now so it provisions with the
			// just-configured runtime (correct from its first boot).
			if a.deps.Activations == nil {
				continue
			}
			if _, err := a.deps.Activations.Activate(ctx, orgID, b.ID); err != nil {
				log.Warn().Err(err).Str("org", orgID).Str("bot", string(b.ID)).
					Msg("activate deferred bot after runtime change failed")
			}
			continue
		}
		// Existing apps are configured through the Helix app UI/API. The
		// org defaults are provisioning inputs, not reconciliation policy.
	}
}

// runtimeConfigComplete reports whether the org's default agent configuration has
// every field a Bot needs to provision. It mirrors resolveWorkerAgentConfig's
// coercion (server package): runtimes without subscription support are always
// api_key and need a provider + model; claude_code and codex_cli default to
// subscription unless credentials is explicitly api_key. Used to hold off
// provisioning until a half-written config is fully in place.
func (a *apiHandler) runtimeConfigComplete(ctx context.Context, orgID string) bool {
	if a.deps.Configs == nil {
		return false
	}
	cfg, err := a.deps.Configs.GetDefaultAgentConfig(ctx, orgID)
	if err != nil {
		return false
	}
	runtime := cfg.Runtime
	if runtime == "" {
		return false
	}
	credentials := cfg.Credentials
	if !runtimeSupportsSubscription(runtime) {
		credentials = "api_key"
	} else if credentials == "" {
		credentials = "subscription"
	}
	if credentials == "subscription" {
		return true
	}
	return cfg.Provider != "" && cfg.Model != ""
}

func runtimeSupportsSubscription(runtime string) bool {
	return runtime == "claude_code" || runtime == "codex_cli"
}

// deleteSetting removes the config row for the given key, falling back to defaults.
//
// @Summary Helix-org: delete a setting
// @Tags HelixOrg
// @Param key path string true "Setting key"
// @Success 204
// @Failure 400 {object} api.ErrorResponse
// @Security ApiKeyAuth
// @Router /api/v1/orgs/{org}/settings/{key} [delete]
func (a *apiHandler) deleteSetting(w http.ResponseWriter, r *http.Request) {
	orgID, err := resolveOrgID(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	key := strings.TrimSpace(r.PathValue("key"))
	if key == "" {
		writeError(w, http.StatusBadRequest, errors.New("key is required"))
		return
	}
	if err := a.deps.Configs.Delete(r.Context(), orgID, key); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
