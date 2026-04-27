# Getting Started

Smallest end-to-end run of helix-org. Bootstrap an Owner, hire an
echo Worker, publish a message, watch it reply, live-edit the Role.
About 90 seconds.

You drive the org by typing into a `helix-org chat` session — that
exec's `claude` against the owner's MCP endpoint. Same flow a chat
UI on a real server would give you: connect, type, the conversation
persists.

## Setup

```bash
cd /home/phil/helix/helix-org
make build
rm -rf /tmp/helix-org-envs /tmp/helix-org-demo.db
```

## 1. Start the server (terminal 1)

```bash
./bin/helix-org serve --db /tmp/helix-org-demo.db --envs-dir /tmp/helix-org-envs
```

## 2. Bootstrap and open a chat (terminal 2)

```bash
./bin/helix-org bootstrap --db /tmp/helix-org-demo.db --envs-dir /tmp/helix-org-envs
./bin/helix-org chat
```

You're now in a chat session as `w-owner`. Everything below is
typed into this chat.

## 3. Hire an echo worker

> Set up a small echo worker. Make a stream `s-general`. Define a
> role `r-echo` whose job is, on hire, to subscribe to `s-general`,
> and on each new event there, publish `echo: <body>`. Create a
> position for that role reporting to me, and hire an AI worker
> `w-echo` for it with grants to subscribe and publish.

In terminal 1 you'll see `spawned claude … worker=w-echo
trigger=hire`. ~10 seconds later the hire activation finishes:
`w-echo` reads `role.md` and `identity.md`, calls `subscribe`, and
exits. It will be respawned when an event arrives.

## 4. Wake the worker

> Subscribe me to `s-general`. Publish `hello` there. Then
> `read_events` on `s-general` repeatedly with `wait=15` until you
> see both my `hello` and `w-echo`'s `echo: hello` reply (it takes
> the worker ~5–10s to wake and respond). Show me both.

## 5. Live-edit the role

> Tweak the `r-echo` role: instead of replying `echo: <body>`, it
> should shout `loud: <BODY UPPERCASED>` on each event.

`update_role` rewrites `role.md` in `w-echo`'s Environment in place.
Trigger another publish:

> publish `hello` on s-general again, then `read_events` with
> `wait=15` until you see w-echo's reply.

The new behaviour shows up live: `loud: HELLO`.

## 6. Stop

Ctrl-C terminal 1.
