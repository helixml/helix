# Config: org graph vs operational config

**Status:** draft, pre-implementation. Resolves where transport
credentials and other operational settings live; `messages.md`
points here for that decision.

## Goal

Two access patterns coexist in helix-org's SQLite database, with
two different mutation paths and two different audiences:

1. **Org graph** — roles, workers, positions, streams,
   subscriptions, grants, events. Mutated through MCP tools by
   Workers (and by the human owner via chat). High frequency,
   prompt-driven, never secret.

2. **Operational config** — transport credentials, Claude model
   selection, public URLs, spawner timeouts, future LLM provider
   settings. Mutated through a CLI (`helix-org config …`) by the
   human operator. Low frequency, secret-bearing, **never goes
   near an LLM**.

Both live in the same SQLite file. Different access paths because
the audiences are different and the threat models are different.
This doc covers the second.

## Non-goals

- Encryption at rest. Secrets land in the SQLite file in plaintext;
  the threat model is "your laptop / server box" and OS access
  controls do the work. Production deployment will need SQLCipher
  or per-row encryption with a master key from env. Out of scope.
- Multi-tenancy. One installation, one config. Multi-tenant orgs
  are a future concern with their own scoping problems.
- Authenticated remote admin. Phase 1 has the operator on the same
  host as the DB file. An HTTP admin endpoint with token auth is
  a follow-on; the CLI surface defined here stays the same when
  that lands (CLI becomes a thin client over HTTP).
- Anything mutable from chat. The whole point is *not* via MCP.

## What lives where

| Lives in DB | Mutation path | Examples |
|-------------|---------------|----------|
| Org graph | MCP tools (chat-driven) | `create_role`, `hire_worker`, `publish` |
| Operational config | `helix-org config` CLI | Postmark token, claude model, public URL |
| Bootstrap config | CLI flags / env on `serve` | DB path, listen addr, envs dir |

The bootstrap flags are the irreducible minimum — things the
server has to know *before* it can open the DB or start serving
MCP. Everything else moves into the `configs` table.

## Schema

```sql
configs (
  key         TEXT PRIMARY KEY,    -- "transport.postmark", "claude.model"
  value       TEXT NOT NULL,        -- JSON: string/number/object — whatever the consumer expects
  updated_at  TIMESTAMP NOT NULL,
  updated_by  TEXT                  -- WorkerID; reserved for audit when auth lands
)
```

Flat keys with dot-namespacing — simpler than a (namespace, key)
composite, and `list --prefix` covers the scoping use case.
Subsystems own their prefix:

- `claude.*` — claude CLI binary, model, public URL
- `transport.<provider>` — provider-specific config
- `dispatcher.*` — activation timeouts, concurrency limits
- (future) `llm.*` — LLM provider/model selection
- (future) `auth.*` — authentication configuration

## CLI surface

```bash
helix-org config set <key> <value>
helix-org config get <key> [--reveal-secrets]
helix-org config list [--prefix <prefix>]
helix-org config delete <key>
```

`<value>` is interpreted as JSON if it parses as such, otherwise
as a string. Most calls are short:

```bash
helix-org config set claude.model claude-opus-4-7
helix-org config set claude.public_url "https://abc123.ngrok.app"
helix-org config set transport.postmark '{"token":"abc123","inbound":"xyz@inbound.postmarkapp.com","from":"you@gmail.com"}'
helix-org config set dispatcher.timeout_seconds 300
```

Reads default to **redacting secrets** (the value is replaced with
`"..."` for fields the schema marks secret). To see plaintext,
opt in explicitly:

```bash
helix-org config get transport.postmark --reveal-secrets
```

`list` is the discoverability surface — every key any subsystem
has registered, with current value (redacted), default, required
flag, and a one-line description from the registry:

```
$ helix-org config list
KEY                          VALUE                       REQUIRED  DESCRIPTION
claude.bin                   claude                      yes       Path to the claude CLI binary.
claude.model                 claude-opus-4-7             no        Claude model passed via --model. Empty = let claude choose.
claude.public_url            https://abc123.ngrok.app    yes       Base URL Workers reach helix-org's MCP endpoint at.
dispatcher.timeout_seconds   300                         no        Max activation duration before kill.
transport.postmark           {"token":"...","from":...}  no        Postmark account config. Required if any stream uses transport=email.
```

The CLI opens the DB file directly — same path the server uses,
same approach `helix-org bootstrap` already takes. SQLite's WAL
mode handles concurrent-writer-while-server-running cleanly:
config writes commit, the server picks up the new value on its
next read.

## Schema validation registry

Each subsystem registers what it expects when it boots:

```go
type ConfigSpec struct {
    Key         string
    Schema      *jsonschema.Schema
    Default     json.RawMessage    // nil = no default; missing key is an error if Required && no default
    Required    bool
    Secrets     []string            // JSON paths within the value that are secret, e.g. ["token"]
    Description string              // one-line for `list`
}
```

Example, when `dispatch` boots:

```go
configs.Register(ConfigSpec{
    Key:         "claude.bin",
    Schema:      stringSchema,
    Default:     json.RawMessage(`"claude"`),
    Required:    true,
    Description: "Path to the claude CLI binary.",
})
```

When the (future) email transport boots:

```go
configs.Register(ConfigSpec{
    Key:         "transport.postmark",
    Schema:      postmarkConfigSchema,
    Required:    false,             // optional unless a stream uses transport=email
    Secrets:     []string{"token"},
    Description: "Postmark account config. Required if any stream uses transport=email.",
})
```

`set` validates the new value against the schema before upserting
— bad shape, no DB write, error to the operator. `get` and `list`
walk the JSON, replacing the paths in `Secrets` with `"..."` unless
`--reveal-secrets`. `list` enumerates every registered key, even
ones with no row yet, so the operator can see what's *settable*,
not just what's set.

## Reading at use-time

Each consumer reads its config from the DB on each operation:

```go
func (s *spawner) spawn(ctx context.Context, ...) error {
    bin, err := s.configs.GetString(ctx, "claude.bin")
    if err != nil {
        return fmt.Errorf("read claude.bin: %w", err)
    }
    // ...
}
```

The accessor methods (`GetString`, `GetInt`, `GetObject[T]`)
handle the common cases:

- Missing key → return registered default; if no default and
  `Required: true`, error.
- Malformed value → error.
- Secrets are returned as-is to consumers — the redaction is a
  CLI presentation concern only. The spawner needs the real
  token to call Postmark.

One SQLite query per operation. With WAL mode and SQLite's page
cache this is cheap; if a hot path turns out to read the same key
thousands of times per second, layer a TTL cache or a write-
broadcast watcher on top. **Phase 1: read every time, simple.**

Live updates Just Work. Change `claude.model`, the next activation
picks it up. Rotate `transport.postmark.token`, the next email
send uses the new one. No restart, no signal handling, no cache
invalidation logic.

## Bootstrap flow

After `helix-org bootstrap`, the operator runs the CLI to populate
operational config:

```bash
helix-org bootstrap --db /var/helix/helix.db --envs-dir /var/helix/envs
helix-org config set claude.public_url "https://helix.example.com"
helix-org config set claude.model "claude-opus-4-7"
# ...any transport configs needed for that installation.
helix-org serve --db /var/helix/helix.db --envs-dir /var/helix/envs
```

`helix-org config list` shows what's still missing — keys marked
`Required: true` with no row and no default surface clearly,
telling the operator what to set next.

A future `helix-org bootstrap --interactive` could walk through
every required key, prompting for values. Out of scope for now —
the four-line block above is fine.

## Open questions

- **Direct DB access vs HTTP admin endpoint.** Phase 1 is direct
  file access (CLI on the same host as the DB). For remote
  deployments we'll want an authenticated HTTP admin endpoint;
  the CLI then becomes a thin client over that. Decide when
  production deployment becomes a thing — until then, SSH is the
  remote-management story.

- **Audit history.** Today only `updated_at` and `updated_by` on
  the current row. A `config_changes` history table would let
  operators review changes over time ("who set
  `transport.postmark.token` last week?"). Cheap to add later;
  not Phase 1.

- **Per-environment overlays.** Production vs staging vs dev:
  same code, different config. The CLI handles this by pointing
  at different DB files (`--db production.db` vs `--db staging.db`).
  If we ever want overlays in one DB ("production with these
  specific overrides for the dev rig") that's an explicit overlay
  layer we'd add deliberately. Defer.

- **Required-with-no-default boot behaviour.** When the server
  boots and a `Required: true` key has no row and no default, do
  we (a) refuse to start, (b) start with that subsystem dormant
  and fail loudly when something tries to use it, or (c) start
  with a warning? Lean toward **(b) per-key**: a missing
  `transport.postmark` shouldn't prevent the server from running
  for users who only use webhook streams, so the email transport
  boots dormant. But a missing `claude.bin` *is* fatal because
  every activation would fail. The required-ness is per-key, not
  per-installation.

- **Encryption at rest.** Production deployment lever; SQLCipher
  is the path of least resistance, per-field encryption with a
  master key from env is more flexible. Don't build until needed.

- **`--reveal-secrets` audit trail.** Every reveal could be
  logged to a separate audit stream so operators can see what
  was viewed when. Today it's local-only by design (someone
  with shell access to the host can already cat the DB), but
  the audit hook is cheap. Open question whether to bother in
  Phase 1.
