package server

// import (
// 	"context"
// 	"net/http"
// 	"net/http/httptest"
// 	"testing"

// 	"github.com/helixml/helix/api/pkg/config"
// 	"github.com/helixml/helix/api/pkg/openai"
// 	"github.com/helixml/helix/api/pkg/openai/manager"
// 	"github.com/helixml/helix/api/pkg/store"
// 	"github.com/helixml/helix/api/pkg/types"
// 	"github.com/stretchr/testify/suite"
// 	"go.uber.org/mock/gomock"
// )

// type ProviderHandlersSuite struct {
// 	suite.Suite

// 	store        *store.MockStore
// 	openAiClient *openai.MockClient
// 	manager      *manager.MockProviderManager

// 	authCtx context.Context

// 	server *HelixAPIServer
// }

// func TestProviderHandlersSuite(t *testing.T) {
// 	suite.Run(t, new(ProviderHandlersSuite))
// }

// func (s *ProviderHandlersSuite) SetupTest() {
// 	ctrl := gomock.NewController(s.T())

// 	cfg := &config.ServerConfig{}
// 	cfg.RAG.PGVector.Provider = string(types.ProviderOpenAI)

// 	s.store = store.NewMockStore(ctrl)
// 	s.openAiClient = openai.NewMockClient(ctrl)
// 	s.manager = manager.NewMockProviderManager(ctrl)

// 	s.authCtx = setRequestUser(context.Background(), types.User{
// 		ID:       "user_id",
// 		Email:    "foo@email.com",
// 		FullName: "Foo Bar",
// 	})

// 	server := &HelixAPIServer{
// 		Cfg:             cfg,
// 		Store:           s.store,
// 		providerManager: s.manager,
// 	}

// 	s.server = server
// }

// func (s *ProviderHandlersSuite) TestListProviders() {
// 	s.store.EXPECT().ListProviderEndpoints(gomock.Any(), gomock.Any()).Return([]*types.ProviderEndpoint{
// 		{
// 			ID:     "openai",
// 			Models: []string{"gpt-4o", "gpt-4o-mini"},
// 		},
// 	}, nil)

// 	req, err := http.NewRequest("GET", "/v1/provider-endpoints", nil)
// 	s.Require().NoError(err)

// 	q := req.URL.Query()
// 	q.Add("with_models", "true")
// 	req.URL.RawQuery = q.Encode()

// 	rr := httptest.NewRecorder()
// 	s.server.listProviderEndpoints(rr, req)
// }
