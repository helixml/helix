# Getting Started

Smallest end-to-end run of helix-org, against the version embedded
inside Helix. Bootstrap an Owner, hire an echo Worker, publish a
message, watch it reply, live-edit the Role. About 90 seconds.

You drive the org by typing into the chat surface at `/ui/`. The
embedded helix-org reuses one of the Helix agents under
`/orgs/<org>/agents` as the chat brain — pick one in the agent
picker once and everything else flows through it.

## Prerequisites

- A running Helix instance you can log into (e.g. `http://localhost:8080`).
- An account on that instance with the `helix-org` alpha feature flag
  granted. From a shell with DB access:

  ```sql
  UPDATE users SET alpha_features = array_append(alpha_features, 'helix-org')
  WHERE email = 'you@example.com';
  ```

- At least one Helix agent under `/orgs/<your-org>/agents` that can
  hold a chat conversation (any `helix_agent` or `zed_external` agent
  works — pick something you've already verified chats OK).

## 1. Open the alpha surface

Sign into Helix. Open the left-hand drawer; you'll see a new entry
labelled **helix-org (alpha)** between *Admin Panel* and *Account
Settings*. Click it. A new tab opens at `/ui/alpha-agents`.

## 2. Pick the chat brain

Choose any agent from the list and click **Use this agent**. The
picker confirms with "Saved." and the **In use** badge moves to your
pick. The choice is hot — the next chat message uses it; no
restart.

Open `/ui/` (linked at the bottom of the picker, or just trim the
path). That's the chat surface.

## 3. Hire an echo worker

Paste this into the chat textarea and send:

> Set up a small echo worker. Make a stream `s-general`. Define a
> role `r-echo` whose job is, on hire, to subscribe to `s-general`,
> and on each new event there, publish `echo: <body>`. Create a
> position for that role reporting to me, and hire an AI worker
> `w-echo` for it with grants to subscribe and publish. Then
> `worker_log` on `w-echo` with `wait=30` until you see
> `=== exit: ok ===` so I know the hire activation finished.

The agent calls the helix-org MCP tools (`create_stream`,
`create_role`, `create_position`, `hire_worker`, `grant_tool`,
`worker_log`) and reports back. The hire activation runs `w-echo`
once — it subscribes to `s-general`, finds nothing, and exits.
Subsequent events on the stream will respawn it automatically.

You can verify on the side at `/ui/org` (the chart should now show
`p-root` with `p-echo` underneath; click either node for detail).

## 4. Wake the worker

Send:

> Subscribe me to `s-general`. Publish `hello` there. Then
> `read_events` on `s-general` repeatedly with `wait=15` until you
> see both my `hello` and `w-echo`'s `echo: hello` reply (it takes
> the worker ~5–10s to wake and respond). Show me both.

You'll see two events come back: yours, then `w-echo`'s reply
~10 seconds later.

`/ui/streams` shows the same events laid out per-stream if you'd
rather watch from there.

## 5. Live-edit the role

Send:

> Tweak the `r-echo` role: instead of replying `echo: <body>`, it
> should shout `loud: <BODY UPPERCASED>` on each event.

`update_role` rewrites the role markdown in place. The change takes
effect on `w-echo`'s next activation — trigger one:

> publish `hello` on `s-general` again, then `read_events` with
> `wait=15` until you see `w-echo`'s reply.

The new behaviour shows up live: `loud: HELLO`.

## Cleaning up

Everything created here lives inside the embedded helix-org SQLite
file at `$FILESTORE_LOCALFS_PATH/helix-org/helix-org.db` on the
Helix host. To start over, stop the Helix API, delete that file
plus the `helix-org/envs/` directory next to it, and restart — the
next request re-bootstraps a fresh owner.

If you only want to reset the chat pick without wiping state, go
back to `/ui/alpha-agents` and pick a different agent.

---

The legacy standalone-binary version of this demo (running
`./bin/helix-org serve` + `./bin/helix-org chat`) is preserved in
git history; that flow still works for hacking on helix-org outside
of Helix, but the SaaS alpha runs it embedded as above.
