package cron

import (
	"context"
	"errors"
	"testing"

	"github.com/helixml/helix/api/pkg/config"
	"github.com/helixml/helix/api/pkg/controller"
	"github.com/helixml/helix/api/pkg/extract"
	"github.com/helixml/helix/api/pkg/filestore"
	"github.com/helixml/helix/api/pkg/janitor"
	"github.com/helixml/helix/api/pkg/notification"
	"github.com/helixml/helix/api/pkg/openai"
	"github.com/helixml/helix/api/pkg/openai/manager"
	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/types"

	oai "github.com/sashabaranov/go-openai"
	"github.com/stretchr/testify/suite"
	"go.uber.org/mock/gomock"
)

func TestActionTestSuite(t *testing.T) {
	suite.Run(t, new(CronTestSuite))
}

type CronTestSuite struct {
	suite.Suite
	ctrl         *gomock.Controller
	store        *store.MockStore
	openAiClient *openai.MockClient
	manager      *manager.MockProviderManager
	controller   *controller.Controller
	notifier     *notification.MockNotifier
	ctx          context.Context
}

func (suite *CronTestSuite) SetupTest() {
	suite.ctrl = gomock.NewController(suite.T())
	suite.store = store.NewMockStore(suite.ctrl)
	suite.openAiClient = openai.NewMockClient(suite.ctrl)
	suite.manager = manager.NewMockProviderManager(suite.ctrl)
	suite.notifier = notification.NewMockNotifier(suite.ctrl)
	suite.ctx = context.Background()

	var err error

	cfg := &config.ServerConfig{}
	cfg.Inference.Provider = string(types.ProviderTogetherAI)

	filestoreMock := filestore.NewMockFileStore(suite.ctrl)
	extractorMock := extract.NewMockExtractor(suite.ctrl)

	suite.manager.EXPECT().GetClient(gomock.Any(), gomock.Any()).Return(suite.openAiClient, nil).AnyTimes()

	suite.controller, err = controller.NewController(context.Background(), controller.Options{
		Config:          cfg,
		Store:           suite.store,
		Janitor:         janitor.NewJanitor(config.Janitor{}),
		ProviderManager: suite.manager,
		Filestore:       filestoreMock,
		Extractor:       extractorMock,
	})
	suite.NoError(err)
}

func (suite *CronTestSuite) TestExecuteCronTask() {
	user := &types.User{
		ID: "test-user",
	}

	app := &types.App{
		ID:        "app-123",
		Owner:     "test-user",
		OwnerType: types.OwnerTypeUser,
		Config: types.AppConfig{
			Helix: types.AppHelixConfig{
				Assistants: []types.AssistantConfig{
					{
						Name:         "test-assistant",
						SystemPrompt: "you are very custom assistant",
					},
				},
			},
		},
	}

	trigger := &types.CronTrigger{
		Input: "test input",
	}

	// Mock GetAppWithTools
	suite.store.EXPECT().GetAppWithTools(gomock.Any(), "app-123").Return(app, nil).Times(2)

	suite.store.EXPECT().ListSecrets(gomock.Any(), gomock.Any()).Return([]*types.Secret{}, nil)

	// Mock GetUser
	suite.store.EXPECT().GetUser(gomock.Any(), &store.GetUserQuery{
		ID: "test-user",
	}).Return(user, nil)

	// Mock CreateTriggerExecution
	suite.store.EXPECT().CreateTriggerExecution(gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, execution *types.TriggerExecution) (*types.TriggerExecution, error) {
			suite.Equal("trigger-123", execution.TriggerConfigurationID)
			suite.Equal(types.TriggerExecutionStatusRunning, execution.Status)
			return execution, nil
		},
	)

	// Mock UpdateSession for the initial session write
	suite.store.EXPECT().UpdateSession(gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, session types.Session) (*types.Session, error) {
			suite.Equal("app-123", session.ParentApp)
			suite.Equal("test-user", session.Owner)
			suite.Equal(types.SessionModeInference, session.Mode)
			suite.Equal(types.SessionTypeText, session.Type)
			suite.Len(session.Interactions, 2)
			return &session, nil
		},
	)

	// Calling LLM chat completion
	suite.openAiClient.EXPECT().CreateChatCompletion(gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, _ oai.ChatCompletionRequest) (oai.ChatCompletionResponse, error) {

			return oai.ChatCompletionResponse{
				Choices: []oai.ChatCompletionChoice{
					{
						Message: oai.ChatCompletionMessage{
							Content: "test-response",
						},
					},
				},
			}, nil
		},
	)

	// Get session
	suite.store.EXPECT().GetSession(gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, id string) (*types.Session, error) {
			session := &types.Session{
				ID: id,
			}
			return session, nil
		},
	).Times(2)

	// Mock UpdateSession for the final session update
	suite.store.EXPECT().UpdateSession(gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, session types.Session) (*types.Session, error) {
			// Verify the assistant interaction was updated with the response
			suite.Len(session.Interactions, 2)
			assistantInteraction := session.Interactions[1]
			suite.Equal(types.CreatorTypeAssistant, assistantInteraction.Creator)
			suite.Equal(types.InteractionStateComplete, assistantInteraction.State)
			suite.True(assistantInteraction.Finished)
			suite.NotEmpty(assistantInteraction.Message)
			return &session, nil
		},
	)

	// Mock UpdateTriggerExecution for success
	suite.store.EXPECT().UpdateTriggerExecution(gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, execution *types.TriggerExecution) (*types.TriggerExecution, error) {
			suite.Equal(types.TriggerExecutionStatusSuccess, execution.Status, execution.Error)
			suite.NotEmpty(execution.Output)

			return execution, nil
		},
	)

	// Mock Notify for success notification
	suite.notifier.EXPECT().Notify(gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, n *notification.Notification) error {
			suite.Equal(notification.EventCronTriggerComplete, n.Event)
			suite.NotEmpty(n.Message)
			return nil
		},
	)

	// Execute the function
	result, err := ExecuteCronTask(suite.ctx, suite.store, suite.controller, suite.notifier, app, "trigger-123", trigger, "test-session")

	// Verify the result
	suite.NoError(err)
	suite.NotEmpty(result)
}

func (suite *CronTestSuite) TestExecuteCronTask_Error() {
	app := &types.App{
		ID:        "app-123",
		Owner:     "test-user",
		OwnerType: types.OwnerTypeUser,
	}

	trigger := &types.CronTrigger{
		Input: "test input",
	}

	// Mock GetAppWithTools to return error
	suite.store.EXPECT().GetAppWithTools(suite.ctx, "app-123").Return(nil, errors.New("database error"))

	// Execute the function
	result, err := ExecuteCronTask(suite.ctx, suite.store, suite.controller, suite.notifier, app, "trigger-123", trigger, "test-session")

	// Verify the error
	suite.Error(err)
	suite.Empty(result)
	suite.Contains(err.Error(), "database error")
}
