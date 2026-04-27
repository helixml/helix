# Webhook

Inbound webhooks as a Stream Transport. Curl a URL, an Event lands
on the Stream, any Worker subscribed to it reacts. This demo wires a
one-Worker secretary that summarises whatever payload you POST and
DMs the summary back.

About 90 seconds.

## How it differs from the other demos

The other demos are "internal": every Stream uses `transport: local`,
events come from Workers calling `publish` (or `dm`). Here `s-inbox`
has `transport: webhook`. The Stream is otherwise normal — same
Subscriptions, same dispatch, same `read_events`. The only new thing
is an HTTP path that turns POSTs into Events on the Stream.

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

## 2. Bootstrap and open a chat (terminal 2)

```bash
cd demos/webhook
../../bin/helix-org bootstrap --db /tmp/helix-webhook.db --envs-dir /tmp/helix-webhook-envs
../../bin/helix-org chat --new
```

## 3. Hire the secretary

> Set up a secretary. Read `./roles/secretary.md` and create role
> `r-secretary` from it. Create a Stream `s-inbox` whose transport
> is a webhook — check the `create_stream` tool schema for the
> exact shape. Create Position `p-secretary` under `p-root` with
> that role. Hire AI worker `w-secretary` into it; identity is
> "You are the secretary." Grant `subscribe` and `dm`. Then
> `worker_log` on `w-secretary` until you see `=== exit: ok ===`.

Once the chat reports the secretary is alive, the webhook is live
at `http://localhost:8080/webhooks/s-inbox` — the URL path is the
Stream's ID, no separate token.

## 4. POST a payload

In a third terminal:

```bash
curl -X POST http://localhost:8080/webhooks/s-inbox \
  -H 'Content-Type: text/plain' \
  --data 'Mistral released a new 3B model this morning. Benchmarks beat Phi-3 on reasoning while fitting in a single 4090. Open weights, Apache 2.0.'
```

Back in chat:

> `read_events` with `wait=30` until I see the secretary's DM. Show me.

The cascade: webhook handler appends the payload as an Event on
`s-inbox` → dispatcher wakes the secretary → the secretary
summarises → calls `dm` to me → I see it land. ~5 seconds end to
end.

## 5. Stop

Ctrl-C terminal 1.

## What this shows

- A Transport is a per-Stream choice, not a system mode. `s-inbox`
  is a webhook stream; the secretary's DM back to the owner is on a
  `local` stream. They mix freely.
- The webhook handler is glue. Once an Event lands on the Stream,
  *everything* downstream — subscriptions, dispatch, the
  secretary's prompt — is the existing local machinery.

## What this doesn't cover (yet)

- **Outbound webhooks** — Events on the Stream POSTed to an external
  URL. Needs a separate transport (or a `direction: in|out|both`
  config on the same one).
- **Auth.** The URL exposes the Stream ID and nothing else — anyone
  who knows or guesses it can post. Production would want HMAC
  signatures, a bearer header, or a separate signing secret on the
  Stream's transport config.
- **Headers, query string, content-type.** The handler currently
  publishes only the request body. Wrapping the full request in a
  structured payload (JSON envelope) is a small extension.
