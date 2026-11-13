package skill

import (
	"context"
	"errors"
	"testing"

	"github.com/sashabaranov/go-openai"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/helixml/helix/api/pkg/agent"
	helix_openai "github.com/helixml/helix/api/pkg/openai"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/helixml/helix/api/pkg/websearch"
)

const testPageURL = "https://example.com/docs"

func browserArgs() map[string]interface{} {
	return map[string]interface{}{"url": testPageURL, "prompt": "what is it about?"}
}

func newTestBrowserTool(t *testing.T, processOutput bool) (*browserTool, *websearch.MockProvider, *helix_openai.MockClient) {
	ctrl := gomock.NewController(t)
	provider := websearch.NewMockProvider(ctrl)
	client := helix_openai.NewMockClient(ctrl)
	llm := agent.NewLLM(nil, nil, nil, &agent.LLMModelConfig{Client: client, Model: "small"})
	skill := NewBrowserSkill(&types.ToolBrowserConfig{Enabled: true, ProcessOutput: processOutput}, provider, llm)
	return skill.Tools[0].(*browserTool), provider, client
}

func pageResponse(text string) *websearch.ContentsResponse {
	return &websearch.ContentsResponse{
		Results:  []websearch.Result{{URL: testPageURL, Text: &text}},
		Statuses: []websearch.ContentStatus{{ID: testPageURL, Status: websearch.ContentStatusSuccess}},
	}
}

func TestBrowserToolReturnsPageText(t *testing.T) {
	tool, provider, _ := newTestBrowserTool(t, false)
	provider.EXPECT().Contents(gomock.Any(), gomock.Any()).
		DoAndReturn(func(ctx context.Context, req *websearch.ContentsRequest) (*websearch.ContentsResponse, error) {
			require.Equal(t, []string{testPageURL}, req.URLs)
			require.True(t, req.Text.Enabled)
			require.Nil(t, req.MaxAgeHours)
			requireTurnCaller(t, ctx)
			return pageResponse("# Docs\n\nHello"), nil
		})

	out, err := tool.Execute(turnContext(), agent.Meta{UserID: "user_1"}, browserArgs())
	require.NoError(t, err)
	require.Equal(t, "# Docs\n\nHello", out)
}

func TestBrowserToolReportsPageFailure(t *testing.T) {
	tool, provider, _ := newTestBrowserTool(t, true)
	provider.EXPECT().Contents(gomock.Any(), gomock.Any()).Return(&websearch.ContentsResponse{
		Statuses: []websearch.ContentStatus{{
			ID:     testPageURL,
			Status: websearch.ContentStatusError,
			Error:  &websearch.ContentError{Tag: websearch.TagCrawlHTTPError, HTTPStatusCode: 404},
		}},
	}, nil)

	_, err := tool.Execute(context.Background(), agent.Meta{}, browserArgs())
	require.EqualError(t, err, "error fetching https://example.com/docs: CRAWL_HTTP_ERROR (HTTP 404)")
}

func TestBrowserToolReportsBlockedDestination(t *testing.T) {
	tool, provider, _ := newTestBrowserTool(t, false)
	provider.EXPECT().Contents(gomock.Any(), gomock.Any()).Return(&websearch.ContentsResponse{
		Statuses: []websearch.ContentStatus{{
			ID:     testPageURL,
			Status: websearch.ContentStatusError,
			Error:  &websearch.ContentError{Tag: websearch.TagBlockedDestination},
		}},
	}, nil)

	_, err := tool.Execute(context.Background(), agent.Meta{}, browserArgs())
	require.EqualError(t, err, "error fetching https://example.com/docs: BLOCKED_DESTINATION")
}

func TestBrowserToolReportsProviderError(t *testing.T) {
	tool, provider, _ := newTestBrowserTool(t, false)
	providerErr := websearch.InvalidRequest("urls[0] is not a valid URL")
	provider.EXPECT().Contents(gomock.Any(), gomock.Any()).Return(nil, providerErr)

	_, err := tool.Execute(context.Background(), agent.Meta{}, browserArgs())
	require.ErrorIs(t, err, providerErr)
	require.ErrorContains(t, err, "error fetching https://example.com/docs")
}

func TestBrowserToolAnswersPromptFromPage(t *testing.T) {
	tool, provider, client := newTestBrowserTool(t, true)
	provider.EXPECT().Contents(gomock.Any(), gomock.Any()).Return(pageResponse("# Docs\n\nHello"), nil)
	client.EXPECT().CreateChatCompletion(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, req openai.ChatCompletionRequest) (openai.ChatCompletionResponse, error) {
			require.Equal(t, "small", req.Model)
			require.Contains(t, req.Messages[1].Content, "what is it about?")
			require.Contains(t, req.Messages[1].Content, "# Docs\n\nHello")
			return openai.ChatCompletionResponse{Choices: []openai.ChatCompletionChoice{
				{Message: openai.ChatCompletionMessage{Content: "It is a greeting."}},
			}}, nil
		})

	out, err := tool.Execute(context.Background(), agent.Meta{}, browserArgs())
	require.NoError(t, err)
	require.Equal(t, "It is a greeting.", out)
}

func TestBrowserToolReportsAnswerFailure(t *testing.T) {
	tool, provider, client := newTestBrowserTool(t, true)
	provider.EXPECT().Contents(gomock.Any(), gomock.Any()).Return(pageResponse("text"), nil)
	client.EXPECT().CreateChatCompletion(gomock.Any(), gomock.Any()).
		Return(openai.ChatCompletionResponse{}, errors.New("model unavailable"))

	_, err := tool.Execute(context.Background(), agent.Meta{}, browserArgs())
	require.EqualError(t, err, "error processing output: model unavailable")
}
