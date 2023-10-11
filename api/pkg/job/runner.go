package job

import (
	"context"
	"time"

	"github.com/bacalhau-project/lilypad/pkg/data"
	"github.com/bacalhau-project/lilypad/pkg/jobcreator"
	lilypadsystem "github.com/bacalhau-project/lilypad/pkg/system"
	"github.com/bacalhau-project/lilypad/pkg/web3"
	"github.com/bacalhau-project/lilysaas/api/pkg/types"
	"github.com/spf13/cobra"
)

type JobRunner struct {
	Ctx        context.Context
	Options    jobcreator.JobCreatorOptions
	Web3SDK    *web3.Web3SDK
	JobCreator *jobcreator.JobCreator
	ErrorChan  chan error
}

func NewJobRunner(ctx context.Context) (*JobRunner, error) {
	// get options without a job to bootstrap the sdk and jobcreator
	options, err := GetJobOptions(types.JobSpec{
		Module: "",
		Inputs: map[string]string{},
	}, false)
	if err != nil {
		return nil, err
	}
	web3SDK, err := web3.NewContractSDK(options.Web3)
	if err != nil {
		return nil, err
	}
	jobCreatorService, err := jobcreator.NewJobCreator(options, web3SDK)
	if err != nil {
		return nil, err
	}
	tmpCommand := &cobra.Command{}
	tmpCommand.SetContext(ctx)
	cmdCtx := lilypadsystem.NewCommandContext(tmpCommand)

	jobCreatorErrors := jobCreatorService.Start(cmdCtx.Ctx, cmdCtx.Cm)

	// wait a short period because we've just started the job creator service
	time.Sleep(100 * time.Millisecond)
	return &JobRunner{
		Options:    options,
		Web3SDK:    web3SDK,
		JobCreator: jobCreatorService,
		ErrorChan:  jobCreatorErrors,
	}, nil
}

func (runner *JobRunner) Subscribe(ctx context.Context, callback jobcreator.JobOfferSubscriber) {
	runner.JobCreator.SubscribeToJobOfferUpdates(callback)
}

func (runner *JobRunner) GetJobOffer(ctx context.Context, request types.JobSpec) (data.JobOffer, error) {
	options, err := GetJobOptions(request, true)
	if err != nil {
		return data.JobOffer{}, err
	}
	return runner.JobCreator.GetJobOfferFromOptions(options.Offer)
}

func (runner *JobRunner) GetJobContainer(ctx context.Context, request types.JobSpec) (data.JobOfferContainer, error) {
	jobOffer, err := runner.GetJobOffer(ctx, request)
	if err != nil {
		return data.JobOfferContainer{}, err
	}
	id, err := data.GetJobOfferID(jobOffer)
	if err != nil {
		return data.JobOfferContainer{}, err
	}
	jobOffer.ID = id
	container := data.GetJobOfferContainer(jobOffer)
	return container, nil
}

func (runner *JobRunner) RunJob(ctx context.Context, request types.JobSpec) (data.JobOfferContainer, error) {
	jobOffer, err := runner.GetJobOffer(ctx, request)
	if err != nil {
		return data.JobOfferContainer{}, err
	}
	return runner.JobCreator.AddJobOffer(jobOffer)

	// result, err := jobCreatorService.GetResult(finalJobOffer.DealID)
	// if err != nil {
	// 	return nil, err
	// }
}
