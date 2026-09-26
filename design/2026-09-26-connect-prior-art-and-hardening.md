# Connect prior art and hardening review

**Scope:** design review of Helix's generic secret intake and proposed Meydan customer-portal connection. This is a plan for the next implementation slice, not a claim that the connector exists. Nango is a useful API-integration reference, but the free self-hosted edition has Auth and Proxy only; its documented product is for API connections, not the website-browser login required for Meydan. See [Nango's self-hosting feature table](https://nango.dev/docs/guides/platform/self-hosting/self-hosting).

## What Nango's design gets right

| Nango pattern | Helix application |
|---|---|
| A short-lived [connect link](https://nango.dev/docs/guides/auth/share-connect-link) is created by the backend with an allowed integration and end-user metadata. | Keep the short-lived link, but let a trusted Helix gateway choose the provider and derive the authenticated customer and conversation. Model-supplied strings are display/request data, not proof of identity. |
| Nango's [MCP connection tool](https://github.com/NangoHQ/nango/blob/master/packages/server/lib/controllers/agent/mcp/createConnection/createConnection.ts) accepts only integrations in that agent session's compiled toolset and refuses one already connected. | The Meydan request tool must select a reviewed connector registered for this bot and resolve the current customer on the server. Retain the generic intake tool for trusted workflows, but do not let it implicitly create an authenticated portal connection. |
| A [Connection](https://nango.dev/docs/guides/auth/auth-guide) persists after authorization and has an ID separate from the temporary connect session. | Introduce a durable, customer-bound `PortalConnection` and a per-attempt `ConnectionAttempt`. Link each secret intake and OTP challenge to one attempt. An intake is not itself the portal connection. |
| Nango [creates the connect session and its short-lived private key in one database transaction](https://github.com/NangoHQ/nango/blob/master/packages/server/lib/services/connectSession.service.ts). | Create attempt, intake, and invitation as one logical operation. If any step fails, leave no orphaned usable link or attempt. Require an explicit connector allowlist; a missing allowlist must not mean every provider is allowed. |
| Nango can make [authenticated API requests through its proxy](https://github.com/NangoHQ/nango#2-proxy), so callers need not handle the credential. | Keep session material in the Helix connector and return bounded read-only results. Do not expose a generic URL/method/body proxy or browser-control MCP tool for a portal session. |
| Nango's [connection read path](https://github.com/NangoHQ/nango/blob/master/packages/server/lib/controllers/v1/connections/connectionId/getConnection.ts) checks a separate `read_credentials` permission before including raw credentials. | Helix's model-visible connection tools should have no corresponding permission at all. A trusted internal worker may redeem a factor once; project and MCP status calls remain metadata only. |
| The [Connect UI](https://nango.dev/docs/guides/platform/free-self-hosting) needs an explicit encryption key and public URL in a self-hosted deployment. | Fail startup/readiness when intake is enabled but the key or canonical HTTPS origin is absent. Keep public invitation, private worker, and management routes separate at ingress. |

## Public issue reports and design implications

These are reports in Nango's tracker, not independently reproduced Helix defects. They identify failure modes worth testing before this flow handles real accounts.

| Report | Failure mode to avoid in Helix |
|---|---|
| [Self-hosted Connect UI used the wrong API origin](https://github.com/NangoHQ/nango/issues/5432) and [public/admin URL coupling caused routing trouble](https://github.com/NangoHQ/nango/issues/7052). | A deployment must have one explicit canonical public Connect origin, separate internal service address, and no implicit vendor/cloud fallback. Check the rendered form, redeem, submit, and status requests through the actual reverse proxy before enabling intake. |
| [A UI missed success after its WebSocket idle timeout](https://github.com/NangoHQ/nango/issues/5849). | Database state is authoritative. UI and bot status polling must recover after reload, disconnect, or missed event. Do not create a second attempt merely because a notification was lost. |
| [Updating an end user cleared its organization](https://github.com/NangoHQ/nango/issues/7297). | Keep organization/project/customer/provider binding immutable for an attempt. Do not use mutable labels or patchable external metadata for authorization; reconnect creates a new attempt. Check binding again on every bot action. |
| [A credential-format regex rejected valid newer API keys](https://github.com/NangoHQ/nango/issues/6741). | The generic intake validates field names, lengths, and required values; provider-specific code validates only stable requirements. Authentication success, not a guessed format regex, decides whether a password or OTP is accepted. Use the same server validator for form and direct HTTP submissions. |
| [A rotated refresh token was written to one field but read from another](https://github.com/NangoHQ/nango/issues/6136). | Define one authoritative place for portal session state and atomic updates. When a session expires or a portal rotates a token, replace the whole encrypted session version and retire the previous one. Never fall back to stale cookies or credentials. |
| [A self-hosted image omitted database migrations](https://github.com/NangoHQ/nango/issues/7422). | Exercise schema migration from the previous Helix version with existing pending/submitted intakes and revoked connections. New columns and indexes need an upgrade test; avoid a deployment that accepts links but cannot complete them. |

## Required Helix design changes before a real portal connector

### 1. Separate requests, attempts, and active connections

Create a `ConnectionAttempt` with immutable `{organization, project, customer, conversation, portal_kind, attempt_generation}` binding and a unique generation within its connection. The backend sets these values from an authenticated customer session and an allowlisted portal registry. A bot may request a connection, but cannot choose its customer, destination origin, redirect targets, or connection handle. A `PortalConnection` holds only the active attempt and the encrypted session reference. Starting a replacement attempt increments the generation; a late success from an older generation cannot become active. Expiring, revoking, or replacing a connection cancels its pending intakes and prevents later tool use.

The existing generic API's `customer_id` and `conversation_id` are caller-supplied routing strings. They remain appropriate for a trusted integration gateway, but must not be treated as authenticated identity. A customer-facing Meydan entry point must resolve them server-side before creating the attempt. The public invite URL remains a bearer capability until redeemed; a guessed or forwarded link does not by itself prove which customer is completing the form. The trusted gateway should present the intended customer and portal in its own UI and, where a logged-in Helix customer session exists, bind redemption to that session.

### 2. Give direct submitters a narrow write capability

The current generic HTTP submission endpoint requires project Update access. Do not hand a project-wide key to a third-party form. For external submission, issue a short-lived, one-attempt, submit-only capability scoped to the exact intake and field schema, or have the trusted customer gateway submit on its behalf. It cannot create intakes, read values or statuses, change bindings, or submit after replacement/revocation. Keep the existing project API for trusted server integrations only.

### 3. Make handoff recoverable without replaying secrets

Replace `ConsumeSecretIntake`'s callback-inside-transaction behavior before network login. Use atomic state transitions: `submitted -> claimed -> redeemed -> outcome`. A claim has a short lease and worker identity; redemption can happen once over an authenticated internal channel. No database lock spans browser activity. Every transition checks attempt generation, owner binding, expiry, and revocation. Reports carry a typed outcome and a connector-owned session reference, never a factor or cookie. If a worker disappears after redemption or form submission, mark the outcome `uncertain`, inspect only safe server-held state, and require reconnection when the result cannot be proven. Do not blindly retry the password or OTP.

Treat OTP as a new, short-lived challenge tied to the same attempt and browser context. Expire the challenge after a small number of entries; preserve only the minimum session state needed to continue. `invalid_credentials`, `otp_required`, `human_challenge`, `portal_changed`, `uncertain`, `connected`, `expired`, and `revoked` must have explicit UI states and transition rules.

### 4. Make session custody and key rotation explicit

For the first prototype, keep the authenticated browser context in the proposed Helix-owned worker and require reconnection after restart. If durable sessions are added, encrypt each context/profile independently, include key version and binding `{portal, organization, project, customer, connection}` as authenticated data, and atomically replace old state. Cookies and local storage are bearer credentials. Revoke invalidates the session handle before deleting state, including in-flight bot operations. Specify idle and absolute expiry and never share a browser profile across customers.

Version the encryption key used for stored secret intakes as well. Rotation must allow already issued, unexpired intakes to finish under a retiring key, then discard that key after their maximum lifetime; missing keys fail closed. Test rotation with pending and submitted intakes. Do not log ciphertext, decrypted values, OTPs, browser traffic, screenshots, or raw portal responses.

### 5. Keep the bot surface typed and reconcile status

The bot receives connection status and named read-only operations only. Each call resolves customer identity from trusted runtime state, checks the active generation and scope, uses the private session, and returns a small validated response. Portal content is untrusted input: do not forward raw HTML, DOM snapshots, page instructions, or arbitrary response bodies to the model. Unexpected form or redirect changes stop the connector with `portal_changed`.

The status endpoint reads committed state; events/WebSockets are optional hints. The Connect page can be reopened to see its final status, and the bot can poll without creating another invitation. Only an explicit retry starts a new generation. Audit attempt ID, state transition, action name, and timestamps; never factors or session material.

## Acceptance checks for the next slice

1. Two customers in one project cannot submit to, redeem, or use each other's connection; a late result from a superseded attempt cannot activate it.
2. Form and direct HTTP paths enforce the same schema and expiry. A direct submitter has no project-wide credential or read access.
3. A lost event, browser reload, or worker crash resolves to an authoritative status without duplicate connection or silent credential replay.
4. Redirection to an unapproved origin, altered login form, or unexpected challenge stops before entering another factor.
5. Revocation or expiry blocks a concurrent read tool and clears or invalidates all saved session references.
6. A self-hosted deployment smoke test confirms every public Connect request stays on the configured Helix origin and that management/worker routes are unreachable through the public form surface.
7. An upgrade and key-rotation test covers pending/submitted intakes and active/revoked connections without disclosing secrets in logs or responses.
