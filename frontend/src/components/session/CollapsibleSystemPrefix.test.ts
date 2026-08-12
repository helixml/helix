import { describe, it, expect } from "vitest";
import { splitSystemPrefix } from "./CollapsibleSystemPrefix";

describe("splitSystemPrefix", () => {
  it("returns null prefix when message has no marker", () => {
    const result = splitSystemPrefix("just a regular message");
    expect(result.prefix).toBeNull();
    expect(result.userText).toBe("just a regular message");
    expect(result.label).toBeNull();
    expect(result.kind).toBeNull();
  });

  it("returns null prefix for empty message", () => {
    const result = splitSystemPrefix("");
    expect(result.prefix).toBeNull();
    expect(result.userText).toBe("");
    expect(result.kind).toBeNull();
  });

  it("splits on **User Request:** marker", () => {
    const message =
      "Planning prompt here.\nLine two of plan.\n\n**User Request:**\nplease build me a thing";
    const result = splitSystemPrefix(message);
    expect(result.prefix).toBe("Planning prompt here.\nLine two of plan.");
    expect(result.userText).toBe("please build me a thing");
    expect(result.label).toBe("User Request");
    expect(result.kind).toBe("user-request");
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

  it("collapses the spec-approved implementation prompt as kind 'approval' with empty userText", () => {
    const message =
      "## CURRENT PHASE: IMPLEMENTATION\n\nYou are now in the IMPLEMENTATION phase. The planning/spec-writing phase is complete.\n- Your design has been approved - now implement the code changes\n\n# Design Approved - Begin Implementation\n\nSpeak English.";
    const result = splitSystemPrefix(message);
    expect(result.kind).toBe("approval");
    expect(result.prefix).toBe(message);
    expect(result.userText).toBe("");
    expect(result.label).toBeNull();
  });

  it("does not collapse approval-style text that appears mid-message", () => {
    const message =
      "I was reading the docs and saw\n\n## CURRENT PHASE: IMPLEMENTATION\n\nwhat does that mean?";
    const result = splitSystemPrefix(message);
    expect(result.kind).toBeNull();
    expect(result.prefix).toBeNull();
    expect(result.userText).toBe(message);
  });

  it("user-request marker wins over approval anchor if both somehow appear (user-request is checked first)", () => {
    // Defensive: if a message contains both, the user-request split path
    // wins because it carries an explicit user body. Approval is the
    // pure-system fallback.
    const message =
      "## CURRENT PHASE: IMPLEMENTATION something\n\n**User Request:**\nhello";
    const result = splitSystemPrefix(message);
    expect(result.kind).toBe("user-request");
    expect(result.userText).toBe("hello");
  });
});
