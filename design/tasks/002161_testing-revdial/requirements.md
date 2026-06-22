# Requirements: Add Dedicated RevDial Connectivity Ping Endpoint

## Background

RevDial is the reverse-dial NAT-traversal mechanism in Helix. Sandboxes and desktop containers connect out to the API server over a persistent WebSocket, and the API server "dials back" through those connections to reach services running inside each container.

Currently the only way to verify that a RevDial tunnel is alive is to call the screenshot endpoint (`GET /api/v1/external-agents/{session-id}/screenshot`), which requires GNOME/X11 to be running and is heavyweight for a simple connectivity check. A lightweight ping avoids this dependency.

## User Stories

**As a developer debugging a stuck session**, I want to call a cheap ping endpoint so that I can confirm the RevDial tunnel is alive without triggering a full screenshot capture.

**As an operator**, I want the `helix spectask` CLI to have a `ping` subcommand so that I can script RevDial health checks in test pipelines.

**As a developer writing integration tests**, I want a `/ping` route on the desktop HTTP server (port 9876) so that tests can verify RevDial routing without a full desktop environment.

## Acceptance Criteria

1. `GET /api/v1/external-agents/{session-id}/ping` returns `200 OK` with `{"status":"ok"}` when the RevDial tunnel is alive.
2. The same endpoint returns `503 Service Unavailable` (with an error message) when the container is not connected via RevDial.
3. `helix spectask ping <session-id>` prints success or failure and exits with an appropriate code.
4. The desktop HTTP server (port 9876) exposes `GET /ping` → `{"status":"ok"}`.
5. Round-trip latency of the ping is measurably lower than a screenshot (no image capture involved).
