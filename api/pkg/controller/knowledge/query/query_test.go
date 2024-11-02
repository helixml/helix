package query

import (
	"context"
	"fmt"
	"os"
	"testing"

	"github.com/helixml/helix/api/pkg/config"
	"github.com/helixml/helix/api/pkg/openai"
	oai "github.com/helixml/helix/api/pkg/openai"
	"github.com/helixml/helix/api/pkg/rag"
	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/types"

	"github.com/kelseyhightower/envconfig"
	"github.com/stretchr/testify/suite"
	"go.uber.org/mock/gomock"
)

type QuerySuite struct {
	suite.Suite
	ctrl         *gomock.Controller
	ctx          context.Context
	store        *store.MockStore
	rag          rag.RAG
	openAiClient *oai.MockClient
	query        *Query
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
			cfg.Providers.TogetherAI.BaseURL)
		cfg.Tools.Model = "meta-llama/Llama-3-8b-chat-hf"
	} else {
		apiClient = openai.NewMockClient(suite.ctrl)
	}

	suite.query = New(&QueryConfig{
		Store:     suite.store,
		APIClient: apiClient,
		GetRAGClient: func(ctx context.Context, knowledge *types.Knowledge) (rag.RAG, error) {
			return suite.rag, nil
		},
		Model: model,
	})
}

func (suite *QuerySuite) TestAnswer() {
	// TODO:
	// suite.T().Skip()

	knowledge := &types.Knowledge{
		Name:    "whr-docs",
		ID:      "kno_01jb21kpj5kjmngy419agfgbam",
		AppID:   "app_01jb21k5rgkxahj0wmrn42tyrx",
		Version: "2024-10-25_14-34-44",
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
