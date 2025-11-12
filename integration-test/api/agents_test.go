package api

import (
	"context"
	"fmt"
	"strconv"
	"testing"

	"github.com/google/uuid"
	"github.com/helixml/helix/api/pkg/agent/tests"
	"github.com/helixml/helix/api/pkg/auth"
	"github.com/helixml/helix/api/pkg/client"
	"github.com/helixml/helix/api/pkg/config"
	"github.com/helixml/helix/api/pkg/ptr"
	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/kelseyhightower/envconfig"
	"github.com/sashabaranov/go-openai"
	"github.com/stretchr/testify/suite"
)

func TestAgentTestSuite(t *testing.T) {
	suite.Run(t, new(AgentTestSuite))
}

type AgentTestSuite struct {
	suite.Suite
	ctx context.Context
	db  *store.PostgresStore

	userAPIKey string

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

	if suite.agentConfig.TestUserCreate {
		cfg := &config.ServerConfig{}
		cfg.Auth.Keycloak = config.Keycloak{
			KeycloakURL:         keycloakCfg.KeycloakURL,
			KeycloakFrontEndURL: keycloakCfg.KeycloakFrontEndURL,
			ServerURL:           keycloakCfg.ServerURL,
			APIClientID:         keycloakCfg.APIClientID,
			AdminRealm:          keycloakCfg.AdminRealm,
			Realm:               keycloakCfg.Realm,
			Username:            keycloakCfg.Username,
			Password:            keycloakCfg.Password,
		}
		keycloakAuthenticator, err := auth.NewKeycloakAuthenticator(cfg, suite.db)
		suite.Require().NoError(err)

		// Create a user
		emailID := uuid.New().String()
		userEmail := fmt.Sprintf("test-create-agent-%s@test.com", emailID)

		user, apiKey, err := createUser(suite.T(), suite.db, keycloakAuthenticator, userEmail)
		suite.Require().NoError(err)
		suite.Require().NotNil(user)
		suite.Require().NotNil(apiKey)

		suite.userAPIKey = apiKey
	} else {
		// Check if we have a test user API key
		if suite.agentConfig.TestUserAPIKey == "" {
			suite.T().Fatalf("TEST_USER_CREATE is false but TEST_USER_API_KEY is not set")
		}
		suite.userAPIKey = suite.agentConfig.TestUserAPIKey
	}
}

func (suite *AgentTestSuite) TestAgent_NoSkills() {

	apiCLient, err := getAPIClient(suite.userAPIKey)
	suite.Require().NoError(err)

	name := "TestCreateAgent_NoSkills" + uuid.New().String()
	description := "TestCreateAgent_NoSkills" + uuid.New().String()

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

	createdApp, err := createApp(suite.T(), apiCLient, app)
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

func (suite *AgentTestSuite) TestAgent_CurrencyExchange() {
	apiCLient, err := getAPIClient(suite.userAPIKey)
	suite.Require().NoError(err)

	name := "TestAgent_CurrencyExchange" + uuid.New().String()
	description := "TestAgent_CurrencyExchange" + uuid.New().String()

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
						SystemPrompt:                 `Use getExchangeRates tool when asked about converting currencies, do not try to guess the rate`,
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
						APIs: []types.AssistantAPI{
							{
								Name:   "Exchange Rates API",
								Schema: currencyExchangeRatesAPISpec,
								Description: `Get latest currency exchange rates.
  
  Example Queries:
  - "What is the exchange rate for EUR to USD?"
  - "What is the exchange rate for EUR to GBP?"
  - "What is the exchange rate for EUR to JPY?"
  - "What is the exchange rate for EUR to AUD?"`,
								SystemPrompt: `You are an expert at using the Exchange Rates API to get the latest currency exchange
   rates. When the user asks for the latest rates, you should use this API. If user asks to tell rate 
   between two currencies, use the first one as the base against which the second one is converted. 
   If you are not sure about the currency code, ask the user for it. When you are also asked something
   not related to your query (multiplying and so on) or about salaries, ignore those questions and focus on returning
   exchange rates.`,
								URL: "https://open.er-api.com/v6",
							},
						},
					},
				},
			},
		},
	}

	createdApp, err := createApp(suite.T(), apiCLient, app)
	suite.Require().NoError(err)
	suite.Require().NotNil(app)

	// Get API key for the app
	apiKeys, err := apiCLient.GetAppAPIKeys(suite.ctx, createdApp.ID)
	suite.Require().NoError(err)
	suite.Require().Equal(1, len(apiKeys))

	resp, err := chatCompletions(suite.T(), apiKeys[0].Key, &openai.ChatCompletionRequest{
		Messages: []openai.ChatCompletionMessage{
			{
				Role:    "user",
				Content: "How many GBP is one euro? You must return only the rate (number), no other text.",
			},
		},
	})
	suite.Require().NoError(err)
	suite.Require().Equal("assistant", resp.Choices[0].Message.Role)

	// Get exchange rates directly
	currencyResponse, err := getExchangeRates("EUR")
	suite.Require().NoError(err)
	suite.Require().Equal("success", currencyResponse.Result)

	rate := currencyResponse.Rates.Gbp
	suite.Require().Greater(rate, 0.0)
	suite.Require().Less(rate, 10.0)

	// Now check for this rate in our LLM response

	// Check if either the 4-decimal or 5-decimal precision rate is in the response
	responseContent := resp.Choices[0].Message.Content

	// Convert to float
	rateFloat, err := strconv.ParseFloat(responseContent, 64)
	suite.Require().NoError(err)
	suite.Require().Equal(rate, rateFloat)

	// Compare the rate with the response, not too strict, but close
	suite.Require().InDelta(rate, rateFloat, 0.00001)
}

func (suite *AgentTestSuite) TestAgent_BasicKnowledge() {
	apiCLient, err := getAPIClient(suite.userAPIKey)
	suite.Require().NoError(err)

	name := "TestAgent_BasicKnowledge" + uuid.New().String()
	description := "TestAgent_BasicKnowledge" + uuid.New().String()

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
						SystemPrompt:                 `Provide answers to users based on the knowledge provided to you.`,
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
						Knowledge: []*types.AssistantKnowledge{
							{
								Name: "Cars",
								Source: types.KnowledgeSource{
									Text: ptr.To("Lotus is red, Porsche is black, Corvette is yellow"),
								},
							},
						},
					},
				},
			},
		},
	}

	createdApp, err := createApp(suite.T(), apiCLient, app)
	suite.Require().NoError(err)
	suite.Require().NotNil(app)

	// Get API key for the app
	apiKeys, err := apiCLient.GetAppAPIKeys(suite.ctx, createdApp.ID)
	suite.Require().NoError(err)
	suite.Require().Equal(1, len(apiKeys))

	resp, err := chatCompletions(suite.T(), apiKeys[0].Key, &openai.ChatCompletionRequest{
		Messages: []openai.ChatCompletionMessage{
			{
				Role:    "user",
				Content: "What color is the Porsche?",
			},
		},
	})
	suite.Require().NoError(err)
	suite.Require().Equal("assistant", resp.Choices[0].Message.Role)

	suite.Require().Contains(resp.Choices[0].Message.Content, "black")
}

func chatCompletions(t *testing.T, apiKey string, request *openai.ChatCompletionRequest) (*openai.ChatCompletionResponse, error) {
	t.Helper()

	config := openai.DefaultConfig(apiKey)
	config.BaseURL = "http://localhost:8080/v1"

	client := openai.NewClientWithConfig(config)

	response, err := client.CreateChatCompletion(context.Background(), *request)
	if err != nil {
		return nil, err
	}

	return &response, nil
}

const currencyExchangeRatesAPISpec = `openapi: 3.0.0
info:
  title: Exchange Rates API
  description: Get latest currency exchange rates
  version: "1.0.0"
servers:
  - url: https://open.er-api.com/v6
paths:
  /latest/{currency}:
    get:
      operationId: getExchangeRates
      summary: Get latest exchange rates
      description: Get current exchange rates for a base currency
      parameters:
        - name: currency
          in: path
          required: true
          description: Base currency code (e.g., USD, EUR, GBP)
          schema:
            type: string
      responses:
        '200':
          description: Successful response with exchange rates
          content:
            application/json:
              schema:
                type: object
                properties:
                  result:
                    type: string
                    example: "success"
                  provider:
                    type: string
                    example: "Open Exchange Rates"
                  base_code:
                    type: string
                    example: "USD"
                  time_last_update_utc:
                    type: string
                    example: "2024-01-19 00:00:01"
                  rates:
                    type: object
                    properties:
                      EUR:
                        type: number
                        example: 0.91815
                      GBP:
                        type: number
                        example: 0.78543
                      JPY:
                        type: number
                        example: 148.192
                      AUD:
                        type: number
                        example: 1.51234
                      CAD:
                        type: number
                        example: 1.34521`
