package server

import (
	"context"
	"testing"

	"github.com/helixml/helix/api/pkg/org/application/bots"
	"github.com/helixml/helix/api/pkg/org/domain/activation"
	"github.com/helixml/helix/api/pkg/org/domain/orgchart"
	orggorm "github.com/helixml/helix/api/pkg/org/infrastructure/persistence/gorm"
	"github.com/helixml/helix/api/pkg/org/interfaces/mcptools"
)

type seedDispatcher struct {
	ids           []orgchart.BotID
	activationIDs []activation.ID
}

func (d *seedDispatcher) DispatchHire(_ context.Context, _ string, id orgchart.BotID, activationID activation.ID) {
	d.ids = append(d.ids, id)
	d.activationIDs = append(d.activationIDs, activationID)
}

func TestSeedChiefOfStaffPreservesContextForNewBotOnly(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	st := orggorm.GetOrgTestDB(t)
	deps := mcptools.DefaultDeps(st).Build()
	dispatcher := &seedDispatcher{}
	deps.Lifecycle.Dispatcher = dispatcher
	seeder := &orgGraphSeeder{lifecycle: deps.Lifecycle, bots: deps.Bots, botStore: st.Bots}

	if err := seeder.SeedChiefOfStaff(ctx, "org-new"); err != nil {
		t.Fatalf("seed new chief of staff: %v", err)
	}
	created, err := st.Bots.Get(ctx, "org-new", chiefOfStaffBotID)
	if err != nil {
		t.Fatalf("get new chief of staff: %v", err)
	}
	if !created.PreserveContext {
		t.Fatal("new chief of staff must preserve conversation context")
	}
	if len(dispatcher.ids) != 1 || dispatcher.ids[0] != chiefOfStaffBotID {
		t.Fatalf("seed dispatched bots = %v, want [chief-of-staff]", dispatcher.ids)
	}
	if len(dispatcher.activationIDs) != 1 || dispatcher.activationIDs[0] == "" {
		t.Fatalf("seed activation ids = %v, want one non-empty id", dispatcher.activationIDs)
	}

	if _, err := deps.Bots.Create(ctx, "org-existing", bots.CreateParams{
		ID:      string(chiefOfStaffBotID),
		Name:    "Chief of Staff",
		Content: chiefOfStaffContent,
	}); err != nil {
		t.Fatalf("create existing chief of staff: %v", err)
	}
	if err := seeder.SeedChiefOfStaff(ctx, "org-existing"); err != nil {
		t.Fatalf("reseed existing chief of staff: %v", err)
	}
	existing, err := st.Bots.Get(ctx, "org-existing", chiefOfStaffBotID)
	if err != nil {
		t.Fatalf("get existing chief of staff: %v", err)
	}
	if existing.PreserveContext {
		t.Fatal("reseed must not override an existing chief of staff's context preference")
	}
}
