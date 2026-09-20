package api_test

import (
	"context"
	"net/http"
	"testing"

	"github.com/helixml/helix/api/pkg/org/domain/orgchart"
	orgapi "github.com/helixml/helix/api/pkg/org/interfaces/server/api"
)

type fakeBotConfigApplier struct {
	calls int
	botID orgchart.NodeID
}

func (f *fakeBotConfigApplier) ApplyConfig(_ context.Context, _ string, botID orgchart.NodeID) error {
	f.calls++
	f.botID = botID
	return nil
}

func TestApplyBotConfig_RestartsExistingSessionWithoutActivation(t *testing.T) {
	deps, st, _ := newDeps(t)
	seedBot(t, st, context.Background(), "b-alice", "# Alice")
	applier := &fakeBotConfigApplier{}
	deps.BotConfigApplier = applier

	rec := do(t, orgapi.Handler(deps), "POST", "/bots/b-alice/apply-config", nil)
	if rec.Code != http.StatusAccepted {
		t.Fatalf("status = %d, want 202; body=%s", rec.Code, rec.Body)
	}
	if applier.calls != 1 || applier.botID != "b-alice" {
		t.Fatalf("apply calls/id = %d/%q, want 1/b-alice", applier.calls, applier.botID)
	}
}
