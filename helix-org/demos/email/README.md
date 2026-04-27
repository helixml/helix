# Email

A customer-service team that runs entirely over email. Customers
email `support@yourdomain.com`; Sam (a helix-org Worker) reads the
mail, drafts a reply, and replies — by email. The whole back-and-
forth lives on one Stream, `s-support`, with `transport: email`.
Threading preserves the conversation: a customer's reply lands on
the same Stream, threaded to Sam's earlier message via standard
`In-Reply-To` / `References` headers.

> **Status note.** The email transport itself isn't shipped yet —
> this demo describes the experience once it is. The Postmark
> setup steps (DNS, API tokens, inbound webhook URL) are real
> and reusable today; the helix-org chat prompts are the target
> shape of the email transport's stream-config and the
> `customer-service` role.

About 15 minutes the first time (Postmark account, DNS records,
domain verification). Re-runs after that are one chat prompt.

## What this demo shows

- **One Stream, both directions.** `s-support` is bidirectional
  email: inbound POSTs from Postmark land as Events; outbound
  events on the same Stream become outbound emails via Postmark's
  send API. Same envelope (`domain.Message`) in both directions.
- **External humans as participants.** `Message.From` carries the
  customer's email address (`alice@example.com`) verbatim; Sam's
  outbound carries `support@yourdomain.com`. No prefixes — value
  shape disambiguates.
- **Threading by header.** Sam sets `InReplyTo` and `ThreadID` on
  the outbound Message. Postmark renders them as RFC2822 headers.
  Mail clients show one threaded conversation. Replies arrive on
  the same Stream so Sam can read prior history before answering.
- **Role drives behaviour.** Customer-service tone, escalation
  rules, when to refuse — all in `roles/customer-service.md`.
  Switching to a snarkier voice is a `update_role` away.

## Prerequisites

- [Postmark](https://postmarkapp.com/) account. Free tier handles
  100 emails/day — plenty for this demo.
- An email address you control to use as the Sender Signature
  (any address — `you@gmail.com`, an iCloud address, whatever).
  No domain required.
- Public URL for your local helix-org so Postmark can reach the
  inbound webhook. [`ngrok http 8080`](https://ngrok.com/) or a
  Cloudflare Tunnel works for testing.
- `helix-org` and `claude` on PATH; `jq` and `curl` for the setup
  commands below.

> **For this installation, Postmark is already wired up.**
> Server `helix-org` (ID 19042071) is provisioned, the Sender
> Signature on `phil@winder.ai` is confirmed, and a sanity-check
> send round-tripped through `phil@winder.ai`'s inbox. Tokens
> live in `~/.helix/postmark` (mode 0600); source it before
> running anything that calls Postmark:
>
> ```bash
> set -a && source ~/.helix/postmark && set +a
> ```
>
> The InboundHookUrl is still empty — it'll be set when the
> email transport ships and the demo can be pointed at a public
> URL. Skip the Postmark setup section below; it's there for
> first-time installations.

(With your own domain, you can graduate to `support@yourdomain.com`
later — see the production setup notes at the end of the
Postmark section.)

## Postmark setup

The first run-through takes some clicking through Postmark's UI,
but every step that *can* be a curl is one. Save your tokens once
and the setup is reproducible.

### 1. Sign up and grab the Account API token

[postmarkapp.com](https://postmarkapp.com/) → sign up → in the
top-right menu, **API Tokens** → **Account API tokens** → copy.
This token manages account-wide things (domains, servers).

```bash
export POSTMARK_ACCOUNT_TOKEN=<your account token>
```

### 2. Add and verify your sending domain

```bash
curl -X POST "https://api.postmarkapp.com/domains" \
  -H "Accept: application/json" \
  -H "Content-Type: application/json" \
  -H "X-Postmark-Account-Token: $POSTMARK_ACCOUNT_TOKEN" \
  -d '{
    "Name": "yourdomain.com",
    "ReturnPathDomain": "pm-bounces.yourdomain.com"
  }' | tee /tmp/pm-domain.json
```

The response includes `ID`, `DKIMHost`, `DKIMTextValue`,
`ReturnPathDomainCNAMEValue`. Save the ID:

```bash
export PM_DOMAIN_ID=$(jq -r '.ID' /tmp/pm-domain.json)
```

Add the DKIM, SPF (`v=spf1 a mx include:spf.mtasv.net ~all`), and
return-path CNAME to your DNS. Then verify:

```bash
curl -X PUT "https://api.postmarkapp.com/domains/${PM_DOMAIN_ID}/verifyDkim" \
  -H "Accept: application/json" \
  -H "X-Postmark-Account-Token: $POSTMARK_ACCOUNT_TOKEN"

curl -X PUT "https://api.postmarkapp.com/domains/${PM_DOMAIN_ID}/verifyReturnPath" \
  -H "Accept: application/json" \
  -H "X-Postmark-Account-Token: $POSTMARK_ACCOUNT_TOKEN"
```

Both should return `"DKIMVerified": true` / `"ReturnPathDomainVerified": true`.

### 3. Create a Postmark Server (transactional)

A Postmark "Server" is one project's worth of email — its own
sending stream, inbound stream, settings, and API token. We need
one for this demo.

```bash
curl -X POST "https://api.postmarkapp.com/servers" \
  -H "Accept: application/json" \
  -H "Content-Type: application/json" \
  -H "X-Postmark-Account-Token: $POSTMARK_ACCOUNT_TOKEN" \
  -d '{
    "Name": "helix-org support",
    "Color": "Green",
    "TrackOpens": false,
    "TrackLinks": "None"
  }' | tee /tmp/pm-server.json
```

Grab the **Server token** from the response (`ApiTokens[0]`). This
is what helix-org uses to send and what authenticates configuration
calls on this Server:

```bash
export POSTMARK_TOKEN=$(jq -r '.ApiTokens[0]' /tmp/pm-server.json)
```

### 4. Set the inbound forwarding domain

Postmark's inbound works two ways — pick one.

**(A) Quick: use the hosted inbound hash address.** Every Postmark
Server has a unique address like `abc123def456@inbound.postmarkapp.com`.
Customers email *that*. Fine for testing, ugly for production.

```bash
curl -s "https://api.postmarkapp.com/server" \
  -H "Accept: application/json" \
  -H "X-Postmark-Server-Token: $POSTMARK_TOKEN" \
  | jq -r '.InboundAddress'
```

**(B) Production: MX your own domain to Postmark.** Tell Postmark
which subdomain to receive on:

```bash
curl -X PUT "https://api.postmarkapp.com/server" \
  -H "Accept: application/json" \
  -H "Content-Type: application/json" \
  -H "X-Postmark-Server-Token: $POSTMARK_TOKEN" \
  -d '{ "InboundDomain": "inbound.yourdomain.com" }'
```

Then add an MX record on `inbound.yourdomain.com` → `inbound.postmarkapp.com`
(priority 10) at your DNS provider, and a forwarding rule (or
catch-all) at your registrar so `support@yourdomain.com`
delivers to `support@inbound.yourdomain.com`.

### 5. Point Postmark at your helix-org instance

Postmark POSTs every inbound email to a URL of your choice. We
want it to hit helix-org's email-transport endpoint, scoped to
`s-support`:

```bash
# Replace https://abc123.ngrok.app with your public URL.
curl -X PUT "https://api.postmarkapp.com/server" \
  -H "Accept: application/json" \
  -H "Content-Type: application/json" \
  -H "X-Postmark-Server-Token: $POSTMARK_TOKEN" \
  -d '{
    "InboundHookUrl": "https://abc123.ngrok.app/email/postmark/s-support"
  }'
```

The path shape — `/email/postmark/<streamID>` — tells the
transport which Stream the inbound message belongs to. One Server
can fan to multiple Streams by giving each its own URL (or its own
Postmark Server, if you prefer hard isolation).

### 6. Sanity-check sending

```bash
curl -X POST "https://api.postmarkapp.com/email" \
  -H "Accept: application/json" \
  -H "Content-Type: application/json" \
  -H "X-Postmark-Server-Token: $POSTMARK_TOKEN" \
  -d '{
    "From": "support@yourdomain.com",
    "To": "you@example.com",
    "Subject": "Postmark wired",
    "TextBody": "If you got this, Postmark + DNS are good."
  }'
```

If that lands in your inbox, Postmark is fully configured. Save
`POSTMARK_TOKEN` somewhere durable; helix-org reads it from env.

## Run the demo

### 1. Bootstrap and configure (one-time)

```bash
cd /home/phil/helix/helix-org
make build
rm -rf /tmp/email-envs /tmp/email.db
./bin/helix-org bootstrap --db /tmp/email.db --envs-dir /tmp/email-envs
```

Then set the Postmark transport config in the database (the CLI
opens the same SQLite file the server uses; live updates work
without a restart):

```bash
source ~/.helix/postmark
./bin/helix-org config set --db /tmp/email.db transport.postmark "{
  \"token\":\"$POSTMARK_SERVER_TOKEN\",
  \"inbound\":\"$POSTMARK_INBOUND\",
  \"from\":\"$POSTMARK_FROM\",
  \"disable_reply_to\":true
}"
```

`disable_reply_to:true` is a workaround for Postmark's
"pending approval" restriction on new accounts. Postmark counts
`Reply-To` as a recipient for the same-domain rule, so a
`Reply-To` at `inbound.postmarkapp.com` blocks any send from a
`winder.ai` `From`. With Reply-To off, replies route to whatever
the customer's mail client defaults to (usually the From address)
rather than back into helix — fine for testing one-shot replies,
but the customer→Sam round trip won't close until Postmark
approves the account. Drop the flag once approved.

### 2. Start the server (terminal 1)

```bash
./bin/helix-org serve --db /tmp/email.db --envs-dir /tmp/email-envs
```

The server logs `email transport enabled provider=postmark` once
the Postmark config is loaded.

### 3. Expose helix-org publicly (terminal 2)

```bash
cloudflared tunnel --url http://localhost:8080
```

Or `ngrok http 8080` if you have ngrok set up. Note the public
URL it prints (e.g. `https://accounts-bookmarks-permission-bloomberg.trycloudflare.com`).

### 4. Point Postmark's inbound webhook at helix-org

```bash
source ~/.helix/postmark
CF_URL=<paste your tunnel URL>
curl -sS -X PUT "https://api.postmarkapp.com/server" \
  -H "Accept: application/json" \
  -H "Content-Type: application/json" \
  -H "X-Postmark-Server-Token: $POSTMARK_SERVER_TOKEN" \
  -d "{\"InboundHookUrl\": \"${CF_URL}/email/postmark\"}"
```

The path is `/email/postmark` (not `/email/postmark/<streamID>`).
The transport extracts the alias from `OriginalRecipient` —
mail to `<hash>+sam@inbound.postmarkapp.com` routes to the
Stream whose alias is `sam`.

### 5. Hire Sam (terminal 3)

```bash
cd demos/email
../../bin/helix-org chat --new
```

> Set up customer service from this directory.
>
> 1. Read `./roles/customer-service.md` and create role
>    `r-customer-service` from it.
>
> 2. Create stream `s-support` with transport kind `email` and
>    config `{"alias":"sam"}`. (Provider creds are server-level —
>    in the `transport.postmark` config row — so the stream only
>    declares the routing identity.)
>
> 3. Create position `p-customer-service` under `p-root` with
>    role `r-customer-service`.
>
> 4. Hire AI worker `w-sam` into it with `identityContent` from
>    `./workers/sam.md`.
>
> 5. Read `roles/customer-service.md`'s `## Tools (MCP)` line and
>    grant exactly those tools to `w-sam`.
>
> 6. `worker_log` on `w-sam` with `wait=60` until you see
>    `=== exit: ok ===` to confirm Sam is subscribed and ready.

### 6. Test: send Sam an email

From your normal mail client (Gmail, etc.), send an email to
`<your-inbound-hash>+sam@inbound.postmarkapp.com`:

> **To:** `6b3bd15f407ea200e7607799b4c9eae8+sam@inbound.postmarkapp.com`
> *(your hash will be different — see `~/.helix/postmark`)*
>
> **Subject:** Webhook stream isn't firing
>
> Hi support — I've got a stream with transport=webhook but my
> POSTs aren't waking the worker. Subscriber is set, server logs
> show 200 on the POST. What am I missing?
>
> — Phil

Back in chat:

> Subscribe me to `s-support`. `read_events` with `wait=60` until
> the email lands and Sam replies. Show me both verbatim.

The cascade: Postmark receives at the alias address → POSTs JSON
to `${tunnel}/email/postmark` → email transport extracts `+sam`
from the recipient, finds the matching Stream, builds
`Message{From:"phil@gmail.com", Subject:"...", Body:"...",
MessageID:"<...>"}` and appends to `s-support` → dispatcher wakes
Sam (Source=="" inbound events still trigger subscriber
activations; what gets *suppressed* is the outbound emit on those
events, so a bidirectional stream doesn't echo to itself) → Sam
reads, drafts a reply, `publish`es to `s-support` with `to`,
`subject`, `inReplyTo`, `threadId` set → email transport renders
the outbound `Message` and POSTs Postmark's `/email` API → your
inbox lights up. ~15–25 seconds end to end (Sam's claude
activation is the slow part).

### 7. Stop

Ctrl-C terminals 1 and 2.

### Closing the customer-reply loop

Once your Postmark account is approved (request from the Postmark
UI; usually <24h), set `disable_reply_to:false`:

```bash
./bin/helix-org config set --db /tmp/email.db transport.postmark "{
  \"token\":\"$POSTMARK_SERVER_TOKEN\",
  \"inbound\":\"$POSTMARK_INBOUND\",
  \"from\":\"$POSTMARK_FROM\"
}"
```

The change takes effect on the next outbound send — no restart.
After that, Sam's outbound emails carry `Reply-To:
<hash>+sam@inbound.postmarkapp.com`, the customer's reply routes
back through Postmark's inbound to `s-support`, and Sam
activates again on the new event. Threading via `Message-ID` /
`In-Reply-To` keeps the conversation in one thread.

## What this shows

- **Email is just another Transport.** Once the transport
  translates Postmark JSON ↔ `domain.Message` at its boundary,
  Sam's role looks identical in shape to a Slack support role or
  an SMS support role — same envelope, same tools, different
  identifiers in `From` / `To`.
- **Threading is the transport's job, not the Role's.** Sam sets
  `InReplyTo` because he's polite; the email transport renders it
  to RFC2822 headers because that's email's threading protocol.
  A Slack version of this role would set `ThreadID` and the Slack
  transport would map to `thread_ts`.
- **Credentials live in the DB, not in MCP.** The
  `transport.postmark` config row is set via the `helix-org config`
  CLI — never via chat. Operational config (provider creds, model
  selection, public URL) is mutated by the operator on the host;
  org-graph mutations (workers, roles, streams) go through MCP.
  Same SQLite file, two access paths. See
  [`design/config.md`](../../design/config.md).
- **Live config updates.** `helix-org config set
  transport.postmark …` takes effect on the next outbound send —
  no server restart, no signal. SQLite WAL mode handles the
  concurrent-writer-while-server-runs case cleanly.
- **One inbound URL, alias-based routing.** All inbound mail flows
  through one Postmark webhook URL (`/email/postmark`); the
  transport reads the `+alias` from `OriginalRecipient` and routes
  to the matching Stream. Adding `billing@` is a `create_stream`
  with `{"alias":"billing"}` — no Postmark UI changes, no extra
  webhook URLs.
- **No echo loops on bidirectional streams.** Inbound events have
  `Source=""`; the dispatcher skips outbound emit for those, so a
  Stream that's both inbound and outbound on the same provider
  doesn't ping-pong. Worker-published events (`Source!=""`) emit
  normally.

## What this doesn't cover (yet)

- **Multiple support aliases on different domains.** All aliases on
  one Postmark Server share the same `From` (the verified Sender
  Signature). For `billing@brand-a.com` vs `support@brand-b.com`
  with different Sender Signatures, you'd want one Postmark Server
  per brand and a per-stream provider override — out of scope today.
- **HTML mail.** The transport hands `Message.Body` the
  `TextBody` from Postmark by default. HTML bodies and rich
  attachments work via `Message.BodyContentType` and
  `Message.Attachments`, but the role prompt below sticks to
  plain text because that's the right default for support replies.
- **Auto-classifying spam / out-of-office / bounces.** Postmark
  marks `Headers["X-Spam-Score"]` and friends; the email transport
  forwards them in `Message.Extra` and the role can filter, but
  this demo doesn't show it.
- **Multi-Worker hand-offs.** Sam escalates by writing "Let me
  get a teammate" — there's no teammate. Adding a
  `r-support-engineer` role that Sam DMs would close the loop;
  out of scope for this README.
