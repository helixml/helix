package server

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/helixml/helix/api/pkg/model"
	"github.com/helixml/helix/api/pkg/openai"
	"github.com/helixml/helix/api/pkg/openai/manager"
	"github.com/helixml/helix/api/pkg/org/application/configregistry"
	"github.com/helixml/helix/api/pkg/org/domain/orgchart"
	helixorgstore "github.com/helixml/helix/api/pkg/org/domain/store"
	orgmemory "github.com/helixml/helix/api/pkg/org/infrastructure/persistence/memory"
	"github.com/helixml/helix/api/pkg/server/helixorg"
	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/store/memorystore"
	"github.com/helixml/helix/api/pkg/system"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

// replaceModelStore adds the two cross-tenant reads the in-memory store lacks.
type replaceModelStore struct {
	*memorystore.MemoryStore
	orgs      []*types.Organization
	endpoints []*types.ProviderEndpoint
}

func (s *replaceModelStore) ListOrganizations(context.Context, *store.ListOrganizationsQuery) ([]*types.Organization, error) {
	return s.orgs, nil
}

func (s *replaceModelStore) GetProviderEndpoint(_ context.Context, q *store.GetProviderEndpointsQuery) (*types.ProviderEndpoint, error) {
	for _, e := range s.endpoints {
		if e.ID == q.ID {
			return e, nil
		}
	}
	return nil, store.ErrNotFound
}

type replaceModelFixture struct {
	srv      *HelixAPIServer
	mem      *memorystore.MemoryStore
	orgStore *helixorgstore.Store
	configs  *configregistry.Registry
}

const (
	retired     = "qwen3.8-flash-next"
	replacement = "glm-5.3-flash"
)

func bunkerConfig(provider, m string) *types.CodeAgentExecutionConfig {
	return &types.CodeAgentExecutionConfig{
		Runtime: types.CodeAgentRuntimeOpenCode, CredentialType: types.CodeAgentCredentialTypeAPIKey,
		ProviderRef: provider, Model: m, ReasoningEffort: "none",
	}
}

func newReplaceModelFixture(t *testing.T, served []types.OpenAIModel) *replaceModelFixture {
	t.Helper()
	ctx := context.Background()
	srv, mem := newForkTestServer(t)
	bunker := &types.ProviderEndpoint{ID: "pe_bunker", Name: "HelixML Bunker", Owner: "admin", BaseURL: "https://bunker.local"}
	srv.Store = &replaceModelStore{
		MemoryStore: mem,
		orgs:        []*types.Organization{{ID: "org_a"}, {ID: "org_b"}, {ID: "org_legacy"}},
		endpoints:   []*types.ProviderEndpoint{bunker},
	}

	ctrl := gomock.NewController(t)
	providerManager := manager.NewMockProviderManager(ctrl)
	providerClient := openai.NewMockClient(ctrl)
	modelInfo := model.NewMockModelInfoProvider(ctrl)
	providerManager.EXPECT().GetClient(gomock.Any(), gomock.Any()).Return(providerClient, nil).AnyTimes()
	providerClient.EXPECT().ListModels(gomock.Any()).Return(served, nil).AnyTimes()
	modelInfo.EXPECT().GetModelInfo(gomock.Any(), gomock.Any()).Return(nil, errors.New("not found")).AnyTimes()
	srv.providerManager = providerManager
	srv.modelInfoProvider = modelInfo

	orgStore := orgmemory.New()
	configs := configregistry.New(orgStore.Configs)
	helixorg.RegisterConfigSpecs(configs)
	srv.helixOrg = &helixOrgHandlers{store: orgStore, configs: configs}

	// org_a: the retired model everywhere, plus an unrelated key that must survive.
	require.NoError(t, configs.Set(ctx, "org_a", configregistry.DefaultAgentConfigKey,
		`{"code_agent_runtime":"opencode","code_agent_credential_type":"api_key","provider":"pe_bunker","model":"qwen3.8-flash-next","reasoning_effort":"none"}`))
	// org_b: same model name on another provider is not this model.
	require.NoError(t, configs.Set(ctx, "org_b", configregistry.DefaultAgentConfigKey,
		`{"code_agent_runtime":"opencode","code_agent_credential_type":"api_key","provider":"pe_other","model":"qwen3.8-flash-next"}`))
	// org_legacy: pre-agent.default keys.
	require.NoError(t, configs.Set(ctx, "org_legacy", "worker.runtime", `"opencode"`))
	require.NoError(t, configs.Set(ctx, "org_legacy", "worker.provider", `"pe_bunker"`))
	require.NoError(t, configs.Set(ctx, "org_legacy", "worker.model", `"qwen3.8-flash-next"`))

	// An org agent linked to a bot: the bot's config follows the agent.
	mem.SeedApp(&types.App{
		ID: "app_cos", OrganizationID: "org_a", Owner: "u1", AgentKind: types.AgentKindOrg,
		Config: types.AppConfig{Helix: types.AppHelixConfig{Assistants: []types.AssistantConfig{{
			Name: "Chief of Staff", AgentType: types.AgentTypeZedExternal,
			CodeAgentRuntime: types.CodeAgentRuntimeOpenCode, CodeAgentCredentialType: types.CodeAgentCredentialTypeAPIKey,
			Provider: "pe_bunker", Model: retired, GenerationModelProvider: "pe_bunker", GenerationModel: retired,
		}}}},
	})
	linked, err := orgchart.NewNode("chief-of-staff", "# CoS", nil, time.Now().UTC(), "org_a")
	require.NoError(t, err)
	linked.AgentID = "app_cos"
	linked.CodeAgentConfig = bunkerConfig("pe_bunker", retired)
	require.NoError(t, orgStore.Nodes.Create(ctx, linked))
	// A bot with only its own config, saved with a provider-prefixed model.
	own, err := orgchart.NewNode("b-slack", "# Slack", nil, time.Now().UTC(), "org_a")
	require.NoError(t, err)
	own.CodeAgentConfig = bunkerConfig("pe_bunker", "pe_bunker/"+retired)
	require.NoError(t, orgStore.Nodes.Create(ctx, own))
	// A bot on another model of the same provider is left alone.
	other, err := orgchart.NewNode("b-glm", "# GLM", nil, time.Now().UTC(), "org_a")
	require.NoError(t, err)
	other.CodeAgentConfig = bunkerConfig("pe_bunker", "glm-5.3-flash-uncensored")
	require.NoError(t, orgStore.Nodes.Create(ctx, other))

	mem.SeedProject(&types.Project{ID: "prj_a", OrganizationID: "org_a",
		CodeAgentConfig: bunkerConfig("pe_bunker", retired), PlanningCodeAgentConfig: bunkerConfig("pe_bunker", retired)})
	mem.SeedProject(&types.Project{ID: "prj_personal", UserID: "u1", CodeAgentConfig: bunkerConfig("pe_bunker", retired)})
	mem.SeedSpecTask(&types.SpecTask{ID: "spt_a", ProjectID: "prj_a", OrganizationID: "org_a",
		CodeAgentConfig: bunkerConfig("pe_bunker", retired), PlanningCodeAgentConfig: bunkerConfig("pe_other", retired)})
	_, err = mem.CreateSession(ctx, types.Session{ID: "ses_bot", Owner: "u1", OrganizationID: "org_a",
		Metadata: types.SessionMetadata{CodeAgentConfig: bunkerConfig("pe_bunker", retired)}})
	require.NoError(t, err)

	return &replaceModelFixture{srv: srv, mem: mem, orgStore: orgStore, configs: configs}
}

func callReplaceModel(t *testing.T, srv *HelixAPIServer, req types.ReplaceModelRequest) *httptest.ResponseRecorder {
	t.Helper()
	body, err := json.Marshal(req)
	require.NoError(t, err)
	r := httptest.NewRequest(http.MethodPost, "/api/v1/admin/models/replace", bytes.NewReader(body))
	r = r.WithContext(setRequestUser(r.Context(), types.User{ID: "admin", Admin: true}))
	rr := httptest.NewRecorder()
	http.HandlerFunc(system.Wrapper(srv.adminReplaceModel)).ServeHTTP(rr, r)
	return rr
}

func (f *replaceModelFixture) models(t *testing.T) map[string]string {
	t.Helper()
	ctx := context.Background()
	got := map[string]string{}
	var def types.AssistantConfig
	require.NoError(t, f.configs.GetObject(ctx, "org_a", configregistry.DefaultAgentConfigKey, &def))
	got["org_a default"] = def.Model
	raw, err := f.configs.GetRaw(ctx, "org_a", configregistry.DefaultAgentConfigKey)
	require.NoError(t, err)
	var stored map[string]any
	require.NoError(t, json.Unmarshal([]byte(raw), &stored))
	got["org_a default effort"] = stored["reasoning_effort"].(string)
	require.NoError(t, f.configs.GetObject(ctx, "org_b", configregistry.DefaultAgentConfigKey, &def))
	got["org_b default"] = def.Model
	legacy, err := f.configs.GetString(ctx, "org_legacy", "worker.model")
	require.NoError(t, err)
	got["org_legacy worker.model"] = legacy
	app, err := f.mem.GetApp(ctx, "app_cos")
	require.NoError(t, err)
	got["agent model"] = app.Config.Helix.Assistants[0].Model
	got["agent generation_model"] = app.Config.Helix.Assistants[0].GenerationModel
	for _, id := range []orgchart.NodeID{"chief-of-staff", "b-slack", "b-glm"} {
		n, err := f.orgStore.Nodes.Get(ctx, "org_a", id)
		require.NoError(t, err)
		got["bot "+string(id)] = n.CodeAgentConfig.Model
	}
	for _, id := range []string{"prj_a", "prj_personal"} {
		p, err := f.mem.GetProject(ctx, id)
		require.NoError(t, err)
		got["project "+id] = p.CodeAgentConfig.Model
	}
	p, err := f.mem.GetProject(ctx, "prj_a")
	require.NoError(t, err)
	got["project prj_a planning"] = p.PlanningCodeAgentConfig.Model
	task, err := f.mem.GetSpecTask(ctx, "spt_a")
	require.NoError(t, err)
	got["task"] = task.CodeAgentConfig.Model
	got["task planning (other provider)"] = task.PlanningCodeAgentConfig.Model
	session, err := f.mem.GetSession(ctx, "ses_bot")
	require.NoError(t, err)
	got["session"] = session.Metadata.CodeAgentConfig.Model
	return got
}

func TestAdminReplaceModel(t *testing.T) {
	f := newReplaceModelFixture(t, []types.OpenAIModel{{ID: replacement, Enabled: true}})
	before := f.models(t)
	req := types.ReplaceModelRequest{ProviderRef: "pe_bunker", FromModel: retired, ToModel: replacement, DryRun: true}

	rr := callReplaceModel(t, f.srv, req)
	require.Equal(t, http.StatusOK, rr.Code, rr.Body.String())
	var dry types.ReplaceModelResponse
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &dry))
	kinds := map[string]int{}
	for _, ref := range dry.References {
		kinds[ref.Kind]++
	}
	require.Equal(t, map[string]int{"org_default": 2, "agent": 1, "org_bot": 2, "project": 2, "spec_task": 1, "session": 1}, kinds)
	require.Equal(t, before, f.models(t), "a dry run must not write")

	req.DryRun = false
	rr = callReplaceModel(t, f.srv, req)
	require.Equal(t, http.StatusOK, rr.Code, rr.Body.String())
	require.Equal(t, map[string]string{
		"org_a default":                  replacement,
		"org_a default effort":           "none",
		"org_b default":                  retired,
		"org_legacy worker.model":        replacement,
		"agent model":                    replacement,
		"agent generation_model":         replacement,
		"bot chief-of-staff":             replacement,
		"bot b-slack":                    replacement,
		"bot b-glm":                      "glm-5.3-flash-uncensored",
		"project prj_a":                  replacement,
		"project prj_personal":           replacement,
		"project prj_a planning":         replacement,
		"task":                           replacement,
		"task planning (other provider)": retired,
		"session":                        replacement,
	}, f.models(t))

	rr = callReplaceModel(t, f.srv, req)
	require.Equal(t, http.StatusOK, rr.Code, rr.Body.String())
	var again types.ReplaceModelResponse
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &again))
	require.Empty(t, again.References, "a second run finds nothing left")
}

func TestAdminReplaceModelRefusesAModelTheProviderDoesNotServe(t *testing.T) {
	f := newReplaceModelFixture(t, []types.OpenAIModel{{ID: "glm-5.3-flash-uncensored", Enabled: true}})
	before := f.models(t)

	rr := callReplaceModel(t, f.srv, types.ReplaceModelRequest{ProviderRef: "pe_bunker", FromModel: retired, ToModel: replacement})

	require.Equal(t, http.StatusBadRequest, rr.Code, rr.Body.String())
	require.Contains(t, rr.Body.String(), `does not serve "glm-5.3-flash"`)
	require.Equal(t, before, f.models(t))
}

// The new-project dialog sends the org's Default Runtime without showing its
// model, so a failure must say where that default is changed.
func TestExplainOrgDefaultRuntimeError(t *testing.T) {
	f := newReplaceModelFixture(t, nil)
	ctx := context.Background()
	cause := errors.New(`the coding-agent config references model "qwen3.8-flash-next" which is not currently available from provider "HelixML Bunker" - pick an available model`)

	fromDefault := bunkerConfig("pe_bunker", retired)
	fromDefault.ReasoningEffort = "none"
	got := f.srv.explainOrgDefaultRuntimeError(ctx, "org_a", fromDefault, cause)
	require.Contains(t, got, "The organization's Default Runtime can't be used")
	require.Contains(t, got, "Settings > Default Runtime")

	chosen := bunkerConfig("pe_bunker", "glm-5.3-flash-uncensored")
	require.Equal(t, cause.Error(), f.srv.explainOrgDefaultRuntimeError(ctx, "org_a", chosen, cause))
}
