# Email

A two-Worker support team that talks to customers — and to each
other — by email. Sam is customer service (alias `sam`); Lee is
engineering (alias `engineer`). When a customer emails Sam with a
technical question Sam can't answer, he forwards it to Lee at
Lee's helix alias. Lee replies by email. Sam paraphrases for the
customer and replies. Every hop crosses Postmark; every Stream
(`s-support`, `s-engineer`) is bidirectional.

About 20 minutes the first time (Postmark account + Sender
Signature). Re-runs after that are one chat prompt.

## What this demo shows

- **Both directions on every Stream.** `s-support` and `s-engineer`
  each accept inbound mail at their `+alias` address *and* render
  outbound `publish` calls back through Postmark's send API.
  Same `domain.Message` envelope in both directions; the only
  per-stream config is `{"alias": "..."}`.
- **Workers as email participants.** Sam emails Lee. Lee emails
  Sam. The customer emails Sam. All three legs use the same
  transport, the same envelope, the same alias-based routing.
  Workers are first-class email participants.
- **Threading is the spine.** Sam sets `ThreadID` on his
  escalation to Lee. Lee preserves it on his reply. Sam reads it
  back to find the original customer query in `s-support` history
  and threads his customer-facing reply to it. The whole
  conversation is one logical thread despite four physical
  emails.
- **Role drives behaviour.** Sam decides "answer myself or
  forward" by reading his own role text — there's no hard-coded
  routing table. Editing `roles/customer-service.md` and running
  `update_role` shifts the policy live.

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

> **For this installation, Postmark is already wired up and
> approved.** Server `helix-org` (ID 19042071) is provisioned, the
> Sender Signature on `phil@winder.ai` is confirmed, and the
> account is past pending-approval, so cross-domain sends to
> `+alias@inbound.postmarkapp.com` work. Tokens live in
> `~/.helix/postmark` (mode 0600); source it before running
> anything that calls Postmark:
>
> ```bash
> set -a && source ~/.helix/postmark && set +a
> ```
>
> Skip the Postmark setup section below; it's there for first-time
> installations.

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
  \"from\":\"$POSTMARK_FROM\"
}"
```

(For pending-approval Postmark accounts, append
`,\"disable_reply_to\":true` — outbound succeeds but customer
replies won't route back through helix until approval lands.)

### 2. Substitute `<INBOUND_HASH>` into role files

The roles at `roles/customer-service.md` and `roles/engineer.md`
each contain `<INBOUND_HASH>+<other-alias>@inbound.postmarkapp.com`
addresses so each Worker knows where to email the other. Fill in
your hash before creating the roles:

```bash
HASH="${POSTMARK_INBOUND%%@*}"
mkdir -p /tmp/email-run/roles /tmp/email-run/workers
for f in demos/email/roles/*.md; do
  sed "s/<INBOUND_HASH>/$HASH/g" "$f" > /tmp/email-run/roles/$(basename "$f")
done
cp demos/email/workers/*.md /tmp/email-run/workers/
```

### 3. Start the server (terminal 1)

```bash
./bin/helix-org serve --db /tmp/email.db --envs-dir /tmp/email-envs
```

The server logs `email transport enabled provider=postmark` once
the Postmark config is loaded.

### 4. Expose helix-org publicly (terminal 2)

```bash
cloudflared tunnel --url http://localhost:8080
```

Or `ngrok http 8080` if you have ngrok set up. Note the public
URL it prints (e.g.
`https://accounts-bookmarks-permission-bloomberg.trycloudflare.com`).

### 5. Point Postmark's inbound webhook at helix-org

```bash
source ~/.helix/postmark
CF_URL=<paste your tunnel URL>
curl -sS -X PUT "https://api.postmarkapp.com/server" \
  -H "Accept: application/json" \
  -H "Content-Type: application/json" \
  -H "X-Postmark-Server-Token: $POSTMARK_SERVER_TOKEN" \
  -d "{\"InboundHookUrl\": \"${CF_URL}/email/postmark\"}"
```

The path is `/email/postmark` (one URL for the whole installation).
The transport extracts the alias from `OriginalRecipient` —
mail to `<hash>+sam@inbound.postmarkapp.com` routes to the Stream
whose alias is `sam`, mail to `<hash>+engineer@…` routes to the
Stream whose alias is `engineer`.

### 6. Hire Sam and Lee (terminal 3)

```bash
cd /tmp/email-run
../../bin/helix-org chat --new
```

> Set up the support team from this directory.
>
> **Customer service.** Read `./roles/customer-service.md` and
> create role `r-customer-service` from its body. Create stream
> `s-support` with transport.kind `email` and config
> `{"alias":"sam"}`. Position `p-customer-service` under `p-root`
> with that role. Hire AI worker `w-sam` with identityContent
> from `./workers/sam.md`. Grant the tools listed in the role's
> `## Tools (MCP)` section.
>
> **Engineering.** Read `./roles/engineer.md` and create role
> `r-engineer`. Create stream `s-engineer` with transport.kind
> `email` and config `{"alias":"engineer"}`. Position `p-engineer`
> under `p-root` with that role. Hire AI worker `w-lee` with
> identityContent from `./workers/lee.md`. Grant the tools listed
> in the role's `## Tools (MCP)` section.
>
> Then `worker_log` on each Worker (`w-sam`, `w-lee`) with
> `wait=60` until you see `=== exit: ok ===` to confirm they're
> subscribed and ready.

### 7. Send Sam an escalation-grade email

From your normal mail client (Gmail, etc.), send an email to your
`+sam` alias address:

> **To:** `<your-inbound-hash>+sam@inbound.postmarkapp.com`
> *(your hash is in `~/.helix/postmark`)*
>
> **Subject:** How does the email transport route inbound mail?
>
> Hi support — I'm trying to figure out how mail to my Postmark
> hash address actually finds the right helix-org Stream. Is the
> stream ID in the URL? In the address? Somewhere else?
>
> — Phil

This is a question Sam can't answer himself (it's about
helix-org internals), so he'll forward it to Lee.

### 8. Watch the four-hop cascade

Back in chat:

> Subscribe me to `s-support` and `s-engineer`. `read_events` with
> `wait=120` until I interrupt; print every event verbatim as it
> arrives.

You'll see, in order:

1. **Customer query** lands on `s-support` (`From: phil@…`,
   `Subject: How does the email transport…`).
2. **Sam's escalation** appears on `s-support`
   (`To: <hash>+engineer@…`, paraphrased question for Lee). Postmark
   sends it; their inbound webhook re-delivers it as…
3. **Sam's escalation arrives on `s-engineer`** (`From: phil@…`
   — Postmark renders our verified Sender Sig as From regardless
   of which Worker published — but the Subject and Body are Sam's).
4. **Lee's reply** appears on `s-engineer` (`To: <hash>+sam@…`,
   `Subject: [eng] Re: …`, technical answer signed `— Lee`).
   Postmark routes it back to…
5. **Lee's reply arrives on `s-support`** (`Subject: [eng] Re: …`).
   Sam reactivates, sees the `[eng]` prefix, walks `s-support`
   history by `ThreadID` to find the customer's original query,
   paraphrases Lee's answer.
6. **Sam's customer-facing reply** appears on `s-support`
   (`To: phil@winder.ai`, plain `Re: …` subject, paraphrased,
   signed `— Sam`). Postmark sends it. Your inbox lights up.

End-to-end ≈ 60–120 seconds (four claude activations: Sam,
Lee, Sam-again; cold-start dominates).

### 9. Stop

Ctrl-C terminals 1 and 2.

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
- **Workers as first-class email participants.** Sam emails Lee.
  Lee emails Sam. The customer emails Sam. All three legs use the
  same transport, the same envelope, and the same alias-based
  routing — Workers aren't a special case. Hiring a third
  participant (Robin in legal? alias `legal`?) is two new
  Streams + two new role files, no transport changes.
- **`ThreadID` as the conversation spine.** Sam's escalation
  carries the customer's `ThreadID`; Lee preserves it; Sam reads
  it back to find the original customer. The whole four-hop
  cascade is one logical thread despite four physical Postmark
  send/receive pairs.

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
