# Design: List Stream Messages REST API (JSON:API)

## Summary

Add `GET /api/v1/orgs/{org}/streams/{id}/messages` to the helix-org JSON REST
surface. It returns a paginated, [jsonapi.org](https://jsonapi.org)-formatted
document of the messages in a stream, with a `meta.total` count and pagination
`links`. The JSON:API plumbing is built as small **composable** components in a
new `api/pkg/org/interfaces/jsonapi` package — meta and pagination are separate
units that each contribute their slice to a shared document.

## Where things live (discovered)

| Concern | Location |
|---|---|
| REST route registration | `api/pkg/org/interfaces/server/api/api.go` → `Routes(deps)` |
| Stream handlers | `api/pkg/org/interfaces/server/api/streams.go` |
| Read facade (handlers go through this — never the store) | `api/pkg/org/application/queries/queries.go` |
| Events store interface | `api/pkg/org/domain/store/store.go` (`Events`) |
| Gorm impl | `api/pkg/org/infrastructure/persistence/gorm/event.go` |
| Memory impl | `api/pkg/org/infrastructure/persistence/memory/memorystore.go` |
| Generic repo + query options | `gorm/repository.go`, `domain/store/options.go` |
| Message envelope | `api/pkg/org/domain/streaming/{message,event}.go` |

**Architectural rule (from existing code):** the `server/api` adapter holds NO
`store.*` repository — handlers read through `queries.Queries`. New reads must be
added to the facade, which delegates one call each to a store repo.

Routes are registered as `net/http` `{method} {pattern}` pairs; patterns are
flat (the `/api/v1/orgs/{org}` prefix is stripped upstream by the helix-org
middleware, which also stashes the orgID resolved via `resolveOrgID(r)`).

## Endpoint

```
GET /api/v1/orgs/{org}/streams/{id}/messages?page[number]=1&page[size]=50
```

Response (`application/vnd.api+json`):

```json
{
  "data": [
    {
      "type": "messages",
      "id": "evt_abc",
      "attributes": {
        "stream_id": "stream_x",
        "source": "w-alice",
        "created_at": "2026-06-16T10:00:00Z",
        "from": "w-alice",
        "to": ["w-bob"],
        "subject": "hello",
        "body": "hi there"
      }
    }
  ],
  "meta": { "total": 1234, "page": 1, "size": 50, "total_pages": 25 },
  "links": {
    "self":  "/api/v1/orgs/acme/streams/stream_x/messages?page[number]=1&page[size]=50",
    "first": "/api/v1/orgs/acme/streams/stream_x/messages?page[number]=1&page[size]=50",
    "next":  "/api/v1/orgs/acme/streams/stream_x/messages?page[number]=2&page[size]=50",
    "last":  "/api/v1/orgs/acme/streams/stream_x/messages?page[number]=25&page[size]=50"
  }
}
```

## Composition design (the core ask)

New package `api/pkg/org/interfaces/jsonapi`. A document is built by composing
independent **components**, each of which contributes its part. This is the
"OOP / via composition" the request calls for: `Meta` and `Pagination` are
distinct objects, not fields baked into one struct.

```go
// Document is the top-level JSON:API document.
type Document struct {
    Data  any    `json:"data"`
    Meta  Meta   `json:"meta,omitempty"`
    Links Links  `json:"links,omitempty"`
}

type Meta  map[string]any
type Links map[string]string

// Component contributes its slice to a Document. Composition = apply many.
type Component interface { Apply(*Document) }

// NewDocument composes a data payload with zero or more components.
func NewDocument(data any, components ...Component) *Document
```

**Meta via composition** — a standalone total contributor:

```go
type TotalMeta struct{ Total int }
func (m TotalMeta) Apply(d *Document) { d.ensureMeta()["total"] = m.Total }
```

**Pagination via composition** — one object that contributes both the page
`links` and the page `meta`, and also computes the store offset/limit:

```go
type Pagination struct {
    BaseURL string // request path + non-page query, for building links
    Number  int    // 1-based current page
    Size    int
    Total   int
}
func (p Pagination) Apply(d *Document) { /* set links self/first/prev/next/last + meta page/size/total_pages */ }
func (p Pagination) Offset() int       { return (p.Number - 1) * p.Size }
func (p Pagination) Limit() int        { return p.Size }
```

Handler usage — meta and pagination composed together, independently:

```go
page  := jsonapi.PageParams(r, defaultSize, maxSize) // parses page[number]/page[size], 400 on bad input
total := a.deps.Queries.CountStreamEvents(ctx, orgID, streamID)
evs   := a.deps.Queries.PageStreamEvents(ctx, orgID, streamID, page.Limit(), page.Offset())
doc   := jsonapi.NewDocument(
    messageResources(evs),
    jsonapi.TotalMeta{Total: total},
    jsonapi.Pagination{BaseURL: r.URL.Path, Number: page.Number, Size: page.Size, Total: total},
)
jsonapi.Write(w, http.StatusOK, doc) // sets application/vnd.api+json
```

A `messages` resource is mapped from a `streaming.Event` by decoding its
`Message` (reuse the same decode the existing `eventCard`/`eventView` mappers
use) into a `MessageResource{Type, ID, Attributes}`.

## Store changes (pagination + total need new capability)

The `Events` interface today offers only `ListForStream(..., limit)` — no offset,
no count, and the generic repo has no `WithOffset` option or exported `Count`.
Add the minimum:

1. `domain/store/options.go`: add `WithOffset(n int)` (no-op when `n <= 0`);
   apply it in `gorm/repository.go`'s option application alongside `WithLimit`.
2. `store.Events` interface: add
   - `CountForStream(ctx, orgID, streamID) (int, error)`
   - `PageForStream(ctx, orgID, streamID, limit, offset int) ([]Event, error)`
   Implement both in the gorm repo (count via `Repository` count helper / a
   `SELECT count(*)`; page = `ListForStream` body + `WithOffset`) and the memory
   repo (slice the in-memory ordered list). Keep ordering identical to
   `ListForStream`: `created_at DESC, id DESC`.
3. `queries.Queries`: add thin pass-throughs `CountStreamEvents` and
   `PageStreamEvents` (one repo call each, matching the facade's existing style).

## Key decisions

- **Page-number pagination**, not cursor: simplest, supports `meta.total` and
  `total_pages` directly, and offset maps cleanly onto the store. Cursor paging
  can be added later without breaking the URL shape.
- **Resource `type` = `messages`** (the user-facing concept), `id` = event id.
  The domain stays "events"; only the wire vocabulary is "messages".
- **Newest-first** ordering, matching every other stream read in the codebase.
- **New `jsonapi` package under org/interfaces**, not buried in `server/api`, so
  it is reusable by any interface adapter and keeps the composition primitives
  separate from the stream handler.
- **`application/vnd.api+json`** content type per the JSON:API spec, via a
  dedicated `jsonapi.Write` (existing `writeJSON` stays `application/json`).
- **Swagger annotations** on the handler + `./stack update_openapi` so the
  generated OpenAPI/TS client stays valid (helix rule: always via generated
  client).

## Implementation Notes

- **Pagination links are query-only relative references** (e.g.
  `?page[number]=2&page[size]=50`), not absolute paths. The helix-org REST
  routes are registered with **flat patterns** and mounted two ways: standalone
  (no prefix) and embedded in helix behind `stripOrgScopedPrefix`
  (`api/pkg/server/helix_org_middleware.go`) which strips `/api/v1/orgs/{org}`
  before the handler runs. So the handler never sees its own mount prefix. A
  query-only relative reference resolves against the client's request URL
  (RFC 3986 §5), giving correct links in **both** deployments without the org
  package needing to import the server package or know `APIPrefix`. This also
  keeps the dependency edge one-way (org never imports server).
- **The gorm test DB is actually the memory store** (`gorm/testdb.go` →
  `memory.New()`), by deliberate project choice — so the store pagination tests
  live in the `gorm_test` package but exercise the shared interface contract
  (which both backends satisfy identically).
- Added a generic `Repository.Count` alongside the existing `Exists` for the
  gorm `CountForStream`.
- `Page` (request input, from `PageParams`) and `Pagination` (response
  component) are deliberately separate: the handler parses a `Page`, calls the
  store with `page.Offset()/page.Limit()`, then composes a `Pagination` from
  `page` + the total count.

## Testing

- `jsonapi` unit tests: `NewDocument` composition, `TotalMeta`, `Pagination`
  links at first/middle/last/out-of-range pages, empty set.
- Handler tests in `server/api` mirroring `streams_parity_test.go`: full page,
  partial last page, empty stream (`total:0`), unknown stream (`404`), bad paging
  params (`400`), and `meta.total` correctness across pages.
- Store tests for `CountForStream`/`PageForStream` in both gorm and memory impls
  (offset windows, ordering, count vs limited list).
