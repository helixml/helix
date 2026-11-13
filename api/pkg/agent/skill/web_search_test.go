package skill

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/helixml/helix/api/pkg/agent"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/helixml/helix/api/pkg/websearch"
)

func newTestSearchTool(t *testing.T) (*searchTool, *websearch.MockProvider) {
	provider := websearch.NewMockProvider(gomock.NewController(t))
	return &searchTool{config: &types.ToolWebSearchConfig{MaxResults: 5}, provider: provider}, provider
}

func TestSearchToolMapsRequestAndFormatsResults(t *testing.T) {
	tool, provider := newTestSearchTool(t)
	text := "Go 1.30 was released."
	provider.EXPECT().Search(gomock.Any(), gomock.Any()).
		DoAndReturn(func(ctx context.Context, req *websearch.SearchRequest) (*websearch.SearchResponse, error) {
			require.Equal(t, "go release", req.Query)
			require.Equal(t, 5, req.NumResults)
			require.Equal(t, websearch.CategoryNews, req.Category)
			require.Equal(t, searchSnippetChars, req.Contents.Text.MaxCharacters)
			require.Equal(t, websearch.Caller{UserID: "user_1"}, websearch.CallerFrom(ctx))
			return &websearch.SearchResponse{Results: []websearch.Result{
				{URL: "https://go.dev/blog", Title: "Go blog", Text: &text},
				{URL: "https://example.com", Title: "No text"},
			}}, nil
		})

	out, err := tool.Execute(context.Background(), agent.Meta{UserID: "user_1"}, map[string]interface{}{
		"query": "go release", "category": "news",
	})
	require.NoError(t, err)
	require.Equal(t, "## Results: \n"+
		"### Source: Go blog\n- URL: https://go.dev/blog\n- Snippet: Go 1.30 was released.\n\n"+
		"### Source: No text\n- URL: https://example.com\n- Snippet: \n\n", out)
}

func TestSearchToolGeneralCategoryIsUnfiltered(t *testing.T) {
	tool, provider := newTestSearchTool(t)
	provider.EXPECT().Search(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, req *websearch.SearchRequest) (*websearch.SearchResponse, error) {
			require.Empty(t, req.Category)
			return &websearch.SearchResponse{}, nil
		})

	out, err := tool.Execute(context.Background(), agent.Meta{}, map[string]interface{}{"query": "q", "category": "general"})
	require.NoError(t, err)
	require.Equal(t, "No search results found for the query.", out)
}

func TestSearchToolRejectsUnknownCategory(t *testing.T) {
	tool, _ := newTestSearchTool(t)
	_, err := tool.Execute(context.Background(), agent.Meta{}, map[string]interface{}{"query": "q", "category": "social_media"})
	require.ErrorContains(t, err, "unsupported category")
}

func TestSearchToolReturnsProviderError(t *testing.T) {
	tool, provider := newTestSearchTool(t)
	provider.EXPECT().Search(gomock.Any(), gomock.Any()).Return(nil, errors.New("searxng down"))

	_, err := tool.Execute(context.Background(), agent.Meta{}, map[string]interface{}{"query": "q"})
	require.ErrorContains(t, err, "searxng down")
}
