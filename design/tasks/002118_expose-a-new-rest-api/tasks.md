# Implementation Tasks: List Stream Messages REST API (JSON:API)

## Store layer (pagination + total)
- [x] Add `WithOffset(n int)` to `api/pkg/org/domain/store/options.go` (no-op when `n <= 0`)
- [x] Apply `WithOffset` in `api/pkg/org/infrastructure/persistence/gorm/apply.go` option handling
- [x] Add `CountForStream` and `PageForStream(limit, offset)` to the `Events` interface in `domain/store/store.go`
- [x] Implement both in the gorm events repo (`gorm/event.go`) + generic `Repository.Count`
- [x] Implement both in the memory events repo (`memory/memorystore.go`)
- [x] Store tests (interface-level, runs against memory backing per test convention): offset windows, ordering, count

## Read facade
- [x] Add `CountStreamEvents` and `PageStreamEvents` pass-throughs to `application/queries/queries.go`

## JSON:API composition helpers (new package)
- [x] Create `api/pkg/org/interfaces/jsonapi/` package
- [x] `Document`, `Meta`, `Links`, `Component` interface, and `NewDocument(data, components...)`
- [x] `TotalMeta` component (contributes `meta.total`) — meta via composition
- [x] `Pagination` component (contributes page `links` + page `meta`) — pagination via composition
- [x] `Page` + `PageParams(r, defaultSize, maxSize)` parser for `page[number]`/`page[size]` (400 on invalid; `Offset()`/`Limit()` helpers)
- [x] `Write(w, status, doc)` setting `Content-Type: application/vnd.api+json`
- [x] Unit tests: composition, total, pagination links (first/middle/last/out-of-range), empty set, query preservation

## Endpoint
- [x] Add `MessageResource`/`MessageAttributes`/`MessagesDocument` DTOs + `messageResource` mapping in `server/api`
- [x] Implement `listStreamMessages` handler in `server/api/streams.go` (resolve org, 404 unknown stream, compose document)
- [x] Register `GET /streams/{id}/messages` in `server/api/api.go` `Routes()`
- [x] Add swagger annotations to the handler
- [x] Handler tests: first page, partial last page, beyond-last (empty), empty stream (`total:0`), unknown stream (404), bad paging (400), `meta.total` across pages

## Wiring & finalize
- [x] Run `./stack update_openapi` to regenerate OpenAPI + TS client (swagger + `frontend/src/api/api.ts` include `StreamsMessagesDetail` + `MessagesDocument`/`MessageResource`/`MessageAttributes`)
- [x] `cd api && CGO_ENABLED=0 go build ./...` and run new tests (all green; full org suite passes)
- [x] Manual check against the inner Helix: published 5 messages, paged through `/messages` — page 1 newest-first w/ `meta.total=5`, `total_pages=3`, links (no prev); last page partial w/ prev & no next; unknown stream → 404; `page[number]=0` → 400
