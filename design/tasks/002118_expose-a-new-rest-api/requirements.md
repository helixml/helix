# Requirements: List Stream Messages REST API (JSON:API)

## Background

A Stream in helix-org (`api/pkg/org/`) is a named source of events. Every
message published to a Stream is stored as a `streaming.Event` whose `Body`
carries a canonical `streaming.Message` JSON envelope (from/to/subject/body/…).

Today the only ways to read a Stream's messages over HTTP are:

- `GET /api/v1/orgs/{org}/streams/{id}/events` — an **SSE** firehose (live tail,
  newest 50, not paginated).
- The bundled `recent_events` inside `GET /streams` (capped at 50, no total).

There is no plain, paginated REST endpoint that lists the messages currently in
a Stream and tells the caller how many there are in total. This task adds one,
formatted per the [jsonapi.org](https://jsonapi.org) specification.

A grep of the codebase confirms **no JSON:API helpers exist anywhere** in helix,
so per the request the composable helpers are built in the org/interfaces
codebase (`api/pkg/org/interfaces/`).

## User Stories

### US-1: List the messages in a stream
As an API consumer, I want `GET` the messages in a stream so I can render or
process its history without holding a live SSE connection.

**Acceptance Criteria**
- `GET /api/v1/orgs/{org}/streams/{id}/messages` returns `200` with a JSON:API
  document whose `data` is an array of `messages` resources, newest first.
- Each resource has `type: "messages"`, `id` (the event id), and `attributes`
  carrying the decoded message (from, to, subject, body, …) plus `stream_id`,
  `source`, and `created_at`.
- Unknown stream id → `404`. Missing org scope → `400`.
- Response `Content-Type` is `application/vnd.api+json`.

### US-2: Total count in `meta`
As an API consumer, I want a `meta.total` field so I know how many messages the
stream holds regardless of the page I requested.

**Acceptance Criteria**
- The top-level `meta` object contains `total` = the full count of messages in
  the stream (not just the current page).
- `total` is correct when the result set spans multiple pages and when it is
  empty (`total: 0`, `data: []`).

### US-3: Pagination
As an API consumer, I want to page through a stream's messages so large streams
return in bounded responses.

**Acceptance Criteria**
- Query params `page[number]` (1-based, default 1) and `page[size]`
  (default 50, capped at 200) control the window.
- The top-level `links` object contains `self`, `first`, `last`, and `prev`/
  `next` (omitted at the boundaries) as fully-formed URLs preserving page size.
- `meta` also exposes the pagination state (`page`, `size`, `total_pages`).
- Out-of-range `page[number]` returns an empty `data` array (not an error);
  invalid/non-numeric paging params → `400`.

### US-4: Composable, reusable helpers
As a maintainer, I want the JSON:API building blocks to compose cleanly so meta
and pagination are independent, reusable components rather than bespoke per-
handler code.

**Acceptance Criteria**
- A JSON:API document is assembled by composing small components; the total-meta
  contributor and the pagination contributor are **separate** composable units.
- Adding the endpoint requires no changes to existing handlers; the helpers are
  usable by any future org/interfaces endpoint.

## Out of Scope
- Filtering/sorting beyond newest-first ordering.
- Cursor-based pagination (page-number pagination only for v1).
- Frontend UI work (beyond regenerating the OpenAPI client so it stays valid).
- Cross-resource `included`/relationships from the JSON:API spec.
