import { describe, expect, it } from "vitest";

import { shouldAutoOpenSpecTaskReview } from "./specTaskAutoOpen";

describe("shouldAutoOpenSpecTaskReview", () => {
  it("keeps spec reviews closed in the Chat task view", () => {
    expect(shouldAutoOpenSpecTaskReview("org_chat-task")).toBe(false);
  });

  it("retains auto-open behavior in the project task view", () => {
    expect(shouldAutoOpenSpecTaskReview("org_project-task-detail")).toBe(true);
  });
});
