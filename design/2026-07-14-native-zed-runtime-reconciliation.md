# Native Zed Runtime Reconciliation

## Symptom

Every user message in a native Zed Agent session inserted an "Agent switched to zed_agent" interaction and opened a new Zed thread.

## Root cause

`CodeAgentRuntime.ZedAgentName()` returned an empty string for the native runtime, while Zed reported and Helix persisted the same runtime as `zed-agent`. Runtime reconciliation treated those two representations as different and cleared the thread before each turn.

## Fix

Use `zed-agent` as the canonical native agent name. Zed explicitly maps that name to `NativeAgent` in its create, load, and reopen paths.

## Verification

- Focused server reconciliation tests pass.
- Affected Go packages build.
- Live Zed test not run: the local Docker stack is absent and the UTM VM on port 2222 is not running.
