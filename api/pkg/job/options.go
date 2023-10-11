package job

import (
	"fmt"

	"github.com/bacalhau-project/lilypad/pkg/jobcreator"
	optionsfactory "github.com/bacalhau-project/lilypad/pkg/options"
	"github.com/bacalhau-project/lilysaas/api/pkg/types"
)

func checkJobCreatorOptions(options jobcreator.JobCreatorOptions, withModule bool) error {
	if withModule {
		err := optionsfactory.CheckModuleOptions(options.Offer.Module)
		if err != nil {
			return err
		}
	}

	err := optionsfactory.CheckWeb3Options(options.Web3)
	if err != nil {
		return err
	}
	err = optionsfactory.CheckServicesOptions(options.Offer.Services)
	if err != nil {
		return err
	}

	if options.Mediation.CheckResultsPercentage < 0 || options.Mediation.CheckResultsPercentage > 100 {
		return fmt.Errorf("mediation-chance must be between 0 and 100")
	}

	return nil
}

func ProcessJobCreatorOptions(options jobcreator.JobCreatorOptions, request types.JobSpec, withModule bool) (jobcreator.JobCreatorOptions, error) {
	if withModule {
		options.Offer.Module.Name = request.Module
		options.Offer.Inputs = request.Inputs
		moduleOptions, err := optionsfactory.ProcessModuleOptions(options.Offer.Module)
		if err != nil {
			return options, err
		}
		options.Offer.Module = moduleOptions
	}

	newWeb3Options, err := optionsfactory.ProcessWeb3Options(options.Web3)
	if err != nil {
		return options, err
	}
	options.Web3 = newWeb3Options

	return options, checkJobCreatorOptions(options, withModule)
}

func GetJobOptions(request types.JobSpec, withModule bool) (jobcreator.JobCreatorOptions, error) {
	return ProcessJobCreatorOptions(optionsfactory.NewJobCreatorOptions(), request, withModule)
}
