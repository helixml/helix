# Mock portal connection handoff

For generic credential and secret collection through HTTP and MCP, see [secret-intakes.md](secret-intakes.md).

This first slice tests a credential handoff without a real portal. The
customer opens a Helix-hosted `/connect` page and enters a demo username,
password, and OTP. No factor is sent to a Helix chat endpoint, agent sandbox,
or model provider. The mock session is encrypted at rest and a project API
exposes only a bounded `account-status` operation.

Enable the mock explicitly:

```bash
HELIX_PORTAL_MOCK_ENABLED=1
HELIX_ENCRYPTION_KEY=<deployment-specific secret>
SERVER_URL=https://<browser-reachable-helix-origin>
```

`HELIX_ENCRYPTION_KEY` is required even for the mock. Do not use real portal
credentials. The form displays `demo` / `demo-password`, followed by OTP
`123456`.

## Project API

The caller needs a Helix bearer key authorized for the project. Keep this key
in a trusted gateway or server backend, never in the browser or an agent
sandbox. The gateway derives the customer and conversation IDs from its own
authenticated session.

| Method | Path | Purpose |
|---|---|---|
| POST | `/api/v1/projects/{id}/portal-connections` | Create a mock connection invitation; requires project Create. |
| GET | `/api/v1/projects/{id}/portal-connections/{connection_id}` | Read status; requires project Get. |
| GET | `/api/v1/projects/{id}/portal-connections/{connection_id}/account-status` | Return a sanitized mock account status when connected; requires project Get. |
| DELETE | `/api/v1/projects/{id}/portal-connections/{connection_id}` | Revoke and clear the mock session; requires project Update. |

Create body:

```json
{
  "customer_id": "customer-123",
  "conversation_id": "chat-456",
  "portal": "mock",
  "brand_name": "Example Support",
  "accent_color": "#20a5a1"
}
```

`customer_id` and `conversation_id` are required, each at most 128 bytes.
`portal` must be `mock`. The optional brand name is at most 80 bytes, and
the optional color must be `#RRGGBB`. The response is `201`:

```json
{
  "connection": {
    "id": "pca_...",
    "project_id": "prj_...",
    "customer_id": "customer-123",
    "conversation_id": "chat-456",
    "portal": "mock",
    "status": "password_pending",
    "expires_at": "..."
  },
  "invite_url": "https://helix.example/connect#..."
}
```

The random invitation token is in the URL fragment, which is not sent to the
server in the initial GET. Trusted page JavaScript removes the fragment; a
customer click on **Continue** exchanges the token once for a short-lived
HttpOnly flow cookie. The invitation
expires after 10 minutes; the form flow expires after 20 minutes. The page
submits the password and OTP directly to Helix, with a CSRF token. It has no
third-party scripts, analytics, or artifact content.

Status values are `password_pending`, `otp_pending`, `connected`, `failed`,
`expired`, and `revoked`. The connected mock session lasts one hour. A status
response contains only metadata and expiry; it never returns credentials or
session tokens. The bounded account-status response is:

```json
{"customer_id":"customer-123","portal":"mock","account_status":"active"}
```

Keep the invitation link out of chat prompts and interaction history. A
gateway can render it as an action button, poll status server-side, then make
the account-status call for its current customer and pass only the result to
the bot. The project API alone does not enforce a gateway's end-customer
identity; that binding is the gateway's responsibility in this mock slice.

## Scope and next adapter

The mock does not contact a portal. A real adapter needs the target portal's
login sequence, OTP challenge behavior, required post-login operations, and
session lifetime. It must own the session outside the model-controlled
browser and expose only bounded operations. The current `brand_name` and
`accent_color` fields are the first white-label controls. Verified custom
domains and Artifact-based design previews are not implemented; ordinary
Artifacts must not host credential fields or execute code on this page.
