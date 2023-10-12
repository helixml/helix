package controller

import (
	"context"
	"fmt"
	"time"

	"github.com/bacalhau-project/lilypad/pkg/data"
	jobutils "github.com/bacalhau-project/lilysaas/api/pkg/job"
	"github.com/bacalhau-project/lilysaas/api/pkg/store"
	"github.com/bacalhau-project/lilysaas/api/pkg/system"
	"github.com/bacalhau-project/lilysaas/api/pkg/types"
)

func (c *Controller) GetStatus(ctx types.RequestContext) (types.UserStatus, error) {
	balanceTransfers, err := c.Store.GetBalanceTransfers(ctx.Ctx, store.GetBalanceTransfersQuery{
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
	return c.Store.GetJobs(ctx.Ctx, store.GetJobsQuery{
		Owner:     ctx.Owner,
		OwnerType: ctx.OwnerType,
	})
}

func (c *Controller) GetTransactions(ctx types.RequestContext) ([]*types.BalanceTransfer, error) {
	return c.Store.GetBalanceTransfers(ctx.Ctx, store.GetBalanceTransfersQuery{
		Owner:     ctx.Owner,
		OwnerType: ctx.OwnerType,
	})
}

func (c *Controller) CreateJob(ctx types.RequestContext, request types.JobSpec) (data.JobOfferContainer, error) {
	container, err := c.JobRunner.RunJob(ctx.Ctx, request)
	if err != nil {
		return container, err
	}
	module, err := jobutils.GetModule(request.Module)
	if err != nil {
		return container, err
	}
	err = c.Store.CreateBalanceTransfer(ctx.Ctx, types.BalanceTransfer{
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
	err = c.Store.CreateJob(ctx.Ctx, types.Job{
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

func (c *Controller) handleJobUpdate(evOffer data.JobOfferContainer) {
	job, err := c.Store.GetJob(context.Background(), evOffer.ID)
	if err != nil {
		fmt.Printf("error loading job: %s --------------------------------------\n", err.Error())
		return
	}
	// we have a race condition where we need to write the job to the solver to get
	// it's ID and then we might not have written the job to the database yet
	// TODO: make lilypad have a way to have deterministic ID's so we can know the
	// job ID before submitting it
	if job == nil {
		// this means the job has not been written to the database yet (probably)
		time.Sleep(time.Millisecond * 100)
		job, err = c.Store.GetJob(context.Background(), evOffer.ID)
		if err != nil {
			return
		}
		if job == nil {
			fmt.Printf("job not found: %s --------------------------------------\n", evOffer.ID)
			return
		}
	}
	jobData := job.Data
	jobData.Container = evOffer

	c.Store.UpdateJob(
		c.Ctx,
		evOffer.ID,
		data.GetAgreementStateString(evOffer.State),
		"",
		jobData,
	)

	job, err = c.Store.GetJob(context.Background(), evOffer.ID)
	if err != nil {
		fmt.Printf("error loading job: %s --------------------------------------\n", err.Error())
		return
	}

	c.JobUpdatesChan <- job
}

// load all jobs that are currently running and check if they are still running
func (c *Controller) checkForRunningJobs(ctx context.Context) error {
	return nil
}
