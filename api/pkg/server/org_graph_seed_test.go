package server

import (
	"context"
	"errors"
	"slices"
	"testing"

	"github.com/helixml/helix/api/pkg/org/application/lifecycle"
	"github.com/helixml/helix/api/pkg/org/application/nodes"
	"github.com/helixml/helix/api/pkg/org/domain/activation"
	"github.com/helixml/helix/api/pkg/org/domain/orgchart"
	orggorm "github.com/helixml/helix/api/pkg/org/infrastructure/persistence/gorm"
	runtimehelix "github.com/helixml/helix/api/pkg/org/infrastructure/runtime/helix"
	"github.com/helixml/helix/api/pkg/org/interfaces/mcptools"
	"github.com/helixml/helix/api/pkg/types"
)

type seedDispatcher struct {
	ids           []orgchart.NodeID
	activationIDs []activation.ID
}

type seedAgentCreator struct{ userIDs *[]string }

func (c seedAgentCreator) CreateAgent(ctx context.Context, _ string, _ string, _ string, _ lifecycle.AgentConfig) (lifecycle.CreatedAgent, error) {
	if c.userIDs != nil {
		*c.userIDs = append(*c.userIDs, runtimehelix.UserIDFromContext(ctx))
	}
	return lifecycle.CreatedAgent{LegacyAppID: "app-test"}, nil
}

type seedHireRecorder struct{ hiringUserIDs []string }

func (r *seedHireRecorder) OnHire(_ context.Context, _ string, _ orgchart.NodeID, hiringUserID string) error {
	r.hiringUserIDs = append(r.hiringUserIDs, hiringUserID)
	return nil
}

// The Chief of Staff is hired as the organization owner, whoever's request
// happens to bootstrap the org: it carries OwnerBotTools, so its agent app,
// project and activations must not belong to an arbitrary member.
func TestSeedChiefOfStaffHiresAsOrganizationOwner(t *testing.T) {
	t.Parallel()
	st := orggorm.GetOrgTestDB(t)
	deps := mcptools.DefaultDeps(st).Build()
	var agentUsers []string
	deps.Lifecycle.Agents = seedAgentCreator{userIDs: &agentUsers}
	hires := &seedHireRecorder{}
	deps.Lifecycle.HireHook = hires
	owner := &types.User{ID: "u-owner", Email: "owner@example.com"}
	seeder := &orgGraphSeeder{lifecycle: deps.Lifecycle, bots: deps.Nodes, botStore: st.Nodes,
		owner: func(context.Context, string) (*types.User, error) { return owner, nil }}

	member := &types.User{ID: "u-member", Email: "member@example.com"}
	ctx := runtimehelix.WithUser(runtimehelix.WithUserID(context.Background(), member.ID), member)
	if err := seeder.SeedChiefOfStaff(ctx, "org-seed-owner"); err != nil {
		t.Fatalf("seed chief of staff: %v", err)
	}
	if len(hires.hiringUserIDs) != 1 || hires.hiringUserIDs[0] != owner.ID {
		t.Fatalf("hiring user ids = %v, want [%s]", hires.hiringUserIDs, owner.ID)
	}
	if len(agentUsers) != 1 || agentUsers[0] != owner.ID {
		t.Fatalf("agent created as %v, want [%s]", agentUsers, owner.ID)
	}

	failing := &orgGraphSeeder{lifecycle: deps.Lifecycle, bots: deps.Nodes, botStore: st.Nodes,
		owner: func(context.Context, string) (*types.User, error) { return nil, errors.New("no owner") }}
	if err := failing.SeedChiefOfStaff(ctx, "org-seed-no-owner"); err == nil {
		t.Fatal("seed without a resolvable owner must fail rather than hire as the requester")
	}
	if _, err := st.Nodes.Get(ctx, "org-seed-no-owner", chiefOfStaffBotID); err == nil {
		t.Fatal("chief of staff was seeded without an owner")
	}
}

func TestSeedChiefOfStaffRespectsDeletionAndExplicitRecreation(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	st := orggorm.GetOrgTestDB(t)
	deps := mcptools.DefaultDeps(st).Build()
	deps.Lifecycle.Agents = seedAgentCreator{}
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
	deps.Lifecycle.Agents = seedAgentCreator{}
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
