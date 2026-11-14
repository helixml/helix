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

	replayQuestion(t, srv)
	return srv, ms, sendChan
}

// replayQuestion is Zed's question_requested, sent on first ask and again
// after a reconnect while the question is still open.
func replayQuestion(t *testing.T, srv *HelixAPIServer) {
	t.Helper()
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

// agentReportsResolution is Zed's question_resolved. submissionID is the
// nonce it echoes back ("" for the pinned Zed, which does not echo it).
func agentReportsResolution(t *testing.T, srv *HelixAPIServer, outcome string, answers map[string]string, submissionID string) {
	t.Helper()
	data := map[string]interface{}{
		"request_id": answererQuestion, "turn_request_id": answererTurn, "thread_id": "thread-1",
		"outcome": outcome, "answers": answers,
	}
	if submissionID != "" {
		data["submission_id"] = submissionID
	}
	require.NoError(t, srv.handleQuestionResolved(answererSessionID, &types.SyncMessage{EventType: "question_resolved", Data: data}))
}

// forwardedSubmissionID is the nonce Helix sent to the agent with the command.
func forwardedSubmissionID(t *testing.T, sendChan chan types.ExternalAgentCommand) string {
	t.Helper()
	require.Len(t, sendChan, 1)
	command := <-sendChan
	id, _ := command.Data["submission_id"].(string)
	require.NotEmpty(t, id)
	return id
}

func cancelAs(t *testing.T, srv *HelixAPIServer, user types.User) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/interactions/int-answerer/questions/"+answererQuestion+"/cancel", http.NoBody)
	req = mux.SetURLVars(req.WithContext(setRequestUser(req.Context(), user)), map[string]string{
		"interaction_id": "int-answerer", "request_id": answererQuestion,
	})
	rr := httptest.NewRecorder()
	system.Wrapper(srv.cancelInteractionQuestion)(rr, req)
	return rr
}

var (
	browserUser = types.User{ID: answererOwner, Type: types.OwnerTypeUser, TokenType: types.TokenTypeSession}
	laneKey     = types.User{
		ID: answererOwner, Type: types.OwnerTypeUser, TokenType: types.TokenTypeAPIKey,
		Token: "hl-secret-key-value", APIKeyType: types.APIkeytypeAPI,
	}
)

func pendingSubmission(t *testing.T, ms *memorystore.MemoryStore) *types.QuestionSubmission {
	t.Helper()
	interaction, err := ms.GetInteraction(context.Background(), "int-answerer")
	require.NoError(t, err)
	require.NotNil(t, interaction.PendingQuestion)
	return interaction.PendingQuestion.Submission
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

func TestQuestionAnsweredBy_BrowserUserWhenNonceEchoed(t *testing.T) {
	srv, ms, sendChan := newAnswererServer(t)

	rr := respondAs(t, srv, browserUser, `{"answers":{"deploy":"staging"}}`)
	require.Equal(t, http.StatusOK, rr.Code, rr.Body.String())
	agentReportsResolution(t, srv, "answered", map[string]string{"deploy": "staging"}, forwardedSubmissionID(t, sendChan))

	by := answeredBy(t, ms)
	require.Equal(t, "user", by["origin"])
	require.Equal(t, answererOwner, by["user_id"])
}

func TestQuestionAnsweredBy_APIKeyFreeTextWhenNonceEchoed(t *testing.T) {
	srv, ms, sendChan := newAnswererServer(t)

	rr := respondAs(t, srv, laneKey, `{"answers":{"deploy":"canary in eu-west only"}}`)
	require.Equal(t, http.StatusOK, rr.Code, rr.Body.String())
	agentReportsResolution(t, srv, "answered", map[string]string{"deploy": "canary in eu-west only"}, forwardedSubmissionID(t, sendChan))

	by := answeredBy(t, ms)
	require.Equal(t, "api", by["origin"])
	require.Equal(t, answererOwner, by["api_key_owner"])
	require.Equal(t, "personal", by["api_key_type"])
	require.Equal(t, types.APIKeyID("hl-secret-key-value"), by["api_key_id"])
	require.NotContains(t, by, "token")
}

// The Zed build pinned today does not echo the nonce. Matching content would
// be wrong (see the permission-prompt test below), so the resolution is
// credited to the agent's own UI, never to the REST sender.
func TestQuestionAnsweredBy_NoNonceEchoIsExternalAgent(t *testing.T) {
	srv, ms, _ := newAnswererServer(t)
	rr := respondAs(t, srv, laneKey, `{"answers":{"deploy":"staging"}}`)
	require.Equal(t, http.StatusOK, rr.Code)

	agentReportsResolution(t, srv, "answered", map[string]string{"deploy": "staging"}, "")

	require.Equal(t, "external_agent", answeredBy(t, ms)["origin"])
}

func TestQuestionAnsweredBy_InZed(t *testing.T) {
	srv, ms, _ := newAnswererServer(t)

	// Typed straight into Zed: nothing was submitted through the API.
	agentReportsResolution(t, srv, "answered", map[string]string{"deploy": "my own words"}, "")

	require.Equal(t, "external_agent", answeredBy(t, ms)["origin"])
}

// Key X's cancel cannot reach the agent; later a human approves the
// permission prompt in Zed, which reports "cancelled" with no answers — the
// same shape as X's cancel. It must not be credited to X.
func TestQuestionAnsweredBy_FailedForwardThenHumanInZed(t *testing.T) {
	srv, ms, _ := newAnswererServer(t)
	srv.externalAgentWSManager = NewExternalAgentWSManager() // agent not connected

	rr := cancelAs(t, srv, laneKey)
	require.Equal(t, http.StatusConflict, rr.Code, rr.Body.String())
	require.Nil(t, pendingSubmission(t, ms), "an undelivered submission is withdrawn")

	agentReportsResolution(t, srv, "cancelled", nil, "")

	require.Equal(t, "external_agent", answeredBy(t, ms)["origin"])
}

// A question replay means the agent is still waiting and any accepted action
// was lost with the old connection: the submission is dropped.
func TestQuestionAnsweredBy_ReplayDropsLostSubmission(t *testing.T) {
	srv, ms, sendChan := newAnswererServer(t)
	rr := cancelAs(t, srv, laneKey)
	require.Equal(t, http.StatusOK, rr.Code)
	staleNonce := forwardedSubmissionID(t, sendChan)
	require.NotNil(t, pendingSubmission(t, ms))

	replayQuestion(t, srv)
	require.Nil(t, pendingSubmission(t, ms))

	// Even a late echo of the lost nonce cannot credit the caller now.
	agentReportsResolution(t, srv, "cancelled", nil, staleNonce)
	require.Equal(t, "external_agent", answeredBy(t, ms)["origin"])
}

// A second action while one is in flight (another replica, or after an API
// restart lost the in-memory claim) is refused rather than overwriting it.
func TestQuestionAnsweredBy_SecondSubmissionInFlightIsRefused(t *testing.T) {
	srv, ms, sendChan := newAnswererServer(t)
	rr := respondAs(t, srv, browserUser, `{"answers":{"deploy":"staging"}}`)
	require.Equal(t, http.StatusOK, rr.Code)
	first := forwardedSubmissionID(t, sendChan)

	// Simulate the other replica: no in-memory claim, same durable row.
	srv.finishQuestionAction("int-answerer", answererQuestion)
	rr = cancelAs(t, srv, laneKey)
	require.Equal(t, http.StatusConflict, rr.Code, rr.Body.String())
	require.Empty(t, sendChan)
	require.Equal(t, first, pendingSubmission(t, ms).ID)
}

// Helix settling a question at turn end records system/question_settled even
// when an API cancel is still on it.
func TestQuestionAnsweredBy_SettleIgnoresSubmission(t *testing.T) {
	srv, ms, _ := newAnswererServer(t)
	rr := cancelAs(t, srv, laneKey)
	require.Equal(t, http.StatusOK, rr.Code)

	interaction, err := ms.GetInteraction(context.Background(), "int-answerer")
	require.NoError(t, err)
	_, changed, err := srv.settlePendingQuestionAsCancelled(context.Background(), interaction)
	require.NoError(t, err)
	require.True(t, changed)

	by := answeredBy(t, ms)
	require.Equal(t, "system", by["origin"])
	require.Equal(t, "question_settled", by["system_kind"])
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
