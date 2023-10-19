package controller

import (
	"github.com/lukemarsden/helix/api/pkg/store"
	"github.com/lukemarsden/helix/api/pkg/types"
)

func (c *Controller) GetStatus(ctx types.RequestContext) (types.UserStatus, error) {
	balanceTransfers, err := c.Options.Store.GetBalanceTransfers(ctx.Ctx, store.GetBalanceTransfersQuery{
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
	return c.Options.Store.GetBalanceTransfers(ctx.Ctx, store.GetBalanceTransfersQuery{
		Owner:     ctx.Owner,
		OwnerType: ctx.OwnerType,
	})
}
