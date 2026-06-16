# Implementation Tasks: List Stream Messages REST API (JSON:API)

## Store layer (pagination + total)
- [~] Add `WithOffset(n int)` to `api/pkg/org/domain/store/options.go` (no-op when `n <= 0`)
- [~] Apply `WithOffset` in `api/pkg/org/infrastructure/persistence/gorm/apply.go` option handling
- [~] Add `CountForStream` and `PageForStream(limit, offset)` to the `Events` interface in `domain/store/store.go`
- [~] Implement both in the gorm events repo (`gorm/event.go`), ordering `created_at DESC, id DESC`
- [~] Implement both in the memory events repo (`memory/memorystore.go`)
- [~] Store tests (gorm + memory): offset windows, ordering, count vs limited list

## Read facade
- [ ] Add `CountStreamEvents` and `PageStreamEvents` pass-throughs to `application/queries/queries.go`

## JSON:API composition helpers (new package)
- [ ] Create `api/pkg/org/interfaces/jsonapi/` package
- [ ] `Document`, `Meta`, `Links`, `Component` interface, and `NewDocument(data, components...)`
- [ ] `TotalMeta` component (contributes `meta.total`) — meta via composition
- [ ] `Pagination` component (contributes page `links` + page `meta`; `Offset()`/`Limit()` helpers) — pagination via composition
- [ ] `PageParams(r, defaultSize, maxSize)` parser for `page[number]`/`page[size]` (400 on invalid)
- [ ] `Write(w, status, doc)` setting `Content-Type: application/vnd.api+json`
- [ ] Unit tests: composition, total, pagination links (first/middle/last/out-of-range), empty set

## Endpoint
- [ ] Add `MessageResource` mapping from `streaming.Event` (decode `Message`) in `server/api`
- [ ] Implement `listStreamMessages` handler in `server/api/streams.go` (resolve org, 404 unknown stream, compose document)
- [ ] Register `GET /streams/{id}/messages` in `server/api/api.go` `Routes()`
- [ ] Add swagger annotations to the handler
- [ ] Handler tests: full page, partial last page, empty (`total:0`), unknown stream (404), bad paging (400), `meta.total` across pages

## Wiring & finalize
- [ ] Run `./stack update_openapi` to regenerate OpenAPI + TS client
- [ ] `cd api && CGO_ENABLED=0 go build ./...` and run new tests
- [ ] Manual check against the inner Helix: publish messages to a stream, page through `/messages`, verify `meta.total` and `links`
