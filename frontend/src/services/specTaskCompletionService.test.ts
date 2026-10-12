import { describe, expect, it } from "vitest";

import { TypesSpecTaskStatus } from "../api/api";
import {
  canMarkDone,
  completionRequestKey,
  openPullRequestCount,
} from "./specTaskCompletionService";

describe("canMarkDone", () => {
  it("allows any status an agent works in, including abandoning before a PR", () => {
    for (const status of ["spec_review", "implementation", "pull_request"]) {
      expect(canMarkDone({ status })).toBe(true);
    }
  });

  it("excludes finished, unstarted and archived tasks", () => {
    expect(canMarkDone({ status: "done" })).toBe(false);
    expect(canMarkDone({ status: "backlog" })).toBe(false);
    expect(canMarkDone({ status: "pull_request", archived: true })).toBe(false);
  });
});

describe("openPullRequestCount", () => {
  it("counts only open pull requests", () => {
    expect(
      openPullRequestCount({
        repo_pull_requests: [{ pr_state: "merged" }, { pr_state: "open" }, { pr_state: "closed" }],
      }),
    ).toBe(1);
    expect(openPullRequestCount({})).toBe(0);
  });
});

describe("completionRequestKey", () => {
  it("identifies a pending request so it is revealed once", () => {
    expect(
      completionRequestKey({ status: TypesSpecTaskStatus.TaskStatusPullRequest, completion_requested_at: "2026-10-07T10:00:00Z" }),
    ).toBe("completion:2026-10-07T10:00:00Z");
  });

  it("is empty with no request, or once the task is done", () => {
    expect(completionRequestKey({ status: TypesSpecTaskStatus.TaskStatusPullRequest })).toBe("");
    expect(
      completionRequestKey({ status: TypesSpecTaskStatus.TaskStatusDone, completion_requested_at: "2026-10-07T10:00:00Z" }),
    ).toBe("");
    expect(completionRequestKey(undefined)).toBe("");
  });
});
