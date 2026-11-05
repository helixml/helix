package server

import (
	"context"
	"net/http"
	"net/http/httptest"
	"slices"
	"testing"
	"time"

	"github.com/gorilla/mux"
	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/helixml/helix/api/pkg/org/application/lifecycle"
	"github.com/helixml/helix/api/pkg/org/application/publishing"
	"github.com/helixml/helix/api/pkg/org/domain/orgchart"
	orgstore "github.com/helixml/helix/api/pkg/org/domain/store"
	"github.com/helixml/helix/api/pkg/org/domain/tool"
	orgmemory "github.com/helixml/helix/api/pkg/org/infrastructure/persistence/memory"
	"github.com/helixml/helix/api/pkg/org/interfaces/mcptools"
	helixorgserver "github.com/helixml/helix/api/pkg/org/interfaces/server"
	"github.com/helixml/helix/api/pkg/types"
)

type mcpGrantAgentCreator struct{}

func (mcpGrantAgentCreator) CreateAgent(context.Context, string, string, string, lifecycle.AgentConfig) (lifecycle.CreatedAgent, error) {
	return lifecycle.CreatedAgent{LegacyAppID: "app-grant-test"}, nil
}

func TestHelixOrgMCPMemberCannotGrantRestrictedTool(t *testing.T) {
	session, st := newMCPGrantGateway(t, types.OrganizationRoleMember, nil)
	result, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name: string(mcptools.AttachToolName),
		Arguments: map[string]any{
			"botId": "b-target",
			"tools": []string{string(mcptools.DeleteBotName)},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !result.IsError {
		t.Error("member principal granted an organization-management tool")
	}
	assertMCPGrantToolAbsent(t, st, "b-target", mcptools.DeleteBotName)
}

func TestHelixOrgMCPOwnerBotCanCreateWithHeldTool(t *testing.T) {
	session, st := newMCPGrantGateway(t, types.OrganizationRoleOwner, nil)
	result, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name: string(mcptools.CreateBotName),
		Arguments: map[string]any{
			"id":       "b-child",
			"content":  "# Child",
			"tools":    []string{string(mcptools.DeleteBotName)},
			"triggers": []string{},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.IsError {
		t.Fatalf("owner Chief of Staff create_bot failed: %+v", result.Content)
	}
	child, err := st.Nodes.Get(context.Background(), "org-test", "b-child")
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Contains(child.Tools, mcptools.DeleteBotName) {
		t.Fatal("owner Chief of Staff child is missing requested held tool")
	}
}

func TestHelixOrgMCPInstanceCannotGrantToolOutsideProfile(t *testing.T) {
	profile := &types.BotInstanceProfile{Tools: []string{string(mcptools.AttachToolName)}}
	session, st := newMCPGrantGateway(t, types.OrganizationRoleOwner, profile)
	result, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name: string(mcptools.AttachToolName),
		Arguments: map[string]any{
			"botId": "b-target",
			"tools": []string{string(mcptools.DeleteBotName)},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !result.IsError {
		t.Error("owner's instance granted a tool outside its effective profile")
	}
	assertMCPGrantToolAbsent(t, st, "b-target", mcptools.DeleteBotName)
}

func newMCPGrantGateway(t *testing.T, role types.OrganizationRole, profile *types.BotInstanceProfile) (*mcp.ClientSession, *orgstore.Store) {
	t.Helper()
	ctx := context.Background()
	st := orgmemory.New()
	cfg := mcptools.DefaultDeps(st)
	cfg.AgentCreator = mcpGrantAgentCreator{}
	cfg.Publishing = publishing.New(publishing.Deps{
		Triggers: st.Triggers,
		Events:   st.Events,
		Now:      cfg.Now,
		NewID:    cfg.NewID,
	})
	reg := mcptools.NewRegistry()
	if err := mcptools.RegisterBuiltins(reg, cfg.Build()); err != nil {
		t.Fatal(err)
	}
	for _, bot := range []orgchart.Node{
		mustMCPGrantNode(t, "b-parent", mcptools.OwnerBotTools()),
		mustMCPGrantNode(t, "b-target", mcptools.DefaultBotTools()),
	} {
		if err := st.Nodes.Create(ctx, bot); err != nil {
			t.Fatal(err)
		}
	}

	orgServer := helixorgserver.NewFromStore(st, reg, nil, nil, nil)
	sessionRow := &types.Session{
		ID: "ses-grant-test", Owner: "user-1", OrganizationID: "org-test",
		Metadata: types.SessionMetadata{OrgWorkerID: "b-parent", BotInstance: profile},
	}
	handlers := &helixOrgHandlers{
		api: orgServer.Handler(), botMCP: orgServer.BotMCPHandler(),
		mcpServer: orgServer, store: st,
		scope: &helixOrgScope{bootstrapped: map[string]bool{"org-test": true}},
	}
	apiServer := &HelixAPIServer{Store: &helixOrgRouteTestStore{role: role, session: sessionRow}}
	backend := NewHelixOrgMCPBackend(apiServer, handlers)
	user := &types.User{
		ID: "user-1", TokenType: types.TokenTypeAPIKey,
		SessionID: sessionRow.ID, OrganizationID: sessionRow.OrganizationID,
	}
	httpServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		backend.ServeHTTP(w, mux.SetURLVars(r, map[string]string{"path": ""}), user)
	}))
	t.Cleanup(httpServer.Close)

	client := mcp.NewClient(&mcp.Implementation{Name: "mcp-grant-test", Version: "0"}, nil)
	session, err := client.Connect(ctx, &mcp.StreamableClientTransport{
		Endpoint: httpServer.URL, DisableStandaloneSSE: true,
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = session.Close() })
	return session, st
}

func mustMCPGrantNode(t *testing.T, id string, tools []tool.Name) orgchart.Node {
	t.Helper()
	bot, err := orgchart.NewNode(orgchart.NodeID(id), "# "+id, tools, time.Now().UTC(), "org-test")
	if err != nil {
		t.Fatal(err)
	}
	return bot
}

func assertMCPGrantToolAbsent(t *testing.T, st *orgstore.Store, id orgchart.NodeID, name tool.Name) {
	t.Helper()
	bot, err := st.Nodes.Get(context.Background(), "org-test", id)
	if err != nil {
		t.Fatal(err)
	}
	if slices.Contains(bot.Tools, name) {
		t.Fatalf("%s gained forbidden tool %s", id, name)
	}
}
