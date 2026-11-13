package skill

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/helixml/helix/api/pkg/agent"
	oai "github.com/helixml/helix/api/pkg/openai"
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
			requireTurnCaller(t, ctx)
			return &websearch.SearchResponse{Results: []websearch.Result{
				{URL: "https://go.dev/blog", Title: "Go blog", Text: &text},
				{URL: "https://example.com", Title: "No text"},
			}}, nil
		})

	out, err := tool.Execute(turnContext(), agent.Meta{UserID: "user_1"}, map[string]interface{}{
		"query": "go release", "category": "news",
	})
	require.NoError(t, err)
	require.Equal(t, "## Results: \n"+
		"### Source: Go blog\n- URL: https://go.dev/blog\n- Snippet: Go 1.30 was released.\n\n"+
		"### Source: No text\n- URL: https://example.com\n- Snippet: \n\n", out)
}

// turnContext is an agent turn's context: its LLM calls bill org_1.
func turnContext() context.Context {
	return oai.SetContextOrganizationID(context.Background(), "org_1")
}

// requireTurnCaller checks a web search is billed like the turn's LLM calls,
// under its own request id, and logged as the agent's.
func requireTurnCaller(t *testing.T, ctx context.Context) {
	caller := websearch.CallerFrom(ctx)
	require.Equal(t, "user_1", caller.UserID)
	require.Equal(t, "org_1", caller.OrgID)
	require.NotEmpty(t, caller.RequestID)
	require.False(t, caller.System)
	require.Equal(t, types.WebSearchSourceAgent, caller.Source)
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
