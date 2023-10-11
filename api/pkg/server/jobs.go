package server

import (
	"context"
	"encoding/json"
	"io"
	"log"
	"net/http"

	"github.com/spf13/cobra"

	"github.com/bacalhau-project/lilypad/pkg/data"
	"github.com/bacalhau-project/lilypad/pkg/jobcreator"
	optionsfactory "github.com/bacalhau-project/lilypad/pkg/options"
	"github.com/bacalhau-project/lilypad/pkg/solver"
	"github.com/bacalhau-project/lilypad/pkg/system"
)

const JOB_COST = 3

type createJobResponse struct {
	// User *types.User `json:"user"`
}

type createJobRequest struct {
	Module string            `json:"module"`
	Inputs map[string]string `json:"inputs"`
}

func ProcessJobCreatorOptions(options jobcreator.JobCreatorOptions, request createJobRequest) (jobcreator.JobCreatorOptions, error) {
	options.Offer.Module.Name = request.Module
	options.Offer.Inputs = request.Inputs

	moduleOptions, err := optionsfactory.ProcessModuleOptions(options.Offer.Module)
	if err != nil {
		return options, err
	}
	options.Offer.Module = moduleOptions
	newWeb3Options, err := optionsfactory.ProcessWeb3Options(options.Web3)
	if err != nil {
		return options, err
	}
	options.Web3 = newWeb3Options
	return options, optionsfactory.CheckJobCreatorOptions(options)
}

func (apiServer *LilysaasAPIServer) createJob(res http.ResponseWriter, req *http.Request) (createJobResponse, error) {
	// user := getRequestUser(req.Context())

	request := createJobRequest{}
	bs, err := io.ReadAll(req.Body)
	if err != nil {
		return createJobResponse{}, err
	}
	err = json.Unmarshal(bs, &request)
	if err != nil {
		return createJobResponse{}, err
	}

	options := optionsfactory.NewJobCreatorOptions()
	options, err = ProcessJobCreatorOptions(options, request)
	if err != nil {
		return createJobResponse{}, err
	}

	stupidCommand := &cobra.Command{}
	stupidCommand.SetContext(context.TODO())
	cmdCtx := system.NewCommandContext(stupidCommand)

	// TODO: make async, add job status command
	result, err := jobcreator.RunJob(cmdCtx, options, func(evOffer data.JobOfferContainer) {
		// TODO: update postgres
		// TODO: ping websocket (later)
	})
	if err != nil {
		return createJobResponse{}, err
	}
	log.Printf("--> got result %s", result.Result.DataID)
	log.Printf("--> look here %s", solver.GetDownloadsFilePath(result.JobOffer.DealID))

	return createJobResponse{
		//User: user,
	}, nil
}
