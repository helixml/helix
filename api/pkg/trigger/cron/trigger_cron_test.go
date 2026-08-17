package cron

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

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
	cronv3 "github.com/robfig/cron/v3"

	oai "github.com/sashabaranov/go-openai"
	"github.com/stretchr/testify/assert"
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

	suite.manager.EXPECT().GetClient(gomock.Any(), gomock.Any()).Return(suite.openAiClient, nil).Times(1)

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

func (suite *CronTestSuite) TestParseCronSchedule() {
	tests := []struct {
		name     string
		schedule string
		expected string
	}{
		{
			name:     "Asia/Dubai timezone",
			schedule: "CRON_TZ=Asia/Dubai 10 8 * * 1,2,3,4,5",
			expected: "Asia/Dubai",
		},
		{
			name:     "UTC timezone",
			schedule: "CRON_TZ=UTC 0 9 * * 1,2,3",
			expected: "UTC",
		},
	}

	for _, tt := range tests {
		suite.T().Run(tt.name, func(_ *testing.T) {
			_, err := cronv3.ParseStandard(tt.schedule)
			suite.NoError(err)
		})
	}
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

	// Mock trigger execution reservation
	suite.store.EXPECT().CreateTriggerExecutionUnlessRunning(gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, execution *types.TriggerExecution) (*types.TriggerExecution, bool, error) {
			suite.Equal("trigger-123", execution.TriggerConfigurationID)
			suite.Equal(types.TriggerExecutionStatusRunning, execution.Status)
			return execution, true, nil
		},
	)

	suite.store.EXPECT().ListInteractions(gomock.Any(), gomock.Any()).Return([]*types.Interaction{}, int64(0), nil)
	suite.store.EXPECT().CreateInteractions(gomock.Any(), gomock.Any()).Return(nil)

	// Mock UpdateSession for the initial session write
	suite.store.EXPECT().UpdateSession(gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, session types.Session) (*types.Session, error) {
			suite.Equal("app-123", session.ParentApp)
			suite.Equal("test-user", session.Owner)
			suite.Equal(types.SessionModeInference, session.Mode)
			suite.Equal(types.SessionTypeText, session.Type)
			return &session, nil
		},
	)

	suite.manager.EXPECT().GetClient(gomock.Any(), &manager.GetClientRequest{
		Provider:  "togetherai",
		Owner:     "test-user",
		OwnerType: types.OwnerTypeUser,
	}).Return(suite.openAiClient, nil).Times(1)

	suite.openAiClient.EXPECT().BillingEnabled().Return(true).AnyTimes()

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
	).Times(1)

	suite.store.EXPECT().UpdateInteraction(gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, interaction *types.Interaction) (*types.Interaction, error) {
			suite.Equal(types.InteractionStateComplete, interaction.State)
			suite.NotEmpty(interaction.ResponseMessage)
			return interaction, nil
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
			suite.Equal(types.EventCronTriggerComplete, n.Event)
			suite.NotEmpty(n.Message)
			return nil
		},
	)

	// Execute the function
	result, err := ExecuteCronTask(suite.ctx, suite.store, suite.controller, suite.notifier, nil, nil, app, "test-user", "trigger-123", trigger, "test-session")

	// Verify the result
	suite.NoError(err)
	suite.Equal(types.TriggerExecutionStatusSuccess, result.Status)
	suite.Equal("test-response", result.Content)
	suite.NotEmpty(result.SessionID)
}

func (suite *CronTestSuite) TestExecuteCronTask_Organization() {
	user := &types.User{
		ID: "test-user-2", // Different from app owner
	}

	app := &types.App{
		ID:             "app-123",
		Owner:          "test-user-1",
		OwnerType:      types.OwnerTypeUser,
		OrganizationID: "test-org",
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
		ID: user.ID,
	}).Return(user, nil)

	// Mock trigger execution reservation
	suite.store.EXPECT().CreateTriggerExecutionUnlessRunning(gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, execution *types.TriggerExecution) (*types.TriggerExecution, bool, error) {
			suite.Equal("trigger-123", execution.TriggerConfigurationID)
			suite.Equal(types.TriggerExecutionStatusRunning, execution.Status)
			return execution, true, nil
		},
	)

	suite.store.EXPECT().ListInteractions(gomock.Any(), gomock.Any()).Return([]*types.Interaction{}, int64(0), nil)
	suite.store.EXPECT().CreateInteractions(gomock.Any(), gomock.Any()).Return(nil)

	// Mock UpdateSession for the initial session write
	suite.store.EXPECT().UpdateSession(gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, session types.Session) (*types.Session, error) {
			suite.Equal("app-123", session.ParentApp)
			suite.Equal(user.ID, session.Owner)
			suite.Equal("test-org", session.OrganizationID)
			suite.Equal(types.SessionModeInference, session.Mode)
			suite.Equal(types.SessionTypeText, session.Type)
			return &session, nil
		},
	)

	suite.manager.EXPECT().GetClient(gomock.Any(), &manager.GetClientRequest{
		Provider:  "togetherai",
		Owner:     "test-org",
		OwnerType: types.OwnerTypeOrg,
	}).Return(suite.openAiClient, nil).Times(1)

	suite.openAiClient.EXPECT().BillingEnabled().Return(true).AnyTimes()

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
	).Times(1)

	suite.store.EXPECT().UpdateInteraction(gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, interaction *types.Interaction) (*types.Interaction, error) {
			suite.Equal(types.InteractionStateComplete, interaction.State)
			suite.Equal(user.ID, interaction.UserID)
			suite.NotEmpty(interaction.ResponseMessage)
			return interaction, nil
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
			suite.Equal(types.EventCronTriggerComplete, n.Event)
			suite.NotEmpty(n.Message)
			return nil
		},
	)

	// Execute the function
	result, err := ExecuteCronTask(suite.ctx, suite.store, suite.controller, suite.notifier, nil, nil, app, user.ID, "trigger-123", trigger, "test-session")

	// Verify the result
	suite.NoError(err)
	suite.Equal(types.TriggerExecutionStatusSuccess, result.Status)
	suite.Equal("test-response", result.Content)
	suite.NotEmpty(result.SessionID)
}

func (suite *CronTestSuite) TestExecuteCronTask_WithEmails() {
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
		Input:  "test input",
		Emails: []string{"alice@example.com", "bob@example.com"},
	}

	// Mock GetAppWithTools
	suite.store.EXPECT().GetAppWithTools(gomock.Any(), "app-123").Return(app, nil).Times(2)

	suite.store.EXPECT().ListSecrets(gomock.Any(), gomock.Any()).Return([]*types.Secret{}, nil)

	// Mock GetUser
	suite.store.EXPECT().GetUser(gomock.Any(), &store.GetUserQuery{
		ID: "test-user",
	}).Return(user, nil)

	// Mock trigger execution reservation
	suite.store.EXPECT().CreateTriggerExecutionUnlessRunning(gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, execution *types.TriggerExecution) (*types.TriggerExecution, bool, error) {
			return execution, true, nil
		},
	)

	suite.store.EXPECT().ListInteractions(gomock.Any(), gomock.Any()).Return([]*types.Interaction{}, int64(0), nil)
	suite.store.EXPECT().CreateInteractions(gomock.Any(), gomock.Any()).Return(nil)

	// Mock UpdateSession
	suite.store.EXPECT().UpdateSession(gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, session types.Session) (*types.Session, error) {
			return &session, nil
		},
	)

	suite.manager.EXPECT().GetClient(gomock.Any(), &manager.GetClientRequest{
		Provider:  "togetherai",
		Owner:     "test-user",
		OwnerType: types.OwnerTypeUser,
	}).Return(suite.openAiClient, nil).Times(1)

	suite.openAiClient.EXPECT().BillingEnabled().Return(true).AnyTimes()

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

	suite.store.EXPECT().GetSession(gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, id string) (*types.Session, error) {
			return &types.Session{ID: id}, nil
		},
	).Times(1)

	suite.store.EXPECT().UpdateInteraction(gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, interaction *types.Interaction) (*types.Interaction, error) {
			return interaction, nil
		},
	)

	suite.store.EXPECT().UpdateTriggerExecution(gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, execution *types.TriggerExecution) (*types.TriggerExecution, error) {
			suite.Equal(types.TriggerExecutionStatusSuccess, execution.Status, execution.Error)
			return execution, nil
		},
	)

	// Verify that the notification includes the configured emails
	suite.notifier.EXPECT().Notify(gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, n *notification.Notification) error {
			suite.Equal(types.EventCronTriggerComplete, n.Event)
			suite.Equal([]string{"alice@example.com", "bob@example.com"}, n.Emails)
			return nil
		},
	)

	result, err := ExecuteCronTask(suite.ctx, suite.store, suite.controller, suite.notifier, nil, nil, app, "test-user", "trigger-123", trigger, "test-session")
	suite.NoError(err)
	suite.Equal(types.TriggerExecutionStatusSuccess, result.Status)
	suite.Equal("test-response", result.Content)
}

func (suite *CronTestSuite) TestExecuteCronTask_FailureNotification_WithEmails() {
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
		Input:  "test input",
		Emails: []string{"alice@example.com"},
	}

	// Mock GetAppWithTools
	suite.store.EXPECT().GetAppWithTools(gomock.Any(), "app-123").Return(app, nil).Times(2)

	suite.store.EXPECT().ListSecrets(gomock.Any(), gomock.Any()).Return([]*types.Secret{}, nil)

	// Mock GetUser
	suite.store.EXPECT().GetUser(gomock.Any(), &store.GetUserQuery{
		ID: "test-user",
	}).Return(user, nil)

	// Mock trigger execution reservation
	suite.store.EXPECT().CreateTriggerExecutionUnlessRunning(gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, execution *types.TriggerExecution) (*types.TriggerExecution, bool, error) {
			return execution, true, nil
		},
	)

	suite.store.EXPECT().ListInteractions(gomock.Any(), gomock.Any()).Return([]*types.Interaction{}, int64(0), nil)
	suite.store.EXPECT().CreateInteractions(gomock.Any(), gomock.Any()).Return(nil)

	// Mock UpdateSession
	suite.store.EXPECT().UpdateSession(gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, session types.Session) (*types.Session, error) {
			return &session, nil
		},
	)

	suite.manager.EXPECT().GetClient(gomock.Any(), &manager.GetClientRequest{
		Provider:  "togetherai",
		Owner:     "test-user",
		OwnerType: types.OwnerTypeUser,
	}).Return(suite.openAiClient, nil).Times(1)

	suite.openAiClient.EXPECT().BillingEnabled().Return(true).AnyTimes()

	// LLM call fails
	suite.openAiClient.EXPECT().CreateChatCompletion(gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, _ oai.ChatCompletionRequest) (oai.ChatCompletionResponse, error) {
			return oai.ChatCompletionResponse{}, errors.New("LLM provider error")
		},
	)

	suite.store.EXPECT().GetSession(gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, id string) (*types.Session, error) {
			return &types.Session{ID: id}, nil
		},
	).AnyTimes()

	suite.store.EXPECT().UpdateInteraction(gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, interaction *types.Interaction) (*types.Interaction, error) {
			return interaction, nil
		},
	).AnyTimes()

	// Mock UpdateTriggerExecution for failure
	suite.store.EXPECT().UpdateTriggerExecution(gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, execution *types.TriggerExecution) (*types.TriggerExecution, error) {
			suite.Equal(types.TriggerExecutionStatusError, execution.Status)
			return execution, nil
		},
	)

	// Verify that the failure notification includes the configured emails
	suite.notifier.EXPECT().Notify(gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, n *notification.Notification) error {
			suite.Equal(types.EventCronTriggerFailed, n.Event)
			suite.Equal([]string{"alice@example.com"}, n.Emails)
			return nil
		},
	)

	result, err := ExecuteCronTask(suite.ctx, suite.store, suite.controller, suite.notifier, nil, nil, app, "test-user", "trigger-123", trigger, "test-session")
	suite.ErrorContains(err, "LLM provider error")
	suite.Empty(result)
}

func (suite *CronTestSuite) TestExecuteCronTask_NoEmails_FallsBackToOwner() {
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
		// No Emails set — should fall back to owner
	}

	suite.store.EXPECT().GetAppWithTools(gomock.Any(), "app-123").Return(app, nil).Times(2)
	suite.store.EXPECT().ListSecrets(gomock.Any(), gomock.Any()).Return([]*types.Secret{}, nil)
	suite.store.EXPECT().GetUser(gomock.Any(), &store.GetUserQuery{ID: "test-user"}).Return(user, nil)
	suite.store.EXPECT().CreateTriggerExecutionUnlessRunning(gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, execution *types.TriggerExecution) (*types.TriggerExecution, bool, error) {
			return execution, true, nil
		},
	)
	suite.store.EXPECT().ListInteractions(gomock.Any(), gomock.Any()).Return([]*types.Interaction{}, int64(0), nil)
	suite.store.EXPECT().CreateInteractions(gomock.Any(), gomock.Any()).Return(nil)
	suite.store.EXPECT().UpdateSession(gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, session types.Session) (*types.Session, error) {
			return &session, nil
		},
	)

	suite.manager.EXPECT().GetClient(gomock.Any(), &manager.GetClientRequest{
		Provider:  "togetherai",
		Owner:     "test-user",
		OwnerType: types.OwnerTypeUser,
	}).Return(suite.openAiClient, nil).Times(1)

	suite.openAiClient.EXPECT().BillingEnabled().Return(true).AnyTimes()

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

	suite.store.EXPECT().GetSession(gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, id string) (*types.Session, error) {
			return &types.Session{ID: id}, nil
		},
	).Times(1)

	suite.store.EXPECT().UpdateInteraction(gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, interaction *types.Interaction) (*types.Interaction, error) {
			return interaction, nil
		},
	)

	suite.store.EXPECT().UpdateTriggerExecution(gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, execution *types.TriggerExecution) (*types.TriggerExecution, error) {
			return execution, nil
		},
	)

	// Verify that the notification has nil/empty Emails (owner fallback)
	suite.notifier.EXPECT().Notify(gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, n *notification.Notification) error {
			suite.Equal(types.EventCronTriggerComplete, n.Event)
			suite.Empty(n.Emails)
			return nil
		},
	)

	result, err := ExecuteCronTask(suite.ctx, suite.store, suite.controller, suite.notifier, nil, nil, app, "test-user", "trigger-123", trigger, "test-session")
	suite.NoError(err)
	suite.Equal(types.TriggerExecutionStatusSuccess, result.Status)
	suite.Equal("test-response", result.Content)
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
	suite.store.EXPECT().CreateTriggerExecutionUnlessRunning(gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, execution *types.TriggerExecution) (*types.TriggerExecution, bool, error) {
			return execution, true, nil
		},
	)

	// Mock GetAppWithTools to return error
	suite.store.EXPECT().GetAppWithTools(suite.ctx, "app-123").Return(nil, errors.New("database error"))
	suite.store.EXPECT().UpdateTriggerExecution(gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, execution *types.TriggerExecution) (*types.TriggerExecution, error) {
			suite.Equal(types.TriggerExecutionStatusError, execution.Status)
			suite.Equal("database error", execution.Error)
			return execution, nil
		},
	)

	// Execute the function
	result, err := ExecuteCronTask(suite.ctx, suite.store, suite.controller, suite.notifier, nil, nil, app, "test-user", "trigger-123", trigger, "test-session")

	// Verify the error
	suite.Error(err)
	suite.Empty(result)
	suite.Contains(err.Error(), "database error")
}

func (suite *CronTestSuite) TestExecuteCronTask_SkipsWhilePreviousBlockingExecutionRuns() {
	app := &types.App{ID: "app-123"}
	trigger := &types.CronTrigger{Input: "test input"}

	suite.store.EXPECT().CreateTriggerExecutionUnlessRunning(gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, execution *types.TriggerExecution) (*types.TriggerExecution, bool, error) {
			execution.Status = types.TriggerExecutionStatusSkipped
			execution.Error = "Previous execution execution-running is still running"
			execution.SessionID = ""
			return execution, false, nil
		},
	)

	result, err := ExecuteCronTask(suite.ctx, suite.store, suite.controller, suite.notifier, nil, nil, app, "test-user", "trigger-123", trigger, "test-session")
	suite.NoError(err)
	suite.Equal(types.TriggerExecutionStatusSkipped, result.Status)
	suite.Empty(result.SessionID)
}

func TestNextRunFormatted(t *testing.T) {
	tests := []struct {
		name     string
		schedule string
		expected string
	}{
		{
			name:     "Asia/Dubai timezone",
			schedule: "CRON_TZ=Asia/Dubai 0 9 * * 1,2,3",
			expected: "Next run:",
		},
		{
			name:     "UTC timezone",
			schedule: "CRON_TZ=UTC 0 9 * * 1,2,3",
			expected: "Next run:",
		},
		{
			name:     "America/New_York timezone",
			schedule: "CRON_TZ=America/New_York 0 9 * * 1,2,3",
			expected: "Next run:",
		},
		{
			name:     "Invalid schedule",
			schedule: "invalid cron schedule",
			expected: "Invalid schedule",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cronTrigger := &types.CronTrigger{
				Schedule: tt.schedule,
				Enabled:  true,
			}

			result := NextRunFormatted(cronTrigger)

			if tt.expected == "Invalid schedule" {
				assert.Equal(t, tt.expected, result)
			} else {
				// For valid schedules, check that the result starts with "Next run:" and contains expected components
				assert.True(t, strings.HasPrefix(result, "Next run:"), "Result should start with 'Next run:'")
				assert.Contains(t, result, "at", "Result should contain 'at'")
				assert.Contains(t, result, ":", "Result should contain time separator")
			}
		})
	}
}

// mockSpecTaskCreator implements SpecTaskCreator for testing
type mockSpecTaskCreator struct {
	createFunc func(ctx context.Context, req *types.CreateTaskRequest) (*types.SpecTask, error)
}

func (m *mockSpecTaskCreator) CreateTaskFromPrompt(ctx context.Context, req *types.CreateTaskRequest) (*types.SpecTask, error) {
	return m.createFunc(ctx, req)
}

type mockExternalAgentStarter struct {
	startFunc func(ctx context.Context, req *types.SessionChatRequest, userID string) (*types.Session, error)
}

func (m *mockExternalAgentStarter) StartExternalAgentSession(ctx context.Context, req *types.SessionChatRequest, userID string) (*types.Session, error) {
	return m.startFunc(ctx, req, userID)
}

func (suite *CronTestSuite) TestExecuteCronTask_InfersExternalAgentConfiguration() {
	externalAgentConfig := &types.ExternalAgentConfig{DesktopType: "sway"}
	app := &types.App{
		ID:             "app-123",
		Owner:          "test-user",
		OwnerType:      types.OwnerTypeUser,
		OrganizationID: "org-123",
		Config: types.AppConfig{Helix: types.AppHelixConfig{
			DefaultAgentType:    types.AgentTypeZedExternal,
			ExternalAgentConfig: externalAgentConfig,
			Assistants: []types.AssistantConfig{{
				AgentType:        types.AgentTypeZedExternal,
				CodeAgentRuntime: types.CodeAgentRuntimeCodexCLI,
			}},
		}},
	}
	trigger := &types.CronTrigger{Input: "Inspect the repository"}

	suite.store.EXPECT().CreateTriggerExecutionUnlessRunning(gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, execution *types.TriggerExecution) (*types.TriggerExecution, bool, error) {
			suite.Equal("trigger-123", execution.TriggerConfigurationID)
			suite.NotEmpty(execution.SessionID)
			return execution, true, nil
		},
	)

	suite.store.EXPECT().ListProjects(gomock.Any(), &store.ListProjectsQuery{
		OrganizationID: "org-123",
	}).Return([]*types.Project{{
		ID:                "project-456",
		DefaultHelixAppID: "app-123",
	}}, nil)

	starter := &mockExternalAgentStarter{
		startFunc: func(_ context.Context, req *types.SessionChatRequest, userID string) (*types.Session, error) {
			suite.Equal("test-user", userID)
			suite.Equal("app-123", req.AppID)
			suite.Equal("0", req.AssistantID)
			suite.Equal("org-123", req.OrganizationID)
			suite.Equal("project-456", req.ProjectID)
			suite.Equal(string(types.AgentTypeZedExternal), req.AgentType)
			suite.Same(externalAgentConfig, req.ExternalAgentConfig)
			suite.Equal("job", req.SessionRole)
			suite.Contains(req.SystemPrompt, "task_completed")
			suite.Len(req.Messages, 1)
			suite.Equal([]any{"Inspect the repository"}, req.Messages[0].Content.Parts)
			return &types.Session{ID: req.SessionID}, nil
		},
	}

	result, err := ExecuteCronTask(suite.ctx, suite.store, suite.controller, suite.notifier, nil, starter, app, "test-user", "trigger-123", trigger, "scheduled-review")
	suite.NoError(err)
	suite.Equal(types.TriggerExecutionStatusRunning, result.Status)
	suite.NotEmpty(result.SessionID)
}

func (suite *CronTestSuite) TestExecuteCronTask_SkipsWhilePreviousExternalExecutionRuns() {
	app := &types.App{ID: "app-123", Config: types.AppConfig{Helix: types.AppHelixConfig{DefaultAgentType: types.AgentTypeZedExternal}}}
	trigger := &types.CronTrigger{Input: "Inspect the repository"}

	suite.store.EXPECT().CreateTriggerExecutionUnlessRunning(gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, execution *types.TriggerExecution) (*types.TriggerExecution, bool, error) {
			execution.Status = types.TriggerExecutionStatusSkipped
			execution.Error = "Previous execution tex_123 is still running"
			return execution, false, nil
		},
	)

	result, err := ExecuteCronTask(suite.ctx, suite.store, suite.controller, suite.notifier, nil, &mockExternalAgentStarter{
		startFunc: func(context.Context, *types.SessionChatRequest, string) (*types.Session, error) {
			suite.Fail("external agent must not start for a skipped execution")
			return nil, nil
		},
	}, app, "test-user", "trigger-123", trigger, "scheduled-review")
	suite.NoError(err)
	suite.Equal(types.TriggerExecutionStatusSkipped, result.Status)
	suite.Empty(result.SessionID)
}

func (suite *CronTestSuite) TestExecuteCronTask_RecordsExternalStartupFailure() {
	app := &types.App{ID: "app-123", Config: types.AppConfig{Helix: types.AppHelixConfig{DefaultAgentType: types.AgentTypeZedExternal}}}
	trigger := &types.CronTrigger{Input: "Inspect the repository", ProjectID: "project-123"}
	suite.store.EXPECT().CreateTriggerExecutionUnlessRunning(gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, candidate *types.TriggerExecution) (*types.TriggerExecution, bool, error) {
			candidate.Created = time.Now()
			return candidate, true, nil
		},
	)
	suite.store.EXPECT().UpdateTriggerExecution(gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, updated *types.TriggerExecution) (*types.TriggerExecution, error) {
			suite.Equal(types.TriggerExecutionStatusError, updated.Status)
			suite.Equal("sandbox unavailable", updated.Error)
			return updated, nil
		},
	)
	suite.notifier.EXPECT().Notify(gomock.Any(), gomock.Any()).Return(nil)

	result, err := ExecuteCronTask(suite.ctx, suite.store, suite.controller, suite.notifier, nil, &mockExternalAgentStarter{
		startFunc: func(context.Context, *types.SessionChatRequest, string) (*types.Session, error) {
			return nil, errors.New("sandbox unavailable")
		},
	}, app, "test-user", "trigger-123", trigger, "scheduled-review")
	suite.EqualError(err, "sandbox unavailable")
	suite.Empty(result)
}

func (suite *CronTestSuite) TestExternalAgentProjectIDRejectsAmbiguousAgent() {
	app := &types.App{ID: "app-123", OrganizationID: "org-123"}
	suite.store.EXPECT().ListProjects(gomock.Any(), &store.ListProjectsQuery{
		OrganizationID: "org-123",
	}).Return([]*types.Project{
		{ID: "project-1", DefaultHelixAppID: "app-123"},
		{ID: "project-2", ProjectManagerHelixAppID: "app-123"},
	}, nil)

	projectID, err := externalAgentProjectID(suite.ctx, suite.store, app, "test-user", "")
	suite.Error(err)
	suite.Empty(projectID)
	suite.Contains(err.Error(), "multiple projects")
}

func (suite *CronTestSuite) TestExecuteCronTask_SpecTaskAction() {
	app := &types.App{
		ID:        "app-123",
		Owner:     "test-user",
		OwnerType: types.OwnerTypeUser,
	}

	trigger := &types.CronTrigger{
		Input:     "Build the login page",
		Action:    "spec_task",
		ProjectID: "proj-456",
	}

	mockCreator := &mockSpecTaskCreator{
		createFunc: func(_ context.Context, req *types.CreateTaskRequest) (*types.SpecTask, error) {
			suite.Equal("proj-456", req.ProjectID)
			suite.Equal("Build the login page", req.Prompt)
			suite.Equal("test-user", req.UserID)
			// Cron-scheduled tasks must auto-start so they skip backlog
			// regardless of the project's AutoStartBacklogTasks setting.
			suite.True(req.AutoStart, "cron-triggered spec tasks must be created with AutoStart=true")
			// A trigger that names nobody must not invent a credential owner —
			// the run keeps authenticating as the app owner, as it always has.
			suite.Empty(req.CredentialOwnerID)
			return &types.SpecTask{
				ID:   "task-789",
				Name: "Build the login page",
			}, nil
		},
	}

	// Mock CreateTriggerExecution
	suite.store.EXPECT().CreateTriggerExecution(gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, execution *types.TriggerExecution) (*types.TriggerExecution, error) {
			suite.Equal("trigger-123", execution.TriggerConfigurationID)
			suite.Equal(types.TriggerExecutionStatusRunning, execution.Status)
			return execution, nil
		},
	)

	// Mock Notify for success
	suite.notifier.EXPECT().Notify(gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, n *notification.Notification) error {
			suite.Equal(types.EventCronTriggerComplete, n.Event)
			suite.Contains(n.Message, "task-789")
			return nil
		},
	)

	// Mock UpdateTriggerExecution for success
	suite.store.EXPECT().UpdateTriggerExecution(gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, execution *types.TriggerExecution) (*types.TriggerExecution, error) {
			suite.Equal(types.TriggerExecutionStatusSuccess, execution.Status)
			suite.Contains(execution.Output, "task-789")
			return execution, nil
		},
	)

	result, err := ExecuteCronTask(suite.ctx, suite.store, suite.controller, suite.notifier, mockCreator, nil, app, "test-user", "trigger-123", trigger, "test-session")
	suite.NoError(err)
	suite.Equal(types.TriggerExecutionStatusSuccess, result.Status)
	suite.Contains(result.Content, "task-789")
}

// A scheduled run has to authenticate as the person it acts for, not as the one
// service account whose key wrote every trigger. This pins the only link in that
// chain that lives here: the owner named on the trigger reaches the task request,
// where the delegation check downstream decides whether the grant is real.
func (suite *CronTestSuite) TestExecuteCronTask_SpecTaskActionCarriesCredentialOwner() {
	app := &types.App{
		ID:        "app-123",
		Owner:     "svc-account",
		OwnerType: types.OwnerTypeUser,
	}

	trigger := &types.CronTrigger{
		Input:             "Work the enterprise pipeline",
		Action:            "spec_task",
		ProjectID:         "proj-456",
		CredentialOwnerID: "usr_chris",
	}

	mockCreator := &mockSpecTaskCreator{
		createFunc: func(_ context.Context, req *types.CreateTaskRequest) (*types.SpecTask, error) {
			suite.Equal("usr_chris", req.CredentialOwnerID)
			// Credential resolution only — the task is still created by, and
			// attributed to, the account that owns the app.
			suite.Equal("svc-account", req.UserID)
			return &types.SpecTask{ID: "task-789", Name: "Work the enterprise pipeline"}, nil
		},
	}

	suite.store.EXPECT().CreateTriggerExecution(gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, execution *types.TriggerExecution) (*types.TriggerExecution, error) {
			return execution, nil
		},
	)
	suite.notifier.EXPECT().Notify(gomock.Any(), gomock.Any()).Return(nil)
	suite.store.EXPECT().UpdateTriggerExecution(gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, execution *types.TriggerExecution) (*types.TriggerExecution, error) {
			return execution, nil
		},
	)

	result, err := ExecuteCronTask(suite.ctx, suite.store, suite.controller, suite.notifier, mockCreator, nil, app, "svc-account", "trigger-123", trigger, "test-session")
	suite.NoError(err)
	suite.Equal(types.TriggerExecutionStatusSuccess, result.Status)
	suite.Contains(result.Content, "task-789")
}

func (suite *CronTestSuite) TestExecuteCronTask_SpecTaskAction_Error() {
	app := &types.App{
		ID:        "app-123",
		Owner:     "test-user",
		OwnerType: types.OwnerTypeUser,
	}

	trigger := &types.CronTrigger{
		Input:     "Build the login page",
		Action:    "spec_task",
		ProjectID: "proj-456",
		Emails:    []string{"user@example.com"},
	}

	mockCreator := &mockSpecTaskCreator{
		createFunc: func(_ context.Context, _ *types.CreateTaskRequest) (*types.SpecTask, error) {
			return nil, errors.New("project not found")
		},
	}

	// Mock CreateTriggerExecution
	suite.store.EXPECT().CreateTriggerExecution(gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, execution *types.TriggerExecution) (*types.TriggerExecution, error) {
			return execution, nil
		},
	)

	// Mock Notify for failure
	suite.notifier.EXPECT().Notify(gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, n *notification.Notification) error {
			suite.Equal(types.EventCronTriggerFailed, n.Event)
			suite.Contains(n.Message, "project not found")
			suite.Equal([]string{"user@example.com"}, n.Emails)
			return nil
		},
	)

	// Mock UpdateTriggerExecution for error
	suite.store.EXPECT().UpdateTriggerExecution(gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, execution *types.TriggerExecution) (*types.TriggerExecution, error) {
			suite.Equal(types.TriggerExecutionStatusError, execution.Status)
			suite.Contains(execution.Error, "project not found")
			return execution, nil
		},
	)

	result, err := ExecuteCronTask(suite.ctx, suite.store, suite.controller, suite.notifier, mockCreator, nil, app, "test-user", "trigger-123", trigger, "test-session")
	suite.Error(err)
	suite.Empty(result)
	suite.Contains(err.Error(), "project not found")
}

func (suite *CronTestSuite) TestExecuteCronTask_SpecTaskAction_MissingProjectID() {
	app := &types.App{
		ID:        "app-123",
		Owner:     "test-user",
		OwnerType: types.OwnerTypeUser,
	}

	trigger := &types.CronTrigger{
		Input:  "Build the login page",
		Action: "spec_task",
		// ProjectID intentionally empty
	}

	mockCreator := &mockSpecTaskCreator{
		createFunc: func(_ context.Context, _ *types.CreateTaskRequest) (*types.SpecTask, error) {
			suite.Fail("CreateTaskFromPrompt should not be called when ProjectID is empty")
			return nil, nil
		},
	}

	// A misconfigured trigger must still leave a failed execution behind — a fire
	// that records nothing looks identical to a scheduler that never fired.
	suite.store.EXPECT().CreateTriggerExecution(gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, execution *types.TriggerExecution) (*types.TriggerExecution, error) {
			return execution, nil
		},
	)
	suite.notifier.EXPECT().Notify(gomock.Any(), gomock.Any()).Return(nil)
	suite.store.EXPECT().UpdateTriggerExecution(gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, execution *types.TriggerExecution) (*types.TriggerExecution, error) {
			suite.Equal(types.TriggerExecutionStatusError, execution.Status)
			suite.Contains(execution.Error, "project_id is required")
			return execution, nil
		},
	)

	result, err := ExecuteCronTask(suite.ctx, suite.store, suite.controller, suite.notifier, mockCreator, nil, app, "test-user", "trigger-123", trigger, "test-session")
	suite.Error(err)
	suite.Empty(result)
	suite.Contains(err.Error(), "project_id is required")
}

func (suite *CronTestSuite) TestExecuteCronTask_SpecTaskAction_NilCreator() {
	app := &types.App{
		ID:        "app-123",
		Owner:     "test-user",
		OwnerType: types.OwnerTypeUser,
	}

	trigger := &types.CronTrigger{
		Input:     "Build the login page",
		Action:    "spec_task",
		ProjectID: "proj-456",
	}

	// The nil creator is the bug that silently killed every scheduled spec_task
	// for months. It must now surface as a recorded, errored execution.
	suite.store.EXPECT().CreateTriggerExecution(gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, execution *types.TriggerExecution) (*types.TriggerExecution, error) {
			return execution, nil
		},
	)
	suite.notifier.EXPECT().Notify(gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, n *notification.Notification) error {
			suite.Equal(types.EventCronTriggerFailed, n.Event)
			return nil
		},
	)
	suite.store.EXPECT().UpdateTriggerExecution(gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, execution *types.TriggerExecution) (*types.TriggerExecution, error) {
			suite.Equal(types.TriggerExecutionStatusError, execution.Status)
			suite.Contains(execution.Error, "spec task creator not configured")
			return execution, nil
		},
	)

	result, err := ExecuteCronTask(suite.ctx, suite.store, suite.controller, suite.notifier, nil, nil, app, "test-user", "trigger-123", trigger, "test-session")
	suite.Error(err)
	suite.Empty(result)
	suite.Contains(err.Error(), "spec task creator not configured")
}

// TestCronResolvesSpecTaskCreatorLate is the regression test for the bug where
// every scheduled spec_task silently no-opped: the scheduler was constructed
// during startup, BEFORE the API server built the SpecDrivenTaskService, so it
// captured a nil creator forever. The provider must be called when the job fires,
// not when the Cron is built.
func (suite *CronTestSuite) TestCronResolvesSpecTaskCreatorLate() {
	var wired SpecTaskCreator // nil at construction, exactly like real startup

	c, err := New(
		&config.ServerConfig{},
		suite.store,
		suite.notifier,
		suite.controller,
		func() SpecTaskCreator { return wired },
		func() ExternalAgentStarter { return nil },
	)
	suite.NoError(err)

	created := false
	wired = &mockSpecTaskCreator{
		createFunc: func(_ context.Context, req *types.CreateTaskRequest) (*types.SpecTask, error) {
			created = true
			suite.Equal("proj-456", req.ProjectID)
			return &types.SpecTask{ID: "task-789", Name: "Scheduled run"}, nil
		},
	}

	suite.store.EXPECT().CreateTriggerExecution(gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, execution *types.TriggerExecution) (*types.TriggerExecution, error) {
			return execution, nil
		},
	)
	suite.notifier.EXPECT().Notify(gomock.Any(), gomock.Any()).Return(nil)
	suite.store.EXPECT().UpdateTriggerExecution(gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, execution *types.TriggerExecution) (*types.TriggerExecution, error) {
			suite.Equal(types.TriggerExecutionStatusSuccess, execution.Status)
			return execution, nil
		},
	)

	c.runCronApp(suite.ctx, &cronApp{
		ID:     "trigger-123",
		UserID: "test-user",
		Name:   "Scheduled run",
		App:    &types.App{ID: "app-123", Owner: "test-user", OwnerType: types.OwnerTypeUser},
		Trigger: &types.CronTrigger{
			Enabled:   true,
			Schedule:  "CRON_TZ=America/New_York 0 9 * * 1-5",
			Input:     "Do the scheduled work",
			Action:    "spec_task",
			ProjectID: "proj-456",
		},
	})

	suite.True(created, "cron job must resolve the spec task creator at fire time, not at construction")
}

// New must reject nil providers — the whole point is that the value can be nil
// while the provider itself never is.
func (suite *CronTestSuite) TestNewRequiresProviders() {
	_, err := New(&config.ServerConfig{}, suite.store, suite.notifier, suite.controller, nil, func() ExternalAgentStarter { return nil })
	suite.Error(err)

	_, err = New(&config.ServerConfig{}, suite.store, suite.notifier, suite.controller, func() SpecTaskCreator { return nil }, nil)
	suite.Error(err)
}

func TestExtractTimezoneFromCron(t *testing.T) {
	tests := []struct {
		name     string
		schedule string
		expected string
	}{
		{
			name:     "Asia/Dubai timezone",
			schedule: "CRON_TZ=Asia/Dubai 0 9 * * 1,2,3",
			expected: "Asia/Dubai",
		},
		{
			name:     "UTC timezone",
			schedule: "CRON_TZ=UTC 0 9 * * 1,2,3",
			expected: "UTC",
		},
		{
			name:     "America/New_York timezone",
			schedule: "CRON_TZ=America/New_York 0 9 * * 1,2,3",
			expected: "America/New_York",
		},
		{
			name:     "No timezone",
			schedule: "0 9 * * 1,2,3",
			expected: "",
		},
		{
			name:     "Empty schedule",
			schedule: "",
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := extractTimezoneFromCron(tt.schedule)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// A scheduled spec_task must be able to ask for just-do-it mode, and the flag has
// to survive into the task request. It is load-bearing far beyond "skip the specs
// step": without it the run is created in spec_generation and parks in spec_review
// waiting for an approval nobody gives, and because BranchName is only assigned on
// the transition to implementation, the agent is then refused every git push
// except helix-specs ("This push is restricted to: helix-specs"). Dropping this
// field silently is what made scheduled runs never do their job.
func (suite *CronTestSuite) TestExecuteCronTask_SpecTaskActionCarriesJustDoItMode() {
	app := &types.App{
		ID:        "app-123",
		Owner:     "svc-account",
		OwnerType: types.OwnerTypeUser,
	}

	trigger := &types.CronTrigger{
		Input:        "Run the daily prospecting pass",
		Action:       "spec_task",
		ProjectID:    "proj-456",
		JustDoItMode: true,
	}

	mockCreator := &mockSpecTaskCreator{
		createFunc: func(_ context.Context, req *types.CreateTaskRequest) (*types.SpecTask, error) {
			suite.True(req.JustDoItMode, "JustDoItMode must reach the task request, or the run parks in spec_review and cannot push")
			// AutoStart is what takes it out of the backlog; both are needed for the
			// run to reach implementation.
			suite.True(req.AutoStart)
			return &types.SpecTask{ID: "task-789", Name: "Run the daily prospecting pass"}, nil
		},
	}

	suite.store.EXPECT().CreateTriggerExecution(gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, execution *types.TriggerExecution) (*types.TriggerExecution, error) {
			return execution, nil
		},
	)
	suite.notifier.EXPECT().Notify(gomock.Any(), gomock.Any()).Return(nil)
	suite.store.EXPECT().UpdateTriggerExecution(gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, execution *types.TriggerExecution) (*types.TriggerExecution, error) {
			return execution, nil
		},
	)

	result, err := ExecuteCronTask(suite.ctx, suite.store, suite.controller, suite.notifier, mockCreator, nil, app, "svc-account", "trigger-123", trigger, "test-session")
	suite.NoError(err)
	suite.Equal(types.TriggerExecutionStatusSuccess, result.Status)
}

// The default must stay false so an existing trigger that never asked for
// just-do-it keeps generating specs — this change is opt-in.
func (suite *CronTestSuite) TestExecuteCronTask_SpecTaskActionDefaultsToSpecGeneration() {
	app := &types.App{ID: "app-123", Owner: "svc-account", OwnerType: types.OwnerTypeUser}
	trigger := &types.CronTrigger{Input: "Plan something", Action: "spec_task", ProjectID: "proj-456"}

	mockCreator := &mockSpecTaskCreator{
		createFunc: func(_ context.Context, req *types.CreateTaskRequest) (*types.SpecTask, error) {
			suite.False(req.JustDoItMode, "unset on the trigger must stay unset on the request")
			return &types.SpecTask{ID: "task-789", Name: "Plan something"}, nil
		},
	}

	suite.store.EXPECT().CreateTriggerExecution(gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, e *types.TriggerExecution) (*types.TriggerExecution, error) { return e, nil },
	)
	suite.notifier.EXPECT().Notify(gomock.Any(), gomock.Any()).Return(nil)
	suite.store.EXPECT().UpdateTriggerExecution(gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, e *types.TriggerExecution) (*types.TriggerExecution, error) { return e, nil },
	)

	_, err := ExecuteCronTask(suite.ctx, suite.store, suite.controller, suite.notifier, mockCreator, nil, app, "svc-account", "trigger-123", trigger, "test-session")
	suite.NoError(err)
}
