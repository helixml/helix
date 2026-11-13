package eval

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// Report scores a candidate run beside the reference run on one set of
// judgments.
type Report struct {
	GeneratedAt   time.Time       `json:"generatedAt"`
	JudgeModel    string          `json:"judgeModel"`
	PromptVersion string          `json:"promptVersion"`
	NewJudgments  int             `json:"newJudgments"`
	Candidate     *ProviderReport `json:"candidate"`
	Reference     *ProviderReport `json:"reference"`
}

type ProviderReport struct {
	Provider string             `json:"provider"`
	Overall  Summary            `json:"overall"`
	ByKind   map[string]Summary `json:"byKind"`
	Queries  []QueryScore       `json:"queries"`
}

// Pools pools the runs' results per query.
func Pools(queries []Query, runs ...*Run) (map[string][]Doc, error) {
	indexed := make([]map[string]QueryRun, len(runs))
	for i, r := range runs {
		m, err := r.byQuery(queries)
		if err != nil {
			return nil, err
		}
		indexed[i] = m
	}
	pools := make(map[string][]Doc, len(queries))
	for _, q := range queries {
		var qrs []QueryRun
		for _, m := range indexed {
			qrs = append(qrs, m[q.ID])
		}
		pools[q.ID] = Pool(qrs...)
	}
	return pools, nil
}

// BuildReport scores both runs. nDCG's ideal ranking for a query is the best
// ordering of every page either provider returned for it.
func BuildReport(queries []Query, judgments *Judgments, candidate, reference *Run) (*Report, error) {
	pools, err := Pools(queries, candidate, reference)
	if err != nil {
		return nil, err
	}
	ideal := make(map[string][]int, len(queries))
	for _, q := range queries {
		for _, d := range pools[q.ID] {
			j, ok := judgments.Lookup(q.ID, d.Key)
			if !ok {
				return nil, fmt.Errorf("query %s: %s has no judgment", q.ID, d.URL)
			}
			ideal[q.ID] = append(ideal[q.ID], j.Grade)
		}
	}
	rep := &Report{GeneratedAt: time.Now().UTC(), JudgeModel: judgments.JudgeModel, PromptVersion: judgments.PromptVersion}
	for _, side := range []struct {
		run *Run
		out **ProviderReport
	}{{candidate, &rep.Candidate}, {reference, &rep.Reference}} {
		runs, err := side.run.byQuery(queries)
		if err != nil {
			return nil, err
		}
		pr := &ProviderReport{Provider: side.run.Provider, ByKind: map[string]Summary{}}
		byKind := map[string][]QueryScore{}
		for _, q := range queries {
			s, err := ScoreQuery(q, runs[q.ID], judgments, ideal[q.ID])
			if err != nil {
				return nil, err
			}
			pr.Queries = append(pr.Queries, s)
			byKind[q.Kind] = append(byKind[q.Kind], s)
		}
		pr.Overall = Summarize(pr.Queries)
		for kind, scores := range byKind {
			pr.ByKind[kind] = Summarize(scores)
		}
		*side.out = pr
	}
	return rep, nil
}

// WriteMarkdown renders the report: headline metrics, metrics per query kind
// and the candidate's weakest queries.
func (r *Report) WriteMarkdown(w io.Writer) error {
	c, ref := r.Candidate, r.Reference
	var b strings.Builder
	fmt.Fprintf(&b, "# Web search relevance: %s vs %s\n\n", c.Provider, ref.Provider)
	fmt.Fprintf(&b, "Generated %s. Judge `%s`, prompt `%s`, %d queries, %d new judgments this run.\n\n",
		r.GeneratedAt.Format(time.RFC3339), r.JudgeModel, r.PromptVersion, c.Overall.Queries, r.NewJudgments)

	b.WriteString("## Overall\n\n")
	fmt.Fprintf(&b, "| Metric | %s | %s | Gap |\n|---|---:|---:|---:|\n", c.Provider, ref.Provider)
	for _, m := range summaryMetrics {
		cv, rv := m.value(c.Overall), m.value(ref.Overall)
		fmt.Fprintf(&b, "| %s | %s | %s | %s |\n", m.name, m.format(cv), m.format(rv), m.format(cv-rv))
	}

	b.WriteString("\n## By query kind\n\nnDCG@10 / mean grade@5 / hit@3, " + c.Provider + " vs " + ref.Provider + ".\n\n")
	b.WriteString("| Kind | n | nDCG@10 | Mean@5 | Hit@3 | Zero results | Fetch failures |\n|---|---:|---|---|---|---|---|\n")
	kinds := make([]string, 0, len(c.ByKind))
	for k := range c.ByKind {
		kinds = append(kinds, k)
	}
	sort.Strings(kinds)
	for _, k := range kinds {
		cs, rs := c.ByKind[k], ref.ByKind[k]
		fmt.Fprintf(&b, "| %s | %d | %.3f vs %.3f | %.2f vs %.2f | %.0f%% vs %.0f%% | %.0f%% vs %.0f%% | %.0f%% vs %.0f%% |\n",
			k, cs.Queries, cs.NDCG10, rs.NDCG10, cs.Mean5, rs.Mean5, 100*cs.Hit3, 100*rs.Hit3,
			100*cs.ZeroResultRate, 100*rs.ZeroResultRate, 100*cs.FetchFailureRate, 100*rs.FetchFailureRate)
	}

	b.WriteString("\n## Per query\n\nSorted by " + c.Provider + " nDCG@10 gap to " + ref.Provider + ", worst first. Grades are listed in rank order (`-` = no page text).\n\n")
	b.WriteString("| Query | Kind | nDCG@10 | " + c.Provider + " grades | " + ref.Provider + " grades |\n|---|---|---|---|---|\n")
	refByID := map[string]QueryScore{}
	for _, s := range ref.Queries {
		refByID[s.QueryID] = s
	}
	order := append([]QueryScore(nil), c.Queries...)
	sort.SliceStable(order, func(i, j int) bool {
		return order[i].NDCG10-refByID[order[i].QueryID].NDCG10 < order[j].NDCG10-refByID[order[j].QueryID].NDCG10
	})
	for _, s := range order {
		rs := refByID[s.QueryID]
		fmt.Fprintf(&b, "| `%s` %s | %s | %.2f vs %.2f | %s | %s |\n",
			s.QueryID, escapeCell(s.Query), s.Kind, s.NDCG10, rs.NDCG10, gradeString(s), gradeString(rs))
	}
	_, err := io.WriteString(w, b.String())
	return err
}

type summaryMetric struct {
	name   string
	value  func(Summary) float64
	format func(float64) string
}

var summaryMetrics = []summaryMetric{
	{"nDCG@10", func(s Summary) float64 { return s.NDCG10 }, fixed(3)},
	{"Mean grade @5 (0-3)", func(s Summary) float64 { return s.Mean5 }, fixed(2)},
	{"Mean grade @10 (0-3)", func(s Summary) float64 { return s.Mean10 }, fixed(2)},
	{"P@3 (grade >= 2)", func(s Summary) float64 { return s.P3 }, percent},
	{"Hit@3 (grade 3 in top 3)", func(s Summary) float64 { return s.Hit3 }, percent},
	{"Zero-result rate", func(s Summary) float64 { return s.ZeroResultRate }, percent},
	{"Fetch-failure rate", func(s Summary) float64 { return s.FetchFailureRate }, percent},
	{"Latency p50 (ms)", func(s Summary) float64 { return float64(s.LatencyP50Ms) }, fixed(0)},
	{"Latency p95 (ms)", func(s Summary) float64 { return float64(s.LatencyP95Ms) }, fixed(0)},
	{"Mean cost ($)", func(s Summary) float64 { return s.MeanCostDollars }, fixed(4)},
}

func fixed(digits int) func(float64) string {
	return func(v float64) string { return fmt.Sprintf("%.*f", digits, v) }
}

func percent(v float64) string { return fmt.Sprintf("%.1f%%", 100*v) }

func gradeString(s QueryScore) string {
	if s.Error != "" {
		return "error"
	}
	if len(s.Hits) == 0 {
		return "none"
	}
	parts := make([]string, len(s.Hits))
	for i, h := range s.Hits {
		parts[i] = fmt.Sprint(h.Grade)
		if !h.HasText {
			parts[i] += "-"
		}
	}
	return strings.Join(parts, " ")
}

func escapeCell(s string) string { return strings.ReplaceAll(s, "|", `\|`) }

// WriteReportFiles writes report.md and report.json to dir.
func WriteReportFiles(dir string, r *Report) error {
	var md strings.Builder
	if err := r.WriteMarkdown(&md); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(dir, "report.md"), []byte(md.String()), 0o644); err != nil {
		return err
	}
	return writeJSON(filepath.Join(dir, "report.json"), r)
}
