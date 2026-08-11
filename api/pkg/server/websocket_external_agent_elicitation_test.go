package server

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/mux"
	"github.com/helixml/helix/api/pkg/config"
	"github.com/helixml/helix/api/pkg/controller"
	"github.com/helixml/helix/api/pkg/pubsub"
	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/system"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/stretchr/testify/suite"
	"go.uber.org/mock/gomock"
)

// ElicitationSuite covers the agent-questions feature: the four sync handlers that
// mirror ACP elicitations from Zed, the REST endpoint that answers them, and the two
// gates that stop Helix "recovering" a turn that is legitimately waiting on a human.
//
// The concurrency cases (two clients answering at once, an answer racing a cancel) are
// expressed as the conditional store write returning false, because that IS the
// semantics of `UPDATE … WHERE status IN (…)` — the production behaviour is decided by
// rows-affected, not by goroutine scheduling. Spawning goroutines here would test the
// Go runtime rather than the feature.
type ElicitationSuite struct {
	suite.Suite
	ctrl   *gomock.Controller
	store  *store.MockStore
	server *HelixAPIServer
}

func TestElicitationSuite(t *testing.T) {
	suite.Run(t, new(ElicitationSuite))
}

func (s *ElicitationSuite) SetupTest() {
	s.ctrl = gomock.NewController(s.T())
	s.store = store.NewMockStore(s.ctrl)

	// Fire-and-forget side effects that most paths trigger. Allowed anywhere so each
	// test only declares the expectations it is actually about.
	s.store.EXPECT().TouchSession(gomock.Any(), gomock.Any()).Return(nil).AnyTimes()

	s.server = &HelixAPIServer{
		Cfg: &config.ServerConfig{
			WebServer: config.WebServer{
				URL:         "http://localhost:0",
				Host:        "localhost",
				Port:        0,
				RunnerToken: "test-runner-token",
			},
		},
		Store:  s.store,
		pubsub: pubsub.NewNoop(),
		Controller: &controller.Controller{
			Options: controller.Options{
				Store:  s.store,
				PubSub: pubsub.NewNoop(),
			},
		},
		externalAgentWSManager:      NewExternalAgentWSManager(),
		externalAgentRunnerManager:  NewExternalAgentRunnerManager(),
		contextMappings:             make(map[string]string),
		requestToSessionMapping:     make(map[string]string),
		requestToInteractionMapping: make(map[string]string),
		pendingCancelChannels:       make(map[string]chan string),
		externalAgentSessionMapping: make(map[string]string),
		externalAgentUserMapping:    make(map[string]string),
		sessionCommentTimeout:       make(map[string]*time.Timer),
		requestToCommenterMapping:   make(map[string]string),
		streamingContexts:           make(map[string]*streamingContext),
		streamingRateLimiter:        make(map[string]time.Time),
	}
}

func (s *ElicitationSuite) TearDownTest() {
	s.ctrl.Finish()
}

// ──────────────────────────────────────────────────────────────────────────────
// Fixtures
// ──────────────────────────────────────────────────────────────────────────────

const (
	testThreadID       = "thread-elicit"
	testSessionID      = "ses_elicit"
	testInteractionID  = "int_elicit"
	testElicitationID  = "elicit-1"
	testRequestID      = "req-elicit"
	testOwnerID        = "user-owner"
	testAskUserSchema  = `{"type":"object","properties":{"question_0":{"oneOf":[{"const":"redis","title":"Redis"},{"const":"memcached","title":"Memcached"}]}}}`
	testEntryIndexZero = "7"
)

func (s *ElicitationSuite) session() *types.Session {
	return &types.Session{
		ID:    testSessionID,
		Owner: testOwnerID,
		Metadata: types.SessionMetadata{
			AgentType:   "zed_external",
			ZedThreadID: testThreadID,
		},
	}
}

func (s *ElicitationSuite) interaction() *types.Interaction {
	return &types.Interaction{
		ID:        testInteractionID,
		SessionID: testSessionID,
		State:     types.InteractionStateWaiting,
	}
}

func (s *ElicitationSuite) pendingElicitation() *types.AgentElicitation {
	return &types.AgentElicitation{
		ID:            testElicitationID,
		SessionID:     testSessionID,
		InteractionID: testInteractionID,
		RequestID:     testRequestID,
		AcpThreadID:   testThreadID,
		EntryIndex:    testEntryIndexZero,
		Message:       "Which cache backend should I use?",
		Mode:          "form",
		Schema:        []byte(testAskUserSchema),
		Status:        types.ElicitationStatusPending,
		LastSeenAt:    time.Now(),
	}
}

// requestedSyncMsg builds an elicitation_requested event as Zed sends it.
func requestedSyncMsg() *types.SyncMessage {
	var schema map[string]interface{}
	_ = json.Unmarshal([]byte(testAskUserSchema), &schema)
	return &types.SyncMessage{
		EventType: "elicitation_requested",
		Data: map[string]interface{}{
			"acp_thread_id":    testThreadID,
			"elicitation_id":   testElicitationID,
			"request_id":       testRequestID,
			"entry_index":      testEntryIndexZero,
			"mode":             "form",
			"message":          "Which cache backend should I use?",
			"status":           types.ElicitationStatusPending,
			"requested_schema": schema,
		},
	}
}

// expectTranscriptWrites allows the incidental lookups writeElicitationEntry performs
// when it builds a streaming context and persists the mirrored entry. These are
// plumbing, not the subject of any test, so they are permitted anywhere.
func (s *ElicitationSuite) expectTranscriptWrites() {
	s.store.EXPECT().GetSession(gomock.Any(), testSessionID).Return(s.session(), nil).AnyTimes()
	s.store.EXPECT().ListInteractions(gomock.Any(), gomock.Any()).
		Return([]*types.Interaction{s.interaction()}, int64(1), nil).AnyTimes()
	s.store.EXPECT().UpdateInteraction(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, i *types.Interaction) (*types.Interaction, error) {
			return i, nil
		}).AnyTimes()
}

// expectTargetResolution wires the store lookups resolveElicitationTarget performs when
// there is no live streaming context: the thread maps to a session, and the session's
// newest interaction is the target.
func (s *ElicitationSuite) expectTargetResolution() {
	s.server.contextMappings[testThreadID] = testSessionID
	s.expectTranscriptWrites()
}

// ──────────────────────────────────────────────────────────────────────────────
// handleElicitationRequested
// ──────────────────────────────────────────────────────────────────────────────

// A new question is persisted verbatim and mirrored into the transcript. The schema is
// the whole UI — anything dropped here is an option the user never gets offered.
func (s *ElicitationSuite) TestRequested_PersistsQuestionAndSchema() {
	s.expectTargetResolution()

	var captured *types.AgentElicitation
	s.store.EXPECT().UpsertAgentElicitation(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, e *types.AgentElicitation) (bool, error) {
			captured = e
			return true, nil
		})
	s.store.EXPECT().GetAgentElicitation(gomock.Any(), testElicitationID).
		Return(s.pendingElicitation(), nil)

	s.NoError(s.server.handleElicitationRequested("agent-1", requestedSyncMsg()))

	s.Require().NotNil(captured)
	s.Equal(testElicitationID, captured.ID)
	s.Equal(testSessionID, captured.SessionID)
	s.Equal(testInteractionID, captured.InteractionID)
	s.Equal(testThreadID, captured.AcpThreadID)
	s.Equal(testEntryIndexZero, captured.EntryIndex, "entry_index is the transcript ordering anchor")
	s.Equal(types.ElicitationStatusPending, captured.Status)
	s.NotEmpty(captured.Schema, "schema must be stored — it is what renders the options")
	s.Contains(string(captured.Schema), "memcached", "every option must survive the round trip")
}

// A missing elicitation_id is a protocol error, not something to guess at.
func (s *ElicitationSuite) TestRequested_MissingIDIsAnError() {
	err := s.server.handleElicitationRequested("agent-1", &types.SyncMessage{
		EventType: "elicitation_requested",
		Data:      map[string]interface{}{"acp_thread_id": testThreadID},
	})
	s.Error(err)
}

// The agent re-announces outstanding questions on every reconnect and on a 15s
// heartbeat. Those re-announcements must not re-notify the user — the upsert reporting
// "not new" is what gates that.
func (s *ElicitationSuite) TestRequested_ReannouncementDoesNotRenotify() {
	s.expectTargetResolution()

	s.store.EXPECT().UpsertAgentElicitation(gomock.Any(), gomock.Any()).Return(false, nil)
	s.store.EXPECT().GetAgentElicitation(gomock.Any(), testElicitationID).
		Return(s.pendingElicitation(), nil)
	// notifyAgentQuestion would call GetSpecTask. Asserting it is never called is the
	// point of this test: gomock fails the run if an unexpected call arrives.
	s.store.EXPECT().GetSpecTask(gomock.Any(), gomock.Any()).Times(0)

	s.NoError(s.server.handleElicitationRequested("agent-1", requestedSyncMsg()))
}

// A question that cannot be routed to an interaction is the exact bug this feature
// exists to fix, so it is logged loudly — but it must not kill the sync connection.
func (s *ElicitationSuite) TestRequested_UnroutableThreadDoesNotBreakSync() {
	s.store.EXPECT().ListSessions(gomock.Any(), gomock.Any()).
		Return(nil, int64(0), nil).AnyTimes()
	s.store.EXPECT().UpsertAgentElicitation(gomock.Any(), gomock.Any()).Times(0)

	msg := requestedSyncMsg()
	msg.Data["acp_thread_id"] = "thread-nobody-knows"

	s.NoError(s.server.handleElicitationRequested("agent-1", msg),
		"an unroutable question is dropped and logged, never returned as a sync error")
}

// An empty request_id must still land: elicitations always happen mid-turn, so a missing
// correlation means something upstream lost it, not that there is no turn. The handler
// falls back to the session's newest interaction.
func (s *ElicitationSuite) TestRequested_EmptyRequestIDFallsBackToNewestInteraction() {
	s.expectTargetResolution()

	var captured *types.AgentElicitation
	s.store.EXPECT().UpsertAgentElicitation(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, e *types.AgentElicitation) (bool, error) {
			captured = e
			return true, nil
		})
	s.store.EXPECT().GetAgentElicitation(gomock.Any(), testElicitationID).
		Return(s.pendingElicitation(), nil)

	msg := requestedSyncMsg()
	msg.Data["request_id"] = ""

	s.NoError(s.server.handleElicitationRequested("agent-1", msg))
	s.Require().NotNil(captured)
	s.Equal(testInteractionID, captured.InteractionID,
		"fallback resolution must still find the waiting interaction")
}

// ──────────────────────────────────────────────────────────────────────────────
// handleElicitationResolved
// ──────────────────────────────────────────────────────────────────────────────

// Every terminal status transitions the row conditionally and maps to the reason the UI
// shows. "declined" is a skip, not an abort: the adapter continues the turn with empty
// answers, which is why the button says "Skip".
func (s *ElicitationSuite) TestResolved_EachTerminalStatusAndItsReason() {
	cases := []struct {
		status string
		reason string
	}{
		{types.ElicitationStatusAccepted, types.ElicitationReasonAnswered},
		{types.ElicitationStatusDeclined, types.ElicitationReasonSkipped},
		{types.ElicitationStatusCancelled, types.ElicitationReasonInterrupted},
		{types.ElicitationStatusCompleted, ""},
	}

	for _, tc := range cases {
		s.Run(tc.status, func() {
			s.SetupTest()
			defer s.TearDownTest()

			resolved := s.pendingElicitation()
			resolved.Status = tc.status
			resolved.ResolutionReason = tc.reason

			s.store.EXPECT().TransitionAgentElicitation(
				gomock.Any(), testElicitationID,
				[]string{types.ElicitationStatusPending, types.ElicitationStatusSubmitting},
				tc.status, tc.reason, gomock.Any(),
			).Return(true, nil)
			s.store.EXPECT().GetAgentElicitation(gomock.Any(), testElicitationID).Return(resolved, nil)
			s.store.EXPECT().GetInteraction(gomock.Any(), testInteractionID).Return(s.interaction(), nil)
			s.expectTranscriptWrites()

			s.NoError(s.server.handleElicitationResolved("agent-1", &types.SyncMessage{
				EventType: "elicitation_resolved",
				Data: map[string]interface{}{
					"elicitation_id": testElicitationID,
					"status":         tc.status,
				},
			}))
		})
	}
}

// A duplicate or late resolved event affects zero rows and is dropped. Terminal is
// final — nothing resurrects an answered question.
func (s *ElicitationSuite) TestResolved_LateEventAffectsNothing() {
	s.store.EXPECT().TransitionAgentElicitation(
		gomock.Any(), testElicitationID, gomock.Any(),
		types.ElicitationStatusCancelled, gomock.Any(), gomock.Any(),
	).Return(false, nil)
	// No reload, no interaction write, no notification clearing.
	s.store.EXPECT().GetAgentElicitation(gomock.Any(), gomock.Any()).Times(0)
	s.store.EXPECT().GetInteraction(gomock.Any(), gomock.Any()).Times(0)

	s.NoError(s.server.handleElicitationResolved("agent-1", &types.SyncMessage{
		EventType: "elicitation_resolved",
		Data: map[string]interface{}{
			"elicitation_id": testElicitationID,
			"status":         types.ElicitationStatusCancelled,
		},
	}))
}

// A "resolved" event carrying the pending status is not a resolution.
func (s *ElicitationSuite) TestResolved_PendingStatusIsIgnored() {
	s.store.EXPECT().TransitionAgentElicitation(
		gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Times(0)

	s.NoError(s.server.handleElicitationResolved("agent-1", &types.SyncMessage{
		EventType: "elicitation_resolved",
		Data: map[string]interface{}{
			"elicitation_id": testElicitationID,
			"status":         types.ElicitationStatusPending,
		},
	}))
}

func (s *ElicitationSuite) TestResolved_MissingIDIsAnError() {
	s.Error(s.server.handleElicitationResolved("agent-1", &types.SyncMessage{
		EventType: "elicitation_resolved",
		Data:      map[string]interface{}{"status": types.ElicitationStatusAccepted},
	}))
}

// ──────────────────────────────────────────────────────────────────────────────
// handleElicitationResync — and the reconnect-is-not-evidence rule
// ──────────────────────────────────────────────────────────────────────────────

// A resync refreshes the liveness stamp of every question the agent says it still holds.
func (s *ElicitationSuite) TestResync_TouchesListedIDs() {
	s.store.EXPECT().TouchAgentElicitations(gomock.Any(), []string{"elicit-1", "elicit-2"}).Return(nil)

	s.NoError(s.server.handleElicitationResync("agent-1", &types.SyncMessage{
		EventType: "elicitation_resync",
		Data: map[string]interface{}{
			"elicitation_ids": []interface{}{"elicit-1", "elicit-2"},
		},
	}))
}

// An empty resync must NOT reap anything inline. This event is per-thread, and a thread
// that failed to load reports nothing — treating "absent from this message" as "dead"
// would kill live questions during a slow reload. The reaper acts on silence over time
// instead.
func (s *ElicitationSuite) TestResync_EmptyListDoesNotReapInline() {
	s.store.EXPECT().ReapStaleAgentElicitations(gomock.Any(), gomock.Any()).Times(0)
	s.store.EXPECT().TouchAgentElicitations(gomock.Any(), gomock.Any()).Times(0)

	s.NoError(s.server.handleElicitationResync("agent-1", &types.SyncMessage{
		EventType: "elicitation_resync",
		Data:      map[string]interface{}{"elicitation_ids": []interface{}{}},
	}))
}

// THE most important rule in this feature: a WebSocket reconnect is not evidence that
// the agent is gone. agent_ready fires on every reconnect, and the commonest cause is
// the Helix API restarting (an Air rebuild) while the desktop container, the Zed process
// and its respond_tx all survive untouched. Resolving questions here would silently kill
// every live question on every API restart.
func (s *ElicitationSuite) TestAgentReady_DoesNotResolveAnyElicitation() {
	s.store.EXPECT().TransitionAgentElicitation(
		gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Times(0)
	s.store.EXPECT().ReapStaleAgentElicitations(gomock.Any(), gomock.Any()).Times(0)

	// Everything else agent_ready touches is incidental to this assertion.
	s.store.EXPECT().GetSession(gomock.Any(), gomock.Any()).Return(s.session(), nil).AnyTimes()
	s.store.EXPECT().ListInteractions(gomock.Any(), gomock.Any()).
		Return([]*types.Interaction{s.interaction()}, int64(1), nil).AnyTimes()
	s.store.EXPECT().UpdateSession(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, sess types.Session) (*types.Session, error) {
			return &sess, nil
		}).AnyTimes()
	s.store.EXPECT().GetSpecTask(gomock.Any(), gomock.Any()).Return(nil, store.ErrNotFound).AnyTimes()
	s.store.EXPECT().ListSessions(gomock.Any(), gomock.Any()).Return(nil, int64(0), nil).AnyTimes()
	s.store.EXPECT().GetNextPendingPrompt(gomock.Any(), gomock.Any()).
		Return(nil, store.ErrNotFound).AnyTimes()

	_ = s.server.handleAgentReady(testSessionID, &types.SyncMessage{
		EventType: "agent_ready",
		Data:      map[string]interface{}{"session_id": testSessionID},
	})
	// The assertion is the Times(0) expectations above: no status was touched.
}

// The reaper is the ONLY place Helix declares a question dead on its own, and it does so
// on evidence — the agent stopped re-affirming it for longer than the grace window.
func (s *ElicitationSuite) TestReaper_CancelsQuestionsPastTheGraceWindow() {
	s.T().Setenv("HELIX_ELICITATION_RESYNC_GRACE_SECONDS", "1")

	reaped := s.pendingElicitation()
	reaped.Status = types.ElicitationStatusCancelled
	reaped.ResolutionReason = types.ElicitationReasonAgentNoLongerHolds

	var cutoff time.Time
	s.store.EXPECT().ReapStaleAgentElicitations(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, olderThan time.Time) ([]*types.AgentElicitation, error) {
			cutoff = olderThan
			return []*types.AgentElicitation{reaped}, nil
		})
	s.store.EXPECT().GetInteraction(gomock.Any(), testInteractionID).Return(s.interaction(), nil)
	s.expectTranscriptWrites()

	s.server.reapStaleElicitations(context.Background())

	s.WithinDuration(time.Now().Add(-time.Second), cutoff, 2*time.Second,
		"cutoff must honour HELIX_ELICITATION_RESYNC_GRACE_SECONDS")
}

// The default grace window is 60s, and a garbage env value does not shrink it to zero —
// that would reap every live question immediately.
func (s *ElicitationSuite) TestGraceWindow_DefaultsAndIgnoresGarbage() {
	s.Equal(defaultElicitationResyncGrace, elicitationResyncGrace())

	s.T().Setenv("HELIX_ELICITATION_RESYNC_GRACE_SECONDS", "not-a-number")
	s.Equal(defaultElicitationResyncGrace, elicitationResyncGrace())

	s.T().Setenv("HELIX_ELICITATION_RESYNC_GRACE_SECONDS", "0")
	s.Equal(defaultElicitationResyncGrace, elicitationResyncGrace())

	s.T().Setenv("HELIX_ELICITATION_RESYNC_GRACE_SECONDS", "5")
	s.Equal(5*time.Second, elicitationResyncGrace())
}

// ──────────────────────────────────────────────────────────────────────────────
// handleElicitationResponseAck
// ──────────────────────────────────────────────────────────────────────────────

// noop and not_found both mean the answer did not apply, so the card must stop offering
// to answer. accepted is a local no-op — the authoritative status arrives separately via
// elicitation_resolved.
func (s *ElicitationSuite) TestAck_ReconcilesNoopAndNotFound() {
	for _, ackStatus := range []string{"noop", "not_found"} {
		s.Run(ackStatus, func() {
			s.SetupTest()
			defer s.TearDownTest()

			cancelled := s.pendingElicitation()
			cancelled.Status = types.ElicitationStatusCancelled
			cancelled.ResolutionReason = types.ElicitationReasonAgentNoLongerHolds

			s.store.EXPECT().TransitionAgentElicitation(
				gomock.Any(), testElicitationID,
				[]string{types.ElicitationStatusPending, types.ElicitationStatusSubmitting},
				types.ElicitationStatusCancelled, types.ElicitationReasonAgentNoLongerHolds, gomock.Any(),
			).Return(true, nil)
			s.store.EXPECT().GetAgentElicitation(gomock.Any(), testElicitationID).Return(cancelled, nil)
			s.store.EXPECT().GetInteraction(gomock.Any(), testInteractionID).Return(s.interaction(), nil)
			s.expectTranscriptWrites()

			s.NoError(s.server.handleElicitationResponseAck("agent-1", &types.SyncMessage{
				EventType: "elicitation_response_ack",
				Data: map[string]interface{}{
					"elicitation_id": testElicitationID,
					"status":         ackStatus,
				},
			}))
		})
	}
}

func (s *ElicitationSuite) TestAck_AcceptedIsALocalNoop() {
	s.store.EXPECT().TransitionAgentElicitation(
		gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Times(0)

	s.NoError(s.server.handleElicitationResponseAck("agent-1", &types.SyncMessage{
		EventType: "elicitation_response_ack",
		Data: map[string]interface{}{
			"elicitation_id": testElicitationID,
			"status":         "accepted",
		},
	}))
}

func (s *ElicitationSuite) TestAck_MissingIDIsAnError() {
	s.Error(s.server.handleElicitationResponseAck("agent-1", &types.SyncMessage{
		EventType: "elicitation_response_ack",
		Data:      map[string]interface{}{"status": "noop"},
	}))
}

// ──────────────────────────────────────────────────────────────────────────────
// POST /api/v1/sessions/{id}/elicitations/{elicitation_id}/respond
// ──────────────────────────────────────────────────────────────────────────────

// connectAgent registers a fake WebSocket connection so the endpoint's
// agent-is-connected check passes and the command has somewhere to go.
func (s *ElicitationSuite) connectAgent(sessionID string) chan types.ExternalAgentCommand {
	sendChan := make(chan types.ExternalAgentCommand, 4)
	s.server.externalAgentWSManager.mu.Lock()
	s.server.externalAgentWSManager.connections[sessionID] = &ExternalAgentWSConnection{
		SessionID:   sessionID,
		SendChan:    sendChan,
		ConnectedAt: time.Now(),
	}
	s.server.externalAgentWSManager.mu.Unlock()
	return sendChan
}

func (s *ElicitationSuite) respond(sessionID, elicitationID string, user *types.User, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost,
		"/api/v1/sessions/"+sessionID+"/elicitations/"+elicitationID+"/respond",
		strings.NewReader(body))
	req = mux.SetURLVars(req, map[string]string{"id": sessionID, "elicitation_id": elicitationID})
	if user != nil {
		req = req.WithContext(setRequestUser(req.Context(), *user))
	}
	rr := httptest.NewRecorder()
	system.Wrapper(s.server.respondToElicitation)(rr, req)
	return rr
}

func (s *ElicitationSuite) TestRespond_HappyPathClaimsAndSends() {
	sendChan := s.connectAgent(testSessionID)

	s.store.EXPECT().GetSession(gomock.Any(), testSessionID).Return(s.session(), nil)
	s.store.EXPECT().GetAgentElicitation(gomock.Any(), testElicitationID).Return(s.pendingElicitation(), nil)
	s.store.EXPECT().TransitionAgentElicitation(
		gomock.Any(), testElicitationID,
		[]string{types.ElicitationStatusPending},
		types.ElicitationStatusSubmitting, "", gomock.Any(),
	).Return(true, nil)

	rr := s.respond(testSessionID, testElicitationID, &types.User{ID: testOwnerID},
		`{"action":"accept","content":{"question_0":"redis"}}`)

	s.Equal(http.StatusOK, rr.Code, "body=%s", rr.Body.String())

	var out types.ElicitationRespondResponse
	s.Require().NoError(json.Unmarshal(rr.Body.Bytes(), &out))
	s.Equal(types.ElicitationStatusSubmitting, out.Status,
		"the endpoint reports submitting; the agent reports the authoritative terminal status")

	select {
	case cmd := <-sendChan:
		s.Equal("respond_elicitation", cmd.Type)
		s.Equal(testElicitationID, cmd.Data["elicitation_id"])
		s.Equal(testThreadID, cmd.Data["acp_thread_id"])
		s.Equal("accept", cmd.Data["action"])
	default:
		s.Fail("no respond_elicitation command was sent to the agent")
	}
}

// Two clients answering at once: both pass every read-only check, and the conditional
// pending→submitting claim decides it. The loser gets a clean 409 and — critically —
// no second command reaches the agent.
func (s *ElicitationSuite) TestRespond_TwoClientsAtOnceOnlyOneWins() {
	sendChan := s.connectAgent(testSessionID)

	s.store.EXPECT().GetSession(gomock.Any(), testSessionID).Return(s.session(), nil).Times(2)
	s.store.EXPECT().GetAgentElicitation(gomock.Any(), testElicitationID).
		Return(s.pendingElicitation(), nil).Times(2)

	gomock.InOrder(
		s.store.EXPECT().TransitionAgentElicitation(
			gomock.Any(), testElicitationID, []string{types.ElicitationStatusPending},
			types.ElicitationStatusSubmitting, "", gomock.Any(),
		).Return(true, nil),
		// The row is no longer pending, so the second conditional write affects 0 rows.
		s.store.EXPECT().TransitionAgentElicitation(
			gomock.Any(), testElicitationID, []string{types.ElicitationStatusPending},
			types.ElicitationStatusSubmitting, "", gomock.Any(),
		).Return(false, nil),
	)

	first := s.respond(testSessionID, testElicitationID, &types.User{ID: testOwnerID},
		`{"action":"accept","content":{"question_0":"redis"}}`)
	second := s.respond(testSessionID, testElicitationID, &types.User{ID: testOwnerID},
		`{"action":"accept","content":{"question_0":"memcached"}}`)

	s.Equal(http.StatusOK, first.Code)
	s.Equal(http.StatusConflict, second.Code, "the loser gets 409, not a second answer")

	s.Len(sendChan, 1, "exactly one command may ever reach the agent")
}

// An answer arriving after the question was cancelled: the row is already terminal, so
// IsLive() rejects it before any write.
func (s *ElicitationSuite) TestRespond_AfterCancelIs409() {
	s.connectAgent(testSessionID)

	cancelled := s.pendingElicitation()
	cancelled.Status = types.ElicitationStatusCancelled

	s.store.EXPECT().GetSession(gomock.Any(), testSessionID).Return(s.session(), nil)
	s.store.EXPECT().GetAgentElicitation(gomock.Any(), testElicitationID).Return(cancelled, nil)
	s.store.EXPECT().TransitionAgentElicitation(
		gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Times(0)

	rr := s.respond(testSessionID, testElicitationID, &types.User{ID: testOwnerID},
		`{"action":"accept","content":{}}`)
	s.Equal(http.StatusConflict, rr.Code)
	s.Contains(rr.Body.String(), "already")
}

// A user who does not own the session cannot answer its questions.
func (s *ElicitationSuite) TestRespond_CrossUserIsForbidden() {
	s.store.EXPECT().GetSession(gomock.Any(), testSessionID).Return(s.session(), nil)
	s.store.EXPECT().GetAgentElicitation(gomock.Any(), gomock.Any()).Times(0)

	rr := s.respond(testSessionID, testElicitationID, &types.User{ID: "someone-else"},
		`{"action":"accept","content":{}}`)
	s.Equal(http.StatusForbidden, rr.Code)
}

func (s *ElicitationSuite) TestRespond_UnknownSessionIs404() {
	s.store.EXPECT().GetSession(gomock.Any(), "ses_missing").Return(nil, store.ErrNotFound)

	rr := s.respond("ses_missing", testElicitationID, &types.User{ID: testOwnerID},
		`{"action":"accept","content":{}}`)
	s.Equal(http.StatusNotFound, rr.Code)
}

func (s *ElicitationSuite) TestRespond_UnknownElicitationIs404() {
	s.store.EXPECT().GetSession(gomock.Any(), testSessionID).Return(s.session(), nil)
	s.store.EXPECT().GetAgentElicitation(gomock.Any(), "elicit-missing").Return(nil, store.ErrNotFound)

	rr := s.respond(testSessionID, "elicit-missing", &types.User{ID: testOwnerID},
		`{"action":"accept","content":{}}`)
	s.Equal(http.StatusNotFound, rr.Code)
}

// A question belonging to a different session is reported as missing, not forbidden:
// the caller is authorised on THIS session, so "forbidden" would confirm the id exists
// somewhere else.
func (s *ElicitationSuite) TestRespond_ForeignElicitationIsReportedMissing() {
	foreign := s.pendingElicitation()
	foreign.SessionID = "ses_someone_else"

	s.store.EXPECT().GetSession(gomock.Any(), testSessionID).Return(s.session(), nil)
	s.store.EXPECT().GetAgentElicitation(gomock.Any(), testElicitationID).Return(foreign, nil)
	s.store.EXPECT().TransitionAgentElicitation(
		gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Times(0)

	rr := s.respond(testSessionID, testElicitationID, &types.User{ID: testOwnerID},
		`{"action":"accept","content":{}}`)
	s.Equal(http.StatusNotFound, rr.Code, "cross-session id must not be distinguishable from a missing one")
}

// A pending question implies a live agent blocked on it. With no WebSocket the answer
// has nowhere to go, so claiming it would strand the row in submitting.
func (s *ElicitationSuite) TestRespond_NoAgentConnectedIs409() {
	s.store.EXPECT().GetSession(gomock.Any(), testSessionID).Return(s.session(), nil)
	s.store.EXPECT().GetAgentElicitation(gomock.Any(), testElicitationID).Return(s.pendingElicitation(), nil)
	s.store.EXPECT().TransitionAgentElicitation(
		gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Times(0)

	rr := s.respond(testSessionID, testElicitationID, &types.User{ID: testOwnerID},
		`{"action":"accept","content":{}}`)
	s.Equal(http.StatusConflict, rr.Code)
	s.Contains(rr.Body.String(), "not connected")
}

func (s *ElicitationSuite) TestRespond_RejectsUnknownAction() {
	rr := s.respond(testSessionID, testElicitationID, &types.User{ID: testOwnerID},
		`{"action":"maybe"}`)
	s.Equal(http.StatusBadRequest, rr.Code)
}

// Decline is a first-class action, not an abort: the adapter turns it into an empty
// answer set and the turn continues. Hence the UI label "Skip".
func (s *ElicitationSuite) TestRespond_DeclineIsAccepted() {
	sendChan := s.connectAgent(testSessionID)

	s.store.EXPECT().GetSession(gomock.Any(), testSessionID).Return(s.session(), nil)
	s.store.EXPECT().GetAgentElicitation(gomock.Any(), testElicitationID).Return(s.pendingElicitation(), nil)
	s.store.EXPECT().TransitionAgentElicitation(
		gomock.Any(), testElicitationID, []string{types.ElicitationStatusPending},
		types.ElicitationStatusSubmitting, "", gomock.Any(),
	).Return(true, nil)

	rr := s.respond(testSessionID, testElicitationID, &types.User{ID: testOwnerID}, `{"action":"decline"}`)
	s.Equal(http.StatusOK, rr.Code, "body=%s", rr.Body.String())

	cmd := <-sendChan
	s.Equal("decline", cmd.Data["action"])
}

// If the send fails after the claim, the claim is rolled back so the user can retry
// rather than the question sitting in submitting until the reaper retires it.
func (s *ElicitationSuite) TestRespond_RollsBackClaimWhenSendFails() {
	// A full send channel makes sendCommandToExternalAgent fail deterministically.
	sendChan := make(chan types.ExternalAgentCommand, 1)
	sendChan <- types.ExternalAgentCommand{Type: "filler"}
	s.server.externalAgentWSManager.mu.Lock()
	s.server.externalAgentWSManager.connections[testSessionID] = &ExternalAgentWSConnection{
		SessionID: testSessionID, SendChan: sendChan, ConnectedAt: time.Now(),
	}
	s.server.externalAgentWSManager.mu.Unlock()

	s.store.EXPECT().GetSession(gomock.Any(), testSessionID).Return(s.session(), nil)
	s.store.EXPECT().GetAgentElicitation(gomock.Any(), testElicitationID).Return(s.pendingElicitation(), nil)

	gomock.InOrder(
		s.store.EXPECT().TransitionAgentElicitation(
			gomock.Any(), testElicitationID, []string{types.ElicitationStatusPending},
			types.ElicitationStatusSubmitting, "", gomock.Any(),
		).Return(true, nil),
		// The rollback: submitting → pending, so the card becomes answerable again.
		s.store.EXPECT().TransitionAgentElicitation(
			gomock.Any(), testElicitationID, []string{types.ElicitationStatusSubmitting},
			types.ElicitationStatusPending, "", gomock.Nil(),
		).Return(true, nil),
	)

	rr := s.respond(testSessionID, testElicitationID, &types.User{ID: testOwnerID},
		`{"action":"accept","content":{"question_0":"redis"}}`)
	s.Equal(http.StatusInternalServerError, rr.Code)
}

// ──────────────────────────────────────────────────────────────────────────────
// GET /api/v1/sessions/{id}/elicitations
// ──────────────────────────────────────────────────────────────────────────────

func (s *ElicitationSuite) TestList_ReturnsLiveQuestionsForOwner() {
	s.store.EXPECT().GetSession(gomock.Any(), testSessionID).Return(s.session(), nil)
	s.store.EXPECT().ListLiveAgentElicitationsForSession(gomock.Any(), testSessionID).
		Return([]*types.AgentElicitation{s.pendingElicitation()}, nil)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/sessions/"+testSessionID+"/elicitations", nil)
	req = mux.SetURLVars(req, map[string]string{"id": testSessionID})
	req = req.WithContext(setRequestUser(req.Context(), types.User{ID: testOwnerID}))
	rr := httptest.NewRecorder()
	system.Wrapper(s.server.listSessionElicitations)(rr, req)

	s.Equal(http.StatusOK, rr.Code, "body=%s", rr.Body.String())

	var out []*types.AgentElicitation
	s.Require().NoError(json.Unmarshal(rr.Body.Bytes(), &out))
	s.Require().Len(out, 1)
	s.Equal(testElicitationID, out[0].ID)
}

func (s *ElicitationSuite) TestList_CrossUserIsForbidden() {
	s.store.EXPECT().GetSession(gomock.Any(), testSessionID).Return(s.session(), nil)
	s.store.EXPECT().ListLiveAgentElicitationsForSession(gomock.Any(), gomock.Any()).Times(0)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/sessions/"+testSessionID+"/elicitations", nil)
	req = mux.SetURLVars(req, map[string]string{"id": testSessionID})
	req = req.WithContext(setRequestUser(req.Context(), types.User{ID: "someone-else"}))
	rr := httptest.NewRecorder()
	system.Wrapper(s.server.listSessionElicitations)(rr, req)

	s.Equal(http.StatusForbidden, rr.Code)
}

// ──────────────────────────────────────────────────────────────────────────────
// Gate 0: auto-wake must leave a turn waiting on a human alone
// ──────────────────────────────────────────────────────────────────────────────

// Waking an interaction that is blocked on a question would cancel the outstanding
// question (a new turn cancels outstanding elicitations) and replace the user's pending
// answer with a "continue" prompt — turning a working feature into a lost question.
func (s *ElicitationSuite) TestAutoWake_SkipsInteractionBlockedOnUserQuestion() {
	stuck := &types.Interaction{
		ID:        testInteractionID,
		SessionID: testSessionID,
		State:     types.InteractionStateWaiting,
		Created:   time.Now().Add(-2 * time.Hour),
		Updated:   time.Now().Add(-2 * time.Hour),
	}

	s.store.EXPECT().HasLiveAgentElicitation(gomock.Any(), testInteractionID).Return(true, nil)
	// Gate 0 must return before any other gate touches the store or the agent.
	s.store.EXPECT().GetSession(gomock.Any(), gomock.Any()).Times(0)
	s.store.EXPECT().IncrementInteractionAutoWakeCount(gomock.Any(), gomock.Any()).Times(0)

	s.server.maybeAutoWake(context.Background(), stuck)
}

// If the blocked-on-human check itself fails we skip rather than guess — waking a turn
// that might be waiting on a person is the more destructive error.
func (s *ElicitationSuite) TestAutoWake_SkipsWhenTheBlockedCheckFails() {
	stuck := &types.Interaction{
		ID:        testInteractionID,
		SessionID: testSessionID,
		State:     types.InteractionStateWaiting,
		Created:   time.Now().Add(-2 * time.Hour),
	}

	s.store.EXPECT().HasLiveAgentElicitation(gomock.Any(), testInteractionID).
		Return(false, fmt.Errorf("db down"))
	s.store.EXPECT().GetSession(gomock.Any(), gomock.Any()).Times(0)

	s.server.maybeAutoWake(context.Background(), stuck)
}

// ──────────────────────────────────────────────────────────────────────────────
// The prompt queue must not defer a follow-up behind a pending question
// ──────────────────────────────────────────────────────────────────────────────

// Baseline: a genuinely busy session (waiting, no question) still defers, so the test
// below is proving the exception rather than the absence of the busy check.
func (s *ElicitationSuite) TestPromptQueue_DefersWhenBusyWithoutAQuestion() {
	s.store.EXPECT().GetSession(gomock.Any(), testSessionID).Return(s.session(), nil)
	s.store.EXPECT().ListInteractions(gomock.Any(), gomock.Any()).
		Return([]*types.Interaction{s.interaction()}, int64(1), nil)
	s.store.EXPECT().HasLiveAgentElicitation(gomock.Any(), testInteractionID).Return(false, nil)

	// Deferred means we never even look for a prompt to send.
	s.store.EXPECT().GetNextPendingPrompt(gomock.Any(), gomock.Any()).Times(0)

	s.server.processPromptQueue(context.Background(), testSessionID)
}

// A pending question holds the interaction in `waiting` indefinitely, and auto-wake
// deliberately leaves it alone — so deferring here would strand the user's message
// forever with no feedback. Dispatching is also what Zed does: starting a turn cancels
// outstanding elicitations, so the question resolves as cancelled and the new turn
// proceeds. Helix does not write that status itself; the agent reports it back.
func (s *ElicitationSuite) TestPromptQueue_DispatchesFollowUpWhenBlockedOnAQuestion() {
	s.store.EXPECT().GetSession(gomock.Any(), testSessionID).Return(s.session(), nil).AnyTimes()
	s.store.EXPECT().ListInteractions(gomock.Any(), gomock.Any()).
		Return([]*types.Interaction{s.interaction()}, int64(1), nil).AnyTimes()
	s.store.EXPECT().HasLiveAgentElicitation(gomock.Any(), testInteractionID).Return(true, nil)

	// The proof: the drain proceeds past the busy check and looks for a prompt.
	s.store.EXPECT().GetNextPendingPrompt(gomock.Any(), testSessionID).Return(nil, nil)

	s.server.processPromptQueue(context.Background(), testSessionID)
}

// If the blocked-on-human check errors we treat the session as busy and defer — the
// safe direction, since dispatching on top of a genuinely streaming turn is worse than
// a delayed message.
func (s *ElicitationSuite) TestPromptQueue_DefersWhenTheBlockedCheckFails() {
	s.store.EXPECT().GetSession(gomock.Any(), testSessionID).Return(s.session(), nil)
	s.store.EXPECT().ListInteractions(gomock.Any(), gomock.Any()).
		Return([]*types.Interaction{s.interaction()}, int64(1), nil)
	s.store.EXPECT().HasLiveAgentElicitation(gomock.Any(), testInteractionID).
		Return(false, fmt.Errorf("db down"))
	s.store.EXPECT().GetNextPendingPrompt(gomock.Any(), gomock.Any()).Times(0)

	s.server.processPromptQueue(context.Background(), testSessionID)
}
