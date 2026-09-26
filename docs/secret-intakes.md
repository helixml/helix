# Secret intake and Connect API

Helix can issue a short-lived link to collect a requested set of secret fields outside chat. The hosted page sends values directly to Helix; the MCP tool and public project API return only metadata and status. A trusted backend connector can consume submitted values once through `HelixAPIServer.ConsumeSecretIntake`, perform a login or other bounded operation, and return only a safe result to the bot. No generic plaintext read endpoint or value-returning MCP tool is provided.

Enable it on the Helix API server:

```bash
HELIX_SECRET_INTAKE_ENABLED=1
HELIX_ENCRYPTION_KEY=<deployment-specific secret>
SERVER_URL=https://<browser-reachable-helix-origin>
```

The link uses the configured `SERVER_URL`, which must be HTTPS except for loopback development. The encryption key is required at startup. Submitted values are encrypted at rest, available to trusted backend code for one hour, and then cleared by a minute-interval reaper. Consuming or revoking clears ciphertext immediately. The mock portal API remains separately gated by `HELIX_PORTAL_MOCK_ENABLED`.

## Project API

Authenticate with a Helix bearer key authorized for the project. Keep the bearer key in a trusted server or gateway, never in browser JavaScript or a model-visible tool result. An external service with project access can submit directly over HTTPS using the `submissions` endpoint; its `204` response is write only.

| Method | Path | Purpose |
|---|---|---|
| POST | `/api/v1/projects/{project_id}/secret-intakes` | Create an intake; project Create access. |
| GET | `/api/v1/projects/{project_id}/secret-intakes/{intake_id}` | Read metadata and status; project Get access. |
| POST | `/api/v1/projects/{project_id}/secret-intakes/{intake_id}/submissions` | Submit `{"values":{"field_name":"value"}}`; project Update access. |
| DELETE | `/api/v1/projects/{project_id}/secret-intakes/{intake_id}` | Revoke and clear ciphertext; project Update access. |

Create request:

```json
{
  "customer_id": "customer-123",
  "conversation_id": "chat-456",
  "title": "Connect your portal",
  "description": "Enter your portal sign-in details.",
  "brand_name": "Example Support",
  "accent_color": "#20a5a1",
  "fields": [
    {"name":"username","label":"Username","type":"text","required":true,"autocomplete":"username"},
    {"name":"password","label":"Password","type":"password","required":true,"autocomplete":"current-password"}
  ]
}
```

`customer_id` and `conversation_id` are required routing metadata, each at most 128 bytes. Derive them from your authenticated customer session; Helix does not authenticate the end customer from these strings alone. `title` is required (max 100 bytes), `description` is optional (max 300 bytes), `brand_name` is optional (max 80 bytes), and `accent_color` must be `#RRGGBB`. Request 1 to 8 unique fields with lowercase `name` identifiers, labels at most 80 bytes, `text` or `password` type, and optional `required` and `autocomplete`. Accepted autocomplete values are `off`, `username`, `current-password`, `new-password`, and `one-time-code`. Each submitted value is limited to 4096 bytes. The server rejects extra fields.

The `201` response has an `intake` object with `id`, project/customer/conversation IDs, field descriptors, `status`, and expiry, plus an `invite_url` like `https://helix.example/connect/intake#<random-token>`. Values never appear in create, status, or revoke responses. Statuses are `pending`, `submitted`, `consumed`, `expired`, and `revoked`. The invitation expires in 10 minutes. A user click redeems it once for a 20-minute HttpOnly, SameSite=Strict flow cookie and a CSRF-protected form. The fragment is removed from browser history before redemption. Treat the URL as a bearer invitation and deliver it through an appropriate customer channel.

Direct submission example:

```http
POST /api/v1/projects/prj_123/secret-intakes/sci_123/submissions
Authorization: Bearer <server-side Helix key>
Content-Type: application/json

{"values":{"username":"alice","password":"..."}}
```

A successful submission returns `204`, with no values. This path is for trusted third-party backends; browsers should use the invitation form.

## MCP tools

The owner bot receives `request_secret_intake` and `get_secret_intake_status`; attach them explicitly to other bots that may ask customers for secrets. The request tool takes the same JSON fields as the create API and returns only `{id,status,invite_url}`. The status tool takes `{"intake_id":"sci_..."}` and returns status and expiry only. Both resolve the caller's project from Helix's authenticated bot runtime state; the model cannot choose a project ID. Give the link to the customer as a link or action, and never ask them to paste values into chat. A new request can collect an OTP later in the flow using a `password` field with `autocomplete: "one-time-code"`.

The URL can be visible in the MCP tool result, but it cannot read submitted values. Do not attach Helix's existing plaintext `get_secret` tool to this intake flow. A real portal integration should call `ConsumeSecretIntake` in a trusted connector, establish and retain its portal session server-side, then expose narrow actions or sanitized status to the bot. No real portal adapter is included in this PR.

## Artifact HTML

Set `artifact_id` in the create request to use a single-file HTML Artifact from the same project. Put exactly one placeholder in its body:

```html
<section>
  <h2>Connect your account</h2>
  <p>Enter the details requested below.</p>
  <div data-helix-form></div>
</section>
```

Helix snapshots the artifact at invitation creation, strips scripts, styles, attributes, external media, and artifact-supplied form controls, then inserts its own form at the placeholder. Safe static headings, paragraphs, emphasis, and lists are retained. The artifact cannot choose the input names, read entered values, or change the form destination. Use `brand_name` and `accent_color` for supported white-label styling. Updating an artifact changes future invitations; it does not alter already issued forms. This constrained rendering is intentional because ordinary Artifacts can contain agent-authored JavaScript and are not trusted to handle credentials.
