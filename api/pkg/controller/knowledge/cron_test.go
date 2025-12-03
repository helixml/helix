package knowledge

import (
	"context"
	"testing"

	"github.com/helixml/helix/api/pkg/config"
	"github.com/helixml/helix/api/pkg/controller/knowledge/browser"
	"github.com/helixml/helix/api/pkg/controller/knowledge/crawler"
	"github.com/helixml/helix/api/pkg/extract"
	"github.com/helixml/helix/api/pkg/filestore"
	"github.com/helixml/helix/api/pkg/rag"
	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/types"

	"github.com/stretchr/testify/suite"
	"go.uber.org/mock/gomock"
)

type CronSuite struct {
	suite.Suite

	ctx context.Context

	extractor *extract.MockExtractor
	crawler   *crawler.MockCrawler
	store     *store.MockStore
	rag       *rag.MockRAG
	filestore *filestore.MockFileStore

	cfg *config.ServerConfig

	reconciler *Reconciler
}

func TestCronSuite(t *testing.T) {
	suite.Run(t, new(CronSuite))
}

func (suite *CronSuite) SetupTest() {
	ctrl := gomock.NewController(suite.T())

	suite.ctx = context.Background()
	suite.extractor = extract.NewMockExtractor(ctrl)
	suite.crawler = crawler.NewMockCrawler(ctrl)
	suite.store = store.NewMockStore(ctrl)
	suite.rag = rag.NewMockRAG(ctrl)
	suite.filestore = filestore.NewMockFileStore(ctrl)

	suite.cfg = &config.ServerConfig{}

	var err error

	b := &browser.Browser{}

	suite.reconciler, err = New(suite.cfg, suite.store, suite.filestore, suite.extractor, suite.rag, b, nil)
	suite.Require().NoError(err)

	suite.reconciler.newRagClient = func(_ *types.RAGSettings) rag.RAG {
		return suite.rag
	}

	suite.reconciler.newCrawler = func(_ *types.Knowledge) (crawler.Crawler, error) {
		return suite.crawler, nil
	}
}

func (suite *CronSuite) Test_CreateJob_Daily() {
	k := &types.Knowledge{
		ID:              "knowledge_id",
		RefreshEnabled:  true,
		RefreshSchedule: "0 0 * * *",
		Source: types.KnowledgeSource{
			Web: &types.KnowledgeSourceWeb{
				URLs: []string{"https://example.com"},
			},
		},
	}

	jobs := suite.reconciler.cron.Jobs()

	err := suite.reconciler.createOrDeleteCronJobs(suite.ctx, []*types.Knowledge{k}, jobs)
	suite.Require().NoError(err)

	jobs = suite.reconciler.cron.Jobs()

	// We should have 1 job
	suite.Require().Len(jobs, 1)

	// Check name
	suite.Require().Equal(jobs[0].Name(), "knowledge_id")

	// Check tags
	suite.Require().Equal(jobs[0].Tags(), []string{"schedule:0 0 * * *"})
}

func (suite *CronSuite) Test_CreateJob_Daily_Humanized() {
	k := &types.Knowledge{
		ID:             "knowledge_id",
		RefreshEnabled: true,
		// Setting to timezone BST
		RefreshSchedule: "TZ=UTC @daily",
		Source: types.KnowledgeSource{
			Web: &types.KnowledgeSourceWeb{
				URLs: []string{"https://example.com"},
			},
		},
	}

	jobs := suite.reconciler.cron.Jobs()

	suite.reconciler.cron.Start()

	err := suite.reconciler.createOrDeleteCronJobs(suite.ctx, []*types.Knowledge{k}, jobs)
	suite.Require().NoError(err)

	jobs = suite.reconciler.cron.Jobs()

	// We should have 1 job
	suite.Require().Len(jobs, 1)

	// Check name
	suite.Require().Equal(jobs[0].Name(), "knowledge_id")

	// Check tags
	suite.Require().Equal(jobs[0].Tags(), []string{"schedule:TZ=UTC @daily"})

	// Check next run
	nextRun, err := jobs[0].NextRun()
	suite.Require().NoError(err)

	suite.False(nextRun.IsZero())
}
