# Webhook

Webhooks as a Stream Transport, in both directions. Curl a URL and an
Event lands on the inbound Stream; an Event appended to an outbound
Stream becomes an HTTP POST to a configured target. This demo wires a
one-Worker secretary that summarises whatever payload you POST into
`s-inbox`, DMs the summary back, and forwards it to `s-outbox` —
which fires an outbound POST to a catcher you control.

About 2 minutes.

## How it differs from the other demos

The other demos are "internal": every Stream uses `transport: local`,
events come from Workers calling `publish` (or `dm`). Here `s-inbox`
has `transport: webhook` (inbound) and `s-outbox` has `transport:
webhook` with an `outbound_url` (outbound). The Streams are otherwise
normal — same Subscriptions, same dispatch, same `read_events`. The
two new things are an HTTP path that turns POSTs into Events on a
Stream, and an emitter that turns Events on a Stream into POSTs.

## Setup

```bash
cd /home/phil/helix/helix-org
make build
rm -rf /tmp/helix-webhook-envs /tmp/helix-webhook.db
```

## 1. Start the server (terminal 1)

```bash
cd demos/webhook
../../bin/helix-org serve --db /tmp/helix-webhook.db --envs-dir /tmp/helix-webhook-envs
```

## 2. Start an outbound catcher (terminal 2)

Anything that accepts POSTs and shows the body works. The simplest
local option:

```bash
nc -lk 9000
```

`nc` won't reply with a proper HTTP response, so the outbound emitter
will time out (5s) and log a warning — but the request body still
hits the listener, which is all we care about for the demo. Swap in
[webhook.site](https://webhook.site/) or any other catcher if you
prefer.

## 3. Bootstrap and open a chat (terminal 3)

```bash
cd demos/webhook
../../bin/helix-org bootstrap --db /tmp/helix-webhook.db --envs-dir /tmp/helix-webhook-envs
../../bin/helix-org chat --new
```

## 4. Hire the secretary

> Set up a secretary. Read `./roles/secretary.md` and create role
> `r-secretary` from it. Create Stream `s-inbox` with `transport:
> webhook` (inbound — no config). Create Stream `s-outbox` with
> `transport: webhook` and config
> `{"outbound_url": "http://localhost:9000"}` (outbound). Check the
> `create_stream` schema for the exact shape. Create Position
> `p-secretary` under `p-root` with that role. Hire AI worker
> `w-secretary` into it; identity is "You are the secretary." Grant
> `subscribe`, `dm`, and `publish`. Then `worker_log` on
> `w-secretary` until you see `=== exit: ok ===`.

Once the chat reports the secretary is alive, the inbound webhook is
live at `http://localhost:8080/webhooks/s-inbox` (the URL path is the
Stream's ID, no separate token), and `s-outbox` is wired to POST to
`http://localhost:9000`.

## 5. POST a payload

In a fourth terminal:

```bash
curl -X POST http://localhost:8080/webhooks/s-inbox \
  -H 'Content-Type: text/plain' \
  --data 'Mistral released a new 3B model this morning. Benchmarks beat Phi-3 on reasoning while fitting in a single 4090. Open weights, Apache 2.0.'
```

Back in chat:

> `read_events` on `s-outbox` with `wait=30` until you see the
> secretary's summary land. Show me.

The cascade: webhook handler appends the payload as an Event on
`s-inbox` → dispatcher wakes the secretary → the secretary
summarises → publishes the summary to `s-outbox` and DMs me → the
outbound emitter POSTs the summary to `localhost:9000`, which
terminal 2 prints.

## 6. Stop

Ctrl-C terminals 1 and 2.

## What this shows

- A Transport is a per-Stream choice, not a system mode. `s-inbox`
  is inbound webhook, `s-outbox` is outbound webhook, the secretary's
  DM back to the owner is on a `local` stream. They mix freely.
- The webhook handler and outbound emitter are glue. Once an Event
  lands on a Stream — whether from a curl, a `publish` call, or a
  `dm` — *everything* downstream is the existing local machinery.
- A single Stream can be inbound *or* outbound (or both, with
  `outbound_url` set on a stream that also accepts POSTs to its
  inbound path) — the dispatcher fires on every append regardless of
  origin, so a webhook stream can be a one-way relay or a full
  bidirectional bridge.

## What this doesn't cover (yet)

- **Auth.** The inbound URL exposes the Stream ID and nothing else —
  anyone who knows or guesses it can post. Outbound POSTs are
  unsigned. Production would want HMAC signatures, a bearer header,
  or a separate signing secret on the Stream's transport config.
- **Headers, query string, content-type.** The handler currently
  publishes only the request body, and the emitter currently sends
  only the event body. Wrapping the full request in a structured
  envelope is a small extension.
- **Retries on outbound failure.** A 5xx or timeout is logged and
  dropped. A small retry-with-backoff would belong here.
