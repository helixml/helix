# Helix-org outbound inventory

This matrix freezes PR 3's outbound boundary. New external actions run from the
Worker sandbox with a credential explicitly granted through `get_secret`.
Provider responses are the receipt. No action creates an event-source edge or
activation merely for auditing.

| Current caller/path | Class | Replacement or retained owner | Receipt/retry contract | Acceptance coverage |
|---|---|---|---|---|
| Slack Topic publish (`Publishing.PublishWithReceipt`) | External action, legacy compatibility | `get_secret` then Slack Web API (`chat.postMessage`, replies, reactions, uploads, edits) | Check HTTP status and Slack `ok`; re-resolve after 401/403 and inspect state before retrying a non-idempotent call | Worker-secret MCP boundary tests; Slack compatibility publish tests |
| Postmark `Transport.Emit` | External action, legacy implementation | `get_secret(POSTMARK_SERVER_TOKEN)` then `POST https://api.postmarkapp.com/email` with `X-Postmark-Server-Token` | Check non-2xx and response body; inspect provider state before retry | Postmark payload/error tests; live provider delivery remains an external QA gate |
| Webhook Topic `outbound_url` | External action, legacy compatibility | `get_secret` for optional auth, then `curl` to the non-secret URL held in Role/configuration | HTTP response is the receipt; caller chooses idempotency key/retry | Publishing compatibility adapter and webhook transport tests |
| GitHub Topic/provider calls | Ingress control plane or external action | Webhook installation remains ingress provisioning; Worker actions use `gh`, git HTTPS, or GitHub API after `get_secret` | Native command/API result; refresh on 401/403 | GitHub ingress suites and Worker-secret GitHub App resolver tests |
| GitLab Topic/provider calls | Ingress control plane or external action | Webhook installation remains ingress provisioning; Worker actions use `glab`, git HTTPS, or GitLab API after `get_secret` | Native command/API result; refresh on 401/403 | GitLab ingress suites |
| `dm` | Helix-internal action | Retained: reporting-line authorization plus Topic compatibility storage until PR 4 | Helix event result; recipient's next operation remains the lifecycle check | DM authorization and lifecycle tests |
| `reports` + `publish` Chat flow | Helix-internal action | Retained until PR 4 replaces Topic storage with Chat Triggers | Helix event result; recipient's next operation remains the lifecycle check | Reporting-line/channel reconciliation tests |
| `ask_human` / `HumanDelivery` | Helix policy action | Retained: human-node validation and explicit in-app/Slack-DM delivery | Helix delivery ID/provider error | Human delivery tests |
| Inbound Slack, Postmark, webhook, GitHub, GitLab, cron, Helix events | Inbound activation | Unchanged until PR 4 Trigger cutover | Append, notify, subscription dispatch, Processor traversal; never echo outbound | Existing transport and dispatch suites |
| Topic REST/MCP publish and Processor output publish | Temporary compatibility | `publishing.LegacyDelivery` is the only provider-delivery decorator through PR 4 | Preserves legacy receipt/error behavior; inbound provenance suppresses delivery | Publishing service and REST/MCP parity tests |

The legacy adapter is a PR 5 deletion target. New services, tools, prompts, and
Trigger/Processor DTOs must not register or call a legacy deliverer.

Native action guidance:

- Retrieve the credential immediately before use and never print it or place it
  in command-line arguments when headers, stdin, or a credential file work.
- On 401/403, call `get_secret` again. Before retrying email, Slack posts, or
  another non-idempotent request, inspect provider state for prior success.
- Postmark uses `POST /email`, `X-Postmark-Server-Token`, and both the HTTP
  status and JSON response body determine success.
- Materialize structured credentials into a permission-restricted file only
  for the lifetime required by the native tool.
