//go:build !nokodit

package rag

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"go.uber.org/mock/gomock"
	"github.com/helixml/kodit/domain/enrichment"
	"github.com/helixml/kodit/domain/repository"
	"github.com/helixml/kodit/domain/tracking"
	"github.com/stretchr/testify/suite"

	"github.com/helixml/helix/api/pkg/config"
	"github.com/helixml/helix/api/pkg/data"
	"github.com/helixml/helix/api/pkg/services"
	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/types"
)

// koditServiceMock implements services.KoditServicer for tests.
// Only the methods exercised by KoditRAG have meaningful implementations;
// the rest return zero values so the struct satisfies the interface.
type koditServiceMock struct {
	registerRepositoryFn   func(ctx context.Context, params *services.RegisterRepositoryParams) (int64, bool, error)
	semanticSearchFn       func(ctx context.Context, koditRepoID int64, query string, limit int, language string) ([]services.KoditFileResult, error)
	deleteRepositoryFn     func(ctx context.Context, koditRepoID int64) error
	rescanCommitFn         func(ctx context.Context, koditRepoID int64, commitSHA string) error
	getRepositoryCommitsFn func(ctx context.Context, koditRepoID int64, limit int) ([]repository.Commit, error)
}

var _ services.KoditServicer = (*koditServiceMock)(nil)

func (m *koditServiceMock) IsEnabled() bool                                { return true }
func (m *koditServiceMock) MCPDocumentation() string                       { return "" }
func (m *koditServiceMock) RescanAllRepositories(_ context.Context) error  { return nil }
func (m *koditServiceMock) RescanCommit(ctx context.Context, id int64, sha string) error {
	if m.rescanCommitFn != nil {
		return m.rescanCommitFn(ctx, id, sha)
	}
	return nil
}
func (m *koditServiceMock) SyncRepository(context.Context, int64) error    { return nil }
func (m *koditServiceMock) EnrichmentCount(context.Context, int64) (int64, error) { return 0, nil }
func (m *koditServiceMock) DeleteTask(context.Context, int64) error        { return nil }
func (m *koditServiceMock) UpdateTaskPriority(context.Context, int64, int) error { return nil }

func (m *koditServiceMock) RegisterRepository(ctx context.Context, params *services.RegisterRepositoryParams) (int64, bool, error) {
	if m.registerRepositoryFn != nil {
		return m.registerRepositoryFn(ctx, params)
	}
	return 0, false, nil
}

func (m *koditServiceMock) VisualSearch(context.Context, int64, string, int) ([]services.KoditFileResult, error) {
	return nil, nil
}

func (m *koditServiceMock) SemanticSearch(ctx context.Context, id int64, query string, limit int, lang string) ([]services.KoditFileResult, error) {
	if m.semanticSearchFn != nil {
		return m.semanticSearchFn(ctx, id, query, limit, lang)
	}
	return nil, nil
}

func (m *koditServiceMock) DeleteRepository(ctx context.Context, id int64) error {
	if m.deleteRepositoryFn != nil {
		return m.deleteRepositoryFn(ctx, id)
	}
	return nil
}

func (m *koditServiceMock) GetRepositoryEnrichments(context.Context, int64, string, string) ([]enrichment.Enrichment, error) {
	return nil, nil
}
func (m *koditServiceMock) GetEnrichment(context.Context, string) (enrichment.Enrichment, error) {
	return enrichment.Enrichment{}, nil
}
func (m *koditServiceMock) GetRepositoryCommits(ctx context.Context, id int64, limit int) ([]repository.Commit, error) {
	if m.getRepositoryCommitsFn != nil {
		return m.getRepositoryCommitsFn(ctx, id, limit)
	}
	return nil, nil
}
func (m *koditServiceMock) SearchSnippets(context.Context, int64, string, int) ([]enrichment.Enrichment, error) {
	return nil, nil
}
func (m *koditServiceMock) GetRepositoryStatus(context.Context, int64) (tracking.RepositoryStatusSummary, error) {
	return tracking.RepositoryStatusSummary{}, nil
}
func (m *koditServiceMock) ListRepositories(context.Context, int, int) ([]repository.Repository, int64, error) {
	return nil, 0, nil
}
func (m *koditServiceMock) RepositorySummary(context.Context, int64) (repository.RepositorySummary, error) {
	return repository.RepositorySummary{}, nil
}
func (m *koditServiceMock) SystemStats(context.Context) (services.KoditSystemStats, error) {
	return services.KoditSystemStats{}, nil
}
func (m *koditServiceMock) RepositoryTasks(context.Context, int64) (services.KoditRepositoryTasks, error) {
	return services.KoditRepositoryTasks{}, nil
}
func (m *koditServiceMock) GetWikiTree(context.Context, int64) ([]services.KoditWikiTreeNode, error) {
	return nil, nil
}
func (m *koditServiceMock) GetWikiPage(context.Context, int64, string) (*services.KoditWikiPage, error) {
	return nil, nil
}
func (m *koditServiceMock) KeywordSearch(context.Context, int64, string, int, string) ([]services.KoditFileResult, error) {
	return nil, nil
}
func (m *koditServiceMock) GrepSearch(context.Context, int64, string, string, int) ([]services.KoditGrepResult, error) {
	return nil, nil
}
func (m *koditServiceMock) ListFiles(context.Context, int64, string) ([]services.KoditFileEntry, error) {
	return nil, nil
}
func (m *koditServiceMock) ReadFile(context.Context, int64, string, int, int) (*services.KoditFileContent, error) {
	return nil, nil
}
func (m *koditServiceMock) ListAllTasks(context.Context, int, int) ([]services.KoditPendingTask, int64, error) {
	return nil, 0, nil
}
func (m *koditServiceMock) ActiveTasks(context.Context) ([]services.KoditActiveTask, error) {
	return nil, nil
}
func (m *koditServiceMock) UpdateChunkingConfig(context.Context, int64, int, int, int) error {
	return nil
}
func (m *koditServiceMock) RenderPageImage(context.Context, int64, string, int) ([]byte, error) {
	return nil, nil
}

// KoditRAGSuite tests the KoditRAG implementation.
type KoditRAGSuite struct {
	suite.Suite
	ctrl      *gomock.Controller
	mockStore *store.MockStore
	mockSvc   *koditServiceMock
	rag       *KoditRAG
}

func TestKoditRAGSuite(t *testing.T) {
	suite.Run(t, new(KoditRAGSuite))
}

func (s *KoditRAGSuite) SetupTest() {
	s.ctrl = gomock.NewController(s.T())
	s.mockStore = store.NewMockStore(s.ctrl)
	s.mockSvc = &koditServiceMock{}
	s.rag = NewKoditRAG(s.mockSvc, s.mockStore, config.FileStore{
		Type:        "fs",
		LocalFSPath: "/tmp/helix/filestore",
	})
}

func (s *KoditRAGSuite) TearDownTest() {
	s.ctrl.Finish()
}

func (s *KoditRAGSuite) TestIndex_IsNoop() {
	err := s.rag.Index(context.Background(), &types.SessionRAGIndexChunk{
		DataEntityID: "de_test",
		Content:      "some content",
	})
	s.NoError(err)
}

func (s *KoditRAGSuite) TestRegisterDirectory_CreatesDataEntity() {
	repoID := int64(42)
	s.mockSvc.registerRepositoryFn = func(_ context.Context, params *services.RegisterRepositoryParams) (int64, bool, error) {
		s.Equal("file:///srv/files/knowledge", params.CloneURL)
		s.Equal(repository.PipelineNameRAG, params.Pipeline)
		return repoID, true, nil
	}

	s.mockStore.EXPECT().
		GetDataEntity(gomock.Any(), "de_123").
		Return(nil, store.ErrNotFound)

	s.mockStore.EXPECT().
		CreateDataEntity(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, e *types.DataEntity) (*types.DataEntity, error) {
			s.Equal("de_123", e.ID)
			s.Equal("user_1", e.Owner)
			s.Require().NotNil(e.KoditRepositoryID)
			s.Equal(repoID, *e.KoditRepositoryID)
			s.Equal("/srv/files/knowledge", e.Config.FilestorePath)
			return e, nil
		})

	err := s.rag.RegisterDirectory(context.Background(), "de_123", "/srv/files/knowledge", "user_1", "user")
	s.NoError(err)
}

func (s *KoditRAGSuite) TestRegisterDirectory_UpdatesExistingDataEntity() {
	repoID := int64(99)
	s.mockSvc.registerRepositoryFn = func(_ context.Context, _ *services.RegisterRepositoryParams) (int64, bool, error) {
		return repoID, false, nil
	}

	// When repo already exists, fetch latest commit and rescan it.
	s.mockSvc.getRepositoryCommitsFn = func(_ context.Context, id int64, limit int) ([]repository.Commit, error) {
		s.Equal(repoID, id)
		s.Equal(1, limit)
		commit := repository.ReconstructCommit(1, "abc123", repoID, "", repository.Author{}, repository.Author{}, time.Time{}, time.Time{}, time.Time{}, "")
		return []repository.Commit{commit}, nil
	}

	rescanned := false
	s.mockSvc.rescanCommitFn = func(_ context.Context, id int64, sha string) error {
		s.Equal(repoID, id)
		s.Equal("abc123", sha)
		rescanned = true
		return nil
	}

	existing := &types.DataEntity{ID: "de_456", Owner: "user_1"}

	s.mockStore.EXPECT().
		GetDataEntity(gomock.Any(), "de_456").
		Return(existing, nil)

	s.mockStore.EXPECT().
		UpdateDataEntity(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, e *types.DataEntity) (*types.DataEntity, error) {
			s.Require().NotNil(e.KoditRepositoryID)
			s.Equal(repoID, *e.KoditRepositoryID)
			s.Equal("/some/path", e.Config.FilestorePath)
			return e, nil
		})

	err := s.rag.RegisterDirectory(context.Background(), "de_456", "/some/path", "user_1", "user")
	s.NoError(err)
	s.True(rescanned)
}

func (s *KoditRAGSuite) TestQuery_UsesStoredRepoID() {
	repoID := int64(7)
	entity := &types.DataEntity{ID: "de_q1", KoditRepositoryID: &repoID}

	s.mockStore.EXPECT().
		GetDataEntity(gomock.Any(), "de_q1").
		Return(entity, nil)

	s.mockSvc.semanticSearchFn = func(_ context.Context, id int64, query string, limit int, _ string) ([]services.KoditFileResult, error) {
		s.Equal(repoID, id)
		s.Equal("test query", query)
		s.Equal(5, limit)
		return []services.KoditFileResult{
			{Path: "pkg/foo.go", Preview: "some content", Content: "some content", Score: 0.9},
		}, nil
	}

	results, err := s.rag.Query(context.Background(), &types.SessionRAGQuery{
		DataEntityID: "de_q1",
		Prompt:       "test query",
		MaxResults:   5,
	})
	s.NoError(err)
	s.Len(results, 1)
	s.Equal("pkg/foo.go", results[0].Source)
	s.Equal("some content", results[0].Content)
}

func (s *KoditRAGSuite) TestQuery_ErrorWhenNoRepoID() {
	entity := &types.DataEntity{ID: "de_norepo", KoditRepositoryID: nil}

	s.mockStore.EXPECT().
		GetDataEntity(gomock.Any(), "de_norepo").
		Return(entity, nil)

	_, err := s.rag.Query(context.Background(), &types.SessionRAGQuery{
		DataEntityID: "de_norepo",
		Prompt:       "query",
	})
	s.Error(err)
	s.Contains(err.Error(), "no kodit repository ID")
}

func (s *KoditRAGSuite) TestDelete_CallsDeleteRepository() {
	repoID := int64(55)
	entity := &types.DataEntity{ID: "de_del", KoditRepositoryID: &repoID}

	s.mockStore.EXPECT().
		GetDataEntity(gomock.Any(), "de_del").
		Return(entity, nil)

	deleted := false
	s.mockSvc.deleteRepositoryFn = func(_ context.Context, id int64) error {
		s.Equal(repoID, id)
		deleted = true
		return nil
	}

	err := s.rag.Delete(context.Background(), &types.DeleteIndexRequest{DataEntityID: "de_del"})
	s.NoError(err)
	s.True(deleted)
}

func (s *KoditRAGSuite) TestDelete_NoopWhenNoRepoID() {
	entity := &types.DataEntity{ID: "de_nodel", KoditRepositoryID: nil}

	s.mockStore.EXPECT().
		GetDataEntity(gomock.Any(), "de_nodel").
		Return(entity, nil)

	err := s.rag.Delete(context.Background(), &types.DeleteIndexRequest{DataEntityID: "de_nodel"})
	s.NoError(err)
}

func (s *KoditRAGSuite) TestDelete_NoopWhenEntityNotFound() {
	s.mockStore.EXPECT().
		GetDataEntity(gomock.Any(), "de_missing").
		Return(nil, store.ErrNotFound)

	err := s.rag.Delete(context.Background(), &types.DeleteIndexRequest{DataEntityID: "de_missing"})
	s.NoError(err)
}

func (s *KoditRAGSuite) TestQuery_PopulatesDocumentID() {
	// Create real files on disk so Query can read and hash them.
	tmpDir := s.T().TempDir()
	fooContent := []byte("package foo\n\nfunc Foo() {}\n")
	barContent := []byte("package bar\n\nfunc Bar() {}\n")

	s.Require().NoError(os.MkdirAll(filepath.Join(tmpDir, "pkg"), 0o755))
	s.Require().NoError(os.WriteFile(filepath.Join(tmpDir, "pkg", "foo.go"), fooContent, 0o644))
	s.Require().NoError(os.WriteFile(filepath.Join(tmpDir, "pkg", "bar.go"), barContent, 0o644))

	repoID := int64(10)
	entity := &types.DataEntity{
		ID:                "de_docid",
		KoditRepositoryID: &repoID,
		Config: types.DataEntityConfig{
			FilestorePath: tmpDir,
		},
	}

	s.mockStore.EXPECT().
		GetDataEntity(gomock.Any(), "de_docid").
		Return(entity, nil)

	s.mockSvc.semanticSearchFn = func(_ context.Context, _ int64, _ string, _ int, _ string) ([]services.KoditFileResult, error) {
		return []services.KoditFileResult{
			{Path: "pkg/foo.go", Preview: "func Foo() {}", Score: 0.95},
			{Path: "pkg/bar.go", Preview: "func Bar() {}", Score: 0.85},
		}, nil
	}

	results, err := s.rag.Query(context.Background(), &types.SessionRAGQuery{
		DataEntityID: "de_docid",
		Prompt:       "find functions",
		MaxResults:   10,
	})
	s.NoError(err)
	s.Len(results, 2)

	// DocumentID must be the content hash of the actual file, not the preview.
	s.Equal(data.ContentHash(fooContent), results[0].DocumentID)
	s.Equal(data.ContentHash(barContent), results[1].DocumentID)

	// Different files must produce different IDs.
	s.NotEqual(results[0].DocumentID, results[1].DocumentID)
}

func (s *KoditRAGSuite) TestRegisterDirectory_PropagatesKoditError() {
	koditErr := errors.New("kodit unavailable")
	s.mockSvc.registerRepositoryFn = func(_ context.Context, _ *services.RegisterRepositoryParams) (int64, bool, error) {
		return 0, false, koditErr
	}

	err := s.rag.RegisterDirectory(context.Background(), "de_err", "/some/path", "user_1", "user")
	s.Error(err)
	s.True(errors.Is(err, koditErr))
}
