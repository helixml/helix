package eval_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/helixml/helix/api/pkg/client"
	"github.com/helixml/helix/api/pkg/websearch"
	"github.com/helixml/helix/api/pkg/websearch/eval"
)

// fakeHelix serves /api/v1/websearch/search: "broken" fails, anything else
// returns a page with text and one without.
func fakeHelix(t *testing.T, seen *[]websearch.SearchRequest) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/api/v1/websearch/search", r.URL.Path)
		var req websearch.SearchRequest
		require.NoError(t, json.NewDecoder(r.Body).Decode(&req))
		*seen = append(*seen, req)
		if req.Query == "broken" {
			http.Error(w, "engines down", http.StatusBadGateway)
			return
		}
		text := "page text"
		_ = json.NewEncoder(w).Encode(websearch.SearchResponse{
			Results: []websearch.Result{
				{URL: "https://a.test/" + req.Query, Title: "A", Text: &text},
				{URL: "https://b.test/", Title: "B"},
			},
			Warnings:    []websearch.Warning{{Code: websearch.WarningPartialResults, Message: "1 of 2 results could not be fetched"}},
			CostDollars: &websearch.CostDollars{Total: 0.012},
		})
	}))
}

func TestCollectAgainstHelix(t *testing.T) {
	var seen []websearch.SearchRequest
	srv := fakeHelix(t, &seen)
	defer srv.Close()
	c, err := client.NewClient(srv.URL, "key", false)
	require.NoError(t, err)

	queries := []eval.Query{
		{ID: "q1", Kind: "news", Query: "ok", Category: "news", NumResults: 10},
		{ID: "q2", Kind: "factual", Query: "broken", NumResults: 10},
	}
	run, err := eval.Collect(context.Background(), "helix", queries, c.WebSearch)
	require.NoError(t, err)

	require.Len(t, seen, 2)
	require.Equal(t, "news", seen[0].Category)
	require.Equal(t, 10, seen[0].NumResults)
	require.Equal(t, eval.TextChars, seen[0].Contents.Text.MaxCharacters)

	ok := run.Queries[0]
	require.Empty(t, ok.Error)
	require.Equal(t, []eval.Hit{{URL: "https://a.test/ok", Title: "A", Text: "page text"}, {URL: "https://b.test/", Title: "B"}}, ok.Hits)
	require.Equal(t, 0.012, ok.CostDollars)
	require.Len(t, ok.Warnings, 1)
	require.Contains(t, run.Queries[1].Error, "engines down")
	require.Empty(t, run.Queries[1].Hits)

	_, err = eval.Collect(context.Background(), "helix", queries[1:], c.WebSearch)
	require.ErrorContains(t, err, "every query failed")
}

type constGrader map[string]int

func (g constGrader) Grade(_ context.Context, _ eval.Query, d eval.Doc) (eval.Verdict, error) {
	return eval.Verdict{Grade: g[d.Key]}, nil
}

func TestReportEndToEnd(t *testing.T) {
	queries := []eval.Query{{ID: "q1", Kind: "technical", Query: "q", NumResults: 10}}
	candidate := &eval.Run{Provider: "helix", Queries: []eval.QueryRun{{QueryID: "q1", LatencyMs: 900, Hits: []eval.Hit{
		{URL: "https://junk.test/"}, {URL: "https://good.test/", Text: "t"},
	}}}}
	reference := &eval.Run{Provider: "exa", Queries: []eval.QueryRun{{QueryID: "q1", LatencyMs: 400, Hits: []eval.Hit{
		{URL: "https://www.good.test", Text: "t"}, {URL: "https://ok.test/", Text: "t"},
	}}}}
	pools, err := eval.Pools(queries, candidate, reference)
	require.NoError(t, err)
	require.Len(t, pools["q1"], 3)

	judgments, err := eval.LoadJudgments(filepath.Join(t.TempDir(), eval.JudgmentsFile), "m")
	require.NoError(t, err)
	_, err = eval.JudgeAll(context.Background(), constGrader{"good.test": 3, "ok.test": 2}, judgments, queries, pools, 1, func(int) error { return nil })
	require.NoError(t, err)

	rep, err := eval.BuildReport(queries, judgments, candidate, reference)
	require.NoError(t, err)
	// Ideal [3,2,0]: IDCG 7 + 3/log2(3) = 8.8928. Helix [0,3]: 7/log2(3) = 4.4165.
	require.InDelta(t, 0.496639, rep.Candidate.Overall.NDCG10, 1e-6)
	require.InDelta(t, 1, rep.Reference.Overall.NDCG10, 1e-9)
	require.InDelta(t, 0.5, rep.Candidate.Overall.FetchFailureRate, 1e-9)

	var md strings.Builder
	require.NoError(t, rep.WriteMarkdown(&md))
	require.Contains(t, md.String(), "| nDCG@10 | 0.497 | 1.000 | -0.503 |")
	require.Contains(t, md.String(), "| 0- 3 | 3 2 |")

	missing := &eval.Run{Provider: "helix"}
	_, err = eval.BuildReport(queries, judgments, missing, reference)
	require.ErrorContains(t, err, "start a new series")
}

// Growing the query set captures only the new queries; what was captured
// before (and paid for) is not searched again or changed.
func TestExtendReferenceCapturesOnlyNewQueries(t *testing.T) {
	ref := &eval.Run{Provider: "exa", Queries: []eval.QueryRun{
		{QueryID: "a", Hits: []eval.Hit{{URL: "https://old.example/", Title: "kept"}}},
	}}
	var searched []string
	search := func(_ context.Context, req *websearch.SearchRequest) (*websearch.SearchResponse, error) {
		searched = append(searched, req.Query)
		return &websearch.SearchResponse{Results: []websearch.Result{{URL: "https://new.example/" + req.Query, Title: req.Query}}}, nil
	}
	queries := []eval.Query{{ID: "a", Query: "qa", NumResults: 10}, {ID: "b", Query: "qb", NumResults: 10}}

	added, err := eval.ExtendReference(context.Background(), ref, queries, search)
	require.NoError(t, err)
	require.Equal(t, 1, added)
	require.Equal(t, []string{"qb"}, searched)
	require.Len(t, ref.Queries, 2)
	require.Equal(t, "kept", ref.Queries[0].Hits[0].Title)
	require.Equal(t, "b", ref.Queries[1].QueryID)

	added, err = eval.ExtendReference(context.Background(), ref, queries, search)
	require.NoError(t, err)
	require.Zero(t, added)
	require.Len(t, searched, 1, "a complete reference searches nothing")
}
