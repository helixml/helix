package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"slices"
	"strings"
	"time"

	"github.com/helixml/helix/api/pkg/org/application/configregistry"
	"github.com/helixml/helix/api/pkg/org/domain/orgchart"
	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/system"
	"github.com/helixml/helix/api/pkg/types"
)

// adminReplaceModel godoc
// @Summary Replace a model everywhere it is selected
// @Description When a provider stops serving a model, every org default
// @Description runtime, agent, org bot, project, spec task and session that
// @Description still selects it fails on its next turn. This moves all of them
// @Description to another model on the same provider. The replacement must be
// @Description served by the provider now. Use dry_run to list them first.
// @Tags    admin
// @Param   request body types.ReplaceModelRequest true "Provider and models"
// @Success 200 {object} types.ReplaceModelResponse
// @Router /api/v1/admin/models/replace [post]
// @Security BearerAuth
func (s *HelixAPIServer) adminReplaceModel(_ http.ResponseWriter, r *http.Request) (*types.ReplaceModelResponse, *system.HTTPError) {
	var req types.ReplaceModelRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		return nil, system.NewHTTPError400(fmt.Sprintf("invalid request body: %s", err))
	}
	req.ProviderRef = strings.TrimSpace(req.ProviderRef)
	req.FromModel = strings.TrimSpace(req.FromModel)
	req.ToModel = strings.TrimSpace(req.ToModel)
	if req.ProviderRef == "" || req.FromModel == "" || req.ToModel == "" {
		return nil, system.NewHTTPError400("provider_ref, from_model and to_model are required")
	}
	if req.FromModel == req.ToModel {
		return nil, system.NewHTTPError400("from_model and to_model are the same")
	}
	endpoint, err := s.Store.GetProviderEndpoint(r.Context(), &store.GetProviderEndpointsQuery{ID: req.ProviderRef})
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return nil, system.NewHTTPError404(fmt.Sprintf("provider %q not found", req.ProviderRef))
		}
		return nil, system.NewHTTPError500(err.Error())
	}
	models, err := s.getProviderModels(r.Context(), endpoint)
	if err != nil {
		return nil, system.NewHTTPError422(fmt.Sprintf("list models of provider %q: %s", endpoint.Name, err))
	}
	if models.Degraded != nil {
		return nil, system.NewHTTPError422(fmt.Sprintf("list models of provider %q: %s", endpoint.Name, models.Degraded))
	}
	if !slices.ContainsFunc(models.Models, func(m types.OpenAIModel) bool { return m.Enabled && m.ID == req.ToModel }) {
		return nil, system.NewHTTPError400(fmt.Sprintf("provider %q does not serve %q", endpoint.Name, req.ToModel))
	}

	refs, err := s.replaceModel(r.Context(), modelReplacement{
		endpoint: endpoint, from: req.FromModel, to: req.ToModel, dryRun: req.DryRun,
	})
	if err != nil {
		return nil, system.NewHTTPError500(err.Error())
	}
	return &types.ReplaceModelResponse{DryRun: req.DryRun, References: refs}, nil
}

type modelReplacement struct {
	endpoint *types.ProviderEndpoint
	from, to string
	dryRun   bool
}

// provider reports whether a stored provider value names the endpoint: by
// ID, or by name for selections saved before providers had IDs.
func (m modelReplacement) provider(ref string) bool {
	if ref == m.endpoint.ID {
		return true
	}
	return ref != "" && !strings.HasPrefix(ref, "pe_") &&
		types.CanonicalProviderName(ref) == types.CanonicalProviderName(m.endpoint.Name)
}

// modelIDs are the forms a selection of the old model is saved in.
func (m modelReplacement) modelIDs() []string {
	return []string{m.from, m.endpoint.ID + "/" + m.from, m.endpoint.Name + "/" + m.from}
}

// swap replaces model when provider names the endpoint and model is the
// old model in any saved form.
func (m modelReplacement) swap(provider string, model *string) bool {
	if !m.provider(provider) || !slices.Contains(m.modelIDs(), *model) {
		return false
	}
	*model = m.to
	return true
}

// swapExecution and swapOverrides replace the pointer with a swapped copy,
// so a dry run never writes through a pointer shared with the store.
func (m modelReplacement) swapExecution(c **types.CodeAgentExecutionConfig) bool {
	if *c == nil {
		return false
	}
	swapped := **c
	if !m.swap(swapped.ProviderRef, &swapped.Model) {
		return false
	}
	*c = &swapped
	return true
}

func (m modelReplacement) swapOverrides(o **types.CodeAgentOverrides) bool {
	if *o == nil {
		return false
	}
	swapped := **o
	if !m.swap(swapped.ProviderRef, &swapped.Model) {
		return false
	}
	*o = &swapped
	return true
}

// swapAssistant covers every provider/model pair of an assistant. A pair
// with no provider of its own uses the assistant's main provider.
func (m modelReplacement) swapAssistant(a *types.AssistantConfig) []string {
	pairs := []struct {
		field    string
		provider string
		model    *string
	}{
		{"model", a.Provider, &a.Model},
		{"generation_model", a.GenerationModelProvider, &a.GenerationModel},
		{"reasoning_model", a.ReasoningModelProvider, &a.ReasoningModel},
		{"small_generation_model", a.SmallGenerationModelProvider, &a.SmallGenerationModel},
		{"small_reasoning_model", a.SmallReasoningModelProvider, &a.SmallReasoningModel},
	}
	var fields []string
	for _, p := range pairs {
		provider := p.provider
		if provider == "" {
			provider = a.Provider
		}
		if m.swap(provider, p.model) {
			fields = append(fields, p.field)
		}
	}
	return fields
}

// replaceModel finds, and unless dryRun rewrites, every saved selection of
// the old model. Agents go first: saving an org agent also re-syncs its
// linked bot and project, through the same path as an edit in the UI.
func (s *HelixAPIServer) replaceModel(ctx context.Context, m modelReplacement) ([]types.ModelReference, error) {
	refs := []types.ModelReference{}
	add := func(kind, id, org string, fields ...string) {
		refs = append(refs, types.ModelReference{Kind: kind, ID: id, OrganizationID: org, Fields: fields})
	}

	orgs, err := s.Store.ListOrganizations(ctx, &store.ListOrganizationsQuery{})
	if err != nil {
		return nil, fmt.Errorf("list organizations: %w", err)
	}
	if s.helixOrg != nil && s.helixOrg.configs != nil {
		for _, org := range orgs {
			fields, err := s.replaceOrgDefaultModel(ctx, s.helixOrg.configs, org.ID, m)
			if err != nil {
				return refs, fmt.Errorf("org %s default runtime: %w", org.ID, err)
			}
			if len(fields) > 0 {
				add("org_default", configregistry.DefaultAgentConfigKey, org.ID, fields...)
			}
		}
	}

	apps, err := s.Store.ListApps(ctx, &store.ListAppsQuery{})
	if err != nil {
		return refs, fmt.Errorf("list agents: %w", err)
	}
	for _, app := range apps {
		updated, err := copyViaJSON(app)
		if err != nil {
			return refs, fmt.Errorf("copy agent %s: %w", app.ID, err)
		}
		var fields []string
		for i := range updated.Config.Helix.Assistants {
			for _, f := range m.swapAssistant(&updated.Config.Helix.Assistants[i]) {
				fields = append(fields, fmt.Sprintf("assistants[%d].%s", i, f))
			}
		}
		if len(fields) == 0 {
			continue
		}
		add("agent", app.ID, app.OrganizationID, fields...)
		if m.dryRun {
			continue
		}
		if _, err := s.Store.UpdateApp(ctx, updated); err != nil {
			return refs, fmt.Errorf("update agent %s: %w", app.ID, err)
		}
		if err := s.syncOrgAgentProjectCodeAgentConfig(ctx, updated); err != nil {
			return refs, fmt.Errorf("sync agent %s to its bot and project: %w", app.ID, err)
		}
	}

	if s.helixOrg != nil && s.helixOrg.store != nil && s.helixOrg.store.Nodes != nil {
		nodes := s.helixOrg.store.Nodes
		ids, err := nodes.ListAllIDs(ctx)
		if err != nil {
			return refs, fmt.Errorf("list org bots: %w", err)
		}
		for _, id := range ids {
			node, err := nodes.Get(ctx, id.OrgID, orgchart.NodeID(id.ID))
			if err != nil {
				return refs, fmt.Errorf("get org bot %s/%s: %w", id.OrgID, id.ID, err)
			}
			config := node.CodeAgentConfig
			if !m.swapExecution(&config) {
				continue
			}
			add("org_bot", id.ID, id.OrgID, "code_agent_config")
			if m.dryRun {
				continue
			}
			if err := nodes.UpdateCodeAgentConfig(ctx, id.OrgID, node.ID, config, time.Now().UTC()); err != nil {
				return refs, fmt.Errorf("update org bot %s/%s: %w", id.OrgID, id.ID, err)
			}
		}
	}

	projects, err := s.Store.ListProjects(ctx, &store.ListProjectsQuery{AllScopes: true})
	if err != nil {
		return refs, fmt.Errorf("list projects: %w", err)
	}
	for _, project := range projects {
		var fields []string
		if m.swapExecution(&project.CodeAgentConfig) {
			fields = append(fields, "code_agent_config")
		}
		if m.swapExecution(&project.PlanningCodeAgentConfig) {
			fields = append(fields, "planning_code_agent_config")
		}
		if len(fields) == 0 {
			continue
		}
		add("project", project.ID, project.OrganizationID, fields...)
		if m.dryRun {
			continue
		}
		if err := s.Store.UpdateProject(ctx, project); err != nil {
			return refs, fmt.Errorf("update project %s: %w", project.ID, err)
		}
	}

	tasks, err := s.Store.ListSpecTasks(ctx, &types.SpecTaskFilters{IncludeArchived: true})
	if err != nil {
		return refs, fmt.Errorf("list spec tasks: %w", err)
	}
	for _, task := range tasks {
		var fields []string
		if m.swapExecution(&task.CodeAgentConfig) {
			fields = append(fields, "code_agent_config")
		}
		if m.swapExecution(&task.PlanningCodeAgentConfig) {
			fields = append(fields, "planning_code_agent_config")
		}
		if m.swapOverrides(&task.CodeAgentOverrides) {
			fields = append(fields, "code_agent_overrides")
		}
		if len(fields) == 0 {
			continue
		}
		add("spec_task", task.ID, task.OrganizationID, fields...)
		if m.dryRun {
			continue
		}
		if err := s.Store.UpdateSpecTask(ctx, task); err != nil {
			return refs, fmt.Errorf("update spec task %s: %w", task.ID, err)
		}
	}

	sessions, _, err := s.Store.ListSessions(ctx, store.ListSessionsQuery{
		AnyOwner: true, AnyOrganization: true, CodeAgentModels: m.modelIDs(),
	})
	if err != nil {
		return refs, fmt.Errorf("list sessions: %w", err)
	}
	for _, session := range sessions {
		var fields []string
		if m.swapExecution(&session.Metadata.CodeAgentConfig) {
			fields = append(fields, "code_agent_config")
		}
		if m.swapOverrides(&session.Metadata.CodeAgentOverrides) {
			fields = append(fields, "code_agent_overrides")
		}
		if len(fields) == 0 {
			continue
		}
		add("session", session.ID, session.OrganizationID, fields...)
		if m.dryRun {
			continue
		}
		if err := s.Store.UpdateSessionMetadata(ctx, session.ID, session.Metadata); err != nil {
			return refs, fmt.Errorf("update session %s: %w", session.ID, err)
		}
	}
	return refs, nil
}

// replaceOrgDefaultModel rewrites the org's default runtime, keeping every
// other key of the stored object as it is. Orgs still on the legacy
// worker.provider/worker.model keys get worker.model rewritten.
func (s *HelixAPIServer) replaceOrgDefaultModel(ctx context.Context, configs *configregistry.Registry, orgID string, m modelReplacement) ([]string, error) {
	if configs.IsConfigured(ctx, orgID, configregistry.DefaultAgentConfigKey) {
		raw, err := configs.GetRaw(ctx, orgID, configregistry.DefaultAgentConfigKey)
		if err != nil {
			return nil, err
		}
		var assistant types.AssistantConfig
		if err := json.Unmarshal([]byte(raw), &assistant); err != nil {
			return nil, fmt.Errorf("decode: %w", err)
		}
		fields := m.swapAssistant(&assistant)
		if len(fields) == 0 || m.dryRun {
			return fields, nil
		}
		var stored map[string]any
		if err := json.Unmarshal([]byte(raw), &stored); err != nil {
			return nil, fmt.Errorf("decode: %w", err)
		}
		for _, f := range fields {
			stored[f] = m.to
		}
		patched, err := json.Marshal(stored)
		if err != nil {
			return nil, err
		}
		return fields, configs.Set(ctx, orgID, configregistry.DefaultAgentConfigKey, string(patched))
	}
	if !configs.IsConfigured(ctx, orgID, "worker.model") {
		return nil, nil
	}
	provider, err := configs.GetString(ctx, orgID, "worker.provider")
	if err != nil {
		return nil, err
	}
	model, err := configs.GetString(ctx, orgID, "worker.model")
	if err != nil {
		return nil, err
	}
	if !m.swap(provider, &model) {
		return nil, nil
	}
	if m.dryRun {
		return []string{"worker.model"}, nil
	}
	value, err := json.Marshal(model)
	if err != nil {
		return nil, err
	}
	return []string{"worker.model"}, configs.Set(ctx, orgID, "worker.model", string(value))
}

func copyViaJSON[T any](v *T) (*T, error) {
	raw, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}
	var out T
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, err
	}
	return &out, nil
}
