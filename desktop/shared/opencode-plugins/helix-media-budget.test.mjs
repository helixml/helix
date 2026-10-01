import assert from "node:assert/strict"
import test from "node:test"

import helixMediaBudget from "./helix-media-budget.mjs"

test("keeps newest images within budget and removes older duplicates and overflow", async () => {
  const newest = "data:image/png;base64,bmV3ZXN0"
  const oversized = `data:image/png;base64,${"A".repeat(6 * 1024 * 1024)}`
  const oldState = {
    status: "completed",
    input: { filePath: "/tmp/older.png" },
    output: "old output",
    attachments: [
      { mime: "text/plain", url: "data:text/plain;base64,dGV4dA==" },
      { mime: "image/png", url: newest },
      { mime: "image/png", url: oversized },
    ],
  }
  const newState = {
    status: "completed",
    output: "new output",
    attachments: [{ mime: "image/png", url: newest }],
  }
  const output = {
    messages: [
      { info: {}, parts: [{ type: "tool", state: oldState }] },
      { info: {}, parts: [{ type: "tool", state: newState }] },
    ],
  }

  const plugin = await helixMediaBudget()
  await plugin["experimental.chat.messages.transform"]({}, output)

  assert.deepEqual(newState.attachments, [{ mime: "image/png", url: newest }])
  assert.deepEqual(oldState.attachments, [{ mime: "text/plain", url: "data:text/plain;base64,dGV4dA==" }])
  assert.match(oldState.output, /duplicate of a newer image/)
  assert.match(oldState.output, /aggregate image budget exceeded/)
  assert.match(oldState.output, /Source: "\/tmp\/older\.png"/)
})
