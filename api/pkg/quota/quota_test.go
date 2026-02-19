package quota

import (
	"context"
	"fmt"
	"testing"

	"github.com/helixml/helix/api/pkg/config"
	external_agent "github.com/helixml/helix/api/pkg/external-agent"
	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/stretchr/testify/suite"
	"github.com/stripe/stripe-go/v76"
	"go.uber.org/mock/gomock"
)

type QuotaManagerSuite struct {
	suite.Suite

	ctrl     *gomock.Controller
	store    *store.MockStore
	executor *external_agent.MockExecutor
	manager  *DefaultQuotaManager
	cfg      *config.ServerConfig
}

func TestQuotaManagerSuite(t *testing.T) {
	suite.Run(t, new(QuotaManagerSuite))
}

func (s *QuotaManagerSuite) SetupTest() {
	s.ctrl = gomock.NewController(s.T())
	s.store = store.NewMockStore(s.ctrl)
	s.executor = external_agent.NewMockExecutor(s.ctrl)

	s.cfg = &config.ServerConfig{}
	s.cfg.SubscriptionQuotas.Projects.Enabled = true
	s.cfg.SubscriptionQuotas.Projects.Free.MaxConcurrentDesktops = 2
	s.cfg.SubscriptionQuotas.Projects.Free.MaxProjects = 3
	s.cfg.SubscriptionQuotas.Projects.Free.MaxRepositories = 3
	s.cfg.SubscriptionQuotas.Projects.Free.MaxSpecTasks = 500
	s.cfg.SubscriptionQuotas.Projects.Pro.MaxConcurrentDesktops = 5
	s.cfg.SubscriptionQuotas.Projects.Pro.MaxProjects = 20
	s.cfg.SubscriptionQuotas.Projects.Pro.MaxRepositories = 20
	s.cfg.SubscriptionQuotas.Projects.Pro.MaxSpecTasks = 10000

	s.manager = NewDefaultQuotaManager(s.store, s.cfg, s.executor)
}

// expectUserQuotaDefaults sets up the common mock expectations for user quota lookups
// with zero resource counts and no active sessions.
func (s *QuotaManagerSuite) expectUserQuotaDefaults(userID string, wallet *types.Wallet, settings *types.SystemSettings) {
	s.store.EXPECT().GetWalletByUser(gomock.Any(), userID).Return(wallet, nil)
	s.store.EXPECT().GetSystemSettings(gomock.Any()).Return(settings, nil)
	s.executor.EXPECT().ListSessions().Return(nil)
	s.store.EXPECT().GetProjectsCount(gomock.Any(), gomock.Any()).Return(int64(0), nil)
	s.store.EXPECT().GetRepositoriesCount(gomock.Any(), gomock.Any()).Return(int64(0), nil)
	s.store.EXPECT().GetSpecTasksCount(gomock.Any(), gomock.Any()).Return(int64(0), nil)
}

// expectOrgQuotaDefaults sets up the common mock expectations for org quota lookups
// with zero resource counts and no active sessions.
func (s *QuotaManagerSuite) expectOrgQuotaDefaults(orgID string, wallet *types.Wallet, settings *types.SystemSettings) {
	s.store.EXPECT().GetWalletByOrg(gomock.Any(), orgID).Return(wallet, nil)
	s.store.EXPECT().GetSystemSettings(gomock.Any()).Return(settings, nil)
	s.executor.EXPECT().ListSessions().Return(nil)
	s.store.EXPECT().GetProjectsCount(gomock.Any(), gomock.Any()).Return(int64(0), nil)
	s.store.EXPECT().GetRepositoriesCount(gomock.Any(), gomock.Any()).Return(int64(0), nil)
	s.store.EXPECT().GetSpecTasksCount(gomock.Any(), gomock.Any()).Return(int64(0), nil)
}

func freeWallet(userID string) *types.Wallet {
	return &types.Wallet{UserID: userID}
}

func proWallet(userID string) *types.Wallet {
	return &types.Wallet{
		UserID:               userID,
		StripeSubscriptionID: "sub_123",
		SubscriptionStatus:   stripe.SubscriptionStatusActive,
	}
}

func orgFreeWallet(orgID string) *types.Wallet {
	return &types.Wallet{OrgID: orgID}
}

func orgProWallet(orgID string) *types.Wallet {
	return &types.Wallet{
		OrgID:                orgID,
		StripeSubscriptionID: "sub_456",
		SubscriptionStatus:   stripe.SubscriptionStatusActive,
	}
}

func enforceQuotasSettings() *types.SystemSettings {
	return &types.SystemSettings{EnforceQuotas: true}
}

func disabledQuotasSettings() *types.SystemSettings {
	return &types.SystemSettings{EnforceQuotas: false}
}

// =============================================================================
// GetQuotas tests
// =============================================================================

func (s *QuotaManagerSuite) TestGetQuotas_UserFreeQuotas() {
	s.expectUserQuotaDefaults("user1", freeWallet("user1"), enforceQuotasSettings())

	resp, err := s.manager.GetQuotas(context.Background(), &types.QuotaRequest{UserID: "user1"})
	s.NoError(err)

	s.Equal("user1", resp.UserID)
	s.Equal(2, resp.MaxConcurrentDesktops)
	s.Equal(3, resp.MaxProjects)
	s.Equal(3, resp.MaxRepositories)
	s.Equal(500, resp.MaxSpecTasks)
	s.Equal(0, resp.ActiveConcurrentDesktops)
	s.Equal(0, resp.Projects)
	s.Equal(0, resp.Repositories)
	s.Equal(0, resp.SpecTasks)
}

func (s *QuotaManagerSuite) TestGetQuotas_UserProQuotas() {
	s.expectUserQuotaDefaults("user1", proWallet("user1"), enforceQuotasSettings())

	resp, err := s.manager.GetQuotas(context.Background(), &types.QuotaRequest{UserID: "user1"})
	s.NoError(err)

	s.Equal(5, resp.MaxConcurrentDesktops)
	s.Equal(20, resp.MaxProjects)
	s.Equal(20, resp.MaxRepositories)
	s.Equal(10000, resp.MaxSpecTasks)
}

func (s *QuotaManagerSuite) TestGetQuotas_UserQuotasDisabled() {
	s.expectUserQuotaDefaults("user1", freeWallet("user1"), disabledQuotasSettings())

	resp, err := s.manager.GetQuotas(context.Background(), &types.QuotaRequest{UserID: "user1"})
	s.NoError(err)

	s.Equal(-1, resp.MaxConcurrentDesktops)
	s.Equal(-1, resp.MaxProjects)
	s.Equal(-1, resp.MaxRepositories)
	s.Equal(-1, resp.MaxSpecTasks)
}

func (s *QuotaManagerSuite) TestGetQuotas_UserWithActiveSessions() {
	wallet := freeWallet("user1")
	s.store.EXPECT().GetWalletByUser(gomock.Any(), "user1").Return(wallet, nil)
	s.store.EXPECT().GetSystemSettings(gomock.Any()).Return(enforceQuotasSettings(), nil)
	s.executor.EXPECT().ListSessions().Return([]*external_agent.ZedSession{
		{UserID: "user1", SessionID: "ses1"},
		{UserID: "user1", SessionID: "ses2"},
		{UserID: "other_user", SessionID: "ses3"},
	})
	s.store.EXPECT().GetProjectsCount(gomock.Any(), gomock.Any()).Return(int64(2), nil)
	s.store.EXPECT().GetRepositoriesCount(gomock.Any(), gomock.Any()).Return(int64(1), nil)
	s.store.EXPECT().GetSpecTasksCount(gomock.Any(), gomock.Any()).Return(int64(42), nil)

	resp, err := s.manager.GetQuotas(context.Background(), &types.QuotaRequest{UserID: "user1"})
	s.NoError(err)

	s.Equal(2, resp.ActiveConcurrentDesktops)
	s.Equal(2, resp.Projects)
	s.Equal(1, resp.Repositories)
	s.Equal(42, resp.SpecTasks)
}

func (s *QuotaManagerSuite) TestGetQuotas_InactiveSubscription() {
	wallet := &types.Wallet{
		UserID:               "user1",
		StripeSubscriptionID: "sub_123",
		SubscriptionStatus:   stripe.SubscriptionStatusCanceled,
	}
	s.expectUserQuotaDefaults("user1", wallet, enforceQuotasSettings())

	resp, err := s.manager.GetQuotas(context.Background(), &types.QuotaRequest{UserID: "user1"})
	s.NoError(err)

	// Cancelled subscription should get free tier limits
	s.Equal(2, resp.MaxConcurrentDesktops)
	s.Equal(3, resp.MaxProjects)
}

func (s *QuotaManagerSuite) TestGetQuotas_OrgFreeQuotas() {
	s.expectOrgQuotaDefaults("org1", orgFreeWallet("org1"), enforceQuotasSettings())

	resp, err := s.manager.GetQuotas(context.Background(), &types.QuotaRequest{
		UserID:         "user1",
		OrganizationID: "org1",
	})
	s.NoError(err)

	s.Equal("org1", resp.OrganizationID)
	s.Equal(2, resp.MaxConcurrentDesktops)
	s.Equal(3, resp.MaxProjects)
	s.Equal(3, resp.MaxRepositories)
	s.Equal(500, resp.MaxSpecTasks)
}

func (s *QuotaManagerSuite) TestGetQuotas_OrgProQuotas() {
	s.expectOrgQuotaDefaults("org1", orgProWallet("org1"), enforceQuotasSettings())

	resp, err := s.manager.GetQuotas(context.Background(), &types.QuotaRequest{
		UserID:         "user1",
		OrganizationID: "org1",
	})
	s.NoError(err)

	s.Equal(5, resp.MaxConcurrentDesktops)
	s.Equal(20, resp.MaxProjects)
	s.Equal(20, resp.MaxRepositories)
	s.Equal(10000, resp.MaxSpecTasks)
}

func (s *QuotaManagerSuite) TestGetQuotas_OrgQuotasDisabled() {
	s.expectOrgQuotaDefaults("org1", orgFreeWallet("org1"), disabledQuotasSettings())

	resp, err := s.manager.GetQuotas(context.Background(), &types.QuotaRequest{
		UserID:         "user1",
		OrganizationID: "org1",
	})
	s.NoError(err)

	s.Equal(-1, resp.MaxConcurrentDesktops)
	s.Equal(-1, resp.MaxProjects)
	s.Equal(-1, resp.MaxRepositories)
	s.Equal(-1, resp.MaxSpecTasks)
}

func (s *QuotaManagerSuite) TestGetQuotas_OrgWithActiveSessions() {
	wallet := orgFreeWallet("org1")
	s.store.EXPECT().GetWalletByOrg(gomock.Any(), "org1").Return(wallet, nil)
	s.store.EXPECT().GetSystemSettings(gomock.Any()).Return(enforceQuotasSettings(), nil)
	s.executor.EXPECT().ListSessions().Return([]*external_agent.ZedSession{
		{OrganizationID: "org1", SessionID: "ses1"},
		{OrganizationID: "org1", SessionID: "ses2"},
		{OrganizationID: "org1", SessionID: "ses3"},
		{OrganizationID: "other_org", SessionID: "ses4"},
	})
	s.store.EXPECT().GetProjectsCount(gomock.Any(), gomock.Any()).Return(int64(5), nil)
	s.store.EXPECT().GetRepositoriesCount(gomock.Any(), gomock.Any()).Return(int64(3), nil)
	s.store.EXPECT().GetSpecTasksCount(gomock.Any(), gomock.Any()).Return(int64(100), nil)

	resp, err := s.manager.GetQuotas(context.Background(), &types.QuotaRequest{
		UserID:         "user1",
		OrganizationID: "org1",
	})
	s.NoError(err)

	s.Equal(3, resp.ActiveConcurrentDesktops)
	s.Equal(5, resp.Projects)
	s.Equal(3, resp.Repositories)
	s.Equal(100, resp.SpecTasks)
}

// =============================================================================
// GetQuotas error handling
// =============================================================================

func (s *QuotaManagerSuite) TestGetQuotas_WalletError() {
	s.store.EXPECT().GetWalletByUser(gomock.Any(), "user1").Return(nil, fmt.Errorf("db error"))

	_, err := s.manager.GetQuotas(context.Background(), &types.QuotaRequest{UserID: "user1"})
	s.Error(err)
	s.Contains(err.Error(), "db error")
}

func (s *QuotaManagerSuite) TestGetQuotas_SystemSettingsError() {
	s.store.EXPECT().GetWalletByUser(gomock.Any(), "user1").Return(freeWallet("user1"), nil)
	s.store.EXPECT().GetSystemSettings(gomock.Any()).Return(nil, fmt.Errorf("settings error"))

	_, err := s.manager.GetQuotas(context.Background(), &types.QuotaRequest{UserID: "user1"})
	s.Error(err)
	s.Contains(err.Error(), "settings error")
}

func (s *QuotaManagerSuite) TestGetQuotas_ProjectsCountError() {
	wallet := freeWallet("user1")
	s.store.EXPECT().GetWalletByUser(gomock.Any(), "user1").Return(wallet, nil)
	s.store.EXPECT().GetSystemSettings(gomock.Any()).Return(enforceQuotasSettings(), nil)
	s.executor.EXPECT().ListSessions().Return(nil)
	s.store.EXPECT().GetProjectsCount(gomock.Any(), gomock.Any()).Return(int64(0), fmt.Errorf("projects count error"))

	_, err := s.manager.GetQuotas(context.Background(), &types.QuotaRequest{UserID: "user1"})
	s.Error(err)
	s.Contains(err.Error(), "projects count error")
}

func (s *QuotaManagerSuite) TestGetQuotas_RepositoriesCountError() {
	wallet := freeWallet("user1")
	s.store.EXPECT().GetWalletByUser(gomock.Any(), "user1").Return(wallet, nil)
	s.store.EXPECT().GetSystemSettings(gomock.Any()).Return(enforceQuotasSettings(), nil)
	s.executor.EXPECT().ListSessions().Return(nil)
	s.store.EXPECT().GetProjectsCount(gomock.Any(), gomock.Any()).Return(int64(0), nil)
	s.store.EXPECT().GetRepositoriesCount(gomock.Any(), gomock.Any()).Return(int64(0), fmt.Errorf("repos count error"))

	_, err := s.manager.GetQuotas(context.Background(), &types.QuotaRequest{UserID: "user1"})
	s.Error(err)
	s.Contains(err.Error(), "repos count error")
}

func (s *QuotaManagerSuite) TestGetQuotas_SpecTasksCountError() {
	wallet := freeWallet("user1")
	s.store.EXPECT().GetWalletByUser(gomock.Any(), "user1").Return(wallet, nil)
	s.store.EXPECT().GetSystemSettings(gomock.Any()).Return(enforceQuotasSettings(), nil)
	s.executor.EXPECT().ListSessions().Return(nil)
	s.store.EXPECT().GetProjectsCount(gomock.Any(), gomock.Any()).Return(int64(0), nil)
	s.store.EXPECT().GetRepositoriesCount(gomock.Any(), gomock.Any()).Return(int64(0), nil)
	s.store.EXPECT().GetSpecTasksCount(gomock.Any(), gomock.Any()).Return(int64(0), fmt.Errorf("spec tasks error"))

	_, err := s.manager.GetQuotas(context.Background(), &types.QuotaRequest{UserID: "user1"})
	s.Error(err)
	s.Contains(err.Error(), "spec tasks error")
}

func (s *QuotaManagerSuite) TestGetQuotas_OrgWalletError() {
	s.store.EXPECT().GetWalletByOrg(gomock.Any(), "org1").Return(nil, fmt.Errorf("org wallet error"))

	_, err := s.manager.GetQuotas(context.Background(), &types.QuotaRequest{
		UserID:         "user1",
		OrganizationID: "org1",
	})
	s.Error(err)
	s.Contains(err.Error(), "org wallet error")
}

// =============================================================================
// LimitReached tests
// =============================================================================

func (s *QuotaManagerSuite) TestLimitReached_DesktopNotReached() {
	wallet := freeWallet("user1")
	s.store.EXPECT().GetWalletByUser(gomock.Any(), "user1").Return(wallet, nil)
	s.store.EXPECT().GetSystemSettings(gomock.Any()).Return(enforceQuotasSettings(), nil)
	s.executor.EXPECT().ListSessions().Return([]*external_agent.ZedSession{
		{UserID: "user1", SessionID: "ses1"},
	})
	s.store.EXPECT().GetProjectsCount(gomock.Any(), gomock.Any()).Return(int64(0), nil)
	s.store.EXPECT().GetRepositoriesCount(gomock.Any(), gomock.Any()).Return(int64(0), nil)
	s.store.EXPECT().GetSpecTasksCount(gomock.Any(), gomock.Any()).Return(int64(0), nil)

	resp, err := s.manager.LimitReached(context.Background(), &types.QuotaLimitReachedRequest{
		UserID:   "user1",
		Resource: types.ResourceDesktop,
	})
	s.NoError(err)
	s.False(resp.LimitReached)
	s.Equal(2, resp.Limit)
}

func (s *QuotaManagerSuite) TestLimitReached_DesktopReached() {
	wallet := freeWallet("user1")
	s.store.EXPECT().GetWalletByUser(gomock.Any(), "user1").Return(wallet, nil)
	s.store.EXPECT().GetSystemSettings(gomock.Any()).Return(enforceQuotasSettings(), nil)
	s.executor.EXPECT().ListSessions().Return([]*external_agent.ZedSession{
		{UserID: "user1", SessionID: "ses1"},
		{UserID: "user1", SessionID: "ses2"},
	})
	s.store.EXPECT().GetProjectsCount(gomock.Any(), gomock.Any()).Return(int64(0), nil)
	s.store.EXPECT().GetRepositoriesCount(gomock.Any(), gomock.Any()).Return(int64(0), nil)
	s.store.EXPECT().GetSpecTasksCount(gomock.Any(), gomock.Any()).Return(int64(0), nil)

	resp, err := s.manager.LimitReached(context.Background(), &types.QuotaLimitReachedRequest{
		UserID:   "user1",
		Resource: types.ResourceDesktop,
	})
	s.NoError(err)
	s.True(resp.LimitReached)
	s.Equal(2, resp.Limit)
}

func (s *QuotaManagerSuite) TestLimitReached_ProjectNotReached() {
	wallet := freeWallet("user1")
	s.store.EXPECT().GetWalletByUser(gomock.Any(), "user1").Return(wallet, nil)
	s.store.EXPECT().GetSystemSettings(gomock.Any()).Return(enforceQuotasSettings(), nil)
	s.executor.EXPECT().ListSessions().Return(nil)
	s.store.EXPECT().GetProjectsCount(gomock.Any(), gomock.Any()).Return(int64(2), nil)
	s.store.EXPECT().GetRepositoriesCount(gomock.Any(), gomock.Any()).Return(int64(0), nil)
	s.store.EXPECT().GetSpecTasksCount(gomock.Any(), gomock.Any()).Return(int64(0), nil)

	resp, err := s.manager.LimitReached(context.Background(), &types.QuotaLimitReachedRequest{
		UserID:   "user1",
		Resource: types.ResourceProject,
	})
	s.NoError(err)
	s.False(resp.LimitReached)
	s.Equal(3, resp.Limit)
}

func (s *QuotaManagerSuite) TestLimitReached_ProjectReached() {
	wallet := freeWallet("user1")
	s.store.EXPECT().GetWalletByUser(gomock.Any(), "user1").Return(wallet, nil)
	s.store.EXPECT().GetSystemSettings(gomock.Any()).Return(enforceQuotasSettings(), nil)
	s.executor.EXPECT().ListSessions().Return(nil)
	s.store.EXPECT().GetProjectsCount(gomock.Any(), gomock.Any()).Return(int64(3), nil)
	s.store.EXPECT().GetRepositoriesCount(gomock.Any(), gomock.Any()).Return(int64(0), nil)
	s.store.EXPECT().GetSpecTasksCount(gomock.Any(), gomock.Any()).Return(int64(0), nil)

	resp, err := s.manager.LimitReached(context.Background(), &types.QuotaLimitReachedRequest{
		UserID:   "user1",
		Resource: types.ResourceProject,
	})
	s.NoError(err)
	s.True(resp.LimitReached)
	s.Equal(3, resp.Limit)
}

func (s *QuotaManagerSuite) TestLimitReached_RepositoryNotReached() {
	wallet := freeWallet("user1")
	s.store.EXPECT().GetWalletByUser(gomock.Any(), "user1").Return(wallet, nil)
	s.store.EXPECT().GetSystemSettings(gomock.Any()).Return(enforceQuotasSettings(), nil)
	s.executor.EXPECT().ListSessions().Return(nil)
	s.store.EXPECT().GetProjectsCount(gomock.Any(), gomock.Any()).Return(int64(0), nil)
	s.store.EXPECT().GetRepositoriesCount(gomock.Any(), gomock.Any()).Return(int64(1), nil)
	s.store.EXPECT().GetSpecTasksCount(gomock.Any(), gomock.Any()).Return(int64(0), nil)

	resp, err := s.manager.LimitReached(context.Background(), &types.QuotaLimitReachedRequest{
		UserID:   "user1",
		Resource: types.ResourceGitRepository,
	})
	s.NoError(err)
	s.False(resp.LimitReached)
	s.Equal(3, resp.Limit)
}

func (s *QuotaManagerSuite) TestLimitReached_RepositoryReached() {
	wallet := freeWallet("user1")
	s.store.EXPECT().GetWalletByUser(gomock.Any(), "user1").Return(wallet, nil)
	s.store.EXPECT().GetSystemSettings(gomock.Any()).Return(enforceQuotasSettings(), nil)
	s.executor.EXPECT().ListSessions().Return(nil)
	s.store.EXPECT().GetProjectsCount(gomock.Any(), gomock.Any()).Return(int64(0), nil)
	s.store.EXPECT().GetRepositoriesCount(gomock.Any(), gomock.Any()).Return(int64(5), nil)
	s.store.EXPECT().GetSpecTasksCount(gomock.Any(), gomock.Any()).Return(int64(0), nil)

	resp, err := s.manager.LimitReached(context.Background(), &types.QuotaLimitReachedRequest{
		UserID:   "user1",
		Resource: types.ResourceGitRepository,
	})
	s.NoError(err)
	s.True(resp.LimitReached)
	s.Equal(3, resp.Limit)
}

func (s *QuotaManagerSuite) TestLimitReached_SpecTaskNotReached() {
	wallet := freeWallet("user1")
	s.store.EXPECT().GetWalletByUser(gomock.Any(), "user1").Return(wallet, nil)
	s.store.EXPECT().GetSystemSettings(gomock.Any()).Return(enforceQuotasSettings(), nil)
	s.executor.EXPECT().ListSessions().Return(nil)
	s.store.EXPECT().GetProjectsCount(gomock.Any(), gomock.Any()).Return(int64(0), nil)
	s.store.EXPECT().GetRepositoriesCount(gomock.Any(), gomock.Any()).Return(int64(0), nil)
	s.store.EXPECT().GetSpecTasksCount(gomock.Any(), gomock.Any()).Return(int64(100), nil)

	resp, err := s.manager.LimitReached(context.Background(), &types.QuotaLimitReachedRequest{
		UserID:   "user1",
		Resource: types.ResourceSpecTask,
	})
	s.NoError(err)
	s.False(resp.LimitReached)
	s.Equal(500, resp.Limit)
}

func (s *QuotaManagerSuite) TestLimitReached_SpecTaskReached() {
	wallet := freeWallet("user1")
	s.store.EXPECT().GetWalletByUser(gomock.Any(), "user1").Return(wallet, nil)
	s.store.EXPECT().GetSystemSettings(gomock.Any()).Return(enforceQuotasSettings(), nil)
	s.executor.EXPECT().ListSessions().Return(nil)
	s.store.EXPECT().GetProjectsCount(gomock.Any(), gomock.Any()).Return(int64(0), nil)
	s.store.EXPECT().GetRepositoriesCount(gomock.Any(), gomock.Any()).Return(int64(0), nil)
	s.store.EXPECT().GetSpecTasksCount(gomock.Any(), gomock.Any()).Return(int64(500), nil)

	resp, err := s.manager.LimitReached(context.Background(), &types.QuotaLimitReachedRequest{
		UserID:   "user1",
		Resource: types.ResourceSpecTask,
	})
	s.NoError(err)
	s.True(resp.LimitReached)
	s.Equal(500, resp.Limit)
}

func (s *QuotaManagerSuite) TestLimitReached_QuotasDisabled() {
	s.expectUserQuotaDefaults("user1", freeWallet("user1"), disabledQuotasSettings())

	resp, err := s.manager.LimitReached(context.Background(), &types.QuotaLimitReachedRequest{
		UserID:   "user1",
		Resource: types.ResourceDesktop,
	})
	s.NoError(err)
	s.False(resp.LimitReached)
	s.Equal(-1, resp.Limit)
}

func (s *QuotaManagerSuite) TestLimitReached_ProUserHigherLimits() {
	wallet := proWallet("user1")
	s.store.EXPECT().GetWalletByUser(gomock.Any(), "user1").Return(wallet, nil)
	s.store.EXPECT().GetSystemSettings(gomock.Any()).Return(enforceQuotasSettings(), nil)
	s.executor.EXPECT().ListSessions().Return([]*external_agent.ZedSession{
		{UserID: "user1", SessionID: "ses1"},
		{UserID: "user1", SessionID: "ses2"},
		{UserID: "user1", SessionID: "ses3"},
	})
	s.store.EXPECT().GetProjectsCount(gomock.Any(), gomock.Any()).Return(int64(0), nil)
	s.store.EXPECT().GetRepositoriesCount(gomock.Any(), gomock.Any()).Return(int64(0), nil)
	s.store.EXPECT().GetSpecTasksCount(gomock.Any(), gomock.Any()).Return(int64(0), nil)

	resp, err := s.manager.LimitReached(context.Background(), &types.QuotaLimitReachedRequest{
		UserID:   "user1",
		Resource: types.ResourceDesktop,
	})
	s.NoError(err)
	// 3 active desktops, pro limit is 5 — not reached
	s.False(resp.LimitReached)
	s.Equal(5, resp.Limit)
}

func (s *QuotaManagerSuite) TestLimitReached_UnknownResource() {
	s.expectUserQuotaDefaults("user1", freeWallet("user1"), enforceQuotasSettings())

	resp, err := s.manager.LimitReached(context.Background(), &types.QuotaLimitReachedRequest{
		UserID:   "user1",
		Resource: types.ResourceUser, // Not a quota-tracked resource
	})
	s.NoError(err)
	s.False(resp.LimitReached)
	s.Equal(0, resp.Limit)
}

func (s *QuotaManagerSuite) TestLimitReached_OrgDesktopReached() {
	wallet := orgFreeWallet("org1")
	s.store.EXPECT().GetWalletByOrg(gomock.Any(), "org1").Return(wallet, nil)
	s.store.EXPECT().GetSystemSettings(gomock.Any()).Return(enforceQuotasSettings(), nil)
	s.executor.EXPECT().ListSessions().Return([]*external_agent.ZedSession{
		{OrganizationID: "org1", SessionID: "ses1"},
		{OrganizationID: "org1", SessionID: "ses2"},
	})
	s.store.EXPECT().GetProjectsCount(gomock.Any(), gomock.Any()).Return(int64(0), nil)
	s.store.EXPECT().GetRepositoriesCount(gomock.Any(), gomock.Any()).Return(int64(0), nil)
	s.store.EXPECT().GetSpecTasksCount(gomock.Any(), gomock.Any()).Return(int64(0), nil)

	resp, err := s.manager.LimitReached(context.Background(), &types.QuotaLimitReachedRequest{
		UserID:         "user1",
		OrganizationID: "org1",
		Resource:       types.ResourceDesktop,
	})
	s.NoError(err)
	s.True(resp.LimitReached)
	s.Equal(2, resp.Limit)
}

func (s *QuotaManagerSuite) TestLimitReached_ErrorPropagated() {
	s.store.EXPECT().GetWalletByUser(gomock.Any(), "user1").Return(nil, fmt.Errorf("db down"))

	_, err := s.manager.LimitReached(context.Background(), &types.QuotaLimitReachedRequest{
		UserID:   "user1",
		Resource: types.ResourceDesktop,
	})
	s.Error(err)
	s.Contains(err.Error(), "db down")
}

func (s *QuotaManagerSuite) TestLimitReached_ExactlyAtLimit() {
	wallet := proWallet("user1")
	s.store.EXPECT().GetWalletByUser(gomock.Any(), "user1").Return(wallet, nil)
	s.store.EXPECT().GetSystemSettings(gomock.Any()).Return(enforceQuotasSettings(), nil)
	s.executor.EXPECT().ListSessions().Return(nil)
	s.store.EXPECT().GetProjectsCount(gomock.Any(), gomock.Any()).Return(int64(20), nil)
	s.store.EXPECT().GetRepositoriesCount(gomock.Any(), gomock.Any()).Return(int64(0), nil)
	s.store.EXPECT().GetSpecTasksCount(gomock.Any(), gomock.Any()).Return(int64(0), nil)

	resp, err := s.manager.LimitReached(context.Background(), &types.QuotaLimitReachedRequest{
		UserID:   "user1",
		Resource: types.ResourceProject,
	})
	s.NoError(err)
	// Exactly at limit (20 == 20) should be considered reached
	s.True(resp.LimitReached)
	s.Equal(20, resp.Limit)
}

func (s *QuotaManagerSuite) TestLimitReached_OverLimit() {
	wallet := freeWallet("user1")
	s.store.EXPECT().GetWalletByUser(gomock.Any(), "user1").Return(wallet, nil)
	s.store.EXPECT().GetSystemSettings(gomock.Any()).Return(enforceQuotasSettings(), nil)
	s.executor.EXPECT().ListSessions().Return(nil)
	s.store.EXPECT().GetProjectsCount(gomock.Any(), gomock.Any()).Return(int64(0), nil)
	s.store.EXPECT().GetRepositoriesCount(gomock.Any(), gomock.Any()).Return(int64(10), nil)
	s.store.EXPECT().GetSpecTasksCount(gomock.Any(), gomock.Any()).Return(int64(0), nil)

	resp, err := s.manager.LimitReached(context.Background(), &types.QuotaLimitReachedRequest{
		UserID:   "user1",
		Resource: types.ResourceGitRepository,
	})
	s.NoError(err)
	// Over limit (10 > 3) should be considered reached
	s.True(resp.LimitReached)
	s.Equal(3, resp.Limit)
}

// =============================================================================
// Subscription status edge cases
// =============================================================================

func (s *QuotaManagerSuite) TestGetQuotas_PastDueSubscription() {
	wallet := &types.Wallet{
		UserID:               "user1",
		StripeSubscriptionID: "sub_123",
		SubscriptionStatus:   stripe.SubscriptionStatusPastDue,
	}
	s.expectUserQuotaDefaults("user1", wallet, enforceQuotasSettings())

	resp, err := s.manager.GetQuotas(context.Background(), &types.QuotaRequest{UserID: "user1"})
	s.NoError(err)

	// Past-due subscription should fall through to free tier
	s.Equal(2, resp.MaxConcurrentDesktops)
	s.Equal(3, resp.MaxProjects)
}

func (s *QuotaManagerSuite) TestGetQuotas_NoSubscriptionID() {
	wallet := &types.Wallet{
		UserID:             "user1",
		SubscriptionStatus: stripe.SubscriptionStatusActive, // Active status but no subscription ID
	}
	s.expectUserQuotaDefaults("user1", wallet, enforceQuotasSettings())

	resp, err := s.manager.GetQuotas(context.Background(), &types.QuotaRequest{UserID: "user1"})
	s.NoError(err)

	// No subscription ID means free tier even with active status
	s.Equal(2, resp.MaxConcurrentDesktops)
}

// =============================================================================
// Routing tests (user vs org)
// =============================================================================

func (s *QuotaManagerSuite) TestGetQuotas_RoutesToOrgWhenOrgIDSet() {
	wallet := orgFreeWallet("org1")
	// Should call GetWalletByOrg, NOT GetWalletByUser
	s.store.EXPECT().GetWalletByOrg(gomock.Any(), "org1").Return(wallet, nil)
	s.store.EXPECT().GetSystemSettings(gomock.Any()).Return(enforceQuotasSettings(), nil)
	s.executor.EXPECT().ListSessions().Return(nil)
	s.store.EXPECT().GetProjectsCount(gomock.Any(), gomock.Any()).Return(int64(0), nil)
	s.store.EXPECT().GetRepositoriesCount(gomock.Any(), gomock.Any()).Return(int64(0), nil)
	s.store.EXPECT().GetSpecTasksCount(gomock.Any(), gomock.Any()).Return(int64(0), nil)

	_, err := s.manager.GetQuotas(context.Background(), &types.QuotaRequest{
		UserID:         "user1",
		OrganizationID: "org1",
	})
	s.NoError(err)
}

func (s *QuotaManagerSuite) TestGetQuotas_RoutesToUserWhenNoOrgID() {
	wallet := freeWallet("user1")
	// Should call GetWalletByUser, NOT GetWalletByOrg
	s.store.EXPECT().GetWalletByUser(gomock.Any(), "user1").Return(wallet, nil)
	s.store.EXPECT().GetSystemSettings(gomock.Any()).Return(enforceQuotasSettings(), nil)
	s.executor.EXPECT().ListSessions().Return(nil)
	s.store.EXPECT().GetProjectsCount(gomock.Any(), gomock.Any()).Return(int64(0), nil)
	s.store.EXPECT().GetRepositoriesCount(gomock.Any(), gomock.Any()).Return(int64(0), nil)
	s.store.EXPECT().GetSpecTasksCount(gomock.Any(), gomock.Any()).Return(int64(0), nil)

	_, err := s.manager.GetQuotas(context.Background(), &types.QuotaRequest{
		UserID: "user1",
	})
	s.NoError(err)
}
