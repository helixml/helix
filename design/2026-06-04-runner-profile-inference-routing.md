# Runner-profile inference routing: cross-network dispatch + model discovery

Date: 2026-06-04
Branch: fix/runner-profile-inference-routing

## Symptom

Operator created a runner profile ("tiny LLM", serves `qwen2.5-0.5b` via vLLM),
assigned it to a sandbox. Profile pulled, vLLM came up healthy, admin panel
showed `running`. But chat completions for the model failed:

    model "qwen2.5-0.5b" is not configured in the default provider "helix"

## Root causes (three independent bugs)

### Bug A - chat path can't see profile-served models

`InternalHelixServer.ListModels` (`api/pkg/openai/helix_openai_server.go`) builds
its list by iterating the `models` DB table and keeping only rows that are also
present on a runner. A profile-served model exists only in the inference
router's `AvailableModels()`, never in the `models` table, so it is filtered
out. The OpenAI chat path validates the requested model via this same
`ListModels` (`assertProviderServesModel` in `controller/inference.go`), so the
request is rejected before routing.

Fix: in `ListModels`, union the router's `AvailableModels()` into the result.
Models served by a running profile are first-class, even with no `models` row.

### Bug B - dispatch used a direct HTTP dial to the sandbox host

`refreshInferenceRouterFromHeartbeat` derived a URL from the sandbox's
registered hostname / IP and stored it on `RunnerState.URL`;
`dispatchHTTPToRunner` then did `http.Client.Do("http://<host>:8090/...")`.

This can only work when the API can route to the sandbox by address. In a real
deployment the GPU sandbox is on a different network / behind NAT and is NOT
directly reachable - it only has an outbound RevDial WebSocket to the API
(the same tunnel hydra uses for exec/files/terminal). The stored hostname
(`sandbox-local`) was unresolvable, so dispatch failed with
`dial tcp: lookup sandbox-local: server misbehaving`.

The design doc's own cloud smoke test (`design/2026-04-28-cloud-gpu-smoke-results.md`)
records inference being proxied "via the Hydra RevDial WebSocket" - the
direct-HTTP code was a regression from the intended design.

Fix: dispatch over RevDial, addressed by sandbox ID, never by network address.
- hydra (sandbox side) gains an inference proxy route that forwards to the
  local inference-proxy (`localhost:8090`) with SSE streaming.
- the API dials `hydra-<sandboxID>` via connman and speaks HTTP over the
  tunnel, streaming the response back into the existing pubsub queue.
- `RunnerState.URL` and all hostname/IP derivation are deleted - the router
  routes by sandbox ID alone.

### Bug C - heartbeat delivery (resolved: not a bug)

During debugging the in-memory router was empty until a heartbeat with
`profile_status=running` was POSTed. This was an artifact of an API-restart
window: the router is in-memory and is wiped on restart, repopulating on the
next 30s heartbeat. Verified in steady state the daemon's heartbeats land and
keep the model listed with no manual poke. No code change.

## Files touched

- `api/pkg/hydra/server.go` - inference proxy route + streaming handler (sandbox side; needs `./stack build-sandbox`)
- `api/pkg/hydra/client.go` - n/a (API speaks the tunnel directly from openai pkg)
- `api/pkg/openai/helix_openai_server.go` - ListModels union (Bug A); RevDial dispatch (Bug B); dialer wiring
- `api/pkg/inferencerouter/router.go` - drop `RunnerState.URL`
- `api/pkg/server/runner_assignment_handlers.go` - drop URL derivation in refresh
- `api/pkg/server/server.go` - wire connman dialer into the inference server

## Deploy

API changes hot-reload via Air. The hydra route is a sandbox-side change and
requires `./stack build-sandbox` + a new session before end-to-end test.

## Verified (prime, 2026-06-04)

- Unprefixed `POST /v1/chat/completions` for `qwen2.5-0.5b` returns a normal
  completion (Bug A).
- `stream:true` streams SSE chunks end-to-end: API -> RevDial tunnel -> hydra
  /api/v1/inference/* -> inference-proxy -> vLLM (Bug B; flush works).
- Model stays listed via the live daemon heartbeats with no manual poke (Bug C).
