import { act, renderHook } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { usePersistedChoice, usePersistedFlag } from "./workspacePreferences";

const LAYOUTS = ["unified", "split"] as const;

describe("workspace inspector preferences", () => {
  beforeEach(() => localStorage.clear());

  it("starts from the default and persists the reviewer's choice", () => {
    const { result } = renderHook(() => usePersistedChoice("layout", LAYOUTS, "unified"));
    expect(result.current[0]).toBe("unified");

    act(() => result.current[1]("split"));

    expect(result.current[0]).toBe("split");
    expect(renderHook(() => usePersistedChoice("layout", LAYOUTS, "unified")).result.current[0]).toBe("split");
  });

  it("ignores a stored value that is no longer a valid option", () => {
    localStorage.setItem("helix.workspace-inspector.layout", "side-by-side-v2");
    const { result } = renderHook(() => usePersistedChoice("layout", LAYOUTS, "unified"));
    expect(result.current[0]).toBe("unified");
  });

  it("round-trips boolean toggles", () => {
    const { result } = renderHook(() => usePersistedFlag("word-wrap"));
    expect(result.current[0]).toBe(false);

    act(() => result.current[1](true));

    expect(result.current[0]).toBe(true);
    expect(renderHook(() => usePersistedFlag("word-wrap")).result.current[0]).toBe(true);
  });

  it("falls back to the default when storage is unavailable", () => {
    // Private windows and some embedded contexts throw on access; a lost
    // preference must not take the inspector down with it.
    const getItem = vi.spyOn(Storage.prototype, "getItem").mockImplementation(() => {
      throw new Error("SecurityError");
    });
    const setItem = vi.spyOn(Storage.prototype, "setItem").mockImplementation(() => {
      throw new Error("SecurityError");
    });

    const { result } = renderHook(() => usePersistedChoice("layout", LAYOUTS, "unified"));
    expect(result.current[0]).toBe("unified");
    act(() => result.current[1]("split"));
    expect(result.current[0]).toBe("split");

    getItem.mockRestore();
    setItem.mockRestore();
  });
});
