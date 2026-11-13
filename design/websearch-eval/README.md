# Web search relevance eval

The ruler for improving the `native` web search provider (SearXNG + fetch +
rank). Each round runs the same queries against Helix, grades every result
with the same LLM judge, and compares Helix with Exa's results for the same
requests. Rounds are comparable only while everything in "Frozen" stays
unchanged.

Code: `api/pkg/websearch/eval` (library) and `helix websearch eval` (CLI).
Rounds: `r<N>-report.md` in this directory.

## Frozen for a series

| What | Where | Why it must not move |
|---|---|---|
| 40 queries (6 kinds) | `api/pkg/websearch/eval/testdata/queries.json` | Scores are means over this set |
| Exa reference | `…/testdata/exa_reference.json` | Captured once; Exa's index drifts daily |
| Judge model | `--judge-model` (default `glm-5.3-flash`) | Bound into `judgments.json` |
| Judge prompt, input layout, output schema | `judgePrompt`, `judgeInputFormat`, `judgeSchema` in `judge.go` | Fingerprinted as `promptVersion` in `judgments.json` |
| Judgments | `…/testdata/judgments.json` | One grade per (query, page), reused by every round |

The CLI refuses to run when the judge model or prompt fingerprint differs from
the one in `judgments.json`, and `--capture-reference` refuses to overwrite an
existing reference.

## How a round works

1. Every query is sent to Helix `/search` one at a time (so latency is not
   inflated by the eval) with `numResults: 10`, `contents.text.maxCharacters:
   1500` and the query's `category`. The exact same request built the Exa
   reference.
2. Helix's and Exa's results are pooled per query by page identity (URL
   normalised, scheme, `www.` and trailing slash ignored).
3. Pages the cache has no grade for are graded **pointwise and blind**: the
   judge sees the query, URL, title and the first 800 characters of the page
   text, never which provider returned it or at what rank. Pointwise grading
   keeps a page's grade independent of whatever else is in the pool, so a
   grade given in round 0 means the same in round 9. When both providers
   return a page, the judge gets the longer text. A page first graded from its
   title alone is re-graded once a later run supplies its text.
4. Metrics are computed per query, then averaged overall and per kind.

## Metrics

All relevance metrics use grades 0–3 (3 = exactly what was asked, 0 =
irrelevant, spam, wall or wrong language).

- **nDCG@10**: gain `2^grade − 1`, discount `log2(rank + 1)`, normalised by the
  best ordering of every pooled page for the query. 0 when nothing in the pool
  is relevant.
- **Mean grade @5 / @10**: missing results count 0, so short lists pay.
- **P@3**: share of the top 3 graded ≥ 2. **Hit@3**: share of queries with a
  grade-3 page in the top 3.
- **Zero-result rate**: queries with no results, including failed searches.
- **Fetch-failure rate**: results returned without page text.
- **Latency p50/p95** (nearest rank) and **mean `costDollars.total`**.

A page a provider repeats in one result list scores 0 after its first
occurrence.

## Running a round

From the repo root, against a stack running the code under test:

```bash
cd api && CGO_ENABLED=0 go build -o /tmp/helix-bin . && cd ..
HELIX_URL=http://localhost:8080 HELIX_API_KEY=… JUDGE_API_KEY=… \
  /tmp/helix-bin websearch eval --out /tmp/websearch-eval/r1
```

`--out` receives `report.md`, `report.json` (every graded hit with the judge's
reason) and `helix_run.json` (the raw Helix responses). `--run
<helix_run.json>` re-scores a saved run without searching again, e.g. after a
judge outage. Copy `report.md` here as `r<N>-report.md` with your analysis and
commit the grown `judgments.json` with it: new pages graded this round are part
of the ruler from now on.

Judging the ~750 pages of a fresh series takes about 6 minutes at the default
concurrency; later rounds judge only the pages Helix returns for the first
time.

## Starting a new series

Changing the queries, the judge model or the judge prompt invalidates every
earlier score. Do it deliberately, then:

1. Move the old `exa_reference.json` and `judgments.json` aside (they belong to
   the old series; keep them with its reports).
2. Capture a new reference once (≈ $0.30 for 40 queries):
   `EXA_API_KEY=… /tmp/helix-bin websearch eval --capture-reference`.
3. Run round 0 of the new series against unchanged code, so the new baseline
   is measured before any ranking change.
