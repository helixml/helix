# Getting Started

Smallest end-to-end run of helix-org. You'll bootstrap an Owner, hire
one AI Worker that echoes events back, publish a message, watch the
Worker wake on it and reply, then live-edit the Role and watch the
Worker's behaviour change. About 90 seconds, two terminals.

The whole thing is driven via `helix-org prompt` — a thin CLI that
spawns a Claude Code instance pointed at your Worker's MCP endpoint.
You write a sentence; Claude calls the helix tools.

## What this shows

- **Bootstrap** creates the Owner — the only Worker not hired through
  `hire_worker`. After this, every mutation goes through MCP.
- **MCP**: every Worker has its own endpoint at `/workers/{id}/mcp`
  exposing only the tools that Worker holds grants for. `helix-org
  prompt` connects Claude to that endpoint.
- **Role vs Identity**: the **Role** is the job (markdown content,
  owner-edited, fans out to every Worker filling it). The
  **Identity** is who the Worker is (per-hire markdown, immutable).
  The system stamps both into the Worker's environment as `role.md`
  and `identity.md`.
- **Spawning**: hiring an AI Worker writes a per-Worker `mcp.json`
  and runs `claude -p` against it. Each activation is one Claude
  turn, then exit.
- **Push dispatch**: when an event lands on a Channel a Worker
  subscribes to, the runtime spawns a fresh Claude for it.

## Setup

```bash
cd /home/phil/helix/helix-org
make build
rm -rf /tmp/helix-org-envs /tmp/helix-org-demo.db
```

`/tmp/helix-org-envs` is where the server will create one
subdirectory per Worker.

## 1. Start the server (terminal 1)

```bash
./bin/helix-org serve \
  --db /tmp/helix-org-demo.db \
  --envs-dir /tmp/helix-org-envs
```

Leave it running. Every HTTP request and every spawn lands here.

## 2. Bootstrap the Owner (terminal 2)

```bash
./bin/helix-org bootstrap
```

You now have `w-owner` with grants for every structural tool. Their
Environment is at `/tmp/helix-org-envs/w-owner`.

## 3. Set up an Echo Worker

One prompt — Claude turns it into the four MCP calls (channel, role,
position, hire):

```bash
./bin/helix-org prompt "Set up a small echo worker. Make a channel
called c-general. Define a role r-echo whose job is, on hire, to
subscribe to c-general, and on each new event there, publish
'echo: <body>'. Create a position for that role reporting to me, and
hire an AI worker called w-echo for it with grants to subscribe and
publish."
```

Claude reports back what it did. In terminal 1 you'll see
`spawned claude … worker=w-echo trigger=hire`. In a third terminal,
start watching every channel — this is your live view of the org:

```bash
./bin/helix-org tail
```

(Use `tail c-general` for just the one channel, or `tail 'c-*'` for
all c-prefixed channels. To watch the worker's *internal* claude
output as well, `tail -f /tmp/helix-org-envs/w-echo/activation.log`
shows the raw stream.)

Within ~10 seconds the hire activation finishes: claude reads
`role.md` and `identity.md`, calls `subscribe` on `c-general`, exits.
The process is gone — Claude will be respawned when an event arrives.

## 4. Wake the Worker with an event

```bash
./bin/helix-org prompt "publish 'hello' on c-general"
```

In the `tail` window you'll see two events land back-to-back: the
owner's `hello`, then the echo worker's reply.

```
HH:MM:SS  c-general  w-owner  hello
HH:MM:SS  c-general  w-echo   echo: hello
```

## 5. Live-edit the Role

`update_role` rewrites `role.md` in every Worker's Environment that
holds this Role. Their next activation picks up the new content with
no redeploy.

```bash
./bin/helix-org prompt "Tweak the r-echo role: instead of replying
'echo: <body>', it should shout 'loud: <BODY UPPERCASED>' on each
event."

cat /tmp/helix-org-envs/w-echo/role.md
```

The file on disk is now v2. Trigger another publish — the Worker
responds in the new style:

```bash
./bin/helix-org prompt "publish 'hello' on c-general"
```

The `tail` window shows the new behaviour live: `w-echo: loud: HELLO`.

## 6. Stop

Ctrl-C terminal 1.

```bash
pkill -f 'claude -p' 2>/dev/null
```

---

## Acting as a different Worker

`helix-org prompt` defaults to `--as w-owner`. To act as another
Worker, pass `--as <workerId>`:

```bash
./bin/helix-org prompt --as w-echo "list the channels you're subscribed to"
```

Claude only sees the tools the named Worker holds grants for, so this
is also how you experiment with restricted-capability Workers without
touching the owner.
