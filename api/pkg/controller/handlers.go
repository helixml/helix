package controller

import (
	"context"
	"errors"
	"time"

	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/system"
	"github.com/helixml/helix/api/pkg/types"
)

func (c *Controller) GetStatus(ctx types.RequestContext) (types.UserStatus, error) {
	usermeta, err := c.Options.Store.GetUserMeta(ctx.Ctx, ctx.User.ID)

	if err != nil || usermeta == nil {
		usermeta = &types.UserMeta{
			ID:     ctx.User.ID,
			Config: types.UserConfig{},
		}
	}

	return types.UserStatus{
		Admin:  ctx.User.Admin,
		User:   ctx.User.ID,
		Config: usermeta.Config,
	}, nil
}

func (c *Controller) CreateAPIKey(ctx types.RequestContext, apiKey *types.APIKey) (*types.APIKey, error) {
	key, err := system.GenerateAPIKey()
	if err != nil {
		return nil, err
	}

	apiKey.Key = key
	apiKey.Owner = ctx.User.ID
	apiKey.OwnerType = ctx.User.Type

	return c.Options.Store.CreateAPIKey(ctx.Ctx, apiKey)
}

func (c *Controller) GetAPIKeys(ctx types.RequestContext) ([]*types.APIKey, error) {
	apiKeys, err := c.Options.Store.ListAPIKeys(ctx.Ctx, &store.ListApiKeysQuery{
		Owner:     ctx.User.ID,
		OwnerType: ctx.User.Type,
		// filter by APIKeyType_API when deciding whether to auto-create user
		// API keys
		Type: types.APIKeyType_API,
	})
	if err != nil {
		return nil, err
	}
	if len(apiKeys) == 0 {
		_, err := c.CreateAPIKey(ctx, &types.APIKey{
			Name: "default",
			Type: types.APIKeyType_API,
		})
		if err != nil {
			return nil, err
		}
		return c.GetAPIKeys(ctx)
	}
	// return all api key types
	apiKeys, err = c.Options.Store.ListAPIKeys(ctx.Ctx, &store.ListApiKeysQuery{
		Owner:     ctx.User.ID,
		OwnerType: ctx.User.Type,
	})
	if err != nil {
		return nil, err
	}
	return apiKeys, nil
}

func (c *Controller) DeleteAPIKey(ctx types.RequestContext, apiKey string) error {
	fetchedApiKey, err := c.Options.Store.GetAPIKey(ctx.Ctx, apiKey)
	if err != nil {
		return err
	}
	if fetchedApiKey == nil {
		return errors.New("no such key")
	}
	// only the owner of an api key can delete it
	if fetchedApiKey.Owner != ctx.User.ID || fetchedApiKey.OwnerType != ctx.User.Type {
		return errors.New("unauthorized")
	}
	err = c.Options.Store.DeleteAPIKey(ctx.Ctx, fetchedApiKey.Key)
	if err != nil {
		return err
	}
	return nil
}

func (c *Controller) CheckAPIKey(ctx context.Context, apiKey string) (*types.APIKey, error) {
	key, err := c.Options.Store.GetAPIKey(ctx, apiKey)
	if err != nil {
		return nil, err
	}
	return key, nil
}

func (c *Controller) cleanOldRunnerMetrics(_ context.Context) error {
	deleteIDs := []string{}
	c.activeRunners.Range(func(i string, metrics *types.RunnerState) bool {
		// any runner that has not reported within the last minute
		// should be removed
		if time.Since(metrics.Created) > (time.Minute * 1) {
			deleteIDs = append(deleteIDs, i)
		}
		return true
	})

	// Perform the deletion logic using the deleteIDs slice
	for _, id := range deleteIDs {
		c.activeRunners.Delete(id)
	}

	return nil
}

func (c *Controller) AddRunnerMetrics(ctx context.Context, metrics *types.RunnerState) (*types.RunnerState, error) {
	c.activeRunners.Store(metrics.ID, metrics)
	return metrics, nil
}

func (c *Controller) GetDashboardData(ctx context.Context) (*types.DashboardData, error) {
	runners := []*types.RunnerState{}
	c.activeRunners.Range(func(i string, metrics *types.RunnerState) bool {
		runners = append(runners, metrics)
		return true
	})
	return &types.DashboardData{
		SessionQueue:              c.sessionSummaryQueue,
		Runners:                   runners,
		GlobalSchedulingDecisions: c.schedulingDecisions,
	}, nil
}

func (c *Controller) updateSubscriptionUser(userID string, stripeCustomerID string, stripeSubscriptionID string, active bool) error {
	existingUser, err := c.Options.Store.GetUserMeta(context.Background(), userID)
	if err != nil || existingUser != nil {
		existingUser = &types.UserMeta{
			ID: userID,
			Config: types.UserConfig{
				StripeCustomerID:     stripeCustomerID,
				StripeSubscriptionID: stripeSubscriptionID,
			},
		}
	}
	existingUser.Config.StripeSubscriptionActive = active
	_, err = c.Options.Store.EnsureUserMeta(context.Background(), *existingUser)
	return err
}

func (c *Controller) HandleSubscriptionEvent(eventType types.SubscriptionEventType, user types.StripeUser) error {
	isSubscriptionActive := true
	if eventType == types.SubscriptionEventTypeDeleted {
		isSubscriptionActive = false
	}
	err := c.updateSubscriptionUser(user.HelixID, user.StripeID, user.SubscriptionID, isSubscriptionActive)
	if err != nil {
		return err
	}
	return c.Options.Janitor.WriteSubscriptionEvent(eventType, user)
}
