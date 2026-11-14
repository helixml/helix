package server

import (
	"context"
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/helixml/helix/api/pkg/pubsub"
	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/store/memorystore"
	"github.com/helixml/helix/api/pkg/types"
)

// Sessions the agent creates for its own threads (helixml/helix#3617) must
// carry the connection session's owner, org, project, spec task and parent so
// they appear wherever the task's sessions do. These tests drive the real sync
// handlers against the in-memory store and then read back through the same
// listing and interaction handlers a task-confined key uses.

const linkageConnectionID = finderSessionID

func newLinkageServer(t *testing.T) (*HelixAPIServer, *memorystore.MemoryStore) {
	t.Helper()
	ms := memorystore.New()
	ms.SeedProject(confinedProject())
	ms.SeedSpecTask(&types.SpecTask{ID: finderTaskID, ProjectID: confinedProjectID, UserID: confinedOwnerID})
	conn := taskSession(linkageConnectionID, finderTaskID)
	conn.OwnerType = types.OwnerTypeUser
	conn.Metadata.AgentType = string(types.AgentTypeZedExternal)
	_, err := ms.CreateSession(context.Background(), *conn)
	require.NoError(t, err)
	return NewTestServer(ms, pubsub.NewNoop()), ms
}

func requireLinkedToConnection(t *testing.T, ms *memorystore.MemoryStore, sessionID, threadID string) {
	t.Helper()
	got, err := ms.GetSession(context.Background(), sessionID)
	require.NoError(t, err)
	require.Equal(t, confinedOwnerID, got.Owner)
	require.Equal(t, types.OwnerTypeUser, got.OwnerType)
	require.Empty(t, got.OrganizationID, "the connection is a personal project session")
	require.Equal(t, confinedProjectID, got.ProjectID)
	require.Equal(t, confinedProjectID, got.Metadata.ProjectID)
	require.Equal(t, finderTaskID, got.Metadata.SpecTaskID)
	require.Equal(t, linkageConnectionID, got.ParentSession)
	require.Equal(t, linkageConnectionID, got.Metadata.ExternalAgentID)
	require.Equal(t, threadID, got.Metadata.ZedThreadID)

	workSession, err := ms.GetSpecTaskWorkSessionByHelixSession(context.Background(), sessionID)
	require.NoError(t, err, "the thread must be tracked as a work session of the task")
	require.Equal(t, finderTaskID, workSession.SpecTaskID)
}

// requireVisibleToTaskKey lists the task's sessions and the new session's
// turns as the task-confined key of the connection's owner.
func requireVisibleToTaskKey(t *testing.T, srv *HelixAPIServer, sessionID, prompt string) {
	t.Helper()
	list, err := srv.listSessions(nil, confinedRequest(http.MethodGet, "/api/v1/sessions?include_external_agents=true", "", nil))
	require.NoError(t, err)
	var ids []string
	for _, summary := range list.Sessions {
		ids = append(ids, summary.SessionID)
	}
	require.Contains(t, ids, sessionID, "task-scoped listing must include the agent-created session")

	interactions, httpErr := srv.listInteractions(nil, confinedRequest(http.MethodGet, "/api/v1/sessions/"+sessionID+"/interactions", "", map[string]string{"id": sessionID}))
	require.Nil(t, httpErr)
	require.Len(t, interactions.Interactions, 1)
	require.Equal(t, prompt, interactions.Interactions[0].PromptMessage)
}

func TestAgentThreadSession_UserMessageOnUnmappedThreadIsLinked(t *testing.T) {
	srv, ms := newLinkageServer(t)

	require.NoError(t, srv.handleMessageAdded(linkageConnectionID, &types.SyncMessage{
		EventType: "message_added",
		Data: map[string]interface{}{
			"acp_thread_id": "thread-native",
			"message_id":    "0",
			"content":       "typed in Zed",
			"role":          "user",
			"request_id":    "req-native",
			// A forged scope in the event must be ignored.
			"spec_task_id": verifierTaskID,
			"project_id":   "prj_other",
		},
	}))

	sessionID := srv.contextMappings[routeKey(linkageConnectionID, "thread-native")]
	require.NotEmpty(t, sessionID)
	require.NotEqual(t, linkageConnectionID, sessionID)
	requireLinkedToConnection(t, ms, sessionID, "thread-native")
	requireVisibleToTaskKey(t, srv, sessionID, "typed in Zed")

	byConnection, _, err := ms.ListSessions(context.Background(), store.ListSessionsQuery{
		ExternalAgentID: linkageConnectionID, IncludeExternalAgents: true,
	})
	require.NoError(t, err)
	require.Len(t, byConnection, 1, "only the child records the connection id; the connection session is its own")
	require.Equal(t, sessionID, byConnection[0].ID)

	// user_created_thread arriving afterwards must not create a second session.
	require.NoError(t, srv.handleUserCreatedThread(linkageConnectionID, &types.SyncMessage{
		EventType: "user_created_thread",
		Data:      map[string]interface{}{"acp_thread_id": "thread-native", "title": "Native"},
	}))
	byConnection, _, err = ms.ListSessions(context.Background(), store.ListSessionsQuery{ExternalAgentID: linkageConnectionID})
	require.NoError(t, err)
	require.Len(t, byConnection, 1)
}

func TestAgentThreadSession_ThreadCreatedIsLinked(t *testing.T) {
	srv, ms := newLinkageServer(t)

	require.NoError(t, srv.handleThreadCreated(linkageConnectionID, &types.SyncMessage{
		EventType: "thread_created",
		Data: map[string]interface{}{
			"acp_thread_id": "thread-new",
			"title":         "New Conversation",
		},
	}))

	sessionID := srv.contextMappings[routeKey(linkageConnectionID, "thread-new")]
	require.NotEmpty(t, sessionID)
	requireLinkedToConnection(t, ms, sessionID, "thread-new")
	requireVisibleToTaskKey(t, srv, sessionID, "New conversation started via Zed")
}

func TestAgentThreadSession_ThreadCreatedWithoutOwnerIsRefused(t *testing.T) {
	srv, ms := newLinkageServer(t)

	err := srv.handleThreadCreated("ses_unknown_connection", &types.SyncMessage{
		EventType: "thread_created",
		Data:      map[string]interface{}{"acp_thread_id": "thread-orphan"},
	})
	require.Error(t, err)

	all, _, err := ms.ListSessions(context.Background(), store.ListSessionsQuery{AnyOrganization: true, IncludeExternalAgents: true})
	require.NoError(t, err)
	require.Len(t, all, 1, "no ownerless session may be created")
	require.Equal(t, linkageConnectionID, all[0].ID)
}

// Lane A's connection naming lane B's thread id must not resolve lane B's
// session, even with the same owner and project. Thread ids are chosen by the
// harness and can be predictable (Goose: 20261114_1).
func TestFindSessionByZedThreadID_DoesNotReachAnotherLane(t *testing.T) {
	ms := memorystore.New()
	ctx := context.Background()
	for _, session := range []types.Session{
		{ID: "ses_lane_a", Owner: confinedOwnerID, ProjectID: confinedProjectID, Metadata: types.SessionMetadata{ZedThreadID: "20261114_1"}},
		{ID: "ses_lane_b", Owner: confinedOwnerID, ProjectID: confinedProjectID, Metadata: types.SessionMetadata{ZedThreadID: "20261114_2"}},
		{ID: "ses_lane_a_child", Owner: confinedOwnerID, ProjectID: confinedProjectID, Metadata: types.SessionMetadata{ZedThreadID: "20261114_3", ExternalAgentID: "ses_lane_a"}},
	} {
		_, err := ms.CreateSession(ctx, session)
		require.NoError(t, err)
	}
	srv := NewTestServer(ms, pubsub.NewNoop())

	_, err := srv.findSessionByZedThreadID(ctx, "ses_lane_a", "20261114_2")
	require.Error(t, err, "lane B's thread must not resolve through lane A's connection")

	own, err := srv.findSessionByZedThreadID(ctx, "ses_lane_a", "20261114_1")
	require.NoError(t, err)
	require.Equal(t, "ses_lane_a", own.ID)
	child, err := srv.findSessionByZedThreadID(ctx, "ses_lane_a", "20261114_3")
	require.NoError(t, err)
	require.Equal(t, "ses_lane_a_child", child.ID)

	// The routing path that uses it: an assistant event for lane B's thread
	// arriving on lane A's connection after an API restart finds nothing.
	err = srv.handleMessageAdded("ses_lane_a", &types.SyncMessage{
		EventType: "message_added",
		Data: map[string]interface{}{
			"acp_thread_id": "20261114_2", "message_id": "4", "content": "leak", "role": "assistant",
		},
	})
	require.Error(t, err)
}

// Helix binding one of a connection's threads to another session records the
// connection on it, so the thread is still found after an API restart.
func TestThreadCreated_BindingRecordsConnection(t *testing.T) {
	srv, ms := newLinkageServer(t)
	ctx := context.Background()
	_, err := ms.CreateSession(ctx, types.Session{ID: "ses_bound", Owner: confinedOwnerID, ProjectID: confinedProjectID})
	require.NoError(t, err)
	// Helix dispatched to ses_bound over this connection's WebSocket.
	conn := &ExternalAgentWSConnection{SessionID: linkageConnectionID, SendChan: make(chan types.ExternalAgentCommand, 1)}
	srv.externalAgentWSManager.registerConnection(linkageConnectionID, conn)
	srv.externalAgentWSManager.registerConnection("ses_bound", conn)
	srv.requestToSessionMapping["req-bound"] = "ses_bound"

	require.NoError(t, srv.handleThreadCreated(linkageConnectionID, &types.SyncMessage{
		EventType: "thread_created",
		Data:      map[string]interface{}{"acp_thread_id": "thread-bound", "request_id": "req-bound"},
	}))

	bound, err := ms.GetSession(ctx, "ses_bound")
	require.NoError(t, err)
	require.Equal(t, linkageConnectionID, bound.Metadata.ExternalAgentID)

	restarted := NewTestServer(ms, pubsub.NewNoop())
	found, err := restarted.findSessionByZedThreadID(ctx, linkageConnectionID, "thread-bound")
	require.NoError(t, err)
	require.Equal(t, "ses_bound", found.ID)
}

// twoLanes: lane A's sandbox is connected; lane B is the same owner's session
// in another project, served by its own (here: absent) connection.
func twoLanes(t *testing.T) (*HelixAPIServer, *memorystore.MemoryStore) {
	t.Helper()
	ms := memorystore.New()
	ctx := context.Background()
	for _, session := range []types.Session{
		{ID: "ses_lane_a", Owner: confinedOwnerID, OwnerType: types.OwnerTypeUser, ProjectID: "prj_a", Metadata: types.SessionMetadata{ZedThreadID: "thread-a"}},
		{ID: "ses_lane_b", Owner: confinedOwnerID, OwnerType: types.OwnerTypeUser, ProjectID: "prj_b", Metadata: types.SessionMetadata{ZedThreadID: "thread-b"}},
	} {
		_, err := ms.CreateSession(ctx, session)
		require.NoError(t, err)
	}
	_, err := ms.CreateInteraction(ctx, &types.Interaction{ID: "int_lane_b", SessionID: "ses_lane_b", State: types.InteractionStateWaiting})
	require.NoError(t, err)
	srv := NewTestServer(ms, pubsub.NewNoop())
	srv.externalAgentWSManager.registerConnection("ses_lane_a", &ExternalAgentWSConnection{SessionID: "ses_lane_a", SendChan: make(chan types.ExternalAgentCommand, 1)})
	return srv, ms
}

func requireLaneBUntouched(t *testing.T, ms *memorystore.MemoryStore, srv *HelixAPIServer) {
	t.Helper()
	ctx := context.Background()
	b, err := ms.GetSession(ctx, "ses_lane_b")
	require.NoError(t, err)
	require.Equal(t, "thread-b", b.Metadata.ZedThreadID)
	require.Empty(t, b.Metadata.ExternalAgentID)
	require.NotContains(t, srv.contextMappings, routeKey("ses_lane_a", "evil"))

	restarted := NewTestServer(ms, pubsub.NewNoop())
	_, err = restarted.findSessionByZedThreadID(ctx, "ses_lane_a", "evil")
	require.Error(t, err, "lane A must not reach lane B after a restart")
}

// Lane A naming lane B's session in thread_created.session_id must not rebind
// B to a thread A controls.
func TestThreadCreated_SessionIDFromAnotherLaneIsRefused(t *testing.T) {
	srv, ms := twoLanes(t)

	err := srv.handleThreadCreated("ses_lane_a", &types.SyncMessage{
		EventType: "thread_created",
		SessionID: "ses_lane_b",
		Data:      map[string]interface{}{"acp_thread_id": "evil"},
	})
	require.Error(t, err)
	requireLaneBUntouched(t, ms, srv)
}

// The request-id paths are bound to the connection the same way: a request
// mapped to lane B's session or turn (PRIORITY 1 and 4) cannot be claimed by
// lane A's connection.
func TestThreadCreated_RequestFromAnotherLaneIsRefused(t *testing.T) {
	t.Run("request to session (priority 1)", func(t *testing.T) {
		srv, ms := twoLanes(t)
		srv.requestToSessionMapping["req-b"] = "ses_lane_b"
		err := srv.handleThreadCreated("ses_lane_a", &types.SyncMessage{
			EventType: "thread_created",
			Data:      map[string]interface{}{"acp_thread_id": "evil", "request_id": "req-b"},
		})
		require.Error(t, err)
		requireLaneBUntouched(t, ms, srv)
	})
	t.Run("in-flight turn (priority 4)", func(t *testing.T) {
		srv, ms := twoLanes(t)
		srv.requestToInteractionMapping["req-b"] = "int_lane_b"
		err := srv.handleThreadCreated("ses_lane_a", &types.SyncMessage{
			EventType: "thread_created",
			Data:      map[string]interface{}{"acp_thread_id": "evil", "request_id": "req-b"},
		})
		require.Error(t, err)
		requireLaneBUntouched(t, ms, srv)
	})
}

// Turns on an agent-created thread belong to the child session, while the
// WebSocket is the parent's. Questions on such a turn must be accepted from
// the parent's connection and refused from another lane's.
func TestQuestionOnChildThreadSession(t *testing.T) {
	srv, ms := twoLanes(t)
	ctx := context.Background()
	_, err := ms.CreateSession(ctx, types.Session{
		ID: "ses_lane_a_child", Owner: confinedOwnerID, OwnerType: types.OwnerTypeUser, ProjectID: "prj_a",
		Metadata: types.SessionMetadata{ZedThreadID: "thread-child", ExternalAgentID: "ses_lane_a"},
	})
	require.NoError(t, err)
	_, err = ms.CreateInteraction(ctx, &types.Interaction{ID: "int_child", SessionID: "ses_lane_a_child", State: types.InteractionStateWaiting})
	require.NoError(t, err)
	srv.requestToInteractionMapping["turn-child"] = "int_child"
	ask := func(connection string) error {
		return srv.handleQuestionRequested(connection, &types.SyncMessage{
			EventType: "question_requested",
			Data: map[string]interface{}{
				"request_id": "q-child", "thread_id": "thread-child", "turn_request_id": "turn-child", "source": "permission",
				"questions": []map[string]interface{}{{"id": "allow", "question": "Allow?"}},
			},
		})
	}

	require.Error(t, ask("ses_lane_b"), "another lane cannot raise a question on lane A's child turn")
	require.NoError(t, ask("ses_lane_a"))

	child, err := ms.GetInteraction(ctx, "int_child")
	require.NoError(t, err)
	require.NotNil(t, child.PendingQuestion)
	require.NoError(t, srv.handleQuestionResolved("ses_lane_a", &types.SyncMessage{
		EventType: "question_resolved",
		Data:      map[string]interface{}{"request_id": "q-child", "turn_request_id": "turn-child", "thread_id": "thread-child", "outcome": "cancelled"},
	}))
}

// Lane A replaying lane B's request id in turn_cancelled or
// chat_response_error must not end lane B's turn.
func TestTurnEventsFromAnotherLaneAreRefused(t *testing.T) {
	for name, event := range map[string]func(*HelixAPIServer) error{
		"turn_cancelled": func(srv *HelixAPIServer) error {
			return srv.handleTurnCancelled("ses_lane_a", &types.SyncMessage{
				EventType: "turn_cancelled", Data: map[string]interface{}{"request_id": "req-b", "status": "cancelled"},
			})
		},
		"chat_response_error": func(srv *HelixAPIServer) error {
			return srv.handleChatResponseError("ses_lane_a", &types.SyncMessage{
				EventType: "chat_response_error", Data: map[string]interface{}{"request_id": "req-b", "error": "boom"},
			})
		},
	} {
		t.Run(name, func(t *testing.T) {
			srv, ms := twoLanes(t)
			srv.requestToInteractionMapping["req-b"] = "int_lane_b"

			require.Error(t, event(srv))

			b, err := ms.GetInteraction(context.Background(), "int_lane_b")
			require.NoError(t, err)
			require.Equal(t, types.InteractionStateWaiting, b.State, "lane B's turn is untouched")
			require.Empty(t, b.Error)
		})
	}
}
