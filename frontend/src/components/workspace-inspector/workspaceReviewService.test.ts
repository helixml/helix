import { describe, expect, it } from "vitest";
import {
  desktopPollInterval,
  desktopQueryRetry,
  isDesktopUnavailableError,
} from "./workspaceReviewService";

const stopped = { response: { status: 503 } };
const broken = { response: { status: 500 } };

describe("workspace query behaviour against a stopped sandbox", () => {
  it("recognises only 503 as a stopped desktop", () => {
    expect(isDesktopUnavailableError(stopped)).toBe(true);
    expect(isDesktopUnavailableError(broken)).toBe(false);
    expect(isDesktopUnavailableError(new Error("network"))).toBe(false);
    expect(isDesktopUnavailableError(undefined)).toBe(false);
    expect(isDesktopUnavailableError(null)).toBe(false);
  });

  it("does not retry a stopped desktop", () => {
    // Every retry re-dials RevDial and logs "container not ready" server-side
    // for a container we already know is gone.
    expect(desktopQueryRetry(0, stopped)).toBe(false);
  });

  it("still retries a genuine failure, up to a bound", () => {
    expect(desktopQueryRetry(0, broken)).toBe(true);
    expect(desktopQueryRetry(1, broken)).toBe(true);
    expect(desktopQueryRetry(2, broken)).toBe(false);
  });

  it("stops the poll once the sandbox has answered 503", () => {
    const interval = desktopPollInterval(3000);
    expect(interval({ state: { error: undefined } })).toBe(3000);
    expect(interval({ state: { error: broken } })).toBe(3000);
    expect(interval({ state: { error: stopped } })).toBe(false);
  });
});
