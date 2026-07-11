---
name: helix-org-cli
description: Use the Helix CLI to operate helix-org (bots, topics, processors, chat) and raw REST via helix api. Use when listing/managing org bots, chatting with chief-of-staff or other bots, or calling any authenticated Helix API path.
---

# Helix-org CLI

Auth (required):

```bash
export HELIX_URL=http://localhost:8080   # or your control plane
export HELIX_API_KEY=hl-…                # user API key (never the runner token)
export HELIX_ORG=unmanned-org            # optional default org (name or id)
```

Build (from repo root):

```bash
cd api && CGO_ENABLED=0 go build -o /tmp/helix-bin .
```

## Org graph commands

```bash
# Bots
helix org bots list [--org NAME]
helix org bots get <bot-id> [--org NAME]
helix org bots start|stop|restart <bot-id> [--org NAME]
helix org bots chat <bot-id> "message…" [--org NAME] [--timeout 300] [--no-start] [--session ses_…]

# Topics / processors
helix org topics list [--org NAME]
helix org processors list [--org NAME]
```

`bots chat` resolves the bot’s project exploratory session (starts the agent if needed), sends the message to `/sessions/{id}/chat`, and prints the assistant reply.

## Escape hatch — `helix api` (like `gh api`)

```bash
helix api /orgs/unmanned-org/bots
helix api -X POST /orgs/unmanned-org/bots/chief-of-staff/activate
helix api -X POST /orgs/unmanned-org/bots/b-mason/stop-agent
helix api --input '{"…":…}' -X POST /sessions/chat
echo '{"message":"hi"}' | helix api -X POST /sessions/ses_…/chat --input -
```

Path may be `/orgs/…`, `/api/v1/orgs/…`, or a full URL. Always authenticated with `HELIX_API_KEY`.

## Typical agent workflow

1. `helix org bots list --org unmanned-org` — see who is running.
2. `helix org bots chat chief-of-staff --org unmanned-org "…"` — talk to CoS.
3. If a first-class command is missing: `helix api GET /orgs/unmanned-org/…`.

## Notes

- Prefer org **name** (`unmanned-org`) or full id (`org_…`); both resolve.
- Do **not** use `oh-hallo-insecure-token` (runner token).
- CoS tools (`list_bot_repositories`, etc.) are MCP tools the bot calls inside its session; the CLI chats with the bot session, it does not expose those MCP tools as CLI verbs (use `helix api` or chat the bot to invoke them).
