package api

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"github.com/helixml/helix/api/pkg/org/application/configregistry"
	"github.com/rs/zerolog/log"
)

// workerRuntimeKeys are the org-default "Default Bot Runtime" settings.
// Changing any of them must re-apply every already-provisioned bot's agent
// app, otherwise a bot seeded/provisioned before the operator picked a
// runtime stays frozen on the seed-time default (claude_code/subscription
// with no model, which Zed renders as its built-in gpt) and the operator's
// later change never reaches the bot.
var workerRuntimeKeys = map[string]bool{
	"worker.runtime":     true,
	"worker.credentials": true,
	"worker.provider":    true,
	"worker.model":       true,
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
	// Propagate a Default Bot Runtime change to already-provisioned bots so
	// they pick up the new runtime/model instead of staying frozen at the
	// config that existed when they were first seeded. Config is committed
	// above, so Ensure below reads the new value.
	if workerRuntimeKeys[key] {
		a.reapplyBotsAfterRuntimeChange(r.Context(), orgID)
	}
	w.WriteHeader(http.StatusNoContent)
}

// reapplyBotsAfterRuntimeChange propagates a Default Bot Runtime change to
// every Bot in the org:
//
//   - A Bot that was deferred at create (no project yet, because no runtime
//     was configured) is activated now — it provisions for the FIRST time
//     with the correct config, so its desktop never comes up on the gpt
//     default.
//   - A Bot that already has a project is re-applied in place (idempotent
//     upsert-by-name that re-reads worker.*), rewriting its agent app's
//     Runtime/Credentials/Provider/Model; a running desktop picks up the new
//     model on its next settings-sync poll.
//
// Config is committed before this runs, so both paths read the new value.
// Best-effort: per-bot failures are logged, not fatal — the settings write
// already succeeded. A live runtime *switch* (e.g. claude_code -> goose_code)
// on an already-running desktop still needs a session restart to swap the
// in-sandbox agent binary; this only re-applies the stored config.
func (a *apiHandler) reapplyBotsAfterRuntimeChange(ctx context.Context, orgID string) {
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
		log.Warn().Err(err).Str("org", orgID).Msg("reapply bots after runtime change: list bots failed")
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
		if a.deps.ProjectEnsurer == nil {
			continue
		}
		if _, _, _, err := a.deps.ProjectEnsurer.Ensure(ctx, orgID, b.ID); err != nil {
			log.Warn().Err(err).Str("org", orgID).Str("bot", string(b.ID)).
				Msg("reapply bot project after runtime change failed")
		}
	}
}

// runtimeConfigComplete reports whether the org's Default Bot Runtime has
// every field a Bot needs to provision. It mirrors resolveWorkerAgentConfig's
// coercion (server package): runtimes without subscription support are always
// api_key and need a provider + model; claude_code and codex_cli default to
// subscription unless credentials is explicitly api_key. Used to hold off
// provisioning until a half-written config is fully in place.
func (a *apiHandler) runtimeConfigComplete(ctx context.Context, orgID string) bool {
	if a.deps.Configs == nil {
		return false
	}
	runtime, _ := a.deps.Configs.GetString(ctx, orgID, "worker.runtime")
	if runtime == "" {
		return false
	}
	credentials, _ := a.deps.Configs.GetString(ctx, orgID, "worker.credentials")
	if !runtimeSupportsSubscription(runtime) {
		credentials = "api_key"
	} else if credentials == "" {
		credentials = "subscription"
	}
	if credentials == "subscription" {
		return true
	}
	provider, _ := a.deps.Configs.GetString(ctx, orgID, "worker.provider")
	model, _ := a.deps.Configs.GetString(ctx, orgID, "worker.model")
	return provider != "" && model != ""
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
