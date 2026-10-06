import { TypesSpecTaskStatus } from "../../api/api";

const PLANNING_WORKSPACE_STATUSES = new Set<TypesSpecTaskStatus>([
  TypesSpecTaskStatus.TaskStatusQueuedSpecGeneration,
  TypesSpecTaskStatus.TaskStatusSpecGeneration,
  TypesSpecTaskStatus.TaskStatusSpecReview,
  TypesSpecTaskStatus.TaskStatusSpecRevision,
  TypesSpecTaskStatus.TaskStatusSpecApproved,
]);

const PUBLISHED_PLAN_STATUSES = new Set<TypesSpecTaskStatus>([
  TypesSpecTaskStatus.TaskStatusSpecReview,
  TypesSpecTaskStatus.TaskStatusSpecRevision,
  TypesSpecTaskStatus.TaskStatusSpecApproved,
]);

export function isSpecTaskPlanningWorkspace(status?: TypesSpecTaskStatus): boolean {
  return status !== undefined && PLANNING_WORKSPACE_STATUSES.has(status);
}

export function shouldLoadSpecTaskDesignReviews(
  status?: TypesSpecTaskStatus,
  designDocsPushedAt?: string,
): boolean {
  return Boolean(
    designDocsPushedAt || (status !== undefined && PUBLISHED_PLAN_STATUSES.has(status)),
  );
}
