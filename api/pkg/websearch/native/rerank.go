package native

import (
	"context"
	"fmt"
	"sort"
	"strings"
)

// maxCandidatePool caps the candidates discovery returns for reranking: about
// what SearXNG's paging (discoverMaxPages) yields for one query.
const maxCandidatePool = 50

// candidatePool is how many candidates to discover for n results. Reranking
// can only promote what discovery found, so it gets a deeper pool; without a
// Ranker the engines' order is final and n is enough.
func (p *Provider) candidatePool(n int) int {
	if p.opts.Ranker == nil {
		return n
	}
	return min(max(3*n, 30), max(n, maxCandidatePool))
}

// rerank keeps the n best candidates by reciprocal rank fusion of the
// engines' order (the order candidates arrive in) and semantic similarity of
// title and snippet to query. Fusing ranks rather than blending scores needs
// no weights, so nothing is tuned to particular queries; rrfK is the
// constant used for merging queries in discovery. Without a Ranker the
// engines' order is kept.
func (p *Provider) rerank(ctx context.Context, query string, candidates []Candidate, n int) ([]Candidate, error) {
	if p.opts.Ranker == nil || len(candidates) < 2 {
		return candidates[:min(n, len(candidates))], nil
	}
	scores, err := p.semanticScores(ctx, query, candidates)
	if err != nil {
		return nil, err
	}
	bySimilarity := make([]int, len(candidates))
	for i := range bySimilarity {
		bySimilarity[i] = i
	}
	sort.SliceStable(bySimilarity, func(a, b int) bool { return scores[bySimilarity[a]] > scores[bySimilarity[b]] })
	similarityRank := make([]int, len(candidates))
	for rank, i := range bySimilarity {
		similarityRank[i] = rank
	}

	fused := make([]float64, len(candidates))
	order := make([]int, len(candidates))
	for i := range candidates {
		fused[i] = 1/float64(rrfK+i+1) + 1/float64(rrfK+similarityRank[i]+1)
		order[i] = i
	}
	sort.SliceStable(order, func(a, b int) bool { return fused[order[a]] > fused[order[b]] })

	out := make([]Candidate, 0, min(n, len(order)))
	for _, i := range order[:min(n, len(order))] {
		out = append(out, candidates[i])
	}
	return out, nil
}

// semanticScores rates each candidate's title and snippet against query.
func (p *Provider) semanticScores(ctx context.Context, query string, candidates []Candidate) ([]float64, error) {
	passages := make([]string, len(candidates))
	for i, c := range candidates {
		passages[i] = strings.TrimSpace(c.Title + "\n" + c.Snippet)
	}
	scores, err := p.opts.Ranker.Rank(ctx, query, passages)
	if err != nil {
		return nil, fmt.Errorf("rank candidates: %w", err)
	}
	if len(scores) != len(candidates) {
		return nil, fmt.Errorf("rank candidates: ranker returned %d scores for %d candidates", len(scores), len(candidates))
	}
	return scores, nil
}
