package controller

import (
	"context"
	"errors"

	"github.com/lukemarsden/helix/api/pkg/store"
	"github.com/lukemarsden/helix/api/pkg/types"
)

func (c *Controller) GetStatus(ctx types.RequestContext) (types.UserStatus, error) {
	balanceTransfers, err := c.Options.Store.GetBalanceTransfers(ctx.Ctx, store.OwnerQuery{
		Owner:     ctx.Owner,
		OwnerType: ctx.OwnerType,
	})
	if err != nil {
		return types.UserStatus{}, err
	}

	// add up the total value of all balance transfers
	credits := 0
	for _, balanceTransfer := range balanceTransfers {
		credits += balanceTransfer.Amount
	}
	return types.UserStatus{
		User:    ctx.Owner,
		Credits: credits,
	}, nil
}

func (c *Controller) GetTransactions(ctx types.RequestContext) ([]*types.BalanceTransfer, error) {
	return c.Options.Store.GetBalanceTransfers(ctx.Ctx, store.OwnerQuery{
		Owner:     ctx.Owner,
		OwnerType: ctx.OwnerType,
	})
}
func (c *Controller) CreateAPIKey(ctx types.RequestContext, name string) (string, error) {
	apiKey, err := c.Options.Store.CreateAPIKey(ctx.Ctx, store.OwnerQuery{
		Owner:     ctx.Owner,
		OwnerType: ctx.OwnerType,
	}, name)
	if err != nil {
		return "", err
	}
	return apiKey, nil
}

func (c *Controller) GetAPIKeys(ctx types.RequestContext) ([]*types.ApiKey, error) {
	apiKeys, err := c.Options.Store.GetAPIKeys(ctx.Ctx, store.OwnerQuery{
		Owner:     ctx.Owner,
		OwnerType: ctx.OwnerType,
	})
	if err != nil {
		return nil, err
	}
	if apiKeys == nil {
		_, err := c.CreateAPIKey(ctx, "default")
		if err != nil {
			return nil, err
		}
		return c.GetAPIKeys(ctx)
	}
	return apiKeys, nil
}

func (c *Controller) DeleteAPIKey(ctx types.RequestContext, apiKey string) error {
	fetchedApiKey, err := c.Options.Store.CheckAPIKey(ctx.Ctx, apiKey)
	if err != nil {
		return err
	}
	if fetchedApiKey == nil {
		return errors.New("no such key")
	}
	// only the owner of an api key can delete it
	if fetchedApiKey.Owner != ctx.Owner || fetchedApiKey.OwnerType != ctx.OwnerType {
		return errors.New("unauthorized")
	}
	err = c.Options.Store.DeleteAPIKey(ctx.Ctx, *fetchedApiKey)
	if err != nil {
		return err
	}
	return nil
}

func (c *Controller) CheckAPIKey(ctx context.Context, apiKey string) (*types.ApiKey, error) {
	key, err := c.Options.Store.CheckAPIKey(ctx, apiKey)
	if err != nil {
		return nil, err
	}
	return key, nil
}
