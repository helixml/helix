package native

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	goopenai "github.com/sashabaranov/go-openai"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	helixopenai "github.com/helixml/helix/api/pkg/openai"
	"github.com/helixml/helix/api/pkg/openai/manager"
	"github.com/helixml/helix/api/pkg/websearch"
)

var testModels = map[Tier]string{TierFast: "fast-m", TierDefault: "default-m", TierStrong: "strong-m"}

var testSources = []Source{
	{URL: "https://a.example", Title: "Alpha", Text: "Alpha text"},
	{URL: "https://b.example", Title: "Beta", Text: "Beta text"},
}

func newTestSynth(t *testing.T) (*LLMSynthesizer, *helixopenai.MockClient, context.Context) {
	ctrl := gomock.NewController(t)
	providers := manager.NewMockProviderManager(ctrl)
	client := helixopenai.NewMockClient(ctrl)
	ctx := websearch.WithCaller(context.Background(), websearch.Caller{UserID: "usr_1"})
	providers.EXPECT().GetClient(gomock.Any(), &manager.GetClientRequest{Provider: "openai", Owner: "usr_1"}).Return(client, nil).AnyTimes()
	s, err := NewLLMSynthesizer(providers, "openai", testModels)
	require.NoError(t, err)
	return s, client, ctx
}

func reply(content string) goopenai.ChatCompletionResponse {
	return goopenai.ChatCompletionResponse{Choices: []goopenai.ChatCompletionChoice{{Message: goopenai.ChatCompletionMessage{Content: content}}}}
}

func schemaOf(t *testing.T, req goopenai.ChatCompletionRequest) map[string]any {
	require.NotNil(t, req.ResponseFormat)
	require.Equal(t, goopenai.ChatCompletionResponseFormatTypeJSONSchema, req.ResponseFormat.Type)
	b, err := json.Marshal(req.ResponseFormat.JSONSchema.Schema)
	require.NoError(t, err)
	var m map[string]any
	require.NoError(t, json.Unmarshal(b, &m))
	return m
}

// sseStream returns a real *ChatCompletionStream that yields the given deltas.
func sseStream(t *testing.T, deltas ...string) *goopenai.ChatCompletionStream {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		for _, d := range deltas {
			b, _ := json.Marshal(map[string]any{"choices": []any{map[string]any{"index": 0, "delta": map[string]any{"content": d}}}})
			fmt.Fprintf(w, "data: %s\n\n", b)
		}
		fmt.Fprint(w, "data: [DONE]\n\n")
	}))
	t.Cleanup(srv.Close)
	cfg := goopenai.DefaultConfig("k")
	cfg.BaseURL = srv.URL
	stream, err := goopenai.NewClientWithConfig(cfg).CreateChatCompletionStream(context.Background(), goopenai.ChatCompletionRequest{Model: "m"})
	require.NoError(t, err)
	return stream
}

func TestNewLLMSynthesizerRequiresProviderAndAllTiers(t *testing.T) {
	_, err := NewLLMSynthesizer(nil, "", testModels)
	require.ErrorContains(t, err, "provider is required")
	_, err = NewLLMSynthesizer(nil, "openai", map[Tier]string{TierDefault: "m"})
	require.ErrorContains(t, err, "no model for tier")
}

func TestSummarizeWithSchemaUsesStrictStructuredOutput(t *testing.T) {
	s, client, ctx := newTestSynth(t)
	schema := json.RawMessage(`{"$schema":"http://json-schema.org/draft-07/schema#","type":"object","properties":{"name":{"type":"string"}},"required":["name"],"additionalProperties":false}`)
	client.EXPECT().CreateChatCompletion(ctx, gomock.Any()).DoAndReturn(func(_ context.Context, req goopenai.ChatCompletionRequest) (goopenai.ChatCompletionResponse, error) {
		require.Equal(t, "fast-m", req.Model)
		require.True(t, req.ResponseFormat.JSONSchema.Strict)
		require.NotContains(t, schemaOf(t, req), "$schema")
		require.Contains(t, req.Messages[0].Content, "Focus the summary on: who")
		require.Contains(t, req.Messages[1].Content, "page body")
		return reply(` {"name": "Ada"} `), nil
	})
	out, err := s.Summarize(ctx, "page body", "who", schema)
	require.NoError(t, err)
	require.Equal(t, `{"name":"Ada"}`, out)
}

func TestSummarizeNonStrictSchemaAndInvalidJSON(t *testing.T) {
	s, client, ctx := newTestSynth(t)
	schema := json.RawMessage(`{"type":"object","properties":{"name":{"type":"string"}}}`)
	client.EXPECT().CreateChatCompletion(ctx, gomock.Any()).DoAndReturn(func(_ context.Context, req goopenai.ChatCompletionRequest) (goopenai.ChatCompletionResponse, error) {
		require.False(t, req.ResponseFormat.JSONSchema.Strict)
		return reply(`{"name": "Ada"`), nil
	})
	_, err := s.Summarize(ctx, "page body", "", schema)
	require.ErrorContains(t, err, "invalid JSON")
}

func TestSummarizeWrapsNonObjectSchema(t *testing.T) {
	s, client, ctx := newTestSynth(t)
	client.EXPECT().CreateChatCompletion(ctx, gomock.Any()).DoAndReturn(func(_ context.Context, req goopenai.ChatCompletionRequest) (goopenai.ChatCompletionResponse, error) {
		got := schemaOf(t, req)
		require.Equal(t, "object", got["type"])
		require.True(t, req.ResponseFormat.JSONSchema.Strict)
		return reply(`{"value":["a","b"]}`), nil
	})
	out, err := s.Summarize(ctx, "page", "", json.RawMessage(`{"type":"array","items":{"type":"string"}}`))
	require.NoError(t, err)
	require.Equal(t, `["a","b"]`, out)
}

func TestSummarizePlainText(t *testing.T) {
	s, client, ctx := newTestSynth(t)
	client.EXPECT().CreateChatCompletion(ctx, gomock.Any()).DoAndReturn(func(_ context.Context, req goopenai.ChatCompletionRequest) (goopenai.ChatCompletionResponse, error) {
		require.Nil(t, req.ResponseFormat)
		return reply("A summary."), nil
	})
	out, err := s.Summarize(ctx, "page", "", nil)
	require.NoError(t, err)
	require.Equal(t, "A summary.", out)
}

func TestExpandQueryDedupesAndCaps(t *testing.T) {
	s, client, ctx := newTestSynth(t)
	client.EXPECT().CreateChatCompletion(ctx, gomock.Any()).DoAndReturn(func(_ context.Context, req goopenai.ChatCompletionRequest) (goopenai.ChatCompletionResponse, error) {
		require.Equal(t, "fast-m", req.Model)
		require.True(t, req.ResponseFormat.JSONSchema.Strict)
		require.Contains(t, req.Messages[1].Content, "Objective: find vendors")
		return reply(`{"queries":["Go embeddings"," go EMBEDDINGS ","vector search in Go","","vector search in go","semantic search golang","extra"]}`), nil
	})
	queries, err := s.ExpandQuery(ctx, "go embeddings", "find vendors", 2)
	require.NoError(t, err)
	require.Equal(t, []string{"vector search in Go", "semantic search golang"}, queries)
}

func TestDescribeReturnsFirstLine(t *testing.T) {
	s, client, ctx := newTestSynth(t)
	client.EXPECT().CreateChatCompletion(ctx, gomock.Any()).Return(reply("\"kiwi bird conservation\"\nextra"), nil)
	q, err := s.Describe(ctx, "Kiwi", "text")
	require.NoError(t, err)
	require.Equal(t, "kiwi bird conservation", q)
}

func TestAnswerStreamsWithNumberedSources(t *testing.T) {
	s, client, ctx := newTestSynth(t)
	client.EXPECT().CreateChatCompletionStream(ctx, gomock.Any()).DoAndReturn(func(_ context.Context, req goopenai.ChatCompletionRequest) (*goopenai.ChatCompletionStream, error) {
		require.Equal(t, "strong-m", req.Model)
		require.True(t, req.Stream)
		user := req.Messages[1].Content
		require.Contains(t, user, "[1] Alpha\nURL: https://a.example\nAlpha text")
		require.Contains(t, user, "[2] Beta\nURL: https://b.example\nBeta text")
		require.Contains(t, user, "Question: why?")
		require.Contains(t, req.Messages[0].Content, "Additional instructions from the user:\nBe brief.")
		return sseStream(t, "Because ", "of Alpha [1]."), nil
	})
	var deltas []string
	answer, err := s.Answer(ctx, &AnswerSynthesis{Tier: TierStrong, Query: "why?", SystemPrompt: "Be brief.", Sources: testSources},
		func(d string) error { deltas = append(deltas, d); return nil })
	require.NoError(t, err)
	require.Equal(t, "Because of Alpha [1].", answer)
	require.Equal(t, []string{"Because ", "of Alpha [1]."}, deltas)
}

func TestAnswerNonStreamingAndUnknownTier(t *testing.T) {
	s, client, ctx := newTestSynth(t)
	client.EXPECT().CreateChatCompletion(ctx, gomock.Any()).Return(reply("Yes [2]."), nil)
	answer, err := s.Answer(ctx, &AnswerSynthesis{Tier: TierDefault, Query: "q", Sources: testSources}, nil)
	require.NoError(t, err)
	require.Equal(t, "Yes [2].", answer)

	_, err = s.Answer(ctx, &AnswerSynthesis{Query: "q"}, nil)
	require.ErrorContains(t, err, `no model for tier ""`)
}

func TestAnswerWithSchemaReturnsObject(t *testing.T) {
	s, client, ctx := newTestSynth(t)
	client.EXPECT().CreateChatCompletion(ctx, gomock.Any()).DoAndReturn(func(_ context.Context, req goopenai.ChatCompletionRequest) (goopenai.ChatCompletionResponse, error) {
		require.Equal(t, "answer", req.ResponseFormat.JSONSchema.Name)
		return reply(`{"ceo":"Ada"}`), nil
	})
	answer, err := s.Answer(ctx, &AnswerSynthesis{Tier: TierDefault, Query: "q", Sources: testSources,
		OutputSchema: json.RawMessage(`{"type":"object","properties":{"ceo":{"type":"string"}},"required":["ceo"],"additionalProperties":false}`)}, nil)
	require.NoError(t, err)
	require.Equal(t, map[string]any{"ceo": "Ada"}, answer)
}

func TestSynthesizeObjectMapsGrounding(t *testing.T) {
	s, client, ctx := newTestSynth(t)
	client.EXPECT().CreateChatCompletion(ctx, gomock.Any()).DoAndReturn(func(_ context.Context, req goopenai.ChatCompletionRequest) (goopenai.ChatCompletionResponse, error) {
		require.Equal(t, "default-m", req.Model)
		require.True(t, req.ResponseFormat.JSONSchema.Strict)
		props := schemaOf(t, req)["properties"].(map[string]any)
		item := props["grounding"].(map[string]any)["items"].(map[string]any)["properties"].(map[string]any)
		require.Equal(t, []any{"ceo", "founded"}, item["field"].(map[string]any)["enum"])
		require.Equal(t, []any{1.0, 2.0}, item["sources"].(map[string]any)["items"].(map[string]any)["enum"])
		require.Contains(t, req.Messages[1].Content, "Objective: due diligence")
		return reply(`{"content":{"ceo":"Ada","founded":1999},"grounding":[{"field":"ceo","sources":[2,1,2],"confidence":"high"},{"field":"founded","sources":[],"confidence":"low"}]}`), nil
	})
	out, err := s.Synthesize(ctx, &SearchSynthesis{Query: "q", Objective: "due diligence", Sources: testSources,
		OutputSchema: json.RawMessage(`{"type":"object","properties":{"ceo":{"type":"string"},"founded":{"type":"integer"}},"required":["ceo","founded"],"additionalProperties":false}`)}, nil)
	require.NoError(t, err)
	require.Equal(t, map[string]any{"ceo": "Ada", "founded": 1999.0}, out.Content)
	require.Equal(t, []websearch.Grounding{
		{Field: "ceo", Confidence: "high", Citations: []websearch.Citation{{URL: "https://b.example", Title: "Beta"}, {URL: "https://a.example", Title: "Alpha"}}},
		{Field: "founded", Confidence: "low", Citations: []websearch.Citation{}},
	}, out.Grounding)
}

func TestSynthesizeObjectRejectsUnknownSource(t *testing.T) {
	s, client, ctx := newTestSynth(t)
	client.EXPECT().CreateChatCompletion(ctx, gomock.Any()).Return(reply(`{"content":{"ceo":"Ada"},"grounding":[{"field":"ceo","sources":[7],"confidence":"high"}]}`), nil)
	_, err := s.Synthesize(ctx, &SearchSynthesis{Query: "q", Sources: testSources,
		OutputSchema: json.RawMessage(`{"type":"object","properties":{"ceo":{"type":"string"}}}`)}, nil)
	require.ErrorContains(t, err, "source [7] does not exist")
}

func TestSynthesizeTextStreamsAndCites(t *testing.T) {
	s, client, ctx := newTestSynth(t)
	client.EXPECT().CreateChatCompletionStream(ctx, gomock.Any()).DoAndReturn(func(_ context.Context, req goopenai.ChatCompletionRequest) (*goopenai.ChatCompletionStream, error) {
		require.Equal(t, "default-m", req.Model)
		require.Contains(t, req.Messages[0].Content, "The answer should be: two sentences")
		return sseStream(t, "Alpha [1] and ", "Beta [2] agree [9]."), nil
	})
	var got strings.Builder
	out, err := s.Synthesize(ctx, &SearchSynthesis{Query: "q", Sources: testSources,
		OutputSchema: json.RawMessage(`{"type":"text","description":"two sentences"}`)},
		func(d string) error { got.WriteString(d); return nil })
	require.NoError(t, err)
	require.Equal(t, "Alpha [1] and Beta [2] agree [9].", out.Content)
	require.Equal(t, got.String(), out.Content)
	require.Equal(t, []websearch.Grounding{{Field: "content", Confidence: "high", Citations: []websearch.Citation{
		{URL: "https://a.example", Title: "Alpha"}, {URL: "https://b.example", Title: "Beta"},
	}}}, out.Grounding)
}

func TestSynthesizeRejectsUnknownSchemaType(t *testing.T) {
	s, _, ctx := newTestSynth(t)
	_, err := s.Synthesize(ctx, &SearchSynthesis{Query: "q", OutputSchema: json.RawMessage(`{"type":"array"}`)}, nil)
	require.ErrorContains(t, err, `must be "text" or "object"`)
}

func TestNumberedSourcesTruncatesEachSource(t *testing.T) {
	rendered := numberedSources([]Source{{URL: "u", Title: "t", Text: strings.Repeat("word ", maxSourceChars)}})
	require.Less(t, len(rendered), maxSourceChars+100)
	require.Contains(t, rendered, "…")
}

func TestStrictCompatible(t *testing.T) {
	parse := func(s string) any {
		var v any
		require.NoError(t, json.Unmarshal([]byte(s), &v))
		return v
	}
	require.True(t, strictCompatible(parse(`{"type":"object","properties":{"a":{"type":"array","items":{"type":"object","properties":{"b":{"type":"string"}},"required":["b"],"additionalProperties":false}}},"required":["a"],"additionalProperties":false}`)))
	require.False(t, strictCompatible(parse(`{"type":"object","properties":{"a":{"type":"object","properties":{"b":{"type":"string"}},"required":["b"]}},"required":["a"],"additionalProperties":false}`)))
	require.False(t, strictCompatible(parse(`{"type":"object","properties":{"a":{"type":"string"},"b":{"type":"string"}},"required":["a"],"additionalProperties":false}`)))
}
