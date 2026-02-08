package quota

import (
	"context"

	"github.com/helixml/helix/api/pkg/config"
	external_agent "github.com/helixml/helix/api/pkg/external-agent"
	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/stripe/stripe-go/v76"
)

type QuotaManager interface {
	GetQuotas(ctx context.Context, req *types.QuotaRequest) (*types.QuotaResponse, error)
	LimitReached(ctx context.Context, req *types.QuotaLimitReachedRequest) (*types.QuotaLimitReachedResponse, error)
}

type DefaultQuotaManager struct {
	store                 store.Store
	cfg                   *config.ServerConfig
	externalAgentExecutor external_agent.Executor
}

func NewDefaultQuotaManager(store store.Store, config *config.ServerConfig, externalAgentExecutor external_agent.Executor) *DefaultQuotaManager {
	return &DefaultQuotaManager{
		store:                 store,
		cfg:                   config,
		externalAgentExecutor: externalAgentExecutor,
	}
}

func (m *DefaultQuotaManager) GetQuotas(ctx context.Context, req *types.QuotaRequest) (*types.QuotaResponse, error) {
	if req.OrganizationID != "" {
		return m.getOrgQuotas(ctx, req.OrganizationID)
	}
	return m.getUserQuotas(ctx, req.UserID)
}

func (m *DefaultQuotaManager) getOrgQuotas(ctx context.Context, orgID string) (*types.QuotaResponse, error) {
	// Check if we have active subscription for this org
	wallet, err := m.store.GetWalletByOrg(ctx, orgID)
	if err != nil {
		return nil, err
	}

	var quotas *types.QuotaResponse

	// If quota enforcement is disabled, return -1 for all quotas
	systemSettings, err := m.store.GetSystemSettings(ctx)
	if err != nil {
		return nil, err
	}

	switch {
	// Quotas disabled
	case !systemSettings.EnforceQuotas:
		quotas = &types.QuotaResponse{
			MaxConcurrentDesktops: -1,
			MaxProjects:           -1,
			MaxRepositories:       -1,
			MaxSpecTasks:          -1,
		}
	// Active subscription
	case wallet.StripeSubscriptionID != "" && wallet.SubscriptionStatus == stripe.SubscriptionStatusActive:
		// Paid plan limits
		quotas = m.getProQuotas()
	default:
		// Free plan limits
		quotas = m.getFreeQuotas()
	}

	quotas.ActiveConcurrentDesktops = m.getActiveConcurrentDesktopsByOrg(ctx, wallet.OrgID)

	projectsCount, err := m.store.GetProjectsCount(ctx, &store.GetProjectsCountQuery{OrganizationID: wallet.OrgID})
	if err != nil {
		return nil, err
	}
	quotas.Projects = int(projectsCount)

	repositoriesCount, err := m.store.GetRepositoriesCount(ctx, &store.GetRepositoriesCountQuery{OrganizationID: wallet.OrgID})
	if err != nil {
		return nil, err
	}
	quotas.Repositories = int(repositoriesCount)

	specTasksCount, err := m.store.GetSpecTasksCount(ctx, &store.GetSpecTasksCountQuery{OrganizationID: wallet.OrgID})
	if err != nil {
		return nil, err
	}
	quotas.SpecTasks = int(specTasksCount)

	quotas.UserID = wallet.UserID
	quotas.OrganizationID = wallet.OrgID

	return quotas, nil
}

func (m *DefaultQuotaManager) getUserQuotas(ctx context.Context, userID string) (*types.QuotaResponse, error) {
	// Check if we have active subscription for this user
	wallet, err := m.store.GetWalletByUser(ctx, userID)
	if err != nil {
		return nil, err
	}

	var quotas *types.QuotaResponse

	// If quota enforcement is disabled, return -1 for all quotas
	systemSettings, err := m.store.GetSystemSettings(ctx)
	if err != nil {
		return nil, err
	}

	switch {
	// Quotas disabled
	case !systemSettings.EnforceQuotas:
		quotas = &types.QuotaResponse{
			MaxConcurrentDesktops: -1,
			MaxProjects:           -1,
			MaxRepositories:       -1,
			MaxSpecTasks:          -1,
		}
	case wallet.StripeSubscriptionID != "" && wallet.SubscriptionStatus == stripe.SubscriptionStatusActive:
		// Paid plan limits
		quotas = m.getProQuotas()
	default:
		// Free plan limits
		quotas = m.getFreeQuotas()
	}

	quotas.UserID = wallet.UserID

	quotas.ActiveConcurrentDesktops = m.getActiveConcurrentDesktopsByUser(ctx, wallet.UserID)

	projectsCount, err := m.store.GetProjectsCount(ctx, &store.GetProjectsCountQuery{UserID: wallet.UserID})
	if err != nil {
		return nil, err
	}
	quotas.Projects = int(projectsCount)

	repositoriesCount, err := m.store.GetRepositoriesCount(ctx, &store.GetRepositoriesCountQuery{UserID: wallet.UserID})
	if err != nil {
		return nil, err
	}

	quotas.Repositories = int(repositoriesCount)

	specTasksCount, err := m.store.GetSpecTasksCount(ctx, &store.GetSpecTasksCountQuery{UserID: wallet.UserID})
	if err != nil {
		return nil, err
	}
	quotas.SpecTasks = int(specTasksCount)

	return quotas, nil
}

func (m *DefaultQuotaManager) getFreeQuotas() *types.QuotaResponse {
	quotas := &types.QuotaResponse{
		MaxConcurrentDesktops: m.cfg.SubscriptionQuotas.Projects.Free.MaxConcurrentDesktops,
		MaxProjects:           m.cfg.SubscriptionQuotas.Projects.Free.MaxProjects,
		MaxRepositories:       m.cfg.SubscriptionQuotas.Projects.Free.MaxRepositories,
		MaxSpecTasks:          m.cfg.SubscriptionQuotas.Projects.Free.MaxSpecTasks,
	}
	return quotas
}

func (m *DefaultQuotaManager) getProQuotas() *types.QuotaResponse {
	quotas := &types.QuotaResponse{
		MaxConcurrentDesktops: m.cfg.SubscriptionQuotas.Projects.Pro.MaxConcurrentDesktops,
		MaxProjects:           m.cfg.SubscriptionQuotas.Projects.Pro.MaxProjects,
		MaxRepositories:       m.cfg.SubscriptionQuotas.Projects.Pro.MaxRepositories,
		MaxSpecTasks:          m.cfg.SubscriptionQuotas.Projects.Pro.MaxSpecTasks,
	}
	return quotas
}

func (m *DefaultQuotaManager) getActiveConcurrentDesktopsByUser(ctx context.Context, userID string) int {
	count := 0
	sessions := m.externalAgentExecutor.ListSessions()
	for _, session := range sessions {
		if session.UserID == userID {
			count++
		}
	}
	return count
}

func (m *DefaultQuotaManager) getActiveConcurrentDesktopsByOrg(ctx context.Context, orgID string) int {
	count := 0
	sessions := m.externalAgentExecutor.ListSessions()
	for _, session := range sessions {
		if session.OrganizationID == orgID {
			count++
		}
	}
	return count
}

func (m *DefaultQuotaManager) LimitReached(ctx context.Context, req *types.QuotaLimitReachedRequest) (*types.QuotaLimitReachedResponse, error) {
	return &types.QuotaLimitReachedResponse{LimitReached: false, Limit: 0}, nil
}
