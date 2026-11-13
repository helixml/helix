package eval

import (
	"fmt"
	"math"
	"sort"
)

// GradedHit is a result with its judged grade. A page a provider repeats
// within one result list is graded 0 after its first occurrence.
type GradedHit struct {
	URL       string `json:"url"`
	Title     string `json:"title"`
	Grade     int    `json:"grade"`
	Reason    string `json:"reason"`
	HasText   bool   `json:"hasText"`
	Duplicate bool   `json:"duplicate,omitempty"`
}

// QueryScore is one provider's score on one query.
type QueryScore struct {
	QueryID     string      `json:"queryId"`
	Kind        string      `json:"kind"`
	Query       string      `json:"query"`
	Hits        []GradedHit `json:"hits"`
	NDCG10      float64     `json:"ndcg10"`
	Mean5       float64     `json:"mean5"`
	Mean10      float64     `json:"mean10"`
	P3          float64     `json:"p3"`
	Hit3        bool        `json:"hit3"`
	LatencyMs   int64       `json:"latencyMs"`
	CostDollars float64     `json:"costDollars"`
	Warnings    []string    `json:"warnings,omitempty"`
	Error       string      `json:"error,omitempty"`
}

// Summary aggregates query scores. Relevance metrics are means over queries.
type Summary struct {
	Queries          int     `json:"queries"`
	Mean5            float64 `json:"mean5"`
	Mean10           float64 `json:"mean10"`
	NDCG10           float64 `json:"ndcg10"`
	P3               float64 `json:"p3"`
	Hit3             float64 `json:"hit3"`
	ZeroResultRate   float64 `json:"zeroResultRate"`
	FetchFailureRate float64 `json:"fetchFailureRate"`
	LatencyP50Ms     int64   `json:"latencyP50Ms"`
	LatencyP95Ms     int64   `json:"latencyP95Ms"`
	MeanCostDollars  float64 `json:"meanCostDollars"`
}

// ScoreQuery grades one provider's results for q. ideal holds the grades of
// every pooled page for q, the ideal ranking nDCG normalises against.
func ScoreQuery(q Query, qr QueryRun, judgments *Judgments, ideal []int) (QueryScore, error) {
	s := QueryScore{QueryID: q.ID, Kind: q.Kind, Query: q.Query, LatencyMs: qr.LatencyMs, CostDollars: qr.CostDollars, Error: qr.Error, Hits: []GradedHit{}}
	for _, w := range qr.Warnings {
		s.Warnings = append(s.Warnings, w.Code+": "+w.Message)
	}
	seen := map[string]bool{}
	grades := make([]int, 0, len(qr.Hits))
	for _, h := range qr.Hits {
		key := PageKey(h.URL)
		j, ok := judgments.Lookup(q.ID, key)
		if !ok {
			return QueryScore{}, fmt.Errorf("query %s: %s has no judgment", q.ID, h.URL)
		}
		gh := GradedHit{URL: h.URL, Title: h.Title, Grade: j.Grade, Reason: j.Reason, HasText: h.Text != "", Duplicate: seen[key]}
		if gh.Duplicate {
			gh.Grade = 0
		}
		seen[key] = true
		s.Hits = append(s.Hits, gh)
		grades = append(grades, gh.Grade)
	}
	s.NDCG10 = NDCG(grades, ideal, 10)
	s.Mean5 = MeanAt(grades, 5)
	s.Mean10 = MeanAt(grades, 10)
	s.P3 = PrecisionAt(grades, 3, 2)
	s.Hit3 = PrecisionAt(grades, 3, 3) > 0
	return s, nil
}

// NDCG is nDCG@k with exponential gain (2^grade - 1) and a log2(rank + 1)
// discount, normalised by the best ordering of ideal. It is 0 when no pooled
// page is relevant.
func NDCG(grades, ideal []int, k int) float64 {
	best := append([]int(nil), ideal...)
	sort.Sort(sort.Reverse(sort.IntSlice(best)))
	idcg := dcg(best, k)
	if idcg == 0 {
		return 0
	}
	return dcg(grades, k) / idcg
}

func dcg(grades []int, k int) float64 {
	var sum float64
	for i, g := range grades {
		if i == k {
			break
		}
		sum += (math.Pow(2, float64(g)) - 1) / math.Log2(float64(i+2))
	}
	return sum
}

// MeanAt is the mean grade of the top k positions; missing results count 0.
func MeanAt(grades []int, k int) float64 {
	sum := 0
	for i := 0; i < k && i < len(grades); i++ {
		sum += grades[i]
	}
	return float64(sum) / float64(k)
}

// PrecisionAt is the share of the top k positions graded at least minGrade;
// missing results count as misses.
func PrecisionAt(grades []int, k, minGrade int) float64 {
	n := 0
	for i := 0; i < k && i < len(grades); i++ {
		if grades[i] >= minGrade {
			n++
		}
	}
	return float64(n) / float64(k)
}

// Percentile is the nearest-rank percentile (p in (0, 1]).
func Percentile(values []int64, p float64) int64 {
	if len(values) == 0 {
		return 0
	}
	sorted := append([]int64(nil), values...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i] < sorted[j] })
	rank := int(math.Ceil(p * float64(len(sorted))))
	if rank < 1 {
		rank = 1
	}
	return sorted[rank-1]
}

func Summarize(scores []QueryScore) Summary {
	s := Summary{Queries: len(scores)}
	if len(scores) == 0 {
		return s
	}
	var latencies []int64
	var hits, noText, zero int
	for _, q := range scores {
		s.Mean5 += q.Mean5
		s.Mean10 += q.Mean10
		s.NDCG10 += q.NDCG10
		s.P3 += q.P3
		if q.Hit3 {
			s.Hit3++
		}
		if len(q.Hits) == 0 {
			zero++
		}
		for _, h := range q.Hits {
			hits++
			if !h.HasText {
				noText++
			}
		}
		latencies = append(latencies, q.LatencyMs)
		s.MeanCostDollars += q.CostDollars
	}
	n := float64(len(scores))
	s.Mean5 /= n
	s.Mean10 /= n
	s.NDCG10 /= n
	s.P3 /= n
	s.Hit3 /= n
	s.MeanCostDollars /= n
	s.ZeroResultRate = float64(zero) / n
	if hits > 0 {
		s.FetchFailureRate = float64(noText) / float64(hits)
	}
	s.LatencyP50Ms = Percentile(latencies, 0.5)
	s.LatencyP95Ms = Percentile(latencies, 0.95)
	return s
}
