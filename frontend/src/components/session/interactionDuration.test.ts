import { describe, expect, it } from "vitest";

import {
  getInteractionDurationMs,
  getInteractionRequestTimeMs,
} from "./interactionDuration";

describe("getInteractionRequestTimeMs", () => {
  it("uses the persisted interaction creation time", () => {
    expect(
      getInteractionRequestTimeMs(
        { created: "2026-08-03T00:00:00.000Z" },
        Date.parse("2026-08-03T00:02:05.000Z"),
      ),
    ).toBe(Date.parse("2026-08-03T00:00:00.000Z"));
  });

  it("uses the fallback when the creation time is missing or invalid", () => {
    expect(getInteractionRequestTimeMs({}, 125000)).toBe(125000);
    expect(getInteractionRequestTimeMs({ created: "invalid" }, 125000)).toBe(125000);
  });
});

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
