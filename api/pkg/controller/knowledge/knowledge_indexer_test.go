package knowledge

import (
	"context"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/helixml/helix/api/pkg/config"
	"github.com/helixml/helix/api/pkg/controller/knowledge/browser"
	"github.com/helixml/helix/api/pkg/controller/knowledge/crawler"
	"github.com/helixml/helix/api/pkg/dataprep/text"
	"github.com/helixml/helix/api/pkg/extract"
	"github.com/helixml/helix/api/pkg/filestore"
	"github.com/helixml/helix/api/pkg/rag"
	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/stretchr/testify/suite"
	"go.uber.org/mock/gomock"
)

type IndexerSuite struct {
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

func TestIndexerSuite(t *testing.T) {
	suite.Run(t, new(IndexerSuite))
}

func (suite *IndexerSuite) SetupTest() {
	ctrl := gomock.NewController(suite.T())

	suite.ctx = context.Background()
	suite.extractor = extract.NewMockExtractor(ctrl)
	suite.crawler = crawler.NewMockCrawler(ctrl)
	suite.store = store.NewMockStore(ctrl)
	suite.rag = rag.NewMockRAG(ctrl)
	suite.filestore = filestore.NewMockFileStore(ctrl)

	suite.cfg = &config.ServerConfig{}
	suite.cfg.RAG.IndexingConcurrency = 1

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

func (suite *IndexerSuite) TestIndex() {
	knowledge := &types.Knowledge{
		ID: "knowledge_id",
		RAGSettings: types.RAGSettings{
			TextSplitter: types.TextSplitterTypeText,
			ChunkSize:    2048,
		},
		Source: types.KnowledgeSource{
			Web: &types.KnowledgeSourceWeb{
				URLs: []string{"https://example.com"},
				Crawler: &types.WebsiteCrawler{
					Enabled: true,
				},
			},
		},
	}

	suite.store.EXPECT().ListKnowledge(gomock.Any(), gomock.Any()).Return([]*types.Knowledge{knowledge}, nil)

	suite.store.EXPECT().UpdateKnowledge(gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, k *types.Knowledge) (*types.Knowledge, error) {
			suite.Equal(types.KnowledgeStateIndexing, k.State)
			suite.Equal("", k.Message)
			suite.Equal("", k.Version, "version should be empty when we start indexing")

			return knowledge, nil
		},
	)

	// It will crawl the web
	suite.crawler.EXPECT().Crawl(gomock.Any()).Return([]*types.CrawledDocument{
		{
			Content:   `Hello world!`,
			SourceURL: "https://example.com",
		},
	}, nil)

	// Second call to update knowledge with info on URLs
	suite.store.EXPECT().UpdateKnowledge(gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, k *types.Knowledge) (*types.Knowledge, error) {
			suite.Equal(types.KnowledgeStateIndexing, k.State) // Still indexing
			suite.Equal(1, len(k.CrawledSources.URLs), "should have 1 crawled source")
			suite.Equal("https://example.com", k.CrawledSources.URLs[0].URL, "should have the correct URL")

			return knowledge, nil
		},
	)

	var version string

	// The metadata lookup is bypassed because the crawler data already has metadata
	// Remove this expectation to fix the test
	// suite.filestore.EXPECT().Get(gomock.Any(), "https://example.com/foo.metadata.yaml").Return(filestore.Item{}, fmt.Errorf("file not found"))

	// Then it will index it
	suite.rag.EXPECT().Index(gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, chunk *types.SessionRAGIndexChunk) error {
			// Split data entity id into knowledge id and version
			dataEntityIDParts := strings.SplitN(chunk.DataEntityID, "-", 2)
			suite.Equal(2, len(dataEntityIDParts))
			suite.Equal("knowledge_id", dataEntityIDParts[0])

			version = dataEntityIDParts[1]

			suite.Equal("https://example.com", chunk.Source)
			suite.Equal("Hello world!", chunk.Content)

			return nil
		},
	)

	suite.store.EXPECT().UpdateKnowledge(gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, k *types.Knowledge) (*types.Knowledge, error) {
			suite.Equal(types.KnowledgeStateReady, k.State)
			suite.Equal("", k.Message)

			suite.Equal(version, k.Version, "version should be set to the version we got from the data entity id")

			return knowledge, nil
		},
	)

	suite.store.EXPECT().UpdateKnowledgeState(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(nil).AnyTimes()

	suite.store.EXPECT().CreateKnowledgeVersion(gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, k *types.KnowledgeVersion) (*types.KnowledgeVersion, error) {
			suite.Equal(version, k.Version, "version should be set to the version we got from the data entity id")
			suite.Equal(types.KnowledgeStateReady, k.State, "knowledge should be ready")
			suite.Equal("", k.Message, "message should be empty")
			suite.Equal(knowledge.ID, k.KnowledgeID, "knowledge id should be set")

			return k, nil
		},
	)

	suite.store.EXPECT().ListKnowledgeVersions(gomock.Any(), &store.ListKnowledgeVersionQuery{
		KnowledgeID: knowledge.ID,
	}).Return([]*types.KnowledgeVersion{}, nil)

	// Start indexing
	err := suite.reconciler.index(suite.ctx)
	suite.NoError(err)

	// Wait for the goroutines to finish
	suite.reconciler.wg.Wait()
}

func (suite *IndexerSuite) TestIndex_ErrorNoFiles() {
	knowledge := &types.Knowledge{
		ID:    "knowledge_id",
		AppID: "app_id",
		RAGSettings: types.RAGSettings{
			TextSplitter: types.TextSplitterTypeText,
			ChunkSize:    2048,
		},
		Source: types.KnowledgeSource{
			Filestore: &types.KnowledgeSourceHelixFilestore{
				Path: "/test",
			},
		},
	}

	suite.store.EXPECT().ListKnowledge(gomock.Any(), gomock.Any()).Return([]*types.Knowledge{knowledge}, nil)

	suite.store.EXPECT().UpdateKnowledge(gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, k *types.Knowledge) (*types.Knowledge, error) {
			suite.Equal(types.KnowledgeStateIndexing, k.State)
			suite.Equal("", k.Message)
			suite.Equal("", k.Version, "version should be empty when we start indexing")

			return knowledge, nil
		},
	)

	// Check filestore
	suite.filestore.EXPECT().List(gomock.Any(), gomock.Any()).Return([]filestore.Item{}, nil)

	// Allow any number of calls to UpdateKnowledge for intermediate updates
	suite.store.EXPECT().UpdateKnowledge(gomock.Any(), gomock.Any()).AnyTimes().Return(knowledge, nil)

	suite.store.EXPECT().UpdateKnowledgeState(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(nil).AnyTimes()

	suite.store.EXPECT().CreateKnowledgeVersion(gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, k *types.KnowledgeVersion) (*types.KnowledgeVersion, error) {
			suite.Equal(types.KnowledgeStateError, k.State, "knowledge should be error")
			suite.Equal("failed to get indexing data, error: no files found in filestore", k.Message)
			suite.Equal(knowledge.ID, k.KnowledgeID, "knowledge id should be set")

			return k, nil
		},
	)

	// Start indexing
	err := suite.reconciler.index(suite.ctx)
	suite.NoError(err)

	// Wait for the goroutines to finish
	suite.reconciler.wg.Wait()
}

func (suite *IndexerSuite) TestIndex_RetryRecent_ErrorNoFiles() {
	knowledge := &types.Knowledge{
		ID:      "knowledge_id",
		AppID:   "app_id",
		Created: time.Now().Add(-1 * time.Minute),
		RAGSettings: types.RAGSettings{
			TextSplitter: types.TextSplitterTypeText,
			ChunkSize:    2048,
		},
		Source: types.KnowledgeSource{
			Filestore: &types.KnowledgeSourceHelixFilestore{
				Path: "/test",
			},
		},
	}

	suite.store.EXPECT().ListKnowledge(gomock.Any(), gomock.Any()).Return([]*types.Knowledge{knowledge}, nil)

	// First update to set state to indexing
	suite.store.EXPECT().UpdateKnowledge(gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, k *types.Knowledge) (*types.Knowledge, error) {
			suite.Equal(types.KnowledgeStateIndexing, k.State)
			suite.Equal("", k.Message)
			suite.Equal("", k.Version, "version should be empty when we start indexing")

			return knowledge, nil
		},
	)

	// Check filestore
	suite.filestore.EXPECT().List(gomock.Any(), gomock.Any()).Return([]filestore.Item{}, nil)

	// Allow any number of calls to UpdateKnowledge for intermediate updates
	suite.store.EXPECT().UpdateKnowledge(gomock.Any(), gomock.Any()).AnyTimes().Return(knowledge, nil)

	suite.store.EXPECT().UpdateKnowledgeState(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(nil).AnyTimes()

	// The test expects a call to CreateKnowledgeVersion
	suite.store.EXPECT().CreateKnowledgeVersion(gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, k *types.KnowledgeVersion) (*types.KnowledgeVersion, error) {
			suite.Equal(types.KnowledgeStatePending, k.State, "knowledge should be pending")
			suite.Equal("waiting for files to be uploaded", k.Message)
			suite.Equal(knowledge.ID, k.KnowledgeID, "knowledge id should be set")

			return k, nil
		},
	)

	// Start indexing
	err := suite.reconciler.index(suite.ctx)
	suite.NoError(err)

	// Wait for the goroutines to finish
	suite.reconciler.wg.Wait()
}

func (suite *IndexerSuite) TestIndex_UpdateLimitsWhenAbove() {
	suite.cfg.RAG.Crawler.MaxDepth = 30

	knowledge := &types.Knowledge{
		ID: "knowledge_id",
		RAGSettings: types.RAGSettings{
			TextSplitter: types.TextSplitterTypeText,
			ChunkSize:    2048,
		},
		Source: types.KnowledgeSource{
			Web: &types.KnowledgeSourceWeb{
				URLs: []string{"https://example.com"},
				Crawler: &types.WebsiteCrawler{
					Enabled:  true,
					MaxDepth: 99999,
				},
			},
		},
	}

	suite.store.EXPECT().ListKnowledge(gomock.Any(), gomock.Any()).Return([]*types.Knowledge{knowledge}, nil)

	suite.store.EXPECT().UpdateKnowledge(gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, k *types.Knowledge) (*types.Knowledge, error) {
			suite.Equal(types.KnowledgeStateIndexing, k.State)
			suite.Equal("", k.Message)
			suite.Equal("", k.Version, "version should be empty when we start indexing")

			suite.Equal(30, k.Source.Web.Crawler.MaxDepth, "max depth should be updated")

			return knowledge, nil
		},
	)

	// It will crawl the web
	suite.crawler.EXPECT().Crawl(gomock.Any()).Return([]*types.CrawledDocument{
		{
			Content:   `Hello world!`,
			SourceURL: "https://example.com/foo",
		},
	}, nil)

	// Second call to update knowledge with info on URLs
	suite.store.EXPECT().UpdateKnowledge(gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, k *types.Knowledge) (*types.Knowledge, error) {
			suite.Equal(types.KnowledgeStateIndexing, k.State) // Still indexing
			suite.Equal(1, len(k.CrawledSources.URLs), "should have 1 crawled source")
			suite.Equal("https://example.com/foo", k.CrawledSources.URLs[0].URL, "should have the correct URL")

			return knowledge, nil
		},
	)

	var version string

	// The metadata lookup is bypassed because the crawler data already has metadata
	// Remove this expectation to fix the test
	// suite.filestore.EXPECT().Get(gomock.Any(), "https://example.com/foo.metadata.yaml").Return(filestore.Item{}, fmt.Errorf("file not found"))

	// Then it will index it
	suite.rag.EXPECT().Index(gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, chunk *types.SessionRAGIndexChunk) error {
			// Split data entity id into knowledge id and version
			dataEntityIDParts := strings.SplitN(chunk.DataEntityID, "-", 2)
			suite.Equal(2, len(dataEntityIDParts))
			suite.Equal("knowledge_id", dataEntityIDParts[0])

			version = dataEntityIDParts[1]

			suite.Equal("https://example.com/foo", chunk.Source)
			suite.Equal("Hello world!", chunk.Content)

			return nil
		},
	)

	suite.store.EXPECT().UpdateKnowledge(gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, k *types.Knowledge) (*types.Knowledge, error) {
			suite.Equal(types.KnowledgeStateReady, k.State)
			suite.Equal("", k.Message)

			suite.Equal(version, k.Version, "version should be set to the version we got from the data entity id")

			return knowledge, nil
		},
	)

	suite.store.EXPECT().UpdateKnowledgeState(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(nil).AnyTimes()

	suite.store.EXPECT().CreateKnowledgeVersion(gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, k *types.KnowledgeVersion) (*types.KnowledgeVersion, error) {
			suite.Equal(version, k.Version, "version should be set to the version we got from the data entity id")
			suite.Equal(types.KnowledgeStateReady, k.State, "knowledge should be ready")
			suite.Equal("", k.Message, "message should be empty")
			suite.Equal(knowledge.ID, k.KnowledgeID, "knowledge id should be set")

			return k, nil
		},
	)

	suite.store.EXPECT().ListKnowledgeVersions(gomock.Any(), &store.ListKnowledgeVersionQuery{
		KnowledgeID: knowledge.ID,
	}).Return([]*types.KnowledgeVersion{}, nil)

	// Start indexing
	err := suite.reconciler.index(suite.ctx)
	suite.NoError(err)

	// Wait for the goroutines to finish
	suite.reconciler.wg.Wait()
}

func (suite *IndexerSuite) Test_deleteOldVersions_LessThanMaxVersions() {
	// Setup
	knowledgeID := "test_knowledge_id"
	maxVersions := 5
	suite.cfg.RAG.MaxVersions = maxVersions

	versions := []*types.KnowledgeVersion{
		{ID: "1", KnowledgeID: knowledgeID, Version: "v1", Created: time.Now().Add(-3 * time.Hour)},
		{ID: "2", KnowledgeID: knowledgeID, Version: "v2", Created: time.Now().Add(-2 * time.Hour)},
		{ID: "3", KnowledgeID: knowledgeID, Version: "v3", Created: time.Now().Add(-1 * time.Hour)},
	}

	// Expectations
	suite.store.EXPECT().ListKnowledgeVersions(gomock.Any(), &store.ListKnowledgeVersionQuery{
		KnowledgeID: knowledgeID,
	}).Return(versions, nil)

	// We don't expect any calls to DeleteKnowledgeVersion since we have fewer versions than the max

	// Execute
	err := suite.reconciler.deleteOldVersions(suite.ctx, &types.Knowledge{ID: knowledgeID})
	suite.NoError(err)
}

func (suite *IndexerSuite) Test_deleteOldVersions_MoreThanMaxVersions() {
	// Setup
	knowledgeID := "test_knowledge_id"
	maxVersions := 3
	suite.cfg.RAG.MaxVersions = maxVersions

	versions := []*types.KnowledgeVersion{
		{ID: "1", KnowledgeID: knowledgeID, Version: "v1", Created: time.Now().Add(-5 * time.Hour)},
		{ID: "2", KnowledgeID: knowledgeID, Version: "v2", Created: time.Now().Add(-4 * time.Hour)},
		{ID: "3", KnowledgeID: knowledgeID, Version: "v3", Created: time.Now().Add(-3 * time.Hour)},
		{ID: "4", KnowledgeID: knowledgeID, Version: "v4", Created: time.Now().Add(-2 * time.Hour)},
		{ID: "5", KnowledgeID: knowledgeID, Version: "v5", Created: time.Now().Add(-1 * time.Hour)},
	}

	// Expectations
	suite.store.EXPECT().ListKnowledgeVersions(gomock.Any(), &store.ListKnowledgeVersionQuery{
		KnowledgeID: knowledgeID,
	}).Return(versions, nil)

	// Expect the rag client to be called twice, once for each version
	suite.rag.EXPECT().Delete(gomock.Any(), gomock.Eq(&types.DeleteIndexRequest{
		DataEntityID: "test_knowledge_id-v1",
	})).Return(nil)
	suite.rag.EXPECT().Delete(gomock.Any(), gomock.Eq(&types.DeleteIndexRequest{
		DataEntityID: "test_knowledge_id-v2",
	})).Return(nil)

	// Expect the two oldest versions to be deleted
	suite.store.EXPECT().DeleteKnowledgeVersion(gomock.Any(), "1").Return(nil)
	suite.store.EXPECT().DeleteKnowledgeVersion(gomock.Any(), "2").Return(nil)

	// Execute
	err := suite.reconciler.deleteOldVersions(suite.ctx, &types.Knowledge{ID: knowledgeID})

	// Assert
	suite.NoError(err)
}

func Test_convertChunksIntoBatches(t *testing.T) {
	type args struct {
		chunks    []*text.DataPrepTextSplitterChunk
		batchSize int
	}
	tests := []struct {
		name string
		args args
		want [][]*text.DataPrepTextSplitterChunk
	}{
		{
			name: "1 chunk",
			args: args{
				chunks:    []*text.DataPrepTextSplitterChunk{{Text: "1"}},
				batchSize: 1,
			},
			want: [][]*text.DataPrepTextSplitterChunk{{{Text: "1"}}},
		},
		// 10 chunks, batch size 3
		{
			name: "10 chunks, batch size 3",
			args: args{
				chunks:    []*text.DataPrepTextSplitterChunk{{Text: "1"}, {Text: "2"}, {Text: "3"}, {Text: "4"}, {Text: "5"}, {Text: "6"}, {Text: "7"}, {Text: "8"}, {Text: "9"}, {Text: "10"}},
				batchSize: 3,
			},
			want: [][]*text.DataPrepTextSplitterChunk{{{Text: "1"}, {Text: "2"}, {Text: "3"}}, {{Text: "4"}, {Text: "5"}, {Text: "6"}}, {{Text: "7"}, {Text: "8"}, {Text: "9"}}, {{Text: "10"}}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := convertChunksIntoBatches(tt.args.chunks, tt.args.batchSize); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("convertChunksIntoBatches() = %v, want %v", got, tt.want)
			}
		})
	}
}
