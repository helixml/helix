import { useMutation, useQueryClient } from "@tanstack/react-query";
import useApi from "../hooks/useApi";
import { TypesSpecTask } from "../api/api";
import { invalidateSpecTaskStatusQueries } from "./specTaskService";

/**
 * Statuses in which an agent is working on the task, so it can be marked done
 * (finished, or abandoned when it turned out not to need a pull request).
 * Tasks never become done on their own — not even when their PRs merge.
 */
const COMPLETABLE_STATUSES: ReadonlySet<string> = new Set([
  "spec_generation",
  "spec_review",
  "spec_revision",
  "implementation",
  "implementation_review",
  "pull_request",
]);

export function canMarkDone(task: { status?: string; archived?: boolean }): boolean {
  return !task.archived && COMPLETABLE_STATUSES.has(task.status ?? "");
}

export function openPullRequestCount(task: {
  repo_pull_requests?: { pr_state?: string }[];
}): number {
  return (task.repo_pull_requests ?? []).filter(
    (pr) => (pr.pr_state ?? "").toLowerCase() === "open",
  ).length;
}

/** Identifies the agent's pending completion request, if any, for one-time reveal. */
export function completionRequestKey(task: TypesSpecTask | undefined): string {
  return task?.completion_requested_at && canMarkDone(task)
    ? `completion:${task.completion_requested_at}`
    : "";
}

export function useDecideCompletion(specTaskId: string) {
  const api = useApi();
  const apiClient = api.getApiClient();
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async ({
      decision,
      comment,
    }: {
      decision: "approve" | "reject";
      comment?: string;
    }) => {
      const response = await apiClient.v1SpecTasksCompletionDecideCreate(
        specTaskId,
        { decision, comment },
      );
      return response.data;
    },
    onSettled: () => {
      invalidateSpecTaskStatusQueries(queryClient, specTaskId);
      queryClient.invalidateQueries({ queryKey: ["attention-events"] });
    },
  });
}
