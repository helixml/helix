package api_test

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/helixml/helix/api/pkg/org/domain/orgchart"
	orgapi "github.com/helixml/helix/api/pkg/org/interfaces/server/api"
)

type chatSurfaceBotRuntime struct {
	projectID string
	sessionID string
}

func (f chatSurfaceBotRuntime) State(_ context.Context, _ string, _ orgchart.NodeID) (orgapi.BotRuntimeInfo, error) {
	return orgapi.BotRuntimeInfo{ProjectID: f.projectID, SessionID: f.sessionID, AgentStatus: "running"}, nil
}

// The chat sidebar lists bots as top-level entries and opens their session
// directly, so the list endpoint must carry the bot's own project and session
// rather than making the sidebar fetch every bot's detail.
func TestRESTAgentListCarriesProjectAndSession(t *testing.T) {
	deps, st, _ := newDeps(t)
	ctx := context.Background()
	seedBot(t, st, ctx, "b-alice", "# Alice")
	deps.BotRuntime = chatSurfaceBotRuntime{projectID: "prj_alice", sessionID: "ses_alice"}

	rec := do(t, orgapi.Handler(deps), http.MethodGet, "/agents", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d; body=%s", rec.Code, rec.Body)
	}
	var got []map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	var alice map[string]any
	for _, bot := range got {
		if bot["id"] == "b-alice" {
			alice = bot
		}
	}
	if alice == nil {
		t.Fatalf("b-alice missing from list: %#v", got)
	}
	if alice["project_id"] != "prj_alice" || alice["session_id"] != "ses_alice" || alice["agent_status"] != "running" {
		t.Fatalf("list row = %#v", alice)
	}
}
