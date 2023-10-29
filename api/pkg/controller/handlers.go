package controller

import (
	"errors"

	"github.com/bacalhau-project/lilypad/pkg/data"
	jobutils "github.com/bacalhau-project/lilysaas/api/pkg/job"
	"github.com/bacalhau-project/lilysaas/api/pkg/store"
	"github.com/bacalhau-project/lilysaas/api/pkg/system"
	"github.com/bacalhau-project/lilysaas/api/pkg/types"
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

func (c *Controller) GetJobs(ctx types.RequestContext) ([]*types.Job, error) {
	return c.Options.Store.GetJobs(ctx.Ctx, store.GetJobsQuery{
		Owner:     ctx.Owner,
		OwnerType: ctx.OwnerType,
	})
}

func (c *Controller) GetTransactions(ctx types.RequestContext) ([]*types.BalanceTransfer, error) {
	return c.Options.Store.GetBalanceTransfers(ctx.Ctx, store.OwnerQuery{
		Owner:     ctx.Owner,
		OwnerType: ctx.OwnerType,
	})
}

func (c *Controller) CreateJob(ctx types.RequestContext, request types.JobSpec) (data.JobOfferContainer, error) {
	container, err := c.Options.JobRunner.RunJob(ctx.Ctx, request)
	if err != nil {
		return container, err
	}
	module, err := jobutils.GetModule(request.Module)
	if err != nil {
		return container, err
	}
	err = c.Options.Store.CreateBalanceTransfer(ctx.Ctx, types.BalanceTransfer{
		ID:          system.GenerateUUID(),
		Owner:       ctx.Owner,
		OwnerType:   ctx.OwnerType,
		PaymentType: types.PaymentTypeJob,
		Amount:      -module.Cost,
		Data: types.BalanceTransferData{
			JobID: container.ID,
		},
	})
	if err != nil {
		return container, err
	}
	err = c.Options.Store.CreateJob(ctx.Ctx, types.Job{
		ID:        container.ID,
		Owner:     ctx.Owner,
		OwnerType: ctx.OwnerType,
		State:     data.GetAgreementStateString(container.State),
		Status:    "",
		Data: types.JobData{
			Spec: types.JobSpec{
				Module: request.Module,
				Inputs: request.Inputs,
			},
			Container: container,
		},
	})
	if err != nil {
		return container, err
	}
	return container, err
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
	return apiKeys, nil
}

func (c *Controller) DeleteAPIKey(ctx types.RequestContext, apiKey types.ApiKey) error {
	fetchedApiKey, err := c.Options.Store.CheckAPIKey(ctx.Ctx, apiKey.Key)
	if err != nil {
		return err
	}
	if fetchedApiKey.Owner != ctx.Owner || fetchedApiKey.OwnerType != ctx.OwnerType {
		return errors.New("unauthorized")
	}
	err = c.Options.Store.DeleteAPIKey(ctx.Ctx, apiKey)
	if err != nil {
		return err
	}
	return nil
}

func (c *Controller) CheckAPIKey(ctx types.RequestContext, apiKey string) (*store.OwnerQuery, error) {
	ownerQuery, err := c.Options.Store.CheckAPIKey(ctx.Ctx, apiKey)
	if err != nil {
		return nil, err
	}
	return ownerQuery, nil
}
