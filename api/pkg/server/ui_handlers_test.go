package server

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/stretchr/testify/suite"
	"go.uber.org/mock/gomock"
)

type UIHandlerSuite struct {
	suite.Suite
	ctrl      *gomock.Controller
	ctx       context.Context
	store     *store.MockStore
	apiServer *HelixAPIServer
}

func TestUIHandlerSuite(t *testing.T) {
	suite.Run(t, new(UIHandlerSuite))
}

func (suite *UIHandlerSuite) SetupTest() {
	suite.ctrl = gomock.NewController(suite.T())
	suite.ctx = context.Background()
	suite.store = store.NewMockStore(suite.ctrl)
	suite.apiServer = &HelixAPIServer{
		Store: suite.store,
	}
}

// Helper functions
func (suite *UIHandlerSuite) createTestRequest(method, path string) *http.Request {
	req := httptest.NewRequest(method, path, nil)
	ctx := setRequestUser(context.Background(), *suite.createMockUser())
	return req.WithContext(ctx)
}

func (suite *UIHandlerSuite) createMockUser() *types.User {
	return &types.User{
		ID:       "user123",
		Email:    "test@example.com",
		Username: "test@example.com",
		FullName: "Test User",
	}
}

// Test cases
func (suite *UIHandlerSuite) TestUIAt() {
	testCases := []struct {
		name           string
		setupRequest   func() *http.Request
		setupMocks     func()
		expectedStatus int
		checkResponse  func(*types.UIAtResponse, error)
	}{
		{
			name: "No app_id returns empty results",
			setupRequest: func() *http.Request {
				return suite.createTestRequest("GET", "/api/v1/ui/at?q=123")
			},
			expectedStatus: http.StatusOK,
			checkResponse: func(resp *types.UIAtResponse, err error) {
				suite.Nil(err)
				suite.NotNil(resp)
				suite.Empty(resp.Data)
			},
		},
		{
			name: "Query with app_id returns all results",
			setupRequest: func() *http.Request {
				req := suite.createTestRequest("GET", "/api/v1/ui/at?app_id=app123")
				return req
			},
			setupMocks: func() {
				suite.store.EXPECT().
					ListKnowledge(gomock.Any(), &store.ListKnowledgeQuery{
						AppID: "app123",
						Owner: "user123",
					}).
					Return([]*types.Knowledge{
						{
							CrawledSources: &types.CrawledSources{
								URLs: []*types.CrawledURL{
									{URL: "http://example.com/test/doc1", DocumentID: "doc1"},
									{URL: "http://example.com/other/doc2", DocumentID: "doc2"},
								},
							},
						},
					}, nil)
			},
			expectedStatus: http.StatusOK,
			checkResponse: func(resp *types.UIAtResponse, err error) {
				suite.Nil(err)
				suite.NotNil(resp)
				suite.Len(resp.Data, 2) // Should return all results
			},
		},
		{
			name: "Query with app_id returns filtered results",
			setupRequest: func() *http.Request {
				req := suite.createTestRequest("GET", "/api/v1/ui/at?q=test&app_id=app123")
				return req
			},
			setupMocks: func() {
				suite.store.EXPECT().
					ListKnowledge(gomock.Any(), &store.ListKnowledgeQuery{
						AppID: "app123",
						Owner: "user123",
					}).
					Return([]*types.Knowledge{
						{
							CrawledSources: &types.CrawledSources{
								URLs: []*types.CrawledURL{
									{URL: "http://example.com/test/doc1", DocumentID: "doc1"},
									{URL: "http://example.com/other/doc2", DocumentID: "doc2"},
								},
							},
						},
					}, nil)
			},
			expectedStatus: http.StatusOK,
			checkResponse: func(resp *types.UIAtResponse, err error) {
				suite.Nil(err)
				suite.NotNil(resp)
				suite.Len(resp.Data, 1) // Only one result should match "test" in the URL
				suite.Equal("test/doc1", resp.Data[0].Label)
			},
		},
		{
			name: "Store error returns 500",
			setupRequest: func() *http.Request {
				req := suite.createTestRequest("GET", "/api/v1/ui/at?q=test&app_id=app123")
				return req
			},
			setupMocks: func() {
				suite.store.EXPECT().
					ListKnowledge(gomock.Any(), gomock.Any()).
					Return(nil, context.DeadlineExceeded)
			},
			expectedStatus: http.StatusInternalServerError,
			checkResponse: func(resp *types.UIAtResponse, err error) {
				suite.NotNil(err)
				suite.Nil(resp)
			},
		},
	}

	for _, tc := range testCases {
		suite.Run(tc.name, func() {
			if tc.setupMocks != nil {
				tc.setupMocks()
			}

			resp, err := suite.apiServer.uiAt(nil, tc.setupRequest())
			if err != nil {
				suite.Equal(tc.expectedStatus, err.StatusCode)
			}

			if tc.checkResponse != nil {
				tc.checkResponse(resp, err)
			}
		})
	}
}
