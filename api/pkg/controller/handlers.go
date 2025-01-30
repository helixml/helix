package controller

import (
	"context"
	"errors"

	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/system"
	"github.com/helixml/helix/api/pkg/types"
)

func (c *Controller) GetStatus(ctx context.Context, user *types.User) (types.UserStatus, error) {
	usermeta, err := c.Options.Store.GetUserMeta(ctx, user.ID)

	if err != nil || usermeta == nil {
		usermeta = &types.UserMeta{
			ID:     user.ID,
			Config: types.UserConfig{},
		}
	}

	return types.UserStatus{
		Admin:  user.Admin,
		User:   user.ID,
		Config: usermeta.Config,
	}, nil
}

func (c *Controller) CreateAPIKey(ctx context.Context, user *types.User, apiKey *types.ApiKey) (*types.ApiKey, error) {
	key, err := system.GenerateAPIKey()
	if err != nil {
		return nil, err
	}

	apiKey.Key = key
	apiKey.Owner = user.ID
	apiKey.OwnerType = user.Type

	return c.Options.Store.CreateAPIKey(ctx, apiKey)
}

func (c *Controller) GetAPIKeys(ctx context.Context, user *types.User) ([]*types.ApiKey, error) {
	apiKeys, err := c.Options.Store.ListAPIKeys(ctx, &store.ListAPIKeysQuery{
		Owner:     user.ID,
		OwnerType: user.Type,
		// filter by APIKeyType_API when deciding whether to auto-create user
		// API keys
		Type: types.APIkeytypeAPI,
	})
	if err != nil {
		return nil, err
	}
	if len(apiKeys) == 0 {
		_, err := c.CreateAPIKey(ctx, user, &types.ApiKey{
			Name: "default",
			Type: types.APIkeytypeAPI,
		})
		if err != nil {
			return nil, err
		}
		return c.GetAPIKeys(ctx, user)
	}
	// return all api key types
	apiKeys, err = c.Options.Store.ListAPIKeys(ctx, &store.ListAPIKeysQuery{
		Owner:     user.ID,
		OwnerType: user.Type,
	})
	if err != nil {
		return nil, err
	}
	return apiKeys, nil
}

func (c *Controller) DeleteAPIKey(ctx context.Context, user *types.User, apiKey string) error {
	fetchedAPIKey, err := c.Options.Store.GetAPIKey(ctx, apiKey)
	if err != nil {
		return err
	}
	if fetchedAPIKey == nil {
		return errors.New("no such key")
	}
	// only the owner of an api key can delete it
	if fetchedAPIKey.Owner != user.ID || fetchedAPIKey.OwnerType != user.Type {
		return errors.New("unauthorized")
	}
	err = c.Options.Store.DeleteAPIKey(ctx, fetchedAPIKey.Key)
	if err != nil {
		return err
	}
	return nil
}

func (c *Controller) CheckAPIKey(ctx context.Context, apiKey string) (*types.ApiKey, error) {
	key, err := c.Options.Store.GetAPIKey(ctx, apiKey)
	if err != nil {
		return nil, err
	}
	return key, nil
}

func (c *Controller) GetDashboardData(_ context.Context) (*types.DashboardData, error) {
	runnerStatuses, err := c.scheduler.RunnerStatus()
	if err != nil {
		return nil, err
	}
	runners := make([]*types.DashboardRunner, 0, len(runnerStatuses))
	for _, runnerStatus := range runnerStatuses {
		runnerSlots, err := c.scheduler.RunnerSlots(runnerStatus.ID)
		if err != nil {
			return nil, err
		}
		runners = append(runners, &types.DashboardRunner{
			ID:          runnerStatus.ID,
			Created:     runnerStatus.Created,
			Updated:     runnerStatus.Updated,
			Version:     runnerStatus.Version,
			TotalMemory: runnerStatus.TotalMemory,
			FreeMemory:  runnerStatus.FreeMemory,
			Labels:      runnerStatus.Labels,
			Slots:       runnerSlots,
		})
	}
	queue, err := c.scheduler.Queue()
	if err != nil {
		return nil, err
	}
	return &types.DashboardData{
		Runners: runners,
		Queue:   queue,
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
