package query

import (
	"context"
	"fmt"
	"os"
	"testing"

	"github.com/helixml/helix/api/pkg/config"
	"github.com/helixml/helix/api/pkg/openai"
	"github.com/helixml/helix/api/pkg/rag"
	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/types"

	"github.com/kelseyhightower/envconfig"
	"github.com/stretchr/testify/suite"
	"go.uber.org/mock/gomock"
)

type QuerySuite struct {
	suite.Suite
	ctrl  *gomock.Controller
	ctx   context.Context
	store *store.MockStore
	rag   rag.RAG
	query *Query
}

func TestQuerySuite(t *testing.T) {
	suite.Run(t, new(QuerySuite))
}

func (suite *QuerySuite) SetupTest() {
	suite.ctx = context.Background()
	ctrl := gomock.NewController(suite.T())

	ragCfg := &types.RAGSettings{}
	ragCfg.Typesense.URL = "http://localhost:8108"
	ragCfg.Typesense.APIKey = "typesense"
	ragCfg.Typesense.Collection = "helix-documents"

	if os.Getenv("TYPESENSE_URL") != "" {
		ragCfg.Typesense.URL = os.Getenv("TYPESENSE_URL")
	}
	if os.Getenv("TYPESENSE_API_KEY") != "" {
		ragCfg.Typesense.APIKey = os.Getenv("TYPESENSE_API_KEY")
	}

	ts, err := rag.NewTypesense(ragCfg)
	suite.Require().NoError(err)

	suite.rag = ts

	suite.store = store.NewMockStore(ctrl)

	var cfg config.ServerConfig
	err = envconfig.Process("", &cfg)
	suite.NoError(err)

	var apiClient openai.Client

	if cfg.Providers.TogetherAI.APIKey != "" {
		apiClient = openai.New(
			cfg.Providers.TogetherAI.APIKey,
			cfg.Providers.TogetherAI.BaseURL,
			cfg.Stripe.BillingEnabled,
		)
		cfg.Tools.Model = "openai/gpt-oss-20b"
	} else {
		apiClient = openai.NewMockClient(suite.ctrl)
	}

	suite.query = New(&Config{
		store:     suite.store,
		apiClient: apiClient,
		getRAGClient: func(_ context.Context, _ *types.Knowledge) (rag.RAG, error) {
			return suite.rag, nil
		},
		model: model,
	})
}

func (suite *QuerySuite) TestAnswer() {
	// TODO:
	suite.T().Skip()

	knowledge := &types.Knowledge{
		Name:    "jenkins",
		ID:      "kno_01j8ga2sr3cgkkycrnyzjmvppy",
		AppID:   "app_01j8ab5q5xpfc3kgxt0a8j3ghd",
		Version: "2024-10-13_15-07-27",
		State:   types.KnowledgeStateReady,
	}

	suite.store.EXPECT().LookupKnowledge(suite.ctx, gomock.Any()).Return(knowledge, nil)

	answer, err := suite.query.Answer(suite.ctx, "How to make HTTP call with a function?", knowledge.AppID, &types.AssistantConfig{
		Knowledge: []*types.AssistantKnowledge{
			{
				Name: knowledge.Name,
			},
		},
	})
	suite.NoError(err)

	fmt.Println(answer)
}
