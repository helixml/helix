package native

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/helixml/helix/api/pkg/websearch"
)

func TestFindSimilar(t *testing.T) {
	e := newEnv(t)
	e.pipeline(nil)
	e.pages["https://src.com/post"] = &Page{Title: "Source", Text: "about rust"}
	e.synth.EXPECT().Describe(gomock.Any(), "Source", "about rust").Return("rust articles", nil)
	got := e.discover(cands("https://src.com/post", "https://www.src.com/other", "https://b.com/", "https://c.com/")...)
	e.rank.EXPECT().Rank(gomock.Any(), "rust articles",
		[]string{"Candidate https://b.com/\nsnippet https://b.com/", "Candidate https://c.com/\nsnippet https://c.com/"}).
		Return([]float64{0.2, 0.9}, nil)
	e.rank.EXPECT().Rank(gomock.Any(), "rust articles", gomock.Any()).Return([]float64{0.5}, nil).Times(2)

	resp, err := e.provider(t).FindSimilar(context.Background(), &websearch.FindSimilarRequest{
		URL: "https://src.com/post", NumResults: 2, ExcludeSourceDomain: true, Category: "news",
		Contents: &websearch.ContentsOptions{Highlights: &websearch.HighlightsOption{Enabled: true}},
	})
	require.NoError(t, err)
	assert.Equal(t, []string{"rust articles"}, got.Queries)
	assert.Equal(t, 3, got.NumResults, "one extra, for the source page")
	assert.Equal(t, "news", got.Category)
	assert.Equal(t, []string{"https://c.com/", "https://b.com/"}, resultURLs(resp.Results), "sorted by score")
	assert.Equal(t, 0.9, *resp.Results[0].Score)
	assert.Equal(t, []float64{0.5}, resp.Results[0].HighlightScores)
	assert.True(t, strings.HasPrefix(resp.RequestID, "req_"))
}

func TestFindSimilarDropsRedirectedSource(t *testing.T) {
	e := newEnv(t)
	e.pipeline(nil)
	e.finals["https://src.com/old"] = "https://src.com/new"
	e.synth.EXPECT().Describe(gomock.Any(), gomock.Any(), gomock.Any()).Return("d", nil)
	e.discover(cands("https://src.com/new", "https://b.com/")...)
	e.rank.EXPECT().Rank(gomock.Any(), "d", gomock.Len(1)).Return([]float64{0.1}, nil)
	resp, err := e.provider(t).FindSimilar(context.Background(), &websearch.FindSimilarRequest{URL: "https://src.com/old"})
	require.NoError(t, err)
	assert.Equal(t, []string{"https://b.com/"}, resultURLs(resp.Results), "the page the source redirects to is the source")
}

func TestFindSimilarKeepsSourceDomainByDefault(t *testing.T) {
	e := newEnv(t)
	e.pipeline(nil)
	e.synth.EXPECT().Describe(gomock.Any(), gomock.Any(), gomock.Any()).Return("d", nil)
	e.discover(cands("https://src.com/", "https://src.com/other")...)
	e.rank.EXPECT().Rank(gomock.Any(), "d", gomock.Len(1)).Return([]float64{0.1}, nil)
	resp, err := e.provider(t).FindSimilar(context.Background(), &websearch.FindSimilarRequest{URL: "https://src.com"})
	require.NoError(t, err)
	assert.Equal(t, []string{"https://src.com/other"}, resultURLs(resp.Results), "only the source page itself is dropped")
}

func TestFindSimilarFailures(t *testing.T) {
	t.Run("needs LLM and ranker", func(t *testing.T) {
		for _, drop := range []func(*Options){
			func(o *Options) { o.Synthesizer = nil },
			func(o *Options) { o.Ranker = nil },
		} {
			e := newEnv(t)
			drop(&e.opts)
			_, err := e.provider(t).FindSimilar(context.Background(), &websearch.FindSimilarRequest{URL: "https://a.com"})
			requireTag(t, err, websearch.TagFeatureDisabled)
		}
	})
	t.Run("source cannot be fetched", func(t *testing.T) {
		e := newEnv(t)
		e.pipeline(map[string]error{"https://a.com/": &FetchError{Tag: websearch.TagCrawlTimeout, Err: errors.New("slow")}})
		_, err := e.provider(t).FindSimilar(context.Background(), &websearch.FindSimilarRequest{URL: "https://a.com"})
		requireTag(t, err, websearch.TagInvalidRequest)
		assert.Contains(t, err.Error(), websearch.TagCrawlTimeout)
	})
	t.Run("ranker score count mismatch", func(t *testing.T) {
		e := newEnv(t)
		e.pipeline(nil)
		e.synth.EXPECT().Describe(gomock.Any(), gomock.Any(), gomock.Any()).Return("d", nil)
		e.discover(cands("https://b.com/", "https://c.com/")...)
		e.rank.EXPECT().Rank(gomock.Any(), gomock.Any(), gomock.Any()).Return([]float64{0.1}, nil)
		_, err := e.provider(t).FindSimilar(context.Background(), &websearch.FindSimilarRequest{URL: "https://a.com"})
		require.ErrorContains(t, err, "1 scores for 2 pages")
	})
}

func TestFindSimilarWarnings(t *testing.T) {
	e := newEnv(t)
	e.pipeline(map[string]error{"https://c.com/": &FetchError{Tag: websearch.TagCrawlTimeout, Err: errors.New("slow")}})
	e.synth.EXPECT().Describe(gomock.Any(), gomock.Any(), gomock.Any()).Return("d", nil)
	e.discover(cands("https://b.com/", "https://c.com/")...)
	e.rank.EXPECT().Rank(gomock.Any(), "d", gomock.Len(2)).Return([]float64{0.1, 0.2}, nil)
	day := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	resp, err := e.provider(t).FindSimilar(context.Background(), &websearch.FindSimilarRequest{
		URL: "https://src.com", StartCrawlDate: &day,
		Contents: &websearch.ContentsOptions{Text: &websearch.TextOption{Enabled: true}, Extras: &websearch.ExtrasOptions{CodeBlocks: 1}},
	})
	require.NoError(t, err)
	var codes []string
	for _, w := range resp.Warnings {
		codes = append(codes, w.Code+" "+w.Path)
	}
	assert.Equal(t, []string{
		"IGNORED_FIELD startCrawlDate",
		"IGNORED_FIELD contents.extras.codeBlocks",
		"PARTIAL_RESULTS ",
	}, codes)
	assert.Contains(t, resp.Warnings[2].Message, "1 of 2 results could not be fetched")
}
