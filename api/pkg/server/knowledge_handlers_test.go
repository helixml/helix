package server

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/helixml/helix/api/pkg/config"
	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/stretchr/testify/suite"

	"go.uber.org/mock/gomock"
)

// fakeKnowledgeManager is a no-op implementation of knowledge.Manager so the
// listKnowledge handler can populate ephemeral progress without a real
// reconciler.
type fakeKnowledgeManager struct{}

func (fakeKnowledgeManager) NextRun(_ context.Context, _ string) (time.Time, error) {
	return time.Time{}, nil
}

func (fakeKnowledgeManager) GetStatus(_ string) types.KnowledgeProgress {
	return types.KnowledgeProgress{}
}

type ListKnowledgeSuite struct {
	suite.Suite

	ctrl  *gomock.Controller
	store *store.MockStore

	authCtx context.Context
	userID  string

	server *HelixAPIServer
}

func TestListKnowledgeSuite(t *testing.T) {
	suite.Run(t, new(ListKnowledgeSuite))
}

func (suite *ListKnowledgeSuite) SetupTest() {
	ctrl := gomock.NewController(suite.T())
	suite.ctrl = ctrl
	suite.store = store.NewMockStore(ctrl)

	suite.userID = "user_id_test"
	suite.authCtx = setRequestUser(context.Background(), types.User{
		ID:   suite.userID,
		Type: types.OwnerTypeUser,
	})

	suite.server = &HelixAPIServer{
		Cfg:              &config.ServerConfig{},
		Store:            suite.store,
		knowledgeManager: fakeKnowledgeManager{},
	}
}

func (suite *ListKnowledgeSuite) newRequest(query string) *http.Request {
	req := httptest.NewRequest("GET", "/api/v1/knowledge"+query, http.NoBody)
	return req.WithContext(suite.authCtx)
}

// When app_id is set, knowledge must be authorized against the app and listed
// by app_id alone — NOT by owner-equality. Regression test for the bug where an
// authorized non-owner (e.g. an org owner viewing a member's project agent) saw
// an empty knowledge list because rows are stamped with the app owner's id.
func (suite *ListKnowledgeSuite) TestAppScoped_AuthorizedNonOwner_NoOwnerFilter() {
	// Global app owned by a different user — the requesting user is authorized
	// via Global, not ownership.
	app := &types.App{
		ID:     "app_id_test",
		Owner:  "some_other_owner",
		Global: true,
	}
	suite.store.EXPECT().GetApp(gomock.Any(), app.ID).Return(app, nil)

	var captured *store.ListKnowledgeQuery
	suite.store.EXPECT().
		ListKnowledge(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, q *store.ListKnowledgeQuery) ([]*types.Knowledge, error) {
			captured = q
			return []*types.Knowledge{
				{ID: "kno_test", Name: "doc", AppID: app.ID, Owner: app.Owner},
			}, nil
		})

	knowledge, httpErr := suite.server.listKnowledge(httptest.NewRecorder(), suite.newRequest("?app_id=app_id_test"))

	suite.Nil(httpErr)
	suite.Require().Len(knowledge, 1)
	suite.Equal("kno_test", knowledge[0].ID)

	// The crucial assertion: scoped by app, with no owner filter.
	suite.Require().NotNil(captured)
	suite.Equal(app.ID, captured.AppID)
	suite.Empty(captured.Owner, "owner filter must not be applied when app_id is set")
	suite.Empty(captured.OwnerType, "owner_type filter must not be applied when app_id is set")
}

// A user with no access to the app must be denied, and the store must never be
// queried for its knowledge.
func (suite *ListKnowledgeSuite) TestAppScoped_Unauthorized() {
	app := &types.App{
		ID:     "app_id_test",
		Owner:  "some_other_owner",
		Global: false,
	}
	suite.store.EXPECT().GetApp(gomock.Any(), app.ID).Return(app, nil)
	// No ListKnowledge expectation — it must not be called.

	knowledge, httpErr := suite.server.listKnowledge(httptest.NewRecorder(), suite.newRequest("?app_id=app_id_test"))

	suite.Nil(knowledge)
	suite.Require().NotNil(httpErr)
	suite.Equal(http.StatusForbidden, httpErr.StatusCode)
}

func (suite *ListKnowledgeSuite) TestAppScoped_AppNotFound() {
	suite.store.EXPECT().GetApp(gomock.Any(), "app_id_test").Return(nil, store.ErrNotFound)

	knowledge, httpErr := suite.server.listKnowledge(httptest.NewRecorder(), suite.newRequest("?app_id=app_id_test"))

	suite.Nil(knowledge)
	suite.Require().NotNil(httpErr)
	suite.Equal(http.StatusNotFound, httpErr.StatusCode)
}

// Without app_id (or org), listing stays scoped to the requesting user's own
// knowledge — the personal-list behaviour is unchanged.
func (suite *ListKnowledgeSuite) TestPersonalScoped_NoAppID() {
	var captured *store.ListKnowledgeQuery
	suite.store.EXPECT().
		ListKnowledge(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, q *store.ListKnowledgeQuery) ([]*types.Knowledge, error) {
			captured = q
			return []*types.Knowledge{}, nil
		})

	_, httpErr := suite.server.listKnowledge(httptest.NewRecorder(), suite.newRequest(""))

	suite.Nil(httpErr)
	suite.Require().NotNil(captured)
	suite.Equal(suite.userID, captured.Owner)
	suite.Equal(types.OwnerTypeUser, captured.OwnerType)
	suite.Empty(captured.AppID)
}
