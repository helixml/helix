package server

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/gorilla/mux"
	"github.com/helixml/helix/api/pkg/controller"
	"github.com/helixml/helix/api/pkg/pubsub"
	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/system"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
	"go.uber.org/mock/gomock"
)

type InteractionQuestionHandlerSuite struct {
	suite.Suite
	ctrl   *gomock.Controller
	store  *store.MockStore
	server *HelixAPIServer
}

func TestInteractionQuestionHandlerSuite(t *testing.T) {
	suite.Run(t, new(InteractionQuestionHandlerSuite))
}

func (s *InteractionQuestionHandlerSuite) SetupTest() {
	s.ctrl = gomock.NewController(s.T())
	s.store = store.NewMockStore(s.ctrl)
	s.server = &HelixAPIServer{
		Store:                  s.store,
		externalAgentWSManager: NewExternalAgentWSManager(),
		Controller: &controller.Controller{
			Options: controller.Options{Store: s.store, PubSub: pubsub.NewNoop()},
		},
	}
}

func (s *InteractionQuestionHandlerSuite) TearDownTest() {
	s.ctrl.Finish()
}

func questionHandlerFixture() (*types.Interaction, *types.Session) {
	interaction := &types.Interaction{
		ID:        "int-question",
		SessionID: "ses-question",
		State:     types.InteractionStateWaiting,
		PendingQuestion: &types.PendingQuestion{
			RequestID: "request-1",
			Questions: []types.UserQuestion{{
				ID:       "deploy",
				Question: "Deploy now?",
				Options:  []types.UserQuestionOption{{Label: "Yes"}, {Label: "No"}},
			}},
		},
	}
	return interaction, &types.Session{ID: interaction.SessionID, Owner: "owner-1"}
}

func (s *InteractionQuestionHandlerSuite) callHandler(
	action string,
	body any,
	user *types.User,
) *httptest.ResponseRecorder {
	var encoded []byte
	if body != nil {
		var err error
		encoded, err = json.Marshal(body)
		if err != nil {
			panic(err)
		}
	}
	req := httptest.NewRequest(
		http.MethodPost,
		"/api/v1/interactions/int-question/questions/request-1/"+action,
		bytes.NewReader(encoded),
	)
	req = mux.SetURLVars(req, map[string]string{
		"interaction_id": "int-question",
		"request_id":     "request-1",
	})
	if user != nil {
		req = req.WithContext(setRequestUser(req.Context(), *user))
	}
	rr := httptest.NewRecorder()
	if action == "respond" {
		system.Wrapper(s.server.respondToInteractionQuestion)(rr, req)
	} else {
		system.Wrapper(s.server.cancelInteractionQuestion)(rr, req)
	}
	return rr
}

func (s *InteractionQuestionHandlerSuite) TestRejectsNonMember() {
	interaction, session := questionHandlerFixture()
	session.OrganizationID = "org-question"
	s.store.EXPECT().GetInteraction(gomock.Any(), interaction.ID).Return(interaction, nil)
	s.store.EXPECT().GetSession(gomock.Any(), session.ID).Return(session, nil)
	s.store.EXPECT().GetOrganizationMembership(gomock.Any(), &store.GetOrganizationMembershipQuery{
		OrganizationID: session.OrganizationID,
		UserID:         "intruder",
	}).Return(nil, store.ErrNotFound)

	rr := s.callHandler("respond", types.QuestionRespondRequest{
		Answers: map[string]string{"deploy": "No"},
	}, &types.User{ID: "intruder"})

	s.Equal(http.StatusForbidden, rr.Code)
}

func (s *InteractionQuestionHandlerSuite) TestReturnsConflictWhenAgentDisconnected() {
	interaction, session := questionHandlerFixture()
	s.store.EXPECT().GetInteraction(gomock.Any(), interaction.ID).Return(interaction, nil)
	// The failed send starts a best-effort wake goroutine, which reloads the
	// same non-desktop session and exits immediately.
	sessionLoads := make(chan struct{}, 2)
	s.store.EXPECT().GetSession(gomock.Any(), session.ID).
		DoAndReturn(func(context.Context, string) (*types.Session, error) {
			sessionLoads <- struct{}{}
			return session, nil
		}).Times(2)

	rr := s.callHandler("respond", types.QuestionRespondRequest{
		Answers: map[string]string{"deploy": "No"},
	}, &types.User{ID: session.Owner})

	s.Equal(http.StatusConflict, rr.Code)
	<-sessionLoads // authorization load completed synchronously
	select {
	case <-sessionLoads:
	case <-time.After(time.Second):
		s.Fail("timed out waiting for disconnected-agent wake attempt")
	}
}

func (s *InteractionQuestionHandlerSuite) TestDoubleAnswerAfterResolutionIsIdempotent() {
	interaction, session := questionHandlerFixture()
	interaction.QuestionHistory = []types.ResolvedQuestion{{
		PendingQuestion: *interaction.PendingQuestion,
		Outcome:         "answered",
	}}
	interaction.PendingQuestion = nil
	s.store.EXPECT().GetInteraction(gomock.Any(), interaction.ID).Return(interaction, nil)
	s.store.EXPECT().GetSession(gomock.Any(), session.ID).Return(session, nil)

	rr := s.callHandler("respond", types.QuestionRespondRequest{
		Answers: map[string]string{"deploy": "No"},
	}, &types.User{ID: session.Owner})

	s.Equal(http.StatusOK, rr.Code)
	s.Contains(rr.Body.String(), `"status":"resolved"`)
}

func (s *InteractionQuestionHandlerSuite) TestConcurrentAnswerAndCancelSendOneCommand() {
	interaction, session := questionHandlerFixture()
	s.store.EXPECT().GetInteraction(gomock.Any(), interaction.ID).Return(interaction, nil).Times(2)
	s.store.EXPECT().GetSession(gomock.Any(), session.ID).Return(session, nil).Times(2)
	sendChan := make(chan types.ExternalAgentCommand, 2)
	s.server.externalAgentWSManager.registerConnection(session.ID, &ExternalAgentWSConnection{
		SessionID: session.ID,
		SendChan:  sendChan,
	})

	start := make(chan struct{})
	results := make(chan *httptest.ResponseRecorder, 2)
	var ready sync.WaitGroup
	ready.Add(2)
	go func() {
		ready.Done()
		<-start
		results <- s.callHandler("respond", types.QuestionRespondRequest{
			Answers: map[string]string{"deploy": "No"},
		}, &types.User{ID: session.Owner})
	}()
	go func() {
		ready.Done()
		<-start
		results <- s.callHandler("cancel", nil, &types.User{ID: session.Owner})
	}()
	ready.Wait()
	close(start)

	first := <-results
	second := <-results
	s.Equal(http.StatusOK, first.Code)
	s.Equal(http.StatusOK, second.Code)
	s.Len(sendChan, 1)
}

func TestValidateQuestionAnswers(t *testing.T) {
	question := &types.PendingQuestion{Questions: []types.UserQuestion{
		{ID: "framework", Options: []types.UserQuestionOption{{Label: "React"}, {Label: "Vue"}}},
		{ID: "features", MultiSelect: true, Options: []types.UserQuestionOption{{Label: "Auth"}, {Label: "Billing"}}},
	}}

	require.NoError(t, validateQuestionAnswers(question, map[string]string{
		"framework": "React",
		"features":  "Auth\nBilling",
	}))
	require.Error(t, validateQuestionAnswers(question, map[string]string{
		"framework": "Svelte",
		"features":  "Auth",
	}))
	require.Error(t, validateQuestionAnswers(question, map[string]string{
		"framework": "React",
	}))
}

func TestValidatePendingQuestionRejectsOptionLabelNewlines(t *testing.T) {
	question := &types.PendingQuestion{
		RequestID:     "request-1",
		ThreadID:      "thread-1",
		TurnRequestID: "turn-1",
		Source:        "permission",
		Questions: []types.UserQuestion{{
			ID:       "features",
			Question: "Select features",
			Options:  []types.UserQuestionOption{{Label: "Auth\nBilling"}},
		}},
	}

	require.ErrorContains(t, validatePendingQuestion(question), "option label containing a newline")
}
