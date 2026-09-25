package api_test

import (
	"context"
	"fmt"
	"net/http"
	"slices"
	"testing"
	"time"

	"github.com/helixml/helix/api/pkg/org/application/instances"
	"github.com/helixml/helix/api/pkg/org/domain/orgchart"
	orgapi "github.com/helixml/helix/api/pkg/org/interfaces/server/api"
	"github.com/helixml/helix/api/pkg/types"
)

// fakeInstances records calls and serves canned sessions.
type fakeInstances struct {
	sessions  []*types.Session
	created   []instances.Params
	deleted   []string
	synced    []orgchart.NodeID
	createErr error
	deleteErr error
}

func (f *fakeInstances) List(context.Context, string, orgchart.NodeID) ([]*types.Session, error) {
	return f.sessions, nil
}

func (f *fakeInstances) Create(_ context.Context, _ string, botID orgchart.NodeID, params instances.Params) (*types.Session, error) {
	if f.createErr != nil {
		return nil, f.createErr
	}
	f.created = append(f.created, params)
	return &types.Session{
		ID:      "ses_new",
		Name:    params.Name,
		Created: time.Date(2026, 9, 24, 12, 0, 0, 0, time.UTC),
		Metadata: types.SessionMetadata{
			OrgWorkerID:    string(botID),
			SessionRole:    types.SessionRoleOrgBotInstance,
			SandboxRuntime: params.SandboxRuntime,
		},
	}, nil
}

func (f *fakeInstances) Delete(_ context.Context, _ string, _ orgchart.NodeID, sessionID string) error {
	if f.deleteErr != nil {
		return f.deleteErr
	}
	f.deleted = append(f.deleted, sessionID)
	return nil
}

func (f *fakeInstances) SyncProfile(_ context.Context, _ string, botID orgchart.NodeID) error {
	f.synced = append(f.synced, botID)
	return nil
}

func TestBotInstanceRoutes(t *testing.T) {
	deps, st, _ := newDeps(t)
	fake := &fakeInstances{sessions: []*types.Session{{
		ID:       "ses_one",
		Name:     "Broker · Sep 24 12:00",
		Owner:    "usr_owner",
		Metadata: types.SessionMetadata{OrgWorkerID: "b-broker", SandboxRuntime: types.SandboxRuntimeUbuntuDesktop, ExternalAgentStatus: "running"},
	}}}
	deps.BotInstances = fake
	h := orgapi.Handler(deps)
	seedBot(t, st, context.Background(), "b-broker", "# Broker")

	rec := do(t, h, "GET", "/bots/b-broker/instances", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("list: got %d; body=%s", rec.Code, rec.Body)
	}
	var list []orgapi.BotInstanceDTO
	decode(t, rec, &list)
	if len(list) != 1 || list[0].SessionID != "ses_one" || list[0].SandboxStatus != "running" || list[0].SandboxRuntime != types.SandboxRuntimeUbuntuDesktop {
		t.Fatalf("list = %+v", list)
	}

	rec = do(t, h, "POST", "/bots/b-broker/instances", orgapi.CreateBotInstanceRequest{
		Name: "Customer 42", SandboxRuntime: types.SandboxRuntimeHeadlessUbuntu, Message: "hello",
	})
	if rec.Code != http.StatusCreated {
		t.Fatalf("create: got %d; body=%s", rec.Code, rec.Body)
	}
	var created orgapi.BotInstanceDTO
	decode(t, rec, &created)
	if created.SessionID != "ses_new" || created.BotID != "b-broker" {
		t.Fatalf("created = %+v", created)
	}
	want := instances.Params{Name: "Customer 42", SandboxRuntime: types.SandboxRuntimeHeadlessUbuntu, Message: "hello"}
	if len(fake.created) != 1 || fake.created[0] != want {
		t.Fatalf("create params = %+v", fake.created)
	}

	rec = do(t, h, "DELETE", "/bots/b-broker/instances/ses_one", nil)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("delete: got %d; body=%s", rec.Code, rec.Body)
	}
	if !slices.Equal(fake.deleted, []string{"ses_one"}) {
		t.Fatalf("deleted = %v", fake.deleted)
	}

	if rec := do(t, h, "GET", "/bots/b-missing/instances", nil); rec.Code != http.StatusNotFound {
		t.Fatalf("unknown bot: got %d", rec.Code)
	}
}

func TestBotInstanceRoutes_ErrorMapping(t *testing.T) {
	deps, st, _ := newDeps(t)
	fake := &fakeInstances{
		createErr: fmt.Errorf("%w: bad runtime", instances.ErrInvalidRequest),
		deleteErr: fmt.Errorf("not yours: %w", instances.ErrForbidden),
	}
	deps.BotInstances = fake
	h := orgapi.Handler(deps)
	seedBot(t, st, context.Background(), "b-broker", "# Broker")

	if rec := do(t, h, "POST", "/bots/b-broker/instances", orgapi.CreateBotInstanceRequest{}); rec.Code != http.StatusBadRequest {
		t.Fatalf("invalid create: got %d", rec.Code)
	}
	if rec := do(t, h, "DELETE", "/bots/b-broker/instances/ses_x", nil); rec.Code != http.StatusForbidden {
		t.Fatalf("forbidden delete: got %d", rec.Code)
	}

	deps.BotInstances = nil
	if rec := do(t, orgapi.Handler(deps), "GET", "/bots/b-broker/instances", nil); rec.Code != http.StatusNotImplemented {
		t.Fatalf("unwired: got %d", rec.Code)
	}
}

func TestPatchBot_InstanceProfile(t *testing.T) {
	deps, st, _ := newDeps(t)
	fake := &fakeInstances{}
	deps.BotInstances = fake
	h := orgapi.Handler(deps)
	seedBot(t, st, context.Background(), "b-broker", "# Broker")

	// A bot that never set a profile reports the minimal default.
	rec := do(t, h, "GET", "/bots/b-broker", nil)
	var detail orgapi.BotDetailDTO
	decode(t, rec, &detail)
	if got := detail.Bot.InstanceProfile; !slices.Equal(got.MCPServers, []string{types.InstanceMCPServerBrowser}) || len(got.Tools) != 0 || got.HelixSkills {
		t.Fatalf("default profile = %+v", got)
	}

	profile := types.BotInstanceProfile{
		SandboxRuntime: types.SandboxRuntimeUbuntuDesktop,
		MCPServers:     []string{"chrome-devtools", "helix-session"},
		Tools:          []string{"chat", "read_events"},
	}
	rec = do(t, h, "PATCH", "/bots/b-broker", orgapi.UpdateBotRequest{InstanceProfile: &profile})
	if rec.Code != http.StatusOK {
		t.Fatalf("PATCH: got %d; body=%s", rec.Code, rec.Body)
	}
	var updated orgapi.BotDTO
	decode(t, rec, &updated)
	if updated.InstanceProfile.SandboxRuntime != types.SandboxRuntimeUbuntuDesktop ||
		!slices.Equal(updated.InstanceProfile.Tools, []string{"chat", "read_events"}) {
		t.Fatalf("updated profile = %+v", updated.InstanceProfile)
	}
	if !slices.Equal(fake.synced, []orgchart.NodeID{"b-broker"}) {
		t.Fatalf("existing instances were not synced: %v", fake.synced)
	}

	for name, bad := range map[string]types.BotInstanceProfile{
		"unknown tool":       {Tools: []string{"no_such_tool"}},
		"bad runtime":        {SandboxRuntime: "windows"},
		"org server by name": {MCPServers: []string{"helix"}},
	} {
		rec := do(t, h, "PATCH", "/bots/b-broker", orgapi.UpdateBotRequest{InstanceProfile: &bad})
		if rec.Code != http.StatusBadRequest {
			t.Errorf("%s: got %d, want 400; body=%s", name, rec.Code, rec.Body)
		}
	}
}

func TestPatchBot_InstanceOrgManagementToolsNeedOwner(t *testing.T) {
	deps, st, _ := newDeps(t)
	deps.BotInstances = &fakeInstances{}
	h := orgapi.Handler(deps)
	seedBot(t, st, context.Background(), "b-broker", "# Broker")

	profile := types.BotInstanceProfile{MCPServers: []string{}, Tools: []string{"create_bot"}}
	rec := doAsRole(t, h, "PATCH", "/bots/b-broker", orgapi.UpdateBotRequest{InstanceProfile: &profile}, types.OrganizationRoleMember, false)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("member granting create_bot to instances: got %d; body=%s", rec.Code, rec.Body)
	}
}
