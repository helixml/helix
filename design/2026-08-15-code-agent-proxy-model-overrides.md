# Code-agent proxy model override RCA

## Incident

SpecTask `spt_01m029jyfb6mktngv1mh577cfm` was configured to run OpenCode with
provider `pe_01kzpnf69hf73basd52k942vs3` and model `qwen3.8-27b`. Its proxy
calls were instead dispatched as `deepseek-v4-flash`, the reusable Agent's
stored default, and failed because that model was not available upstream.

## Evidence

- The task row and both execution-config endpoints reported `qwen3.8-27b`.
- The session's live `/zed-config` reported
  `ds4-flash-node06/qwen3.8-27b`.
- The running OpenCode process had
  `model=helix/ds4-flash-node06/qwen3.8-27b` in
  `OPENCODE_CONFIG_CONTENT`.
- OpenCode's own log recorded every attempted stream with
  `modelID=ds4-flash-node06/qwen3.8-27b`.
- Helix LLM call `llmc_01m02aa01cfe9yxmndszknazrp` recorded
  `deepseek-v4-flash` and the upstream 404.

This rules out the picker, persistence, Zed config, settings sync, and
OpenCode. The rewrite occurred inside the Helix OpenAI-compatible proxy.

## Root cause

The session-scoped API key correctly attributed the proxy call to the coding
Agent by setting `ChatCompletionOptions.AppID`. The controller then loaded the
reusable Agent and unconditionally replaced the incoming request's model and
provider with that Agent's stored assistant defaults. It did not apply the
session or SpecTask `CodeAgentOverrides`, even though those overrides had been
used to generate the code-agent configuration.

Normal chat worked because it did not enter this app-backed proxy path.

## Fix

Proxy attribution now carries the authoritative code-agent overrides:

- session overrides for exploratory/project coding sessions;
- SpecTask overrides and Agent ID for task sessions.

The controller applies those overrides to a copy of the loaded Agent before
performing its existing authoritative model selection. This preserves the
security property that a sandbox cannot select an arbitrary model while making
the controller agree with the model configuration Helix issued to the sandbox.

## Verification

- Controller and proxy-attribution unit tests pass.
- The affected server packages build successfully.
- After hot reload, sending `Reply with exactly: PROXY_OVERRIDE_OK` through the
  original SpecTask completed successfully. LLM call
  `llmc_01m02b7wxggf0sa606yknqyf1b` used provider
  `pe_01kzpnf69hf73basd52k942vs3`, model `qwen3.8-27b`, produced 120 completion
  tokens, and had no error.
