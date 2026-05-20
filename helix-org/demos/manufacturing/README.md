# Manufacturing — NCR Triage

A factory-floor demo. An operator raises a Non-Conformance Report on
a tablet; in 60–90 seconds, three channels light up with a
containment plan, customer apology drafts, and a held supplier
email. The supervisor approves on Slack. The agent confirms back.
The audience watches humans make three decisions instead of chasing
data across seven systems.

About 8 minutes start-to-finish, including the pitch and outcome
slide. The active demo on stage is ~2 minutes of cascade.

> **Read this whole README before going on stage.** There is one
> stable script. Stick to it. Ad-libbing in front of a crowd is how
> demos die.

## What the audience sees

- **Two browser windows tiled side-by-side**:
  1. The mock-channels phone view at `http://localhost:7765/` —
     looks like a phone with three tabs: Email, Slack, SMS.
  2. (Optional) A slide with the closing numbers — 36 hours vs ~90
     seconds. Switch to it for the closing line.
- **One curl** that simulates the operator tapping "Raise NCR" on
  the shop-floor tablet (drive it from a small terminal off-screen
  or a separate browser tab).
- **One Slack-style reply** that you click into and type in the
  mock-channels UI.

That's the whole show.

## Why this is hard to mess up

- No real Slack/SMS/email accounts. The
  [`comms-demo`](https://github.com/helixml/comms-demo) container
  pretends to be all three.
- No external data sources. The "enrichment" data (SPC, maintenance
  log, related NCRs, supplier history, affected orders) is baked
  into the agent's role file. The agent never reaches out to
  anything.
- One agent, one role file. Two activations: NCR raised → fan out;
  supervisor reply → confirm.
- Three channels, one per kind, matching the comms-demo `seed` CLI
  shape exactly.

If a step misbehaves on stage, look at **Recovery** at the bottom —
every failure mode here has a one-line fix.

## Prerequisites

Run helix-org against a live Helix instance (production-shape
sandbox spawning + chat). For this demo:

- A Helix server you can reach (e.g. `https://app.helix.ml`) and an
  API key on it.
- A public URL for *your* helix-org so the in-sandbox agent can
  call back into MCP. `cloudflared tunnel --url http://localhost:8080`
  is the simplest option; ngrok works too.
- Docker (for the mock-channels container).
- `jq` and `curl` for the setup commands below.

(A pure-local run with the `claude` spawner is possible too — set
`spawner.kind=claude` and `chat.backend=claude` instead — but the
"on stage" beats below assume the Helix path because that's what
gets demoed.)

## Pre-flight checklist (do this 10 minutes before going live)

Run the whole demo once end-to-end on the actual machine you'll
present from. Do not assume yesterday's run will work today.

```bash
# 1. helix-org binary built
cd /home/phil/helix/helix-org
make build

# 2. comms-demo container pullable and starts cleanly
docker pull ghcr.io/helixml/comms-demo:main

# 3. Fresh demo state
rm -rf /tmp/manufacturing-envs /tmp/manufacturing.db /tmp/manufacturing-mock
mkdir -p /tmp/manufacturing-mock && chmod 777 /tmp/manufacturing-mock

# 4. Tunnel binary on path (or use ngrok)
cloudflared --version || echo "install cloudflared first"
```

If any of these fail, **fix them now**, not on stage.

## One-time setup (≤ 5 minutes)

### 1. Open a public tunnel to localhost:8080 (terminal 1)

```bash
cloudflared tunnel --url http://localhost:8080
```

Note the `https://*.trycloudflare.com` URL it prints. Export it; the
helix-org config below needs it.

```bash
export CF_URL=https://your-tunnel.trycloudflare.com
export HELIX_URL=https://app.helix.ml
export HELIX_API_KEY=hl-your-key-here
```

### 2. Configure helix-org for the Helix backend (terminal 2)

```bash
cd /home/phil/helix/helix-org
./bin/helix-org config set --db /tmp/manufacturing.db spawner.kind '"helix"'
./bin/helix-org config set --db /tmp/manufacturing.db chat.backend '"helix"'
./bin/helix-org config set --db /tmp/manufacturing.db helix.url "\"$HELIX_URL\""
./bin/helix-org config set --db /tmp/manufacturing.db helix.api_key "\"$HELIX_API_KEY\""
./bin/helix-org config set --db /tmp/manufacturing.db helix.org_url "\"$CF_URL\""
```

### 3. Start the helix-org server (terminal 2)

```bash
./bin/helix-org serve \
  --db /tmp/manufacturing.db \
  --envs-dir /tmp/manufacturing-envs
```

You should see a `spawner: helix` line and a `server listening
addr=:8080` line. Leave it running.

### 4. Start mock-channels (terminal 3)

```bash
docker run -d --rm --name mfg-mock --network host \
  -v /tmp/manufacturing-mock:/data \
  ghcr.io/helixml/comms-demo:main \
  serve --addr :7765 --db /data/mock-channels.db
```

Open `http://localhost:7765/` in **browser tab #1**. You should see
the empty phone view. Leave it open — you'll watch messages stream
in here.

### 5. Seed the three mock channels (terminal 3)

The comms-demo `seed` command creates one channel per kind
(email/slack/sms) and points each at a Helix stream ID:

```bash
docker exec mfg-mock mock-channels seed \
  --db /data/mock-channels.db \
  --helix-base http://localhost:8080 \
  --email-stream s-supplier \
  --slack-stream s-supervisor \
  --sms-stream   s-customers
```

This creates channels `email-main`, `slack-general`, `sms-main` —
those are the channel IDs you'll use in `outbound_url` below.

### 6. Hire the quality bot (terminal 4)

```bash
cd /home/phil/helix/helix-org
./bin/helix-org chat --new
```

> **Always pass `--new`** when you've rebuilt the binary or upgraded
> helix-org. The chat-driving claude caches MCP tool schemas at the
> start of a session — without `--new` it'll keep using stale
> definitions (missing enum constraints, outdated descriptions) even
> though the server has fresh ones. `--new` forces a clean session
> and a fresh `tools/list`.

Paste **the entire block below** (everything between the lines that
read `BEGIN` and `END`) into the chat as one message. It contains
the role markdown inline so the agent doesn't need to read any
files — that matters because the chat backend runs the agent inside
a Helix sandbox that doesn't have this repo checked out. If you ask
it to "read ./roles/quality-bot.md", it'll wander off into kodit /
curl / ls trying to find a file that isn't there.

The agent should use only the helix-org MCP tools (`create_role`,
`create_stream`, `create_position`, `hire_worker`, `worker_log`).
The prompt makes that explicit.

```
=== BEGIN ===
Set up the manufacturing demo. Use ONLY the helix-org MCP tools
(create_role, create_stream, create_position, hire_worker,
worker_log). Do NOT read files from the filesystem, do NOT use
kodit, do NOT curl any URLs — the role content is inlined below.

Step 1. Call `create_role` with id `r-quality-bot` and content set
to exactly this markdown:

# Quality Bot

You are the on-call quality coordinator for a packaged-goods plant.
When a Non-Conformance Report (NCR) is raised on the shop floor you
turn it into a containment plan, fan out to every channel that
needs to act, and wait for the production supervisor's approval
before confirming anything. You don't make judgement calls — you
assemble evidence, propose actions, and route decisions to humans.

## Streams

- `s-ncr-raised` — inbound webhook. Subscribe on hire.
- `s-supervisor` — Slack DM channel for the production supervisor.
  Bidirectional. Subscribe on hire — the supervisor's reply
  triggers your second activation.
- `s-customers` — SMS channel reaching account managers. Outbound
  only.
- `s-supplier` — email channel to the raw-material supplier's QA
  desk. Outbound only. Held by default — only publish here when
  the supervisor's reply contains the exact phrase `implicate
  supplier`. (Anything else, including `supplier ok` /
  `supplier cleared`, leaves the email killed.)

## Reference data (mocked for the demo — use verbatim)

- Plant: Lincoln Line 3 (powder fill, 50 g sachets).
- Recent SPC: weight has drifted 1.4 g light over the last 8 hours.
  Spec is 50.0 g ± 1.5 g.
- Maintenance log: dosing valve V-3-2 last serviced 11 weeks ago,
  scheduled service is at 12 weeks (one week away).
- Related NCRs (last 12 months): two prior on V-3-2, both
  weight-light, both closed with valve recalibration.
- Active raw-material lot: WX-2207 from supplier Marston Powders.
  Last 6 lots all in spec.
- Affected orders if batch 24-1107 is quarantined: PO-5512 (Acme
  Foods, due Thursday) and PO-5520 (Brightline, due Friday). Both
  can be filled from Line 4 with a 4-hour delay.

## Triggers

### On hire
Subscribe to `s-ncr-raised` and `s-supervisor`. Exit.

### On a new event on `s-ncr-raised`
1. Publish to `s-supervisor`: ≤ 8 lines, lead with bold
   recommendation, cover batch ID, suspected cause (valve drift),
   proposed split (quarantine 24-1107, reroute to Line 4), note
   maintenance work order queued for V-3-2. End with: Reply
   'approve' to confirm; add 'implicate supplier' if lot WX-2207
   is at fault.
2. Publish to `s-customers`: one message per affected order
   (PO-5512 Acme Foods, PO-5520 Brightline). ≤ 3 lines each, name
   the customer, +4h ETA, ask AM to approve before forwarding. Set
   `to` to a single-element array like `["acme-am"]`.
3. Do NOT publish to `s-supplier` yet — that's held pending
   engineer review.
Exit.

### On a new event on `s-supervisor`
Read `Message.Body`. Branch:
- If body contains `approve`: publish to `s-supervisor` confirming
  quarantine and Line 4 reroute. Sign `— Quality Bot`.
  - If body ALSO contains the exact phrase `implicate supplier`:
    publish to `s-supplier` a polite email asking Marston Powders
    QA to review lot WX-2207 (subject:
    `NCR 24-1107 — lot WX-2207 review request`). Mention in the
    supervisor reply that supplier email has gone out.
  - Otherwise (e.g. `supplier ok`, `supplier cleared`, or no
    mention of supplier at all): do NOT publish to `s-supplier` —
    supplier is cleared. Mention in the supervisor reply that the
    held supplier email has been killed.
Exit.

## Tools (MCP)
- subscribe
- publish

## Style
Short sentences. Lead with the verb or the number. No hedging.
Sign outbound messages `— Quality Bot` (except the short SMS
drafts).

Step 2. Create four streams (all webhook transport, id and name set
to the stream name):
- `s-ncr-raised` — inbound only (no outbound URL).
- `s-supervisor` — outbound URL
  `http://localhost:7765/in/slack-general`.
- `s-customers` — outbound URL
  `http://localhost:7765/in/sms-main`.
- `s-supplier` — outbound URL
  `http://localhost:7765/in/email-main`.

Step 3. Create position `p-quality` under `p-root` with role
`r-quality-bot`.

Step 4. Hire AI worker `w-quality-bot` into `p-quality`. Identity:
"You are Quality Bot, the on-call NCR coordinator at Lincoln
Plant." Grant `subscribe` and `publish`.

Step 5. Call `worker_log` on `w-quality-bot` with `wait=180`. The
first activation against Helix takes 60–120 s as the sandbox
cold-starts. Report when it finishes.
=== END ===
```

When the chat says the hire is done, `http://localhost:8080/webhooks/s-ncr-raised`
is live. **Smoke-test it before going on stage:**

```bash
curl -sS -o /dev/null -w '%{http_code}\n' -X POST \
  http://localhost:8080/webhooks/s-ncr-raised \
  -H 'Content-Type: application/json' -d '{"body":"smoke"}'
```

You must see `200`. If you see `404` with body
`stream "s-ncr-raised" is not a webhook stream`, the chat agent
created `s-ncr-raised` with the default `local` transport instead of
`webhook` — go back to the chat and recreate it with
`{"id":"s-ncr-raised","name":"s-ncr-raised","transport":{"kind":"webhook"}}`.

(The smoke event lands in `s-ncr-raised` and triggers a real bot
activation. That's fine — discard it before showtime by restarting
helix-org's `serve` process; in-flight activations are interruptible
and the next NCR starts a clean cascade.)

**Now you are ready to demo.**

## On stage

### Beat 0 — the pitch (30 seconds, do not skip)

Read this verbatim. The numbers do the work; don't paraphrase.

> Line 3 just produced a batch where 4% of units failed the in-line
> weight check. Normally that triggers a two-day paper trail
> involving production, quality, engineering, and the supplier.
> Watch the stream do the legwork.

### Beat 1 — the operator raises the NCR (10 seconds)

Switch to the small terminal. Run:

```bash
curl -sS -X POST http://localhost:8080/webhooks/s-ncr-raised \
  -H 'Content-Type: application/json' \
  -d '{
    "from": "operator-rosa",
    "subject": "NCR — batch 24-1107, weights light",
    "body": "Batch 24-1107, weights running light, started about an hour ago, looks like the dosing valve."
  }'
```

While you press enter:

> "Rosa on Line 3 just dictated a 15-second voice note into the
> tablet. That curl is the tablet POSTing the transcribed NCR."

### Beat 2 — the cascade (30–60 seconds)

Switch to the mock-channels browser tab. Within ~30–60 seconds you
should see:

1. **Slack DM (slack-general → w-quality-bot)** — quarantine
   recommendation, reroute to Line 4, mention of the queued valve
   work order, ending with `Reply 'approve' to confirm containment;
   add 'supplier' if you think lot WX-2207 is at fault.`
2. **SMS (sms-main)** — two drafts, one per affected order
   (Acme PO-5512, Brightline PO-5520), each addressed to its
   account manager, asking for AM approval before forwarding.
3. **Email (email-main)** — *no message*. Point this out:

> "Notice nothing in the supplier email pane. The agent drafted it
> and held it. Sending the supplier a complaint before engineering
> has confirmed the cause is exactly the kind of mistake we want
> humans to be the ones not to make."

### Beat 3 — the supervisor decides (15 seconds)

Click into the slack-general thread in the mock-channels UI. Click
the reply box. Type — verbatim, including the lower-case:

```
approve, valve drift confirmed by engineering, supplier ok
```

Press send. Out loud:

> "That's three decisions in one Slack reply: approve containment,
> mark the root cause, clear the supplier. Ten seconds of judgement
> work."

### Beat 4 — the agent closes the loop (30–60 seconds)

A new thread appears in slack-general with a confirmation:
quarantine in motion, both POs rerouted to Line 4, supplier email
killed because the supervisor cleared the supplier.

The email-supplier pane stays empty. **That's the win.**

### Beat 5 — the close (30 seconds)

Switch to the closing slide (or just say it).

> "Traditional NCR cycle time on a defect like this is 36 hours,
> mostly waiting on the data. We just hit containment in under two
> minutes. The CAPA closes when maintenance signs off the valve
> service — call it 4 hours.
>
> Notice what the humans did and didn't do. They didn't gather
> evidence, they didn't draft documents, they didn't chase
> suppliers. They made three decisions. That's the split we're
> after."

Stop here. Do not start a Q&A live demo.

## Recovery — failure modes and one-line fixes

| Symptom | Cause | Fix |
|---|---|---|
| `curl` returns `404 stream "s-ncr-raised" is not a webhook stream` | Stream was created with the default `local` transport. (Should be impossible on a recent binary — the `create_stream` schema now enums the valid kinds. If you see this, you're on a stale chat session with cached schemas — restart `chat --new`.) | In chat: `create_stream` with `{"id":"s-ncr-raised","name":"s-ncr-raised","transport":{"kind":"webhook"}}` — re-creating overwrites. Then re-run the smoke test. |
| `curl` returns `404 stream not found` | Stream `id` wasn't set on create (got an auto-UUID instead). | In chat: `list_streams`. If `s-ncr-raised` is missing or shows a UUID id, recreate it with `id="s-ncr-raised"` AND `name="s-ncr-raised"`. |
| Slack pane empty after curl | mock-channels not reachable from helix-org. | `docker ps` for `mfg-mock`; confirm port 7765 is free; container started with `--network host`. |
| Hire takes > 3 minutes | Helix sandbox cold-start. | Wait it out. The second activation reuses the warm session and is much faster. |
| Cascade hits `tool_error: stream "s-X": record not found` | Role file mentions a stream you didn't create. | The role only references `s-ncr-raised`, `s-supervisor`, `s-customers`, `s-supplier`. If the agent invented one, that's a model hallucination — re-issue the hire prompt verbatim. |
| Reply on slack-general doesn't trigger Beat 4 | mock-channels can't reach helix-org at `--helix-base`. | Confirm the seed used `--helix-base http://localhost:8080` and that the container is on `--network host`. |
| Agent posts to email-main in Beat 2 | Role-file drift. | Re-read `roles/quality-bot.md` — the held-by-default rule lives in the "Triggers" section. |

If something goes catastrophically wrong on stage: **don't debug
live**. Cut to the closing slide, deliver Beat 5 verbatim, and offer
to walk anyone through the live system in the hallway.

## Resetting between runs

```bash
# In the helix-org terminal, Ctrl-C the server.
docker stop mfg-mock 2>/dev/null
rm -rf /tmp/manufacturing-envs /tmp/manufacturing.db /tmp/manufacturing-mock
```

Then redo "One-time setup" from step 1. The whole reset takes
under 3 minutes; if you're presenting twice in one day, do a fresh
reset between runs — flake-resistant trumps clever.

## What this demo shows

- **The agent is glue, not a decision-maker.** Every action with
  external consequences (quarantine, supplier complaint, customer
  notification) waits on a human. The agent's value is the minute
  of evidence-gathering and drafting that used to take a day.
- **Channels are interchangeable.** Slack, SMS, and email are the
  same `domain.Message` envelope going through the same webhook
  transport. Swapping `mock-channels` for real Slack / Twilio /
  Postmark is a config change, not a rewrite.
- **The hold pattern.** The supplier email is drafted but not sent.
  This is the cleanest illustration of "social enforcement plus
  one human gate" — the agent could send it, but its role text says
  not to until the supervisor's reply contains the trigger word.

## What this demo deliberately leaves out

- Real voice transcription / photo attachments. The curl body is
  the transcript; pretend the photo is in the NCR record.
- Real MES / SAP / CMMS integration. The reference data is in the
  role file. A production install would replace the "Reference
  data" section with tools that fetch the same data live.
- Multi-line plants, multi-batch genealogy, real CAPA tracking. One
  line, one batch, one valve. Every extension is additive.
- Authentication on the inbound webhook. Anyone who can reach
  `:8080` can post an NCR. Production would HMAC-sign or token-gate
  the URL.
