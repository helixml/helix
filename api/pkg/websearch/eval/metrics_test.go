package eval

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// Expected values are worked by hand: gain 2^g-1, discount log2(rank+1).
func TestNDCG(t *testing.T) {
	cases := []struct {
		name   string
		grades []int
		ideal  []int
		want   float64
	}{
		// DCG 7 + 3/log2(3) = 8.8928; IDCG 7 + 7/log2(3) + 3/2 = 12.9165.
		{"partial", []int{3, 2, 0}, []int{0, 2, 3, 3}, 0.688482},
		// DCG 7/log2(4) = 3.5; IDCG 7.
		{"relevant page at rank 3", []int{0, 0, 3}, []int{3, 0, 0}, 0.5},
		{"swapped", []int{2, 3}, []int{3, 2}, 0.833991},
		{"perfect", []int{3, 2, 1}, []int{1, 2, 3}, 1},
		{"no results", nil, []int{3, 1}, 0},
		{"nothing relevant in the pool", []int{0, 0}, []int{0, 0, 0}, 0},
		// The 11th result is past the cut-off and earns nothing.
		{"cut at 10", []int{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 3}, []int{3}, 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			require.InDelta(t, tc.want, NDCG(tc.grades, tc.ideal, 10), 1e-6)
		})
	}
}

func TestMeanAndPrecision(t *testing.T) {
	cases := []struct {
		name   string
		grades []int
		mean5  float64
		p3     float64
		hit3   float64
	}{
		{"short list counts missing as 0", []int{3, 2}, 1.0, 2.0 / 3, 1.0 / 3},
		{"full list", []int{1, 3, 2, 0, 3, 3}, 1.8, 2.0 / 3, 1.0 / 3},
		{"grade 3 only after rank 3", []int{1, 1, 1, 3}, 1.2, 0, 0},
		{"empty", nil, 0, 0, 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			require.InDelta(t, tc.mean5, MeanAt(tc.grades, 5), 1e-9)
			require.InDelta(t, tc.p3, PrecisionAt(tc.grades, 3, 2), 1e-9)
			require.InDelta(t, tc.hit3, PrecisionAt(tc.grades, 3, 3), 1e-9)
		})
	}
}

func TestPercentile(t *testing.T) {
	values := []int64{100, 20, 90, 40, 60, 10, 80, 30, 70, 50}
	require.Equal(t, int64(50), Percentile(values, 0.5))
	require.Equal(t, int64(100), Percentile(values, 0.95))
	require.Equal(t, int64(10), Percentile(values, 0.01))
	require.Equal(t, int64(7), Percentile([]int64{7}, 0.95))
	require.Equal(t, int64(0), Percentile(nil, 0.5))
}

func TestSummarize(t *testing.T) {
	scores := []QueryScore{
		{NDCG10: 1, Mean5: 3, Mean10: 2, P3: 1, Hit3: true, LatencyMs: 100, CostDollars: 0.01,
			Hits: []GradedHit{{HasText: true}, {HasText: false}, {HasText: true}, {HasText: true}}},
		{NDCG10: 0.5, Mean5: 1, Mean10: 0.5, P3: 1.0 / 3, LatencyMs: 300, CostDollars: 0.03,
			Hits: []GradedHit{{HasText: false}}},
		{Error: "boom", LatencyMs: 200, Hits: []GradedHit{}},
	}
	got := Summarize(scores)
	require.Equal(t, 3, got.Queries)
	require.InDelta(t, 0.5, got.NDCG10, 1e-9)
	require.InDelta(t, 4.0/3, got.Mean5, 1e-9)
	require.InDelta(t, 2.5/3, got.Mean10, 1e-9)
	require.InDelta(t, 4.0/9, got.P3, 1e-9)
	require.InDelta(t, 1.0/3, got.Hit3, 1e-9)
	require.InDelta(t, 1.0/3, got.ZeroResultRate, 1e-9)
	require.InDelta(t, 2.0/5, got.FetchFailureRate, 1e-9)
	require.Equal(t, int64(200), got.LatencyP50Ms)
	require.Equal(t, int64(300), got.LatencyP95Ms)
	require.InDelta(t, 0.04/3, got.MeanCostDollars, 1e-9)
}

func TestScoreQueryGradesDuplicatesZero(t *testing.T) {
	q := Query{ID: "q1", Kind: "technical", Query: "x"}
	j := newTestJudgments(t,
		Judgment{QueryID: "q1", Key: "a.com/x", Verdict: Verdict{Grade: 3}},
		Judgment{QueryID: "q1", Key: "b.com", Verdict: Verdict{Grade: 2}},
	)
	qr := QueryRun{QueryID: "q1", Hits: []Hit{
		{URL: "https://a.com/x", Text: "t"},
		{URL: "https://www.a.com/x/"},
		{URL: "https://b.com/"},
	}}
	s, err := ScoreQuery(q, qr, j, []int{3, 2})
	require.NoError(t, err)
	require.Equal(t, []int{3, 0, 2}, []int{s.Hits[0].Grade, s.Hits[1].Grade, s.Hits[2].Grade})
	require.True(t, s.Hits[1].Duplicate)
	require.False(t, s.Hits[1].HasText)
	// DCG 7 + 3/2 = 8.5; IDCG 7 + 3/log2(3) = 8.8928.
	require.InDelta(t, 0.955831, s.NDCG10, 1e-6)
	require.True(t, s.Hit3)

	_, err = ScoreQuery(q, QueryRun{Hits: []Hit{{URL: "https://unjudged.com"}}}, j, nil)
	require.ErrorContains(t, err, "no judgment")
}
