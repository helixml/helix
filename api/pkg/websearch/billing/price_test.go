package billing

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/helixml/helix/api/pkg/config"
	"github.com/helixml/helix/api/pkg/websearch"
)

// testPrices are the defaults (config.WebSearchPrices).
var testPrices = config.WebSearchPrices{
	SearchLite: 0.004, Search: 0.007, SearchDeep: 0.012, ResultOver10: 0.001,
	ContentPage: 0.001, Answer: 0.005, FindSimilar: 0.007,
}

func TestSearchBase(t *testing.T) {
	for typ, want := range map[string]float64{
		"":               0.007,
		"auto":           0.007,
		"fast":           0.007,
		"lite":           0.004,
		"instant":        0.004,
		"deep":           0.012,
		"deep-lite":      0.012,
		"deep-reasoning": 0.012,
	} {
		got, err := searchBase(testPrices, typ)
		require.NoError(t, err, typ)
		require.Equal(t, want, got, typ)
	}

	_, err := searchBase(testPrices, "neural")
	e := websearch.AsError(err)
	require.Equal(t, http.StatusBadRequest, e.Status)
	require.Equal(t, websearch.TagFeatureDisabled, e.Tag)
}

func ptr(v float64) *float64 { return &v }

func str(s string) *string { return &s }

// pages returns n results carrying the given content.
func pages(n int, r websearch.Result) []websearch.Result {
	out := make([]websearch.Result, n)
	for i := range out {
		out[i] = r
	}
	return out
}

func TestResultsCost(t *testing.T) {
	text := websearch.Result{Text: str("t")}
	all := websearch.Result{Text: str("t"), Highlights: []string{"h"}, Summary: str("s")}
	for _, tc := range []struct {
		name    string
		results []websearch.Result
		want    websearch.CostDollars
	}{
		{"no results", nil, websearch.CostDollars{Total: 0.007}},
		{"ten results are included", pages(10, websearch.Result{}), websearch.CostDollars{Total: 0.007}},
		{"each result beyond ten", pages(13, websearch.Result{}), websearch.CostDollars{Total: 0.010}},
		{"text per page", pages(3, text), websearch.CostDollars{Total: 0.010, Contents: &websearch.ContentsCost{Text: ptr(0.003)}}},
		{
			"each content type per page",
			pages(2, all),
			websearch.CostDollars{Total: 0.013, Contents: &websearch.ContentsCost{Text: ptr(0.002), Highlights: ptr(0.002), Summary: ptr(0.002)}},
		},
		{
			"only content returned is charged",
			[]websearch.Result{{Highlights: []string{}}, {Summary: str("s")}},
			websearch.CostDollars{Total: 0.008, Contents: &websearch.ContentsCost{Summary: ptr(0.001)}},
		},
		{
			"subpages are pages, not results",
			[]websearch.Result{{Text: str("t"), Subpages: []websearch.Result{text, {Highlights: []string{"h"}, Subpages: []websearch.Result{text}}}}},
			websearch.CostDollars{Total: 0.011, Contents: &websearch.ContentsCost{Text: ptr(0.003), Highlights: ptr(0.001)}},
		},
		{
			"results beyond ten and their contents",
			pages(12, text),
			websearch.CostDollars{Total: 0.021, Contents: &websearch.ContentsCost{Text: ptr(0.012)}},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, &tc.want, resultsCost(testPrices, testPrices.Search, tc.results))
		})
	}
}

func TestContentsCost(t *testing.T) {
	for _, tc := range []struct {
		name    string
		results []websearch.Result
		want    websearch.CostDollars
	}{
		{"nothing returned", nil, websearch.CostDollars{}},
		{"extras and metadata are free", []websearch.Result{{Title: "t", Extras: &websearch.Extras{Links: []string{"l"}}}}, websearch.CostDollars{}},
		{
			"no base and no result charge",
			pages(12, websearch.Result{Text: str("t")}),
			websearch.CostDollars{Total: 0.012, Contents: &websearch.ContentsCost{Text: ptr(0.012)}},
		},
		{
			"text and highlights",
			[]websearch.Result{{Text: str("t"), Highlights: []string{"a", "b"}}},
			websearch.CostDollars{Total: 0.002, Contents: &websearch.ContentsCost{Text: ptr(0.001), Highlights: ptr(0.001)}},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, &tc.want, contentsCost(testPrices, tc.results))
		})
	}
}
