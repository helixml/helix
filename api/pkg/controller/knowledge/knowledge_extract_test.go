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

func (suite *ExtractorSuite) Test_getIndexingData_CrawlerEnabled_PDFExtensionExtractedDirectly() {
	knowledge := &types.Knowledge{
		ID: "knowledge_id",
		Source: types.KnowledgeSource{
			Web: &types.KnowledgeSourceWeb{
				URLs: []string{"https://arxiv.org/pdf/2602.23242.pdf"},
				Crawler: &types.WebsiteCrawler{
					Enabled: true,
				},
			},
		},
	}

	// PDF URL with extension should go through the extractor, NOT the crawler
	suite.extractor.EXPECT().Extract(gomock.Any(), &extract.Request{
		URL: "https://arxiv.org/pdf/2602.23242.pdf",
	}).Return("extracted PDF content", nil)

	data, err := suite.reconciler.getIndexingData(suite.ctx, knowledge)
	suite.NoError(err)
	suite.Equal(1, len(data))
	suite.Equal("https://arxiv.org/pdf/2602.23242.pdf", data[0].Source)
	suite.Contains(string(data[0].Data), "extracted PDF content")
}

func (suite *ExtractorSuite) Test_getIndexingData_CrawlerEnabled_PDFContentTypeDetectedByHEAD() {
	// Simulate a URL with no file extension that returns application/pdf Content-Type
	pdfServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodHead {
			w.Header().Set("Content-Type", "application/pdf")
			w.WriteHeader(http.StatusOK)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer pdfServer.Close()

	knowledge := &types.Knowledge{
		ID: "knowledge_id",
		Source: types.KnowledgeSource{
			Web: &types.KnowledgeSourceWeb{
				URLs: []string{pdfServer.URL + "/pdf/2602.23242"},
				Crawler: &types.WebsiteCrawler{
					Enabled: true,
				},
			},
		},
	}

	// HEAD detects PDF → extractor is called, not crawler
	suite.extractor.EXPECT().Extract(gomock.Any(), &extract.Request{
		URL: pdfServer.URL + "/pdf/2602.23242",
	}).Return("extracted PDF via HEAD", nil)

	data, err := suite.reconciler.getIndexingData(suite.ctx, knowledge)
	suite.NoError(err)
	suite.Equal(1, len(data))
	suite.Contains(string(data[0].Data), "extracted PDF via HEAD")
}

func (suite *ExtractorSuite) Test_getIndexingData_CrawlerEnabled_MixedURLs() {
	// HTML server for HEAD check on a non-extension URL
	htmlServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusOK)
	}))
	defer htmlServer.Close()

	knowledge := &types.Knowledge{
		ID: "knowledge_id",
		Source: types.KnowledgeSource{
			Web: &types.KnowledgeSourceWeb{
				URLs: []string{
					htmlServer.URL + "/page",
					"https://example.com/report.pdf",
				},
				Crawler: &types.WebsiteCrawler{
					Enabled: true,
				},
			},
		},
	}

	// PDF URL (by extension) goes through extractor
	suite.extractor.EXPECT().Extract(gomock.Any(), &extract.Request{
		URL: "https://example.com/report.pdf",
	}).Return("extracted PDF content", nil)

	// HTML URL (confirmed by HEAD) goes through crawler
	suite.crawler.EXPECT().Crawl(gomock.Any()).Return([]*types.CrawledDocument{
		{
			Content:   "Hello from crawler",
			SourceURL: htmlServer.URL + "/page",
		},
	}, nil)

	data, err := suite.reconciler.getIndexingData(suite.ctx, knowledge)
	suite.NoError(err)
	suite.Equal(2, len(data))
}

func (suite *ExtractorSuite) Test_getIndexingData_CrawlerEnabled_AllDocURLs_CrawlerNotCalled() {
	knowledge := &types.Knowledge{
		ID: "knowledge_id",
		Source: types.KnowledgeSource{
			Web: &types.KnowledgeSourceWeb{
				URLs: []string{
					"https://example.com/a.pdf",
					"https://example.com/b.docx",
				},
				Crawler: &types.WebsiteCrawler{
					Enabled: true,
				},
			},
		},
	}

	// Both go through extractor; crawler should NOT be called
	suite.extractor.EXPECT().Extract(gomock.Any(), &extract.Request{
		URL: "https://example.com/a.pdf",
	}).Return("pdf content", nil)
	suite.extractor.EXPECT().Extract(gomock.Any(), &extract.Request{
		URL: "https://example.com/b.docx",
	}).Return("docx content", nil)

	data, err := suite.reconciler.getIndexingData(suite.ctx, knowledge)
	suite.NoError(err)
	suite.Equal(2, len(data))
}

func (suite *ExtractorSuite) Test_getIndexingData_CrawlerEnabled_OriginalURLsNotMutated() {
	knowledge := &types.Knowledge{
		ID: "knowledge_id",
		Source: types.KnowledgeSource{
			Web: &types.KnowledgeSourceWeb{
				URLs: []string{
					"https://example.com/page.html",
					"https://example.com/report.pdf",
				},
				Crawler: &types.WebsiteCrawler{
					Enabled: true,
				},
			},
		},
	}

	suite.extractor.EXPECT().Extract(gomock.Any(), gomock.Any()).Return("pdf content", nil)
	suite.crawler.EXPECT().Crawl(gomock.Any()).Return([]*types.CrawledDocument{
		{Content: "html", SourceURL: "https://example.com/page.html"},
	}, nil)

	_, err := suite.reconciler.getIndexingData(suite.ctx, knowledge)
	suite.NoError(err)

	// Original URLs must not have been mutated
	suite.Equal([]string{
		"https://example.com/page.html",
		"https://example.com/report.pdf",
	}, knowledge.Source.Web.URLs)
}

func TestIsDocumentURLByExtension(t *testing.T) {
	tests := []struct {
		url  string
		want bool
	}{
		{"https://arxiv.org/pdf/2602.23242.pdf", true},
		{"https://example.com/report.PDF", true},
		{"https://example.com/doc.pdf?token=abc", true},
		{"https://example.com/doc.pdf#page=3", true},
		{"https://example.com/file.docx", true},
		{"https://example.com/file.xlsx", true},
		{"https://example.com/file.pptx", true},
		{"https://example.com/file.csv", true},
		{"https://example.com/file.epub", true},
		// Non-document URLs
		{"https://example.com/page", false},
		{"https://example.com/page.html", false},
		{"https://example.com/api/data", false},
		{"https://arxiv.org/abs/2602.23242", false},
		// Edge cases
		{"", false},
		{"https://example.com", false},
		{"https://example.com/", false},
		{"https://example.com/file.pdf/download", false},
		{"https://example.com/file.pdf.bak", false},
	}

	for _, tt := range tests {
		t.Run(tt.url, func(t *testing.T) {
			got := isDocumentURLByExtension(tt.url)
			if got != tt.want {
				t.Errorf("isDocumentURLByExtension(%q) = %v, want %v", tt.url, got, tt.want)
			}
		})
	}
}

func TestIsDocumentContentType(t *testing.T) {
	tests := []struct {
		ct   string
		want bool
	}{
		{"application/pdf", true},
		{"application/pdf; charset=utf-8", true},
		{"APPLICATION/PDF", true},
		{"application/vnd.openxmlformats-officedocument.wordprocessingml.document", true},
		{"application/vnd.ms-excel", true},
		{"text/csv", true},
		{"text/html", false},
		{"text/html; charset=utf-8", false},
		{"application/json", false},
		{"", false},
	}

	for _, tt := range tests {
		t.Run(tt.ct, func(t *testing.T) {
			got := isDocumentContentType(tt.ct)
			if got != tt.want {
				t.Errorf("isDocumentContentType(%q) = %v, want %v", tt.ct, got, tt.want)
			}
		})
	}
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
