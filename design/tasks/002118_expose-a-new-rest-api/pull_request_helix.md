# feat(api): paginated JSON:API endpoint for stream messages

## Summary

Adds `GET /api/v1/orgs/{org}/streams/{id}/messages` to the helix-org REST surface:
a [jsonapi.org](https://jsonapi.org)-formatted, paginated list of the messages in
a stream, newest first. The top-level `meta` carries the total item count and the
page state; `links` carry page-based navigation (self/first/prev/next/last).

The JSON:API plumbing is built as a small, **composition-first** toolkit in a new
`api/pkg/org/interfaces/jsonapi` package (the codebase had no JSON:API helpers).
A document is assembled by applying independent components, so meta and pagination
are separate, reusable units rather than fields baked into one bespoke response:

```go
doc := jsonapi.NewDocument(resources,
    jsonapi.TotalMeta{Total: total},                                   // meta via composition
    jsonapi.Pagination{Number: page.Number, Size: page.Size, Total: total, Query: r.URL.Query()}, // pagination via composition
)
jsonapi.Write(w, http.StatusOK, doc)
```

## Changes

- **New `jsonapi` package** (`api/pkg/org/interfaces/jsonapi`): `Document`/`Meta`/
  `Links`, a `Component` interface + `NewDocument(data, components...)`, the
  `TotalMeta` and `Pagination` components, a `Page`/`PageParams` query parser
  (`page[number]`/`page[size]`), and `Write` (sets `application/vnd.api+json`).
- **Endpoint**: `listStreamMessages` handler + `GET /streams/{id}/messages` route,
  with `MessageResource`/`MessageAttributes`/`MessagesDocument` DTOs and a
  `messageResource` mapper that decodes the canonical `streaming.Message`.
- **Store**: added `WithOffset` query option (+ apply), a generic
  `Repository.Count`, and `Events.PageForStream`/`Events.CountForStream` on both
  the gorm and memory backends. Facade gains `PageStreamEvents`/`CountStreamEvents`.
- **Generated**: regenerated `swagger.json`/`swagger.yaml` and the TS API client.
- **Tests**: jsonapi unit tests (composition, total, pagination links at
  first/middle/last/out-of-range, empty set, query preservation); store
  pagination/count tests; handler tests (first page, partial last page,
  beyond-last, empty stream, unknown stream 404, bad paging 400, total across pages).

## Design notes

- **Page-number pagination** (not cursor): supports `meta.total`/`total_pages`
  and maps cleanly to offset/limit.
- **Query-only relative links** (e.g. `?page[number]=2&page[size]=50`): per
  RFC 3986 these resolve against the request URL, so links are correct whether
  the org API runs standalone or embedded behind the prefix-stripping middleware,
  without the org package depending on the server package.

## Verification

`go build ./...` clean; full `pkg/org/...` suite green. Verified live in the inner
Helix: published 5 messages and paged through `/messages` — newest-first data,
`meta.total=5`/`total_pages=3`, correct boundary links, 404 on unknown stream,
400 on `page[number]=0`.
