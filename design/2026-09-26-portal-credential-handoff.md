# Portal credential handoff for customer-facing bots

**Date:** 2026-09-26
**Status:** mock handoff implemented on this branch; real portal adapter,
verified custom domain, and Artifact preview integration remain future work.

The mock implementation is gated by `HELIX_PORTAL_MOCK_ENABLED=1` and a
non-empty `HELIX_ENCRYPTION_KEY`. It accepts only the published demo
username/password/OTP. Its trusted form is a Go HTML template in
`api/pkg/server/templates/portal_connection.html`, served outside the Helix
SPA so the page does not load chat or analytics JavaScript. See
`docs/portal-connections.md` for the actual API contract.

## Goal

Let a support conversation connect to a legacy portal that requires a username,
password, and then an OTP. The customer receives a short-lived branded link and
enters both factors on a Helix-hosted page. Passwords, OTPs, pre-auth state,
portal cookies, and any reusable portal session remain outside chat messages,
task attachments, interaction records, bot sandboxes, and model requests.

This is a credential-handling integration: the Helix backend receives the
customer's portal password. The page must say so clearly. A branded Helix page
must not claim to be the portal's own login page.

## Placement in Helix

Use existing components rather than a new service:

- The Helix API owns connection attempts, authorization, audit events,
  one-time invitations, the portal adapter contract, and encrypted session
  state.
- The Helix frontend owns a trusted `/connect/:invitation` form. It is a
  separate flow from the chat composer and never posts to `/sessions/chat`.
  A white-label hostname routes to this same trusted page and API origin,
  with verified domain ownership and TLS. It is never an artifact hostname.
- If a portal needs a real browser, run a deterministic adapter in a
  dedicated Hydra-managed browser sandbox with no coding agent, model client,
  Helix user API key, or filesystem shared with a customer task. The API owns
  that sandbox's lifecycle. For a portal with a stable HTTP login API, the
  adapter can run in the API without a browser.
- A customer-facing bot receives only task-scoped connection status and
  narrow portal operations. The server derives the customer and connection
  from trusted gateway/task context, never from a model-supplied account ID.

The existing support-bot proposal in
`design/2026-09-24-support-bot-poc-plan.md` lets an agent use a logged-in
Chrome browser and `get_secret`. That is incompatible with the stronger
claim that portal credentials and reusable sessions are inaccessible to a
model-controlled runtime. The existing browser plan can remain a lower-trust
PoC, but it must not be described as this credential-isolated flow. The
lightweight-task design in `design/2026-09-24-lightweight-tasks.md` supplies
the customer-case boundary; its project-bound gateway key must be unable to
read connection secrets or call generic connector administration APIs.

## Customer flow

1. A trusted gateway creates a `portal_connection_attempt` for its canonical
   customer and conversation, naming an operator-configured portal. The bot
   may request a connection but cannot choose the portal URL or mint the link.
2. The gateway presents a **Connect portal** action with a random, short-lived
   invitation URL. It never sends the invitation token to the model as prompt
   text. The invitation grants only access to this attempt's form.
3. On opening the URL, the server exchanges the invitation for a short-lived,
   HttpOnly, Secure, SameSite connection-flow cookie and redirects to a URL
   without the token. A GET does not consume the invitation, so link previews
   cannot invalidate it. State-changing requests require CSRF protection.
4. The customer enters username and password. The trusted form posts directly
   to the connection API over HTTPS. The portal adapter submits them to the
   configured portal and reports either an error or `otp_required`.
5. The same form collects the OTP and posts it directly to the API. The
   adapter completes the portal challenge. Neither factor enters Helix chat,
   artifact content, bot tools, or model-visible logs.
6. On success, the API records `connected` and stores only the portal session
   material needed for later use. The chat receives a status event with a
   connection ID, portal name, and expiry, not a credential or cookie.
7. The bot calls only bounded portal operations. The adapter performs those
   operations with the session and returns the minimum task result. On expiry
   or revocation, the user reconnects.

The attempt state machine is `created -> password_pending -> otp_pending ->
connected`, with terminal `failed`, `expired`, and `revoked` states. Password
and OTP submission have separate rate limits and attempt caps. Concurrent
submissions and completion are idempotent under a database lock. An invitation
cannot be rebound to another conversation or customer.

## API shape

The route names are illustrative; the authorization boundaries are the
contract.

| Caller | Operation | Returned data |
|---|---|---|
| Authorized gateway | Create attempt for its own project/customer/conversation | Invitation URL, attempt ID, expiry |
| Connection-flow browser cookie | Submit password; submit OTP; read this attempt's status | Next form state or safe error only |
| Authorized gateway | Read status or revoke its own attempt | Status, timestamps, portal identity |
| Bound customer task | Invoke an allow-listed portal operation | Sanitized operation result only |

No endpoint returns a password, OTP, pre-auth cookie, portal cookie, or raw
browser state. The task key cannot create invitations, access another task's
connection, invoke arbitrary URLs, execute JavaScript, or export cookies.
Logs and traces record an attempt ID and state transition, never request
bodies or portal responses containing secrets. Error strings are allow-listed
rather than copied from the portal.

The portal-specific adapter defines `beginLogin`, `submitOtp`, `perform`,
`sessionExpiresAt`, and `logout`. It uses operator-configured origins and
operations. It must not accept model-provided login URLs or selectors. Where
portal behavior requires an LLM to interpret pages, this strong isolation
contract cannot be maintained without a constrained intermediary that
sanitizes observations and controls actions; arbitrary browser access is
outside this design.

## Artifacts and editable branding

Helix Artifacts are useful for previews, versioning, and publishing design
assets. `design/2026-08-20-project-artifacts.md` explicitly scopes them to
static, agent-authored content. The current artifact policy in
`api/pkg/server/artifact_handlers.go` allows scripts and external HTTPS
connections. Therefore an ordinary HTML or SPA Artifact must not own the
credential fields or receive password/OTP values.

Use one trusted connection-page component. Its form, submit destinations,
portal identity, security copy, and flow state come from Helix code and
operator configuration. Branding is a validated data manifest held in that
configuration: logo image, colors, typography choices, and a small set of
safe text slots. At approval time, Helix ingests the allowed image bytes and
theme values into the portal configuration; the live form does not load an
artifact's HTML or JavaScript. No artifact JavaScript, HTML event handlers,
third-party fonts, or arbitrary CSS run in
the credential form's origin. The page uses a strict CSP and no third-party
analytics. A revision to the theme cannot silently change where secrets go.

Helix Code and spec tasks can edit theme source in a project repository,
publish a preview Artifact, and request review. An authorized operator pins
the approved theme values to the portal configuration. The live page changes
on approval, not on every artifact update. Trusted form behavior and the adapter
are code changes that go through the normal reviewed build and deployment
path. The customer-facing untrusted bot cannot publish or approve a live
credential page.

## Storage and lifecycle

- Do not persist passwords or OTPs after the attempt. Keep only the minimum
  transient challenge state required between steps, with a short expiry.
  Some portals need the password again when submitting OTP; such adapters
  need a short-lived encrypted transient value, then must delete it on
  completion, failure, or expiry.
- Encrypt portal session material at rest with a key outside the database;
  scope it by organization, portal, and canonical customer. Keep it out of
  task workspaces and Helix Secrets APIs exposed to agents. Set an explicit
  expiry and support immediate revocation and portal logout where available.
- Apply `Cache-Control: no-store`, `Referrer-Policy: no-referrer`, strict CSP,
  framing restrictions, and no credential-bearing URL parameters to the
  connection flow. Prevent body capture in reverse-proxy logs, APM, error
  reporting, and browser analytics.
- The gateway must authenticate its end customer and provide a stable
  customer ID. A bearer invitation alone proves possession of the link,
  not the customer's identity; if the gateway cannot authenticate, the
  product must explicitly choose an additional verification step.

## First implementation slice

1. One configured mock portal with password followed by OTP, plus a bounded
   `get_account_status` operation. No generic browser scripting API.
2. Helix API attempt state, invitation exchange, flow endpoints, status,
   expiry, revocation, and audit events. Use the existing API process and
   store rather than introducing a new service.
3. Trusted connection page in the Helix frontend. Start with a built-in
   theme. Add validated theme configuration and an Artifact-based preview
   workflow after the secret path works end to end.
4. Project/task-scoped gateway and tool authorization. Test that attempts,
   customers, tasks, and portal sessions cannot cross boundaries.
5. Verify the complete password -> OTP -> portal operation flow while
   inspecting chat payloads, interaction records, sandbox environment,
   model requests, browser traces, logs, and telemetry for both factors and
   session material. Include expiry, duplicate use, wrong customer, wrong
   project, failed OTP, revocation, and artifact update cases.

Before a real portal adapter is built, specify its login behavior, required
post-login operations, session lifetime, and whether its terms permit this
kind of automation. Those details determine whether the adapter can run in
the API or needs a dedicated Hydra-managed browser.

## Generic intake extension in this PR

The follow-up prototype adds a generic, project-scoped secret intake alongside the mock portal. MCP tools create an invitation and read status; neither returns secret values. The project API can create, inspect, revoke, and accept write-only direct submissions from a trusted third-party backend. A Helix backend connector can consume encrypted values via `ConsumeSecretIntake` and must convert them to a bounded operation or session outside model-visible output. No real portal connector is included.

The optional Artifact integration is narrower than the original preview idea above: a same-project, single-file HTML Artifact is sanitized to static text and a single `data-helix-form` slot at invitation creation. Helix inserts its own form in that slot. Artifact scripts, styles, attributes, controls, and external resources are discarded. This lets a spec task change approved copy for new invitations without granting artifact code access to credentials. It does not provide arbitrary Artifact-hosted forms or custom JavaScript.

Values are encrypted at rest, expire after one hour, and are cleared by a reaper, consumption, or revocation. The generic intake does not authenticate the end customer; the gateway still owns that binding. A future portal adapter should consume the values and discard them after establishing a server-held portal session.
