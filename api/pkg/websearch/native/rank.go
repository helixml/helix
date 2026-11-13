package native

import (
	"context"
	"fmt"
	"math"
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"

	goopenai "github.com/sashabaranov/go-openai"

	"github.com/helixml/helix/api/pkg/openai/manager"
	"github.com/helixml/helix/api/pkg/websearch"
)

const (
	// maxEmbeddingInputs is OpenAI's per-request input limit, query included.
	maxEmbeddingInputs = 2048

	// Highlights passages: sentences are packed into passages of
	// minPassageChars..maxPassageChars. Only the first maxHighlightPassages
	// (~150 KB of text) are embedded, which bounds the embedding cost of very
	// long pages.
	minPassageChars          = 300
	maxPassageChars          = 600
	maxHighlightPassages     = 256
	defaultHighlightMaxChars = 1500
)

// EmbeddingRanker scores passages by cosine similarity of their embeddings.
type EmbeddingRanker struct {
	providers manager.ProviderManager
	provider  string
	model     string
}

var _ Ranker = (*EmbeddingRanker)(nil)

func NewEmbeddingRanker(providers manager.ProviderManager, provider, model string) *EmbeddingRanker {
	return &EmbeddingRanker{providers: providers, provider: provider, model: model}
}

// Rank embeds the query and passages in one request. Scores are cosine
// similarity mapped from [-1,1] to [0,1]. Blank passages score 0 and are not
// sent, since embedding APIs reject empty input.
func (r *EmbeddingRanker) Rank(ctx context.Context, query string, passages []string) ([]float64, error) {
	if strings.TrimSpace(query) == "" {
		return nil, fmt.Errorf("rank: empty query")
	}
	scores := make([]float64, len(passages))
	inputs := []string{query}
	var positions []int // inputs[i+1] is passages[positions[i]]
	for i, p := range passages {
		if strings.TrimSpace(p) == "" {
			continue
		}
		inputs = append(inputs, p)
		positions = append(positions, i)
	}
	if len(positions) == 0 {
		return scores, nil
	}
	if len(inputs) > maxEmbeddingInputs {
		return nil, fmt.Errorf("rank: %d passages exceeds the limit of %d", len(positions), maxEmbeddingInputs-1)
	}

	client, err := r.providers.GetClient(ctx, &manager.GetClientRequest{
		Provider: r.provider,
		Owner:    websearch.CallerFrom(ctx).UserID,
	})
	if err != nil {
		return nil, fmt.Errorf("rank: get client for provider %q: %w", r.provider, err)
	}
	resp, err := client.CreateEmbeddings(ctx, goopenai.EmbeddingRequest{
		Input: inputs,
		Model: goopenai.EmbeddingModel(r.model),
	})
	if err != nil {
		return nil, fmt.Errorf("rank: create embeddings with %s/%s: %w", r.provider, r.model, err)
	}
	if len(resp.Data) != len(inputs) {
		return nil, fmt.Errorf("rank: got %d embeddings for %d inputs", len(resp.Data), len(inputs))
	}
	vectors := make([][]float32, len(inputs))
	for _, e := range resp.Data {
		if e.Index < 0 || e.Index >= len(inputs) || vectors[e.Index] != nil {
			return nil, fmt.Errorf("rank: invalid embedding index %d", e.Index)
		}
		vectors[e.Index] = e.Embedding
	}
	for i, pos := range positions {
		cos, err := cosine(vectors[0], vectors[i+1])
		if err != nil {
			return nil, fmt.Errorf("rank: %w", err)
		}
		scores[pos] = (cos + 1) / 2
	}
	return scores, nil
}

func cosine(a, b []float32) (float64, error) {
	if len(a) == 0 || len(a) != len(b) {
		return 0, fmt.Errorf("embedding dimensions %d and %d do not match", len(a), len(b))
	}
	var dot, na, nb float64
	for i := range a {
		dot += float64(a[i]) * float64(b[i])
		na += float64(a[i]) * float64(a[i])
		nb += float64(b[i]) * float64(b[i])
	}
	if na == 0 || nb == 0 {
		return 0, nil
	}
	return math.Max(-1, math.Min(1, dot/math.Sqrt(na*nb))), nil
}

// Highlights returns the passages of text most relevant to query, best
// first, whose combined length is at most maxChars (default 1500). Passages
// are verbatim slices of text. When no whole passage fits in maxChars, the
// best one is cut at a word boundary.
func Highlights(ctx context.Context, r Ranker, query, text string, maxChars int) ([]string, []float64, error) {
	if maxChars <= 0 {
		maxChars = defaultHighlightMaxChars
	}
	passages := splitPassages(text)
	if len(passages) == 0 {
		return nil, nil, nil
	}
	if len(passages) > maxHighlightPassages {
		passages = passages[:maxHighlightPassages]
	}
	scores, err := r.Rank(ctx, query, passages)
	if err != nil {
		return nil, nil, fmt.Errorf("highlights: %w", err)
	}
	if len(scores) != len(passages) {
		return nil, nil, fmt.Errorf("highlights: ranker returned %d scores for %d passages", len(scores), len(passages))
	}

	order := make([]int, len(passages))
	for i := range order {
		order[i] = i
	}
	sort.SliceStable(order, func(a, b int) bool { return scores[order[a]] > scores[order[b]] })

	var highlights []string
	var picked []float64
	used := 0
	for _, i := range order {
		if used+len(passages[i]) > maxChars {
			continue
		}
		highlights = append(highlights, passages[i])
		picked = append(picked, scores[i])
		used += len(passages[i])
	}
	if len(highlights) == 0 {
		best := order[0]
		return []string{cutAtWord(passages[best], maxChars)}, []float64{scores[best]}, nil
	}
	return highlights, picked, nil
}

// splitPassages packs the sentences of a Markdown document into passages of
// roughly 300-600 characters. Lines and sentence ends are boundaries; a
// sentence longer than 600 characters is cut at word boundaries. Every
// passage is a trimmed slice of text.
func splitPassages(text string) []string {
	var passages []string
	start, end := -1, -1
	flush := func() {
		if start >= 0 {
			passages = append(passages, text[start:end])
		}
		start, end = -1, -1
	}
	for _, s := range sentenceSpans(text) {
		for _, piece := range splitLong(text, s) {
			if start >= 0 && piece[1]-start > maxPassageChars {
				flush()
			}
			if start < 0 {
				start = piece[0]
			}
			end = piece[1]
			if end-start >= minPassageChars {
				flush()
			}
		}
	}
	flush()
	return passages
}

// sentenceSpans returns trimmed [start,end) spans split at newlines and at
// '.', '!' or '?' followed by whitespace.
func sentenceSpans(text string) [][2]int {
	var spans [][2]int
	add := func(from, to int) {
		for from < to && isSpace(text[from]) {
			from++
		}
		for to > from && isSpace(text[to-1]) {
			to--
		}
		if from < to {
			spans = append(spans, [2]int{from, to})
		}
	}
	from := 0
	for i := 0; i < len(text); i++ {
		switch text[i] {
		case '\n':
			add(from, i)
			from = i + 1
		case '.', '!', '?':
			if i+1 < len(text) && isSpace(text[i+1]) {
				add(from, i+1)
				from = i + 1
			}
		}
	}
	add(from, len(text))
	return spans
}

// splitLong cuts a span longer than maxPassageChars into pieces at word
// boundaries (or at a rune boundary when a single word is too long).
func splitLong(text string, span [2]int) [][2]int {
	var pieces [][2]int
	from, to := span[0], span[1]
	for to-from > maxPassageChars {
		cut := from + len(cutAtWord(text[from:to], maxPassageChars))
		pieces = append(pieces, [2]int{from, cut})
		from = cut
		for from < to && isSpace(text[from]) {
			from++
		}
	}
	if from < to {
		pieces = append(pieces, [2]int{from, to})
	}
	return pieces
}

// cutAtWord returns the longest prefix of s of at most n bytes that ends at
// a word boundary, or at a rune boundary when s has no space in range.
func cutAtWord(s string, n int) string {
	if len(s) <= n {
		return s
	}
	cut := n
	for cut > 0 && !utf8.RuneStart(s[cut]) {
		cut--
	}
	if i := strings.LastIndexFunc(s[:cut], unicode.IsSpace); i > 0 {
		cut = i
	}
	return strings.TrimRightFunc(s[:cut], unicode.IsSpace)
}

func isSpace(b byte) bool {
	return b == ' ' || b == '\t' || b == '\n' || b == '\r' || b == '\f' || b == '\v'
}
