import { describe, it, expect } from "vitest";
import { splitSystemPrefix } from "./CollapsibleSystemPrefix";

describe("splitSystemPrefix", () => {
  it("returns null prefix when message has no marker", () => {
    const result = splitSystemPrefix("just a regular message");
    expect(result.prefix).toBeNull();
    expect(result.userText).toBe("just a regular message");
    expect(result.label).toBeNull();
  });

  it("returns null prefix for empty message", () => {
    const result = splitSystemPrefix("");
    expect(result.prefix).toBeNull();
    expect(result.userText).toBe("");
  });

  it("splits on **User Request:** marker", () => {
    const message =
      "Planning prompt here.\nLine two of plan.\n\n**User Request:**\nplease build me a thing";
    const result = splitSystemPrefix(message);
    expect(result.prefix).toBe("Planning prompt here.\nLine two of plan.");
    expect(result.userText).toBe("please build me a thing");
    expect(result.label).toBe("User Request");
  });

  it("splits on **Original Request (for context only...):** marker", () => {
    const message =
      'Planning prompt here.\n\n**Original Request (for context only - any questions have already been resolved in the specs):**\n> "do the thing"';
    const result = splitSystemPrefix(message);
    expect(result.prefix).toBe("Planning prompt here.");
    expect(result.userText).toBe('> "do the thing"');
    expect(result.label).toContain("Original Request");
  });

  it("does not split when the marker is at the very start (no prefix)", () => {
    // Without a leading prefix + blank line before the marker, regex does not match
    const result = splitSystemPrefix("**User Request:**\nhello");
    expect(result.prefix).toBeNull();
    expect(result.userText).toBe("**User Request:**\nhello");
  });

  it("trims whitespace from both prefix and userText", () => {
    const message =
      "  Planning prompt with whitespace  \n\n**User Request:**\n   user message   ";
    const result = splitSystemPrefix(message);
    expect(result.prefix).toBe("Planning prompt with whitespace");
    expect(result.userText).toBe("user message");
  });

  it("handles multiline user request body", () => {
    const message =
      "Planning prompt.\n\n**User Request:**\nLine 1\nLine 2\n\nParagraph 2";
    const result = splitSystemPrefix(message);
    expect(result.userText).toBe("Line 1\nLine 2\n\nParagraph 2");
  });
});
