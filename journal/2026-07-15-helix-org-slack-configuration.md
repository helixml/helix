# Helix org Slack configuration notes

Date: 2026-07-15
Status: operational notes for later documentation

## Prime deployment facts

- Prime repository: `/home/luke/priya/helix`
- Public origin: `https://prime.helix.ml`
- `SERVER_URL`: `https://prime.helix.ml`
- Helix OAuth callback:
  `https://prime.helix.ml/api/v1/slack/oauth/callback`
- Slack Events Request URL:
  `https://prime.helix.ml/api/v1/slack/events`

`SERVER_URL` must have no trailing slash. Prime's Cloudflare Tunnel public
hostname forwards to `http://localhost:8080`. Cloudflare Access must bypass
authentication for the callback and events endpoints. The callback needs to be
public for the browser OAuth round trip; the events endpoint needs to be public
for Slack's URL verification and signed event delivery.

After changing `SERVER_URL`, recreate the stack so containers receive the new
environment. A restart does not apply environment changes.

## Slack app manifest

Create the app with Slack's official `From a manifest` flow using the manifest
copied from Helix. Do not use a `manifest_json` deep link.

For Prime the generated app features include:

```yaml
features:
  bot_user:
    display_name: Helix
    always_online: true
  app_home:
    home_tab_enabled: false
    messages_tab_enabled: true
    messages_tab_read_only_enabled: false
```

The writable App Home Messages tab is required for users to send DMs to Helix.

Current generated bot scopes from
`frontend/src/components/dashboard/slackManifest.ts`:

- `app_mentions:read`
- `channels:history`
- `channels:read`
- `channels:join`
- `groups:history`
- `groups:read`
- `im:history`
- `im:write`
- `chat:write`
- `chat:write.customize`
- `reactions:write`
- `files:write`
- `users:read`
- `users:read.email`

Current bot event subscriptions are derived from the history/read scopes:

- `app_mention`
- `message.channels`
- `message.groups`
- `message.im`

`message.im` is required for inbound DMs. The REST manifest must set the Events
Request URL above and leave Socket Mode disabled.

## Configure Helix

In the Slack app's Basic Information and OAuth settings, collect the Client ID,
Client Secret, and Signing Secret. Do not put their values in this file.

As a Helix deployment administrator:

1. Open Admin Panel -> Service Connections.
2. Create or edit the deployment-wide Slack app connection.
3. Set REST ingress mode.
4. Copy the Slack Client ID and Client Secret for the OAuth code exchange.
5. Copy the Slack Signing Secret for signed Events API verification.
6. Save the connection.

The Client ID enables the org OAuth CTA, the Client Secret completes the OAuth
code exchange, and the Signing Secret authenticates inbound events.

As the Helix org owner:

1. Open org Chart -> Settings -> Slack.
2. Select `Connect workspace`.
3. Choose the Slack workspace and approve the requested bot scopes.
4. Confirm the success feedback and the workspace row in Connected workspaces.

Connecting the same Slack team to the same Helix org refreshes its install and
token. A Slack team can be connected to only one live Helix org because inbound
events identify the Slack team but not the Helix org. A cross-org attempt
returns a conflict; disconnect the old org binding before moving the workspace.

After adding or changing scopes or event subscriptions, reinstall or
reauthorize the app in the workspace. Existing tokens do not gain new scopes
automatically.

For public or private channel ingestion, invite the Helix bot to every channel
it should read. App installation alone does not add it to channels.

## Validation

1. Confirm the Cloudflare Tunnel routes `https://prime.helix.ml` to the Helix
   service on `http://localhost:8080`.
2. In Slack Event Subscriptions, enter
   `https://prime.helix.ml/api/v1/slack/events` and confirm Slack marks the URL
   verified.
3. Confirm OAuth & Permissions contains the exact callback
   `https://prime.helix.ml/api/v1/slack/oauth/callback`.
4. In Helix org Settings, select `Connect workspace`. Expected: Slack shows the
   workspace picker and the requested bot scopes, then returns to
   `/orgs/:org_id/helix-org/settings` with `Slack workspace connected` feedback.
5. Confirm Connected workspaces shows the Slack team and installed app.
6. Invite the Helix bot to a test channel, send a message containing an exact
   Helix Bot ID, and confirm an inbound event appears on the workspace Slack
   topic and the named Bot activates.
7. Configure a human for Slack DM delivery, send an `expectsReply` `ask_human`,
   reply in the resulting Slack thread, and confirm the originating Bot receives
   the reply.
8. Send an unmatched channel message and confirm it does not activate an
   unrelated Bot.
9. Check API logs during steps 2, 6, and 7. Expected: Events API requests reach
   Helix, signature validation succeeds, the Slack team resolves to one org
   workspace, and thread routing records or finds the intended participant.

## DM routing architecture

Current implemented behavior on the reviewed branch:

- An `expectsReply` outbound Slack message records the originating Bot against
  the Slack message timestamp.
- An explicit Slack thread reply carries `thread_ts`; thread-follow uses that
  root to route the reply to the recorded Bot.
- A new top-level DM has no `thread_ts` and currently needs an exact Helix Bot
  ID to match a managed router output. An unmatched top-level DM activates no
  Bot.
- All inbound replies and DMs depend on Slack Events API delivery. Outbound Web
  API success does not prove inbound routing is configured.

Approved in-progress DM routing change:

- Explicit thread replies continue to use `thread_ts`.
- A normal top-level DM correlates by workspace router plus Slack DM channel to
  the latest replyable Bot for that conversation. It does not correlate by the
  new message timestamp.
- An unmatched top-level DM with no replyable conversation and no exact Bot ID
  still activates nobody.
- This correlation still depends on receiving `message.im` through the Events
  API.

## Troubleshooting map

| Symptom | Check and action |
|---|---|
| Slack reports `invalid_client_id` | The Helix Service Connection Client ID is wrong, belongs to another Slack app, or contains copied whitespace. Copy the Client ID from the same app whose manifest and callback are configured. |
| Slack reports `redirect_uri` mismatch | Set `SERVER_URL` exactly to `https://prime.helix.ml`, recreate the stack, and configure the exact callback URL in Slack OAuth & Permissions. |
| Slack authorization shows no scopes requested | Confirm the current manifest scopes, confirm the running backend is current, recreate the stack if needed, and start OAuth again from Helix rather than a saved authorize URL. |
| Slack app has no bot user | The app was not created from the current manifest or its `features.bot_user` was removed. Restore a `Helix` bot user with `always_online: true`, then reinstall. |
| Slack reports a missing bot scope | Add the scope from the current list, update event subscriptions if applicable, and reinstall or reauthorize the workspace. |
| No `Connect workspace` CTA | If no app exists, a deployment admin must use `Configure Slack app`. If an app exists but OAuth is incomplete, add its Client ID and Client Secret in Admin Panel -> Service Connections; the manual bot-token path remains available. |
| Slack says sending messages is turned off | Enable App Home -> Messages Tab and allow users to send messages; `messages_tab_read_only_enabled` must be false. |
| OAuth feedback banner shows literal `+` characters | The current callback encodes query values and the current UI decodes them with `URLSearchParams`. A literal plus indicates a stale callback/frontend or double encoding; deploy the matching build and hard refresh. |
| Workspace already connected | The Slack team is bound to another Helix org. Disconnect it there before connecting it to this org. Same-org reconnect should refresh the existing install. |
| OAuth callback returns HTTP 502 | Check Prime API logs and outbound access to Slack. Verify the Client Secret belongs to the configured Client ID. The current callback normally redirects with friendly error feedback, so a raw 502 can also indicate a stale backend or proxy response. |
| Manual bot-token validation returns HTTP 502 | Helix could not reach Slack or Slack returned a non-token upstream failure. An explicitly invalid token should return HTTP 400 instead. |
| Slack workspace topic exists but is empty | No event reached ingestion. Check the Events Request URL, Cloudflare Access bypass, signing secret, subscribed events, granted scopes, app reinstall, and channel invitation. |
| Slack topic has an event but no Bot activates | For current top-level routing, include an exact Helix Bot ID. For a reply, verify `thread_ts` and prior participant recording. Do not treat an unmatched message as an ingestion failure. |
| No inbound Events API requests appear | Verify `message.im` or the relevant channel event is subscribed, the request URL is verified, Access bypass is active, the bot is invited to the channel, and the workspace was reinstalled after scope changes. |
| Generated URLs contain `//api/...` | Remove the trailing slash from `SERVER_URL`, recreate the stack, and regenerate or recopy the manifest. The OAuth callback now normalizes its slash, but all deployment URLs should use the canonical origin without one. |

## Security notes

- Never record Client Secret, Signing Secret, bot token, or OAuth state in
  journals, screenshots, tickets, or command output.
- Cloudflare Access bypass is limited to the OAuth callback and Slack events
  endpoints. Do not bypass the org Settings or administrator APIs.
- Slack event signatures and timestamps remain the trust boundary for the
  public events endpoint.
