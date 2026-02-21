package controller

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	external_agent "github.com/helixml/helix/api/pkg/external-agent"
	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/sashabaranov/go-openai"
)

type testExternalAgentChannels struct {
	response chan string
	done     chan bool
	err      chan error
}

func TestRunExternalAgentBlocking(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockStore := store.NewMockStore(ctrl)
	mockExecutor := external_agent.NewMockExecutor(ctrl)
	c := &Controller{
		Options: Options{
			Store:                 mockStore,
			ExternalAgentExecutor: mockExecutor,
		},
	}

	session := &types.Session{
		ID: "session-1",
		Metadata: types.SessionMetadata{
			ZedThreadID: "thread-1",
		},
		Interactions: []*types.Interaction{
			{
				ID:        "interaction-1",
				SessionID: "session-1",
				UserID:    "user-1",
			},
		},
	}

	mockExecutor.EXPECT().
		GetSession("session-1").
		Return(&external_agent.ZedSession{SessionID: "session-1", Status: "ready"}, nil)
	mockStore.EXPECT().
		GetInteraction(gomock.Any(), "interaction-1").
		Return(&types.Interaction{
			ID:              "interaction-1",
			SessionID:       "session-1",
			UserID:          "user-1",
			ResponseMessage: "full response from db",
		}, nil)
	mockStore.EXPECT().
		UpdateInteraction(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, interaction *types.Interaction) (*types.Interaction, error) {
			return interaction, nil
		})

	mu := &sync.Mutex{}
	channelsByRequestID := map[string]testExternalAgentChannels{}
	cleanupCalled := false
	var waitingSessionID string
	var waitingInteractionID string
	var mappedRequestID string
	var mappedSessionID string

	c.SetExternalAgentHooks(ExternalAgentHooks{
		WaitForExternalAgentReady: func(_ context.Context, _ string, _ time.Duration) error {
			return nil
		},
		GetAgentNameForSession: func(_ context.Context, _ *types.Session) string {
			return "zed-agent"
		},
		SendCommand: func(_ string, command types.ExternalAgentCommand) error {
			requestID, ok := command.Data["request_id"].(string)
			require.True(t, ok)
			mu.Lock()
			requestChannels := channelsByRequestID[requestID]
			mu.Unlock()
			go func() {
				requestChannels.response <- "stream chunk"
				requestChannels.done <- true
			}()
			return nil
		},
		StoreResponseChannel: func(_ string, requestID string, responseChan chan string, doneChan chan bool, errorChan chan error) {
			mu.Lock()
			channelsByRequestID[requestID] = testExternalAgentChannels{
				response: responseChan,
				done:     doneChan,
				err:      errorChan,
			}
			mu.Unlock()
		},
		CleanupResponseChannel: func(_ string, _ string) {
			cleanupCalled = true
		},
		SetWaitingInteraction: func(sessionID, interactionID string) {
			waitingSessionID = sessionID
			waitingInteractionID = interactionID
		},
		SetRequestSessionMapping: func(requestID, sessionID string) {
			mappedRequestID = requestID
			mappedSessionID = sessionID
		},
	})

	result, err := c.RunExternalAgent(context.Background(), RunExternalAgentRequest{
		Session: session,
		ChatCompletionRequest: openai.ChatCompletionRequest{
			Messages: []openai.ChatCompletionMessage{
				{
					Role:    openai.ChatMessageRoleUser,
					Content: "hello external agent",
				},
			},
		},
		Mode:  ExternalAgentModeBlocking,
		Start: time.Now(),
	})
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, "full response from db", result.FullResponse)
	assert.NotEmpty(t, result.RequestID)
	assert.Equal(t, "session-1", waitingSessionID)
	assert.Equal(t, "interaction-1", waitingInteractionID)
	assert.Equal(t, "session-1", mappedSessionID)
	assert.Equal(t, result.RequestID, mappedRequestID)
	assert.True(t, cleanupCalled)
}

func TestRunExternalAgentStreaming(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockStore := store.NewMockStore(ctrl)
	mockExecutor := external_agent.NewMockExecutor(ctrl)
	c := &Controller{
		Options: Options{
			Store:                 mockStore,
			ExternalAgentExecutor: mockExecutor,
		},
	}

	session := &types.Session{
		ID: "session-2",
		Metadata: types.SessionMetadata{
			ZedThreadID: "thread-2",
		},
		Interactions: []*types.Interaction{
			{
				ID:        "interaction-2",
				SessionID: "session-2",
				UserID:    "user-2",
			},
		},
	}

	mockExecutor.EXPECT().
		GetSession("session-2").
		Return(&external_agent.ZedSession{SessionID: "session-2", Status: "ready"}, nil)
	mockStore.EXPECT().
		GetInteraction(gomock.Any(), "interaction-2").
		Return(&types.Interaction{
			ID:              "interaction-2",
			SessionID:       "session-2",
			UserID:          "user-2",
			ResponseMessage: "stream final response from db",
		}, nil)
	mockStore.EXPECT().
		UpdateInteraction(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, interaction *types.Interaction) (*types.Interaction, error) {
			return interaction, nil
		})

	mu := &sync.Mutex{}
	channelsByRequestID := map[string]testExternalAgentChannels{}

	c.SetExternalAgentHooks(ExternalAgentHooks{
		WaitForExternalAgentReady: func(_ context.Context, _ string, _ time.Duration) error {
			return nil
		},
		GetAgentNameForSession: func(_ context.Context, _ *types.Session) string {
			return "qwen"
		},
		SendCommand: func(_ string, command types.ExternalAgentCommand) error {
			requestID, ok := command.Data["request_id"].(string)
			require.True(t, ok)
			mu.Lock()
			requestChannels := channelsByRequestID[requestID]
			mu.Unlock()
			go func() {
				requestChannels.response <- "chunk-1"
				requestChannels.done <- true
			}()
			return nil
		},
		StoreResponseChannel: func(_ string, requestID string, responseChan chan string, doneChan chan bool, errorChan chan error) {
			mu.Lock()
			channelsByRequestID[requestID] = testExternalAgentChannels{
				response: responseChan,
				done:     doneChan,
				err:      errorChan,
			}
			mu.Unlock()
		},
		CleanupResponseChannel:   func(_ string, _ string) {},
		SetWaitingInteraction:    func(_, _ string) {},
		SetRequestSessionMapping: func(_, _ string) {},
	})

	result, err := c.RunExternalAgent(context.Background(), RunExternalAgentRequest{
		Session: session,
		ChatCompletionRequest: openai.ChatCompletionRequest{
			Messages: []openai.ChatCompletionMessage{
				{
					Role: openai.ChatMessageRoleUser,
					MultiContent: []openai.ChatMessagePart{
						{Type: openai.ChatMessagePartTypeText, Text: "hello "},
						{Type: openai.ChatMessagePartTypeText, Text: "stream"},
					},
				},
			},
		},
		Mode:  ExternalAgentModeStreaming,
		Start: time.Now(),
	})
	require.NoError(t, err)
	require.NotNil(t, result)
	require.NotNil(t, result.Stream)

	select {
	case chunk := <-result.Stream.Chunks:
		assert.Equal(t, "chunk-1", chunk)
	case err := <-result.Stream.Errors:
		require.NoError(t, err)
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for chunk")
	}

	select {
	case <-result.Stream.Done:
	case err := <-result.Stream.Errors:
		require.NoError(t, err)
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for stream done")
	}
}

func TestRunExternalAgentErrorPaths(t *testing.T) {
	t.Run("missing user message", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		mockStore := store.NewMockStore(ctrl)
		mockExecutor := external_agent.NewMockExecutor(ctrl)
		c := &Controller{
			Options: Options{
				Store:                 mockStore,
				ExternalAgentExecutor: mockExecutor,
			},
		}

		session := &types.Session{
			ID: "session-3",
			Interactions: []*types.Interaction{
				{ID: "interaction-3", SessionID: "session-3", UserID: "user-3"},
			},
		}

		mockExecutor.EXPECT().
			GetSession("session-3").
			Return(&external_agent.ZedSession{SessionID: "session-3", Status: "ready"}, nil)

		c.SetExternalAgentHooks(ExternalAgentHooks{
			WaitForExternalAgentReady: func(_ context.Context, _ string, _ time.Duration) error { return nil },
			SendCommand:               func(_ string, _ types.ExternalAgentCommand) error { return nil },
			StoreResponseChannel:      func(_ string, _ string, _ chan string, _ chan bool, _ chan error) {},
			CleanupResponseChannel:    func(_ string, _ string) {},
			SetWaitingInteraction:     func(_, _ string) {},
			SetRequestSessionMapping:  func(_, _ string) {},
		})

		_, err := c.RunExternalAgent(context.Background(), RunExternalAgentRequest{
			Session: session,
			ChatCompletionRequest: openai.ChatCompletionRequest{
				Messages: []openai.ChatCompletionMessage{
					{Role: openai.ChatMessageRoleAssistant, Content: "not a user prompt"},
				},
			},
		})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "no user message found")
	})

	t.Run("send command failure updates interaction", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		mockStore := store.NewMockStore(ctrl)
		mockExecutor := external_agent.NewMockExecutor(ctrl)
		c := &Controller{
			Options: Options{
				Store:                 mockStore,
				ExternalAgentExecutor: mockExecutor,
			},
		}

		session := &types.Session{
			ID: "session-4",
			Interactions: []*types.Interaction{
				{ID: "interaction-4", SessionID: "session-4", UserID: "user-4"},
			},
		}

		mockExecutor.EXPECT().
			GetSession("session-4").
			Return(&external_agent.ZedSession{SessionID: "session-4", Status: "ready"}, nil)
		mockStore.EXPECT().
			UpdateInteraction(gomock.Any(), gomock.Any()).
			DoAndReturn(func(_ context.Context, interaction *types.Interaction) (*types.Interaction, error) {
				return interaction, nil
			})

		c.SetExternalAgentHooks(ExternalAgentHooks{
			WaitForExternalAgentReady: func(_ context.Context, _ string, _ time.Duration) error {
				return nil
			},
			SendCommand: func(_ string, _ types.ExternalAgentCommand) error {
				return fmt.Errorf("send failed")
			},
			StoreResponseChannel:   func(_ string, _ string, _ chan string, _ chan bool, _ chan error) {},
			CleanupResponseChannel: func(_ string, _ string) {},
			SetWaitingInteraction:  func(_, _ string) {},
			SetRequestSessionMapping: func(_, _ string) {
			},
		})

		_, err := c.RunExternalAgent(context.Background(), RunExternalAgentRequest{
			Session: session,
			ChatCompletionRequest: openai.ChatCompletionRequest{
				Messages: []openai.ChatCompletionMessage{
					{Role: openai.ChatMessageRoleUser, Content: "hi"},
				},
			},
		})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to send command to external agent")
	})
}
