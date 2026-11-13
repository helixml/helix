# Round 0: native provider baseline

- Code under test: `main` at `19ea2144d`, dev stack (`localhost:8080`, API via Air on this checkout).
- Native provider config: SearXNG with `SEARCH_SEARXNG_ENGINES` unset, so the default **`yandex` only**; LLM `ds4-flash-node06/glm-5.3-flash`; embeddings `test-embeddings/bge-small-en-v1.5`. The dev stack has **no `llamaindex` service**, so PDF/office extraction is unavailable.
- Exa reference captured 2026-11-13T09:23Z (`exa_reference.json`, 40 searches, $0.28). Helix run 2026-11-13T09:24Z.
- Judge `glm-5.3-flash`, prompt `f09afd17e5be`, 751 judgments (all new this round).

Generated tables follow (`helix websearch eval` output); analysis is below them.

## Overall

| Metric | helix | exa | Gap |
|---|---:|---:|---:|
| nDCG@10 | 0.670 | 0.844 | -0.174 |
| Mean grade @5 (0-3) | 2.24 | 2.57 | -0.33 |
| Mean grade @10 (0-3) | 2.13 | 2.46 | -0.33 |
| P@3 (grade >= 2) | 80.0% | 86.7% | -6.7% |
| Hit@3 (grade 3 in top 3) | 85.0% | 100.0% | -15.0% |
| Zero-result rate | 5.0% | 0.0% | 5.0% |
| Fetch-failure rate | 13.9% | 0.0% | 13.9% |
| Latency p50 (ms) | 5968 | 1225 | 4743 |
| Latency p95 (ms) | 11141 | 1771 | 9370 |
| Mean cost ($) | 0.0148 | 0.0070 | 0.0078 |

## By query kind

nDCG@10 / mean grade@5 / hit@3, helix vs exa.

| Kind | n | nDCG@10 | Mean@5 | Hit@3 | Zero results | Fetch failures |
|---|---:|---|---|---|---|---|
| company-people | 6 | 0.811 vs 0.439 | 2.27 vs 1.20 | 83% vs 100% | 0% vs 0% | 13% vs 0% |
| descriptive | 8 | 0.557 vs 0.969 | 2.03 vs 2.98 | 75% vs 100% | 12% vs 0% | 17% vs 0% |
| factual | 5 | 0.522 vs 0.864 | 1.76 vs 2.60 | 80% vs 100% | 20% vs 0% | 25% vs 0% |
| news | 6 | 0.513 vs 0.919 | 2.03 vs 2.77 | 67% vs 100% | 0% vs 0% | 2% vs 0% |
| non-english | 5 | 0.681 vs 0.855 | 2.40 vs 2.60 | 100% vs 100% | 0% vs 0% | 10% vs 0% |
| technical | 10 | 0.838 vs 0.925 | 2.68 vs 2.90 | 100% vs 100% | 0% vs 0% | 16% vs 0% |

## Per query

Sorted by helix nDCG@10 gap to exa, worst first. Grades are listed in rank order (`-` = no page text).

| Query | Kind | nDCG@10 | helix grades | exa grades |
|---|---|---|---|---|
| `desc-05` research papers on benchmarks for evaluating retrieval-augmented generation | descriptive | 0.00 vs 1.00 | error | 3 3 3 3 3 3 3 3 3 3 |
| `fact-03` who designed the Go programming language gopher mascot | factual | 0.00 vs 0.99 | error | 3 3 3 3 3 1 3 2 2 3 |
| `news-04` major cloud provider outage postmortem | news | 0.20 vs 0.98 | 2 1 2 1 1 | 3 3 3 3 3 3 3 3 2 1 |
| `desc-01` startups building open-source vector databases | descriptive | 0.31 vs 0.90 | 0 2 2- 2 2 2 2 1 0 2 | 2 3 3 3 3 3 3 3 2 3 |
| `news-01` Kubernetes new release announcement | news | 0.33 vs 0.91 | 3 1 1 0 1 1 1 1 | 3 3 3 3 1 3 2 1 3 3 |
| `intl-es` buenas prácticas de seguridad para APIs REST | non-english | 0.45 vs 0.93 | 1 2 3 2- 2 2 3 1 2 2 | 3 3 3 3 3 3 3 0 3 3 |
| `desc-03` personal blogs of engineers writing about building a compiler from scratch | descriptive | 0.55 vs 1.00 | 1 3 1- 1 3 2 2 3 3 3 | 3 3 3 3 3 3 3 3 3 3 |
| `desc-02` blog posts arguing against microservices | descriptive | 0.55 vs 1.00 | 3 2- 2 2 2 3- 2 1 2 0 | 3 3 3 3 3 3 3 3 3 3 |
| `news-03` Nvidia quarterly earnings results data center revenue | news | 0.38 vs 0.81 | 2 2 2 1 1 1 3 1 1 1 | 2 3 3 1 3 3 3 3 2 2 |
| `fact-04` maximum request payload size for a synchronous AWS Lambda invocation | factual | 0.47 vs 0.84 | 1 3 2- 1 3 1- 3 1 2 2 | 3 1 3 3 3 3 3 3 3 2 |
| `ent-01` Hugging Face | company-people | 0.47 vs 0.82 | 1 1 1 2 1 1 2- 1- 1- 1 | 3 1 1 1 1 1 1 1 1 1 |
| `news-05` OpenAI new model launch | news | 0.66 vs 1.00 | 1 3 3- 3 3 3 1 3 2 1 | 3 3 3 3 3 3 3 3 3 3 |
| `desc-07` open-source tools for running large language models locally on a laptop | descriptive | 0.60 vs 0.93 | 2 3 2 3- 2 2 3- 2 2 2 | 3 3 3 3 3 3 3 3 2 2 |
| `tech-05` react useEffect cleanup abort fetch request with AbortController | technical | 0.73 vs 1.00 | 3 2- 3- 3- 2- 2 3 2 3 1 | 3 3 3 3 3 3 3 3 3 3 |
| `desc-06` essays about the decline of the open web and SEO spam in search results | descriptive | 0.68 vs 0.92 | 3 3 3 2- 0 1 2 3 2 2 | 3 3 3 3 3 3 3 2 3 2 |
| `news-02` EU AI Act enforcement and fines | news | 0.66 vs 0.90 | 3 3 1 3 3 1 2 2 1 2 | 3 3 2 3 3 3 3 2 3 3 |
| `intl-fr` comment installer un serveur VPN WireGuard sur Ubuntu | non-english | 0.40 vs 0.64 | 1 3 2- 1 2 0 1 1 1 1- | 3 0 0 3 0 0 3 2 3 0 |
| `desc-08` case studies of companies moving from AWS to their own bare metal servers | descriptive | 0.80 vs 1.00 | 3 3- 3 3 3 2 1 2 3 1 | 3 3 3 3 3 3 3 3 3 3 |
| `tech-04` golang propagate context cancellation to worker goroutines example | technical | 0.68 vs 0.87 | 2- 3 3 1 1- 3 3 2 3 3 | 3 3 3 3 2 0 3 3 3 3 |
| `intl-ja` Pythonで非同期処理を実装する方法 asyncio | non-english | 0.82 vs 1.00 | 3 3 3 2 2 3 2 3 3 2 | 3 3 3 3 3 3 3 3 3 3 |
| `tech-03` kubernetes pod stuck in CrashLoopBackOff how to debug | technical | 0.85 vs 1.00 | 3 2 3 3 3 3 0 3- 3- 3 | 3 3 3 3 3 3 3 3 3 3 |
| `tech-09` python asyncio.gather vs TaskGroup differences in error handling | technical | 0.67 vs 0.81 | 3 2 1 3 2 3 2 2 2 2 | 3 2 3 2 3 2 3 2 3 2 |
| `fact-01` default value of max_connections in PostgreSQL | factual | 0.63 vs 0.75 | 3 1- 2 2 1 3 3- 2 1 2- | 3 1 2 3 3 3 0 2 2 3 |
| `fact-05` year the first transatlantic telegraph cable was completed | factual | 0.85 vs 0.96 | 3 3 1- 3 3 3 3 3 1 3 | 3 3 3 3 3 3 3 3 2 3 |
| `fact-02` boiling point of water at the summit of Mount Everest | factual | 0.67 vs 0.77 | 2- 2 3 3 2 2- 3- 3 3 2- | 3 2 2 3 0 3 3 3 3 3 |
| `tech-07` git undo last commit but keep the changes | technical | 0.87 vs 0.96 | 2 3- 3 3 3 3 3 3 3 3 | 3 3 3 3 3 3 3 2 3 3 |
| `intl-lt` kaip įregistruoti individualią veiklą Lietuvoje | non-english | 0.80 vs 0.88 | 3- 2 3 3 3 2 2 3 3 2 | 3 3 3 3 3 2 2 3 2 3 |
| `news-06` ransomware attack on hospital systems | news | 0.84 vs 0.91 | 3 2 3 3 3 3 3 2 2 3 | 3 3 3 3 2 3 3 3 3 2 |
| `tech-01` how to configure nginx rate limiting per client IP | technical | 0.94 vs 1.00 | 3 3 3 3 3 3 3 3 1 3 | 3 3 3 3 3 3 3 3 3 3 |
| `tech-06` rust error cannot borrow as mutable more than once at a time fix | technical | 0.85 vs 0.90 | 3- 3 2 3 2 3- 2 3 3 3 | 3 3 2 3 3 3 3 2 3 3 |
| `desc-04` companies offering self-hosted alternatives to Slack | descriptive | 0.96 vs 1.00 | 3 3 3- 3 3 3 3- 3- 3 2- | 3 3 3 3 3 3 3 3 3 3 |
| `tech-02` postgres find and kill long running queries | technical | 0.91 vs 0.92 | 3 3 3- 3 3 3 3 3 1- 2 | 3 3 3 3 3 2 3 3 3 2 |
| `tech-08` terraform state lock error with S3 backend and DynamoDB force-unlock | technical | 0.88 vs 0.87 | 3 3 2 3 3 3- 3- 3 1 3 | 2 3 3 3 3 3 3 3 3 3 |
| `tech-10` docker compose depends_on condition service_healthy healthcheck example | technical | 1.00 vs 0.92 | 3 3 3 3 3 3 3 3 3 3- | 3 3 3 3 3 3 2 3 2 3 |
| `intl-de` Kündigungsfrist Mietwohnung bei Eigenbedarf des Vermieters | non-english | 0.94 vs 0.83 | 3 3 2 3 3 3 3 3- 3 3 | 3 3 3 3 2 2 3 3 2 2 |
| `ent-04` Andrej Karpathy | company-people | 0.77 vs 0.30 | 1 3 3 3 2 3 2 2 3 3 | 3 1 0 1 0 0 1 0 0 0 |
| `ent-03` Mistral AI, French large language model startup | company-people | 0.86 vs 0.36 | 3 2 3 3 2 3- 3 0 3 2 | 3 1 1 1 1 1 0 1 1 1 |
| `ent-05` Mitchell Hashimoto, co-founder of HashiCorp | company-people | 0.85 vs 0.34 | 3 2 2- 2 2 3 3 2 3- 3 | 3 1 1 0 1 0 0 1 1 0 |
| `ent-02` Tailscale, the WireGuard-based VPN company | company-people | 1.00 vs 0.45 | 3 3 3 3- 2 2- 2 1 2 2 | 3 1 1 1 1 1 1 1 1 1 |
| `ent-06` Simon Willison, creator of Datasette | company-people | 0.91 vs 0.36 | 3 2 3 1 3 1 1 2 1 3 | 3 1 0 0 0 0 0 0 0 0 |

## Reading the numbers

- **Helix trails Exa by 0.174 nDCG@10 overall (0.670 vs 0.844), and by 0.270 (0.645 vs 0.915) without the company/people kind.** Hit@3 85% vs 100%.
- **Helix's ordering carries little relevance signal.** Mean grade by rank: Helix 2.39 at rank 1, falling to 2.17 at rank 10; Exa 2.92 at rank 1. 8 of the 38 Helix queries that returned results have a grade ≤ 1 at rank 1.
- **Company/people: Helix "wins", but Exa is a weak reference there.** These queries are entity lookups ("Andrej Karpathy", category `people`). Exa's `company`/`people` categories return one exact profile followed by similar entities or namesakes (Twingate and Netmaker for Tailscale, eight other Karpathys on LinkedIn), which the judge grades 0–1. Use the absolute grades for this kind; don't optimise toward Exa's.
- **Technical queries saturate.** Both providers sit near 0.85–0.95; 60% of all judgments are grade 3. The signal for ranking work is in descriptive, news and factual.
- **Latency and cost:** Helix p50 6.0 s / p95 11.1 s vs Exa 1.2 s / 1.8 s. Helix bills $0.0148 per query vs Exa's $0.007: Helix charges for page text per result.

## Judge spot-check (15 judgments)

A stratified sample (2 technical, 3 descriptive, 2 news, 2 company/people, 3 non-English, 3 factual; seeded random), read against the excerpt the judge saw:

| Query | Page | Judge | Mine |
|---|---|---:|---:|
| tech-05 | dev.to "Clean Up Async Requests in useEffect Hooks" | 3 | 3 |
| tech-03 | 100daysofkubernetes CrashLoopBackOff guide | 3 | 3 |
| desc-06 | SEJ "Ex-Googler answers why Google Search is getting worse" | 2 | 2 |
| desc-01 | searchcitrus.com (open-source vector DB startup) | 3 | 3 |
| desc-01 | Arabic YouTube workshop on vector search | 0 | 0 |
| news-06 | note.com commentary on Saga University hospital ransomware | 2 | 2 |
| news-04 | msn.com Google Cloud outage, "Continue reading" shell | 1 | 1 |
| ent-01 | minishlab.github.io (unrelated lab) | 1 | 1 |
| ent-03 | magistral.ai (a different, 2-person company) | 1 | 0 |
| intl-ja | techmania.jp Python `sleep` tutorial | 2 | 1 |
| intl-de | juraforum.de Eigenbedarf notice periods | 3 | 3 |
| intl-ja | docs.python.org/ja asyncio tasks | 3 | 3 |
| fact-02 | New Scientist on pressure and boiling point (no Everest figure) | 2 | 2 |
| fact-01 | Fujitsu PDF in Japanese for an English query | 0 | 0 |
| fact-01 | postgresqlco.nf max_connections (default 100) | 3 | 3 |

Agreement: 13/15 exact, 15/15 within one grade. Both disagreements are the judge being one grade lenient. The judge is usable as the ruler. Systematic effects to keep in mind:

1. **Lenient when it can't see the text.** Pages graded from URL and title alone average 2.30, against 2.37 for pages with text. Reasons often fill gaps from prior knowledge ("typically also covers…"). Fetch failures therefore cost almost nothing in relevance metrics. They are measured by the fetch-failure rate instead, so read the two together.
2. **Strict on language, by rubric.** English pages for the French query get 0 (6 pages, all on-topic WireGuard tutorials). This is what the prompt asks for, and it drags Exa's `intl-fr` down to 0.64.
3. **No length reward found.** Pages with ≥ 800 characters of text average 2.40 and pages with 300–800 average 2.38. Pages under 300 characters average 1.33, but those are mostly shells and walls.
4. **The dev-stack route to the judge ignores system messages.** Verified with a probe: a system prompt saying "reply BANANA" is ignored. With the rubric in a system message, the judge returned grades of 5, 0 and 1 for the same page. The prompt is therefore sent as the single user message. The native provider's own LLM calls on this route may lose their system prompts the same way. **Not investigated**; check before trusting `/answer` or `outputSchema` quality here.
5. Temperature 0 does not make the reasoning text deterministic. Grades were stable (4/4 identical in a probe), and the cache fixes each grade for later rounds anyway.

## Worst five queries for Helix

1. **`desc-05` RAG evaluation benchmarks: HTTP 500, no results.** One result was a PDF. Extraction calls `http://llamaindex:5000`, which does not resolve on the dev stack, and `extract document` errors are not treated as a page failure (`native/contents.go` `fetchHits` only skips `pageFailure` errors), so one unreadable page fails the whole search.
2. **`fact-03` Go gopher designer: HTTP 500.** Same cause, from a Manning sample-lesson PDF.
3. **`news-04` cloud outage postmortem: only 5 results, none a postmortem.** Category `news` routes to SearXNG's news engines, and most were suspended during the run (brave.news "too many requests", qwant news, reuters, startpage news). What came back is breaking-news coverage, three of it msn.com "Continue reading" shells. Exa returned postmortems and root-cause write-ups (Cloudflare's incident posts, Ars Technica and CNN on the AWS root cause).
4. **`desc-01` startups building open-source vector databases.** A keyword engine on a semantic query. Rank 1 is a LinkedIn post behind a login wall (0). The rest are "top 10 vector databases" listicles (2) and an Arabic YouTube video (0). Exa returned pages about the startups themselves: YC company pages (LanceDB, Epsilla), TechCrunch launch stories (Qdrant, Marqo), citrus's homepage and company LinkedIn pages. Eight of its ten are graded 3.
5. **`news-01` Kubernetes new release announcement.** 8 results; only rank 1 (K8s 1.37 on msn) is on target. The rest are KubeCon ecosystem news, a SUSE advisory and a Broadcom stock story. This is again the degraded news-engine path.

Also notable: **`ent-01` "Hugging Face"** (category `company`) never returns `huggingface.co`. The category rewrite turns the query into "Hugging Face company", which surfaces TikTok, comparison sites and employee directories.

## Hypotheses for the biggest relevance wins (ranked by expected impact)

1. **Rerank an over-fetched candidate pool by relevance before fetching.** Discover about 3× `numResults` candidates, score them against the query with the already-configured embedding model on title and snippet (or a cheap LLM pass for the top 20), then fetch the top N. *Evidence:* the near-flat grade-by-rank curve (2.39 → 2.17 vs Exa's 2.92 at rank 1); 8 queries with rank-1 grade ≤ 1; descriptive nDCG 0.56 vs 0.97, where keyword order is weakest. This targets nDCG and Hit@3 directly.
2. **Make an extraction failure a per-page failure, not a search failure.** *Evidence:* 2 of 40 queries (the whole 5% zero-result rate) returned 500 over one PDF each; each scores 0. If they recover to the Helix average, overall nDCG rises by about 0.03. Separately, either add `llamaindex` to the dev stack or disable document extraction when it is not configured, so the eval measures what production runs.
3. **Fix the news path.** Fall back to the general engines plus a freshness filter or recency boost when news engines are suspended, or use them by default. Demote shell pages (msn.com is the top low-grade domain: 10 results graded ≤ 1). *Evidence:* news nDCG 0.51 vs 0.92; news-01 and news-04 returned 8 and 5 results; every news query warned about suspended engines.
4. **Query rewriting for descriptive "find pages like…" queries.** Turn "startups building X" into searches for the entities' own pages, e.g. LLM expansion as in `deep-lite`, merged with RRF. *Evidence:* desc-01, desc-02 and desc-03 lose by 0.45–0.6 nDCG; Helix returns listicles and aggregators where Exa returns primary sources.
5. **Backfill unfetchable results and fix the top fetch failures.** 13.9% of Helix results have no text vs 0% for Exa. stackoverflow.com (8) and medium.com (5) account for most of them. Replacing an unreadable result with the next readable candidate costs nothing in relevance (the judge barely penalises title-only pages) but gives agents usable text. Lower metric impact, high practical value.
6. **A second engine merged by RRF.** The default is `yandex` alone; `bing` or `brave` would add recall for canonical pages (e.g. the missing `huggingface.co`). *Evidence is indirect*, and engines rate-limit, so measure it as its own round.
7. **Category `company`: don't append "company" to a query that already names one.** Or boost the result whose domain matches the entity name. *Evidence:* ent-01. Low priority: the kind already scores 0.81.
