package native

import (
	"context"
	"strings"
	"testing"

	goopenai "github.com/sashabaranov/go-openai"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	helixopenai "github.com/helixml/helix/api/pkg/openai"
	"github.com/helixml/helix/api/pkg/openai/manager"
	"github.com/helixml/helix/api/pkg/websearch"
)

func TestSplitPassagesAreVerbatimAndBounded(t *testing.T) {
	var b strings.Builder
	b.WriteString("# Title\n\n")
	for i := 0; i < 40; i++ {
		b.WriteString("This is sentence number ")
		b.WriteString(strings.Repeat("x", i%7))
		b.WriteString(" about Go embeddings. ")
		if i%9 == 8 {
			b.WriteString("\n\n- a list item\n")
		}
	}
	b.WriteString(strings.Repeat("longword ", 150)) // one 1350-char "sentence"
	text := b.String()

	passages := splitPassages(text)
	require.Greater(t, len(passages), 3)
	for _, p := range passages {
		require.Contains(t, text, p)
		require.LessOrEqual(t, len(p), maxPassageChars)
		require.Equal(t, strings.TrimSpace(p), p)
		require.NotEmpty(t, p)
	}
	require.Greater(t, len(text)/len(passages), minPassageChars*3/4)
}

func TestSplitPassagesShortText(t *testing.T) {
	require.Equal(t, []string{"Hello world. Bye."}, splitPassages("  Hello world. Bye.\n"))
	require.Empty(t, splitPassages(" \n\n "))
}

func TestEmbeddingRankerRank(t *testing.T) {
	ctrl := gomock.NewController(t)
	providers := manager.NewMockProviderManager(ctrl)
	client := helixopenai.NewMockClient(ctrl)
	ctx := websearch.WithCaller(context.Background(), websearch.Caller{UserID: "usr_1"})

	providers.EXPECT().GetClient(ctx, &manager.GetClientRequest{Provider: "openai", Owner: "usr_1"}).Return(client, nil)
	client.EXPECT().CreateEmbeddings(ctx, gomock.Any()).DoAndReturn(func(_ context.Context, req goopenai.EmbeddingRequest) (goopenai.EmbeddingResponse, error) {
		require.Equal(t, goopenai.EmbeddingModel("text-embedding-3-small"), req.Model)
		require.Equal(t, []string{"query", "same", "opposite"}, req.Input) // blank passage not sent
		// Returned out of order on purpose: Index decides the mapping.
		return goopenai.EmbeddingResponse{Data: []goopenai.Embedding{
			{Index: 2, Embedding: []float32{-1, 0}},
			{Index: 0, Embedding: []float32{1, 0}},
			{Index: 1, Embedding: []float32{2, 0}},
		}}, nil
	})

	r := NewEmbeddingRanker(providers, "openai", "text-embedding-3-small")
	scores, err := r.Rank(ctx, "query", []string{"same", "  ", "opposite"})
	require.NoError(t, err)
	require.InDeltaSlice(t, []float64{1, 0, 0}, scores, 1e-9)
}

func TestEmbeddingRankerRejectsShortResponse(t *testing.T) {
	ctrl := gomock.NewController(t)
	providers := manager.NewMockProviderManager(ctrl)
	client := helixopenai.NewMockClient(ctrl)
	providers.EXPECT().GetClient(gomock.Any(), gomock.Any()).Return(client, nil)
	client.EXPECT().CreateEmbeddings(gomock.Any(), gomock.Any()).Return(goopenai.EmbeddingResponse{Data: []goopenai.Embedding{{Embedding: []float32{1}}}}, nil)

	_, err := NewEmbeddingRanker(providers, "openai", "m").Rank(context.Background(), "q", []string{"a"})
	require.ErrorContains(t, err, "got 1 embeddings for 2 inputs")
}

// keywordRanker scores a passage 1 when it contains the query, else 0.1.
type keywordRanker struct{}

func (keywordRanker) Rank(_ context.Context, query string, passages []string) ([]float64, error) {
	scores := make([]float64, len(passages))
	for i, p := range passages {
		scores[i] = 0.1
		if strings.Contains(p, query) {
			scores[i] = 1
		}
	}
	return scores, nil
}

func TestHighlightsPicksBestWithinBudget(t *testing.T) {
	filler := strings.Repeat("Unrelated filler sentence here. ", 40)
	text := filler + "\nThe kiwi bird is flightless.\n" + filler

	highlights, scores, err := Highlights(context.Background(), keywordRanker{}, "kiwi", text, 700)
	require.NoError(t, err)
	require.Contains(t, highlights[0], "kiwi")
	require.Equal(t, 1.0, scores[0])
	total := 0
	for _, h := range highlights {
		require.Contains(t, text, h)
		total += len(h)
	}
	require.LessOrEqual(t, total, 700)
	require.IsNonIncreasing(t, scores)

	highlights, _, err = Highlights(context.Background(), keywordRanker{}, "kiwi", text, 0)
	require.NoError(t, err)
	total = 0
	for _, h := range highlights {
		total += len(h)
	}
	require.LessOrEqual(t, total, defaultHighlightMaxChars)
	require.Greater(t, len(highlights), 1)
}

func TestHighlightsCutsBestWhenNothingFits(t *testing.T) {
	text := strings.Repeat("kiwi bird ", 40)
	highlights, _, err := Highlights(context.Background(), keywordRanker{}, "kiwi", text, 42)
	require.NoError(t, err)
	require.Equal(t, []string{"kiwi bird kiwi bird kiwi bird kiwi bird"}, highlights)
}

func TestHighlightsEmptyText(t *testing.T) {
	highlights, scores, err := Highlights(context.Background(), keywordRanker{}, "q", "  ", 0)
	require.NoError(t, err)
	require.Nil(t, highlights)
	require.Nil(t, scores)
}
