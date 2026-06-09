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
//      (row exists, ComputeState=provisioning) - bridge path
//   2. Reconnect of a previously-online host (row exists, any other
//      ComputeState) - legacy reset path
//   3. Self-registered host appearing for the first time (no row) -
//      legacy insert path
//
// The bridge path (1) is the new behaviour added in D2; the others
// are regression guards to prove we didn't break the existing flows.

func TestEnsureSandboxRegistered_BridgesManagerProvisionedRow(t *testing.T) {
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

	// The bridge MUST update the row in place (preserving Provider,
	// ProviderID) and Save it; not call ResetSandboxOnReconnect, and
	// not insert a duplicate row.
	mockStore.EXPECT().
		RegisterSandboxInstance(gomock.Any(), gomock.AssignableToTypeOf(&types.SandboxInstance{})).
		DoAndReturn(func(_ context.Context, inst *types.SandboxInstance) error {
			if inst.ID != "sbx_provisioning" {
				t.Errorf("wrong ID: %q", inst.ID)
			}
			if inst.Provider != "yellowdog-prod" {
				t.Errorf("Provider clobbered: %q", inst.Provider)
			}
			if inst.ProviderID != "ydid:workreq:abc" {
				t.Errorf("ProviderID clobbered: %q", inst.ProviderID)
			}
			if inst.ComputeState != string(compute.StateReady) {
				t.Errorf("ComputeState should transition to ready, got %q", inst.ComputeState)
			}
			if inst.Status != "online" {
				t.Errorf("Status should be online, got %q", inst.Status)
			}
			if inst.IPAddress != "10.0.0.1" {
				t.Errorf("IPAddress should be set, got %q", inst.IPAddress)
			}
			return nil
		})

	server.ensureSandboxRegistered(context.Background(), "sbx_provisioning", "10.0.0.1")
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

func TestEnsureSandboxRegistered_BridgeStoreErrorIsLoggedAndReturns(t *testing.T) {
	// If the Save fails during the bridge transition, we don't fall
	// through to the legacy paths - the bridge function must surface
	// the error and return (the next reconcile cycle will retry).
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
		RegisterSandboxInstance(gomock.Any(), gomock.Any()).
		Return(errors.New("simulated db error"))

	server.ensureSandboxRegistered(context.Background(), "sbx_provisioning_err", "10.0.0.1")
}
