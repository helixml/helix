package server

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gorilla/mux"
	external_agent "github.com/helixml/helix/api/pkg/external-agent"
	"github.com/helixml/helix/api/pkg/store/memorystore"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

func seedCodingAgent(mem *memorystore.MemoryStore, id, provider, model string) {
	mem.SeedApp(&types.App{
		ID:        id,
		Owner:     "user_a",
		AgentKind: types.AgentKindCoding,
		Config: types.AppConfig{Helix: types.AppHelixConfig{
			Name: id,
			Assistants: []types.AssistantConfig{{
				AgentType:               types.AgentTypeZedExternal,
				CodeAgentRuntime:        types.CodeAgentRuntimeZedAgent,
				GenerationModelProvider: provider,
				GenerationModel:         model,
			}},
		}},
	})
}

// newOrgChatSession is an org bot / project chat session: an external coding
// agent with no SpecTask behind it, so it owns its own configuration.
func newOrgChatSession(owner string) *types.Session {
	session := newTestParentSession(owner)
	session.Metadata.SpecTaskID = ""
	session.Metadata.SessionRole = "exploratory"
	session.Metadata.CodeAgentRuntime = types.CodeAgentRuntimeZedAgent
	session.Metadata.ZedAgentName = types.CodeAgentRuntimeZedAgent.ZedAgentName()
	return session
}

func callGetSessionExecutionConfig(t *testing.T, srv *HelixAPIServer, user types.User, sessionID string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/sessions/"+sessionID+"/execution-config", http.NoBody)
	req = mux.SetURLVars(req, map[string]string{"id": sessionID})
	req = req.WithContext(setRequestUser(req.Context(), user))
	rr := httptest.NewRecorder()
	srv.getSessionExecutionConfig(rr, req)
	return rr
}

func callUpdateSessionExecutionConfig(
	t *testing.T,
	srv *HelixAPIServer,
	user types.User,
	sessionID string,
	body types.SessionExecutionConfigUpdateRequest,
) *httptest.ResponseRecorder {
	t.Helper()
	raw, err := json.Marshal(body)
	require.NoError(t, err)
	req := httptest.NewRequest(http.MethodPatch, "/api/v1/sessions/"+sessionID+"/execution-config", bytes.NewReader(raw))
	req = mux.SetURLVars(req, map[string]string{"id": sessionID})
	req = req.WithContext(setRequestUser(req.Context(), user))
	rr := httptest.NewRecorder()
	srv.updateSessionExecutionConfig(rr, req)
	return rr
}

func TestGetSessionExecutionConfigReportsSessionOverrides(t *testing.T) {
	srv, mem := newForkTestServer(t)
	seedCodingAgent(mem, "app_parent", "anthropic", "claude-opus-4-7")
	session := newOrgChatSession("user_a")
	session.Metadata.CodeAgentOverrides = &types.CodeAgentOverrides{
		ProviderRef:     "openai",
		Model:           "gpt-5",
		ReasoningEffort: "high",
	}
	seedParentWithInteractions(t, mem, session, 1)

	rr := callGetSessionExecutionConfig(t, srv, types.User{ID: "user_a"}, session.ID)
	require.Equal(t, http.StatusOK, rr.Code, rr.Body.String())

	var config types.AgentExecutionConfig
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &config))
	assert.True(t, config.AgentAvailable)
	assert.Equal(t, "app_parent", config.AgentID)
	// The session's overrides win over the agent's own model selection.
	assert.Equal(t, "openai", config.ProviderRef)
	assert.Equal(t, "gpt-5", config.Model)
	assert.Equal(t, "high", config.ReasoningEffort)
	// ...and are echoed so the composer can round-trip an edit.
	require.NotNil(t, config.CodeAgentOverrides)
	assert.Equal(t, "gpt-5", config.CodeAgentOverrides.Model)
}

func TestGetSessionExecutionConfigReportsOwnedConfig(t *testing.T) {
	srv, mem := newForkTestServer(t)
	seedCodingAgent(mem, "app_parent", "anthropic", "claude-opus-4-7")
	session := newOrgChatSession("user_a")
	session.Metadata.CodeAgentConfig = &types.CodeAgentExecutionConfig{
		Runtime:        types.CodeAgentRuntimeOpenCode,
		CredentialType: types.CodeAgentCredentialTypeAPIKey,
		ProviderRef:    "anthropic",
		Model:          "claude-sonnet-4-7",
	}
	seedParentWithInteractions(t, mem, session, 1)

	rr := callGetSessionExecutionConfig(t, srv, types.User{ID: "user_a"}, session.ID)
	require.Equal(t, http.StatusOK, rr.Code, rr.Body.String())

	var config types.AgentExecutionConfig
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &config))
	assert.Equal(t, "app_parent", config.AgentID)
	assert.Equal(t, types.CodeAgentRuntimeOpenCode, config.Runtime)
	require.NotNil(t, config.CodeAgentConfig)
	assert.Equal(t, "claude-sonnet-4-7", config.CodeAgentConfig.Model)
}

func TestUpdateSessionExecutionConfigPersistsCompleteConfigAndKeepsAgent(t *testing.T) {
	srv, mem := newForkTestServer(t)
	ctx := context.Background()
	seedCodingAgent(mem, "app_parent", "anthropic", "claude-opus-4-7")
	session := newOrgChatSession("user_a")
	session.Metadata.CodeAgentOverrides = &types.CodeAgentOverrides{ReasoningEffort: "low"}
	seedParentWithInteractions(t, mem, session, 1)

	config := &types.CodeAgentExecutionConfig{
		Runtime:         types.CodeAgentRuntimeOpenCode,
		CredentialType:  types.CodeAgentCredentialTypeAPIKey,
		ProviderRef:     "anthropic",
		Model:           "claude-sonnet-4-7",
		ReasoningEffort: "high",
	}
	rr := callUpdateSessionExecutionConfig(t, srv, types.User{ID: "user_a"}, session.ID,
		types.SessionExecutionConfigUpdateRequest{CodeAgentConfig: config})
	require.Equal(t, http.StatusOK, rr.Code, rr.Body.String())

	updated, err := mem.GetSession(ctx, session.ID)
	require.NoError(t, err)
	assert.Equal(t, "app_parent", updated.ParentApp, "the Helix Agent still owns instructions and tools")
	assert.Equal(t, config, updated.Metadata.CodeAgentConfig)
	assert.Nil(t, updated.Metadata.CodeAgentOverrides)
	assert.Equal(t, types.CodeAgentRuntimeOpenCode, updated.Metadata.CodeAgentRuntime)
}

// A SpecTask session reports its TASK's configuration, not the session row's:
// the task is the single source of truth for the sessions it drives.
func TestGetSessionExecutionConfigPrefersSpecTask(t *testing.T) {
	srv, mem := newForkTestServer(t)
	seedCodingAgent(mem, "app_parent", "anthropic", "claude-opus-4-7")
	seedCodingAgent(mem, "app_task", "anthropic", "claude-sonnet-4-7")
	session := newTestParentSession("user_a")
	session.Metadata.SpecTaskID = "task_test"
	seedParentWithInteractions(t, mem, session, 1)
	taskConfig := &types.CodeAgentExecutionConfig{
		Runtime:        types.CodeAgentRuntimeClaudeCode,
		CredentialType: types.CodeAgentCredentialTypeSubscription,
		Model:          "claude-haiku-4-5",
	}
	mem.SeedSpecTask(&types.SpecTask{
		ID:                session.Metadata.SpecTaskID,
		PlanningSessionID: session.ID,
		CodeAgentConfig:   taskConfig,
	})

	rr := callGetSessionExecutionConfig(t, srv, types.User{ID: "user_a"}, session.ID)
	require.Equal(t, http.StatusOK, rr.Code, rr.Body.String())

	var config types.AgentExecutionConfig
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &config))
	assert.Empty(t, config.AgentID)
	assert.Equal(t, "claude-haiku-4-5", config.Model)
	require.Equal(t, taskConfig, config.CodeAgentConfig)
}

func TestUpdateSessionExecutionConfigPersistsOverridesAndRestartsThread(t *testing.T) {
	srv, mem := newForkTestServer(t)
	ctrl := gomock.NewController(t)
	executor := external_agent.NewMockExecutor(ctrl)
	srv.externalAgentExecutor = executor
	ctx := context.Background()
	seedCodingAgent(mem, "app_parent", "anthropic", "claude-opus-4-7")
	session := newOrgChatSession("user_a")
	session.Metadata.ZedThreadID = "ctx_old_thread"
	seedParentWithInteractions(t, mem, session, 2)
	executor.EXPECT().HasRunningContainer(gomock.Any(), session.ID).Return(true)

	rr := callUpdateSessionExecutionConfig(t, srv, types.User{ID: "user_a"}, session.ID,
		types.SessionExecutionConfigUpdateRequest{
			CodeAgentOverrides: &types.CodeAgentOverrides{
				ProviderRef:     "anthropic",
				Model:           "claude-sonnet-4-7",
				ReasoningEffort: "low",
			},
		})
	require.Equal(t, http.StatusOK, rr.Code, rr.Body.String())

	var response types.SessionExecutionConfigUpdateResponse
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &response))
	assert.True(t, response.AgentThreadRestarted)
	assert.Empty(t, response.SpecTaskID)

	updated, err := mem.GetSession(ctx, session.ID)
	require.NoError(t, err)
	require.NotNil(t, updated.Metadata.CodeAgentOverrides)
	assert.Equal(t, "claude-sonnet-4-7", updated.Metadata.CodeAgentOverrides.Model)
	assert.Equal(t, "low", updated.Metadata.CodeAgentOverrides.ReasoningEffort)
	// The next turn must open a NEW Zed thread on the new configuration.
	assert.Empty(t, updated.Metadata.ZedThreadID)
	assert.False(t, updated.Metadata.AgentSwitchedAt.IsZero())
}

func TestUpdateSessionExecutionConfigWhileStoppedStaysIdleAndSeedsNextTurn(t *testing.T) {
	srv, mem := newForkTestServer(t)
	ctrl := gomock.NewController(t)
	executor := external_agent.NewMockExecutor(ctrl)
	srv.externalAgentExecutor = executor
	ctx := context.Background()
	seedCodingAgent(mem, "app_parent", "anthropic", "claude-opus-4-7")
	session := newOrgChatSession("user_a")
	session.Metadata.ZedThreadID = "ctx_stopped_thread"
	session.Metadata.ExternalAgentStatus = "stopped"
	seedParentWithInteractions(t, mem, session, 2)
	staleHandoff, err := mem.CreateInteraction(ctx, &types.Interaction{
		Created:       time.Now(),
		Updated:       time.Now(),
		SessionID:     session.ID,
		UserID:        session.Owner,
		GenerationID:  session.GenerationID,
		Mode:          types.SessionModeInference,
		Trigger:       types.InteractionTriggerForkHandoff,
		State:         types.InteractionStateWaiting,
		PromptMessage: "stale offline handoff",
	})
	require.NoError(t, err)
	executor.EXPECT().HasRunningContainer(gomock.Any(), session.ID).Return(false)

	rr := callUpdateSessionExecutionConfig(t, srv, types.User{ID: "user_a"}, session.ID,
		types.SessionExecutionConfigUpdateRequest{
			CodeAgentOverrides: &types.CodeAgentOverrides{
				ProviderRef: "anthropic",
				Model:       "claude-sonnet-4-7",
			},
		})
	require.Equal(t, http.StatusOK, rr.Code, rr.Body.String())

	var response types.SessionExecutionConfigUpdateResponse
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &response))
	assert.False(t, response.AgentThreadRestarted, "a stopped sandbox must not report a live thread restart")

	updated, err := mem.GetSession(ctx, session.ID)
	require.NoError(t, err)
	assert.Equal(t, "stopped", updated.Metadata.ExternalAgentStatus)
	assert.Empty(t, updated.Metadata.ZedThreadID)
	assert.False(t, updated.Metadata.AgentSwitchedAt.IsZero())
	require.NotNil(t, updated.Metadata.CodeAgentOverrides)
	assert.Equal(t, "claude-sonnet-4-7", updated.Metadata.CodeAgentOverrides.Model)

	interactions, _, err := mem.ListInteractions(ctx, &types.ListInteractionsQuery{
		SessionID: session.ID,
		PerPage:   100,
	})
	require.NoError(t, err)
	var seedCount, waitingCount int
	for _, interaction := range interactions {
		if interaction.Trigger == types.InteractionTriggerForkSeed {
			seedCount++
		}
		if interaction.State == types.InteractionStateWaiting {
			waitingCount++
		}
	}
	assert.Equal(t, 1, seedCount, "the next started thread needs one transcript seed")
	assert.Zero(t, waitingCount, "offline model changes must not make the chat look active")

	interrupted, err := mem.GetInteraction(ctx, staleHandoff.ID)
	require.NoError(t, err)
	assert.Equal(t, types.InteractionStateInterrupted, interrupted.State)
	seededPrompt := srv.maybePrependTranscript(ctx, updated, "first message after start")
	assert.Contains(t, seededPrompt, "agent reply 1")
	assert.Contains(t, seededPrompt, "first message after start")
}

// The session endpoint is the generic one: a SpecTask session writes through to
// its task so the two surfaces cannot drift apart.
func TestUpdateSessionExecutionConfigWritesThroughToSpecTask(t *testing.T) {
	srv, mem := newForkTestServer(t)
	ctx := context.Background()
	seedCodingAgent(mem, "app_parent", "anthropic", "claude-opus-4-7")
	session := newTestParentSession("user_a")
	session.Metadata.SpecTaskID = "task_test"
	session.Metadata.CodeAgentRuntime = types.CodeAgentRuntimeZedAgent
	session.Metadata.ZedAgentName = types.CodeAgentRuntimeZedAgent.ZedAgentName()
	seedParentWithInteractions(t, mem, session, 1)
	mem.SeedSpecTask(&types.SpecTask{
		ID:                session.Metadata.SpecTaskID,
		PlanningSessionID: session.ID,
		CodeAgentConfig: &types.CodeAgentExecutionConfig{
			Runtime:        types.CodeAgentRuntimeZedAgent,
			CredentialType: types.CodeAgentCredentialTypeSubscription,
			Model:          "claude-opus-4-7",
		},
	})

	updatedConfig := &types.CodeAgentExecutionConfig{
		Runtime:         types.CodeAgentRuntimeClaudeCode,
		CredentialType:  types.CodeAgentCredentialTypeSubscription,
		Model:           "claude-sonnet-4-7",
		ReasoningEffort: "high",
	}
	rr := callUpdateSessionExecutionConfig(t, srv, types.User{ID: "user_a"}, session.ID,
		types.SessionExecutionConfigUpdateRequest{
			CodeAgentConfig: updatedConfig,
		})
	require.Equal(t, http.StatusOK, rr.Code, rr.Body.String())

	var response types.SessionExecutionConfigUpdateResponse
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &response))
	assert.Equal(t, session.Metadata.SpecTaskID, response.SpecTaskID)

	task, err := mem.GetSpecTask(ctx, session.Metadata.SpecTaskID)
	require.NoError(t, err)
	assert.Equal(t, updatedConfig, task.CodeAgentConfig)
	assert.Empty(t, task.HelixAppID)
	assert.Nil(t, task.CodeAgentOverrides)

	updated, err := mem.GetSession(ctx, session.ID)
	require.NoError(t, err)
	assert.Empty(t, updated.ParentApp)
	assert.Nil(t, updated.Metadata.CodeAgentOverrides, "task-driven sessions must not carry competing overrides")
}

func TestUpdateSessionExecutionConfigRejectsNonCodingSession(t *testing.T) {
	srv, mem := newForkTestServer(t)
	session := newOrgChatSession("user_a")
	session.Metadata.AgentType = "helix"
	seedParentWithInteractions(t, mem, session, 1)

	rr := callUpdateSessionExecutionConfig(t, srv, types.User{ID: "user_a"}, session.ID,
		types.SessionExecutionConfigUpdateRequest{
			CodeAgentOverrides: &types.CodeAgentOverrides{Model: "gpt-5"},
		})
	require.Equal(t, http.StatusBadRequest, rr.Code)
	assert.Contains(t, rr.Body.String(), "external coding agent")
}

func TestUpdateSessionExecutionConfigRejectsUnsupportedReasoningEffort(t *testing.T) {
	srv, mem := newForkTestServer(t)
	seedCodingAgent(mem, "app_parent", "anthropic", "claude-opus-4-7")
	session := newOrgChatSession("user_a")
	seedParentWithInteractions(t, mem, session, 1)

	rr := callUpdateSessionExecutionConfig(t, srv, types.User{ID: "user_a"}, session.ID,
		types.SessionExecutionConfigUpdateRequest{
			CodeAgentOverrides: &types.CodeAgentOverrides{ReasoningEffort: "turbo"},
		})
	require.Equal(t, http.StatusBadRequest, rr.Code)
	assert.Contains(t, rr.Body.String(), "unsupported reasoning effort")
}

// A no-op PATCH must not restart the agent thread — re-opening the picker and
// re-selecting the current model should cost nothing.
func TestUpdateSessionExecutionConfigNoOpDoesNotRestartThread(t *testing.T) {
	srv, mem := newForkTestServer(t)
	ctx := context.Background()
	seedCodingAgent(mem, "app_parent", "anthropic", "claude-opus-4-7")
	session := newOrgChatSession("user_a")
	session.Metadata.ZedThreadID = "ctx_old_thread"
	session.Metadata.CodeAgentOverrides = &types.CodeAgentOverrides{
		ProviderRef: "anthropic",
		Model:       "claude-sonnet-4-7",
	}
	seedParentWithInteractions(t, mem, session, 1)

	rr := callUpdateSessionExecutionConfig(t, srv, types.User{ID: "user_a"}, session.ID,
		types.SessionExecutionConfigUpdateRequest{
			CodeAgentOverrides: &types.CodeAgentOverrides{
				ProviderRef: "anthropic",
				Model:       "claude-sonnet-4-7",
			},
		})
	require.Equal(t, http.StatusOK, rr.Code, rr.Body.String())

	var response types.SessionExecutionConfigUpdateResponse
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &response))
	assert.False(t, response.AgentThreadRestarted)

	updated, err := mem.GetSession(ctx, session.ID)
	require.NoError(t, err)
	assert.Equal(t, "ctx_old_thread", updated.Metadata.ZedThreadID, "a no-op must leave the live thread alone")
}
