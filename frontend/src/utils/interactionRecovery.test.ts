import { describe, expect, it } from "vitest";

import { TypesInteractionState } from "../api/api";
import { lastSuccessfulInteractionIndex } from "./interactionRecovery";

const complete = (id: string) => ({
  id,
  state: TypesInteractionState.InteractionStateComplete,
});
const failed = (id: string) => ({
  id,
  state: TypesInteractionState.InteractionStateError,
  error: "agent turn aborted",
});
const running = (id: string) => ({
  id,
  state: TypesInteractionState.InteractionStateWaiting,
});

describe("lastSuccessfulInteractionIndex", () => {
  it("returns -1 when nothing has succeeded", () => {
    expect(lastSuccessfulInteractionIndex([failed("a"), failed("b")])).toBe(-1);
    expect(lastSuccessfulInteractionIndex([])).toBe(-1);
    expect(lastSuccessfulInteractionIndex(undefined)).toBe(-1);
  });

  it("finds the last clean completion, so earlier failures count as overtaken", () => {
    // The incident shape: a turn dies, the session carries on and answers
    // something else. Index 0 is before index 2, so its alarm is stale.
    const interactions = [failed("a"), failed("b"), complete("c")];
    expect(lastSuccessfulInteractionIndex(interactions)).toBe(2);
  });

  it("ignores a completed interaction that carries an error", () => {
    // A turn can reach a terminal state and still have failed. Treating that
    // as a recovery point would hide the very error the user needs to see.
    const withError = {
      id: "x",
      state: TypesInteractionState.InteractionStateComplete,
      error: "boom",
    };
    expect(lastSuccessfulInteractionIndex([complete("a"), withError])).toBe(0);
  });

  it("does not count a turn that is still running", () => {
    // An in-flight turn has not recovered anything yet — the previous failure
    // is still the latest thing that actually happened.
    expect(lastSuccessfulInteractionIndex([failed("a"), running("b")])).toBe(-1);
  });
});
