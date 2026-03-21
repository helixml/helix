//go:build !nokodit

package rag

import (
	"context"
	"errors"
	"testing"

	"go.uber.org/mock/gomock"
	"github.com/helixml/kodit/domain/enrichment"
	"github.com/helixml/kodit/domain/repository"
	"github.com/helixml/kodit/domain/tracking"
	"github.com/stretchr/testify/suite"

	"github.com/helixml/helix/api/pkg/config"
	"github.com/helixml/helix/api/pkg/services"
	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/types"
)

// koditServiceMock implements services.KoditServicer for tests.
// Only the methods exercised by KoditRAG have meaningful implementations;
// the rest return zero values so the struct satisfies the interface.
type koditServiceMock struct {
	registerRepositoryFn func(ctx context.Context, cloneURL, upstreamURL string) (int64, bool, error)
	semanticSearchFn     func(ctx context.Context, koditRepoID int64, query string, limit int, language string) ([]services.KoditFileResult, error)
	deleteRepositoryFn   func(ctx context.Context, koditRepoID int64) error
}

var _ services.KoditServicer = (*koditServiceMock)(nil)

func (m *koditServiceMock) IsEnabled() bool                                { return true }
func (m *koditServiceMock) MCPDocumentation() string                       { return "" }
func (m *koditServiceMock) RescanCommit(context.Context, int64, string) error { return nil }
func (m *koditServiceMock) SyncRepository(context.Context, int64) error    { return nil }
func (m *koditServiceMock) EnrichmentCount(context.Context, int64) (int64, error) { return 0, nil }
func (m *koditServiceMock) DeleteTask(context.Context, int64) error        { return nil }
func (m *koditServiceMock) UpdateTaskPriority(context.Context, int64, int) error { return nil }

func (m *koditServiceMock) RegisterRepository(ctx context.Context, cloneURL, upstreamURL string) (int64, bool, error) {
	if m.registerRepositoryFn != nil {
		return m.registerRepositoryFn(ctx, cloneURL, upstreamURL)
	}
	return 0, false, nil
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
func (m *koditServiceMock) GetRepositoryCommits(context.Context, int64, int) ([]repository.Commit, error) {
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
	s.mockSvc.registerRepositoryFn = func(_ context.Context, cloneURL, _ string) (int64, bool, error) {
		s.Equal("file:///srv/files/knowledge", cloneURL)
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
			return e, nil
		})

	err := s.rag.RegisterDirectory(context.Background(), "de_123", "/srv/files/knowledge", "user_1", "user")
	s.NoError(err)
}

func (s *KoditRAGSuite) TestRegisterDirectory_UpdatesExistingDataEntity() {
	repoID := int64(99)
	s.mockSvc.registerRepositoryFn = func(_ context.Context, _, _ string) (int64, bool, error) {
		return repoID, false, nil
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
			return e, nil
		})

	err := s.rag.RegisterDirectory(context.Background(), "de_456", "/some/path", "user_1", "user")
	s.NoError(err)
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
			{Path: "pkg/foo.go", Preview: "some content", Score: 0.9},
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

func (s *KoditRAGSuite) TestRegisterDirectory_PropagatesKoditError() {
	koditErr := errors.New("kodit unavailable")
	s.mockSvc.registerRepositoryFn = func(_ context.Context, _, _ string) (int64, bool, error) {
		return 0, false, koditErr
	}

	err := s.rag.RegisterDirectory(context.Background(), "de_err", "/some/path", "user_1", "user")
	s.Error(err)
	s.True(errors.Is(err, koditErr))
}
