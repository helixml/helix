package azuredevops

import (
	"github.com/microsoft/azure-devops-go-api/azuredevops/v7"
)

type azureDevOpsClient struct {
	connection *azuredevops.Connection
}

func newAzureDevOpsClient(organizationURL string, personalAccessToken string) *azureDevOpsClient {
	connection := azuredevops.NewPatConnection(organizationURL, personalAccessToken)

	return &azureDevOpsClient{
		connection: connection,
	}
}
