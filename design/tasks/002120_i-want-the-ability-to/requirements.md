# Requirements: Add Clear Session Capability for ACP and Internal Agent Backends

## Overview

Users need to "clear" a Helix session — wipe its conversation history so the
session starts fresh — while keeping the session itself (its ID, name, project,
ownership and config) intact. Clearing must behave correctly for **both** runtime
backends a session can use:

1. **ACP / Zed backend** — sessions driven over the external-agent WebSocket to a
   headless Zed editor (`CodeAgentRuntime = zed_agent`, `ExternalAgentConfig` set).
2. **Internal Go agent backend** — sessions driven by the in-process agent
   (`api/pkg/agent`), whose message history is seeded from the DB.

The clear operation should be exposed as a single method that "hangs off" the
Helix session, dispatching to the correct backend underneath — i.e. a small
compositional API rather than backend-specific code scattered across handlers.

## User Stories

### US-1: Clear an internal-agent session
**As a** Helix user chatting with the internal Go agent
**I want** to clear the conversation
**So that** I can start a fresh conversation in the same session without creating a new one.

**Acceptance Criteria:**
- [ ] Calling clear removes all interactions for the session from the database.
- [ ] The session record (ID, name, project, owner, model, metadata) is preserved.
- [ ] The next message starts from an empty history (the agent re-seeds an empty `MessageList`).
- [ ] Clearing an already-empty session is a no-op and returns success (idempotent).

### US-2: Clear an ACP / Zed session
**As a** Helix user working with a headless-Zed external agent
**I want** to clear the conversation
**So that** the Zed agent forgets prior context and starts a clean thread.

**Acceptance Criteria:**
- [ ] All interactions for the session are removed from the database.
- [ ] The Zed agent is told to start a fresh thread (the conversational context on the Zed side is reset, not just the Helix copy).
- [ ] Any in-flight streaming/turn for the session is stopped before clearing so no late tokens repopulate the cleared history.
- [ ] If no live WebSocket connection exists, the DB is still cleared and the next connection opens a fresh thread (no error surfaced to the user for the transient case).

### US-3: One method, correct backend
**As a** developer
**I want** `Clear` to be a single method on a session abstraction that picks the right backend
**So that** callers (HTTP handler, internal tooling) don't need to know whether a session is Zed-backed or internal-agent-backed.

**Acceptance Criteria:**
- [ ] A single entry point performs the shared DB clear and then delegates backend-specific reset.
- [ ] Adding a future backend means implementing one interface, not editing the handler.

### US-4: HTTP endpoint
**As a** frontend / API client
**I want** an endpoint to clear a session
**So that** the UI can offer a "Clear conversation" action.

**Acceptance Criteria:**
- [ ] `POST /api/v1/sessions/{id}/clear` clears the session.
- [ ] Authorization matches existing session-mutating endpoints (owner / org access).
- [ ] Returns the updated (now empty) session or a success status.
- [ ] Returns 404 for unknown sessions and 403 for unauthorized callers.

## Out of Scope
- Deleting the session entirely (already exists: `DELETE /api/v1/sessions/{id}`).
- Frontend UI button styling/placement (backend + API only; a follow-up can wire UI).
- Clearing/resetting session metadata such as title history, fork lineage, or pause state — only conversation interactions are cleared.

## Non-Functional
- Clearing must be **TDD'd**: store-level, backend-level, and handler-level tests written first.
- Clearing must be atomic at the DB level (one delete, not a per-interaction loop).
- Must not close the external-agent WebSocket connection (the agent stays available for the next message).
