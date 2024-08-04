package server

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/golang/mock/gomock"
	oai "github.com/lukemarsden/go-openai2"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"

	"github.com/helixml/helix/api/pkg/config"
	"github.com/helixml/helix/api/pkg/controller"
	"github.com/helixml/helix/api/pkg/extract"
	"github.com/helixml/helix/api/pkg/filestore"
	"github.com/helixml/helix/api/pkg/janitor"
	"github.com/helixml/helix/api/pkg/openai"
	"github.com/helixml/helix/api/pkg/pubsub"
	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/types"
)

func TestOpenAIChatSuite(t *testing.T) {
	suite.Run(t, new(OpenAIChatSuite))
}

type OpenAIChatSuite struct {
	suite.Suite

	store        *store.MockStore
	pubsub       pubsub.PubSub
	openAiClient *openai.MockClient

	authCtx context.Context
	userID  string

	server *HelixAPIServer
}

func (suite *OpenAIChatSuite) SetupTest() {
	ctrl := gomock.NewController(suite.T())

	suite.store = store.NewMockStore(ctrl)
	ps, err := pubsub.New(suite.T().TempDir())
	suite.NoError(err)

	suite.openAiClient = openai.NewMockClient(ctrl)
	suite.pubsub = ps

	filestoreMock := filestore.NewMockFileStore(ctrl)
	extractorMock := extract.NewMockExtractor(ctrl)

	suite.userID = "user_id"
	suite.authCtx = setRequestUser(context.Background(), types.User{
		ID:       suite.userID,
		Email:    "foo@email.com",
		FullName: "Foo Bar",
	})

	cfg := &config.ServerConfig{}
	cfg.Tools.Enabled = false

	c, err := controller.NewController(context.Background(), controller.ControllerOptions{
		Config:       cfg,
		Store:        suite.store,
		Janitor:      janitor.NewJanitor(config.Janitor{}),
		OpenAIClient: suite.openAiClient,
		Filestore:    filestoreMock,
		Extractor:    extractorMock,
	})
	suite.NoError(err)

	suite.server = &HelixAPIServer{
		pubsub:     suite.pubsub,
		Controller: c,
	}
}

func (suite *OpenAIChatSuite) TestChatCompletions_Basic_Blocking() {

	req, err := http.NewRequest("POST", "/v1/chat/completions", bytes.NewBufferString(`{
		"model": "meta-llama/Meta-Llama-3.1-8B-Instruct-Turbo",
		"stream": false,
		"messages": [
			{
				"role": "system",
				"content": "You are a helpful assistant."
			},
			{
				"role": "user",
				"content": "tell me about oceans!"
			}
		]
	}`))
	suite.NoError(err)

	req = req.WithContext(suite.authCtx)

	rec := httptest.NewRecorder()

	suite.openAiClient.EXPECT().CreateChatCompletion(gomock.Any(), gomock.Any()).
		DoAndReturn(func(ctx context.Context, req oai.ChatCompletionRequest) (oai.ChatCompletionResponse, error) {
			suite.Equal("meta-llama/Meta-Llama-3.1-8B-Instruct-Turbo", req.Model)

			suite.Require().Equal(2, len(req.Messages))

			suite.Equal("system", req.Messages[0].Role)
			suite.Equal("user", req.Messages[1].Role)

			return oai.ChatCompletionResponse{
				Model: "meta-llama/Meta-Llama-3.1-8B-Instruct-Turbo",
				Choices: []oai.ChatCompletionChoice{
					{
						Message: oai.ChatCompletionMessage{
							Role:    "assistant",
							Content: "**model-result**",
						},
						FinishReason: "stop",
					},
				},
			}, nil
		})

	// Begin the chat
	suite.server.createChatCompletion(rec, req)

	suite.Equal(http.StatusOK, rec.Code)

	var resp oai.ChatCompletionResponse
	err = json.Unmarshal(rec.Body.Bytes(), &resp)
	suite.NoError(err)

	suite.Equal("meta-llama/Meta-Llama-3.1-8B-Instruct-Turbo", resp.Model)
	require.Equal(suite.T(), 1, len(resp.Choices), "should contain 1 choice")
	suite.Equal(oai.FinishReasonStop, resp.Choices[0].FinishReason)
	suite.Equal("assistant", resp.Choices[0].Message.Role)
	suite.Equal("**model-result**", resp.Choices[0].Message.Content)
}

func (suite *OpenAIChatSuite) TestChatCompletions_Streaming() {

	req, err := http.NewRequest("POST", "/v1/chat/completions", bytes.NewBufferString(`{
		"model": "mistralai/Mistral-7B-Instruct-v0.1",
		"stream": true,
		"messages": [
			{
				"role": "system",
				"content": "You are a helpful assistant."
			},
			{
				"role": "user",
				"content": "tell me about oceans!"
			}
		]
	}`))
	suite.NoError(err)

	req = req.WithContext(suite.authCtx)

	rec := httptest.NewRecorder()

	// First we check whether user should get the priority
	suite.store.EXPECT().GetUserMeta(gomock.Any(), "user_id").Return(&types.UserMeta{
		Config: types.UserConfig{
			StripeSubscriptionActive: true,
		},
	}, nil)

	var sessionID string

	// Creating the session
	suite.store.EXPECT().CreateSession(gomock.Any(), gomock.Any()).
		DoAndReturn(func(ctx context.Context, session types.Session) (*types.Session, error) {
			suite.Equal("user_id", session.Owner)
			// suite.Equal(types.OwnerTypeUser, session.OwnerType)
			suite.Equal(suite.userID, session.Owner)
			suite.Equal(types.SessionModeInference, session.Mode)
			suite.Equal(types.SessionTypeText, session.Type)
			suite.Equal(types.ModelName("mistralai/Mistral-7B-Instruct-v0.1"), session.ModelName)
			suite.Equal("You are a helpful assistant.", session.Interactions[0].Message)
			suite.Equal("tell me about oceans!", session.Interactions[1].Message)
			suite.Equal(types.CreatorTypeSystem, session.Interactions[0].Creator)
			suite.Equal(types.CreatorTypeUser, session.Interactions[1].Creator)
			suite.NotEmpty(session.ID, "session ID should be set")

			sessionID = session.ID

			modelMessages := []*types.RunnerTaskResponse{
				{
					Message: "msg-1",
				},
				{
					Message: "msg-2",
				},
				{
					Message: "msg-3",
				},
				{
					Done: true,
				},
			}
			// Publish messages
			time.AfterFunc(100*time.Millisecond, func() {
				for _, msg := range modelMessages {
					msg1, err := json.Marshal(&types.WebsocketEvent{
						Type: "worker_task_response",
						Session: &types.Session{
							ID: "session_id",
						},
						WorkerTaskResponse: &types.RunnerTaskResponse{
							Message: msg.Message,
							Done:    msg.Done,
						},
					})
					suite.NoError(err)

					suite.pubsub.Publish(
						context.Background(),
						pubsub.GetSessionQueue("user_id", session.ID),
						msg1)
				}
			})

			return &session, nil
		})

	// Begin the chat
	suite.server.createChatCompletion(rec, req)

	suite.Equal(http.StatusOK, rec.Code)

	suite.T().Logf("session ID: %s", sessionID)

	// validate headers
	suite.Equal("text/event-stream", rec.Header().Get("Content-Type"))
	suite.Equal("no-cache", rec.Header().Get("Cache-Control"))
	suite.Equal("keep-alive", rec.Header().Get("Connection"))
	suite.Equal("chunked", rec.Header().Get("Transfer-Encoding"))

	var (
		startFound = false
		stopFound  = false
		fullResp   string
	)

	// Read chunks
	scanner := bufio.NewScanner(rec.Body)
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "data: ") {
			jsonData := line[6:] // Remove "data: " prefix
			if jsonData == "[DONE]" {
				break
			}

			var data types.OpenAIResponse
			err := json.Unmarshal([]byte(jsonData), &data)
			suite.NoError(err)

			suite.Equal("mistralai/Mistral-7B-Instruct-v0.1", data.Model)
			suite.Equal(1, len(data.Choices))
			// suite.Equal("assistant", data.Choices[0].Delta.Role)
			suite.Equal("chat.completion.chunk", data.Object)

			fullResp = fullResp + data.Choices[0].Delta.Content

			switch data.Choices[0].Delta.Content {
			case "msg-1":
				suite.Equal("msg-1", data.Choices[0].Delta.Content)
			case "msg-2":
				suite.Equal("msg-2", data.Choices[0].Delta.Content)
			case "msg-3":
				suite.Equal("msg-3", data.Choices[0].Delta.Content)
			case "":
				if data.Choices[0].Delta.Content == "" && data.Choices[0].Delta.Role == "assistant" {
					startFound = true
				}

				if data.Choices[0].Delta.Content == "" && data.Choices[0].FinishReason == "stop" {
					stopFound = true
				}
			default:
				suite.Fail("unexpected message")
			}
		}
	}

	suite.T().Log(fullResp)

	suite.True(startFound, "start chunk not found")
	suite.True(stopFound, "stop chunk not found")
}
