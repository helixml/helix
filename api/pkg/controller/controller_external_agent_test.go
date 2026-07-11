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
		SetRequestInteractionMapping: func(requestID, interactionID string) {
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
		CleanupResponseChannel:       func(_ string, _ string) {},
		SetRequestInteractionMapping: func(_, _ string) {},
		SetRequestSessionMapping:     func(_, _ string) {},
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
			WaitForExternalAgentReady:    func(_ context.Context, _ string, _ time.Duration) error { return nil },
			SendCommand:                  func(_ string, _ types.ExternalAgentCommand) error { return nil },
			StoreResponseChannel:         func(_ string, _ string, _ chan string, _ chan bool, _ chan error) {},
			CleanupResponseChannel:       func(_ string, _ string) {},
			SetRequestInteractionMapping: func(_, _ string) {},
			SetRequestSessionMapping:     func(_, _ string) {},
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
		// markExternalAgentInteractionError reloads before writing so it does
		// not clobber streamed content with a stale in-memory object.
		mockStore.EXPECT().
			GetInteraction(gomock.Any(), "interaction-4").
			Return(&types.Interaction{
				ID:        "interaction-4",
				SessionID: "session-4",
				UserID:    "user-4",
				State:     types.InteractionStateWaiting,
			}, nil)
		mockStore.EXPECT().
			UpdateInteraction(gomock.Any(), gomock.Any()).
			DoAndReturn(func(_ context.Context, interaction *types.Interaction) (*types.Interaction, error) {
				assert.Equal(t, types.InteractionStateError, interaction.State)
				assert.Contains(t, interaction.Error, "send failed")
				return interaction, nil
			})

		c.SetExternalAgentHooks(ExternalAgentHooks{
			WaitForExternalAgentReady: func(_ context.Context, _ string, _ time.Duration) error {
				return nil
			},
			SendCommand: func(_ string, _ types.ExternalAgentCommand) error {
				return fmt.Errorf("send failed")
			},
			StoreResponseChannel:         func(_ string, _ string, _ chan string, _ chan bool, _ chan error) {},
			CleanupResponseChannel:       func(_ string, _ string) {},
			SetRequestInteractionMapping: func(_, _ string) {},
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

// TestRunExternalAgentUsesInteractionIDAsRequestID pins the request_id
// convention so completion events and waiter channels share one key.
func TestRunExternalAgentUsesInteractionIDAsRequestID(t *testing.T) {
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
		ID: "session-reqid",
		Metadata: types.SessionMetadata{
			ZedThreadID: "thread-reqid",
		},
		Interactions: []*types.Interaction{
			{ID: "int_fixed_id", SessionID: "session-reqid", UserID: "user-1"},
		},
	}

	mockExecutor.EXPECT().
		GetSession("session-reqid").
		Return(&external_agent.ZedSession{SessionID: "session-reqid", Status: "ready"}, nil)
	mockStore.EXPECT().
		GetInteraction(gomock.Any(), "int_fixed_id").
		Return(&types.Interaction{
			ID:              "int_fixed_id",
			SessionID:       "session-reqid",
			ResponseMessage: "ok",
			State:           types.InteractionStateComplete,
		}, nil)

	var sentRequestID string
	var storedRequestID string
	var mappedRequestID string

	c.SetExternalAgentHooks(ExternalAgentHooks{
		WaitForExternalAgentReady: func(_ context.Context, _ string, _ time.Duration) error { return nil },
		GetAgentNameForSession:    func(_ context.Context, _ *types.Session) string { return "zed-agent" },
		SendCommand: func(_ string, command types.ExternalAgentCommand) error {
			sentRequestID, _ = command.Data["request_id"].(string)
			go func() {
				// done under the same id the waiter registered
			}()
			return nil
		},
		StoreResponseChannel: func(_ string, requestID string, _ chan string, doneChan chan bool, _ chan error) {
			storedRequestID = requestID
			go func() { doneChan <- true }()
		},
		CleanupResponseChannel:       func(_ string, _ string) {},
		SetRequestInteractionMapping: func(requestID, _ string) { mappedRequestID = requestID },
		SetRequestSessionMapping:     func(_, _ string) {},
	})

	result, err := c.RunExternalAgent(context.Background(), RunExternalAgentRequest{
		Session: session,
		ChatCompletionRequest: openai.ChatCompletionRequest{
			Messages: []openai.ChatCompletionMessage{
				{Role: openai.ChatMessageRoleUser, Content: "hi"},
			},
		},
		Mode:  ExternalAgentModeBlocking,
		Start: time.Now(),
	})
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, "int_fixed_id", result.RequestID)
	assert.Equal(t, "int_fixed_id", sentRequestID)
	assert.Equal(t, "int_fixed_id", storedRequestID)
	assert.Equal(t, "int_fixed_id", mappedRequestID)
	assert.Equal(t, "ok", result.FullResponse)
}

// TestWaitTimeoutPreservesStreamedContent is the ses_01kx8knjxsa8rap7fxpe1bzafs
// regression: the agent finished and streamed content into the DB, but the
// waiter timed out with a stale empty interaction and wiped the reply.
func TestWaitTimeoutPreservesStreamedContent(t *testing.T) {
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
		ID: "session-timeout",
		Metadata: types.SessionMetadata{
			ZedThreadID: "thread-timeout",
		},
		Interactions: []*types.Interaction{
			{ID: "int-timeout", SessionID: "session-timeout", UserID: "user-1"},
		},
	}

	mockExecutor.EXPECT().
		GetSession("session-timeout").
		Return(&external_agent.ZedSession{SessionID: "session-timeout", Status: "ready"}, nil)

	// First GetInteraction: timeout path checking for already-complete.
	// Second GetInteraction: markExternalAgentInteractionError reload.
	// Both see Waiting + full streamed content.
	streamed := &types.Interaction{
		ID:              "int-timeout",
		SessionID:       "session-timeout",
		UserID:          "user-1",
		State:           types.InteractionStateWaiting,
		ResponseMessage: "I am the Chief of Staff with full capabilities...",
	}
	mockStore.EXPECT().
		GetInteraction(gomock.Any(), "int-timeout").
		Return(streamed, nil).
		Times(2)

	var written *types.Interaction
	mockStore.EXPECT().
		UpdateInteraction(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, interaction *types.Interaction) (*types.Interaction, error) {
			// Copy so later mutations don't race the assertion.
			cp := *interaction
			written = &cp
			return interaction, nil
		})

	c.SetExternalAgentHooks(ExternalAgentHooks{
		WaitForExternalAgentReady: func(_ context.Context, _ string, _ time.Duration) error { return nil },
		GetAgentNameForSession:    func(_ context.Context, _ *types.Session) string { return "zed-agent" },
		// Never signal done — force the timeout path.
		SendCommand:                  func(_ string, _ types.ExternalAgentCommand) error { return nil },
		StoreResponseChannel:         func(_ string, _ string, _ chan string, _ chan bool, _ chan error) {},
		CleanupResponseChannel:       func(_ string, _ string) {},
		SetRequestInteractionMapping: func(_, _ string) {},
		SetRequestSessionMapping:     func(_, _ string) {},
	})

	_, err := c.RunExternalAgent(context.Background(), RunExternalAgentRequest{
		Session: session,
		ChatCompletionRequest: openai.ChatCompletionRequest{
			Messages: []openai.ChatCompletionMessage{
				{Role: openai.ChatMessageRoleUser, Content: "hello"},
			},
		},
		Mode:            ExternalAgentModeBlocking,
		Start:           time.Now(),
		ResponseTimeout: 50 * time.Millisecond,
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "external agent response timeout")
	require.NotNil(t, written)
	assert.Equal(t, types.InteractionStateError, written.State)
	assert.Equal(t, "External agent response timeout", written.Error)
	// Critical: the streamed reply must survive the error write.
	assert.Equal(t, "I am the Chief of Staff with full capabilities...", written.ResponseMessage)
}

// TestWaitTimeoutAlreadyCompleteIsSuccess: if message_completed finished the
// turn but never poked doneChan, the timeout must not demote complete → error.
func TestWaitTimeoutAlreadyCompleteIsSuccess(t *testing.T) {
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
		ID: "session-done",
		Metadata: types.SessionMetadata{
			ZedThreadID: "thread-done",
		},
		Interactions: []*types.Interaction{
			{ID: "int-done", SessionID: "session-done", UserID: "user-1"},
		},
	}

	mockExecutor.EXPECT().
		GetSession("session-done").
		Return(&external_agent.ZedSession{SessionID: "session-done", Status: "ready"}, nil)

	// Timeout reloads and finds complete — must return success and NOT call UpdateInteraction.
	mockStore.EXPECT().
		GetInteraction(gomock.Any(), "int-done").
		Return(&types.Interaction{
			ID:              "int-done",
			SessionID:       "session-done",
			State:           types.InteractionStateComplete,
			ResponseMessage: "already finished reply",
		}, nil)

	c.SetExternalAgentHooks(ExternalAgentHooks{
		WaitForExternalAgentReady:    func(_ context.Context, _ string, _ time.Duration) error { return nil },
		GetAgentNameForSession:       func(_ context.Context, _ *types.Session) string { return "zed-agent" },
		SendCommand:                  func(_ string, _ types.ExternalAgentCommand) error { return nil },
		StoreResponseChannel:         func(_ string, _ string, _ chan string, _ chan bool, _ chan error) {},
		CleanupResponseChannel:       func(_ string, _ string) {},
		SetRequestInteractionMapping: func(_, _ string) {},
		SetRequestSessionMapping:     func(_, _ string) {},
	})

	result, err := c.RunExternalAgent(context.Background(), RunExternalAgentRequest{
		Session: session,
		ChatCompletionRequest: openai.ChatCompletionRequest{
			Messages: []openai.ChatCompletionMessage{
				{Role: openai.ChatMessageRoleUser, Content: "hello"},
			},
		},
		Mode:            ExternalAgentModeBlocking,
		Start:           time.Now(),
		ResponseTimeout: 50 * time.Millisecond,
	})
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, "already finished reply", result.FullResponse)
}

// TestAgentErrorPreservesStreamedContent: errorChan path must not wipe content.
func TestAgentErrorPreservesStreamedContent(t *testing.T) {
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
		ID: "session-err",
		Metadata: types.SessionMetadata{
			ZedThreadID: "thread-err",
		},
		Interactions: []*types.Interaction{
			{ID: "int-err", SessionID: "session-err", UserID: "user-1"},
		},
	}

	mockExecutor.EXPECT().
		GetSession("session-err").
		Return(&external_agent.ZedSession{SessionID: "session-err", Status: "ready"}, nil)

	mockStore.EXPECT().
		GetInteraction(gomock.Any(), "int-err").
		Return(&types.Interaction{
			ID:              "int-err",
			SessionID:       "session-err",
			State:           types.InteractionStateWaiting,
			ResponseMessage: "partial stream before crash",
		}, nil)

	var written *types.Interaction
	mockStore.EXPECT().
		UpdateInteraction(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, interaction *types.Interaction) (*types.Interaction, error) {
			cp := *interaction
			written = &cp
			return interaction, nil
		})

	c.SetExternalAgentHooks(ExternalAgentHooks{
		WaitForExternalAgentReady: func(_ context.Context, _ string, _ time.Duration) error { return nil },
		GetAgentNameForSession:    func(_ context.Context, _ *types.Session) string { return "zed-agent" },
		SendCommand:               func(_ string, _ types.ExternalAgentCommand) error { return nil },
		StoreResponseChannel: func(_ string, _ string, _ chan string, _ chan bool, errorChan chan error) {
			go func() {
				time.Sleep(10 * time.Millisecond)
				errorChan <- fmt.Errorf("agent process exited")
			}()
		},
		CleanupResponseChannel:       func(_ string, _ string) {},
		SetRequestInteractionMapping: func(_, _ string) {},
		SetRequestSessionMapping:     func(_, _ string) {},
	})

	_, err := c.RunExternalAgent(context.Background(), RunExternalAgentRequest{
		Session: session,
		ChatCompletionRequest: openai.ChatCompletionRequest{
			Messages: []openai.ChatCompletionMessage{
				{Role: openai.ChatMessageRoleUser, Content: "hello"},
			},
		},
		Mode:            ExternalAgentModeBlocking,
		Start:           time.Now(),
		ResponseTimeout: 2 * time.Second,
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "agent process exited")
	require.NotNil(t, written)
	assert.Equal(t, types.InteractionStateError, written.State)
	assert.Equal(t, "partial stream before crash", written.ResponseMessage)
	assert.Contains(t, written.Error, "agent process exited")
}

// TestMarkErrorDoesNotDemoteComplete ensures markExternalAgentInteractionError
// is a no-op write when the row is already complete.
func TestMarkErrorDoesNotDemoteComplete(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockStore := store.NewMockStore(ctrl)
	c := &Controller{
		Options: Options{Store: mockStore},
	}

	session := &types.Session{ID: "ses"}
	interaction := &types.Interaction{
		ID:              "int",
		SessionID:       "ses",
		ResponseMessage: "", // stale waiter copy
	}

	mockStore.EXPECT().
		GetInteraction(gomock.Any(), "int").
		Return(&types.Interaction{
			ID:              "int",
			SessionID:       "ses",
			State:           types.InteractionStateComplete,
			ResponseMessage: "full reply that must stay",
		}, nil)
	// No UpdateInteraction — demotion is skipped.

	c.markExternalAgentInteractionError(session, interaction, time.Now(), "External agent response timeout", "")
	assert.Equal(t, types.InteractionStateComplete, interaction.State)
	assert.Equal(t, "full reply that must stay", interaction.ResponseMessage)
	assert.Empty(t, interaction.Error)
}
