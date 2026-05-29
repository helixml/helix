# Admin Runner Logs

Date: 2026-05-29
Status: Proposal

## Motivation

When something goes wrong inside a Runner (outer helix-sandbox container or its inner ubuntu-external dev container), the path to "see what is happening" today is:

- For Runners that the operator owns (dev boxes, local infra): `docker logs <name>` on the host.
- For Runners hosted by a customer or partner (separate VPC, on-prem behind a firewall, third-party scheduler): there is no in-product path. Operators currently either ask for SSH access or wait for the task to finish and pull post-mortem logs from whatever object store the Runner was configured to upload to.

A recent end-to-end test against remote Runner infrastructure made the cost concrete. A spec task dispatched fine, the agent did its work, but live video streaming failed with `failed to change state to PLAYING`. Diagnosing the cause required pulling a post-mortem log file from object storage (4MB, ~3700 lines), grepping through it, and finding `[render-node] WARNING: Could not find nvidia driver for GPU_VENDOR=nvidia, falling back to software rendering` near the top. That information was there the whole time, but only visible after the run completed. A live log view in the admin UI would have surfaced it within seconds of the Runner starting.

This problem is not specific to any one deployment. Any time Helix runs in an environment where operators cannot SSH to the box, the same gap exists. The fix is reusable across every customer engagement.

## Goal

Add an admin-only endpoint that streams Runner logs to the operator's browser in near-real-time, via the existing RevDial control connection from the Runner back to the control plane. Surface it in the admin UI on the Agent Sandboxes panel that already lists Runners.

In scope for v1:

- One endpoint that streams the outer Runner container's stdout (which already includes hydra's prefixed stream of inner-container logs).
- One frontend tab inside the existing Agent Sandboxes admin panel that opens this stream for the selected Runner.
- Admin-only auth gating, matching the rest of the `/admin/runners/...` namespace.
- WebSocket transport, matching the existing streaming WS used for video.

Out of scope for v1:

- Selecting a specific inner container (e.g. only `ubuntu-external-<id>`). The outer Runner stdout already contains everything, prefix-tagged. Per-container filtering is a follow-up.
- Historical log retention beyond what hydra naturally buffers in its current process lifetime.
- Searching, regex filtering, downloading. Future.
- Aggregated multi-Runner views.

## Why this is small

There is strong precedent for every layer of the stack.

### Admin URL namespace already exists

`server.go:1176-1184` registers a family of `/admin/runners/{runner_id}/...` endpoints:

```
/admin/runner-profiles                                  GET POST
/admin/runner-profiles/{id}                             GET PUT DELETE
/admin/runners/{runner_id}/compatible-profiles          GET
/admin/runners/{runner_id}/assignment                   GET
/admin/runners/{runner_id}/assign-profile               POST
/admin/runners/{runner_id}/clear-profile                POST
```

Adding `/admin/runners/{runner_id}/logs` (WS upgrade) slots straight in. Auth and admin gating already wired by the admin router.

### Runner-via-RevDial exec helper already exists

`claude_subscription_handlers.go:672` defines `execInContainer(ctx, runnerID, command)` which dials a runner over RevDial, POSTs to `http://localhost:9876/exec` on the runner side, and returns the result. That endpoint is implemented in hydra. Today it is request/response, not streaming.

We do not extend `execInContainer` directly. Instead we follow the same pattern: a new hydra endpoint that streams.

### Streaming via RevDial already works

`session_expose_handlers.go` proxies HTTP requests and WebSocket upgrades through the RevDial connection to hydra. The streaming WS for desktop video (`external_agent_handlers.go:1424` `Proxying stream WebSocket to screenshot-server via RevDial`) is the closest precedent: it upgrades the client connection to WS, dials hydra, opens a WS back, and pumps bytes both directions with reconnect support (`pkg/proxy/resilient.go`).

The admin logs endpoint can lift this pattern verbatim. The only differences are which RevDial endpoint we point at, and the auth check (admin instead of session-owner).

### Hydra already streams container logs to its own stdout

Post-mortem log files from recent test runs show hydra prefixing inner-container logs with `[HYDRA] [DESKTOP ubuntu-external-<id>] ...` and emitting them on its own stdout. That stream is already running for every active Runner. The new hydra endpoint just needs to fan a WebSocket subscription onto that existing stream.

## Design

### Endpoint

```
GET /api/v1/admin/runners/{runner_id}/logs
Upgrade: websocket
```

Query parameters (v1):

- `tail` (int, default 200): number of trailing lines to send on connect, before live tail starts.
- `follow` (bool, default true): stay connected and emit new lines. If false, send tail then close.

Response: WebSocket sending one log line per message, encoded as JSON:

```json
{"t":"2026-05-29T10:34:01.863Z","stream":"stdout","line":"[HYDRA] [DESKTOP ubuntu-external-...] ..."}
```

`stream` is `stdout` or `stderr`. Hydra may not distinguish today; if not, emit `stdout` always for v1.

On error (runner not connected, RevDial failure), close with a clean WebSocket close code and a status frame:

```json
{"error":"runner not connected"}
```

### Auth

The handler validates the requesting token has admin scope. The existing admin router middleware enforces this for `/admin/...` routes. No new auth code.

### Server-side handler

New file `api/pkg/server/admin_runner_logs.go`:

```
func (s *HelixAPIServer) streamRunnerLogs(w http.ResponseWriter, r *http.Request) {
    runnerID := mux.Vars(r)["runner_id"]

    // 1. Validate runner is registered + admin can see it.
    // 2. Upgrade the incoming connection to a WebSocket.
    // 3. Dial the runner over RevDial.
    // 4. Send a control message to hydra: GET /logs?tail=200&follow=true.
    // 5. Pump bytes from RevDial WS -> client WS, line-by-line.
    // 6. On disconnect, close both.
}
```

Reuse the existing `pkg/proxy/resilient.go` helpers if they fit; otherwise model after `external_agent_handlers.go:1424`.

### Runner-side endpoint (hydra)

New endpoint in hydra: `GET /logs?tail=N&follow=true` on `http://localhost:9876`.

Hydra already maintains its own log buffer (the stream we see in `taskoutput.txt`). The endpoint:

1. Upgrades to WebSocket.
2. Emits the trailing N lines.
3. Subscribes to the live tail and emits each new line as a WS message.
4. Closes when the client disconnects.

Implementation note: hydra's log capture is already running. The endpoint is a fan-out, not a fresh `docker logs -f` invocation. This matters because spawning per-subscriber `docker logs` would leak processes if a connection dies uncleanly.

### Frontend

In `frontend/src/components/admin/AgentSandboxes.tsx`, add a "Logs" tab next to the existing per-sandbox controls. When opened:

1. Open WS to `wss://{control-plane-host}/api/v1/admin/runners/{runner_id}/logs?tail=200&follow=true`.
2. Render lines in a virtualized, scroll-locked terminal-style view. Reuse the dependency already in the repo for any existing log display (check Kodit Repositories tab, settings logs, etc. for an existing component).
3. Show a connection status indicator at the top: connected / reconnecting / disconnected.
4. Provide a "pause" toggle that stops auto-scrolling so the operator can read.
5. Provide a "download last 5000 lines" button that fetches a snapshot HTTP endpoint (out of scope v1; future).

UX detail: the same panel already shows the half-baked Inference Profile section flagged in `Task #5 (Admin dashboard cleanup)`. This is a good moment to fix both together if the user wants to bundle.

## Implementation plan

Ordered by dependency:

1. **Hydra side: `/logs` WebSocket endpoint.** Smallest, most self-contained. Test directly against a running hydra inside the helix-sandbox container with `websocat`. Land first.
2. **Helix API: `/api/v1/admin/runners/{runner_id}/logs` handler.** Proxy WS via RevDial to hydra. Test from a curl-style WS client (`wscat`) once authenticated as admin.
3. **Frontend: admin panel Logs tab.** Build against the new endpoint. Includes connection status, pause, basic styling.
4. **End-to-end test.** Spin up a Runner (local or remote), trigger a spec task, watch logs flow in real time.
5. **Docs.** Brief mention in the admin UI section of the README.

Estimated effort: 1-2 days end-to-end for one engineer.

## Open questions

- **Per-container filtering.** Hydra's stdout includes prefix-tagged lines from every inner container. Operators often only want one container's logs. Should v1 support a `container=<name>` query parameter that drops lines not matching that prefix? Adds complexity to hydra. **Recommend deferring to v2** unless an operator hits this on day one.
- **Multiple concurrent subscribers per Runner.** If two admins open the logs tab simultaneously, hydra should fan out without re-reading any history. Verify the buffer model supports that before landing.
- **Auth model for "admin sees logs of any Runner."** Today, admin scope is a single boolean. If Helix grows org-scoped admin (which it has, per `[[feedback_org_membership_git_403]]` design doc), the logs endpoint should respect that — an org admin should only see Runners assigned to their org. Check `lookupOrg` pattern in `wallet_handlers.go` for the standard.
- **Log retention.** Hydra likely buffers a bounded amount (e.g. last 10k lines) in process memory. If an admin opens logs for a Runner that has been running for a week, history beyond that buffer is gone. Acceptable for v1; document the limit clearly.
- **Frontend dependency.** Is there an existing in-repo virtualized log viewer component? If yes, reuse. If not, this PR may need to add `react-window` or similar — that adds bundle size, worth flagging.
- **Drain on Runner disconnect.** If the Runner's RevDial connection drops mid-stream, what does the admin see? Recommend: brief reconnect attempt (5s), then close with a clear "runner disconnected" status. Do not silently hang.

## Followups (explicit non-goals captured for later)

- v2: per-container filter.
- v2: server-side log retention (a circular file buffer on the control plane, optional, off by default).
- v2: `/admin/runners/{runner_id}/logs/snapshot.txt` for downloading the last N lines as a static file (admins love this for ticket attachments).
- v2: search and grep within the live stream.
- v2: integration with admin notifications (e.g. "tail N lines on every error-level line").
