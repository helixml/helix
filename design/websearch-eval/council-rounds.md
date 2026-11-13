# Backend council — score log — series "websearch results pipeline"

One block per round, append-only. The trend number is the **Overall** row.
**Gap** = scope minus reference, same reviewer, same round.

**Scope:** `api/pkg/searxng/{searxng,searxng_test}.go`,
`api/pkg/websearch/native/{discover,search,rank,provider,synth,similar}.go` + their `_test.go`
(14 files, ≈33k tokens; nothing trimmed).
**Reference:** bleve v2.6.3 (`github.com/blevesearch/bleve` @ `d6b0c82cd5c1`), `search/scorer` + `search/collector`,
non-vector files (14 files, ≈25k tokens). Frozen for this series.
**Bar:** every council reviewer's Overall gap ≥ −0.5, and no scope category below 7 (log only, never in a brief).
**Mechanics:** see `HOWTO.md` in the series scratch dir
(`/tmp/claude-1000/-home-karolis-go-src-github-com-helixml-helix/1d688f49-00db-49d0-8566-0a55f0a8e3d6/scratchpad/bcq/`).

| Round | Kind | Commit | Reviewer | Scope | Reference | Gap | Note |
|-------|------|--------|----------|-------|-----------|-----|------|
| q0 | gate | 19ea2144d | GPT-6 (luna) | 6 | 9 | −3 | baseline |
| q0 | gate | 19ea2144d | Opus 5.5 | 6 | 8 | −2 | baseline |
| q0 | gate | 19ea2144d | GLM-5.3 Flash | 7 | 8 | −1 | baseline |

---

## Round q0 — 2026-11-13 — scope: websearch results pipeline — commit `19ea2144d` — gate (baseline)

**Snapshot:** scratch pin `39f7375204f6` (in-scope files byte-identical to helix `19ea2144d`), `bcq/q0/`.
Gates in a clean worktree of `19ea2144d`: `go build ./...`, `go vet ./pkg/websearch/... ./pkg/searxng/`,
`go test -race -count=1 ./pkg/websearch/... ./pkg/searxng/` — green.
**Reviewers:** GPT-6 luna (codex, effort high), Claude Opus 5.5 (native subagent), GLM-5.3 Flash (opencode node06, high);
ids `bcouncil-q0-{gpt6,opus55,glm53}`.

### Scores (scope / reference)

| Category | GPT-6 | Opus 5.5 | GLM-5.3 |
|----------|-------|----------|---------|
| Long-term design | 6 / 9 | 6 / 8 | 7 / 9 |
| Structure | 7 / 9 | 7 / 7 | 8 / 9 |
| Interfaces & abstractions | 7 / 9 | 7 / 7 | 8 / 8 |
| Simplicity | 7 / 8 | 7 / 6 | 8 / 7 |
| Readability | 7 / 9 | 8 / 7 | 8 / 7 |
| Edge cases & failure handling | 5 / 8 | 5 / 8 | 6 / 8 |
| Tests | 6 / 8 | 7 / 8 | 7 / 9 |
| Operability | 5 / 8 | 5 / 7 | 6 / 7 |
| **Overall** | **6 / 9 (−3)** | **6 / 8 (−2)** | **7 / 8 (−1)** |

**Bar met:** no. Overall gaps −3 / −2 / −1. Categories below 7: Edge cases (5, 5, 6), Operability (5, 5, 6),
Long-term design (6, 6), Tests (6, GPT-6).

### Verified findings (no fixes this round — baseline; feeds round 1 planning)
Repro tests are saved in `bcq/q0-repro/` (they fail or log the defect at `19ea2144d`) and become the regression tests.

**(a) Relevance-relevant**
1. **One junk result URL fails the whole query, so a plain `/search` fails with 503** (all 3). `discover.go:260-262`.
   Repro `TestQ0_JunkURLSinksSearch`: a result `javascript:void(0)` next to valid results returns
   `503 search engines unavailable: query: result url "javascript:void(0)": not an absolute http(s) URL`.
2. **One unparseable `publishedDate` fails the whole SearXNG page** (all 3). `searxng.go:153-156`. The
   existing test `searxng_test.go:112` ("bad date") asserts the failure as the correct behaviour. Repro `TestQ0_BadDateSinksSearch`:
   `"13 Nov 2026"` gives a 503. The same class of problem: `searxng.go:143-144` fails the page on an empty `unresponsive_engines` entry.
3. **Junk on page ≥2 throws away the candidates from earlier pages and drops the whole query** (GPT-6). `discover.go:262`
   returns `queryRun{}` and discards `run`. Repro `TestQ0_Page2JunkDiscardsPage1`: page 1 `go.dev`, page 2 `ftp://x/` gives
   0 candidates and a "this query failed" warning.
4. **The ranking signals are parsed and then thrown away** (all 3). `discover.go:269` builds `Candidate` without
   `searxng.Result.Score`/`Engines` (`searxng.go:77-78`). The per-query ranks and the fused RRF score
   (`discover.go:317`) are also never exposed. An eval cannot explain an ordering, and a re-ranker cannot use engine agreement.
5. **Deduplication matches only exact normalised URLs** (Opus). `discover.go:264`, `:305`; `NormalizeURL` keeps the scheme,
   `www.`, the trailing slash and tracking parameters. Repro `TestQ0_DedupeVariants`: `https://go.dev/doc`, `http://go.dev/doc`,
   `https://www.go.dev/doc/` and `…?utm_source=x` give 4 results. SearXNG merges some of these within one response;
   across pages and queries nothing does. Constraint: ids remain normalised URLs (fixed decision), so a fix
   needs a separate dedupe key.
6. **`findSimilar` with `excludeSourceDomain` silently returns too few results** (Opus, GLM). `similar.go:48` asks for
   `n+1`, and `similar.go:66` filters the source host afterwards. It does not pass the source host into `ExcludeDomains`.
   Repro `TestQ0_ExcludeSourceDomainUnderfills`: `numResults:3` returns 1 result with no warning, and Discover was called
   with `ExcludeDomains=[]`.
7. **The country locale overrides the query's language** (Opus). `discover.go:197-201`: with any mapped
   `userLocation.country`, `language=<locale>` replaces `auto`. The behaviour is confirmed. Whether it hurts relevance is for the
   eval to measure (needs a non-English/mismatched-country slice).
8. **Deep-search expansions are not held to the query language, and together they outweigh the user query** (Opus).
   The prompt at `synth.go:79` has no language instruction. With `userQueryWeight=2` (`discover.go:31`) and 4 deep expansions at weight 1,
   expansions carry 4/6 of the fusion weight. The arithmetic is confirmed; the impact is for the eval to measure.
9. **Triager observation (not raised by the panel), confirmed:** `/search` order is SearXNG rank fused by RRF only.
   The configured `Ranker` is used for highlights and findSimilar but never re-ranks `/search` results (`search.go:124-157`).
   `Result.Score` is never set on `/search`, and `Candidate.Snippet` is used only by findSimilar (`similar.go:117`).
   So the cheapest relevance lever, re-ranking by title+snippet embedding, is not wired into `/search`.
10. **No offline relevance evaluation in scope** (GPT-6). True at `19ea2144d`. It is being added separately
   (`api/pkg/websearch/eval/`).

**(b) Other engineering**
11. **A cancelled or timed-out request is reported as a 503 upstream outage** (Opus). `discover.go:115-119`. Repro
   `TestQ0_CancelledIs503`: with a cancelled ctx, Discover returns a `*websearch.Error` 503 and
   `errors.Is(err, context.Canceled)==false`. The handler then logs it at Error level (`server/websearch_handlers.go:236-237`).
12. **Internal SearXNG details reach API callers** (Opus, GLM). `searxng.go:124` wraps a `*url.Error` that carries the internal URL.
   `searxng.go:130` adds up to 4 KB of response body. `discover.go:104`/`:118` put `err.Error()` into customer warnings
   and `Error.Message`, which `websearch_handlers.go:239` writes out. Repro output:
   `searxng: Get "http://127.0.0.1:…/search?engines=bing%2Cduckduckgo&…": context canceled`, `searxng: status 500: boom`.
13. **No timeout on SearXNG calls** (Opus, GLM). `searxng.go:47` uses `http.DefaultClient`. The API server runs with
   `WriteTimeout: 0`/`ReadTimeout: 0` (`server/server.go:1011-1012`), and nothing in the path adds a deadline. A stalled
   SearXNG hangs the search until the client gives up. Confirmed by reading the code; not reproduced with a hanging server.
14. **Engine degradation is invisible to operators** (Opus, GLM). The scope has no log or metric
   (grep: no `log.` in any scope file). Unresponsive engines and CAPTCHAs, failed queries and early paging stops appear only in
   per-request customer warnings.
15. **URLs are normalised 3 times, with error paths that cannot be reached** (Opus, GLM). `provider.go:118` and `similar.go:62`
   re-normalise `Candidate.URL`, which `discover.go:260` already normalised. `discover.go:442` re-parses it.

### Rejected / deferred (with evidence)
- **Empty search + `outputSchema` → 404** (GLM, marked contract change): deferred. `provider.go:149-153` is a
  deliberate earlier decision (previous series r3: "no search results at all is 404 NOT_FOUND; results that all
  failed to fetch stay 502"). Only the owner can change it, as a wire-status choice.
- GPT-6's line citations are offset by about 12 lines from the bundle (for example "discover.go:248-253" for what is at 260-262). The substance matches.
  Logged so later rounds do not read the cited lines literally.
- No claim was rejected as false this round: every other finding reproduced or was confirmed in the code.

### Fixed this round
- None (baseline round, by owner instruction).

### Biggest risk called out
- All three reviewers: engine junk (one bad URL or date) turns a single-query `/search` into a customer-visible 503 (Opus adds: the outage
  message also leaks internal infrastructure details).
