package knowledge

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/helixml/helix/api/pkg/config"
	"github.com/helixml/helix/api/pkg/controller/knowledge/browser"
	"github.com/helixml/helix/api/pkg/controller/knowledge/crawler"
	"github.com/helixml/helix/api/pkg/extract"
	"github.com/helixml/helix/api/pkg/filestore"
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
	filestore *filestore.MockFileStore
	cfg       *config.ServerConfig

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
	suite.filestore = filestore.NewMockFileStore(ctrl)

	suite.cfg = &config.ServerConfig{}

	b := &browser.Browser{}

	var err error

	suite.reconciler, err = New(suite.cfg, suite.store, suite.filestore, suite.extractor, suite.rag, b, nil)
	suite.Require().NoError(err)
	suite.reconciler.newRagClient = func(_ *types.RAGSettings) rag.RAG {
		return suite.rag
	}

	suite.reconciler.newCrawler = func(_ *types.Knowledge) (crawler.Crawler, error) {
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

	data, err := suite.reconciler.getIndexingData(suite.ctx, knowledge)
	suite.NoError(err)
	suite.Equal(1, len(data))
	suite.Equal("https://example.com", data[0].Source)
	suite.Contains(string(data[0].Data), "Hello, world!")
}

func (suite *ExtractorSuite) Test_getIndexingData_CrawlerDisabled_ExtractDisabled() {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprintln(w, "Hello, world!")
	}))
	defer ts.Close()

	knowledge := &types.Knowledge{
		ID: "knowledge_id",
		RAGSettings: types.RAGSettings{
			DisableChunking: true,
		},
		Source: types.KnowledgeSource{
			Web: &types.KnowledgeSourceWeb{
				URLs: []string{ts.URL},
				Crawler: &types.WebsiteCrawler{
					Enabled: false,
				},
			},
		},
	}

	data, err := suite.reconciler.getIndexingData(suite.ctx, knowledge)
	suite.NoError(err)
	suite.Equal(1, len(data))
	suite.Equal(ts.URL, data[0].Source)
	suite.Contains(string(data[0].Data), "Hello, world!")
}

func (suite *ExtractorSuite) Test_getIndexingData_CrawlerDisabled_ExtractEnabled() {
	knowledge := &types.Knowledge{
		ID: "knowledge_id",
		RAGSettings: types.RAGSettings{
			DisableChunking: false,
		},
		Source: types.KnowledgeSource{
			Web: &types.KnowledgeSourceWeb{
				URLs: []string{"https://example.com"},
				Crawler: &types.WebsiteCrawler{
					Enabled: false,
				},
			},
		},
	}

	suite.extractor.EXPECT().Extract(gomock.Any(), &extract.Request{
		URL: "https://example.com",
	}).Return("Hello, world!", nil)

	data, err := suite.reconciler.getIndexingData(suite.ctx, knowledge)
	suite.NoError(err)
	suite.Equal(1, len(data))
	suite.Equal("https://example.com", data[0].Source)
	suite.Contains(string(data[0].Data), "Hello, world!")
}

// TestIsMicrosoftOAuthProvider tests the isMicrosoftOAuthProvider helper function
// which detects Microsoft-compatible OAuth providers (both explicit Microsoft type
// and Custom providers with Microsoft OAuth URLs)
func TestIsMicrosoftOAuthProvider(t *testing.T) {
	tests := []struct {
		name     string
		provider types.OAuthProvider
		expected bool
	}{
		{
			name: "explicit Microsoft provider type",
			provider: types.OAuthProvider{
				Type: types.OAuthProviderTypeMicrosoft,
			},
			expected: true,
		},
		{
			name: "custom provider with Microsoft auth URL",
			provider: types.OAuthProvider{
				Type:    types.OAuthProviderTypeCustom,
				AuthURL: "https://login.microsoftonline.com/tenant-id/oauth2/v2.0/authorize",
			},
			expected: true,
		},
		{
			name: "custom provider with Microsoft token URL",
			provider: types.OAuthProvider{
				Type:     types.OAuthProviderTypeCustom,
				TokenURL: "https://login.microsoftonline.com/tenant-id/oauth2/v2.0/token",
			},
			expected: true,
		},
		{
			name: "custom provider with both Microsoft URLs",
			provider: types.OAuthProvider{
				Type:     types.OAuthProviderTypeCustom,
				AuthURL:  "https://login.microsoftonline.com/tenant-id/oauth2/v2.0/authorize",
				TokenURL: "https://login.microsoftonline.com/tenant-id/oauth2/v2.0/token",
			},
			expected: true,
		},
		{
			name: "custom provider with non-Microsoft URLs",
			provider: types.OAuthProvider{
				Type:     types.OAuthProviderTypeCustom,
				AuthURL:  "https://accounts.google.com/o/oauth2/auth",
				TokenURL: "https://oauth2.googleapis.com/token",
			},
			expected: false,
		},
		{
			name: "Google provider type",
			provider: types.OAuthProvider{
				Type: types.OAuthProviderTypeGoogle,
			},
			expected: false,
		},
		{
			name: "GitHub provider type",
			provider: types.OAuthProvider{
				Type: types.OAuthProviderTypeGitHub,
			},
			expected: false,
		},
		{
			name: "empty provider",
			provider: types.OAuthProvider{},
			expected: false,
		},
		{
			name: "custom provider with empty URLs",
			provider: types.OAuthProvider{
				Type:     types.OAuthProviderTypeCustom,
				AuthURL:  "",
				TokenURL: "",
			},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isMicrosoftOAuthProvider(tt.provider)
			if result != tt.expected {
				t.Errorf("isMicrosoftOAuthProvider() = %v, want %v", result, tt.expected)
			}
		})
	}
}
