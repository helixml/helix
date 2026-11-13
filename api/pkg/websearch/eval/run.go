package eval

import (
	"context"
	"fmt"
	"time"

	"github.com/helixml/helix/api/pkg/websearch"
)

// SearchFunc is one provider's /search: a Helix client's WebSearch or the
// exa provider's Search.
type SearchFunc func(ctx context.Context, req *websearch.SearchRequest) (*websearch.SearchResponse, error)

// Run is every query's outcome against one provider.
type Run struct {
	Provider   string     `json:"provider"`
	CapturedAt time.Time  `json:"capturedAt"`
	Queries    []QueryRun `json:"queries"`
}

// QueryRun is one query's outcome. A failed search is recorded, not fatal:
// it scores as a query with no results.
type QueryRun struct {
	QueryID     string              `json:"queryId"`
	Hits        []Hit               `json:"hits"`
	Warnings    []websearch.Warning `json:"warnings,omitempty"`
	LatencyMs   int64               `json:"latencyMs"`
	CostDollars float64             `json:"costDollars"`
	Error       string              `json:"error,omitempty"`
}

// Hit is one result. An empty Text means the provider could not read the
// page.
type Hit struct {
	URL   string `json:"url"`
	Title string `json:"title"`
	Text  string `json:"text,omitempty"`
}

// Collect runs the queries one at a time, so latencies are not inflated by
// the eval's own concurrency.
func Collect(ctx context.Context, provider string, queries []Query, search SearchFunc) (*Run, error) {
	run := &Run{Provider: provider, CapturedAt: time.Now().UTC()}
	failed := 0
	for _, q := range queries {
		start := time.Now()
		resp, err := search(ctx, q.Request())
		qr := QueryRun{QueryID: q.ID, LatencyMs: time.Since(start).Milliseconds(), Hits: []Hit{}}
		if err != nil {
			if ctx.Err() != nil {
				return nil, ctx.Err()
			}
			qr.Error = err.Error()
			failed++
		} else {
			qr.Warnings = resp.Warnings
			if resp.CostDollars != nil {
				qr.CostDollars = resp.CostDollars.Total
			}
			for _, r := range resp.Results {
				h := Hit{URL: r.URL, Title: r.Title}
				if r.Text != nil {
					h.Text = *r.Text
				}
				qr.Hits = append(qr.Hits, h)
			}
		}
		run.Queries = append(run.Queries, qr)
	}
	if failed == len(queries) {
		return nil, fmt.Errorf("%s: every query failed, first error: %s", provider, run.Queries[0].Error)
	}
	return run, nil
}

// ExtendReference captures search results for the queries ref has none
// for, leaving every existing entry as it was: queries added to the set
// grow the reference without re-capturing (and re-paying for) the rest.
// It returns how many queries it captured.
func ExtendReference(ctx context.Context, ref *Run, queries []Query, search SearchFunc) (int, error) {
	have := make(map[string]bool, len(ref.Queries))
	for _, qr := range ref.Queries {
		have[qr.QueryID] = true
	}
	var missing []Query
	for _, q := range queries {
		if !have[q.ID] {
			missing = append(missing, q)
		}
	}
	if len(missing) == 0 {
		return 0, nil
	}
	added, err := Collect(ctx, ref.Provider, missing, search)
	if err != nil {
		return 0, err
	}
	ref.Queries = append(ref.Queries, added.Queries...)
	return len(missing), nil
}

func LoadRun(path string) (*Run, error) {
	var run Run
	if err := readJSON(path, &run); err != nil {
		return nil, err
	}
	return &run, nil
}

func SaveRun(path string, run *Run) error { return writeJSON(path, run) }

// byQuery indexes a run and checks it covers every query.
func (r *Run) byQuery(queries []Query) (map[string]QueryRun, error) {
	m := make(map[string]QueryRun, len(r.Queries))
	for _, qr := range r.Queries {
		m[qr.QueryID] = qr
	}
	for _, q := range queries {
		if _, ok := m[q.ID]; !ok {
			return nil, fmt.Errorf("%s run has no result for query %q: the query set changed, start a new series", r.Provider, q.ID)
		}
	}
	return m, nil
}
