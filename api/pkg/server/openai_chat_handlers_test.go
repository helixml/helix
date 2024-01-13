package server

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/suite"

	"github.com/lukemarsden/helix/api/pkg/controller"
	"github.com/lukemarsden/helix/api/pkg/janitor"
	"github.com/lukemarsden/helix/api/pkg/pubsub"
	"github.com/lukemarsden/helix/api/pkg/store"
	"github.com/lukemarsden/helix/api/pkg/types"
)

func TestOpenAIChatSuite(t *testing.T) {
	suite.Run(t, new(OpenAIChatSuite))
}

type OpenAIChatSuite struct {
	suite.Suite

	store  *store.MockStore
	pubsub pubsub.PubSub

	authCtx context.Context
	userID  string

	server *HelixAPIServer
}

func (suite *OpenAIChatSuite) SetupTest() {
	ctrl := gomock.NewController(suite.T())

	suite.store = store.NewMockStore(ctrl)
	suite.pubsub = pubsub.New()

	suite.userID = "user_id"
	suite.authCtx = setRequestUser(context.Background(), types.UserData{
		ID:       suite.userID,
		Email:    "foo@email.com",
		FullName: "Foo Bar",
	})

	suite.server = &HelixAPIServer{
		pubsub: suite.pubsub,
		Controller: &controller.Controller{
			Options: controller.ControllerOptions{
				Store:   suite.store,
				Janitor: janitor.NewJanitor(janitor.JanitorOptions{}),
			},
		},
		adminAuth: &adminAuth{},
	}
}

func (suite *OpenAIChatSuite) TestChatCompletions_Blocking() {

	req, err := http.NewRequest("POST", "/v1/chat/completions", bytes.NewBufferString(`{
		"model": "mistralai/Mistral-7B-Instruct-v0.1",
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

	// First we check whether user should get the priority
	suite.store.EXPECT().GetBalanceTransfers(gomock.Any(), store.OwnerQuery{
		Owner:     "user_id",
		OwnerType: types.OwnerTypeUser,
	}).Return([]*types.BalanceTransfer{}, nil)

	suite.store.EXPECT().GetUserMeta(gomock.Any(), "user_id").Return(&types.UserMeta{
		Config: types.UserConfig{
			StripeSubscriptionActive: true,
		},
	}, nil)

	// Creating the session
	suite.store.EXPECT().CreateSession(gomock.Any(), gomock.Any()).
		DoAndReturn(func(ctx context.Context, session types.Session) (*types.Session, error) {
			suite.Equal("user_id", session.Owner)
			suite.Equal(types.OwnerTypeUser, session.OwnerType)
			suite.Equal(suite.userID, session.Owner)
			suite.Equal(types.SessionModeInference, session.Mode)
			suite.Equal(types.SessionTypeText, session.Type)
			suite.Equal(types.ModelName("mistralai/Mistral-7B-Instruct-v0.1"), session.ModelName)
			suite.Equal("You are a helpful assistant.", session.Interactions[0].Messages[0].Content)
			suite.Equal("tell me about oceans!", session.Interactions[0].Messages[1].Content)
			suite.Equal("system", session.Interactions[0].Messages[0].Role)
			suite.Equal("user", session.Interactions[0].Messages[1].Role)
			suite.NotEmpty(session.ID, "session ID should be set")

			bts, err := json.Marshal(&types.WebsocketEvent{
				Type: "session_update",
				Session: &types.Session{
					ID: "session_id",
					Interactions: []types.Interaction{
						{
							State:   types.InteractionStateComplete,
							Message: "**model-result**",
						},
					},
				}})
			suite.NoError(err)

			suite.pubsub.Publish(
				context.Background(),
				session.ID,
				bts, pubsub.WithPublishNamespace("user_id"))

			return &session, nil
		})

	// Begin the chat
	suite.server.createChatCompletion(rec, req)

	suite.Equal(http.StatusOK, rec.Code)

	var resp types.OpenAIResponse
	err = json.Unmarshal(rec.Body.Bytes(), &resp)
	suite.NoError(err)

	suite.Equal("mistralai/Mistral-7B-Instruct-v0.1", resp.Model)
	suite.Equal(1, len(resp.Choices))
	suite.Equal("stop", resp.Choices[0].FinishReason)
	suite.Equal("assistant", resp.Choices[0].Message.Role)
	suite.Equal("**model-result**", resp.Choices[0].Message.Content)
}
