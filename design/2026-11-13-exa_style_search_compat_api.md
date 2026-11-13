# Exa-style search compatibility API

**Date:** 2026-11-13
**Status:** Draft, for discussion
**Goal:** Give internal customers a web search, contents, and answer API with the same wire format as the Exa API. Behind it there is one Go interface, so each deployment can run either Helix's own SearXNG + Chrome stack or Exa without changes for the callers.

Scale is out of scope. This doc covers functional parity, the interface boundary, and the work to get there.

Exa reference: OpenAPI spec `exa-labs/openapi-spec` version `2.0.0`, read 2026-11-13. Its field names are quoted exactly below.

---

## 1. What Helix has today

| Piece | Where | What it does |
|---|---|---|
| SearXNG client | `api/pkg/searxng/searxng.go` | `GET /search?format=json`. Engines hardcoded to `bing,duckduckgo,google,startpage,yandex`, `safesearch=0`. Runs several queries in parallel, dedupes by URL, sorts by SearXNG `score`, truncates to `MaxResults` (default 10). |
| SearXNG deployment | `docker-compose*.yaml`, `charts/.../searxng_*.yaml`, `searxng/settings.yml` | `searxng/searxng:latest`, JSON format enabled, botdetection limiter on. |
| Headless Chrome | `api/pkg/controller/knowledge/browser/browser.go` | go-rod against `ghcr.io/go-rod/rod` (`RAG_CRAWLER_CHROME_URL`), with browser and page pools. |
| Page → text | `knowledge/readability` (go-readability), `html-to-markdown` | Main-content extraction and Markdown conversion. |
| Site crawler | `knowledge/crawler/default.go` (colly + rod), `firecrawl.go` | Crawls one site to depth N for RAG ingestion. |
| Document extraction | `api/pkg/extract` | PDF and office documents to text through the extractor service. |
| Agent skills | `agent/skill/web_search.go`, `browser_skill.go` | `web_search` (query/category/language) and `Browser` (url + prompt, summarised by an LLM). These are only available inside Helix app agents. |
| LLM and embeddings | `/v1/chat/completions`, `/v1/embeddings`, provider routing | Available for highlights, summaries, and answers. |
| SSRF guard | `api/pkg/netguard` | Used for provider endpoints, git, and outbound calls. **Not used** by the browser, crawler, or `Browser` skill. |

**Missing:** there is no external search API. `GET /api/v1/search` already exists, but it is `unifiedSearch` over Helix's own resources, so the new API needs its own path prefix. There is also no content cache, no document IDs, no highlights, no structured summaries, no answer endpoint, and no request metering for search.

---

## 2. Exa surface and parity tiers

| Exa endpoint | Parity target | Why |
|---|---|---|
| `POST /search` | **Required** | The core endpoint. |
| `POST /contents` | **Required** | The core endpoint. Chrome makes this realistic. |
| `POST /answer` (+ SSE) | **Required** | Built from search, contents, and a Helix LLM. |
| `POST /findSimilar` | Approximate | Exa has deprecated it. Implement as a rewrite to `/search`. |
| `/agent/runs*` | Later (v2) | Async research runs. Map to the Helix agent loop once v1 works. |
| `/monitors*`, `/batches*`, `/v0/websets*` | Out of scope | Product features, not search primitives. Monitors could later be built on Helix triggers. |

### Fidelity legend

- ✅ **Native.** Same semantics with SearXNG + Chrome.
- 🟡 **Approximate.** Same field and same intent, lower fidelity. Return a `warnings[]` entry.
- 🔧 **Build.** Feasible, but needs new code (cache, LLM, or embeddings).
- ❌ **Not feasible** without a web-scale index of our own. Reject with HTTP 400 and `tag: "FEATURE_DISABLED"`.

---

## 3. Gap matrix: `POST /search`

### Request

| Exa field | Helix today | Fidelity | Plan |
|---|---|---|---|
| `query` | `q` | ✅ | |
| `type: auto` / `lite` | Plain SearXNG query | ✅ | `lite` means no fetching. That is already true today. |
| `type: deep` / `deep-lite` | — | 🔧 | LLM query expansion into N queries. `searxng.Search` already fans out over several queries. Then fetch, rerank, and optionally synthesise. |
| `type` other or legacy (`instant`, `fast`, `deep-reasoning`) | — | 🟡 | Map `instant`/`fast` to `auto` and `deep-reasoning` to `deep`. Exa marks this enum as open. |
| `additionalQueries` (1–10) | Multi-query exists internally | ✅ | Pass through to `Queries[]`. Accept only for deep types, as Exa does. |
| `numResults` 1–100 | Single page, about 10–30 merged | 🟡 | Request `pageno=1..k` until we have N unique URLs. Engine coverage limits real recall. |
| `category: news` | SearXNG `news` category | ✅ | |
| `category: company` / `people` / `personal site` / `publication` / `financial report` | — | 🟡 | Query rewrite plus domain hints, e.g. `people` adds `linkedin.com`, `financial report` adds `sec.gov` and IR pages. Also enforce Exa's 400s: `company` and `people` reject date filters, and `people` rejects `excludeDomains`. |
| `includeDomains` / `excludeDomains` (≤1200, `host`, `host/path`, `*.host`) | — | 🟡 | Up to about 10 domains: send `site:` / `-site:` operators to engines that support them. Above that: post-filter only, which reduces recall. Path and wildcard matching happen in the post-filter. |
| `startPublishedDate` / `endPublishedDate` | — | 🟡 | Coarse SearXNG `time_range` (day/week/month/year) plus a post-filter on `publishedDate`. A result with no date is kept and flagged in a warning. Exa estimates dates for every result; we can't. |
| `startCrawlDate` / `endCrawlDate` | — | ✅ | Ignored by Exa as well. Emit an `IGNORED_FIELD` warning. |
| `includeText` / `excludeText` (deprecated) | — | 🔧 | Post-filter on fetched text. This forces a fetch. |
| `userLocation` (country or coordinates) | `language` only | 🟡 | Map country to the SearXNG `language`/locale (e.g. `en-US`). Coordinates are ignored, with a warning. |
| `moderation` | `safesearch=0` hardcoded | 🟡 | Map to `safesearch=2`. Exa uses a classifier; ours depends on the engines. |
| `contents` (inline) | — | 🔧 | Run the `/contents` pipeline on the top N results in parallel. See §4. |
| `outputSchema` (`text` / `object`, ≤10 props, ≤2 levels) | — | 🔧 | LLM synthesis over fetched contents, with JSON-schema-constrained output and `grounding[]` per field. Validate Exa's schema limits before calling the LLM. |
| `objective`, `systemPrompt` | — | 🔧 | Prompt inputs to deep, synthesis, and highlights. |
| `stream` (only with `outputSchema`) | — | 🔧 | SSE event types `text-delta`, `grounding`, `results`, `done`, `error`, and `stream-reset`; the stream ends with `data: [DONE]`. |
| `compliance: hipaa` | — | ❌ | A property of the deployment, not of a request. Reject. |
| `context` (deprecated) | — | ✅ | Concatenated text of the results. Cheap to support. |
| `userIdentifier` | — | ✅ | Store it on the usage record. |

### Response

| Exa field | Today | Plan |
|---|---|---|
| `requestId` | — | ULID. Also send the `x-request-id` header. |
| `results[].url`, `title` | ✅ | |
| `results[].id` | — | Content-cache key, `doc_<sha256(normalised url)>`, which `/contents` can resolve. Exa calls its IDs temporary, so we're free to choose the format. |
| `results[].publishedDate` | Sometimes, from SearXNG | Fill from page metadata (`article:published_time`, JSON-LD) when the page has been fetched. |
| `results[].author` | — | From the readability byline or JSON-LD. Fetch only. |
| `results[].image` | SearXNG `thumbnail` / `img_src` | Otherwise `og:image` after fetch. |
| `results[].favicon` | — | From `<link rel=icon>` after fetch. Never call a third-party favicon service: it leaks queries and breaks air-gapped installs. |
| `results[].text` / `highlights` / `summary` / `subpages` / `extras` | — | §4. |
| `results[].entities[]` | — | ❌. This needs Exa's entity graph. Omit the field. |
| `score` | SearXNG score | Exa's search results don't include it. Only `findSimilar` does. Leave it off `/search` to match Exa. |
| `costDollars` | — | Internal chargeback from a price table in config, with the same breakdown keys. Zero if no price table is configured. |
| `searchTime`, `warnings[]`, `notices[]`, `output` | — | Build. `warnings` is where each 🟡 downgrade is reported. |
| `resolvedSearchType` (deprecated) | — | The type we actually ran. |

---

## 4. Gap matrix: `POST /contents` (and `contents` inside `/search`)

| Exa field | Helix today | Fidelity | Plan |
|---|---|---|---|
| `urls` (1–100) | rod fetch, one at a time | ✅ | Bounded parallelism over the page pool. A plain HTTP GET first; Chrome only when the page needs JS (heuristic: empty readability output, or `<noscript>`-heavy pages). |
| `ids` | — | 🔧 | Resolve through the content cache. Unknown ID → `statuses[].status = error`. |
| PDFs and documents | `pkg/extract` exists | ✅ | Route by `Content-Type`. |
| `text: true` | readability + markdown | ✅ | |
| `text.maxCharacters` (default 5,000,000 on `/contents`) | — | ✅ | Truncate. |
| `text.includeHtmlTags` | Raw HTML available | ✅ | Return the readability HTML instead of Markdown. |
| `text.linkStyle` `full` / `citation` | `full` only | 🔧 | A converter rule for `citation` that appends a `[n]: url` list. |
| `text.verbosity`, `includeSections` / `excludeSections` | Readability gives only the body | 🟡 | `compact` = readability body. `full` = whole-document Markdown. Sections come from `<header>`, `<nav>`, `<footer>`, `<aside>`, and ARIA roles, best-effort (Exa also says best-effort). |
| `highlights` (`query`, `maxCharacters`) | — | 🔧 | Chunk the text, embed it with `/v1/embeddings`, and rank by cosine similarity to `highlights.query` (or to the search query). Fall back to BM25 when no embedding model is configured. Errors if neither works. Fill `highlightScores` for findSimilar. |
| `highlights.dynamic` / `verbosity` (Exa beta) | — | ❌ for v1 | Reject unless the `Exa-Beta` header is set. Support later. |
| `summary` (`query`, `schema`) | `Browser` skill summarises with an LLM | 🔧 | Use a small, fast model from the org's providers. `schema` → JSON-mode / structured output, validated. |
| `extras.links` / `imageLinks` / `richImageLinks` (0–1000) | Not extracted | ✅ | Collect from the DOM during fetch. |
| `maxAgeHours` (−1 … 720) | No cache | 🔧 | **New content cache** (see §6): `-1` = cache only, `0` = always fetch, N = fetch again if the cached copy is older than N hours. |
| `livecrawl` (deprecated) | — | ✅ | Map to `maxAgeHours`. Reject the combination of both, as Exa does. |
| `livecrawlTimeout` (≤90000 ms, default 10000) | Fixed `pageTimeout` | ✅ | Per-request page timeout. |
| `snapshotAsOf` | — | ❌ | Needs historical snapshots. Reject until the cache keeps versions. |
| `subpages` (0–100), `subpageTarget` | colly crawler | 🔧 | A bounded same-host BFS, links ranked by `subpageTarget` keyword match. Reuse the link extraction from `extras`. |
| `statuses[]` `{id, status, source, error{tag, httpStatusCode}}` | — | 🔧 | `source` = `cached` / `crawled`. Our error tags: `CRAWL_NOT_FOUND`, `CRAWL_TIMEOUT`, `CRAWL_HTTP_ERROR`, `SOURCE_NOT_AVAILABLE`, `BLOCKED_DESTINATION`. Exa's spec leaves this tag set open, so clients must already handle unknown tags. Check our names against real Exa responses in the conformance suite (§9). |

---

## 5. `/answer` and `/findSimilar`

**`POST /answer`** — 🔧, built entirely from parts we already have:

1. Run `/search` (`type: auto`, roughly 8 results, `contents.highlights`).
2. Build an LLM prompt with numbered sources and call the model that `model` maps to:
   - `exa-fast` → small model
   - `exa` → default model
   - `exa-pro` / `exa-research` → strongest model, and `exa-research` also uses `deep` search
   The mapping is deployment config.
3. Return `answer`, which is a string, or an object when `outputSchema` (draft 7) is set. Also return `citations[]` (title, url, …, plus `text` when `text: true`) and `costDollars`.
4. `stream: true`: OpenAI chat-chunk-shaped `choices[].delta` events, then a `citations` event, then `costDollars`. Errors are sent as `{tag:"ERROR", payload:{error:{code,message}}}`. Exa doesn't document a `[DONE]` terminator for `/answer`; confirm against the live API before choosing ours.

**`POST /findSimilar`** — 🟡. Exa has deprecated it and recommends describing the source in a `/search` query. Implement it the same way:

1. Fetch `url`.
2. LLM writes a one-line description of the page.
3. Run `/search` with that description.
4. Apply `excludeSourceDomain`.
5. Return `score` from rerank similarity.

This is semantic similarity layered over keyword engines. It is not Exa's embedding neighbourhood, so quality will be noticeably lower.

---

## 6. Gaps that won't close (be explicit with customers)

1. **No neural index.** Exa embeds its own crawl of the web, so descriptive queries ("startups building X", "blog posts arguing Y") retrieve by meaning. SearXNG forwards keyword queries to Google, Bing and other engines. LLM query rewriting (`deep`) narrows the gap, but parity here isn't achievable without crawling and embedding a web-scale corpus. That is the scale question this doc sets aside.
2. **Recall and filters depend on the upstream engines.** Date, domain, and category filters are post-filters or operator hacks. Results after filtering can be far fewer than `numResults`.
3. **Upstream rate limits and ToS.** Scraped engines (Google especially) throttle or CAPTCHA SearXNG under load and forbid automated querying. This is a legal and operational risk for an internal customer product, independent of scale. Decide which engines are allowed (§10).
4. **No entities, no snapshots, no HIPAA mode.**
5. **Latency.** Inline `contents` means real Chrome fetches, typically 1–10 s per page. Exa usually answers from its cache. The content cache only helps with repeated URLs.

---

## 7. Interface design (the swap point)

### 7.1 External API: Exa wire format, Helix auth

- **Base path:** `/api/v1/websearch`, with routes `/search`, `/contents`, `/answer`, `/findSimilar`. Exa SDKs take a `base_url` and append these paths, so `exa-py` / `exa-js` work against Helix with only the URL and key changed: `Exa(api_key=HELIX_KEY, base_url="https://helix.example/api/v1/websearch")`.
- **Auth:** accept `x-api-key` (what Exa SDKs send) **and** `Authorization: Bearer`. Both resolve to a normal Helix API key, which gives the org, quota, and audit trail. The `x-api-key` header is accepted only on these routes.
- **Errors:** Exa's envelope `{requestId, error, tag}` with Exa's status codes (400/401/402/403/429/500/503). For example, quota exhausted → 402, and a feature we can't honour → 403 `FEATURE_DISABLED`. A recognised field we deliberately don't support → 400 with `tag: "FEATURE_DISABLED"` and the field path. **We never silently ignore a field that changes the meaning of the results.** Fields that are only hints, and fields Exa itself ignores, produce a `warnings[]` entry instead.
- **Headers:** `x-request-id` always.
- **Schema:** Go structs generated from, or hand-mirrored against, the pinned Exa OpenAPI version, plus swagger annotations so `./stack update_openapi` gives the frontend and CLI a client. One request/response type set is used by both providers.

### 7.1a Official SDK compatibility (verified against source, `exa-py` 2.28.0 / `exa-js` 2.28.0)

Both SDKs work against a custom host. Facts from reading the packaged code:

| Aspect | `exa-py` | `exa-js` | Consequence for Helix |
|---|---|---|---|
| Base URL | `Exa(api_key, base_url="https://api.exa.ai")` | `new Exa(apiKey, baseURL = "https://api.exa.ai")` | Supported. **No env var** (only `EXA_API_KEY` is read), so callers must change one constructor argument. Existing code can't be redirected by environment alone. |
| URL building | `self.base_url + endpoint` | `this.baseURL + endpoint` | A path prefix works: `https://helix/api/v1/websearch` + `/search`. A trailing slash in the base gives `//search`, so the router must tolerate it or the docs must say "no trailing slash". |
| Auth header | `x-api-key` only | `x-api-key` only | Helix **must** accept `x-api-key` on these routes. |
| Other namespaces | `/websets/v0/...`, `/research/v1`, `/monitors`, `/agent/runs` | the same, plus `/websets` prefix | All relative to the base URL, so they can live under the same prefix later. Until then they return Exa-envelope 404 / `FEATURE_DISABLED`. |
| Client-side validation | `validate_search_options` rejects unknown option names | — | Not our concern. The SDK only sends fields it knows about. |
| Response parsing | Lenient `.get()`; only `results` is a required key | Lenient | Optional fields we can't fill (`entities`, `author`, …) can be omitted safely. |
| Errors | Any status ≥400 raises `ValueError(status, body)` | Parses `error` as a string, or as `error.message` | Exa's `{requestId, error, tag}` envelope satisfies both. |
| SSE | Strips `data: `, skips lines that aren't JSON | Skips `[DONE]` explicitly | Either terminator is safe. |

So once phases 1–3 are done, `search`, `search_and_contents`, `get_contents`, `find_similar`, `answer`, `stream_answer` and `stream_search` are fully usable, with only `base_url` and the key changed. `agent`, `research`, `monitors` and `websets` are reachable but unimplemented until §10 phase 4 or later.

**Not verified:** third-party wrappers (Exa MCP server, LangChain `ExaSearchRetriever`, LlamaIndex reader, Vercel AI SDK tool). Check each one for a base-URL option before promising it to customers. A wrapper that hardcodes `api.exa.ai` needs an upstream patch or a fork.

### 7.2 Internal interface

```go
// package websearch — the only thing handlers, agent skills and the CLI depend on.
type Provider interface {
    Search(ctx context.Context, req *SearchRequest) (*SearchResponse, error)
    Contents(ctx context.Context, req *ContentsRequest) (*ContentsResponse, error)
    Answer(ctx context.Context, req *AnswerRequest, stream AnswerSink) (*AnswerResponse, error)
    FindSimilar(ctx context.Context, req *FindSimilarRequest) (*FindSimilarResponse, error)
    // Capabilities reports which fields/values this provider honours natively,
    // approximately, or not at all. The handler uses it to produce
    // FEATURE_DISABLED errors and warnings[] identically for every provider.
    Capabilities() Capabilities
}
```

Two implementations:

- **`websearch/exa`**: a thin HTTP client. It forwards requests unchanged and maps errors 1:1. It is the reference implementation that defines parity, and the escape hatch for deployments that would rather pay Exa.
- **`websearch/native`**: composed of replaceable parts, so we can later swap SearXNG for another meta-search or Chrome for a hosted browser without touching the API:

```
native.Provider
 ├─ Discoverer   (SearXNG)              query → candidate URLs + snippets
 ├─ Fetcher      (HTTP → Chrome → extract)  url → raw doc (html/pdf), status, timings
 ├─ Extractor    (readability, md, DOM)  raw doc → text/sections/links/metadata
 ├─ Ranker       (embeddings | BM25)    highlights, rerank, findSimilar score
 ├─ Synthesiser  (Helix LLM)            summary, outputSchema, answer, deep query expansion
 └─ ContentCache (Postgres + filestore) url → versioned doc, fetched_at, ids
```

`Answer` and `FindSimilar` in `native` are built only from the other parts. The `exa` provider forwards them.

**Provider selection** is server config (`WEBSEARCH_PROVIDER=native|exa`, `WEBSEARCH_EXA_API_KEY`), and later possibly per org. It is not a request parameter, so customers never see which provider served them.

### 7.3 Consolidate internal consumers

Today `searxng.SearchProvider` is used directly by `agent/skill/web_search.go`. The skill and the `Browser` skill should call `websearch.Provider` instead. Then switching a deployment to Exa also switches Helix agents, and there is one code path to secure and meter. Delete `searxng.SearchProvider` once nothing uses it. A `helix web search|contents|answer` CLI subcommand (and a helixml/skills entry) gives sandbox coding agents the same API.

---

## 8. Required fixes before this is exposed externally

1. **SSRF in the fetch path (blocker).** `browser.go`, `crawler/default.go`, and the `Browser` skill fetch arbitrary user URLs with no `netguard` check. A public `/contents` would let any API key read cloud metadata and internal services. Netguard's dial-time check protects Go HTTP clients but not Chrome, which resolves names itself. Fix: run Chrome's traffic through `--proxy-server` pointing at a CONNECT proxy **inside the API process**, which enforces `netguard.Policy` at dial time. Plain-HTTP fetches use netguard's transport directly. Redirects are re-checked per hop by the same dialer. (No new service: the API already owns this trust boundary.) WebRTC can send UDP around the proxy, so managed (`rod-manager`) Chrome is launched with `--force-webrtc-ip-handling-policy=disable_non_proxied_udp`; a Chrome reached directly through `RAG_CRAWLER_CHROME_URL` (launcher disabled) must be started with that flag by the operator.
2. **`searxng.Search` swallows errors.** When every query fails it logs and returns empty results. It must return the error so we can send a 5xx and a `statuses` / `warnings` entry.
3. **Hardcoded `engines` and `safesearch=0`.** Move them to config, and choose them per request (moderation).
4. **`searxng/searxng:latest`** is unpinned. Pin a version, because the JSON result shape is what we parse.
5. **Fetch isolation.** One page pool is shared by RAG crawling, agent browsing, and the API. Give the API its own bounded pool and per-org concurrency limits, so one customer's 100-URL `/contents` can't starve agents.

---

## 9. Verifying parity

- **Schema conformance:** validate every Helix response against the pinned Exa OpenAPI spec (kin-openapi) in Go tests. Fail on extra or missing required fields.
- **SDK smoke tests:** run `exa-py` and `exa-js` against a local Helix with `base_url` overridden: search, contents, answer (with streaming), and the error paths. Pin the SDK versions in the test and bump them deliberately, because newer SDKs add fields and namespaces.
- **Differential suite:** a fixed set of about 50 queries and URLs, sent to both providers, scoring:
  - URL overlap@10
  - share of results with `publishedDate` / `author`
  - text extraction similarity on the same URLs
  - `statuses` agreement
  - p50/p95 latency
  This is the parity dashboard, and the regression gate when switching engines. It runs against a real Exa key on demand, not in every CI run.
- **Feature matrix test:** for every field in §3 and §4, assert that the native provider either honours it, adds the documented warning, or returns `FEATURE_DISABLED`. This keeps the doc and the code from drifting apart.

---

## 10. Phasing

| Phase | Scope | Exit criterion |
|---|---|---|
| **0** | §8 fixes; `websearch` package and types; `exa` provider; skill refactor onto `Provider` | Agents work unchanged on `native`; with `WEBSEARCH_PROVIDER=exa` they work through Exa. |
| **1** | `/search` (no inline contents), `/contents` (text, extras, statuses, PDFs), content cache with `maxAgeHours`, `x-api-key` auth, usage records, error envelope | `exa-py` `search()` and `get_contents()` work against Helix; schema conformance passes. |
| **2** | Inline `contents`, highlights (embeddings/BM25), summary (+ schema), subpages, domain/date/category approximations, `numResults` pagination | Differential suite baseline recorded. |
| **3** | `/answer` + SSE, `/search` `outputSchema` synthesis + stream, `deep` / `additionalQueries`, `findSimilar` | SDK `answer(stream=True)` works. |
| **4 (v2)** | `/agent/runs` on the Helix agent loop; cache versioning → `snapshotAsOf` | — |

---

## 11. Open questions

1. **Engine policy.** Do we keep Google/Bing scraping through SearXNG for a customer-facing product, or move to licensed engine APIs (Brave Search API, Bing replacement) as SearXNG engines? This affects legality more than code.
2. **Content cache storage.** A Postgres row per URL (metadata, `fetched_at`) plus the body in filestore, or the body in Postgres? How long do we keep it, and should cached content be shared across orgs? Sharing is efficient but leaks fetch timing between tenants.
3. **Chargeback.** Do we populate `costDollars` from an internal price table, or always send zero and rely on Helix usage records?
4. **Model mapping** for `/answer` `model` values. Is it per deployment, or does it come from the org's default provider?
5. **Scope of `x-api-key`.** Accept it only on `/api/v1/websearch/*` (proposed), or treat it as a general Helix auth header?
