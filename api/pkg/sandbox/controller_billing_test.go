package sandbox

import (
	"context"
	"testing"
	"time"

	"github.com/helixml/helix/api/pkg/config"
	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/stretchr/testify/suite"
	"go.uber.org/mock/gomock"
)

type ControllerBillingSuite struct {
	suite.Suite
	ctrl       *gomock.Controller
	store      *store.MockStore
	controller *Controller
	ctx        context.Context
}

func TestControllerBillingSuite(t *testing.T) {
	suite.Run(t, new(ControllerBillingSuite))
}

func (s *ControllerBillingSuite) SetupTest() {
	s.ctrl = gomock.NewController(s.T())
	s.store = store.NewMockStore(s.ctrl)
	s.ctx = context.Background()

	runtimes, err := NewRuntimeRegistry(config.Sandboxes{
		Runtimes:       "headless-ubuntu=ubuntu:22.04|sleep infinity",
		DefaultRuntime: "headless-ubuntu",
	})
	s.Require().NoError(err)
	s.controller = New(s.store, nil, runtimes, "", "")
}

func (s *ControllerBillingSuite) TearDownTest() {
	s.ctrl.Finish()
}

func (s *ControllerBillingSuite) TestReapBillingChargesElapsedMinutesAfterRestart() {
	lastChargedAt := time.Now().Add(-7*time.Minute - 10*time.Second)
	startedAt := lastChargedAt.Add(-time.Minute)
	sb := &types.Sandbox{
		ID:                   "sbx_restart",
		OrganizationID:       "org_1",
		Runtime:              types.SandboxRuntimeHeadlessUbuntu,
		Status:               types.SandboxStatusRunning,
		VCPUs:                4,
		StartedAt:            &startedAt,
		BillingLastChargedAt: &lastChargedAt,
	}

	s.store.EXPECT().GetSystemSettings(gomock.Any()).Return(&types.SystemSettings{
		SandboxBillingEnabled:                true,
		SandboxHeadlessPriceCreditsPerSecond: 0.5,
	}, nil)
	s.store.EXPECT().ListSandboxes(gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, q *store.ListSandboxesQuery) ([]*types.Sandbox, error) {
			s.Require().Equal(types.SandboxStatusRunning, q.Status)
			return []*types.Sandbox{sb}, nil
		},
	)
	s.store.EXPECT().GetWalletByOrg(gomock.Any(), "org_1").Return(&types.Wallet{
		ID:      "wallet_1",
		OrgID:   "org_1",
		Balance: 1000,
	}, nil)
	s.store.EXPECT().UpdateWalletBalance(gomock.Any(), "wallet_1", gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, walletID string, amount float64, meta types.TransactionMetadata) (*types.Wallet, error) {
			s.Require().InDelta(-840.0, amount, 0.0001)
			s.Require().Equal(types.TransactionTypeUsage, meta.TransactionType)
			s.Require().Equal("sbx_restart", meta.SandboxID)
			s.Require().Equal(types.SandboxRuntimeHeadlessUbuntu, meta.SandboxRuntime)
			s.Require().Equal("headless", meta.SandboxPricingType)
			return &types.Wallet{ID: walletID, Balance: 790}, nil
		},
	)
	s.store.EXPECT().SetSandboxBillingLastChargedAt(gomock.Any(), "sbx_restart", gomock.Any()).DoAndReturn(
		func(_ context.Context, _ string, chargedAt time.Time) error {
			s.Require().WithinDuration(lastChargedAt.Add(7*time.Minute), chargedAt, time.Millisecond)
			return nil
		},
	)

	s.Require().NoError(s.controller.ReapBilling(s.ctx))
}

func (s *ControllerBillingSuite) TestDeleteBillsRemainingSecondsSinceLastMinute() {
	lastChargedAt := time.Now().Add(-45 * time.Second)
	startedAt := lastChargedAt.Add(-time.Minute)
	sb := &types.Sandbox{
		ID:                   "sbx_delete",
		OrganizationID:       "org_1",
		Runtime:              types.SandboxRuntimeUbuntuDesktop,
		Status:               types.SandboxStatusRunning,
		StartedAt:            &startedAt,
		BillingLastChargedAt: &lastChargedAt,
	}

	s.store.EXPECT().GetSandbox(gomock.Any(), "sbx_delete").Return(sb, nil)
	s.store.EXPECT().GetSystemSettings(gomock.Any()).Return(&types.SystemSettings{
		SandboxBillingEnabled:               true,
		SandboxDesktopPriceCreditsPerSecond: 0.2,
	}, nil)
	s.store.EXPECT().GetWalletByOrg(gomock.Any(), "org_1").Return(&types.Wallet{
		ID:      "wallet_1",
		OrgID:   "org_1",
		Balance: 100,
	}, nil)
	s.store.EXPECT().UpdateWalletBalance(gomock.Any(), "wallet_1", gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, walletID string, amount float64, meta types.TransactionMetadata) (*types.Wallet, error) {
			s.Require().InDelta(-9.0, amount, 0.5)
			s.Require().Equal(types.TransactionTypeUsage, meta.TransactionType)
			s.Require().Equal("sbx_delete", meta.SandboxID)
			s.Require().Equal(types.SandboxRuntimeUbuntuDesktop, meta.SandboxRuntime)
			s.Require().Equal("desktop", meta.SandboxPricingType)
			return &types.Wallet{ID: walletID, Balance: 91}, nil
		},
	)
	s.store.EXPECT().SetSandboxBillingLastChargedAt(gomock.Any(), "sbx_delete", gomock.Any()).Return(nil)
	s.store.EXPECT().SetSandboxStatus(gomock.Any(), "sbx_delete", types.SandboxStatusStopping, "").Return(nil)
	s.store.EXPECT().GetAPIKey(gomock.Any(), gomock.Any()).Return(nil, store.ErrNotFound)
	s.store.EXPECT().DeleteSandbox(gomock.Any(), "sbx_delete").Return(nil)

	s.Require().NoError(s.controller.Delete(s.ctx, "sbx_delete"))
}

func (s *ControllerBillingSuite) TestReapBillingStopsSandboxAndDrainsRemainingCredits() {
	lastChargedAt := time.Now().Add(-time.Minute - 5*time.Second)
	startedAt := lastChargedAt.Add(-time.Minute)
	sb := &types.Sandbox{
		ID:                   "sbx_insufficient",
		OrganizationID:       "org_1",
		Runtime:              types.SandboxRuntimeHeadlessUbuntu,
		Status:               types.SandboxStatusRunning,
		StartedAt:            &startedAt,
		BillingLastChargedAt: &lastChargedAt,
	}

	s.store.EXPECT().GetSystemSettings(gomock.Any()).Return(&types.SystemSettings{
		SandboxBillingEnabled:                true,
		SandboxHeadlessPriceCreditsPerSecond: 0.1,
	}, nil)
	s.store.EXPECT().ListSandboxes(gomock.Any(), gomock.Any()).Return([]*types.Sandbox{sb}, nil)
	s.store.EXPECT().GetWalletByOrg(gomock.Any(), "org_1").Return(&types.Wallet{
		ID:      "wallet_1",
		OrgID:   "org_1",
		Balance: 5,
	}, nil)
	s.store.EXPECT().UpdateWalletBalance(gomock.Any(), "wallet_1", -5.0, gomock.Any()).DoAndReturn(
		func(_ context.Context, walletID string, amount float64, meta types.TransactionMetadata) (*types.Wallet, error) {
			s.Require().Equal("sbx_insufficient", meta.SandboxID)
			return &types.Wallet{ID: walletID, Balance: 0}, nil
		},
	)
	s.store.EXPECT().GetSandbox(gomock.Any(), "sbx_insufficient").Return(sb, nil)
	s.store.EXPECT().GetSystemSettings(gomock.Any()).Return(&types.SystemSettings{
		SandboxBillingEnabled:                true,
		SandboxHeadlessPriceCreditsPerSecond: 0.1,
	}, nil)
	s.store.EXPECT().GetWalletByOrg(gomock.Any(), "org_1").Return(&types.Wallet{
		ID:      "wallet_1",
		OrgID:   "org_1",
		Balance: 0,
	}, nil)
	s.store.EXPECT().SetSandboxStatus(gomock.Any(), "sbx_insufficient", types.SandboxStatusStopping, "").Return(nil)
	s.store.EXPECT().GetAPIKey(gomock.Any(), gomock.Any()).Return(nil, store.ErrNotFound)
	s.store.EXPECT().DeleteSandbox(gomock.Any(), "sbx_insufficient").Return(nil)

	s.Require().NoError(s.controller.ReapBilling(s.ctx))
}

func (s *ControllerBillingSuite) TestCreateRejectsSandboxWhenBillingEnabledAndCreditsAreBelowOneMinute() {
	s.store.EXPECT().GetSystemSettings(gomock.Any()).Return(&types.SystemSettings{
		SandboxBillingEnabled:                true,
		SandboxHeadlessPriceCreditsPerSecond: 0.2,
	}, nil)
	s.store.EXPECT().ListSandboxes(gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, q *store.ListSandboxesQuery) ([]*types.Sandbox, error) {
			s.Require().Equal("org_1", q.OrganizationID)
			return nil, nil
		},
	)
	s.store.EXPECT().GetWalletByOrg(gomock.Any(), "org_1").Return(&types.Wallet{
		ID:      "wallet_1",
		OrgID:   "org_1",
		Balance: 47.99,
	}, nil)

	_, err := s.controller.Create(s.ctx, "org_1", "user_1", &types.CreateSandboxRequest{
		Runtime:  types.SandboxRuntimeHeadlessUbuntu,
		VCPUs:    4,
		MemoryMB: 8192,
	})
	s.Require().Error(err)
	s.Require().Contains(err.Error(), "insufficient credits")
}

func (s *ControllerBillingSuite) TestCreateRejectsInvalidSandboxResources() {
	_, err := s.controller.Create(s.ctx, "org_1", "user_1", &types.CreateSandboxRequest{
		Runtime:  types.SandboxRuntimeHeadlessUbuntu,
		VCPUs:    2,
		MemoryMB: 4096,
	})
	s.Require().Error(err)
	s.Require().Contains(err.Error(), "invalid sandbox resources")
}

func (s *ControllerBillingSuite) TestResolveSandboxResourcesAllowsOnlyPresets() {
	tests := []struct {
		name     string
		req      *types.CreateSandboxRequest
		vcpus    int
		memoryMB int
	}{
		{name: "default", req: &types.CreateSandboxRequest{}, vcpus: 1, memoryMB: 2048},
		{name: "small", req: &types.CreateSandboxRequest{VCPUs: 1, MemoryMB: 2048}, vcpus: 1, memoryMB: 2048},
		{name: "medium", req: &types.CreateSandboxRequest{VCPUs: 4, MemoryMB: 8192}, vcpus: 4, memoryMB: 8192},
		{name: "large", req: &types.CreateSandboxRequest{VCPUs: 8, MemoryMB: 16384}, vcpus: 8, memoryMB: 16384},
	}
	for _, tt := range tests {
		s.Run(tt.name, func() {
			vcpus, memoryMB, err := resolveSandboxResources(tt.req)
			s.Require().NoError(err)
			s.Require().Equal(tt.vcpus, vcpus)
			s.Require().Equal(tt.memoryMB, memoryMB)
		})
	}
}

func (s *ControllerBillingSuite) TestCreateRejectsHeadlessSandboxWhenOrgLimitReached() {
	existing := make([]*types.Sandbox, 0, 10)
	for i := 0; i < 10; i++ {
		existing = append(existing, &types.Sandbox{
			ID:             "sbx_existing",
			OrganizationID: "org_1",
			Runtime:        types.SandboxRuntimeHeadlessUbuntu,
			Status:         types.SandboxStatusRunning,
		})
	}

	s.store.EXPECT().GetSystemSettings(gomock.Any()).Return(&types.SystemSettings{
		MaxConcurrentHeadlessSandboxes: 10,
	}, nil)
	s.store.EXPECT().ListSandboxes(gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, q *store.ListSandboxesQuery) ([]*types.Sandbox, error) {
			s.Require().Equal("org_1", q.OrganizationID)
			return existing, nil
		},
	)

	_, err := s.controller.Create(s.ctx, "org_1", "user_1", &types.CreateSandboxRequest{
		Runtime: types.SandboxRuntimeHeadlessUbuntu,
	})
	s.Require().Error(err)
	s.Require().Contains(err.Error(), "headless sandbox concurrency limit")
}

func (s *ControllerBillingSuite) TestCreateRejectsDesktopSandboxWhenOrgLimitReached() {
	existing := make([]*types.Sandbox, 0, 10)
	for i := 0; i < 10; i++ {
		existing = append(existing, &types.Sandbox{
			ID:             "sbx_desktop",
			OrganizationID: "org_1",
			Runtime:        types.SandboxRuntimeUbuntuDesktop,
			Status:         types.SandboxStatusPending,
		})
	}

	s.store.EXPECT().GetSystemSettings(gomock.Any()).Return(&types.SystemSettings{
		MaxConcurrentDesktopSandboxes: 10,
	}, nil)
	s.store.EXPECT().ListSandboxes(gomock.Any(), gomock.Any()).Return(existing, nil)

	_, err := s.controller.Create(s.ctx, "org_1", "user_1", &types.CreateSandboxRequest{
		Runtime: types.SandboxRuntimeUbuntuDesktop,
	})
	s.Require().Error(err)
	s.Require().Contains(err.Error(), "desktop sandbox concurrency limit")
}
