import { describe, expect, it } from "vitest";
import { TypesSpecTaskStatus } from "../../api/api";
import {
  isSpecTaskPlanningWorkspace,
  shouldLoadSpecTaskDesignReviews,
} from "./specTaskPlanningWorkspace";

describe("spec task planning workspace", () => {
  it.each([
    TypesSpecTaskStatus.TaskStatusQueuedSpecGeneration,
    TypesSpecTaskStatus.TaskStatusSpecGeneration,
    TypesSpecTaskStatus.TaskStatusSpecReview,
    TypesSpecTaskStatus.TaskStatusSpecRevision,
    TypesSpecTaskStatus.TaskStatusSpecApproved,
  ])("uses the planning workspace for %s", (status) => {
    expect(isSpecTaskPlanningWorkspace(status)).toBe(true);
  });

  it.each([
    TypesSpecTaskStatus.TaskStatusQueuedImplementation,
    TypesSpecTaskStatus.TaskStatusImplementationQueued,
    TypesSpecTaskStatus.TaskStatusImplementation,
    TypesSpecTaskStatus.TaskStatusPullRequest,
    TypesSpecTaskStatus.TaskStatusDone,
  ])("uses the implementation workspace for %s", (status) => {
    expect(isSpecTaskPlanningWorkspace(status)).toBe(false);
  });

  it("loads reviews from either the push timestamp or a review status", () => {
    expect(
      shouldLoadSpecTaskDesignReviews(
        TypesSpecTaskStatus.TaskStatusSpecGeneration,
        "2026-09-14T08:43:06Z",
      ),
    ).toBe(true);
    expect(
      shouldLoadSpecTaskDesignReviews(TypesSpecTaskStatus.TaskStatusSpecReview),
    ).toBe(true);
    expect(
      shouldLoadSpecTaskDesignReviews(TypesSpecTaskStatus.TaskStatusSpecGeneration),
    ).toBe(false);
  });
});
