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

type ControllerSessionSuite struct {
	suite.Suite
	ctrl       *gomock.Controller
	store      *store.MockStore
	controller *Controller
	ctx        context.Context
}

func TestControllerSessionSuite(t *testing.T) {
	suite.Run(t, new(ControllerSessionSuite))
}

func (s *ControllerSessionSuite) SetupTest() {
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

func (s *ControllerSessionSuite) TearDownTest() {
	s.ctrl.Finish()
}

func (s *ControllerSessionSuite) beginRequest() *types.BeginSandboxSessionRequest {
	return &types.BeginSandboxSessionRequest{
		SessionID:      "ses_1",
		OrganizationID: "org_1",
		Owner:          "user_1",
		ProjectID:      "prj_1",
		SpecTaskID:     "spt_1",
		Name:           "Add billing",
		Runtime:        types.SandboxRuntimeUbuntuDesktop,
		VCPUs:          4,
		MemoryMB:       8192,
	}
}

func (s *ControllerSessionSuite) TestBeginSessionCreatesNeverExpiringDesktopRow() {
	s.store.EXPECT().GetSystemSettings(gomock.Any()).Return(&types.SystemSettings{}, nil)
	s.store.EXPECT().GetSandboxBySession(gomock.Any(), "ses_1").Return(nil, store.ErrNotFound)
	s.store.EXPECT().ListSandboxes(gomock.Any(), gomock.Any()).Return(nil, nil)
	s.store.EXPECT().CreateSandbox(gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, sb *types.Sandbox) (*types.Sandbox, error) {
			s.Require().Equal("ses_1", sb.SessionID)
			s.Require().Equal("spt_1", sb.SpecTaskID)
			s.Require().Equal(types.SandboxRuntimeUbuntuDesktop, sb.Runtime)
			s.Require().Equal(types.SandboxStatusPending, sb.Status)
			s.Require().Equal(4, sb.VCPUs)
			s.Require().Equal(8192, sb.MemoryMB)
			// Negative TTL leaves expires_at NULL so ReapExpired never tears
			// down a desktop the task still owns.
			s.Require().Equal(-1, sb.TimeoutSeconds)
			s.Require().True(sb.SessionBacked())
			sb.ID = "sbx_new"
			return sb, nil
		},
	)

	sb, err := s.controller.BeginSession(s.ctx, s.beginRequest())
	s.Require().NoError(err)
	s.Require().Equal("sbx_new", sb.ID)
}

func (s *ControllerSessionSuite) TestBeginSessionSkipsMeteringWithoutOrganization() {
	req := s.beginRequest()
	req.OrganizationID = ""

	// No store calls at all: wallets are org-scoped, so there is nothing to bill.
	sb, err := s.controller.BeginSession(s.ctx, req)
	s.Require().NoError(err)
	s.Require().Nil(sb)
}

func (s *ControllerSessionSuite) TestBeginSessionRejectsStartWhenOrgCannotAffordAMinute() {
	s.store.EXPECT().GetSystemSettings(gomock.Any()).Return(&types.SystemSettings{
		SandboxBillingEnabled:               true,
		SandboxDesktopPriceCreditsPerSecond: 0.1,
	}, nil)
	s.store.EXPECT().GetSandboxBySession(gomock.Any(), "ses_1").Return(nil, store.ErrNotFound)
	s.store.EXPECT().ListSandboxes(gomock.Any(), gomock.Any()).Return(nil, nil)
	// One minute at 4 cores costs 0.1 * 60 * 4 = 24 credits.
	s.store.EXPECT().GetWalletByOrg(gomock.Any(), "org_1").Return(&types.Wallet{
		ID:      "wallet_1",
		OrgID:   "org_1",
		Balance: 23.99,
	}, nil)

	_, err := s.controller.BeginSession(s.ctx, s.beginRequest())
	s.Require().Error(err)
	s.Require().Contains(err.Error(), "insufficient credits")
}

func (s *ControllerSessionSuite) TestBeginSessionRejectsStartWhenDesktopLimitReached() {
	existing := make([]*types.Sandbox, 0, 3)
	for i := 0; i < 3; i++ {
		existing = append(existing, &types.Sandbox{
			ID:             "sbx_other",
			OrganizationID: "org_1",
			SessionID:      "ses_other",
			Runtime:        types.SandboxRuntimeUbuntuDesktop,
			Status:         types.SandboxStatusRunning,
		})
	}

	s.store.EXPECT().GetSystemSettings(gomock.Any()).Return(&types.SystemSettings{
		MaxConcurrentDesktopSandboxes: 3,
	}, nil)
	s.store.EXPECT().GetSandboxBySession(gomock.Any(), "ses_1").Return(nil, store.ErrNotFound)
	s.store.EXPECT().ListSandboxes(gomock.Any(), gomock.Any()).Return(existing, nil)

	_, err := s.controller.BeginSession(s.ctx, s.beginRequest())
	s.Require().Error(err)
	s.Require().Contains(err.Error(), "desktop sandbox concurrency limit")
}

// A paused task being resumed must not be blocked by the concurrency slot it
// is itself about to reoccupy.
func (s *ControllerSessionSuite) TestBeginSessionReusesRowOnRestartWithoutCountingItsOwnSlot() {
	stoppedAt := time.Now().Add(-time.Hour)
	chargedAt := stoppedAt.Add(-time.Minute)
	existing := &types.Sandbox{
		ID:                   "sbx_existing",
		OrganizationID:       "org_1",
		SessionID:            "ses_1",
		Runtime:              types.SandboxRuntimeUbuntuDesktop,
		Status:               types.SandboxStatusStopped,
		VCPUs:                1,
		MemoryMB:             2048,
		StoppedAt:            &stoppedAt,
		BillingLastChargedAt: &chargedAt,
	}
	// The org is at its cap of 1, and the only active desktop is this
	// session's own row.
	others := []*types.Sandbox{{
		ID:             "sbx_existing",
		OrganizationID: "org_1",
		SessionID:      "ses_1",
		Runtime:        types.SandboxRuntimeUbuntuDesktop,
		Status:         types.SandboxStatusRunning,
	}}

	s.store.EXPECT().GetSystemSettings(gomock.Any()).Return(&types.SystemSettings{
		MaxConcurrentDesktopSandboxes: 1,
	}, nil)
	s.store.EXPECT().GetSandboxBySession(gomock.Any(), "ses_1").Return(existing, nil)
	s.store.EXPECT().ListSandboxes(gomock.Any(), gomock.Any()).Return(others, nil)
	s.store.EXPECT().UpdateSandbox(gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, sb *types.Sandbox) (*types.Sandbox, error) {
			s.Require().Equal("sbx_existing", sb.ID)
			s.Require().Equal(types.SandboxStatusPending, sb.Status)
			s.Require().Equal(4, sb.VCPUs)
			s.Require().Equal(8192, sb.MemoryMB)
			// The stopped gap must not be billed on resume.
			s.Require().Nil(sb.BillingLastChargedAt)
			s.Require().Nil(sb.StartedAt)
			return sb, nil
		},
	)

	sb, err := s.controller.BeginSession(s.ctx, s.beginRequest())
	s.Require().NoError(err)
	s.Require().Equal("sbx_existing", sb.ID)
}

func (s *ControllerSessionSuite) TestMarkSessionRunningOpensBillingWindow() {
	s.store.EXPECT().GetSandboxBySession(gomock.Any(), "ses_1").Return(&types.Sandbox{
		ID:        "sbx_1",
		SessionID: "ses_1",
		Status:    types.SandboxStatusPending,
	}, nil)
	s.store.EXPECT().SetSandboxContainer(gomock.Any(), "sbx_1", "host_1", "cnt_1").Return(nil)
	s.store.EXPECT().SetSandboxStatus(gomock.Any(), "sbx_1", types.SandboxStatusRunning, "").Return(nil)

	s.Require().NoError(s.controller.MarkSessionRunning(s.ctx, "ses_1", "host_1", "cnt_1"))
}

// A desktop that never booted must not keep a concurrency slot: pending counts
// as active.
func (s *ControllerSessionSuite) TestMarkSessionFailedClosesRowWithoutCharging() {
	s.store.EXPECT().GetSandboxBySession(gomock.Any(), "ses_1").Return(&types.Sandbox{
		ID:             "sbx_1",
		OrganizationID: "org_1",
		SessionID:      "ses_1",
		Runtime:        types.SandboxRuntimeUbuntuDesktop,
		Status:         types.SandboxStatusPending,
	}, nil)
	// billSandboxFinal returns immediately for a row that never ran, so no
	// wallet call is expected here.
	s.store.EXPECT().SetSandboxStatus(gomock.Any(), "sbx_1", types.SandboxStatusFailed, "boom").Return(nil)

	s.Require().NoError(s.controller.MarkSessionFailed(s.ctx, "ses_1", "boom"))
}

func (s *ControllerSessionSuite) TestMarkSessionStoppedIsANoOpForUnmeteredSessions() {
	s.store.EXPECT().GetSandboxBySession(gomock.Any(), "ses_legacy").Return(nil, store.ErrNotFound)

	s.Require().NoError(s.controller.MarkSessionStopped(s.ctx, "ses_legacy"))
}

// Resizing 1 -> 8 vCPUs must settle the outstanding window at ONE core before
// the row starts billing at eight. Without the flush, billSandbox would
// multiply the whole elapsed window by the new core count.
func (s *ControllerSessionSuite) TestResizeSessionSettlesChargesAtOldCoreCountFirst() {
	chargedAt := time.Now().Add(-30 * time.Second)
	startedAt := chargedAt.Add(-time.Minute)
	existing := &types.Sandbox{
		ID:                   "sbx_1",
		OrganizationID:       "org_1",
		SessionID:            "ses_1",
		Runtime:              types.SandboxRuntimeUbuntuDesktop,
		Status:               types.SandboxStatusRunning,
		VCPUs:                1,
		MemoryMB:             2048,
		StartedAt:            &startedAt,
		BillingLastChargedAt: &chargedAt,
	}

	s.store.EXPECT().GetSandboxBySession(gomock.Any(), "ses_1").Return(existing, nil)
	s.store.EXPECT().GetSystemSettings(gomock.Any()).Return(&types.SystemSettings{
		SandboxBillingEnabled:               true,
		SandboxDesktopPriceCreditsPerSecond: 1,
	}, nil)
	s.store.EXPECT().GetWalletByOrg(gomock.Any(), "org_1").Return(&types.Wallet{
		ID:      "wallet_1",
		OrgID:   "org_1",
		Balance: 10000,
	}, nil)
	s.store.EXPECT().UpdateWalletBalance(gomock.Any(), "wallet_1", gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, walletID string, amount float64, _ types.TransactionMetadata) (*types.Wallet, error) {
			// 30 elapsed seconds at 1 credit/s/core and ONE core: ~-30, not -240.
			s.Require().InDelta(-30.0, amount, 1.0)
			return &types.Wallet{ID: walletID, Balance: 9970}, nil
		},
	)
	s.store.EXPECT().SetSandboxBillingLastChargedAt(gomock.Any(), "sbx_1", gomock.Any()).Return(nil)
	s.store.EXPECT().SetSandboxResources(gomock.Any(), "sbx_1", 8, 16384).Return(nil)

	s.Require().NoError(s.controller.ResizeSession(s.ctx, "ses_1", 8, 16384))
}

func (s *ControllerSessionSuite) TestResizeSessionSkipsFlushWhenSizeUnchanged() {
	s.store.EXPECT().GetSandboxBySession(gomock.Any(), "ses_1").Return(&types.Sandbox{
		ID:        "sbx_1",
		SessionID: "ses_1",
		Status:    types.SandboxStatusRunning,
		VCPUs:     4,
		MemoryMB:  8192,
	}, nil)

	s.Require().NoError(s.controller.ResizeSession(s.ctx, "ses_1", 4, 8192))
}

// A session-backed container is registered with hydra under the SESSION id, so
// deleting the row must hand teardown back to the executor rather than asking
// hydra to delete a container named after the row.
func (s *ControllerSessionSuite) TestDeleteRoutesSessionBackedTeardownToDesktopStopper() {
	stopped := ""
	s.controller.SetDesktopStopper(func(_ context.Context, sessionID string) error {
		stopped = sessionID
		return nil
	})

	s.store.EXPECT().GetSandbox(gomock.Any(), "sbx_1").Return(&types.Sandbox{
		ID:             "sbx_1",
		OrganizationID: "org_1",
		SessionID:      "ses_1",
		HostDeviceID:   "host_1",
		Runtime:        types.SandboxRuntimeUbuntuDesktop,
		Status:         types.SandboxStatusRunning,
	}, nil)
	s.store.EXPECT().GetSystemSettings(gomock.Any()).Return(&types.SystemSettings{}, nil)
	s.store.EXPECT().SetSandboxStatus(gomock.Any(), "sbx_1", types.SandboxStatusStopping, "").Return(nil)
	s.store.EXPECT().GetAPIKey(gomock.Any(), gomock.Any()).Return(nil, store.ErrNotFound)
	s.store.EXPECT().DeleteSandbox(gomock.Any(), "sbx_1").Return(nil)

	s.Require().NoError(s.controller.Delete(s.ctx, "sbx_1"))
	s.Require().Equal("ses_1", stopped)
}

func (s *ControllerSessionSuite) TestDeleteFailsLoudlyWhenNoDesktopStopperIsWired() {
	s.store.EXPECT().GetSandbox(gomock.Any(), "sbx_1").Return(&types.Sandbox{
		ID:             "sbx_1",
		OrganizationID: "org_1",
		SessionID:      "ses_1",
		Runtime:        types.SandboxRuntimeUbuntuDesktop,
		Status:         types.SandboxStatusRunning,
	}, nil)
	s.store.EXPECT().GetSystemSettings(gomock.Any()).Return(&types.SystemSettings{}, nil)
	s.store.EXPECT().SetSandboxStatus(gomock.Any(), "sbx_1", types.SandboxStatusStopping, "").Return(nil)

	err := s.controller.Delete(s.ctx, "sbx_1")
	s.Require().Error(err)
	s.Require().Contains(err.Error(), "no desktop stopper wired")
}

func (s *ControllerSessionSuite) TestHydraOpsIDPrefersSessionForSessionBackedRows() {
	sessionBacked := &types.Sandbox{ID: "sbx_1", SessionID: "ses_1"}
	controllerManaged := &types.Sandbox{ID: "sbx_2"}

	s.Require().Equal("ses_1", sessionBacked.HydraOpsID())
	s.Require().Equal("sbx_2", controllerManaged.HydraOpsID())
}
