# Spike Results: Zed startup impact of configuring many agents

**Method.** Ran the real Zed binary (`/zed-build/zed` from `helix-ubuntu:babf01`)
headless under Xvfb as user `retro`, with crafted `settings.json` variants, over
a 25s observation window. Sampled CPU (utime+stime from `/proc`), RSS, and child
process count for the zed tree and for spawned MCP processes. Script:
`spike/run_spike.sh`. Host: 48 cores. **Caveat:** software rendering (llvmpipe)
gives a high, noisy CPU floor (~22s CPU/25s window at idle), so small CPU deltas
are within noise; RSS and process-count are the clean signals.

## Numbers

| Variant | agent_servers | context_servers | procs spawned | zed RSS | MCP RSS | TOTAL RSS | zed CPU |
|---|---|---|---|---|---|---|---|
| baseline (run A) | 1 | 0 | 0 | 328 MB | 0 | 328 MB | 22.2s |
| **agents100 (run A)** | **100** | 0 | **0** | **328 MB** | 0 | **328 MB** | 23.1s |
| baseline (run B) | 1 | 0 | 0 | 330 MB | 0 | 330 MB | 22.5s |
| **agents100 (run B)** | **100** | 0 | **0** | **329 MB** | 0 | **329 MB** | 22.8s |
| **mcp100** (per-agent MCP union) | 1 | **100** | **100** | 332 MB | **3932 MB** | **4265 MB** | 21.0s |

## Findings

1. **Configuring 100 `agent_servers` is essentially free.**
   - **Zero** subprocesses spawned, RSS **flat** (328→329 MB), CPU delta ~+0.6s
     (within the llvmpipe noise floor). The agent_server processes spawn lazily
     on first thread use (`AgentConnectionCache`), so a long list only costs JSON
     parse + registry bookkeeping. This confirms the code analysis.
   - (An earlier single run showed +13s; repeated runs show it was renderer
     noise, not real.)

2. **The cost is MCP `context_servers`, not agents.**
   - 100 context servers spawned **100 processes at startup** and consumed
     **~3.9 GB RSS** (≈13× baseline) — and these were *do-nothing* stubs. Real
     MCP servers (npx-based, actual tool init) would cost materially more CPU and
     add an `npx` download/spawn storm on a cold container.
   - Zed shares `context_servers` per-project across all agents, so unioning
     every agent's MCPs is both expensive (above) and semantically wrong (no
     per-agent isolation).

## Decision implication (for interactive sign-off)

The all-vs-selective question splits cleanly along the agent_servers/MCP line:

- **Pre-configuring all agents' `agent_servers` is cheap and safe** — it enables
  instant switching with no reconfiguration and lets users start a new thread
  with any agent from Zed's own UI.
- **MCP `context_servers` must stay scoped to the selected agent** (rewritten by
  the daemon on switch), regardless of the agent-list decision — both for
  startup cost and because Zed can't isolate them per-agent.

→ Recommended: **Hybrid** — list all agents' `agent_servers` up front; keep
`context_servers` scoped to the active agent. This is Strategy A for the agent
list (no per-switch restart needed to make the agent resolvable) plus the
selective-MCP discipline of Strategy B. Awaiting reviewer confirmation before
building.
