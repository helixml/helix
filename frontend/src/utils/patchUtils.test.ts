import { describe, it, expect } from "vitest";
import { applyPatch } from "./patchUtils";

// Regression coverage for the live chat truncation bug (task 002552).
// A dropped best-effort NATS patch used to be swallowed by a "pure append" branch,
// splicing the two sides of the hole together and leaving permanently corrupt text with
// no error anywhere. See design/2026-08-04-chat-message-truncation-clobber.md.
describe("applyPatch", () => {
  it("applies the first patch to empty content", () => {
    expect(applyPatch("", 0, "Hello", 5)).toBe("Hello");
  });

  it("appends when the offset is exactly the current length", () => {
    expect(applyPatch("Hello", 5, " world", 11)).toBe("Hello world");
  });

  it("applies a backwards edit, replacing the tail", () => {
    // e.g. a tool_call entry whose status flips from Pending to Completed
    expect(applyPatch("Status: Pending", 8, "Completed", 17)).toBe(
      "Status: Completed",
    );
  });

  it("handles omitempty zero values arriving as undefined", () => {
    expect(
      applyPatch("", undefined as any, "abc", 3),
    ).toBe("abc");
    expect(applyPatch("", 0, undefined as any, 0)).toBe("");
  });

  it("returns null when a patch was dropped (offset beyond current content)", () => {
    // Client holds "Al"; the patch that carried the next 131 chars was dropped, so this
    // patch starts at offset 133. Appending here would produce "Albs the spiral..." —
    // the exact real-world corruption this guards against.
    expect(applyPatch("Al", 133, "bs the spiral iron stair", 157)).toBeNull();
  });

  it("returns null when total_length disagrees with the reconstruction", () => {
    // Checksum mismatch means our baseline differs from the server's accumulator.
    expect(applyPatch("Hello", 5, " world", 999)).toBeNull();
  });

  it("never silently shortens content to total_length", () => {
    // The old implementation sliced content down to total_length, destroying data
    // mid-word. A disagreement must surface as divergence, not as a lossy truncation.
    const result = applyPatch("Hello world", 11, "!", 5);
    expect(result).toBeNull();
  });

  it("reconstructs a realistic streaming sequence exactly", () => {
    const chunks = ["The keeper ", "climbs the ", "spiral stair"];
    let content = "";
    let offset = 0;
    for (const chunk of chunks) {
      const next = applyPatch(content, offset, chunk, offset + chunk.length);
      expect(next).not.toBeNull();
      content = next as string;
      offset = content.length;
    }
    expect(content).toBe("The keeper climbs the spiral stair");
  });
});
