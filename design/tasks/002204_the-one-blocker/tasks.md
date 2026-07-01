# Implementation Tasks: Roll Out and Verify create_bot/subscribe MCP Array-Parameter Fix

The code fix is already at HEAD (#2774; server side #2768). This task is
deploy + live verification, not a re-implementation. Only write code if the
contingency branch is triggered.

## Confirm the fix is present at HEAD

- [ ] Confirm `git log` on helix `main` includes `169cb6421` (#2774) and #2768.
- [ ] Run `go test ./api/pkg/org/interfaces/mcptools/... ./api/pkg/agent/skill/mcp/...`
      and confirm `TestSchemaWireArrayParams` and `buildparameters_union_test` pass.

## Confirm the runtime runs the fixed build

- [ ] Identify which stack the reporting Bot connects to (inner Helix API +
      desktop image) and check the **deployed** commit, not just the repo.
- [ ] If the API container predates #2768/#2774, rebuild/redeploy it (dev: Air
      hot-reload at HEAD; prod: version bump per the release runbook).
- [ ] If the Bot's tool schema comes from the desktop harness, confirm the
      desktop image (`helix-ubuntu:<sha>` / pinned `sandbox-versions.txt`)
      includes the consumer fix; rebuild if stale.

## Verify live (close the gap 002203 left open)

- [ ] Against the live org MCP endpoint, run `tools/list` and assert
      `create_bot.tools`, `create_bot.topics`, `attach_tool.tools`,
      `detach_tool.tools`, `subscribe.topicIds`, `unsubscribe.topicIds` are all
      `type:"array"` with `items.type:"string"` (never `string`, never a union).
- [ ] Make a live `create_bot` call passing `tools` and `topics` as real JSON
      arrays; confirm it succeeds (no `cannot unmarshal string into []string`).
- [ ] Make a live `subscribe` call passing `topicIds` as a JSON array; confirm
      success.

## Re-activate the Bot

- [ ] Trigger a fresh activation of the reporting Bot so it re-reads the
      corrected schema, and confirm it proceeds past its schema check.

## Contingency (only if live schema/LLM still shows string)

- [ ] Locate the desktop harness MCP schema converter (e.g.
      `qwen-code/.../tools/mcp-client.ts`) and check for array/nullable-union
      → string collapse.
- [ ] Apply the same non-null type resolution used by `resolveSchemaType`.
- [ ] Rebuild the desktop/qwen image, bump `sandbox-versions.txt`, re-verify
      the live `tools/list` and `create_bot`/`subscribe` calls.

## Wrap up

- [ ] Record the deployed commit and the live `create_bot`/`subscribe` results
      (success output) as evidence.
- [ ] Check CI green if any code changed (contingency only).
