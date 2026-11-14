package server

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gorilla/mux"
	"github.com/stretchr/testify/require"

	"github.com/helixml/helix/api/pkg/pubsub"
	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/store/memorystore"
	"github.com/helixml/helix/api/pkg/types"
)

// A user entry Helix already recorded must never become a second turn
// (helixml/helix#3619), whether Zed re-reports it or echoes a turn whose
// request_id Helix has consumed or forgotten.

const (
	entrySessionID = "ses_entries"
	entryThreadID  = "thread-entries"
)

func newEntryServer(t *testing.T) (*HelixAPIServer, *memorystore.MemoryStore) {
	t.Helper()
	ms := memorystore.New()
	_, err := ms.CreateSession(context.Background(), types.Session{
		ID: entrySessionID, Owner: "usr_entries", OwnerType: types.OwnerTypeUser,
		Metadata: types.SessionMetadata{ZedThreadID: entryThreadID},
	})
	require.NoError(t, err)
	return NewTestServer(ms, pubsub.NewNoop()), ms
}

func sendUserEntry(t *testing.T, srv *HelixAPIServer, entryID, content, requestID string) {
	t.Helper()
	sendUserEntryOnThread(t, srv, entryThreadID, entryID, content, requestID)
}

func sendUserEntryOnThread(t *testing.T, srv *HelixAPIServer, threadID, entryID, content, requestID string) {
	t.Helper()
	require.NoError(t, srv.handleMessageAdded(entrySessionID, &types.SyncMessage{
		EventType: "message_added",
		Data: map[string]interface{}{
			"acp_thread_id": threadID,
			"message_id":    entryID,
			"content":       content,
			"role":          "user",
			"request_id":    requestID,
		},
	}))
}

func entryInteractions(t *testing.T, ms *memorystore.MemoryStore) []*types.Interaction {
	t.Helper()
	interactions, _, err := ms.ListInteractions(context.Background(), &types.ListInteractionsQuery{SessionID: entrySessionID})
	require.NoError(t, err)
	return interactions
}

func TestUserEntry_ReportedTwiceIsOneTurn(t *testing.T) {
	srv, ms := newEntryServer(t)

	// Zed gives every native user entry a fresh request_id, so the replay of
	// entry 2 arrives under a different one.
	sendUserEntry(t, srv, "2", "fix the flaky test", "req-first")
	sendUserEntry(t, srv, "2", "fix the flaky test", "req-replayed")

	interactions := entryInteractions(t, ms)
	require.Len(t, interactions, 1)
	require.Equal(t, "2", interactions[0].ExternalAgentUserEntryID)
	require.Equal(t, "req-first", interactions[0].ExternalAgentRequestID)
}

func TestUserEntry_ReplayAfterReconnectIsNotANewTurn(t *testing.T) {
	srv, ms := newEntryServer(t)
	sendUserEntry(t, srv, "4", "add a migration", "req-before-restart")

	// API restart: every in-memory map is gone; only the store survives.
	restarted := NewTestServer(ms, pubsub.NewNoop())
	sendUserEntry(t, restarted, "4", "add a migration", "req-after-restart")
	// The original request_id echoed after the restart is also recognised.
	sendUserEntry(t, restarted, "5", "add a migration", "req-before-restart")

	require.Len(t, entryInteractions(t, ms), 1)
}

func TestUserEntry_EchoOfConsumedRequestIsNotANewTurn(t *testing.T) {
	srv, ms := newEntryServer(t)
	sendUserEntry(t, srv, "0", "first", "req-consumed")
	// message_completed consumed the request: the sentinel is "".
	srv.requestToInteractionMapping["req-consumed"] = ""

	sendUserEntry(t, srv, "6", "first", "req-consumed")

	require.Len(t, entryInteractions(t, ms), 1)
}

func TestUserEntry_RenumberedEntryWithNewTextIsANewTurn(t *testing.T) {
	srv, ms := newEntryServer(t)
	sendUserEntry(t, srv, "1", "old message", "req-old")
	// After an agent restart renumbers entries, index 1 can hold a genuinely
	// new message; it must not be swallowed as a replay.
	sendUserEntry(t, srv, "1", "brand new message", "req-new")

	require.Len(t, entryInteractions(t, ms), 2)
}

func TestUserEntry_RequestFromAnotherSessionDoesNotSuppress(t *testing.T) {
	srv, ms := newEntryServer(t)
	_, err := ms.CreateInteraction(context.Background(), &types.Interaction{
		ID: "int_other", SessionID: "ses_other", ExternalAgentRequestID: "req-shared",
	})
	require.NoError(t, err)

	sendUserEntry(t, srv, "0", "hello", "req-shared")

	require.Len(t, entryInteractions(t, ms), 1)
}

// A reseed changes what the agent receives without changing the stored
// prompt (helixml/helix#3620); the record of it is part of the interaction
// the API returns.
func TestThreadReseed_RecordedAndExposedOnInteractions(t *testing.T) {
	srv, ms := newEntryServer(t)
	session, err := ms.GetSession(context.Background(), entrySessionID)
	require.NoError(t, err)
	session.Metadata.CodeAgentRuntime = types.CodeAgentRuntimeClaudeCode
	interaction, err := ms.CreateInteraction(context.Background(), &types.Interaction{
		SessionID: entrySessionID, UserID: session.Owner, PromptMessage: "keep going", State: types.InteractionStateWaiting,
	})
	require.NoError(t, err)

	require.NoError(t, srv.recordThreadReseed(context.Background(), session, interaction, "dsh", "[preamble]\n"))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/sessions/"+entrySessionID+"/interactions", http.NoBody)
	req = mux.SetURLVars(req.WithContext(setRequestUser(req.Context(), types.User{ID: session.Owner, Type: types.OwnerTypeUser})), map[string]string{"id": entrySessionID})
	page, httpErr := srv.listInteractions(nil, req)
	require.Nil(t, httpErr)
	require.Len(t, page.Interactions, 1)

	raw, err := json.Marshal(page.Interactions[0])
	require.NoError(t, err)
	var decoded struct {
		PromptMessage string `json:"prompt_message"`
		ThreadReseeds []struct {
			Reason         string `json:"reason"`
			Runtime        string `json:"runtime"`
			AgentName      string `json:"agent_name"`
			PreambleSHA256 string `json:"preamble_sha256"`
			PreambleLength int    `json:"preamble_length"`
			Provenance     struct {
				Origin     string `json:"origin"`
				SystemKind string `json:"system_kind"`
			} `json:"provenance"`
		} `json:"thread_reseeds"`
	}
	require.NoError(t, json.Unmarshal(raw, &decoded))
	require.Equal(t, "keep going", decoded.PromptMessage, "the stored prompt is unchanged")
	require.Len(t, decoded.ThreadReseeds, 1)
	r := decoded.ThreadReseeds[0]
	require.Equal(t, "agent_cannot_load_session", r.Reason)
	require.Equal(t, string(types.CodeAgentRuntimeClaudeCode), r.Runtime)
	require.Equal(t, "dsh", r.AgentName)
	require.Equal(t, len("[preamble]\n"), r.PreambleLength)
	require.Len(t, r.PreambleSHA256, 64)
	require.Equal(t, "system", r.Provenance.Origin)
	require.Equal(t, "thread_reseed", r.Provenance.SystemKind)
}

// The same Helix session moves to a new Zed thread on thread recovery (a dead
// ACP thread is replaced) and on an in-place agent switch. Entry indexes
// restart at 0 there, so "continue" at index 1 in the new thread is a new
// turn even though the old thread had the same text at the same index.
func TestUserEntry_SameIndexAndTextInANewThreadIsANewTurn(t *testing.T) {
	for _, reason := range []string{"thread recovery", "agent switch"} {
		t.Run(reason, func(t *testing.T) {
			srv, ms := newEntryServer(t)
			sendUserEntry(t, srv, "1", "continue", "req-old-thread")

			// The session now runs on a fresh ACP thread.
			srv.contextMappings[routeKey(entrySessionID, "thread-next")] = entrySessionID
			sendUserEntryOnThread(t, srv, "thread-next", "1", "continue", "req-new-thread")

			interactions := entryInteractions(t, ms)
			require.Len(t, interactions, 2)

			// A true replay of the new thread's entry is still one turn.
			sendUserEntryOnThread(t, srv, "thread-next", "1", "continue", "req-replayed")
			require.Len(t, entryInteractions(t, ms), 2)
		})
	}
}

// The request map is global; an id mapped to another session's interaction
// must not suppress a turn in this one.
func TestUserEntry_RequestMappedToAnotherSessionDoesNotSuppress(t *testing.T) {
	srv, ms := newEntryServer(t)
	_, err := ms.CreateInteraction(context.Background(), &types.Interaction{ID: "int_elsewhere", SessionID: "ses_elsewhere"})
	require.NoError(t, err)
	srv.requestToInteractionMapping["req-elsewhere"] = "int_elsewhere"

	sendUserEntry(t, srv, "0", "hello", "req-elsewhere")

	require.Len(t, entryInteractions(t, ms), 1)
}

// staleReadStore hides recorded entries from the read-side check, as if a
// second reader checked before the first reader's insert landed.
type staleReadStore struct{ *memorystore.MemoryStore }

func (staleReadStore) GetInteractionByExternalAgentUserEntry(context.Context, string, int, string, string) (*types.Interaction, error) {
	return nil, store.ErrNotFound
}

// Two readers of a reconnecting agent can both pass the read-side check; the
// store's uniqueness on the entry turns the second insert into a dedupe hit
// rather than a second turn or an error.
func TestUserEntry_ConcurrentInsertIsADedupeHit(t *testing.T) {
	ms := memorystore.New()
	_, err := ms.CreateSession(context.Background(), types.Session{
		ID: entrySessionID, Owner: "usr_entries", OwnerType: types.OwnerTypeUser,
		Metadata: types.SessionMetadata{ZedThreadID: entryThreadID},
	})
	require.NoError(t, err)
	srv := NewTestServer(staleReadStore{ms}, pubsub.NewNoop())

	sendUserEntry(t, srv, "3", "ship it", "req-reader-1")
	sendUserEntry(t, srv, "3", "ship it", "req-reader-2")

	require.Len(t, entryInteractions(t, ms), 1)
}
