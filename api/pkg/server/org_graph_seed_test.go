package server

import (
	"context"
	"slices"
	"testing"

	"github.com/helixml/helix/api/pkg/org/application/lifecycle"
	"github.com/helixml/helix/api/pkg/org/application/nodes"
	"github.com/helixml/helix/api/pkg/org/domain/activation"
	"github.com/helixml/helix/api/pkg/org/domain/orgchart"
	orggorm "github.com/helixml/helix/api/pkg/org/infrastructure/persistence/gorm"
	"github.com/helixml/helix/api/pkg/org/interfaces/mcptools"
)

type seedDispatcher struct {
	ids           []orgchart.NodeID
	activationIDs []activation.ID
}

func TestSeedChiefOfStaffRespectsDeletionAndExplicitRecreation(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	st := orggorm.GetOrgTestDB(t)
	deps := mcptools.DefaultDeps(st).Build()
	seeder := &orgGraphSeeder{lifecycle: deps.Lifecycle, bots: deps.Nodes, botStore: st.Nodes}
	const orgID = "org-chief-deletion"

	if err := seeder.SeedChiefOfStaff(ctx, orgID); err != nil {
		t.Fatalf("seed chief of staff: %v", err)
	}
	if err := deps.Lifecycle.Delete(ctx, orgID, chiefOfStaffBotID); err != nil {
		t.Fatalf("delete chief of staff: %v", err)
	}
	if err := seeder.SeedChiefOfStaff(ctx, orgID); err != nil {
		t.Fatalf("bootstrap after deletion: %v", err)
	}
	if _, err := st.Nodes.Get(ctx, orgID, chiefOfStaffBotID); err == nil {
		t.Fatal("bootstrap recreated explicitly deleted chief of staff")
	}

	if _, err := deps.Lifecycle.Create(ctx, orgID, lifecycle.CreateParams{
		ID:              string(chiefOfStaffBotID),
		Name:            "Chief of Staff",
		Content:         chiefOfStaffContent,
		PreserveContext: true,
		DeferActivation: true,
	}); err != nil {
		t.Fatalf("explicitly recreate chief of staff: %v", err)
	}
	marked, err := deps.Lifecycle.ChiefOfStaffDeletionMarked(ctx, orgID)
	if err != nil {
		t.Fatal(err)
	}
	if marked {
		t.Fatal("explicit recreation did not clear chief of staff deletion marker")
	}
}

func (d *seedDispatcher) DispatchHire(_ context.Context, _ string, id orgchart.NodeID, activationID activation.ID) {
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
	seeder := &orgGraphSeeder{lifecycle: deps.Lifecycle, bots: deps.Nodes, botStore: st.Nodes}

	if err := seeder.SeedChiefOfStaff(ctx, "org-new"); err != nil {
		t.Fatalf("seed new chief of staff: %v", err)
	}
	created, err := st.Nodes.Get(ctx, "org-new", chiefOfStaffBotID)
	if err != nil {
		t.Fatalf("get new chief of staff: %v", err)
	}
	if !created.PreserveContext {
		t.Fatal("new chief of staff must preserve conversation context")
	}
	for _, name := range mcptools.AssetManagementTools {
		if !slices.Contains(created.Tools, name) {
			t.Errorf("new chief of staff missing asset management tool %q", name)
		}
	}
	if len(dispatcher.ids) != 0 {
		t.Fatalf("seed dispatched bots = %v, want none before runtime selection", dispatcher.ids)
	}
	activations, err := st.Activations.ListForWorker(ctx, "org-new", chiefOfStaffBotID, 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(activations) != 0 {
		t.Fatalf("seed activations = %v, want none before runtime selection", activations)
	}

	if _, err := deps.Nodes.Create(ctx, "org-existing", nodes.CreateParams{
		ID:      string(chiefOfStaffBotID),
		Name:    "Chief of Staff",
		Content: chiefOfStaffContent,
	}); err != nil {
		t.Fatalf("create existing chief of staff: %v", err)
	}
	if err := seeder.SeedChiefOfStaff(ctx, "org-existing"); err != nil {
		t.Fatalf("reseed existing chief of staff: %v", err)
	}
	existing, err := st.Nodes.Get(ctx, "org-existing", chiefOfStaffBotID)
	if err != nil {
		t.Fatalf("get existing chief of staff: %v", err)
	}
	if existing.PreserveContext {
		t.Fatal("reseed must not override an existing chief of staff's context preference")
	}
	for _, name := range mcptools.AssetManagementTools {
		if !slices.Contains(existing.Tools, name) {
			t.Errorf("reseeded chief of staff missing asset management tool %q", name)
		}
	}
}
