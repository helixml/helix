package mcptools

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/helixml/helix/api/pkg/org/application/activations"
	"github.com/helixml/helix/api/pkg/org/application/lifecycle"
	"github.com/helixml/helix/api/pkg/org/domain/orgchart"
	orggorm "github.com/helixml/helix/api/pkg/org/infrastructure/persistence/gorm"
)

type gatedInstances struct{ release chan struct{} }

func (g gatedInstances) DeleteAll(context.Context, string, orgchart.NodeID) error {
	<-g.release
	return nil
}

// TestMCPToolsWhileBotIsBeingDeleted pins that, while a bot's delete is in
// flight, list_bots and get_bot hide it, and start_bot and delete_bot refuse
// rather than provisioning it or running a second cascade.
func TestMCPToolsWhileBotIsBeingDeleted(t *testing.T) {
	st := orggorm.GetOrgTestDB(t)
	release := make(chan struct{})
	lc := &lifecycle.Service{Store: st, Instances: gatedInstances{release: release}}
	deps := DefaultDeps(st)
	deps.Now = func() time.Time { return time.Date(2026, 6, 10, 0, 0, 0, 0, time.UTC) }
	deps.Lifecycle = lc
	injectTestPublishing(&deps)
	deps.Activations = activations.New(activations.Deps{Repo: st.Activations, NewID: func() string { return "act" }})
	reg := NewRegistry()
	deps.KnownTools = Catalogue(reg)
	if err := RegisterBuiltins(reg, deps.Build()); err != nil {
		t.Fatalf("register builtins: %v", err)
	}
	owner, err := orgchart.NewNode("b-owner", "# Owner", OwnerBotTools(), deps.Now(), "org-test")
	if err != nil {
		t.Fatalf("seed owner: %v", err)
	}
	if err := st.Nodes.Create(context.Background(), owner); err != nil {
		t.Fatalf("create owner: %v", err)
	}
	caller := botCaller{id: "b-owner", orgID: "org-test", admin: true}
	seedBotRow(t, st, "b-leaving")

	if err := lc.StartDelete(context.Background(), "org-test", "b-leaving"); err != nil {
		t.Fatalf("StartDelete: %v", err)
	}

	raw, err := invoke(t, reg, caller, ListBotsName, map[string]any{})
	if err != nil {
		t.Fatalf("list_bots: %v", err)
	}
	var listed struct {
		Bots []struct {
			ID string `json:"id"`
		} `json:"bots"`
	}
	if err := json.Unmarshal(raw, &listed); err != nil {
		t.Fatalf("decode list_bots: %v", err)
	}
	for _, b := range listed.Bots {
		if b.ID == "b-leaving" {
			t.Fatal("list_bots shows a bot being deleted")
		}
	}
	if _, err := invoke(t, reg, caller, GetBotName, map[string]any{"id": "b-leaving"}); !errors.Is(err, lifecycle.ErrDeleteInProgress) {
		t.Fatalf("get_bot: err=%v, want ErrDeleteInProgress", err)
	}
	if _, err := invoke(t, reg, caller, StartBotName, map[string]any{"bot_id": "b-leaving"}); !errors.Is(err, lifecycle.ErrDeleteInProgress) {
		t.Fatalf("start_bot: err=%v, want ErrDeleteInProgress", err)
	}
	if _, err := invoke(t, reg, caller, DeleteBotName, map[string]any{"botId": "b-leaving"}); !errors.Is(err, lifecycle.ErrDeleteInProgress) {
		t.Fatalf("delete_bot: err=%v, want ErrDeleteInProgress", err)
	}

	close(release)
	deadline := time.Now().Add(5 * time.Second)
	for lc.Deleting("org-test", "b-leaving") {
		if time.Now().After(deadline) {
			t.Fatal("background delete did not finish")
		}
		time.Sleep(10 * time.Millisecond)
	}
}
