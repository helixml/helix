package query

import (
	"context"
	"fmt"
	"os"
	"testing"

	"github.com/helixml/helix/api/pkg/config"
	"github.com/helixml/helix/api/pkg/openai"
	oai "github.com/helixml/helix/api/pkg/openai"
	helix_langchain "github.com/helixml/helix/api/pkg/openai/langchain"
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

	knowledge := &types.Knowledge{
		Name:    "helix-docs",
		ID:      "kno_01jejsctgqj5kpxrphshcj80fr",
		AppID:   "app_01jejsctgm721hfn1mcrsme00k",
		Version: "2024-12-14_15-12-35",
		State:   types.KnowledgeStateReady,
	}

	suite.store.EXPECT().LookupKnowledge(suite.ctx, gomock.Any()).Return(knowledge, nil)

	answer, err := suite.query.Answer(suite.ctx, "what are the minimum requirements for installing helix?", knowledge.AppID, &types.AssistantConfig{
		Model: model,
		Knowledge: []*types.AssistantKnowledge{
			{
				Name: knowledge.Name,
			},
		},
	})
	suite.NoError(err)

	fmt.Println(answer)
}

func (suite *QuerySuite) Test_combineResults() {
	prompt := "what are the minimum requirements for installing helix?"
	results := []string{
		`## Requirements [Permalink for this section](\#requirements - **Control Plane** is the Helix API, web interface, and postgres database and requires: - Linux, macOS or Windows - [Docker](https://docs.docker.com/get-started/get-docker/) - 4 CPUs, 8GB RAM and 50GB+ free disk space - **Inference Provider** requires **ONE OF**: - An NVIDIA GPU if you want to use private Helix Runners ( [example]`,
		"Based on the provided context, the essential specifications for installing Helix are:\n\n1. Start an inference session on Helix.\n2. Download the Helix Installer and run the installer script.\n3. Edit the provider configuration in the `values-example.yaml` file to specify a remote provider (e.g. `openai` or `togetherai`) and provide API keys.\n4. Install the control plane helm chart with the latest images.\n5. Ensure all pods start and inspect the logs if they do not.\n6. Configure the Kubernetes deployment by overriding the settings in the `values.yaml` file.\n\nNote that these specifications are based on the provided context and may not be exhaustive.",
	}

	llm, err := helix_langchain.New(suite.query.apiClient, suite.query.model)
	suite.NoError(err)

	answer, err := suite.query.combineResults(suite.ctx, llm, prompt, results)
	suite.NoError(err)

	suite.Assert().Contains(answer, "4")
	suite.Assert().Contains(answer, "CPUs")
	suite.Assert().Contains(answer, "8GB RAM")

	fmt.Println(answer)
}
