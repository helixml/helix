package server

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gorilla/mux"
	"github.com/helixml/helix/api/pkg/config"
	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/system"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/stretchr/testify/suite"
	"go.uber.org/mock/gomock"
)

type ListAppInteractionsSuite struct {
	suite.Suite

	ctrl  *gomock.Controller
	store *store.MockStore

	authCtx context.Context
	userID  string
	appID   string

	server *HelixAPIServer
}

func TestListAppInteractionsSuite(t *testing.T) {
	suite.Run(t, new(ListAppInteractionsSuite))
}

func (suite *ListAppInteractionsSuite) SetupTest() {
	ctrl := gomock.NewController(suite.T())
	suite.ctrl = ctrl
	suite.store = store.NewMockStore(ctrl)

	suite.userID = "user_id_test"
	suite.appID = "app_id_test"

	suite.authCtx = setRequestUser(context.Background(), types.User{
		ID:       suite.userID,
		Email:    "foo@email.com",
		FullName: "Foo Bar",
	})

	suite.server = &HelixAPIServer{
		Cfg:   &config.ServerConfig{},
		Store: suite.store,
	}
}

func (suite *ListAppInteractionsSuite) call(query string) (*types.PaginatedInteractions, *system.HTTPError) {
	req := httptest.NewRequest("GET", "/api/v1/apps/"+suite.appID+"/interactions?"+query, http.NoBody)
	req = req.WithContext(suite.authCtx)
	req = mux.SetURLVars(req, map[string]string{"id": suite.appID})

	app := &types.App{ID: suite.appID, Owner: suite.userID}
	suite.store.EXPECT().GetApp(gomock.Any(), suite.appID).Return(app, nil)

	resp, httpErr := suite.server.listAppInteractions(httptest.NewRecorder(), req)
	return resp, httpErr
}

// Page is 0-indexed end-to-end: the UI sends page=0 for the first page and the
// store receives Page=0 so the SQL offset is 0. Sending page=1 used to skip
// pageSize rows and return empty when fewer rows existed.
func (suite *ListAppInteractionsSuite) TestListAppInteractions_Page0FirstPage() {
	suite.store.EXPECT().
		ListInteractions(gomock.Any(), gomock.AssignableToTypeOf(&types.ListInteractionsQuery{})).
		DoAndReturn(func(_ context.Context, q *types.ListInteractionsQuery) ([]*types.Interaction, int64, error) {
			suite.Equal(0, q.Page, "page=0 from caller must reach the store as Page=0")
			suite.Equal(100, q.PerPage)
			suite.Equal(suite.appID, q.AppID)
			suite.Equal(suite.userID, q.UserID)
			return []*types.Interaction{{ID: "i_1"}}, 1, nil
		})

	resp, httpErr := suite.call("page=0&pageSize=100")

	suite.Require().Nil(httpErr)
	suite.Require().Len(resp.Interactions, 1)
	suite.Equal("i_1", resp.Interactions[0].ID)
}

func (suite *ListAppInteractionsSuite) TestListAppInteractions_Page1SecondPage() {
	suite.store.EXPECT().
		ListInteractions(gomock.Any(), gomock.AssignableToTypeOf(&types.ListInteractionsQuery{})).
		DoAndReturn(func(_ context.Context, q *types.ListInteractionsQuery) ([]*types.Interaction, int64, error) {
			suite.Equal(1, q.Page, "page=1 is the second page (offset = 1*pageSize)")
			suite.Equal(50, q.PerPage)
			return []*types.Interaction{}, 0, nil
		})

	_, httpErr := suite.call("page=1&pageSize=50")
	suite.Require().Nil(httpErr)
}

// Missing/invalid page should default to Page=0.
func (suite *ListAppInteractionsSuite) TestListAppInteractions_DefaultPage() {
	suite.store.EXPECT().
		ListInteractions(gomock.Any(), gomock.AssignableToTypeOf(&types.ListInteractionsQuery{})).
		DoAndReturn(func(_ context.Context, q *types.ListInteractionsQuery) ([]*types.Interaction, int64, error) {
			suite.Equal(0, q.Page)
			suite.Equal(10, q.PerPage)
			return []*types.Interaction{}, 0, nil
		})

	_, httpErr := suite.call("")
	suite.Require().Nil(httpErr)
}

// When feedback filter is supplied the handler must clear UserID so feedback
// from any user of the app is returned.
func (suite *ListAppInteractionsSuite) TestListAppInteractions_FeedbackClearsUserID() {
	suite.store.EXPECT().
		ListInteractions(gomock.Any(), gomock.AssignableToTypeOf(&types.ListInteractionsQuery{})).
		DoAndReturn(func(_ context.Context, q *types.ListInteractionsQuery) ([]*types.Interaction, int64, error) {
			suite.Equal("like", q.Feedback)
			suite.Empty(q.UserID, "feedback queries must drop the UserID filter")
			suite.Equal(0, q.Page)
			return []*types.Interaction{}, 0, nil
		})

	_, httpErr := suite.call("page=0&pageSize=100&feedback=like")
	suite.Require().Nil(httpErr)
}

func (suite *ListAppInteractionsSuite) TestListAppInteractions_InvalidFeedback() {
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/api/v1/apps/"+suite.appID+"/interactions?feedback=meh", http.NoBody)
	req = req.WithContext(suite.authCtx)
	req = mux.SetURLVars(req, map[string]string{"id": suite.appID})

	app := &types.App{ID: suite.appID, Owner: suite.userID}
	suite.store.EXPECT().GetApp(gomock.Any(), suite.appID).Return(app, nil)
	// ListInteractions must NOT be called for invalid feedback.

	_, httpErr := suite.server.listAppInteractions(rec, req)
	suite.Require().NotNil(httpErr)
	suite.Equal(http.StatusBadRequest, httpErr.StatusCode)
}

// Non-owner without admin or org membership must be rejected before any
// ListInteractions call happens.
func (suite *ListAppInteractionsSuite) TestListAppInteractions_Forbidden() {
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/api/v1/apps/"+suite.appID+"/interactions", http.NoBody)
	req = req.WithContext(suite.authCtx)
	req = mux.SetURLVars(req, map[string]string{"id": suite.appID})

	app := &types.App{ID: suite.appID, Owner: "someone_else"}
	suite.store.EXPECT().GetApp(gomock.Any(), suite.appID).Return(app, nil)

	_, httpErr := suite.server.listAppInteractions(rec, req)
	suite.Require().NotNil(httpErr)
	suite.Equal(http.StatusForbidden, httpErr.StatusCode)
}

// TotalPages must be computed from totalCount and pageSize, and the response
// echoes the 0-indexed page the caller asked for.
func (suite *ListAppInteractionsSuite) TestListAppInteractions_TotalPagesAndEchoedPage() {
	suite.store.EXPECT().
		ListInteractions(gomock.Any(), gomock.AssignableToTypeOf(&types.ListInteractionsQuery{})).
		DoAndReturn(func(_ context.Context, q *types.ListInteractionsQuery) ([]*types.Interaction, int64, error) {
			suite.Equal(2, q.Page)
			suite.Equal(10, q.PerPage)
			return []*types.Interaction{{ID: "x"}}, 25, nil
		})

	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/api/v1/apps/"+suite.appID+"/interactions?page=2&pageSize=10", http.NoBody)
	req = req.WithContext(suite.authCtx)
	req = mux.SetURLVars(req, map[string]string{"id": suite.appID})

	app := &types.App{ID: suite.appID, Owner: suite.userID}
	suite.store.EXPECT().GetApp(gomock.Any(), suite.appID).Return(app, nil)

	resp, httpErr := suite.server.listAppInteractions(rec, req)
	suite.Require().Nil(httpErr)
	suite.Equal(int64(25), resp.TotalCount)
	suite.Equal(3, resp.TotalPages, "ceil(25/10) == 3")
	suite.Equal(2, resp.Page, "echoed page is the 0-indexed value the caller sent")
	suite.Equal(10, resp.PageSize)
}
