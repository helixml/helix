package knowledge

import (
	"context"
	"reflect"
	"strings"
	"testing"

	"github.com/helixml/helix/api/pkg/config"
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

	suite.reconciler, _ = New(suite.cfg, suite.store, suite.filestore, suite.extractor, suite.rag)

	suite.reconciler.newRagClient = func(settings *types.RAGSettings) rag.RAG {
		return suite.rag
	}

	suite.reconciler.newCrawler = func(k *types.Knowledge) (crawler.Crawler, error) {
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
		func(ctx context.Context, k *types.Knowledge) (*types.Knowledge, error) {
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

	var version string

	// Then it will index it
	suite.rag.EXPECT().Index(gomock.Any(), gomock.Any()).DoAndReturn(
		func(ctx context.Context, chunk *types.SessionRAGIndexChunk) error {
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
		func(ctx context.Context, k *types.Knowledge) (*types.Knowledge, error) {
			suite.Equal(types.KnowledgeStateReady, k.State)
			suite.Equal("", k.Message)

			suite.Equal(version, k.Version, "version should be set to the version we got from the data entity id")

			return knowledge, nil
		},
	)

	suite.store.EXPECT().UpdateKnowledgeState(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(nil).AnyTimes()

	suite.store.EXPECT().CreateKnowledgeVersion(gomock.Any(), gomock.Any()).DoAndReturn(
		func(ctx context.Context, k *types.KnowledgeVersion) (*types.KnowledgeVersion, error) {
			suite.Equal(version, k.Version, "version should be set to the version we got from the data entity id")
			suite.Equal(types.KnowledgeStateReady, k.State, "knowledge should be ready")
			suite.Equal("", k.Message, "message should be empty")
			suite.Equal(knowledge.ID, k.KnowledgeID, "knowledge id should be set")

			return k, nil
		},
	)

	// Start indexing
	suite.reconciler.index(suite.ctx)

	// Wait for the goroutines to finish
	suite.reconciler.wg.Wait()
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
