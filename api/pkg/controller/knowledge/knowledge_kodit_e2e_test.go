//go:build !nokodit

package knowledge

import (
	"context"
	"testing"

	"github.com/helixml/kodit/domain/enrichment"
	"github.com/helixml/kodit/domain/repository"
	"github.com/helixml/kodit/domain/tracking"
	"github.com/stretchr/testify/suite"
	"go.uber.org/mock/gomock"

	"github.com/helixml/helix/api/pkg/config"
	"github.com/helixml/helix/api/pkg/controller/knowledge/browser"
	"github.com/helixml/helix/api/pkg/extract"
	"github.com/helixml/helix/api/pkg/filestore"
	ragpkg "github.com/helixml/helix/api/pkg/rag"
	"github.com/helixml/helix/api/pkg/services"
	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/types"
)

// koditSvcE2E is a minimal KoditServicer for E2E tests.
type koditSvcE2E struct {
	registerFn func(ctx context.Context, cloneURL, upstreamURL string) (int64, bool, error)
}

var _ services.KoditServicer = (*koditSvcE2E)(nil)

func (k *koditSvcE2E) RegisterRepository(ctx context.Context, cloneURL, upstreamURL string) (int64, bool, error) {
	if k.registerFn != nil {
		return k.registerFn(ctx, cloneURL, upstreamURL)
	}
	return 0, false, nil
}

func (k *koditSvcE2E) IsEnabled() bool                                                                              { return true }
func (k *koditSvcE2E) MCPDocumentation() string                                                                      { return "" }
func (k *koditSvcE2E) DeleteRepository(context.Context, int64) error                                                 { return nil }
func (k *koditSvcE2E) RescanCommit(context.Context, int64, string) error                                             { return nil }
func (k *koditSvcE2E) SyncRepository(context.Context, int64) error                                                   { return nil }
func (k *koditSvcE2E) EnrichmentCount(context.Context, int64) (int64, error)                                         { return 0, nil }
func (k *koditSvcE2E) DeleteTask(context.Context, int64) error                                                       { return nil }
func (k *koditSvcE2E) UpdateTaskPriority(context.Context, int64, int) error                                          { return nil }
func (k *koditSvcE2E) GetRepositoryEnrichments(context.Context, int64, string, string) ([]enrichment.Enrichment, error) { return nil, nil }
func (k *koditSvcE2E) GetEnrichment(context.Context, string) (enrichment.Enrichment, error)                          { return enrichment.Enrichment{}, nil }
func (k *koditSvcE2E) GetRepositoryCommits(context.Context, int64, int) ([]repository.Commit, error)                 { return nil, nil }
func (k *koditSvcE2E) SearchSnippets(context.Context, int64, string, int) ([]enrichment.Enrichment, error)           { return nil, nil }
func (k *koditSvcE2E) GetRepositoryStatus(context.Context, int64) (tracking.RepositoryStatusSummary, error) {
	return tracking.RepositoryStatusSummary{}, nil
}
func (k *koditSvcE2E) ListRepositories(context.Context, int, int) ([]repository.Repository, int64, error) {
	return nil, 0, nil
}
func (k *koditSvcE2E) RepositorySummary(context.Context, int64) (repository.RepositorySummary, error) {
	return repository.RepositorySummary{}, nil
}
func (k *koditSvcE2E) SystemStats(context.Context) (services.KoditSystemStats, error) {
	return services.KoditSystemStats{}, nil
}
func (k *koditSvcE2E) RepositoryTasks(context.Context, int64) (services.KoditRepositoryTasks, error) {
	return services.KoditRepositoryTasks{}, nil
}
func (k *koditSvcE2E) GetWikiTree(context.Context, int64) ([]services.KoditWikiTreeNode, error) { return nil, nil }
func (k *koditSvcE2E) GetWikiPage(context.Context, int64, string) (*services.KoditWikiPage, error) {
	return nil, nil
}
func (k *koditSvcE2E) SemanticSearch(context.Context, int64, string, int, string) ([]services.KoditFileResult, error) {
	return nil, nil
}
func (k *koditSvcE2E) KeywordSearch(context.Context, int64, string, int, string) ([]services.KoditFileResult, error) {
	return nil, nil
}
func (k *koditSvcE2E) GrepSearch(context.Context, int64, string, string, int) ([]services.KoditGrepResult, error) {
	return nil, nil
}
func (k *koditSvcE2E) ListFiles(context.Context, int64, string) ([]services.KoditFileEntry, error) { return nil, nil }
func (k *koditSvcE2E) ReadFile(context.Context, int64, string, int, int) (*services.KoditFileContent, error) {
	return nil, nil
}
func (k *koditSvcE2E) ListAllTasks(context.Context, int, int) ([]services.KoditPendingTask, int64, error) {
	return nil, 0, nil
}
func (k *koditSvcE2E) ActiveTasks(context.Context) ([]services.KoditActiveTask, error) { return nil, nil }
func (k *koditSvcE2E) UpdateChunkingConfig(context.Context, int64, int, int, int) error { return nil }

// KoditE2ESuite is a high-level integration test of the kodit knowledge indexing pipeline.
// It verifies that indexKnowledge routes to kodit when the RAG client is a KoditIndexer,
// registers the correct file:// URI, and persists the kodit repository ID on the DataEntity.
type KoditE2ESuite struct {
	suite.Suite
	ctrl          *gomock.Controller
	mockStore     *store.MockStore
	mockFS        *filestore.MockFileStore
	mockExtractor *extract.MockExtractor
	koditSvc      *koditSvcE2E
	koditRAG      *ragpkg.KoditRAG
	reconciler    *Reconciler
	cfg           *config.ServerConfig
}

func TestKoditE2ESuite(t *testing.T) {
	suite.Run(t, new(KoditE2ESuite))
}

func (s *KoditE2ESuite) SetupTest() {
	s.ctrl = gomock.NewController(s.T())
	s.mockStore = store.NewMockStore(s.ctrl)
	s.mockFS = filestore.NewMockFileStore(s.ctrl)
	s.koditSvc = &koditSvcE2E{}

	s.cfg = &config.ServerConfig{}
	s.cfg.RAG.DefaultRagProvider = "kodit"
	s.cfg.RAG.MaxVersions = 3
	s.cfg.Controller.FilePrefixGlobal = "dev"
	s.cfg.FileStore.Type = "fs"
	s.cfg.FileStore.LocalFSPath = "/tmp/helix/filestore"

	fsCfg := config.FileStore{
		Type:        "fs",
		LocalFSPath: "/tmp/helix/filestore",
	}
	s.koditRAG = ragpkg.NewKoditRAG(s.koditSvc, s.mockStore, fsCfg)

	s.mockExtractor = extract.NewMockExtractor(s.ctrl)

	var err error
	s.reconciler, err = New(s.cfg, s.mockStore, s.mockFS, s.mockExtractor, s.koditRAG, &browser.Browser{}, nil)
	s.Require().NoError(err)
}

func (s *KoditE2ESuite) TearDownTest() {
	s.ctrl.Finish()
}

// TestKoditIndexing_RegistersDirectoryAndSetsRepoID verifies the end-to-end flow:
// 1. indexKnowledge detects KoditIndexer and skips normal extraction
// 2. RegisterDirectory is called with the correct file:// URI
// 3. The returned kodit repo ID is stored on the DataEntity
// 4. Knowledge state remains Indexing (kodit processes async; status checker transitions to Ready)
func (s *KoditE2ESuite) TestKoditIndexing_RegistersDirectoryAndSetsRepoID() {
	const (
		appID   = "app_abc"
		version = "v1"
		repoID  = int64(17)
	)

	knowledge := &types.Knowledge{
		ID:        "know_001",
		AppID:     appID,
		Owner:     "user_1",
		OwnerType: types.OwnerTypeUser,
		Source: types.KnowledgeSource{
			Filestore: &types.KnowledgeSourceHelixFilestore{
				Path: "documents",
			},
		},
	}

	// Expected local path: /tmp/helix/filestore/dev/apps/app_abc/documents
	expectedFileURI := "file:///tmp/helix/filestore/dev/apps/app_abc/documents"

	// kodit should be called with the correct file:// URI
	registerCalled := false
	s.koditSvc.registerFn = func(_ context.Context, cloneURL, _ string) (int64, bool, error) {
		s.Equal(expectedFileURI, cloneURL)
		registerCalled = true
		return repoID, true, nil
	}

	// updateProgress is called before RegisterDirectory
	s.mockStore.EXPECT().
		UpdateKnowledgeState(gomock.Any(), knowledge.ID, types.KnowledgeStateIndexing, "registering directory with kodit").
		Return(nil)

	dataEntityID := types.GetDataEntityID(knowledge.ID, version)

	// Store expects a GetDataEntity (returns not found) then CreateDataEntity
	s.mockStore.EXPECT().
		GetDataEntity(gomock.Any(), dataEntityID).
		Return(nil, store.ErrNotFound)

	s.mockStore.EXPECT().
		CreateDataEntity(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, e *types.DataEntity) (*types.DataEntity, error) {
			s.Equal(dataEntityID, e.ID)
			s.Require().NotNil(e.KoditRepositoryID)
			s.Equal(repoID, *e.KoditRepositoryID)
			return e, nil
		})

	// After registration, knowledge stays in Indexing state (kodit processes async)
	s.mockStore.EXPECT().
		UpdateKnowledge(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, k *types.Knowledge) (*types.Knowledge, error) {
			s.Equal(types.KnowledgeStateIndexing, k.State)
			s.Equal(version, k.Version)
			return k, nil
		})

	// Knowledge version is created in Indexing state
	s.mockStore.EXPECT().
		CreateKnowledgeVersion(gomock.Any(), gomock.Any()).
		Return(&types.KnowledgeVersion{}, nil)

	err := s.reconciler.indexKnowledge(context.Background(), knowledge, version)
	s.NoError(err)
	s.True(registerCalled, "kodit RegisterRepository should have been called")
}

// TestKoditIndexing_FallsBackToNormalPipelineForWebSource verifies that non-filestore
// sources (e.g. web) still go through the normal extraction pipeline even when the
// RAG client is a KoditRAG.
func (s *KoditE2ESuite) TestKoditIndexing_FallsBackToNormalPipelineForWebSource() {
	knowledge := &types.Knowledge{
		ID:    "know_web",
		AppID: "app_web",
		Source: types.KnowledgeSource{
			Web: &types.KnowledgeSourceWeb{
				URLs: []string{"https://example.com"},
			},
		},
	}

	// The kodit RegisterRepository should NOT be called since it's a web source.
	registerCalled := false
	s.koditSvc.registerFn = func(_ context.Context, _, _ string) (int64, bool, error) {
		registerCalled = true
		return 0, false, nil
	}

	// Normal pipeline: getIndexingData will fail (no HTTP client configured)
	// but it won't call RegisterRepository — that's what we're verifying.
	// We expect the normal path to set knowledge to error via UpdateKnowledgeState and UpdateKnowledge.
	s.mockStore.EXPECT().
		UpdateKnowledgeState(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		Return(nil).AnyTimes()
	s.mockStore.EXPECT().
		UpdateKnowledge(gomock.Any(), gomock.Any()).
		Return(knowledge, nil).AnyTimes()
	s.mockFS.EXPECT().
		List(gomock.Any(), gomock.Any()).
		Return(nil, nil).AnyTimes()
	s.mockExtractor.EXPECT().
		Extract(gomock.Any(), gomock.Any()).
		Return("", nil).AnyTimes()

	// Just run it; we don't care about the error, only that register was not called.
	_ = s.reconciler.indexKnowledge(context.Background(), knowledge, "v1")
	s.False(registerCalled, "kodit RegisterRepository should NOT be called for web sources")
}
