import { describe, expect, it } from "vitest";

import { getInteractionDurationMs } from "./interactionDuration";

describe("getInteractionDurationMs", () => {
  it("prefers the server measured duration", () => {
    expect(
      getInteractionDurationMs({
        duration_ms: 2500,
        created: "2026-08-03T00:00:00.000Z",
        completed: "2026-08-03T00:01:00.000Z",
      }),
    ).toBe(2500);
  });

  it("falls back to interaction timestamps", () => {
    expect(
      getInteractionDurationMs({
        duration_ms: 0,
        created: "2026-08-03T00:00:00.000Z",
        completed: "2026-08-03T00:00:02.500Z",
      }),
    ).toBe(2500);
  });
});
