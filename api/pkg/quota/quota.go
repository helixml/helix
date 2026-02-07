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

	if wallet.StripeSubscriptionID != "" && wallet.SubscriptionStatus == stripe.SubscriptionStatusActive {
		// Paid plan limits
		quotas = m.getProQuotas()
	} else {
		// Free plan limits
		quotas = m.getFreeQuotas()
	}

	quotas.ActiveConcurrentDesktops = m.getActiveConcurrentDesktopsByOrg(ctx, wallet.OrgID)

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

	if wallet.StripeSubscriptionID != "" && wallet.SubscriptionStatus == stripe.SubscriptionStatusActive {
		// Paid plan limits
		quotas = m.getProQuotas()
	} else {
		// Free plan limits
		quotas = m.getFreeQuotas()
	}

	quotas.UserID = wallet.UserID
	quotas.OrganizationID = wallet.OrgID

	quotas.ActiveConcurrentDesktops = m.getActiveConcurrentDesktopsByOrg(ctx, wallet.OrgID)

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
