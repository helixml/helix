package api_test

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"

	"github.com/helixml/helix/api/pkg/org/application/activations"
	"github.com/helixml/helix/api/pkg/org/application/configregistry"
	"github.com/helixml/helix/api/pkg/org/application/nodes"
	"github.com/helixml/helix/api/pkg/org/domain/orgchart"
	orgapi "github.com/helixml/helix/api/pkg/org/interfaces/server/api"
	"github.com/helixml/helix/api/pkg/types"
)

type deferredRuntime struct{}

func (deferredRuntime) State(context.Context, string, orgchart.NodeID) (orgapi.BotRuntimeInfo, error) {
	return orgapi.BotRuntimeInfo{}, errors.New("not provisioned")
}

type recordingDefaultApplier struct {
	applied  bool
	appID    string
	defaults types.AssistantConfig
}

func (a *recordingDefaultApplier) ApplyAgentDefaults(_ context.Context, appID string, defaults types.AssistantConfig) error {
	a.applied = true
	a.appID = appID
	a.defaults = defaults
	return nil
}

type orderedEnsurer struct {
	defaults *recordingDefaultApplier
	called   bool
}

func (e *orderedEnsurer) Ensure(context.Context, string, orgchart.NodeID) (string, string, string, error) {
	if !e.defaults.applied {
		return "", "", "", errors.New("provisioned before defaults were applied")
	}
	e.called = true
	return "prj-agent", e.defaults.appID, "", nil
}

func TestSettingDefaultAppliesDeferredAgentBeforeActivation(t *testing.T) {
	deps, st, reg := newDeps(t)
	reg.Register(configregistry.Spec{Key: configregistry.DefaultAgentConfigKey, Type: configregistry.TypeObject})
	ctx := context.Background()
	created, err := deps.Nodes.Create(ctx, "org-test", nodes.CreateParams{
		ID: "b-agent", Name: "Agent", Content: "instructions", AgentID: "app-agent",
	})
	if err != nil {
		t.Fatal(err)
	}
	applier := &recordingDefaultApplier{}
	ensurer := &orderedEnsurer{defaults: applier}
	deps.AgentDefaultApplier = applier
	deps.BotRuntime = deferredRuntime{}
	deps = wireActivate(deps, st, ensurer, &fakeDispatcher{})

	rec := do(t, orgapi.Handler(deps), http.MethodPut, "/settings/"+configregistry.DefaultAgentConfigKey, orgapi.SetSettingRequest{
		Value: `{"code_agent_runtime":"codex_cli","code_agent_credential_type":"subscription","model":"gpt-5.6","reasoning_effort":"high"}`,
	})
	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d; body=%s", rec.Code, rec.Body)
	}
	if !ensurer.called {
		t.Fatal("deferred Agent was not activated")
	}
	if applier.appID != created.AgentID ||
		applier.defaults.CodeAgentRuntime != types.CodeAgentRuntimeCodexCLI ||
		applier.defaults.Model != "gpt-5.6" {
		t.Fatalf("applied defaults = app %q config %+v", applier.appID, applier.defaults)
	}
}

func TestSettingDefaultRejectsInvalidRuntimeConfiguration(t *testing.T) {
	deps, _, reg := newDeps(t)
	reg.Register(configregistry.Spec{Key: configregistry.DefaultAgentConfigKey, Type: configregistry.TypeObject})
	deps.ValidateDefaultAgentConfig = func(_ context.Context, _ string, config types.AssistantConfig) error {
		if config.Model == "missing-model" {
			return errors.New("model is not available")
		}
		return nil
	}

	rec := do(t, orgapi.Handler(deps), http.MethodPut, "/settings/"+configregistry.DefaultAgentConfigKey, orgapi.SetSettingRequest{
		Value: `{"code_agent_runtime":"zed_agent","code_agent_credential_type":"api_key","provider":"helix","model":"missing-model"}`,
	})
	if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), "model is not available") {
		t.Fatalf("status = %d; body=%s", rec.Code, rec.Body.String())
	}
	if reg.IsConfigured(context.Background(), "org-test", configregistry.DefaultAgentConfigKey) {
		t.Fatal("invalid default runtime was persisted")
	}
}

var _ activations.ProjectEnsurer = (*orderedEnsurer)(nil)
