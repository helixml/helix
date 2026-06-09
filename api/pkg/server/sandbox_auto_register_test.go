package server

import (
	"context"
	"errors"
	"testing"

	"github.com/helixml/helix/api/pkg/sandbox/compute"
	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/types"
	"go.uber.org/mock/gomock"
)

// These tests cover the three branches of ensureSandboxRegistered:
//   1. Manager-provisioned host registering for the first time
//      (row exists, ComputeState in {provisioning, failed}) - bridge
//      forward-transitions to ready via THREE targeted column updates
//      (compute_state, status, network). The split was driven by a
//      review finding that the previous full-row gorm.Save approach
//      would clobber concurrent heartbeat writes.
//   2. Reconnect of a previously-online host (row exists, ComputeState
//      ready/terminating/terminated/empty) - legacy reset path
//   3. Self-registered host appearing for the first time (no row) -
//      legacy insert path

func TestEnsureSandboxRegistered_BridgesProvisioningRow_TargetedUpdates(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockStore := store.NewMockStore(ctrl)
	server := &HelixAPIServer{Store: mockStore}

	existing := &types.SandboxInstance{
		ID:           "sbx_provisioning",
		Provider:     "yellowdog-prod",
		ProviderID:   "ydid:workreq:abc",
		ComputeState: string(compute.StateProvisioning),
		Status:       "offline",
		MaxSandboxes: 20,
	}
	mockStore.EXPECT().
		ListSandboxInstances(gomock.Any()).
		Return([]*types.SandboxInstance{existing}, nil)

	// Bridge MUST use targeted column updates, NOT the full-row Save
	// path (RegisterSandboxInstance). Regression guard for a
	// review-surfaced bug: full-row Save would clobber heartbeat
	// columns that the heartbeat goroutine writes between our load
	// and our save.
	mockStore.EXPECT().
		UpdateSandboxInstanceComputeState(gomock.Any(), "sbx_provisioning", string(compute.StateReady)).
		Return(nil)
	mockStore.EXPECT().
		UpdateSandboxInstanceStatus(gomock.Any(), "sbx_provisioning", "online").
		Return(nil)
	mockStore.EXPECT().
		UpdateSandboxInstanceNetwork(gomock.Any(), "sbx_provisioning", "10.0.0.1", gomock.Any(), gomock.Any()).
		Return(nil)

	server.ensureSandboxRegistered(context.Background(), "sbx_provisioning", "10.0.0.1")
}

func TestEnsureSandboxRegistered_BridgesFailedRow_RecoveryPath(t *testing.T) {
	// Regression guard for a review-surfaced recovery gap: a host
	// whose row got rolled forward to ComputeState=failed (transient
	// HealthCheck blip, Provider thought it was dead) may still
	// phone home later.
	// The bridge MUST forward-transition it to ready, otherwise it
	// stays failed forever - isAvailable() returns false, Manager
	// pre-warms a replacement, and the host serves but doesn't count.
	ctrl := gomock.NewController(t)
	mockStore := store.NewMockStore(ctrl)
	server := &HelixAPIServer{Store: mockStore}

	existing := &types.SandboxInstance{
		ID:           "sbx_recovered",
		Provider:     "yellowdog-prod",
		ProviderID:   "ydid:workreq:xyz",
		ComputeState: string(compute.StateFailed),
		Status:       "offline",
	}
	mockStore.EXPECT().
		ListSandboxInstances(gomock.Any()).
		Return([]*types.SandboxInstance{existing}, nil)
	mockStore.EXPECT().
		UpdateSandboxInstanceComputeState(gomock.Any(), "sbx_recovered", string(compute.StateReady)).
		Return(nil)
	mockStore.EXPECT().
		UpdateSandboxInstanceStatus(gomock.Any(), "sbx_recovered", "online").
		Return(nil)
	mockStore.EXPECT().
		UpdateSandboxInstanceNetwork(gomock.Any(), "sbx_recovered", "10.0.0.2", gomock.Any(), gomock.Any()).
		Return(nil)
	// And critically NOT ResetSandboxOnReconnect or RegisterSandboxInstance.

	server.ensureSandboxRegistered(context.Background(), "sbx_recovered", "10.0.0.2")
}

func TestEnsureSandboxRegistered_LegacyReconnectPathUnchanged(t *testing.T) {
	// Regression guard: a row in any non-provisioning state must go
	// through ResetSandboxOnReconnect, not the bridge path. Otherwise
	// every reconnect would re-trigger the "first-time" registration
	// log spam and overwrite heartbeat fields.
	ctrl := gomock.NewController(t)
	mockStore := store.NewMockStore(ctrl)
	server := &HelixAPIServer{Store: mockStore}

	existing := &types.SandboxInstance{
		ID:           "sbx_existing",
		Status:       "offline", // crashed and reconnecting
		ComputeState: string(compute.StateReady),
	}
	mockStore.EXPECT().
		ListSandboxInstances(gomock.Any()).
		Return([]*types.SandboxInstance{existing}, nil)
	mockStore.EXPECT().
		ResetSandboxOnReconnect(gomock.Any(), "sbx_existing").
		Return(nil)
	// Crucial: RegisterSandboxInstance must NOT be called for a
	// reconnect. The mock controller fails the test if any call is
	// made that wasn't EXPECT()-ed.

	server.ensureSandboxRegistered(context.Background(), "sbx_existing", "10.0.0.1")
}

func TestEnsureSandboxRegistered_LegacyReconnectForEmptyComputeState(t *testing.T) {
	// A row with empty ComputeState is a legacy self-registered host
	// (Manager-provisioned rows always have ComputeState set). The
	// empty value must NOT match the provisioning branch.
	ctrl := gomock.NewController(t)
	mockStore := store.NewMockStore(ctrl)
	server := &HelixAPIServer{Store: mockStore}

	existing := &types.SandboxInstance{
		ID:           "sbx_legacy",
		Status:       "online",
		ComputeState: "", // legacy self-registered
		Provider:     "",
	}
	mockStore.EXPECT().
		ListSandboxInstances(gomock.Any()).
		Return([]*types.SandboxInstance{existing}, nil)
	mockStore.EXPECT().
		ResetSandboxOnReconnect(gomock.Any(), "sbx_legacy").
		Return(nil)

	server.ensureSandboxRegistered(context.Background(), "sbx_legacy", "10.0.0.1")
}

func TestEnsureSandboxRegistered_NoRowInsertsFreshLegacyRow(t *testing.T) {
	// Self-registered host that has never been seen before: no row
	// exists, so we INSERT a fresh one with Provider="" (legacy).
	ctrl := gomock.NewController(t)
	mockStore := store.NewMockStore(ctrl)
	server := &HelixAPIServer{Store: mockStore}

	mockStore.EXPECT().
		ListSandboxInstances(gomock.Any()).
		Return([]*types.SandboxInstance{}, nil)
	mockStore.EXPECT().
		RegisterSandboxInstance(gomock.Any(), gomock.AssignableToTypeOf(&types.SandboxInstance{})).
		DoAndReturn(func(_ context.Context, inst *types.SandboxInstance) error {
			if inst.ID != "sbx_new" {
				t.Errorf("wrong ID: %q", inst.ID)
			}
			if inst.Provider != "" {
				t.Errorf("legacy auto-register should leave Provider empty, got %q", inst.Provider)
			}
			if inst.ComputeState != "" {
				t.Errorf("legacy auto-register should leave ComputeState empty, got %q", inst.ComputeState)
			}
			if inst.Status != "online" {
				t.Errorf("Status should be online, got %q", inst.Status)
			}
			return nil
		})

	server.ensureSandboxRegistered(context.Background(), "sbx_new", "10.0.0.1")
}

func TestEnsureSandboxRegistered_BridgeEarlyStoreErrorAborts(t *testing.T) {
	// If the first targeted update (compute_state) fails, we MUST
	// NOT proceed to the subsequent updates and MUST NOT fall through
	// to the legacy paths. Next reconcile cycle will retry.
	ctrl := gomock.NewController(t)
	mockStore := store.NewMockStore(ctrl)
	server := &HelixAPIServer{Store: mockStore}

	existing := &types.SandboxInstance{
		ID:           "sbx_provisioning_err",
		Provider:     "yellowdog-prod",
		ComputeState: string(compute.StateProvisioning),
	}
	mockStore.EXPECT().
		ListSandboxInstances(gomock.Any()).
		Return([]*types.SandboxInstance{existing}, nil)
	mockStore.EXPECT().
		UpdateSandboxInstanceComputeState(gomock.Any(), "sbx_provisioning_err", string(compute.StateReady)).
		Return(errors.New("simulated db error"))
	// And NOT UpdateSandboxInstanceStatus, NOT UpdateSandboxInstanceNetwork,
	// NOT ResetSandboxOnReconnect, NOT RegisterSandboxInstance.

	server.ensureSandboxRegistered(context.Background(), "sbx_provisioning_err", "10.0.0.1")
}
