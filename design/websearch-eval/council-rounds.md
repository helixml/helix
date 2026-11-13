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
| q1 | gate | 94aaca786 | GPT-6 (luna) | 7 | 9 | −2 | +rerank.go, dedupe.go in scope |
| q1 | gate | 94aaca786 | Opus 5.5 | 7 | 8 | −1 | |
| q1 | gate | 94aaca786 | GLM-5.3 Flash | 7 | 8 | −1 | |

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

---

## Round q1 — 2026-11-13 — scope: websearch results pipeline (+ rerank.go, dedupe.go) — commit `94aaca786` — gate

**Snapshot:** scratch pin `7c620caffb34` (in-scope files byte-identical to helix `94aaca786`), `bcq/q1/`.
16 files, ≈36k tokens (scope) + ≈25k (frozen bleve reference).
**Scope change:** `api/pkg/websearch/native/{rerank,dedupe}.go` were added to the scope. They are new files in the
results pipeline (rerank, near-duplicate key) and have no test files of their own. The delta said so. The series continues
because the old 14 files are all still in scope and the reference is unchanged; read the q0→q1 trend with that in mind.
**Gates** (clean worktree of `94aaca786`, since removed): `go build ./...`, `go vet ./pkg/websearch/... ./pkg/searxng/`,
`go test -race -count=1 ./pkg/websearch/... ./pkg/searxng/` were green.
**Reviewers:** GPT-6 luna (codex, effort high), Claude Opus 5.5 (native subagent), GLM-5.3 Flash (opencode node06, high).
Ids: `bcouncil-q1-{gpt6,opus55,glm53}`. All three returned scores. Replies are in `bcq/q1in/reply-*.md`.
**What shipped since q0** (the delta listed these): junk URL, bad date, empty engine entry and a failed later page now cost only themselves;
the SearXNG client has a 15s timeout; a cancelled ctx returns the ctx error; engine errors go to `logDegradedDiscovery`
and callers get a fixed message; the `company` category is searched as given; dedupe uses a scheme/www/slash/tracking-free key;
an RRF(k=60) rerank of engines' order × title+snippet similarity runs over a deeper pool when a Ranker is configured;
`excludeSourceDomain` is pushed into discovery; the URL re-normalisation is gone; extraction-service failures are per-page
`EXTRACTION_FAILED`. Tests deleted: the `"bad date"` and `"bad engines"` cases of `TestSearch_Errors`. They asserted the q0 defect.

### Scores (scope / reference)

| Category | GPT-6 | Opus 5.5 | GLM-5.3 |
|----------|-------|----------|---------|
| Long-term design | 7 / 9 (−2) | 7 / 8 (−1) | 7 / 8 (−1) |
| Structure | 7 / 9 (−2) | 7 / 7 (0) | 8 / 8 (0) |
| Interfaces & abstractions | 7 / 9 (−2) | 8 / 8 (0) | 8 / 8 (0) |
| Simplicity | 7 / 9 (−2) | 8 / 7 (+1) | 7 / 6 (+1) |
| Readability | 7 / 9 (−2) | 8 / 6 (+2) | 8 / 7 (+1) |
| Edge cases & failure handling | 6 / 9 (−3) | 6 / 8 (−2) | 7 / 9 (−2) |
| Tests | 6 / 9 (−3) | 7 / 7 (0) | 8 / 9 (−1) |
| Operability | 6 / 9 (−3) | 6 / 7 (−1) | 8 / 6 (+2) |
| **Overall** | **7 / 9 (−2)** | **7 / 8 (−1)** | **7 / 8 (−1)** |

**Change vs q0, Overall gap:** GPT-6 −3 → −2, Opus −2 → −1, GLM −1 → −1. Scope Overall went 6/6/7 → 7/7/7.
Scope category moves vs q0: GPT-6 Long-term design +1, Edge cases +1, Ops +1, Structure/Interfaces/Readability unchanged;
Opus Long-term design +1, Interfaces +1, Simplicity +1, Edge cases +1, Ops +1; GLM Edge cases +1, Tests +1, Ops +2,
Simplicity −1.
**Reference drift:** GPT-6 scored the frozen reference higher. Its q0 Simplicity, Edge cases, Tests and Ops were 8, and every category is
9 now, so its gaps understate its scope gains. Opus and GLM moved individual reference categories by ±1 in both directions,
with an unchanged Overall (8).
**Bar** (every reviewer Overall gap ≥ −0.5, no scope category < 7): **not met.** Overall gaps are −2, −1 and −1.
Categories below 7: GPT-6 Edge cases 6, Tests 6, Ops 6; Opus Edge cases 6, Ops 6.

### Verified findings (no fixes this round, by owner instruction; feeds round-2 planning)
I verified each finding against the code at `94aaca786`. The edge-case repros are in `bcq/q1-repro/zz_q1_repro_test.go`.
Each one fails at `94aaca786` and becomes the regression test when the fix lands.

**(a) Results-quality relevant**
1. **Engine signals are dropped before reranking** (all 3; q0 #4, still open). `discover.go:327` builds `Candidate` without
   `searxng.Result.Score`/`Engines`. `mergeRanked` (`discover.go:345-383`) keeps neither the fused score nor which queries
   returned the page. The reranker sees only position, and the eval cannot attribute a regression to a stage.
2. **Merging drops a snippet that another occurrence had** (Opus, GLM). `fillMissing` (`discover.go:386-393`) fills only
   the date and the image. Repro `TestQ1_MergeLosesSnippet`: query `q` returns `x.com/a` with a snippet at rank 2, and `q2` returns it at rank 0
   with no snippet. The merged candidate has `Snippet == ""`, so `semanticScores` (`rerank.go:63-66`) embeds only the title.
3. **Expansion queries page as deep as the rerank pool** (Opus). `candidatePool` (`rerank.go:21`) is passed as
   `DiscoverRequest.NumResults` (`search.go:114`), and every query pages until it is reached (`discover.go:297`). Repro
   `TestQ1_PoolPagingOnEveryQuery`: 3 queries with a pool of 30 make 9 SearXNG calls. Paging only the user's query that deep would make 5.
   The cost shows in r1's +1.2 s p50, and the extra calls add CAPTCHA pressure on scraped engines. Nothing bounds a whole discovery in time:
   pages are sequential at up to 15s each × 5 pages. The time bound was confirmed by reading the code, not reproduced.
4. **findSimilar returns the source page under a www or scheme variant** (GLM). `similar.go:65-67` drops the source by exact
   URL. Discovery deduplicates by the scheme- and www-free key but keeps whichever URL ranked first. Repro
   `TestQ1_SimilarKeepsSourceVariant`: source `https://src.com/post`, discovery returns `https://www.src.com/post`,
   and that URL comes back as a result.
5. **The source-domain exclusion is cut when the caller already has 10 excludes** (Opus). `similar.go:50` appends the source
   host last, and `siteOperators` keeps only the first `siteOperatorCap`=10 (`discover.go:460-466`). Repro
   `TestQ1_SourceExclusionPastCap`: the SearXNG `q` has no `-site:src.com`. The post-filter still removes the source pages,
   so the response comes back short, which is the q0 #6 symptom again.
6. **`ref` is treated as a tracking parameter** (Opus). `dedupe.go:12`. Repro `TestQ1_RefParamCollapsesDistinctPages`:
   GitLab `…/README.md?ref=main` and `?ref=v1.0` get the same key, so distinct pages collapse.
7. **The first URL of a near-duplicate wins, even `http` over `https`** (Opus). `discover.go:322-325` keeps the first seen. Repro
   `TestQ1_HTTPVariantWinsOverHTTPS` returns `http://go.dev/doc`. The result id and the later fetch use plain http.
8. **Expansions are labelled `additionalQueries[N]`** (Opus, GLM). `discover.go:172-179`. It is deliberate ("reported alike")
   but wrong for deep search. A deep request with no `additionalQueries` gets warnings at paths the caller never sent. Also, query
   weight is decided by slice index (`qi == 0`), so expansion weight cannot be tuned apart from caller-supplied queries.
   Low severity; it matters for relevance only if expansion weight is tuned.

**(b) Other engineering**
9. **A Ranker failure is a 500 that leaks the provider and model** (Opus; GLM raised the outage half). `rerank.go:71` wraps
   the error, and `websearch.AsError` turns any unknown error into `500 INTERNAL_ERROR` with `Message: err.Error()`, which the handler writes
   out (`server/websearch_handlers.go:233-239`). Repro `TestQ1_RerankFailureIsInternalAndLeaks`: status 500, message
   `"rank candidates: rank: create embeddings with openai/secret-model: status 429"`. The same applies to `ExpandQuery`
   (`search.go:105-107`) and to `mostSimilar`. Failing the search is the owner's no-fallback choice. The error class and the leak are defects.
10. **Reciprocal rank fusion is coded twice** (Opus). `discover.go:372` and `rerank.go:51` share only `rrfK`. Consolidation only; k stays.
11. **A defensive re-parse of a just-normalised URL fails the whole query** (Opus, GLM; q0 #15 residue). `discover.go:317-320`.
   It cannot be reached in practice: `NormalizeURL` has just parsed and serialised the URL. If it ever fired, it would cost the query, not the result.
12. **The SearXNG success body is decoded without a size cap** (GLM). `searxng.go:141`. The error path is capped at 4 KB (`:136`).
   Confirmed by reading. The risk is low, since SearXNG is operator-run.
13. **Object grounding fails the search on an out-of-range source number** (GLM). `synth.go:281` uses `citationsFor`, which errors,
   while the text path (`citedSources`, `synth.go:468-476`) skips such markers. The behaviour is deliberate and pinned by
   `TestSynthesizeObjectRejectsUnknownSource`. The inconsistency is real, and changing it is an owner call.
14. **Test gaps:** `dedupe.go` and `rerank.go` have no direct edge tests (Opus, GPT-6). There is no `n > maxCandidatePool` test, no
   wrong-score-count test at the `Ranker` seam (`rerank.go:73-75` handles it, untested), and no filtered-only page test. Two synth tests
   assert prompt phrases (`synth_test.go:85`, `:227`) that are exact concatenations of the code (Opus, minor).
15. **The degraded-discovery log fires at Warn whenever any engine is unresponsive** (Opus). `discover.go:129-131`, `:162`. There is no
   per-engine metric. That it fires on "nearly every search" was not measured.

### Rejected (with evidence)
- **"The pool is capped at 50 even when n is larger, so a reranked search returns fewer than n"** (GPT-6, `rerank.go:21`):
  false. `min(max(3n,30), max(n,50))` is `n` for every n > 50, so the pool is never below n.
- **"`www.example.com:443` and `example.com` get different dedupe keys"** (GPT-6, `dedupe.go:15`): false. `NormalizeURL`
  strips default ports (`normalize_url.go:23-25`) before `dedupeKey` sees the host. A non-default port is a different origin.
- **"Paging counts a URL as new before the filter, so a filtered-only page stops paging"** (GPT-6, `discover.go:326`): the
  behaviour is correct. `TestQ1_FilteredOnlyPageKeepsPaging` passes: page 1 is all excluded, and page 2's allowed result is returned.
  It remains only a test gap (see 14).
- **"Unresponsive-engine reasons in public warnings can leak response details"** (GPT-6, `discover.go:192`): not supported.
  The reasons are SearXNG's own short error labels per engine (timeout, CAPTCHA, too many requests), not response
  bodies. Engine names were already in the q0 output, and nothing new is exposed.
- **"No log line distinguishes a ranker failure"** (GLM, biggest risk): false as stated. The handler logs every ≥500 at
  Error with the wrapped error, `rank candidates: …` (`websearch_handlers.go:235-237`). The status class and the leak
  are real (see 9).
- GPT-6's line citations were again offset by 2–11 lines (for example `discover.go:329` for `:327`, `rerank.go:26` for `:21`). The substance
  was checked against the actual lines.

### Fixed this round
- None (triage only, by owner instruction; round-2 planning combines this with the relevance eval).
