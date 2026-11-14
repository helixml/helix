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
