# Getting Started

Smallest end-to-end run of helix-org. You'll bootstrap an Owner, hire
one AI Worker that echoes events back, publish a message, watch the
Worker wake on it and reply, then live-edit the Role and watch the
Worker's behaviour change. About 90 seconds, two terminals.

The whole thing is driven by talking to `claude` directly. Bootstrap
registers the owner's MCP endpoint with your `claude` CLI, and from
then on you write a sentence to `claude` and it calls the helix tools
on your behalf.

## What this shows

- **Bootstrap** creates the Owner — the only Worker not hired through
  `hire_worker`. After this, every mutation goes through MCP.
- **MCP**: every Worker has its own endpoint at `/workers/{id}/mcp`
  exposing only the tools that Worker holds grants for. `claude`
  talks to the owner's endpoint via the entry bootstrap installed.
- **Role vs Identity**: the **Role** is the job (markdown content,
  owner-edited, fans out to every Worker filling it). The
  **Identity** is who the Worker is (per-hire markdown, immutable).
  The system stamps both into the Worker's environment as `role.md`
  and `identity.md`.
- **Spawning**: hiring an AI Worker writes a per-Worker `mcp.json`
  and runs `claude -p` against it. Each activation is one Claude
  turn, then exit.
- **Push dispatch**: when an event lands on a Stream a Worker
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

Bootstrap opens the SQLite store directly, so it needs the same
`--db` and `--envs-dir` you passed to `serve`. (It doesn't talk to
the running server — there's no Worker to dial yet.)

```bash
./bin/helix-org bootstrap \
  --db /tmp/helix-org-demo.db \
  --envs-dir /tmp/helix-org-envs \
  --install-claude-mcp
```

You now have `w-owner` with grants for every built-in tool. Their
Environment is at `/tmp/helix-org-envs/w-owner`. `--install-claude-mcp`
adds an `helix-org` MCP entry to your user-scope `claude` config
pointing at `http://localhost:8080/workers/w-owner/mcp`.

## 3. Set up an Echo Worker

One prompt — Claude turns it into the four MCP calls (stream, role,
position, hire):

```bash
claude -p --permission-mode bypassPermissions "Set up a small echo
worker. Make a stream called s-general. Define a role r-echo whose
job is, on hire, to subscribe to s-general, and on each new event
there, publish 'echo: <body>'. Create a position for that role
reporting to me, and hire an AI worker called w-echo for it with
grants to subscribe and publish."
```

Claude reports back what it did. In terminal 1 you'll see
`spawned claude … worker=w-echo trigger=hire`. In a third terminal,
ask `claude` to watch the room — the owner's MCP entry is already
registered, so plain `claude` works:

```bash
claude --permission-mode bypassPermissions "Subscribe me to every stream and tell me about each event as it lands. Use read_events with wait=60 and keep going until I interrupt."
```

Claude calls `subscribe` once per stream, then loops `read_events`,
streaming a one-line summary as each event lands. Narrow it to one
stream with "Subscribe me to s-general only…". To watch the worker's
*internal* claude output as well,
`tail -f /tmp/helix-org-envs/w-echo/activation.log` shows the raw
stream.

Within ~10 seconds the hire activation finishes: claude reads
`role.md` and `identity.md`, calls `subscribe` on `s-general`, exits.
The process is gone — Claude will be respawned when an event arrives.

## 4. Wake the Worker with an event

```bash
claude -p --permission-mode bypassPermissions "publish 'hello' on s-general"
```

In the watcher window you'll see two events land back-to-back: the
owner's `hello`, then the echo worker's reply.

## 5. Live-edit the Role

`update_role` rewrites `role.md` in every Worker's Environment that
holds this Role. Their next activation picks up the new content with
no redeploy.

```bash
claude -p --permission-mode bypassPermissions "Tweak the r-echo
role: instead of replying 'echo: <body>', it should shout 'loud:
<BODY UPPERCASED>' on each event."

cat /tmp/helix-org-envs/w-echo/role.md
```

The file on disk is now v2. Trigger another publish — the Worker
responds in the new style:

```bash
claude -p --permission-mode bypassPermissions "publish 'hello' on s-general"
```

The watcher window shows the new behaviour live: `w-echo: loud: HELLO`.

## 6. Stop

Ctrl-C terminal 1.

```bash
pkill -f 'claude -p' 2>/dev/null
```

---

## Acting as a different Worker

`--install-claude-mcp` only registers the owner's endpoint. To drive
the org as a different Worker, register that Worker's endpoint as a
separate MCP entry:

```bash
claude mcp add-json --scope user helix-echo \
  '{"type":"http","url":"http://localhost:8080/workers/w-echo/mcp"}'

claude -p --permission-mode bypassPermissions \
  --strict-mcp-config --mcp-config '{"mcpServers":{"helix-echo":{"type":"http","url":"http://localhost:8080/workers/w-echo/mcp"}}}' \
  "list the streams you're subscribed to"
```

Claude only sees the tools the named Worker holds grants for, so this
is how you experiment with restricted-capability Workers without
touching the owner.
