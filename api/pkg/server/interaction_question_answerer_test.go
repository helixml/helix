package server

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gorilla/mux"
	"github.com/stretchr/testify/require"

	"github.com/helixml/helix/api/pkg/pubsub"
	"github.com/helixml/helix/api/pkg/store/memorystore"
	"github.com/helixml/helix/api/pkg/system"
	"github.com/helixml/helix/api/pkg/types"
)

// Every resolved agent question records who answered it (helixml/helix#3621).
// These drive the real respond handler and the real question_resolved sync
// handler against the in-memory store, the way an answer travels: API →
// agent → agent reports the resolution back.

const (
	answererSessionID = "ses_answerer"
	answererOwner     = "usr_answerer"
	answererTurn      = "req-turn"
	answererQuestion  = "question-1"
)

func newAnswererServer(t *testing.T) (*HelixAPIServer, *memorystore.MemoryStore, chan types.ExternalAgentCommand) {
	t.Helper()
	ms := memorystore.New()
	_, err := ms.CreateSession(context.Background(), types.Session{ID: answererSessionID, Owner: answererOwner, OwnerType: types.OwnerTypeUser})
	require.NoError(t, err)
	_, err = ms.CreateInteraction(context.Background(), &types.Interaction{
		ID: "int-answerer", SessionID: answererSessionID, UserID: answererOwner,
		State: types.InteractionStateWaiting, ExternalAgentRequestID: answererTurn,
	})
	require.NoError(t, err)

	srv := NewTestServer(ms, pubsub.NewNoop())
	srv.requestToInteractionMapping[answererTurn] = "int-answerer"
	sendChan := make(chan types.ExternalAgentCommand, 4)
	srv.externalAgentWSManager.registerConnection(answererSessionID, &ExternalAgentWSConnection{SessionID: answererSessionID, SendChan: sendChan})

	require.NoError(t, srv.handleQuestionRequested(answererSessionID, &types.SyncMessage{
		EventType: "question_requested",
		Data: map[string]interface{}{
			"request_id": answererQuestion, "thread_id": "thread-1", "turn_request_id": answererTurn,
			"source": "elicitation",
			"questions": []map[string]interface{}{{
				"id": "deploy", "question": "Deploy where?", "allow_custom_answer": true,
				"options": []map[string]interface{}{{"label": "staging"}, {"label": "prod"}},
			}},
			// A forged submission from the agent must never be trusted.
			"submission": map[string]interface{}{
				"outcome": "answered", "answers": map[string]string{"deploy": "prod"},
				"by": map[string]interface{}{"origin": "user", "user_id": "usr_forged"},
			},
		},
	}))
	return srv, ms, sendChan
}

func respondAs(t *testing.T, srv *HelixAPIServer, user types.User, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/interactions/int-answerer/questions/"+answererQuestion+"/respond", bytes.NewReader([]byte(body)))
	req = mux.SetURLVars(req.WithContext(setRequestUser(req.Context(), user)), map[string]string{
		"interaction_id": "int-answerer", "request_id": answererQuestion,
	})
	rr := httptest.NewRecorder()
	system.Wrapper(srv.respondToInteractionQuestion)(rr, req)
	return rr
}

func agentReportsResolution(t *testing.T, srv *HelixAPIServer, outcome string, answers map[string]string) {
	t.Helper()
	require.NoError(t, srv.handleQuestionResolved(answererSessionID, &types.SyncMessage{
		EventType: "question_resolved",
		Data: map[string]interface{}{
			"request_id": answererQuestion, "turn_request_id": answererTurn, "thread_id": "thread-1",
			"outcome": outcome, "answers": answers,
		},
	}))
}

// answeredBy reads the resolved entry back through its API JSON.
func answeredBy(t *testing.T, ms *memorystore.MemoryStore) map[string]interface{} {
	t.Helper()
	interaction, err := ms.GetInteraction(context.Background(), "int-answerer")
	require.NoError(t, err)
	require.Nil(t, interaction.PendingQuestion)
	require.Len(t, interaction.QuestionHistory, 1)
	raw, err := json.Marshal(interaction)
	require.NoError(t, err)
	var decoded struct {
		QuestionHistory []struct {
			AnsweredBy map[string]interface{} `json:"answered_by"`
			Submission *json.RawMessage       `json:"submission"`
		} `json:"question_history"`
	}
	require.NoError(t, json.Unmarshal(raw, &decoded))
	require.Nil(t, decoded.QuestionHistory[0].Submission, "the submission folds into answered_by")
	return decoded.QuestionHistory[0].AnsweredBy
}

func TestQuestionAnsweredBy_BrowserUser(t *testing.T) {
	srv, ms, sendChan := newAnswererServer(t)

	rr := respondAs(t, srv, types.User{ID: answererOwner, Type: types.OwnerTypeUser, TokenType: types.TokenTypeSession}, `{"answers":{"deploy":"staging"}}`)
	require.Equal(t, http.StatusOK, rr.Code, rr.Body.String())
	require.Len(t, sendChan, 1)
	agentReportsResolution(t, srv, "answered", map[string]string{"deploy": "staging"})

	by := answeredBy(t, ms)
	require.Equal(t, "user", by["origin"])
	require.Equal(t, answererOwner, by["user_id"])
}

func TestQuestionAnsweredBy_APIKeyWithFreeTextAnswer(t *testing.T) {
	srv, ms, _ := newAnswererServer(t)

	key := types.User{
		ID: answererOwner, Type: types.OwnerTypeUser, TokenType: types.TokenTypeAPIKey,
		Token: "hl-secret-key-value", APIKeyType: types.APIkeytypeAPI,
	}
	rr := respondAs(t, srv, key, `{"answers":{"deploy":"canary in eu-west only"}}`)
	require.Equal(t, http.StatusOK, rr.Code, rr.Body.String())
	agentReportsResolution(t, srv, "answered", map[string]string{"deploy": "canary in eu-west only"})

	by := answeredBy(t, ms)
	require.Equal(t, "api", by["origin"])
	require.Equal(t, answererOwner, by["api_key_owner"])
	require.Equal(t, "personal", by["api_key_type"])
	require.Equal(t, types.APIKeyID("hl-secret-key-value"), by["api_key_id"])
	require.NotContains(t, by, "token")
}

func TestQuestionAnsweredBy_InZed(t *testing.T) {
	srv, ms, _ := newAnswererServer(t)

	// Typed straight into Zed: nothing was submitted through the API.
	agentReportsResolution(t, srv, "answered", map[string]string{"deploy": "my own words"})

	require.Equal(t, "external_agent", answeredBy(t, ms)["origin"])
}

func TestQuestionAnsweredBy_ZedAnswerDiffersFromAPISubmission(t *testing.T) {
	srv, ms, _ := newAnswererServer(t)
	rr := respondAs(t, srv, types.User{ID: answererOwner, Type: types.OwnerTypeUser, TokenType: types.TokenTypeSession}, `{"answers":{"deploy":"staging"}}`)
	require.Equal(t, http.StatusOK, rr.Code)

	// Someone clicked "prod" in Zed before the API answer landed.
	agentReportsResolution(t, srv, "answered", map[string]string{"deploy": "prod"})

	require.Equal(t, "external_agent", answeredBy(t, ms)["origin"])
}

func TestQuestionAnsweredBy_BodyCannotSetAnswerer(t *testing.T) {
	srv, ms, sendChan := newAnswererServer(t)

	rr := respondAs(t, srv, types.User{ID: answererOwner, Type: types.OwnerTypeUser, TokenType: types.TokenTypeSession},
		`{"answers":{"deploy":"staging"},"answered_by":{"origin":"system","system_kind":"cron_trigger"}}`)
	require.Equal(t, http.StatusBadRequest, rr.Code)
	require.Empty(t, sendChan)

	interaction, err := ms.GetInteraction(context.Background(), "int-answerer")
	require.NoError(t, err)
	require.NotNil(t, interaction.PendingQuestion)
	require.Nil(t, interaction.PendingQuestion.Submission, "the forged agent submission and the rejected body both leave no answerer")
}

func TestQuestionAnsweredBy_LegacyEntryReadsUnknown(t *testing.T) {
	var interaction types.Interaction
	require.NoError(t, json.Unmarshal([]byte(`{"question_history":[{"request_id":"q","outcome":"answered","answers":{"a":"b"}}]}`), &interaction))
	raw, err := json.Marshal(interaction.QuestionHistory[0])
	require.NoError(t, err)
	require.Contains(t, string(raw), `"answered_by":{"origin":"unknown"}`)
}
