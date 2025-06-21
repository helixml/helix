package api

import (
	"context"
	"fmt"
	"testing"

	"github.com/google/uuid"
	"github.com/helixml/helix/api/pkg/agent/tests"
	"github.com/helixml/helix/api/pkg/auth"
	"github.com/helixml/helix/api/pkg/client"
	"github.com/helixml/helix/api/pkg/config"
	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/kelseyhightower/envconfig"
	"github.com/stretchr/testify/suite"
)

func TestAgentTestSuite(t *testing.T) {
	suite.Run(t, new(AgentTestSuite))
}

type AgentTestSuite struct {
	suite.Suite
	ctx      context.Context
	db       *store.PostgresStore
	keycloak *auth.KeycloakAuthenticator

	agentConfig *tests.Config
}

func (suite *AgentTestSuite) SetupTest() {
	suite.ctx = context.Background()
	store, err := getStoreClient()
	suite.Require().NoError(err)
	suite.db = store

	var keycloakCfg config.Keycloak

	agentConfig, err := tests.LoadConfig()
	suite.Require().NoError(err)
	suite.agentConfig = agentConfig

	err = envconfig.Process("", &keycloakCfg)
	suite.NoError(err)

	keycloakAuthenticator, err := auth.NewKeycloakAuthenticator(&config.Keycloak{
		KeycloakURL:         keycloakCfg.KeycloakURL,
		KeycloakFrontEndURL: keycloakCfg.KeycloakFrontEndURL,
		ServerURL:           keycloakCfg.ServerURL,
		APIClientID:         keycloakCfg.APIClientID,
		FrontEndClientID:    keycloakCfg.FrontEndClientID,
		AdminRealm:          keycloakCfg.AdminRealm,
		Realm:               keycloakCfg.Realm,
		Username:            keycloakCfg.Username,
		Password:            keycloakCfg.Password,
	}, suite.db)
	suite.Require().NoError(err)

	suite.keycloak = keycloakAuthenticator
}

func (suite *AgentTestSuite) TestCreateAgent_NoSkills() {
	// Create a user
	emailID := uuid.New().String()
	userEmail := fmt.Sprintf("test-create-agent-%s@test.com", emailID)

	user, apiKey, err := createUser(suite.T(), suite.db, suite.keycloak, userEmail)
	suite.Require().NoError(err)
	suite.Require().NotNil(user)
	suite.Require().NotNil(apiKey)

	apiCLient, err := getAPIClient(apiKey)
	suite.Require().NoError(err)

	name := "AgentTestSuite" + uuid.New().String()
	description := "AgentTestSuite" + uuid.New().String()

	app := &types.App{
		Config: types.AppConfig{
			Helix: types.AppHelixConfig{
				Name:        name,
				Description: description,
				Assistants: []types.AssistantConfig{
					{
						Name:                         name,
						Description:                  description,
						AgentMode:                    true,
						ReasoningModelProvider:       suite.agentConfig.ReasoningModelProvider,
						ReasoningModel:               suite.agentConfig.ReasoningModel,
						ReasoningModelEffort:         suite.agentConfig.ReasoningModelEffort,
						GenerationModelProvider:      suite.agentConfig.GenerationModelProvider,
						GenerationModel:              suite.agentConfig.GenerationModel,
						SmallReasoningModelProvider:  suite.agentConfig.SmallReasoningModelProvider,
						SmallReasoningModel:          suite.agentConfig.SmallReasoningModel,
						SmallReasoningModelEffort:    suite.agentConfig.SmallReasoningModelEffort,
						SmallGenerationModelProvider: suite.agentConfig.SmallGenerationModelProvider,
						SmallGenerationModel:         suite.agentConfig.SmallGenerationModel,
					},
				},
			},
		},
	}

	createdApp, err := createApp(suite.T(), apiCLient, user, app)
	suite.Require().NoError(err)
	suite.Require().NotNil(app)

	// List apps, need to find ours
	found := false
	apps, err := apiCLient.ListApps(suite.ctx, &client.AppFilter{})
	suite.Require().NoError(err)
	for _, app := range apps {
		if app.ID == createdApp.ID {
			found = true
			break
		}
	}
	suite.Require().True(found, "App not found")
}
