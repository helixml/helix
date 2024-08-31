package knowledge

import (
	"context"
	"testing"

	"github.com/helixml/helix/api/pkg/config"
	"github.com/helixml/helix/api/pkg/controller/knowledge/crawler"
	"github.com/helixml/helix/api/pkg/extract"
	"github.com/helixml/helix/api/pkg/rag"
	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/types"
	"go.uber.org/mock/gomock"

	"github.com/stretchr/testify/suite"
)

type ExtractorSuite struct {
	suite.Suite

	ctx context.Context

	extractor *extract.MockExtractor
	crawler   *crawler.MockCrawler
	store     *store.MockStore
	rag       *rag.MockRAG

	cfg *config.ServerConfig

	reconciler *Reconciler
}

func TestExtractorSuite(t *testing.T) {
	suite.Run(t, new(ExtractorSuite))
}

func (suite *ExtractorSuite) SetupTest() {
	ctrl := gomock.NewController(suite.T())

	suite.ctx = context.Background()
	suite.extractor = extract.NewMockExtractor(ctrl)
	suite.crawler = crawler.NewMockCrawler(ctrl)
	suite.store = store.NewMockStore(ctrl)
	suite.rag = rag.NewMockRAG(ctrl)

	suite.cfg = &config.ServerConfig{}

	suite.reconciler, _ = New(suite.cfg, suite.store, suite.extractor, nil)

	suite.reconciler.newRagClient = func(settings *types.RAGSettings) rag.RAG {
		return suite.rag
	}

	suite.reconciler.newCrawler = func(k *types.Knowledge) (crawler.Crawler, error) {
		return suite.crawler, nil
	}
}

func (suite *ExtractorSuite) Test_getIndexingData_CrawlerEnabled() {
	knowledge := &types.Knowledge{
		ID: "knowledge_id",
		Source: types.KnowledgeSource{
			Web: &types.KnowledgeSourceWeb{
				URLs: []string{"https://example.com"},
				Crawler: &types.WebsiteCrawler{
					Enabled: true,
				},
			},
		},
	}

	suite.crawler.EXPECT().Crawl(gomock.Any()).Return([]*types.CrawledDocument{
		{
			Content:   "Hello, world!",
			SourceURL: "https://example.com",
		},
	}, nil)

	suite.reconciler.extractDataFromWebWithCrawler(suite.ctx, knowledge)
}
