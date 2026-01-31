package cron

import (
	"context"
	"errors"
	"strings"
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

	// Mock CreateTriggerExecution
	suite.store.EXPECT().CreateTriggerExecution(gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, execution *types.TriggerExecution) (*types.TriggerExecution, error) {
			suite.Equal("trigger-123", execution.TriggerConfigurationID)
			suite.Equal(types.TriggerExecutionStatusRunning, execution.Status)
			return execution, nil
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
		Provider: "togetherai",
		Owner:    "test-user",
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
	result, err := ExecuteCronTask(suite.ctx, suite.store, suite.controller, suite.notifier, app, "test-user", "trigger-123", trigger, "test-session")

	// Verify the result
	suite.NoError(err)
	suite.NotEmpty(result)
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

	// Mock CreateTriggerExecution
	suite.store.EXPECT().CreateTriggerExecution(gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, execution *types.TriggerExecution) (*types.TriggerExecution, error) {
			suite.Equal("trigger-123", execution.TriggerConfigurationID)
			suite.Equal(types.TriggerExecutionStatusRunning, execution.Status)
			return execution, nil
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
		Provider: "togetherai",
		Owner:    "test-org",
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
	result, err := ExecuteCronTask(suite.ctx, suite.store, suite.controller, suite.notifier, app, user.ID, "trigger-123", trigger, "test-session")

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
	result, err := ExecuteCronTask(suite.ctx, suite.store, suite.controller, suite.notifier, app, "test-user", "trigger-123", trigger, "test-session")

	// Verify the error
	suite.Error(err)
	suite.Empty(result)
	suite.Contains(err.Error(), "database error")
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
