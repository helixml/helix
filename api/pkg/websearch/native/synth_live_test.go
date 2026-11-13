//go:build live

// Live checks against a real OpenAI-compatible API. Run with:
//
//	LIVE_API_KEY=... go test -tags live -run Live -v ./pkg/websearch/native/
//
// LIVE_BASE_URL, LIVE_FAST_MODEL, LIVE_DEFAULT_MODEL and LIVE_EMBED_MODEL
// default to OpenAI. LIVE_RAW=1 talks to the endpoint with go-openai directly
// instead of Helix's client, which routes Gemini URLs to the native SDK.
package native

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"

	goopenai "github.com/sashabaranov/go-openai"
	"github.com/stretchr/testify/require"

	helixopenai "github.com/helixml/helix/api/pkg/openai"
	"github.com/helixml/helix/api/pkg/openai/manager"
)

type liveProviders struct {
	manager.ProviderManager
	client helixopenai.Client
}

func (p liveProviders) GetClient(context.Context, *manager.GetClientRequest) (helixopenai.Client, error) {
	return p.client, nil
}

// rawClient is go-openai without Helix's provider routing.
type rawClient struct {
	helixopenai.Client
	c *goopenai.Client
}

func (r rawClient) CreateChatCompletion(ctx context.Context, req goopenai.ChatCompletionRequest) (goopenai.ChatCompletionResponse, error) {
	return r.c.CreateChatCompletion(ctx, req)
}

func (r rawClient) CreateChatCompletionStream(ctx context.Context, req goopenai.ChatCompletionRequest) (*goopenai.ChatCompletionStream, error) {
	return r.c.CreateChatCompletionStream(ctx, req)
}

func (r rawClient) CreateEmbeddings(ctx context.Context, req goopenai.EmbeddingRequest) (goopenai.EmbeddingResponse, error) {
	return r.c.CreateEmbeddings(ctx, req)
}

func liveEnv(name, def string) string {
	if v := os.Getenv(name); v != "" {
		return v
	}
	return def
}

func liveSetup(t *testing.T) liveProviders {
	key := os.Getenv("LIVE_API_KEY")
	if key == "" {
		t.Skip("LIVE_API_KEY not set")
	}
	baseURL := liveEnv("LIVE_BASE_URL", "https://api.openai.com/v1")
	if os.Getenv("LIVE_RAW") != "" {
		cfg := goopenai.DefaultConfig(key)
		cfg.BaseURL = baseURL
		return liveProviders{client: rawClient{c: goopenai.NewClientWithConfig(cfg)}}
	}
	return liveProviders{client: helixopenai.New(key, baseURL, false)}
}

var liveSources = []Source{
	{URL: "https://go.dev/doc/go1.22", Title: "Go 1.22 Release Notes", Text: "Go 1.22 was released in February 2024. Previously, the variables declared by a for loop were created once and updated by each iteration. In Go 1.22, each iteration of the loop creates new variables, to avoid accidental sharing bugs. For loops may now range over integers."},
	{URL: "https://go.dev/blog/loopvar-preview", Title: "Fixing For Loops in Go 1.22", Text: "Go 1.21 included a preview of a change to for loop scoping that we plan to ship in Go 1.22, removing one of the most common Go mistakes. The change was proposed by David Chase and Russ Cox."},
}

func TestLiveSynth(t *testing.T) {
	providers := liveSetup(t)
	s, err := NewLLMSynthesizer(providers, "openai", map[Tier]string{
		TierFast:    liveEnv("LIVE_FAST_MODEL", "gpt-4.1-nano"),
		TierDefault: liveEnv("LIVE_DEFAULT_MODEL", "gpt-4.1-mini"),
		TierStrong:  liveEnv("LIVE_DEFAULT_MODEL", "gpt-4.1-mini"),
	})
	require.NoError(t, err)
	ctx := context.Background()

	summary, err := s.Summarize(ctx, liveSources[0].Text, "loop variables", json.RawMessage(`{"$schema":"http://json-schema.org/draft-07/schema#","type":"object","properties":{"version":{"type":"string"},"change":{"type":"string"}},"required":["version","change"],"additionalProperties":false}`))
	require.NoError(t, err)
	t.Logf("summary (strict schema): %s", summary)

	summary, err = s.Summarize(ctx, liveSources[0].Text, "", json.RawMessage(`{"type":"object","properties":{"version":{"type":"string"},"releaseMonth":{"type":"string"}}}`))
	require.NoError(t, err)
	t.Logf("summary (non-strict schema): %s", summary)

	queries, err := s.ExpandQuery(ctx, "go 1.22 loop variable change", "", 3)
	require.NoError(t, err)
	t.Logf("expanded: %q", queries)

	desc, err := s.Describe(ctx, liveSources[1].Title, liveSources[1].Text)
	require.NoError(t, err)
	t.Logf("describe: %s", desc)

	var deltas int
	answer, err := s.Answer(ctx, &AnswerSynthesis{Tier: TierDefault, Query: "What changed about for loops in Go 1.22 and who proposed it?", Sources: liveSources},
		func(string) error { deltas++; return nil })
	require.NoError(t, err)
	require.Greater(t, deltas, 1)
	t.Logf("answer (%d deltas): %s", deltas, answer)

	obj, err := s.Answer(ctx, &AnswerSynthesis{Tier: TierFast, Query: "Which Go version changed loop variables?", Sources: liveSources,
		OutputSchema: json.RawMessage(`{"type":"object","properties":{"version":{"type":"string"}},"required":["version"]}`)}, nil)
	require.NoError(t, err)
	t.Logf("answer object: %v", obj)

	out, err := s.Synthesize(ctx, &SearchSynthesis{Query: "Go 1.22 loop variable change", Sources: liveSources,
		OutputSchema: json.RawMessage(`{"type":"object","properties":{"version":{"type":"string"},"proposers":{"type":"array","items":{"type":"string"}},"releaseDate":{"type":"string"}},"required":["version","proposers","releaseDate"],"additionalProperties":false}`)}, nil)
	require.NoError(t, err)
	b, _ := json.Marshal(out)
	t.Logf("synthesize object: %s", b)
	require.NotEmpty(t, out.Grounding)

	var text strings.Builder
	out, err = s.Synthesize(ctx, &SearchSynthesis{Query: "Go 1.22 loop variable change", Sources: liveSources,
		OutputSchema: json.RawMessage(`{"type":"text","description":"two sentences"}`)}, func(d string) error { text.WriteString(d); return nil })
	require.NoError(t, err)
	require.Equal(t, text.String(), out.Content)
	b, _ = json.Marshal(out)
	t.Logf("synthesize text: %s", b)
}

func TestLiveRank(t *testing.T) {
	providers := liveSetup(t)
	r := NewEmbeddingRanker(providers, "openai", liveEnv("LIVE_EMBED_MODEL", "text-embedding-3-small"))
	text := strings.Repeat("Bananas are rich in potassium and are a popular fruit eaten worldwide. ", 6) + "\n" +
		strings.Repeat("In Go 1.22 each loop iteration gets its own copy of the loop variable. ", 6) + "\n" +
		strings.Repeat("The Eiffel Tower is in Paris and was completed in 1889 for the World's Fair. ", 6)
	highlights, scores, err := Highlights(context.Background(), r, "golang for loop semantics", text, 500)
	require.NoError(t, err)
	t.Logf("highlights: %q scores: %v", highlights, scores)
	require.Contains(t, highlights[0], "Go 1.22")
}
